import { describe, expect, it, vi } from 'vitest';
import type { ChatClient } from '@zan/shared';
import { OrchestratorService } from './orchestrator.service.js';
import type { RequestsRepository } from '../requests/requests.repository.js';
import type { RequestStepRecord, RequestWithDetails } from '../requests/request.types.js';
import type {
  SearchAgent,
  VerificationAgent,
  EditorAgent,
  DocumentAgent,
} from '../agents/agent.types.js';
import type { MetricsService } from '../metrics/metrics.service.js';

function fakeMetrics(): MetricsService {
  return {
    observeHttpRequest: vi.fn(),
    observePipelineStep: vi.fn(),
    observeRequestCompleted: vi.fn(),
    observeVerificationReview: vi.fn(),
  } as unknown as MetricsService;
}

/** По умолчанию — "уточнение не требуется", чтобы не ломать сценарии без Этапа 13 в фокусе. */
function fakeChat(response = '{"status":"ready","details":null}'): ChatClient {
  return {
    complete: vi.fn(async () => response),
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

function agents(
  overrides: {
    search?: SearchAgent;
    verification?: VerificationAgent;
    editor?: EditorAgent;
    document?: DocumentAgent;
  } = {},
) {
  const search: SearchAgent = overrides.search ?? {
    run: vi.fn(async () => ({ draftAnswer: 'черновой ответ' })),
  };
  const verification: VerificationAgent = overrides.verification ?? {
    run: vi.fn(async () => ({ revisedAnswer: 'проверенный ответ', concerns: [] })),
  };
  const editor: EditorAgent = overrides.editor ?? { run: vi.fn(async () => ({ summary: 'итог' })) };
  const document: DocumentAgent = overrides.document ?? {
    run: vi.fn(async () => ({ title: 'т', fileFormat: 'text/plain', content: 'содержимое' })),
  };
  return { search, verification, editor, document };
}

describe('OrchestratorService', () => {
  it('доводит успешный пайплайн до completed и сохраняет итоговый summary', async () => {
    const request = baseRequest();
    const repo = fakeRepo(request);
    const { search, verification, editor, document } = agents();
    const orchestrator = new OrchestratorService(
      repo,
      search,
      verification,
      editor,
      document,
      fakeMetrics(),
      fakeChat(),
    );

    await orchestrator.processRequest('req-1');

    expect(repo.updateRequestStatus).toHaveBeenCalledWith('req-1', 'processing');
    expect(repo.updateRequestStatus).toHaveBeenCalledWith('req-1', 'completed', {
      resultSummary: 'итог',
    });
    expect(repo.createStep).toHaveBeenCalledTimes(3);
    expect(document.run).not.toHaveBeenCalled();
  });

  it('запускает шаг document только когда includeDocument = true, и создаёт запись документа', async () => {
    const request = baseRequest({ includeDocument: true, documentType: 'заявление' });
    const repo = fakeRepo(request);
    const { search, verification, editor, document } = agents();
    const orchestrator = new OrchestratorService(
      repo,
      search,
      verification,
      editor,
      document,
      fakeMetrics(),
      fakeChat(),
    );

    await orchestrator.processRequest('req-1');

    // draftAnswer, переданный дальше editor/document, — это revisedAnswer verification, а не
    // сырой draftAnswer от search напрямую.
    expect(document.run).toHaveBeenCalledWith(
      expect.objectContaining({ documentType: 'заявление', draftAnswer: 'проверенный ответ' }),
    );
    expect(repo.createDocument).toHaveBeenCalledWith(
      'req-1',
      'заявление',
      'т',
      'text/plain',
      'содержимое',
    );
    expect(repo.updateRequestStatus).toHaveBeenCalledWith('req-1', 'completed', {
      resultSummary: 'итог',
    });
  });

  it('останавливает пайплайн при ошибке блокирующего шага (editor)', async () => {
    const request = baseRequest();
    const repo = fakeRepo(request);
    const failingEditor: EditorAgent = {
      run: vi.fn(async () => {
        throw new Error('editor упал');
      }),
    };
    const { search, verification, document } = agents({ editor: failingEditor });
    const orchestrator = new OrchestratorService(
      repo,
      search,
      verification,
      failingEditor,
      document,
      fakeMetrics(),
      fakeChat(),
    );

    await orchestrator.processRequest('req-1');

    expect(repo.updateRequestStatus).toHaveBeenCalledWith('req-1', 'failed', {
      errorMessage: 'editor упал',
    });
  });

  it('не останавливает пайплайн при инфраструктурном сбое verification — раунд fail-open', async () => {
    const request = baseRequest();
    const repo = fakeRepo(request);
    const failingVerification: VerificationAgent = {
      run: vi.fn(async () => {
        throw new Error('верификация упала');
      }),
    };
    const { search, editor, document } = agents({ verification: failingVerification });
    const orchestrator = new OrchestratorService(
      repo,
      search,
      failingVerification,
      editor,
      document,
      fakeMetrics(),
      fakeChat(),
    );

    await orchestrator.processRequest('req-1');

    // Сбой (исключение) — не то же самое, что замечания: раунд засчитывается пройденным без
    // замечаний, search не переспрашивается повторно.
    expect(search.run).toHaveBeenCalledTimes(1);
    expect(editor.run).toHaveBeenCalled();
    expect(repo.updateRequestStatus).toHaveBeenCalledWith('req-1', 'completed', {
      resultSummary: 'итог',
    });
    expect(repo.updateRequestStatus).not.toHaveBeenCalledWith('req-1', 'failed', expect.anything());
  });

  it(
    'при замечаниях verification НЕ переспрашивает Агента 1 (search вызывается ровно один раз) ' +
      'и передаёт editor/document revisedAnswer от verification как есть',
    async () => {
      const request = baseRequest();
      const repo = fakeRepo(request);
      const { search } = agents();
      const verification: VerificationAgent = {
        run: vi.fn(async () => ({
          revisedAnswer: 'смягчённый ответ после правок',
          concerns: ['номер статьи не подтверждён'],
        })),
      };
      const { editor, document } = agents();
      const orchestrator = new OrchestratorService(
        repo,
        search,
        verification,
        editor,
        document,
        fakeMetrics(),
        fakeChat(),
      );

      await orchestrator.processRequest('req-1');

      // Переспрос убран 2026-08-28 (см. класс-комментарий OrchestratorService) — search и
      // verification вызываются ровно по разу, независимо от того, нашла ли verification
      // замечания; замечания идут дальше только как revisedAnswer + запись в request_steps.
      expect(search.run).toHaveBeenCalledTimes(1);
      expect(verification.run).toHaveBeenCalledTimes(1);
      expect(editor.run).toHaveBeenCalledWith(
        expect.objectContaining({ draftAnswer: 'смягчённый ответ после правок' }),
      );
      expect(repo.updateRequestStatus).toHaveBeenCalledWith('req-1', 'completed', {
        resultSummary: 'итог',
      });
    },
  );

  it('ничего не делает, если запрос не найден', async () => {
    const repo = fakeRepo(null);
    const { search, verification, editor, document } = agents();
    const orchestrator = new OrchestratorService(
      repo,
      search,
      verification,
      editor,
      document,
      fakeMetrics(),
      fakeChat(),
    );

    await orchestrator.processRequest('missing');

    expect(repo.updateRequestStatus).not.toHaveBeenCalled();
    expect(search.run).not.toHaveBeenCalled();
  });

  it('останавливает пайплайн и просит уточнение, если LLM решает, что вопроса недостаточно', async () => {
    const request = baseRequest();
    const repo = fakeRepo(request);
    const { search, verification, editor, document } = agents();
    const chat = fakeChat('{"status":"needs_clarification","details":"Какая у вас ситуация?"}');
    const orchestrator = new OrchestratorService(
      repo,
      search,
      verification,
      editor,
      document,
      fakeMetrics(),
      chat,
    );

    await orchestrator.processRequest('req-1');

    expect(repo.setClarificationQuestion).toHaveBeenCalledWith('req-1', 'Какая у вас ситуация?');
    expect(repo.updateRequestStatus).toHaveBeenCalledWith('req-1', 'needs_clarification');
    expect(search.run).not.toHaveBeenCalled();
    expect(repo.createStep).not.toHaveBeenCalled();
  });

  it('останавливает пайплайн и помечает запрос failed, если LLM решает, что вопрос не по теме права РК', async () => {
    const request = baseRequest();
    const repo = fakeRepo(request);
    const { search, verification, editor, document } = agents();
    const chat = fakeChat(
      '{"status":"out_of_topic","details":"Этот сервис отвечает только на юридические вопросы по законодательству РК."}',
    );
    const orchestrator = new OrchestratorService(
      repo,
      search,
      verification,
      editor,
      document,
      fakeMetrics(),
      chat,
    );

    await orchestrator.processRequest('req-1');

    expect(repo.updateRequestStatus).toHaveBeenCalledWith('req-1', 'failed', {
      errorMessage: 'Этот сервис отвечает только на юридические вопросы по законодательству РК.',
    });
    expect(repo.setClarificationQuestion).not.toHaveBeenCalled();
    expect(search.run).not.toHaveBeenCalled();
    expect(repo.createStep).not.toHaveBeenCalled();
  });

  it('не спрашивает уточнение повторно и учитывает ответ пользователя в запросах к агентам', async () => {
    const request = baseRequest({
      clarificationQuestion: 'Какая у вас ситуация?',
      clarificationAnswer: 'Меня уволили без предупреждения',
    });
    const repo = fakeRepo(request);
    const { search, verification, editor, document } = agents();
    // Если бы оркестратор спросил повторно, тест бы упал на JSON.parse('') — уточнение не должно запрашиваться снова.
    const chat = fakeChat('');
    const orchestrator = new OrchestratorService(
      repo,
      search,
      verification,
      editor,
      document,
      fakeMetrics(),
      chat,
    );

    await orchestrator.processRequest('req-1');

    expect(chat.complete).not.toHaveBeenCalled();
    expect(search.run).toHaveBeenCalledWith({
      queryText: 'вопрос\n\nУточнение пользователя: Меня уволили без предупреждения',
    });
    expect(repo.updateRequestStatus).toHaveBeenCalledWith('req-1', 'completed', {
      resultSummary: 'итог',
    });
  });
});
