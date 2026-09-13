import { Global, Module } from '@nestjs/common';
import { RequestsModule } from '../requests/requests.module.js';
import { RealtimeGateway } from './realtime.gateway.js';
import { RequestEventsService } from './request-events.service.js';
import { REQUEST_EVENTS } from './request-events.types.js';

/**
 * Глобальный, как LlmModule (см. llm/llm.module.ts) — REQUEST_EVENTS нужен и
 * OrchestratorService, и RequestsService (requests/requests.service.ts), подключать его отдельно
 * в оба модуля избыточно. RealtimeGateway экспортирован отдельно (не только через REQUEST_EVENTS)
 * — main.ts достаёт его напрямую через app.get(), чтобы привязать к HTTP-серверу после
 * app.listen() (см. main.ts).
 */
@Global()
@Module({
  imports: [RequestsModule],
  providers: [RealtimeGateway, { provide: REQUEST_EVENTS, useClass: RequestEventsService }],
  exports: [REQUEST_EVENTS, RealtimeGateway],
})
export class RealtimeModule {}
