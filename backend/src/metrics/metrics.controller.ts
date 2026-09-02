import { Controller, Get, Header, Inject } from '@nestjs/common';
import { MetricsService } from './metrics.service.js';

/**
 * Prometheus-совместимый эндпоинт для сборщика метрик. Не защищён аутентификацией — как и
 * большинство подобных эндпоинтов (Prometheus сам не умеет слать заголовки авторизации без
 * доп. конфигурации): в проде его закрывают на уровне сети/реверс-прокси, а не в коде.
 */
@Controller('metrics')
export class MetricsController {
  constructor(@Inject(MetricsService) private readonly metrics: MetricsService) {}

  @Get()
  @Header('Cache-Control', 'no-store')
  @Header('Content-Type', 'text/plain; version=0.0.4; charset=utf-8')
  async get(): Promise<string> {
    return this.metrics.exposition();
  }
}
