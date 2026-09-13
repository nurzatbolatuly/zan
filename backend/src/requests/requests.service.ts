import { BadRequestException, Inject, Injectable, NotFoundException } from '@nestjs/common';
import { InjectQueue } from '@nestjs/bullmq';
import type { Queue } from 'bullmq';
import { REQUESTS_REPOSITORY, type RequestsRepository } from './requests.repository.js';
import {
  REQUEST_PROCESSING_QUEUE,
  type RequestProcessingJobData,
} from '../queue/queue.constants.js';
import type { CreateRequestDto } from './dto/create-request.dto.js';
import type { CreateDocumentDto } from './dto/create-document.dto.js';
import type { SubmitClarificationDto } from './dto/submit-clarification.dto.js';
import type { RequestRecord, RequestSummary, RequestWithDetails } from './request.types.js';
import { REQUEST_EVENTS, type RequestEventsPublisher } from '../realtime/request-events.types.js';

/** История обращений (Этап 13 — личный кабинет): сколько последних запросов сессии показываем. */
const HISTORY_LIMIT = 50;

@Injectable()
export class RequestsService {
  constructor(
    @Inject(REQUESTS_REPOSITORY) private readonly repo: RequestsRepository,
    @InjectQueue(REQUEST_PROCESSING_QUEUE) private readonly queue: Queue<RequestProcessingJobData>,
    @Inject(REQUEST_EVENTS) private readonly events: RequestEventsPublisher,
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
    // WS-канал этого requestId уже закрыт (см. RealtimeGateway.broadcast — needs_clarification
    // paused-статус) — снапшот здесь публиковать некому, фронт переоткрывает подключение сам,
    // явно, при отправке уточнения (см. RequestStatusView.tsx: resumeLiveUpdates).
    await this.queue.add('process', { requestId: id });
  }

  /**
   * Этап 18 (§9.14): догенерация документа к уже завершённому ответу — пользователь сначала
   * получил обычный текстовый ответ, а решение приложить документ принял, уже увидев его (не
   * заранее, через чекбокс в композере, см. create()). Требует status='completed' с непустым
   * resultSummary — иначе агенту document нечего использовать как готовый ответ по делу (а сам
   * запрос ещё либо не начинался, либо идёт, либо ждёт уточнения/провалился).
   * Явно запрещаем повторный вызов, если документ уже есть, — CreateDocumentDto не даёт способа
   * заменить документ другим типом, только создать первый (см. requests.repository.ts
   * setDocumentRequested — общий статус запроса не трогается, поэтому WS-канал уже закрыт;
   * прогресс генерации фронт отслеживает поллингом по request_steps, см. RequestStatusView.tsx).
   */
  async requestDocument(
    id: string,
    dto: CreateDocumentDto,
    ownerTokenHash: string | null,
  ): Promise<void> {
    const [request, owned] = await Promise.all([
      this.repo.findRequestWithDetails(id),
      this.repo.isOwnedBy(id, ownerTokenHash),
    ]);
    if (!request || !owned) {
      throw new NotFoundException('Запрос не найден');
    }
    if (request.status !== 'completed' || request.resultSummary === null) {
      throw new BadRequestException(
        'Документ можно составить только к уже полученному ответу на вопрос',
      );
    }
    if (request.document) {
      throw new BadRequestException('Документ для этого ответа уже создан');
    }

    await this.repo.setDocumentRequested(id, dto.documentType);
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
    // Пайплайн сам заметит отмену только между шагами (см. orchestrator.service.ts) — публикуем
    // снапшот сразу, чтобы открытая вкладка увидела 'cancelled' немедленно, а не только когда
    // (если) оркестратор доберётся до следующей проверки статуса.
    await this.events.publishSnapshot(id);
  }
}
