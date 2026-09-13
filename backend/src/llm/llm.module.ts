import { Global, Module } from '@nestjs/common';
import { OpenAiChatClient, createLogger } from '@zan/shared';
import { config } from '../config.js';
import { MetricsService } from '../metrics/metrics.service.js';
import { CHAT_CLIENT } from './llm.constants.js';

/**
 * Общий LLM-клиент (OpenAI) за DI-токеном — по тому же принципу, что SUPABASE_ADMIN в
 * db/db.module.ts. Нужен обоим агентам (answer, document), поэтому это инфраструктурная
 * зависимость (@Global()), а не приватный провайдер модуля agents/answer.
 *
 * Второй аргумент — колбэк, которым backend подключает к клиенту метрику
 * zan_llm_request_duration_ms (см. MetricsService.observeLlmCall) — агрегированный тренд по
 * модели/операции/исходу — И структурированный лог `openai.call` с requestId, если он был
 * передан (см. ChatCompletionRequest.requestId в chatClient.ts). Это второе — прямой ответ на
 * запрос "сколько реально ждём OpenAI на этот конкретный запрос, отдельно от общего времени
 * обработки": Prometheus-гистограмма агрегирует по всем запросам сразу, из неё нельзя достать
 * время одного конкретного requestId; лог — можно, просто фильтром по нему.
 */
@Global()
@Module({
  providers: [
    {
      provide: CHAT_CLIENT,
      useFactory: (metrics: MetricsService) => {
        const logger = createLogger('openai-client');
        return new OpenAiChatClient(config.openAiApiKey, (info) => {
          metrics.observeLlmCall(info.model, info.operation, info.outcome, info.durationMs);
          const logMeta = {
            requestId: info.requestId,
            model: info.model,
            operation: info.operation,
            outcome: info.outcome,
            duration_ms: info.durationMs,
          };
          // outcome: 'error' логировался на уровне info вместе с успехами — сырой сбой вызова
          // OpenAI (таймаут/rate limit/5xx) тонул в обычном трафике и не отличался от него ни
          // визуально, ни для алертинга по error-level (см. AllExceptionsFilter/orchestrator,
          // которые ловят это позже, но без деталей самого сырого вызова — модель/операция/duration).
          if (info.outcome === 'error') {
            logger.error(logMeta, 'openai.call');
          } else {
            logger.info(logMeta, 'openai.call');
          }
        });
      },
      inject: [MetricsService],
    },
  ],
  exports: [CHAT_CLIENT],
})
export class LlmModule {}
