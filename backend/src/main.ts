import 'reflect-metadata';
import cookieParser from 'cookie-parser';
import helmet from 'helmet';
import { NestFactory } from '@nestjs/core';
import { createLogger } from '@zan/shared';
import { AppModule } from './app.module.js';
import { config } from './config.js';
import { AllExceptionsFilter } from './common/all-exceptions.filter.js';
import { createHttpLoggingMiddleware } from './common/http-logging.middleware.js';
import { MetricsService } from './metrics/metrics.service.js';

const logger = createLogger('backend');

// Глобального ValidationPipe намеренно нет: он определяет DTO-класс по reflect-metadata
// параметра контроллера (design:paramtypes), а мы собираем backend через tsx (esbuild),
// который эту метадату не эмитит — Nest в этом случае молча пропускает валидацию.
// Каждый контроллер валидирует тело запроса явно через src/common/validate-dto.ts.

async function bootstrap(): Promise<void> {
  const app = await NestFactory.create(AppModule, { logger: false });
  app.enableShutdownHooks();

  // Этап 8 — безопасные заголовки (CSP/HSTS/X-Frame-Options/…) и cookie сессии владения
  // (см. src/common/session.ts). credentials: true нужен, чтобы браузер отправлял/принимал
  // cookie сессии при кросс-портовых (frontend:3001 → backend:3000) запросах в dev — оба
  // считаются "same-site" (localhost), поэтому SameSite=Lax этому не мешает.
  app.use(helmet());
  app.use(cookieParser());
  app.enableCors({ origin: config.frontendOrigin, credentials: true });
  app.useGlobalFilters(new AllExceptionsFilter());

  // Этап 9 — структурированный лог + метрика на каждый HTTP-запрос. Регистрируется руками
  // (не через MiddlewareConsumer), чтобы получить MetricsService напрямую из контейнера,
  // без дополнительного @Module.configure() ради одного middleware.
  app.use(createHttpLoggingMiddleware(app.get(MetricsService)));

  await app.listen(config.port);
  logger.info({ port: config.port }, 'Backend запущен');
}

bootstrap().catch((error) => {
  logger.error(
    { error: error instanceof Error ? error.message : String(error) },
    'Не удалось запустить backend',
  );
  process.exitCode = 1;
});
