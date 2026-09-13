import { Inject, Injectable } from '@nestjs/common';
import { createLogger, type Logger } from '@zan/shared';
import { REQUESTS_REPOSITORY, type RequestsRepository } from '../requests/requests.repository.js';
import type { AgentName } from '../requests/request.types.js';
import { RealtimeGateway } from './realtime.gateway.js';
import type { RequestEventsPublisher } from './request-events.types.js';

@Injectable()
export class RequestEventsService implements RequestEventsPublisher {
  private readonly logger: Logger = createLogger('request-events');

  constructor(
    @Inject(REQUESTS_REPOSITORY) private readonly repo: RequestsRepository,
    @Inject(RealtimeGateway) private readonly gateway: RealtimeGateway,
  ) {}

  async publishSnapshot(requestId: string): Promise<void> {
    // Если на requestId никто не подписан — не тратим лишний SELECT на каждый шаг пайплайна
    // (см. RealtimeGateway.hasSubscribers): большинство джоб в очереди в моменте обрабатываются
    // без открытой вкладки браузера, REST-поллинг тут не нужен вообще.
    if (!this.gateway.hasSubscribers(requestId)) return;
    try {
      const details = await this.repo.findRequestWithDetails(requestId);
      if (!details) return;
      this.gateway.broadcast(requestId, { type: 'snapshot', data: details });
    } catch (error) {
      // Побочный эффект для UX (см. RequestEventsPublisher) — сбой публикации не должен уронить
      // пайплайн; REST-поллинг фронтенда (фолбэк, см. RequestStatusView.tsx) всё равно подхватит
      // актуальное состояние на следующем тике.
      this.logger.warn(
        { error: error instanceof Error ? error.message : String(error), requestId },
        'Не удалось опубликовать снапшот через WS',
      );
    }
  }

  publishToken(requestId: string, agentName: AgentName, delta: string): void {
    this.gateway.broadcast(requestId, { type: 'token', agentName, delta });
  }
}
