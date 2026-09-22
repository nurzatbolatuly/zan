import { logger } from "./logger";
import { createTraceId } from "./traceId";
import { getApiBaseUrl } from "./env";

/**
 * Единственная точка похода в сеть (FRONT_CODING_STANDARDS.md §1, §4.2).
 * Каждый запрос логируется автоматически (старт/успех/ошибка) — фиче
 * не нужно логировать сам факт HTTP-вызова руками, только доменные события.
 *
 * Аутентификация/сессия (session_id) сюда намеренно не добавлена: механизм
 * не зафиксирован в backend-roadmap.md (см. PLAN.md §9, открытый вопрос) —
 * добавляется в Stage 6 вместе с решением, а не угадывается сейчас.
 */

export class ApiError extends Error {
  readonly status: number;
  readonly traceId: string;
  readonly body: unknown;

  constructor(message: string, status: number, traceId: string, body: unknown) {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.traceId = traceId;
    this.body = body;
  }
}

interface ApiFetchOptions extends Omit<RequestInit, "body"> {
  body?: unknown;
  /** Свой traceId, если запрос — часть более крупной операции (FRONT_CODING_STANDARDS.md §4.4) */
  traceId?: string;
}

export async function apiFetch<T>(
  path: string,
  options: ApiFetchOptions = {},
): Promise<T> {
  const { body, traceId = createTraceId(), headers, method = "GET", ...rest } = options;
  const url = `${getApiBaseUrl()}${path}`;
  const startedAt = performance.now();

  logger.info({
    scope: "api",
    event: "request_started",
    traceId,
    data: { method, path },
  });

  let response: Response;
  try {
    response = await fetch(url, {
      ...rest,
      method,
      headers: {
        "Content-Type": "application/json",
        "X-Trace-Id": traceId,
        ...headers,
      },
      body: body !== undefined ? JSON.stringify(body) : undefined,
    });
  } catch (error) {
    logger.error({
      scope: "api",
      event: "request_failed",
      traceId,
      data: { method, path, reason: "network" },
      error,
    });
    throw new ApiError("Сеть недоступна, попробуйте ещё раз", 0, traceId, null);
  }

  const durationMs = Math.round(performance.now() - startedAt);
  const contentType = response.headers.get("content-type") ?? "";
  const payload = contentType.includes("application/json")
    ? await response.json().catch(() => null)
    : null;

  if (!response.ok) {
    logger.error({
      scope: "api",
      event: "request_failed",
      traceId,
      data: { method, path, status: response.status, durationMs },
      error: payload,
    });
    throw new ApiError(
      `Запрос ${path} завершился ошибкой ${response.status}`,
      response.status,
      traceId,
      payload,
    );
  }

  logger.info({
    scope: "api",
    event: "request_succeeded",
    traceId,
    data: { method, path, status: response.status, durationMs },
  });

  return payload as T;
}

export const api = {
  get: <T>(path: string, options?: ApiFetchOptions) =>
    apiFetch<T>(path, { ...options, method: "GET" }),
  post: <T>(path: string, body?: unknown, options?: ApiFetchOptions) =>
    apiFetch<T>(path, { ...options, method: "POST", body }),
  patch: <T>(path: string, body?: unknown, options?: ApiFetchOptions) =>
    apiFetch<T>(path, { ...options, method: "PATCH", body }),
  delete: <T>(path: string, options?: ApiFetchOptions) =>
    apiFetch<T>(path, { ...options, method: "DELETE" }),
};
