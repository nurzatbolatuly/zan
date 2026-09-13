import { Injectable, OnModuleDestroy } from '@nestjs/common';
import { Counter, Histogram, Registry, collectDefaultMetrics } from 'prom-client';
import type { AgentName, RequestStatus, RequestStepStatus } from '../requests/request.types.js';

/**
 * Этап 9 (наблюдаемость): метрики для сборки Prometheus-совместимым коллектором (GET /metrics,
 * см. metrics.controller.ts). Единственное место, где создаются counters/histograms — агенты и
 * оркестратор только вызывают методы этого сервиса (DRY, Single Responsibility), не работают
 * с prom-client напрямую.
 */
@Injectable()
export class MetricsService implements OnModuleDestroy {
  readonly registry = new Registry();

  private readonly httpRequestDuration = new Histogram({
    name: 'zan_http_request_duration_ms',
    help: 'Длительность HTTP-запроса к backend API в миллисекундах',
    labelNames: ['method', 'route', 'status_code'] as const,
    buckets: [10, 50, 100, 250, 500, 1000, 2500, 5000, 10000],
    registers: [this.registry],
  });

  private readonly pipelineStepDuration = new Histogram({
    name: 'zan_pipeline_step_duration_ms',
    help: 'Длительность шага пайплайна агентов в миллисекундах',
    labelNames: ['agent', 'status'] as const,
    buckets: [100, 250, 500, 1000, 2500, 5000, 10000, 20000, 40000],
    registers: [this.registry],
  });

  private readonly pipelineStepTotal = new Counter({
    name: 'zan_pipeline_step_total',
    help: 'Количество завершённых шагов пайплайна по агенту и статусу',
    labelNames: ['agent', 'status'] as const,
    registers: [this.registry],
  });

  private readonly requestsTotal = new Counter({
    name: 'zan_requests_total',
    help: 'Количество пользовательских запросов по итоговому статусу',
    labelNames: ['status'] as const,
    registers: [this.registry],
  });

  /**
   * Задержка отдельного сырого вызова OpenAI (не шага пайплайна целиком — шаг может включать
   * запись в БД и логирование вокруг вызова, см. zan_pipeline_step_duration_ms). Лейбл `model`
   * — ключевой: переключение DEFAULT_CHAT_MODEL (см. chatClient.ts) должно быть видно здесь
   * в реальном времени по каждой модели отдельно, а не задним числом по логам.
   */
  private readonly llmRequestDuration = new Histogram({
    name: 'zan_llm_request_duration_ms',
    help: 'Длительность одного сырого вызова OpenAI в миллисекундах',
    labelNames: ['model', 'operation', 'outcome'] as const,
    buckets: [250, 500, 1000, 2500, 5000, 10000, 20000, 40000, 60000, 90000, 120000],
    registers: [this.registry],
  });

  private readonly llmRequestTotal = new Counter({
    name: 'zan_llm_request_total',
    help: 'Количество сырых вызовов OpenAI по модели, типу вызова и исходу',
    labelNames: ['model', 'operation', 'outcome'] as const,
    registers: [this.registry],
  });

  constructor() {
    collectDefaultMetrics({ register: this.registry, prefix: 'zan_process_' });
  }

  observeHttpRequest(method: string, route: string, statusCode: number, durationMs: number): void {
    this.httpRequestDuration.observe(
      { method, route, status_code: String(statusCode) },
      durationMs,
    );
  }

  observePipelineStep(agent: AgentName, status: RequestStepStatus, durationMs: number): void {
    this.pipelineStepDuration.observe({ agent, status }, durationMs);
    this.pipelineStepTotal.inc({ agent, status });
  }

  observeRequestCompleted(status: RequestStatus): void {
    this.requestsTotal.inc({ status });
  }

  observeLlmCall(model: string, operation: string, outcome: string, durationMs: number): void {
    this.llmRequestDuration.observe({ model, operation, outcome }, durationMs);
    this.llmRequestTotal.inc({ model, operation, outcome });
  }

  async exposition(): Promise<string> {
    return this.registry.metrics();
  }

  get contentType(): string {
    return this.registry.contentType;
  }

  onModuleDestroy(): void {
    this.registry.clear();
  }
}
