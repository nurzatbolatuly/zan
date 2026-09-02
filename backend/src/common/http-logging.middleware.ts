import { randomUUID } from 'node:crypto';
import type { NextFunction, Request, Response } from 'express';
import { createLogger, type Logger } from '@zan/shared';
import { MetricsService } from '../metrics/metrics.service.js';

export const REQUEST_ID_HEADER = 'x-request-id';

/**
 * Этап 9 (наблюдаемость): один структурированный лог на каждый HTTP-запрос (метод, путь,
 * статус, длительность, request_id), плюс метрика для Prometheus. Отдельная сущность от
 * request_id пайплайна (requests.id, см. orchestrator.service.ts) — этот id существует уже
 * на входе в API, до того как запрос вообще создан (или даже если он будет отклонён валидацией).
 */

/** IP — персональные данные; храним/логируем в усечённом виде (маскируем последний октет/группу). */
function maskIp(ip: string | undefined): string {
  if (!ip) return 'unknown';
  if (ip.includes('.')) {
    const parts = ip.split('.');
    return parts.length === 4 ? `${parts[0]}.${parts[1]}.${parts[2]}.xxx` : 'unknown';
  }
  const groups = ip.split(':');
  return groups.length > 2 ? `${groups.slice(0, 4).join(':')}::xxxx` : 'unknown';
}

/** UUID в пути (/requests/:id) заменяем плейсхолдером — иначе лейбл route в метриках взорвётся по кардинальности. */
function normalizeRoute(path: string): string {
  return path.replace(/[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}/gi, ':id');
}

export function createHttpLoggingMiddleware(metrics: MetricsService) {
  const logger: Logger = createLogger('http');

  return function httpLoggingMiddleware(req: Request, res: Response, next: NextFunction): void {
    const startedAt = Date.now();
    const incomingRequestId = req.header(REQUEST_ID_HEADER);
    const requestId =
      incomingRequestId && incomingRequestId.length <= 100 ? incomingRequestId : randomUUID();
    req.headers[REQUEST_ID_HEADER] = requestId;
    res.setHeader(REQUEST_ID_HEADER, requestId);

    res.on('finish', () => {
      const durationMs = Date.now() - startedAt;
      const route = normalizeRoute(req.path);
      logger.info(
        {
          requestId,
          method: req.method,
          path: route,
          status: res.statusCode,
          duration_ms: durationMs,
          ip: maskIp(req.ip),
        },
        'http.request',
      );
      metrics.observeHttpRequest(req.method, route, res.statusCode, durationMs);
    });

    next();
  };
}
