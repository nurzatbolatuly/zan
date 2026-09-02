import { Global, Module } from '@nestjs/common';
import { OpenAiChatClient } from '@zan/shared';
import { config } from '../config.js';
import { MetricsService } from '../metrics/metrics.service.js';
import { CHAT_CLIENT } from './llm.constants.js';

/**
 * Общий LLM-клиент (OpenAI) за DI-токеном — по тому же принципу, что SUPABASE_ADMIN в
 * db/db.module.ts. Нужен всем 4 агентам, поэтому это инфраструктурная зависимость
 * (@Global()), а не приватный провайдер модуля agents/search.
 *
 * Второй аргумент — колбэк, которым backend подключает к клиенту свою метрику
 * zan_llm_request_duration_ms (см. MetricsService.observeLlmCall); MetricsModule глобален, так
 * что MetricsService доступен здесь без прямого импорта его модуля (тот же паттерн, что у
 * OrchestratorService).
 */
@Global()
@Module({
  providers: [
    {
      provide: CHAT_CLIENT,
      useFactory: (metrics: MetricsService) =>
        new OpenAiChatClient(config.openAiApiKey, (info) =>
          metrics.observeLlmCall(info.model, info.operation, info.outcome, info.durationMs),
        ),
      inject: [MetricsService],
    },
  ],
  exports: [CHAT_CLIENT],
})
export class LlmModule {}
