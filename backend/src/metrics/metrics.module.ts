import { Global, Module } from '@nestjs/common';
import { MetricsService } from './metrics.service.js';
import { MetricsController } from './metrics.controller.js';

/**
 * @Global() по тому же принципу, что DbModule/LlmModule: метрики — сквозная инфраструктурная
 * зависимость, нужная оркестратору, HTTP-логированию и (в будущем) любому новому агенту, а не
 * только контроллеру /metrics.
 */
@Global()
@Module({
  controllers: [MetricsController],
  providers: [MetricsService],
  exports: [MetricsService],
})
export class MetricsModule {}
