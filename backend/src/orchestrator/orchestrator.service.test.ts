import { describe, expect, it, vi } from 'vitest';
import type { ChatClient } from '@zan/shared';
import { OrchestratorService } from './orchestrator.service.js';
import type { RequestsRepository } from '../requests/requests.repository.js';
import type { RequestStepRecord, RequestWithDetails } from '../requests/request.types.js';
import type { AnswerAgent, DocumentAgent } from '../agents/agent.types.js';
import type { MetricsService } from '../metrics/metrics.service.js';
import type { RequestEventsPublisher } from '../realtime/request-events.types.js';

function fakeEvents(): RequestEventsPublisher {
  return { publishSnapshot: vi.fn(async () => undefined), publishToken: vi.fn() };
}

function fakeMetrics(): MetricsService {
  return {
    observeHttpRequest: vi.fn(),
    observePipelineStep: vi.fn(),
    observeRequestCompleted: vi.fn(),
  } as unknown as MetricsService;
}

/** По умолчанию — "уточнение не требуется", чтобы не ломать сценарии без Этапа 13 в фокусе. */
function fakeChat(response = '{"status":"ready","details":null}'): ChatClient {
  return {
    complete: vi.fn(async () => response),
    completeWithWebSearchMeta: vi.fn(), // clarification-check не использует web_search
    completeWithWebSearchMetaStream: vi.fn(),
  };
}

function baseRequest(overrides: Partial<RequestWithDetails> = {}): RequestWithDetails {
  return {
    id: 'req-1',
    status: 'pending',
    queryText: 'вопрос',
    includeDocument: false,
    documentType: null,
    resultSummary: null,
    errorMessage: null,
    clarificationQuestion: null,
    clarificationAnswer: null,
    createdAt: new Date(),
    updatedAt: new Date(),
    steps: [],
    document: null,
    ...overrides,
  };
}

function fakeRepo(request: RequestWithDetails | null): RequestsRepository {
  let ordinalCounter = 0;
  return {
    createRequest: vi.fn(),
    findRequestWithDetails: vi.fn(async () => request),
    isOwnedBy: vi.fn(async () => true),
    listByOwner: vi.fn(async () => []),
    updateRequestStatus: vi.fn(async () => undefined),
    getStatus: vi.fn(async () => request?.status ?? null),
    cancelIfActive: vi.fn(async () => true),
    setClarificationQuestion: vi.fn(async () => undefined),
    setClarificationAnswer: vi.fn(async () => undefined),
    setDocumentRequested: vi.fn(async () => undefined),
    createStep: vi.fn(async (requestId, agentName, ordinal) => {
      ordinalCounter += 1;
      const step: RequestStepRecord = {
        id: `step-${ordinalCounter}`,
        requestId,
        agentName,
        ordinal,
        status: 'pending',
        input: null,
        output: null,
        errorMessage: null,
        startedAt: null,
        finishedAt: null,
      };
      return step;
    }),
    startStep: vi.fn(async () => undefined),
    finishStep: vi.fn(async () => undefined),
    createDocument: vi.fn(async (requestId, documentType, title, fileFormat, content) => ({
      id: 'doc-1',
      requestId,
      documentType,
      title,
      fileFormat,
      content,
      createdAt: new Date(),
    })),
  };
}

function agents(overrides: { answer?: AnswerAgent; document?: DocumentAgent } = {}) {
  const answer: AnswerAgent = overrides.answer ?? {
    run: vi.fn(async () => ({
      answer: 'итоговый ответ',
      responseId: 'resp_answer_1',
      citedUrls: [],
    })),
  };
  const document: DocumentAgent = overrides.document ?? {
    run: vi.fn(async () => ({ title: 'т', fileFormat: 'text/plain', content: 'содержимое' })),
  };
  return { answer, document };
}

describe('OrchestratorService', () => {
  it('доводит успешный пайплайн до completed и сохраняет итоговый ответ', async () => {
    const request = baseRequest();
    const repo = fakeRepo(request);
    const { answer, document } = agents();
    const orchestrator = new OrchestratorService(
      repo,
      answer,
      document,
      fakeMetrics(),
      fakeChat(),
      fakeEvents(),
    );

    await orchestrator.processRequest('req-1');

    expect(repo.updateRequestStatus).toHaveBeenCalledWith('req-1', 'processing');
    expect(repo.updateRequestStatus).toHaveBeenCalledWith('req-1', 'completed', {
      resultSummary: 'итоговый ответ',
    });
    expect(repo.createStep).toHaveBeenCalledTimes(1);
    expect(document.run).not.toHaveBeenCalled();
  });

  it('запускает шаг document только когда includeDocument = true, и создаёт запись документа', async () => {
    const request = baseRequest({ includeDocument: true, documentType: 'заявление' });
    const repo = fakeRepo(request);
    const { answer, document } = agents();
    const orchestrator = new OrchestratorService(
      repo,
      answer,
      document,
      fakeMetrics(),
      fakeChat(),
      fakeEvents(),
    );

    await orchestrator.processRequest('req-1');

    expect(document.run).toHaveBeenCalledWith(
      expect.objectContaining({
        documentType: 'заявление',
        draftAnswer: 'итоговый ответ',
        previousResponseId: 'resp_answer_1',
      }),
    );
    expect(repo.createDocument).toHaveBeenCalledWith(
      'req-1',
      'заявление',
      'т',
      'text/plain',
      'содержимое',
    );
    expect(repo.updateRequestStatus).toHaveBeenCalledWith('req-1', 'completed', {
      resultSummary: 'итоговый ответ',
    });
  });

  it('останавливает пайплайн при ошибке шага answer', async () => {
    const request = baseRequest();
    const repo = fakeRepo(request);
    const failingAnswer: AnswerAgent = {
      run: vi.fn(async () => {
        throw new Error('answer упал');
      }),
    };
    const { document } = agents();
    const orchestrator = new OrchestratorService(
      repo,
      failingAnswer,
      document,
      fakeMetrics(),
      fakeChat(),
      fakeEvents(),
    );

    await orchestrator.processRequest('req-1');

    expect(repo.updateRequestStatus).toHaveBeenCalledWith('req-1', 'failed', {
      errorMessage: 'answer упал',
    });
    expect(document.run).not.toHaveBeenCalled();
  });

  it(
    'НЕ останавливает пайплайн при ошибке шага document (fail-open, см. runDocumentStep) — ' +
      'ответ пользователю всё равно доходит до completed',
    async () => {
      const request = baseRequest({ includeDocument: true, documentType: 'заявление' });
      const repo = fakeRepo(request);
      const { answer } = agents();
      const failingDocument: DocumentAgent = {
        run: vi.fn(async () => {
          throw new Error('Не хватает даты увольнения, чтобы составить документ по существу.');
        }),
      };
      const orchestrator = new OrchestratorService(
        repo,
        answer,
        failingDocument,
        fakeMetrics(),
        fakeChat(),
        fakeEvents(),
      );

      await orchestrator.processRequest('req-1');

      expect(repo.createDocument).not.toHaveBeenCalled();
      expect(repo.updateRequestStatus).toHaveBeenCalledWith('req-1', 'completed', {
        resultSummary: 'итоговый ответ',
      });
      expect(repo.updateRequestStatus).not.toHaveBeenCalledWith(
        'req-1',
        'failed',
        expect.anything(),
      );
    },
  );

  it('ничего не делает, если запрос не найден', async () => {
    const repo = fakeRepo(null);
    const { answer, document } = agents();
    const orchestrator = new OrchestratorService(
      repo,
      answer,
      document,
      fakeMetrics(),
      fakeChat(),
      fakeEvents(),
    );

    await orchestrator.processRequest('missing');

    expect(repo.updateRequestStatus).not.toHaveBeenCalled();
    expect(answer.run).not.toHaveBeenCalled();
  });

  it('останавливает пайплайн и просит уточнение, если LLM решает, что вопроса недостаточно', async () => {
    const request = baseRequest();
    const repo = fakeRepo(request);
    const { answer, document } = agents();
    const chat = fakeChat('{"status":"needs_clarification","details":"Какая у вас ситуация?"}');
    const orchestrator = new OrchestratorService(
      repo,
      answer,
      document,
      fakeMetrics(),
      chat,
      fakeEvents(),
    );

    await orchestrator.processRequest('req-1');

    expect(repo.setClarificationQuestion).toHaveBeenCalledWith('req-1', 'Какая у вас ситуация?');
    expect(repo.updateRequestStatus).toHaveBeenCalledWith('req-1', 'needs_clarification');
    expect(answer.run).not.toHaveBeenCalled();
    expect(repo.createStep).not.toHaveBeenCalled();
  });

  it('останавливает пайплайн и помечает запрос failed, если вопрос не по теме права РК', async () => {
    const request = baseRequest();
    const repo = fakeRepo(request);
    const { answer, document } = agents();
    const chat = fakeChat(
      '{"status":"out_of_topic","details":"Этот сервис отвечает только на юридические вопросы по законодательству РК."}',
    );
    const orchestrator = new OrchestratorService(
      repo,
      answer,
      document,
      fakeMetrics(),
      chat,
      fakeEvents(),
    );

    await orchestrator.processRequest('req-1');

    expect(repo.updateRequestStatus).toHaveBeenCalledWith('req-1', 'failed', {
      errorMessage: 'Этот сервис отвечает только на юридические вопросы по законодательству РК.',
    });
    expect(repo.setClarificationQuestion).not.toHaveBeenCalled();
    expect(answer.run).not.toHaveBeenCalled();
    expect(repo.createStep).not.toHaveBeenCalled();
  });

  it(
    'Этап 18: для уже завершённого запроса (resultSummary есть) запускает только document, ' +
      'не трогая answer/clarification и не меняя общий статус запроса',
    async () => {
      const request = baseRequest({
        status: 'completed',
        resultSummary: 'итоговый ответ',
        includeDocument: true,
        documentType: 'иск',
        steps: [
          {
            id: 'step-answer',
            requestId: 'req-1',
            agentName: 'answer',
            ordinal: 1,
            status: 'success',
            input: null,
            output: { answer: 'итоговый ответ', responseId: 'resp_answer_1', citedUrls: [] },
            errorMessage: null,
            startedAt: new Date(),
            finishedAt: new Date(),
          },
        ],
      });
      const repo = fakeRepo(request);
      const { answer, document } = agents();
      const orchestrator = new OrchestratorService(
        repo,
        answer,
        document,
        fakeMetrics(),
        fakeChat(),
        fakeEvents(),
      );

      await orchestrator.processRequest('req-1');

      expect(answer.run).not.toHaveBeenCalled();
      expect(document.run).toHaveBeenCalledWith(
        expect.objectContaining({
          queryText: 'вопрос',
          draftAnswer: 'итоговый ответ',
          documentType: 'иск',
          previousResponseId: 'resp_answer_1',
        }),
      );
      expect(repo.createDocument).toHaveBeenCalledWith(
        'req-1',
        'иск',
        'т',
        'text/plain',
        'содержимое',
      );
      expect(repo.updateRequestStatus).not.toHaveBeenCalledWith('req-1', 'processing');
      expect(repo.updateRequestStatus).not.toHaveBeenCalledWith(
        'req-1',
        'completed',
        expect.anything(),
      );
    },
  );

  it('Этап 18: не генерирует документ повторно, если он уже создан (идемпотентность)', async () => {
    const request = baseRequest({
      status: 'completed',
      resultSummary: 'итоговый ответ',
      document: {
        id: 'doc-1',
        requestId: 'req-1',
        documentType: 'иск',
        title: 'т',
        fileFormat: 'text/plain',
        content: 'содержимое',
        createdAt: new Date(),
      },
    });
    const repo = fakeRepo(request);
    const { answer, document } = agents();
    const orchestrator = new OrchestratorService(
      repo,
      answer,
      document,
      fakeMetrics(),
      fakeChat(),
      fakeEvents(),
    );

    await orchestrator.processRequest('req-1');

    expect(document.run).not.toHaveBeenCalled();
    expect(repo.createDocument).not.toHaveBeenCalled();
  });

  it('не спрашивает уточнение повторно и учитывает ответ пользователя в запросах к агентам', async () => {
    const request = baseRequest({
      clarificationQuestion: 'Какая у вас ситуация?',
      clarificationAnswer: 'Меня уволили без предупреждения',
    });
    const repo = fakeRepo(request);
    const { answer, document } = agents();
    // Если бы оркестратор спросил повторно, тест бы упал на JSON.parse('') — уточнение не должно запрашиваться снова.
    const chat = fakeChat('');
    const orchestrator = new OrchestratorService(
      repo,
      answer,
      document,
      fakeMetrics(),
      chat,
      fakeEvents(),
    );

    await orchestrator.processRequest('req-1');

    expect(chat.complete).not.toHaveBeenCalled();
    expect(answer.run).toHaveBeenCalledWith(
      expect.objectContaining({
        queryText: 'вопрос\n\nУточнение пользователя: Меня уволили без предупреждения',
      }),
    );
    expect(repo.updateRequestStatus).toHaveBeenCalledWith('req-1', 'completed', {
      resultSummary: 'итоговый ответ',
    });
  });
});
