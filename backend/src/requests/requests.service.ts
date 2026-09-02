import { BadRequestException, Inject, Injectable, NotFoundException } from '@nestjs/common';
import { InjectQueue } from '@nestjs/bullmq';
import type { Queue } from 'bullmq';
import { REQUESTS_REPOSITORY, type RequestsRepository } from './requests.repository.js';
import {
  REQUEST_PROCESSING_QUEUE,
  type RequestProcessingJobData,
} from '../queue/queue.constants.js';
import type { CreateRequestDto } from './dto/create-request.dto.js';
import type { SubmitClarificationDto } from './dto/submit-clarification.dto.js';
import type { RequestRecord, RequestSummary, RequestWithDetails } from './request.types.js';

/** История обращений (Этап 13 — личный кабинет): сколько последних запросов сессии показываем. */
const HISTORY_LIMIT = 50;

@Injectable()
export class RequestsService {
  constructor(
    @Inject(REQUESTS_REPOSITORY) private readonly repo: RequestsRepository,
    @InjectQueue(REQUEST_PROCESSING_QUEUE) private readonly queue: Queue<RequestProcessingJobData>,
  ) {}

  async create(dto: CreateRequestDto, ownerTokenHash: string | null): Promise<RequestRecord> {
    const includeDocument = dto.includeDocument ?? false;
    if (includeDocument && !dto.documentType) {
      throw new BadRequestException('Укажите documentType, если includeDocument = true');
    }

    const request = await this.repo.createRequest({
      queryText: dto.queryText,
      includeDocument,
      documentType: dto.documentType ?? null,
      ownerTokenHash,
    });

    await this.queue.add('process', { requestId: request.id });
    return request;
  }

  /**
   * ownerTokenHash === null означает "запрашивающий не прислал cookie сессии" — доступ
   * разрешён только к запросам без владельца (см. RequestsRepository.isOwnedBy). Чужой запрос
   * скрывается за тем же NotFoundException, что и отсутствующий id, — не подтверждаем факт
   * существования чужого запроса.
   */
  async getById(id: string, ownerTokenHash: string | null): Promise<RequestWithDetails> {
    const [request, owned] = await Promise.all([
      this.repo.findRequestWithDetails(id),
      this.repo.isOwnedBy(id, ownerTokenHash),
    ]);
    if (!request || !owned) {
      throw new NotFoundException('Запрос не найден');
    }
    return request;
  }

  /**
   * Без cookie сессии некого искать — не идём в БД с null (это вернуло бы чужие "бесхозные"
   * записи, см. RequestsRepository.isOwnedBy), а просто отдаём пустую историю.
   */
  async listMine(ownerTokenHash: string | null): Promise<RequestSummary[]> {
    if (!ownerTokenHash) return [];
    return this.repo.listByOwner(ownerTokenHash, HISTORY_LIMIT);
  }

  /**
   * Ровно один раунд уточнения (Этап 13): принимает ответ только пока запрос в статусе
   * 'needs_clarification' — повторный вызов после того, как пайплайн уже возобновился,
   * отклоняется, а не тихо перезаписывает ответ.
   */
  async submitClarification(
    id: string,
    dto: SubmitClarificationDto,
    ownerTokenHash: string | null,
  ): Promise<void> {
    const [request, owned] = await Promise.all([
      this.repo.findRequestWithDetails(id),
      this.repo.isOwnedBy(id, ownerTokenHash),
    ]);
    if (!request || !owned) {
      throw new NotFoundException('Запрос не найден');
    }
    if (request.status !== 'needs_clarification') {
      throw new BadRequestException('Запрос не ожидает уточнения');
    }

    await this.repo.setClarificationAnswer(id, dto.answer);
    await this.repo.updateRequestStatus(id, 'pending');
    await this.queue.add('process', { requestId: id });
  }

  /**
   * Soft-cancel: помечает запрос 'cancelled', джобу из BullMQ не снимает — если пайплайн уже
   * идёт, оркестратор сам заметит смену статуса между шагами и остановится (см.
   * orchestrator.service.ts). Для ещё не взятой в работу джобы отмена срабатывает мгновенно.
   */
  async cancel(id: string, ownerTokenHash: string | null): Promise<void> {
    const owned = await this.repo.isOwnedBy(id, ownerTokenHash);
    if (!owned) {
      throw new NotFoundException('Запрос не найден');
    }
    const cancelled = await this.repo.cancelIfActive(id);
    if (!cancelled) {
      throw new BadRequestException('Запрос уже завершён и не может быть отменён');
    }
  }
}
