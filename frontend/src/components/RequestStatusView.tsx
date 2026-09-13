'use client';

import { useCallback, useEffect, useRef, useState, type FormEvent } from 'react';
import {
  ApiError,
  cancelRequest,
  createRequest,
  fetchRequestDetails,
  requestDocument,
  requestEventsUrl,
} from '../lib/api';
import type { RequestDetails, RequestEventMessage, RequestStatus } from '../lib/types';
import { MarkdownAnswer } from './MarkdownAnswer';
import { DocumentDownloadButton } from './DocumentDownloadButton';
import { ClarificationForm } from './ClarificationForm';
import { MIN_QUERY_LENGTH, MAX_QUERY_LENGTH, MAX_DOCUMENT_TYPE_LENGTH } from './RequestForm';
import { useDictionary } from '../i18n/DictionaryProvider';
import type { Dictionary } from '../i18n/dictionary';

/**
 * Этап 14: основной канал обновлений — WS (см. requestEventsUrl в lib/api.ts, RealtimeGateway на
 * бэкенде), пуш в реальном времени вместо ожидания следующего тика. Этот интервал теперь только
 * фолбэк — на случай, если WS не смог подключиться или неожиданно оборвался (прокси/файрвол,
 * блокирующий WS, временный сетевой сбой) — тогда ChatExchange возвращается к старому поведению.
 */
const POLL_INTERVAL_MS = 2000;
const TICK_INTERVAL_MS = 1000;

/**
 * Этап 18: поллинг прогресса догенерации документа к уже завершённому ответу (см.
 * requestDocument в lib/api.ts). Общий статус запроса при этом не меняется (остаётся
 * 'completed', см. backend/src/orchestrator/orchestrator.service.ts
 * processDocumentOnlyResume) — поэтому основной WS-канал здесь не помогает (сервер уже закрыл
 * его после снапшота 'completed', см. RealtimeGateway), прогресс отслеживаем отдельным коротким
 * поллингом именно шага 'document'.
 */
const DOCUMENT_POLL_INTERVAL_MS = 2000;
/** Защита от вечного спиннера, если что-то пошло не так на бэкенде и шаг 'document' так и не
 *  дошёл до терминального статуса — после этого просто прекращаем ждать. */
const DOCUMENT_POLL_TIMEOUT_MS = 90_000;

/**
 * Порог для предупреждения "обработка идёт необычно долго" (см. dict.status.stalledWarning) —
 * пайплайн зовёт несколько последовательных LLM-вызовов, часть из них теперь с живым
 * веб-поиском (см. backend/src/agents/*), поэтому легитимный успешный прогон уже может занимать
 * десятки секунд — порог выбран заметно выше типичной длительности, чтобы не пугать зря, но
 * достаточно низким, чтобы реально зависший запрос (не тот сервис отвечает / неверный API-ключ)
 * не выглядел неотличимо от "просто работает".
 */
const STALL_WARNING_SECONDS = 90;

/**
 * 'needs_clarification' тоже "не крутится" сам по себе — пайплайн стоит и ждёт ответ
 * пользователя (см. backend/src/orchestrator/orchestrator.service.ts). Сервер сам закрывает WS
 * после снапшота с таким статусом (см. RealtimeGateway.broadcast), а не только на
 * completed/failed/cancelled; возобновляется вручную через resumeLiveUpdates() после отправки
 * уточнения (ClarificationForm.onSubmitted).
 */
function isPaused(status: RequestStatus): boolean {
  return (
    status === 'completed' ||
    status === 'failed' ||
    status === 'needs_clarification' ||
    status === 'cancelled'
  );
}

/** Те же статусы, что и RequestsRepository.cancelIfActive на бэкенде — только для них кнопка
 *  отмены имеет смысл. */
function isCancelable(status: RequestStatus): boolean {
  return status === 'pending' || status === 'processing' || status === 'needs_clarification';
}

/** Единственные статусы, где пайплайн реально "в полёте" — для них имеет смысл счётчик времени
 *  и предупреждение о зависании; needs_clarification ждёт человека, а не сервис. */
function isActivelyProcessing(status: RequestStatus): boolean {
  return status === 'pending' || status === 'processing';
}

/**
 * Текст "печатает…" под текущий этап — вместо списка всех шагов показываем только тот, что
 * реально сейчас выполняется (последняя по ordinal запись со status='running'; при параллельных
 * editor/document это editor, у него ordinal меньше — оба всё равно про один и тот же общий этап
 * "готовлю ответ"). Пока запись шага ещё не создана (джоба только легла в очередь либо между
 * шагами) — общая формулировка вместо конкретного названия агента.
 */
function currentStageLabel(steps: RequestDetails['steps'], dict: Dictionary): string {
  const running = steps.find((s) => s.status === 'running');
  return running ? dict.pipeline[running.agentName] : dict.status.processingGeneric;
}

function TypingDots() {
  return (
    <span className="inline-flex items-center gap-1" aria-hidden>
      {[0, 150, 300].map((delay) => (
        <span
          key={delay}
          className="h-1.5 w-1.5 animate-bounce rounded-full bg-slate-400"
          style={{ animationDelay: `${delay}ms` }}
        />
      ))}
    </span>
  );
}

/**
 * Один обмен "вопрос пользователя → ответ ассистента" в чате — опрашивает свой requestId
 * независимо от остальных. `onStatusChange` передаётся только для последнего обмена в списке
 * (см. RequestStatusView) — им композер следующего сообщения решает, можно ли уже отправлять
 * новый вопрос, или предыдущий ещё не дошёл до paused-статуса.
 */
function ChatExchange({
  requestId,
  onStatusChange,
}: {
  requestId: string;
  onStatusChange?: (status: RequestStatus) => void;
}) {
  const dict = useDictionary();
  const [details, setDetails] = useState<RequestDetails | null>(null);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [now, setNow] = useState(() => Date.now());
  const [isCancelling, setIsCancelling] = useState(false);
  const [cancelError, setCancelError] = useState<string | null>(null);
  // Потоковый текст финального ответа (см. RequestEventMessage 'token', Агент "answer" —
  // единственный, кто стримит, см. law-answer.agent.ts). Раньше здесь же стримился и черновик
  // отдельного Агента 1 (search) — откачено (Этап 15): пользователь видел меняющийся черновик,
  // который потом переписывался другими агентами, и это подрывало доверие, а не создавало его.
  // Теперь то, что приходит через 'token', — уже финальный ответ, его больше никто не перепишет.
  // После completed рендерится уже resultSummary из снапшота (тот же текст), streamingText
  // сбрасывается.
  const [streamingText, setStreamingText] = useState('');
  const timerRef = useRef<ReturnType<typeof setInterval> | null>(null);
  const wsRef = useRef<WebSocket | null>(null);
  // Счётчик поколений запроса: обычный поллинг-фолбэк и poll() из handleCancel могут
  // пересечься — если ответ более старого вызова прилетит позже, он не должен затереть более
  // свежее состояние. Применяем только результат последнего запущенного poll().
  const pollSeqRef = useRef(0);
  // Поправка на рассинхрон часов клиента с сервером (мс, serverTime - clientTime) — без неё
  // "прошло N сек" и порог "зависания" считались бы от локальных часов клиента напрямую и могли
  // сработать раньше/позже реального времени обработки при неточных часах устройства. Берётся
  // только из REST-ответа (заголовок Date) — WS-сообщения его не несут.
  const clockSkewRef = useRef(0);
  // Актуальный статус для колбэков WS (onclose), которым нужен статус В МОМЕНТ события, а не тот,
  // что был захвачен замыканием при открытии соединения.
  const statusRef = useRef<RequestStatus | null>(null);

  // Этап 18: догенерация документа к уже готовому ответу (см. requestDocument в lib/api.ts) —
  // 'form' показывает поле "какой документ нужен", 'submitting'/'generating' — спиннер.
  const [docState, setDocState] = useState<'idle' | 'form' | 'submitting' | 'generating'>('idle');
  const [docType, setDocType] = useState('');
  const [docError, setDocError] = useState<string | null>(null);
  const docPollRef = useRef<ReturnType<typeof setInterval> | null>(null);
  // true, только если мы реально застали шаг 'document' в статусе 'running' — отличает терминальный
  // статус ИМЕННО этой попытки от уже устаревшего 'failed' с предыдущей (если пользователь
  // изначально ставил галочку при отправке вопроса, см. §9.13, и та попытка не удалась).
  const sawDocRunningRef = useRef(false);

  const stopDocPolling = useCallback(() => {
    if (docPollRef.current) {
      clearInterval(docPollRef.current);
      docPollRef.current = null;
    }
  }, []);

  const stopPollingFallback = useCallback(() => {
    if (timerRef.current) {
      clearInterval(timerRef.current);
      timerRef.current = null;
    }
  }, []);

  const applyDetails = useCallback(
    (data: RequestDetails) => {
      statusRef.current = data.status;
      setDetails(data);
      setLoadError(null);
      if (data.status !== 'processing') setStreamingText('');
      onStatusChange?.(data.status);
      if (isPaused(data.status)) stopPollingFallback();
    },
    [onStatusChange, stopPollingFallback],
  );

  const poll = useCallback(async () => {
    const seq = ++pollSeqRef.current;
    try {
      const clientTimeBeforeRequest = Date.now();
      const { data, serverTimeMs } = await fetchRequestDetails(requestId);
      if (seq !== pollSeqRef.current) return;
      if (serverTimeMs !== null) {
        clockSkewRef.current = serverTimeMs - clientTimeBeforeRequest;
      }
      applyDetails(data);
    } catch (err) {
      if (seq !== pollSeqRef.current) return;
      setLoadError(err instanceof ApiError ? err.message : dict.status.loadError);
      stopPollingFallback();
    }
  }, [requestId, dict, applyDetails, stopPollingFallback]);

  const startPollingFallback = useCallback(() => {
    if (timerRef.current) return;
    timerRef.current = setInterval(() => void poll(), POLL_INTERVAL_MS);
  }, [poll]);

  /**
   * Открывает WS-канал этого запроса (см. requestEventsUrl). Сервер сам закрывает соединение
   * сразу после снапшота с paused-статусом (см. RealtimeGateway.broadcast) — это штатное
   * завершение, а не сбой, поэтому onclose включает поллинг-фолбэк, только если запрос всё ещё
   * "в полёте" на момент закрытия — иначе событий и не предвидится, поллинг не нужен.
   */
  const connectLive = useCallback(() => {
    if (wsRef.current) return;
    const ws = new WebSocket(requestEventsUrl(requestId));
    wsRef.current = ws;

    ws.onmessage = (event) => {
      let message: RequestEventMessage;
      try {
        message = JSON.parse(String(event.data)) as RequestEventMessage;
      } catch {
        return;
      }
      if (message.type === 'snapshot') {
        applyDetails(message.data);
      } else if (message.type === 'token' && message.agentName === 'answer') {
        setStreamingText((prev) => prev + message.delta);
      }
    };
    ws.onclose = () => {
      wsRef.current = null;
      if (statusRef.current && isPaused(statusRef.current)) return;
      startPollingFallback();
    };
  }, [requestId, applyDetails, startPollingFallback]);

  const refreshAndGoLive = useCallback(async () => {
    await poll();
    if (statusRef.current && !isPaused(statusRef.current)) connectLive();
  }, [poll, connectLive]);

  useEffect(() => {
    void refreshAndGoLive();

    return () => {
      stopPollingFallback();
      stopDocPolling();
      // ws.close() асинхронный — onclose всё равно сработает уже после этой функции. Без
      // отвязки обработчиков он увидит "соединение оборвалось" (а не намеренное закрытие при
      // размонтировании) и сам вызовет startPollingFallback() — интервал переживёт компонент
      // и будет вечно опрашивать бэкенд, обновляя состояние уже нигде не отрендеренного
      // ChatExchange (утечка, реального бага была замечена при обрыве во время processing).
      if (wsRef.current) {
        wsRef.current.onclose = null;
        wsRef.current.onmessage = null;
        wsRef.current.close();
        wsRef.current = null;
      }
    };
  }, [refreshAndGoLive, stopPollingFallback, stopDocPolling]);

  // Отдельный, более частый тик (1с) только для счётчика "прошло N сек" в баннере — независимо
  // от статуса обновлений выше, чтобы счётчик шёл плавно, а не скачками.
  const status = details?.status;
  useEffect(() => {
    if (!status || !isActivelyProcessing(status)) return;
    const tick = setInterval(() => setNow(Date.now()), TICK_INTERVAL_MS);
    return () => clearInterval(tick);
  }, [status]);

  // Пайплайн после needs_clarification/cancelled закрывает WS сам (см. connectLive) — после
  // отправки уточнения нужно явно обновиться и переоткрыть соединение, а не просто "продолжить
  // поллинг", как было до Этапа 14.
  const resumeLiveUpdates = useCallback(() => {
    stopPollingFallback();
    void refreshAndGoLive();
  }, [refreshAndGoLive, stopPollingFallback]);

  // Следит за прогрессом уже запущенной догенерации документа (см. handleDocSubmit) — общий
  // статус запроса не меняется, поэтому единственный сигнал прогресса — сам шаг 'document' в
  // очередном снапшоте, полученном обычным поллингом (см. docPollRef).
  useEffect(() => {
    if (docState !== 'generating' || !details) return;
    const documentStep = details.steps.find((s) => s.agentName === 'document');
    if (documentStep?.status === 'running') sawDocRunningRef.current = true;
    const isThisAttemptDone =
      details.document !== null || (documentStep?.status === 'failed' && sawDocRunningRef.current);
    if (isThisAttemptDone) {
      stopDocPolling();
      setDocState('idle');
      setDocType('');
    }
  }, [details, docState, stopDocPolling]);

  async function handleDocSubmit(event: FormEvent<HTMLFormElement>): Promise<void> {
    event.preventDefault();
    const trimmedType = docType.trim();
    if (!trimmedType) return;
    setDocState('submitting');
    setDocError(null);
    try {
      await requestDocument(requestId, trimmedType);
      sawDocRunningRef.current = false;
      setDocState('generating');
      stopDocPolling();
      docPollRef.current = setInterval(() => void poll(), DOCUMENT_POLL_INTERVAL_MS);
      setTimeout(() => {
        stopDocPolling();
        setDocState((current) => (current === 'generating' ? 'idle' : current));
      }, DOCUMENT_POLL_TIMEOUT_MS);
    } catch (err) {
      setDocError(err instanceof ApiError ? err.message : dict.form.genericError);
      setDocState('form');
    }
  }

  async function handleCancel(): Promise<void> {
    // Необратимое действие в один клик легко нажать случайно — короткое подтверждение стоит того.
    if (!window.confirm(dict.status.cancelConfirm)) return;
    setIsCancelling(true);
    setCancelError(null);
    try {
      await cancelRequest(requestId);
      await poll();
    } catch (err) {
      setCancelError(err instanceof ApiError ? err.message : dict.status.cancelError);
    } finally {
      setIsCancelling(false);
    }
  }

  if (loadError) {
    return <p className="text-sm text-red-600">{loadError}</p>;
  }

  if (!details) {
    return <p className="text-sm text-slate-500">{dict.status.loading}</p>;
  }

  const elapsedSeconds = Math.max(
    0,
    Math.floor((now + clockSkewRef.current - new Date(details.createdAt).getTime()) / 1000),
  );
  const activelyProcessing = isActivelyProcessing(details.status);
  const stalled = activelyProcessing && elapsedSeconds >= STALL_WARNING_SECONDS;

  // Причина, по которой шаг document не удался (fail-open, см. runDocumentStep в
  // orchestrator.service.ts) — null, если документ не запрашивался или удался.
  const documentFailure = details.steps.find(
    (s) => s.agentName === 'document' && s.status === 'failed',
  )?.errorMessage;

  const bubbleToneClass =
    details.status === 'failed'
      ? 'border-red-200 bg-red-50 text-red-800'
      : details.status === 'needs_clarification'
        ? 'border-amber-200 bg-amber-50 text-amber-900'
        : details.status === 'cancelled'
          ? 'border-slate-200 bg-slate-50 text-slate-500'
          : 'border-slate-200 bg-white text-slate-800';

  return (
    <div className="flex flex-col gap-4">
      {/* Реплика пользователя — как отправленное сообщение в чате. */}
      <div className="flex justify-end">
        <div className="max-w-[85%] rounded-2xl rounded-br-sm bg-blue-600 px-4 py-2.5 text-sm whitespace-pre-wrap text-white">
          {details.queryText}
        </div>
      </div>

      {/* Реплика ассистента — единственный статус вместо списка шагов, ответ подставляется в
          этот же пузырь, как только пайплайн завершён (никакого отдельного экрана/секции). */}
      <div className="flex justify-start">
        <div
          className={`max-w-[85%] rounded-2xl rounded-bl-sm border px-4 py-3 text-sm ${bubbleToneClass}`}
        >
          {activelyProcessing && (
            <div className="flex flex-col gap-1.5">
              {streamingText ? (
                // Этап 14/15 — финальный ответ печатается по мере генерации вместо
                // статус-лейбла (см. connectLive) — пока модель ищёт страницы через web_search,
                // текста ещё нет, показываем TypingDots ниже; как только она начинает писать
                // ответ, это уже финальный текст (см. law-answer.agent.ts), его больше никто не
                // перепишет.
                <p className="whitespace-pre-wrap text-slate-800">
                  {streamingText}
                  <span className="ml-0.5 inline-block w-1.5 animate-pulse">▍</span>
                </p>
              ) : (
                <div className="flex items-center gap-2 text-slate-600">
                  <TypingDots />
                  <span>{currentStageLabel(details.steps, dict)}</span>
                </div>
              )}
              <p className="text-xs text-slate-400">{dict.status.elapsedLabel(elapsedSeconds)}</p>
              {stalled && (
                <p className="mt-1 text-xs text-amber-700">{dict.status.stalledWarning}</p>
              )}
            </div>
          )}

          {details.status === 'needs_clarification' && details.clarificationQuestion && (
            <div className="flex flex-col gap-2">
              <p>{details.clarificationQuestion}</p>
              <ClarificationForm
                requestId={details.id}
                question={details.clarificationQuestion}
                onSubmitted={resumeLiveUpdates}
              />
            </div>
          )}

          {details.status === 'failed' && (
            <p>
              {dict.status.failedPrefix}
              {details.errorMessage ? `: ${details.errorMessage}` : '.'}
            </p>
          )}

          {details.status === 'cancelled' && <p>{dict.status.cancelledLabel}</p>}

          {details.status === 'completed' && details.resultSummary && (
            <div className="flex flex-col gap-3">
              <MarkdownAnswer markdown={details.resultSummary} />
              {details.document && (
                <div className="border-t border-slate-200 pt-3">
                  <DocumentDownloadButton document={details.document} />
                </div>
              )}
              {/* document — fail-open (см. runDocumentStep в orchestrator.service.ts): основной
                  ответ уже completed, а причина, по которой документ не готов, лежит в самом
                  шаге, а не в errorMessage запроса — details.document остаётся null в этом
                  случае. */}
              {!details.document && documentFailure && (
                <p className="border-t border-slate-200 pt-3 text-xs text-amber-700">
                  {dict.status.documentErrorPrefix(documentFailure)}
                </p>
              )}

              {/* Этап 18: догенерация документа к уже готовому ответу — решает рассинхрон, когда
                  пользователь ставил галочку "приложить документ" не сразу, а уже после того, как
                  увидел ответ: тут агенту document не нужно, чтобы вопрос пересказывали заново в
                  композере, он берёт queryText/resultSummary этого же запроса (см.
                  backend/src/requests/requests.service.ts requestDocument). */}
              {!details.document && (
                <div className="border-t border-slate-200 pt-3">
                  {docState === 'idle' && (
                    <button
                      type="button"
                      onClick={() => setDocState('form')}
                      className="text-xs font-medium text-blue-600 underline decoration-dotted underline-offset-2 transition hover:text-blue-700"
                    >
                      {dict.status.attachDocumentButton}
                    </button>
                  )}

                  {docState === 'form' && (
                    <form
                      onSubmit={(event) => void handleDocSubmit(event)}
                      className="flex flex-col gap-1.5"
                    >
                      <label
                        htmlFor={`docType-${details.id}`}
                        className="text-xs font-medium text-slate-600"
                      >
                        {dict.status.attachDocumentTypeLabel}
                      </label>
                      <input
                        id={`docType-${details.id}`}
                        type="text"
                        value={docType}
                        onChange={(event) => setDocType(event.target.value)}
                        placeholder={dict.form.documentTypePlaceholder}
                        maxLength={MAX_DOCUMENT_TYPE_LENGTH}
                        autoFocus
                        className="rounded-lg border border-slate-300 px-3 py-1.5 text-sm text-slate-900 focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
                      />
                      <div className="flex gap-3">
                        <button
                          type="submit"
                          disabled={!docType.trim()}
                          className="rounded-lg bg-blue-600 px-3 py-1.5 text-xs font-medium text-white transition hover:bg-blue-700 disabled:cursor-not-allowed disabled:bg-slate-300"
                        >
                          {dict.status.attachDocumentSubmit}
                        </button>
                        <button
                          type="button"
                          onClick={() => {
                            setDocState('idle');
                            setDocType('');
                            setDocError(null);
                          }}
                          className="text-xs font-medium text-slate-500 underline decoration-dotted underline-offset-2 transition hover:text-slate-700"
                        >
                          {dict.status.attachDocumentCancel}
                        </button>
                      </div>
                      {docError && <p className="text-xs text-red-600">{docError}</p>}
                    </form>
                  )}

                  {(docState === 'submitting' || docState === 'generating') && (
                    <div className="flex items-center gap-2 text-xs text-slate-600">
                      <TypingDots />
                      <span>{dict.status.attachDocumentGenerating}</span>
                    </div>
                  )}
                </div>
              )}
            </div>
          )}

          {isCancelable(details.status) && (
            <div className="mt-2 border-t border-slate-200/70 pt-2">
              <button
                type="button"
                onClick={() => void handleCancel()}
                disabled={isCancelling}
                className="text-xs font-medium text-red-600 underline decoration-dotted underline-offset-2 transition hover:text-red-700 disabled:cursor-not-allowed disabled:text-slate-400"
              >
                {isCancelling ? dict.status.cancelling : dict.status.cancelButton}
              </button>
              {cancelError && <p className="mt-1 text-xs text-red-600">{cancelError}</p>}
            </div>
          )}
        </div>
      </div>
    </div>
  );
}

/**
 * Страница диалога: список обменов (см. ChatExchange) + композер внизу для следующего сообщения.
 * Важно: это НЕ многоходовой диалог с общим контекстом на бэкенде — каждое сообщение уходит как
 * независимый /requests (см. backend/src/orchestrator/orchestrator.service.ts, у него нет памяти
 * о предыдущих запросах в этом же "чате"). Композер просто даёт отправить следующий вопрос, не
 * возвращаясь на главную форму — визуально это диалог, но агенты каждый раз начинают с нуля.
 */
export function RequestStatusView({ requestId }: { requestId: string }) {
  const dict = useDictionary();
  const [requestIds, setRequestIds] = useState<string[]>([requestId]);
  const [latestStatus, setLatestStatus] = useState<RequestStatus | null>(null);
  const [composerText, setComposerText] = useState('');
  // Чекбокс "приложить документ" живёт и здесь (Этап 17, §9.13) — не только на первом экране
  // (RequestForm.tsx): любое сообщение в чате, не только первое, может запросить документ.
  const [includeDocument, setIncludeDocument] = useState(false);
  const [documentType, setDocumentType] = useState('');
  const [isSending, setIsSending] = useState(false);
  const [sendError, setSendError] = useState<string | null>(null);

  const trimmedLength = composerText.trim().length;
  const isComposerTextValid =
    trimmedLength >= MIN_QUERY_LENGTH && composerText.length <= MAX_QUERY_LENGTH;
  const isDocumentTypeValid = !includeDocument || documentType.trim().length > 0;
  // Пока последний обмен не дошёл до paused-статуса (или ждёт уточнения — это отдельная форма,
  // не новый вопрос), следующее сообщение отправлять рано: бэкенд не хранит контекст между
  // сообщениями, так что параллельная отправка просто запутает пользователя, какой ответ к
  // какому вопросу относится.
  const isWaitingForReply =
    latestStatus === null ||
    isActivelyProcessing(latestStatus) ||
    latestStatus === 'needs_clarification';
  const canSend = isComposerTextValid && isDocumentTypeValid && !isSending && !isWaitingForReply;

  async function handleSend(event: FormEvent<HTMLFormElement>): Promise<void> {
    event.preventDefault();
    if (!canSend) return;

    setIsSending(true);
    setSendError(null);
    try {
      const response = await createRequest({
        queryText: composerText,
        includeDocument,
        documentType: includeDocument ? documentType.trim() : null,
      });
      setRequestIds((prev) => [...prev, response.id]);
      setLatestStatus(response.status);
      setComposerText('');
      setIncludeDocument(false);
      setDocumentType('');
    } catch (err) {
      setSendError(err instanceof ApiError ? err.message : dict.form.genericError);
    } finally {
      setIsSending(false);
    }
  }

  return (
    <div className="flex flex-col gap-6">
      {requestIds.map((id, index) => (
        <ChatExchange
          key={id}
          requestId={id}
          {...(index === requestIds.length - 1 ? { onStatusChange: setLatestStatus } : {})}
        />
      ))}

      <form
        onSubmit={(event) => void handleSend(event)}
        className="sticky bottom-4 z-10 flex flex-col gap-1.5 rounded-2xl border border-slate-200 bg-white/95 p-2 shadow-lg backdrop-blur"
      >
        <div className="flex items-end gap-2">
          <textarea
            value={composerText}
            onChange={(event) => setComposerText(event.target.value)}
            rows={2}
            maxLength={MAX_QUERY_LENGTH}
            placeholder={dict.form.queryPlaceholder}
            disabled={isWaitingForReply || isSending}
            className="max-h-40 flex-1 resize-none rounded-xl border-0 bg-transparent px-3 py-2 text-sm text-slate-900 focus:ring-0 focus:outline-none disabled:text-slate-400"
          />
          <button
            type="submit"
            disabled={!canSend}
            className="shrink-0 rounded-xl bg-blue-600 px-4 py-2 text-sm font-medium text-white transition hover:bg-blue-700 disabled:cursor-not-allowed disabled:bg-slate-300"
          >
            {isSending ? dict.form.submitting : dict.form.submit}
          </button>
        </div>

        <div className="flex items-center gap-2 px-1">
          <input
            id="composerIncludeDocument"
            type="checkbox"
            checked={includeDocument}
            onChange={(event) => setIncludeDocument(event.target.checked)}
            disabled={isWaitingForReply || isSending}
            className="h-4 w-4 rounded border-slate-300 text-blue-600 focus:ring-blue-500 disabled:cursor-not-allowed"
          />
          <label htmlFor="composerIncludeDocument" className="text-xs text-slate-600">
            {dict.form.includeDocumentLabel}
          </label>
        </div>
        {includeDocument && (
          <input
            type="text"
            value={documentType}
            onChange={(event) => setDocumentType(event.target.value)}
            placeholder={dict.form.documentTypePlaceholder}
            maxLength={MAX_DOCUMENT_TYPE_LENGTH}
            disabled={isWaitingForReply || isSending}
            className="mx-1 rounded-lg border border-slate-300 px-3 py-1.5 text-sm text-slate-900 focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 disabled:cursor-not-allowed disabled:bg-slate-100"
          />
        )}

        {isWaitingForReply && !isSending && (
          <p className="px-1 text-xs text-slate-400">{dict.status.waitingForReplyHint}</p>
        )}
        {sendError && <p className="px-1 text-xs text-red-600">{sendError}</p>}
      </form>
    </div>
  );
}
