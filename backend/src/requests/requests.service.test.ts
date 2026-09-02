import { describe, expect, it, vi } from 'vitest';
import { BadRequestException, NotFoundException } from '@nestjs/common';
import type { Queue } from 'bullmq';
import { RequestsService } from './requests.service.js';
import type { RequestsRepository } from './requests.repository.js';
import type { RequestRecord, RequestWithDetails } from './request.types.js';
import type { RequestProcessingJobData } from '../queue/queue.constants.js';

function fakeRepo(overrides: Partial<RequestsRepository> = {}): RequestsRepository {
  return {
    createRequest: vi.fn(),
    findRequestWithDetails: vi.fn(),
    isOwnedBy: vi.fn(async () => true),
    listByOwner: vi.fn(async () => []),
    updateRequestStatus: vi.fn(),
    setClarificationQuestion: vi.fn(),
    setClarificationAnswer: vi.fn(),
    createStep: vi.fn(),
    startStep: vi.fn(),
    finishStep: vi.fn(),
    createDocument: vi.fn(),
    ...overrides,
  } as unknown as RequestsRepository;
}

function fakeQueue(): Queue<RequestProcessingJobData> {
  return { add: vi.fn(async () => undefined) } as unknown as Queue<RequestProcessingJobData>;
}

describe('RequestsService', () => {
  it('создаёт запрос и ставит задачу в очередь', async () => {
    const created: RequestRecord = {
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
    };
    const repo = fakeRepo({ createRequest: vi.fn(async () => created) });
    const queue = fakeQueue();
    const service = new RequestsService(repo, queue);

    const result = await service.create({ queryText: 'вопрос' }, 'hash-1');

    expect(result).toBe(created);
    expect(repo.createRequest).toHaveBeenCalledWith(
      expect.objectContaining({ ownerTokenHash: 'hash-1' }),
    );
    expect(queue.add).toHaveBeenCalledWith('process', { requestId: 'req-1' });
  });

  it('отклоняет includeDocument без documentType и не создаёт запрос', async () => {
    const repo = fakeRepo();
    const queue = fakeQueue();
    const service = new RequestsService(repo, queue);

    await expect(
      service.create({ queryText: 'вопрос', includeDocument: true }, 'hash-1'),
    ).rejects.toBeInstanceOf(BadRequestException);
    expect(repo.createRequest).not.toHaveBeenCalled();
    expect(queue.add).not.toHaveBeenCalled();
  });

  it('выбрасывает NotFoundException, если запрос не найден', async () => {
    const repo = fakeRepo({ findRequestWithDetails: vi.fn(async () => null) });
    const service = new RequestsService(repo, fakeQueue());

    await expect(service.getById('missing', 'hash-1')).rejects.toBeInstanceOf(NotFoundException);
  });

  it('выбрасывает NotFoundException, если запрос найден, но принадлежит другой сессии', async () => {
    const details = { id: 'req-1', steps: [], document: null } as unknown as RequestWithDetails;
    const repo = fakeRepo({
      findRequestWithDetails: vi.fn(async () => details),
      isOwnedBy: vi.fn(async () => false),
    });
    const service = new RequestsService(repo, fakeQueue());

    await expect(service.getById('req-1', 'чужой-hash')).rejects.toBeInstanceOf(NotFoundException);
  });

  it('возвращает запрос с деталями, если найден и принадлежит сессии', async () => {
    const details = { id: 'req-1', steps: [], document: null } as unknown as RequestWithDetails;
    const repo = fakeRepo({ findRequestWithDetails: vi.fn(async () => details) });
    const service = new RequestsService(repo, fakeQueue());

    await expect(service.getById('req-1', 'hash-1')).resolves.toBe(details);
  });

  it('listMine возвращает пустой список без похода в БД, если нет cookie сессии', async () => {
    const repo = fakeRepo();
    const service = new RequestsService(repo, fakeQueue());

    await expect(service.listMine(null)).resolves.toEqual([]);
    expect(repo.listByOwner).not.toHaveBeenCalled();
  });

  it('listMine делегирует репозиторию, если сессия есть', async () => {
    const summaries = [{ id: 'req-1' }] as unknown as Awaited<
      ReturnType<RequestsRepository['listByOwner']>
    >;
    const repo = fakeRepo({ listByOwner: vi.fn(async () => summaries) });
    const service = new RequestsService(repo, fakeQueue());

    await expect(service.listMine('hash-1')).resolves.toBe(summaries);
    expect(repo.listByOwner).toHaveBeenCalledWith('hash-1', expect.any(Number));
  });

  it('submitClarification отклоняет запрос не в статусе needs_clarification', async () => {
    const details = {
      id: 'req-1',
      status: 'processing',
      steps: [],
      document: null,
    } as unknown as RequestWithDetails;
    const repo = fakeRepo({ findRequestWithDetails: vi.fn(async () => details) });
    const service = new RequestsService(repo, fakeQueue());

    await expect(
      service.submitClarification('req-1', { answer: 'ответ' }, 'hash-1'),
    ).rejects.toBeInstanceOf(BadRequestException);
    expect(repo.setClarificationAnswer).not.toHaveBeenCalled();
  });

  it('submitClarification сохраняет ответ, возвращает в очередь и переоткрывает статус', async () => {
    const details = {
      id: 'req-1',
      status: 'needs_clarification',
      steps: [],
      document: null,
    } as unknown as RequestWithDetails;
    const repo = fakeRepo({ findRequestWithDetails: vi.fn(async () => details) });
    const queue = fakeQueue();
    const service = new RequestsService(repo, queue);

    await service.submitClarification('req-1', { answer: 'меня уволили' }, 'hash-1');

    expect(repo.setClarificationAnswer).toHaveBeenCalledWith('req-1', 'меня уволили');
    expect(repo.updateRequestStatus).toHaveBeenCalledWith('req-1', 'pending');
    expect(queue.add).toHaveBeenCalledWith('process', { requestId: 'req-1' });
  });

  it('submitClarification выбрасывает NotFoundException для чужой сессии', async () => {
    const details = {
      id: 'req-1',
      status: 'needs_clarification',
      steps: [],
      document: null,
    } as unknown as RequestWithDetails;
    const repo = fakeRepo({
      findRequestWithDetails: vi.fn(async () => details),
      isOwnedBy: vi.fn(async () => false),
    });
    const service = new RequestsService(repo, fakeQueue());

    await expect(
      service.submitClarification('req-1', { answer: 'ответ' }, 'чужой-hash'),
    ).rejects.toBeInstanceOf(NotFoundException);
  });
});
