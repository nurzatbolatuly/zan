'use client';

import { useCallback, useEffect, useRef, useState, type FormEvent } from 'react';
import { ApiError, cancelRequest, createRequest, fetchRequestDetails } from '../lib/api';
import type { RequestDetails, RequestStatus } from '../lib/types';
import { MarkdownAnswer } from './MarkdownAnswer';
import { DocumentDownloadButton } from './DocumentDownloadButton';
import { ClarificationForm } from './ClarificationForm';
import { MIN_QUERY_LENGTH, MAX_QUERY_LENGTH } from './RequestForm';
import { useDictionary } from '../i18n/DictionaryProvider';
import type { Dictionary } from '../i18n/dictionary';

const POLL_INTERVAL_MS = 2000;
const TICK_INTERVAL_MS = 1000;

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
 * пользователя (см. backend/src/orchestrator/orchestrator.service.ts), поэтому поллинг
 * останавливается и там же, а не только на completed/failed; возобновляется вручную через
 * resumePolling() после отправки уточнения (ClarificationForm.onSubmitted).
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
  const timerRef = useRef<ReturnType<typeof setInterval> | null>(null);
  // Счётчик поколений запроса: обычный 2с-интервал и poll() из handleCancel могут пересечься —
  // если ответ интервала прилетит позже ответа на отмену, он не должен затереть более свежее
  // состояние. Применяем только результат последнего запущенного poll().
  const pollSeqRef = useRef(0);
  // Поправка на рассинхрон часов клиента с сервером (мс, serverTime - clientTime) — без неё
  // "прошло N сек" и порог "зависания" считались бы от локальных часов клиента напрямую и могли
  // сработать раньше/позже реального времени обработки при неточных часах устройства.
  const clockSkewRef = useRef(0);

  const poll = useCallback(async () => {
    const seq = ++pollSeqRef.current;
    try {
      const clientTimeBeforeRequest = Date.now();
      const { data, serverTimeMs } = await fetchRequestDetails(requestId);
      if (seq !== pollSeqRef.current) return;
      if (serverTimeMs !== null) {
        clockSkewRef.current = serverTimeMs - clientTimeBeforeRequest;
      }
      setDetails(data);
      setLoadError(null);
      onStatusChange?.(data.status);
      if (isPaused(data.status) && timerRef.current) {
        clearInterval(timerRef.current);
        timerRef.current = null;
      }
    } catch (err) {
      if (seq !== pollSeqRef.current) return;
      setLoadError(err instanceof ApiError ? err.message : dict.status.loadError);
      if (timerRef.current) {
        clearInterval(timerRef.current);
        timerRef.current = null;
      }
    }
  }, [requestId, dict, onStatusChange]);

  useEffect(() => {
    void poll();
    timerRef.current = setInterval(() => void poll(), POLL_INTERVAL_MS);

    return () => {
      if (timerRef.current) clearInterval(timerRef.current);
    };
  }, [poll]);

  // Отдельный, более частый тик (1с) только для счётчика "прошло N сек" в баннере — независимо
  // от 2-секундного поллинга статуса выше, чтобы счётчик шёл плавно, а не скачками по 2с.
  const status = details?.status;
  useEffect(() => {
    if (!status || !isActivelyProcessing(status)) return;
    const tick = setInterval(() => setNow(Date.now()), TICK_INTERVAL_MS);
    return () => clearInterval(tick);
  }, [status]);

  const resumePolling = useCallback(() => {
    if (timerRef.current) clearInterval(timerRef.current);
    void poll();
    timerRef.current = setInterval(() => void poll(), POLL_INTERVAL_MS);
  }, [poll]);

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
              <div className="flex items-center gap-2 text-slate-600">
                <TypingDots />
                <span>{currentStageLabel(details.steps, dict)}</span>
              </div>
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
                onSubmitted={resumePolling}
              />
            </div>
          )}

          {details.status === 'failed' && (
            <p>
              {dict.status.failedPrefix}
              {details.errorMessage ? `: ${details.errorMessage}` : '.'}
            </p>
          )}

          {details.status === 'cancelled' && <p>{dict.status.statusLabel.cancelled}</p>}

          {details.status === 'completed' && details.resultSummary && (
            <div className="flex flex-col gap-3">
              <MarkdownAnswer markdown={details.resultSummary} />
              {details.document && (
                <div className="border-t border-slate-200 pt-3">
                  <DocumentDownloadButton document={details.document} />
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
  const [isSending, setIsSending] = useState(false);
  const [sendError, setSendError] = useState<string | null>(null);

  const trimmedLength = composerText.trim().length;
  const isComposerTextValid =
    trimmedLength >= MIN_QUERY_LENGTH && composerText.length <= MAX_QUERY_LENGTH;
  // Пока последний обмен не дошёл до paused-статуса (или ждёт уточнения — это отдельная форма,
  // не новый вопрос), следующее сообщение отправлять рано: бэкенд не хранит контекст между
  // сообщениями, так что параллельная отправка просто запутает пользователя, какой ответ к
  // какому вопросу относится.
  const isWaitingForReply =
    latestStatus === null ||
    isActivelyProcessing(latestStatus) ||
    latestStatus === 'needs_clarification';
  const canSend = isComposerTextValid && !isSending && !isWaitingForReply;

  async function handleSend(event: FormEvent<HTMLFormElement>): Promise<void> {
    event.preventDefault();
    if (!canSend) return;

    setIsSending(true);
    setSendError(null);
    try {
      const response = await createRequest({
        queryText: composerText,
        includeDocument: false,
        documentType: null,
      });
      setRequestIds((prev) => [...prev, response.id]);
      setLatestStatus(response.status);
      setComposerText('');
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
        {isWaitingForReply && !isSending && (
          <p className="px-1 text-xs text-slate-400">{dict.status.waitingForReplyHint}</p>
        )}
        {sendError && <p className="px-1 text-xs text-red-600">{sendError}</p>}
      </form>
    </div>
  );
}
