import { Module } from '@nestjs/common';
import { APP_GUARD } from '@nestjs/core';
import { BullModule } from '@nestjs/bullmq';
import { ThrottlerGuard, ThrottlerModule } from '@nestjs/throttler';
import { config } from './config.js';
import { DbModule } from './db/db.module.js';
import { LlmModule } from './llm/llm.module.js';
import { MetricsModule } from './metrics/metrics.module.js';
import { AnalyticsModule } from './analytics/analytics.module.js';
import { RequestsModule } from './requests/requests.module.js';
import { OrchestratorModule } from './orchestrator/orchestrator.module.js';
import { QueueModule } from './queue/queue.module.js';
import { RealtimeModule } from './realtime/realtime.module.js';
import { parseRedisConnection } from './common/redis-connection.js';

@Module({
  imports: [
    DbModule,
    LlmModule,
    MetricsModule,
    AnalyticsModule,
    // Rate limiting (Этап 8 — защита от abuse): один общий лимит на IP по умолчанию, POST
    // /requests (запускает 4 вызова LLM) дополнительно ограничен строже через @Throttle
    // на самом контроллере (см. requests.controller.ts).
    ThrottlerModule.forRoot([{ ttl: 60_000, limit: 60 }]),
    BullModule.forRoot({
      connection: {
        ...parseRedisConnection(config.redisUrl),
        // Обязательное требование BullMQ для соединений воркера/очереди.
        maxRetriesPerRequest: null,
      },
    }),
    RequestsModule,
    OrchestratorModule,
    QueueModule,
    RealtimeModule,
  ],
  providers: [{ provide: APP_GUARD, useClass: ThrottlerGuard }],
})
export class AppModule {}
