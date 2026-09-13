import { Inject, Injectable } from '@nestjs/common';
import { createLogger, withStepLogging, type ChatClient, type Logger } from '@zan/shared';
import {
  ANSWER_AGENT,
  DOCUMENT_AGENT,
  type AnswerAgent,
  type DocumentAgent,
  type DocumentAgentOutput,
} from '../agents/agent.types.js';
import { REQUESTS_REPOSITORY, type RequestsRepository } from '../requests/requests.repository.js';
import type { AgentName, RequestWithDetails } from '../requests/request.types.js';
import { MetricsService } from '../metrics/metrics.service.js';
import { CHAT_CLIENT } from '../llm/llm.constants.js';
import { REQUEST_EVENTS, type RequestEventsPublisher } from '../realtime/request-events.types.js';
import { checkNeedsClarification } from './clarification-check.js';

/**
 * Управляет пайплайном: (опциональный один раунд уточнения) → answer → (опционально) document.
 * Каждый шаг логируется и персистится в request_steps — статус виден в реальном времени через
 * GET /requests/:id и WS (см. realtime/).
 *
 * Этап 17 (2026-09-12, §9.13): чекбокс "приложить документ" вернулся — теперь доступен и на
 * первом экране, и в композере продолжения чата (см. RequestForm.tsx/RequestStatusView.tsx), а
 * не только на первом. Автоопределение по тексту сообщения (Этап 16, §9.12) убрано — отдельный
 * классификатор на каждое сообщение не оправдал себя (лишний вызов там, где явный чекбокс дешевле
 * и точнее). Шаг document теперь fail-open: если агент решает, что контекста недостаточно для
 * содержательного документа (см. law-document.agent.ts), это не роняет весь запрос — ответ
 * пользователю всё равно доходит до 'completed', а причина, по которой документ не готов,
 * остаётся в request_steps шага document (видна на фронте отдельным уведомлением).
 *
 * До Этапа 15 (2026-09-12) здесь было 3 последовательных LLM-вызова (search → verification →
 * editor) — см. историю в INSTRUCTIONS.md §9.0-9.9. Слиты в один (Агент "answer",
 * law-answer.agent.ts, см. agent.types.ts): поиск, самопроверка и финальное форматирование ответа
 * за один вызов с `web_search`. Причина слияния — не только латентность (один вызов быстрее трёх
 * последовательных), но и UX: пробовали стримить пользователю ПРОМЕЖУТОЧНЫЙ черновик (Агент 1) —
 * откатили в тот же день, потому что человек видел текст, который потом менялся Агентом 2 и
 * переписывался Агентом 3, и это сбивало с толку, а не создавало доверие. Одному агенту нечего
 * менять после себя — то, что он стримит, и есть финальный ответ (см. §9.10).
 *
 * answer ходит живьём через web_search на adilet.zan.kz/zan.gov.kz — локального индекса
 * законодательства нет и не будет (см. §9.6/9.8: доступа к adilet.zan.kz нет ни у разработчика,
 * ни у прода, единственный канал — сам агент через web_search OpenAI).
 *
 * Осознанный компромисс §9.10: независимая перепроверка вторым LLM-вызовом (Агент 2, бывший
 * "единственный content-quality gate пайплайна") заменена на самопроверку в рамках одного вызова
 * (см. промпт в law-answer.agent.ts) — модель, проверяющая сама себя, статистически слабее ловит
 * собственные ошибки, чем независимый второй вызов со скептической ролью. Выбрано сознательно в
 * пользу латентности/простоты, не потому что это архитектурно эквивалентно.
 */
class RequestCancelledError extends Error {}

/**
 * Достаёт responseId из сохранённого output шага 'answer' (см. AnswerAgentOutput, finishStep в
 * runStep) — request_steps.output типизирован как `unknown` в БД, поэтому парсим защищённо, а не
 * приводим типом вслепую.
 */
function extractAnswerResponseId(output: unknown): string | null {
  if (typeof output !== 'object' || output === null) return null;
  const responseId = (output as Record<string, unknown>).responseId;
  return typeof responseId === 'string' ? responseId : null;
}

@Injectable()
export class OrchestratorService {
  private readonly logger: Logger = createLogger('orchestrator');

  constructor(
    @Inject(REQUESTS_REPOSITORY) private readonly repo: RequestsRepository,
    @Inject(ANSWER_AGENT) private readonly answerAgent: AnswerAgent,
    @Inject(DOCUMENT_AGENT) private readonly documentAgent: DocumentAgent,
    @Inject(MetricsService) private readonly metrics: MetricsService,
    @Inject(CHAT_CLIENT) private readonly chat: ChatClient,
    @Inject(REQUEST_EVENTS) private readonly events: RequestEventsPublisher,
  ) {}

  async processRequest(requestId: string): Promise<void> {
    const log = this.logger.child({ requestId });
    const pipelineStartedAt = Date.now();

    // Всё тело функции — в одном try/catch, а не только шаги пайплайна: до этого коммита
    // findRequestWithDetails/updateRequestStatus/publishSnapshot ниже не были защищены вовсе —
    // сбой БД/WS здесь (напр. Redis/Postgres недоступны) улетал бы наверх необработанным прямо в
    // BullMQ WorkerHost, где ничего не логирует ошибку job'ы (см. @OnWorkerEvent('failed') в
    // request-processing.processor.ts — это лишь подстраховка, а не основной путь логирования).
    try {
      const request = await this.repo.findRequestWithDetails(requestId);
      if (!request) {
        log.error({}, 'Запрос не найден при обработке');
        return;
      }
      if (request.status === 'cancelled') {
        // Пользователь отменил запрос, пока джоба ждала своей очереди в BullMQ (см.
        // RequestsService.cancel) — не запускаем пайплайн вовсе.
        log.info({}, 'Запрос отменён до начала обработки — пропускаем');
        return;
      }

      if (request.resultSummary !== null) {
        // Этап 18: запрос уже когда-то дошёл до 'completed' (resultSummary — единственный
        // признак этого, см. mapRequest) — значит это не новый вопрос, а догенерация документа
        // к уже данному ответу (см. RequestsService.requestDocument). Общий статус запроса
        // намеренно не трогаем и не гоняем заново clarification/answer — у нас уже есть и вопрос,
        // и финальный ответ, нужен только шаг document.
        await this.processDocumentOnlyResume(requestId, request, log);
        return;
      }

      await this.repo.updateRequestStatus(requestId, 'processing');
      await this.events.publishSnapshot(requestId);

      const queryText = await this.resolveEffectiveQueryText(requestId, request, log);
      if (queryText === null) {
        // Пайплайн приостановлен — оркестратор уже выставил статус 'needs_clarification'
        // и сохранил вопрос внутри resolveEffectiveQueryText. Продолжится отдельным вызовом
        // processRequest, когда пользователь ответит (см. RequestsService.submitClarification).
        return;
      }

      const answerOutput = await this.runStep(
        requestId,
        'answer',
        1,
        {
          queryText,
          requestId,
          onToken: (delta: string) => this.events.publishToken(requestId, 'answer', delta),
        },
        (input) => this.answerAgent.run(input),
        (output) => ({ answerLength: output.answer.length }),
      );

      const documentType = request.documentType ?? 'документ';
      const documentOutput = request.includeDocument
        ? await this.runDocumentStep(
            requestId,
            queryText,
            answerOutput.answer,
            documentType,
            answerOutput.responseId,
            log,
          )
        : null;

      if (documentOutput) {
        await this.repo.createDocument(
          requestId,
          documentType,
          documentOutput.title,
          documentOutput.fileFormat,
          documentOutput.content,
        );
      }

      // Ещё одна проверка перед финальной записью: document — самый долгий из оставшихся шагов
      // (если включён), и отмена вполне могла прийти уже после того, как его собственный
      // runStep() прошёл свою проверку (см. runStep) — без этого completed тихо перезаписал бы
      // cancelled.
      if ((await this.repo.getStatus(requestId)) === 'cancelled') {
        log.info({}, 'Запрос отменён во время последнего шага — не помечаем завершённым');
        return;
      }

      await this.repo.updateRequestStatus(requestId, 'completed', {
        resultSummary: answerOutput.answer,
      });
      await this.events.publishSnapshot(requestId);
      this.metrics.observeRequestCompleted('completed');
      log.info({ duration_ms: Date.now() - pipelineStartedAt }, 'Запрос обработан успешно');
    } catch (error) {
      if (error instanceof RequestCancelledError) {
        this.metrics.observeRequestCompleted('cancelled');
        log.info(
          { duration_ms: Date.now() - pipelineStartedAt },
          'Запрос отменён пользователем — пайплайн остановлен между шагами',
        );
        return;
      }
      const message = error instanceof Error ? error.message : String(error);
      await this.repo.updateRequestStatus(requestId, 'failed', { errorMessage: message });
      await this.events.publishSnapshot(requestId);
      this.metrics.observeRequestCompleted('failed');
      log.error(
        {
          error: message,
          stack: error instanceof Error ? error.stack : undefined,
          duration_ms: Date.now() - pipelineStartedAt,
        },
        'Обработка запроса провалена',
      );
    }
  }

  /**
   * document — единственный опциональный шаг, который может законно отказаться сам по себе (не
   * инфраструктурный сбой, а осознанное решение модели, см. INSUFFICIENT_CONTEXT_TITLE в
   * law-document.agent.ts): если данных из ответа/вопроса не хватает на содержательный документ,
   * агент бросает исключение с понятной причиной вместо того, чтобы выдумывать факты. Fail-open
   * здесь — по дизайну, не заглушка сбоя: пользователь уже получил (или получит) полноценный
   * текстовый ответ, документ — дополнительная опция, её отказ не должен превращать успешный
   * ответ в 'failed'. Причина остаётся в request_steps шага document (errorMessage) — фронт
   * показывает её отдельным уведомлением вместо кнопки скачивания (см. RequestStatusView.tsx).
   */
  private async runDocumentStep(
    requestId: string,
    queryText: string,
    draftAnswer: string,
    documentType: string,
    previousResponseId: string,
    log: Logger,
  ): Promise<DocumentAgentOutput | null> {
    try {
      return await this.runStep(
        requestId,
        'document',
        2,
        { queryText, draftAnswer, documentType, previousResponseId, requestId },
        (input) => this.documentAgent.run(input),
        (output) => ({ contentSize: output.content.length }),
      );
    } catch (error) {
      if (error instanceof RequestCancelledError) throw error;
      const message = error instanceof Error ? error.message : String(error);
      log.warn({ error: message }, 'Document не удался — не блокирует основной ответ');
      return null;
    }
  }

  /**
   * Этап 18 (§9.14): догенерация документа к уже завершённому запросу — пользователь увидел
   * ответ и только тогда решил приложить документ (см. RequestsService.requestDocument), вместо
   * того чтобы пересказывать вопрос заново в композере продолжения чата (см. §9.13 — та же
   * галочка в композере по-прежнему работает для НОВОГО вопроса в паре с документом, здесь же
   * речь о документе к уже прозвучавшему ответу).
   *
   * Контекст берём из уже сохранённых queryText/resultSummary этого же запроса — заново
   * clarification/answer не гоняем, это и есть исправление рассинхрона (агенту document раньше
   * неоткуда было взять контекст обсуждения, если он приходил отдельным независимым запросом).
   * previousResponseId — из output шага 'answer' (см. extractAnswerResponseId): нужен document
   * агенту, только если он сам идёт в web_search (см. mayHaveOfficialForm в law-document.agent.ts).
   *
   * Идемпотентно: если документ уже есть (например, повторная доставка джобы BullMQ), не
   * генерируем второй — просто выходим.
   */
  private async processDocumentOnlyResume(
    requestId: string,
    request: RequestWithDetails,
    log: Logger,
  ): Promise<void> {
    if (request.document) {
      log.info({}, 'Документ для этого запроса уже создан — пропускаем повторную генерацию');
      return;
    }
    if (request.resultSummary === null) {
      // Не должно происходить — сюда попадают только через ветку `resultSummary !== null` в
      // processRequest, — но без этой проверки TS не сузит тип для runDocumentStep ниже.
      log.error({}, 'processDocumentOnlyResume вызван без resultSummary — пропускаем');
      return;
    }

    const answerStep = request.steps.find(
      (step) => step.agentName === 'answer' && step.status === 'success',
    );
    const previousResponseId = extractAnswerResponseId(answerStep?.output) ?? '';
    const documentType = request.documentType ?? 'документ';

    const documentOutput = await this.runDocumentStep(
      requestId,
      request.queryText,
      request.resultSummary,
      documentType,
      previousResponseId,
      log,
    );

    if (documentOutput) {
      await this.repo.createDocument(
        requestId,
        documentType,
        documentOutput.title,
        documentOutput.fileFormat,
        documentOutput.content,
      );
    }
  }

  /**
   * Этап 13 — один раунд уточнения перед Агентом "answer" (плюс отсев нерелевантных вопросов).
   * Тот же LLM-вызов теперь возвращает один из трёх статусов: 'ready' | 'needs_clarification' |
   * 'out_of_topic' (см. clarification-check.ts). `clarificationQuestion === null` значит, что
   * проверка ещё не запускалась для этого запроса (первый прогон) — запускаем её один раз; если
   * она уже когда-то отработала (вопрос уже сохранён, вне зависимости от того, спросили что-то
   * или нет), повторно не проверяем — это и есть гарантия "ровно один раунд".
   * Возвращает null, если пайплайн должен остановиться (ждать ответ пользователя, либо запрос
   * отклонён как нерелевантный).
   */
  private async resolveEffectiveQueryText(
    requestId: string,
    request: RequestWithDetails,
    log: Logger,
  ): Promise<string | null> {
    if (request.clarificationQuestion === null) {
      const result = await checkNeedsClarification(this.chat, request.queryText, requestId);

      if (result.status === 'needs_clarification' && result.details) {
        await this.repo.setClarificationQuestion(requestId, result.details);
        await this.repo.updateRequestStatus(requestId, 'needs_clarification');
        await this.events.publishSnapshot(requestId);
        log.info({ question: result.details }, 'Требуется уточнение у пользователя');
        return null;
      }

      if (result.status === 'out_of_topic' && result.details) {
        // Нет отдельного RequestStatus под "нерелевантный вопрос" — переиспользуем 'failed' с
        // errorMessage, у него уже есть готовое отображение в UI (RequestStatusView.tsx), а
        // заводить под это отдельный статус/миграцию БД ради одного нового случая избыточно.
        await this.repo.updateRequestStatus(requestId, 'failed', { errorMessage: result.details });
        await this.events.publishSnapshot(requestId);
        log.info({ reason: result.details }, 'Вопрос отклонён как не относящийся к праву РК');
        return null;
      }
    }

    return request.clarificationAnswer
      ? `${request.queryText}\n\nУточнение пользователя: ${request.clarificationAnswer}`
      : request.queryText;
  }

  /**
   * `summarize` формирует PII-свободную сводку по выходу шага (счётчики, длины — не текст
   * пользователя и не текст статей) для структурированного лога и метрик; полный `output` при
   * этом персистится только в request_steps (БД, доступ ограничен владельцем запроса — см.
   * requests.repository.ts isOwnedBy), не в логах.
   */
  private async runStep<TInput, TOutput>(
    requestId: string,
    agentName: AgentName,
    ordinal: number,
    input: TInput,
    run: (input: TInput) => Promise<TOutput>,
    summarize: (output: TOutput) => Record<string, unknown> = () => ({}),
  ): Promise<TOutput> {
    // Единственная точка, где пайплайн реально проверяет отмену (см. RequestCancelledError) —
    // между шагами, а не посреди самого LLM-вызова: обрывать сам fetch к OpenAI на полпути не
    // умеем (ChatClient не принимает AbortSignal), поэтому уже начатый шаг всегда доводится до
    // конца, а следующий просто не стартует.
    if ((await this.repo.getStatus(requestId)) === 'cancelled') {
      throw new RequestCancelledError();
    }

    const step = await this.repo.createStep(requestId, agentName, ordinal);
    const log = this.logger.child({ requestId, stepId: step.id, agentName });
    await this.repo.startStep(step.id, input);
    await this.events.publishSnapshot(requestId);

    const startedAt = Date.now();
    try {
      const output = await withStepLogging(log, `orchestrator.${agentName}`, {}, async () => {
        try {
          const result = await run(input);
          await this.repo.finishStep(step.id, 'success', { output: result });
          await this.events.publishSnapshot(requestId);
          return result;
        } catch (error) {
          const message = error instanceof Error ? error.message : String(error);
          await this.repo.finishStep(step.id, 'failed', { errorMessage: message });
          await this.events.publishSnapshot(requestId);
          throw error;
        }
      });
      const durationMs = Date.now() - startedAt;
      this.metrics.observePipelineStep(agentName, 'success', durationMs);
      log.info(
        { ...summarize(output), duration_ms: durationMs },
        `orchestrator.${agentName}.summary`,
      );
      return output;
    } catch (error) {
      this.metrics.observePipelineStep(agentName, 'failed', Date.now() - startedAt);
      throw error;
    }
  }
}
