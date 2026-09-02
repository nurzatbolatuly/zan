import { Inject, Injectable } from '@nestjs/common';
import { createLogger, withStepLogging, type ChatClient, type Logger } from '@zan/shared';
import {
  SEARCH_AGENT,
  VERIFICATION_AGENT,
  EDITOR_AGENT,
  DOCUMENT_AGENT,
  type SearchAgent,
  type VerificationAgent,
  type VerificationAgentOutput,
  type EditorAgent,
  type DocumentAgent,
} from '../agents/agent.types.js';
import { REQUESTS_REPOSITORY, type RequestsRepository } from '../requests/requests.repository.js';
import type { AgentName, RequestWithDetails } from '../requests/request.types.js';
import { MetricsService } from '../metrics/metrics.service.js';
import { CHAT_CLIENT } from '../llm/llm.constants.js';
import { checkNeedsClarification } from './clarification-check.js';
import { config } from '../config.js';

/**
 * Управляет пайплайном: (опциональный один раунд уточнения) → search → verification (ровно один
 * проход каждого, без переспроса) → (editor ∥ document). Каждый шаг логируется и персистится в
 * request_steps — статус и (для verification) список замечаний виден в реальном времени через
 * GET /requests/:id.
 *
 * search и verification оба ходят живьём через web_search на adilet.zan.kz/zan.gov.kz —
 * локального индекса законодательства нет (см. INSTRUCTIONS.md §9: пробовали офлайн-корпус,
 * вернулись к live-поиску, т.к. синк корпуса требовал сетевого доступа, которого не было в части
 * инфраструктуры).
 *
 * До 2026-08-28 при найденных verification замечаниях оркестратор переспрашивал Агента 1 с их
 * учётом, до `MAX_VERIFICATION_ROUNDS` раундов. Убрано: замер на реальном прод-запросе показал,
 * что повторный раунд не гарантирует устранения замечаний (4 замечания до и после повтора), но
 * всегда добавляет латентность — в live-режиме до ~85с за один лишний раунд (см. §9.5
 * INSTRUCTIONS.md). Теперь замечания verification НЕ блокируют пайплайн и не вызывают переспрос —
 * они идут дальше как есть, персистятся в request_steps шага verification и логируются отдельно
 * (см. runSearchAndVerify) как диагностическая информация для ручной проверки при доработке
 * промптов, а не как автоматический gate.
 */
/**
 * Не инфраструктурная ошибка — сигнал из runStep(), что запрос отменён пользователем (см.
 * RequestsService.cancel), пойманный в processRequest() отдельно от реальных сбоев, чтобы не
 * перезаписать уже выставленный статус 'cancelled' на 'failed'.
 */
class RequestCancelledError extends Error {}

@Injectable()
export class OrchestratorService {
  private readonly logger: Logger = createLogger('orchestrator');

  constructor(
    @Inject(REQUESTS_REPOSITORY) private readonly repo: RequestsRepository,
    @Inject(SEARCH_AGENT) private readonly searchAgent: SearchAgent,
    @Inject(VERIFICATION_AGENT) private readonly verificationAgent: VerificationAgent,
    @Inject(EDITOR_AGENT) private readonly editorAgent: EditorAgent,
    @Inject(DOCUMENT_AGENT) private readonly documentAgent: DocumentAgent,
    @Inject(MetricsService) private readonly metrics: MetricsService,
    @Inject(CHAT_CLIENT) private readonly chat: ChatClient,
  ) {}

  async processRequest(requestId: string): Promise<void> {
    const log = this.logger.child({ requestId });
    const pipelineStartedAt = Date.now();
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

    await this.repo.updateRequestStatus(requestId, 'processing');

    try {
      const queryText = await this.resolveEffectiveQueryText(requestId, request, log);
      if (queryText === null) {
        // Пайплайн приостановлен — оркестратор уже выставил статус 'needs_clarification'
        // и сохранил вопрос внутри resolveEffectiveQueryText. Продолжится отдельным вызовом
        // processRequest, когда пользователь ответит (см. RequestsService.submitClarification).
        return;
      }

      const { answer: verifiedAnswer, nextOrdinal } = await this.runSearchAndVerify(
        requestId,
        queryText,
        log,
      );

      const documentType = request.documentType ?? 'документ';
      const [editorOutput, documentOutput] = await Promise.all([
        this.runStep(
          requestId,
          'editor',
          nextOrdinal,
          { queryText, draftAnswer: verifiedAnswer },
          (input) => this.editorAgent.run(input),
          (output) => ({ summaryLength: output.summary.length }),
        ),
        request.includeDocument
          ? this.runStep(
              requestId,
              'document',
              nextOrdinal + 1,
              { queryText, draftAnswer: verifiedAnswer, documentType },
              (input) => this.documentAgent.run(input),
              (output) => ({ contentSize: output.content.length }),
            )
          : Promise.resolve(null),
      ]);

      if (documentOutput) {
        await this.repo.createDocument(
          requestId,
          documentType,
          documentOutput.title,
          documentOutput.fileFormat,
          documentOutput.content,
        );
      }

      // Ещё одна проверка перед финальной записью: editor/document — самые долгие шаги
      // пайплайна, и отмена вполне могла прийти уже после того, как их собственный runStep()
      // прошёл свою проверку (см. runStep) — без этого completed тихо перезаписал бы cancelled.
      if ((await this.repo.getStatus(requestId)) === 'cancelled') {
        log.info({}, 'Запрос отменён во время последнего шага — не помечаем завершённым');
        return;
      }

      await this.repo.updateRequestStatus(requestId, 'completed', {
        resultSummary: editorOutput.summary,
      });
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
      this.metrics.observeRequestCompleted('failed');
      log.error(
        { error: message, duration_ms: Date.now() - pipelineStartedAt },
        'Обработка запроса провалена',
      );
    }
  }

  /**
   * Этап 13 — один раунд уточнения перед Агентом 1 (плюс отсев нерелевантных вопросов). Тот же
   * LLM-вызов теперь возвращает один из трёх статусов: 'ready' | 'needs_clarification' |
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
      const result = await checkNeedsClarification(this.chat, request.queryText);

      if (result.status === 'needs_clarification' && result.details) {
        await this.repo.setClarificationQuestion(requestId, result.details);
        await this.repo.updateRequestStatus(requestId, 'needs_clarification');
        log.info({ question: result.details }, 'Требуется уточнение у пользователя');
        return null;
      }

      if (result.status === 'out_of_topic' && result.details) {
        // Нет отдельного RequestStatus под "нерелевантный вопрос" — переиспользуем 'failed' с
        // errorMessage, у него уже есть готовое отображение в UI (RequestStatusView.tsx), а
        // заводить под это отдельный статус/миграцию БД ради одного нового случая избыточно.
        await this.repo.updateRequestStatus(requestId, 'failed', { errorMessage: result.details });
        log.info({ reason: result.details }, 'Вопрос отклонён как не относящийся к праву РК');
        return null;
      }
    }

    return request.clarificationAnswer
      ? `${request.queryText}\n\nУточнение пользователя: ${request.clarificationAnswer}`
      : request.queryText;
  }

  /**
   * Ядро правки: search → verification, ровно один проход каждого (см. класс-комментарий выше —
   * переспрос убран 2026-08-28). Замечания verification НЕ блокируют и не запускают повтор —
   * они только логируются и остаются в request_steps шага verification (видны через
   * GET /requests/:id) как отдельная диагностическая информация, независимо от текста ответа.
   * `nextOrdinal` — первый свободный порядковый номер шага для editor/document, которые
   * идут следом (см. processRequest).
   */
  private async runSearchAndVerify(
    requestId: string,
    queryText: string,
    log: Logger,
  ): Promise<{ answer: string; nextOrdinal: number }> {
    const searchOutput = await this.runStep(
      requestId,
      'search',
      1,
      { queryText },
      (input) => this.searchAgent.run(input),
      (output) => ({ answerLength: output.draftAnswer.length }),
    );
    const draftAnswer = searchOutput.draftAnswer;

    if (config.skipVerification) {
      log.warn({}, 'SKIP_VERIFICATION=true — verification-агент пропущен, замер latency');
      return { answer: draftAnswer, nextOrdinal: 2 };
    }

    const verification = await this.runVerificationStep(requestId, 2, queryText, draftAnswer, log);
    this.metrics.observeVerificationReview(verification.concerns.length);

    if (verification.concerns.length > 0) {
      log.warn(
        { concerns: verification.concerns },
        'Verification нашла замечания — не блокируем и не переспрашиваем Агента 1, ' +
          'замечания сохранены отдельно в шаге verification для ручной проверки',
      );
    }

    return { answer: verification.revisedAnswer, nextOrdinal: 3 };
  }

  /**
   * Verification — настоящий LLM-вызов с web_search (см. law-verification.agent.ts), может упасть
   * инфраструктурно — fail-open: раунд засчитывается пройденным без расхождений, не блокирует
   * пайплайн.
   */
  private async runVerificationStep(
    requestId: string,
    ordinal: number,
    queryText: string,
    draftAnswer: string,
    log: Logger,
  ): Promise<VerificationAgentOutput> {
    try {
      return await this.runStep(
        requestId,
        'verification',
        ordinal,
        { queryText, draftAnswer },
        (input) => this.verificationAgent.run(input),
        (output) => ({ concernsCount: output.concerns.length }),
      );
    } catch (error) {
      if (error instanceof RequestCancelledError) throw error;
      const message = error instanceof Error ? error.message : String(error);
      log.warn({ error: message }, 'Verification упала — раунд fail-open, без расхождений');
      return { revisedAnswer: draftAnswer, concerns: [] };
    }
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

    const startedAt = Date.now();
    try {
      const output = await withStepLogging(log, `orchestrator.${agentName}`, {}, async () => {
        try {
          const result = await run(input);
          await this.repo.finishStep(step.id, 'success', { output: result });
          return result;
        } catch (error) {
          const message = error instanceof Error ? error.message : String(error);
          await this.repo.finishStep(step.id, 'failed', { errorMessage: message });
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
