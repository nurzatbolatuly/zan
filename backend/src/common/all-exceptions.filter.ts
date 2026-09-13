import type { ArgumentsHost, ExceptionFilter } from '@nestjs/common';
import { Catch, HttpException, HttpStatus } from '@nestjs/common';
import type { Request, Response } from 'express';
import { createLogger, type Logger } from '@zan/shared';
import { REQUEST_ID_HEADER } from './http-logging.middleware.js';

/**
 * Supabase-js не оборачивает ошибку PostgREST в настоящий `Error` без явного `.throwOnError()`
 * на каждом запросе (см. requests.repository.ts — этого нигде нет) — `{ data, error } = await
 * ...; if (error) throw error;` бросает голый JSON-объект `{message, code, details, hint}`.
 * `exception instanceof Error` для него `false`, а `String(plainObject)` даёт бесполезное
 * "[object Object]" в логах — отсюда была нечитаемая ошибка при отмене запроса (недостающее
 * значение enum). Эта функция вытаскивает читаемое сообщение из обоих видов исключений.
 */
function formatError(exception: unknown): string {
  if (exception instanceof Error) return exception.message;
  if (
    exception &&
    typeof exception === 'object' &&
    'message' in exception &&
    typeof (exception as { message?: unknown }).message === 'string'
  ) {
    const { message, code, details, hint } = exception as {
      message: string;
      code?: string;
      details?: string;
      hint?: string;
    };
    return [message, code && `code=${code}`, details, hint].filter(Boolean).join(' | ');
  }
  try {
    return JSON.stringify(exception);
  } catch {
    return String(exception);
  }
}

/**
 * Этап 8/9: (а) не даёт непойманной ошибке утечь клиенту как стектрейс/внутреннее сообщение
 * (безопасность — не раскрываем детали реализации), (б) логирует каждую 5xx-ошибку через pino
 * с request_id для трассировки (main.ts запускает Nest с `logger: false`, поэтому без этого
 * фильтра необработанные ошибки нигде не оседали бы в структурированных логах).
 */
@Catch()
export class AllExceptionsFilter implements ExceptionFilter {
  private readonly logger: Logger = createLogger('http-exception');

  catch(exception: unknown, host: ArgumentsHost): void {
    const ctx = host.switchToHttp();
    const response = ctx.getResponse<Response>();
    const request = ctx.getRequest<Request>();

    const isHttpException = exception instanceof HttpException;
    const status = isHttpException ? exception.getStatus() : HttpStatus.INTERNAL_SERVER_ERROR;
    const rawBody = isHttpException
      ? exception.getResponse()
      : { statusCode: status, message: 'Внутренняя ошибка сервера' };
    // getResponse() отдаёт голую строку для части встроенных исключений Nest (например,
    // ThrottlerException) — приводим к единой форме {statusCode, message}, которую уже
    // ожидает frontend (см. frontend/src/lib/api.ts parseErrorMessage).
    const body = typeof rawBody === 'string' ? { statusCode: status, message: rawBody } : rawBody;

    if (status >= HttpStatus.INTERNAL_SERVER_ERROR) {
      this.logger.error(
        {
          requestId: request.headers[REQUEST_ID_HEADER],
          method: request.method,
          path: request.path,
          error: formatError(exception),
          // Без stack сообщение вроде "Cannot read properties of undefined" не даёт зацепиться
          // за место в коде — только текст без stack бесполезен для реальной отладки 500-х.
          stack: exception instanceof Error ? exception.stack : undefined,
        },
        'Необработанная ошибка запроса',
      );
    }

    response.status(status).json(body);
  }
}
