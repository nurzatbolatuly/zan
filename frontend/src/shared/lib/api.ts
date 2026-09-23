import { logger } from "./logger";
import { createTraceId } from "./traceId";
import { getApiBaseUrl } from "./env";
import { useSessionStore } from "@/shared/stores/useSessionStore";
import { useAdminAuthStore } from "@/shared/stores/useAdminAuthStore";
import type { ApiErrorBody } from "@/shared/types/apiError";

/**
 * Единственная точка похода в сеть (FRONT_CODING_STANDARDS.md §1, §4.2).
 * Каждый запрос логируется автоматически (старт/успех/ошибка) — фиче
 * не нужно логировать сам факт HTTP-вызова руками, только доменные события.
 *
 * Сессия — `Authorization: Session <token>` (bearer, не cookie — решение
 * Stage 6, см. instructions.md «Деплой»/«Общий словарь FE↔BE»): токен
 * читается из useSessionStore и добавляется автоматически, когда есть.
 * `401 session_invalid` — бэк никогда не создаёт сессию сам (см.
 * openapi.yaml#/sessions/me) — apiFetch создаёт новую и повторяет запрос
 * один раз (см. ensureSession/handleSessionInvalid ниже), не зацикливаясь.
 */

export class ApiError extends Error {
  readonly status: number;
  readonly traceId: string;
  readonly body: ApiErrorBody | null;

  constructor(
    message: string,
    status: number,
    traceId: string,
    body: ApiErrorBody | null,
  ) {
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
  /** Добавляет X-Admin-Token из useAdminAuthStore (/admin/* роуты). */
  admin?: boolean;
  /** Внутреннее — не повторять запрос ещё раз при повторном 401 (защита от цикла). */
  skipSessionRetry?: boolean;
}

function isFormData(body: unknown): body is FormData {
  return typeof FormData !== "undefined" && body instanceof FormData;
}

async function performFetch<T>(
  path: string,
  options: ApiFetchOptions,
  traceId: string,
): Promise<T> {
  const { body, headers, method = "GET", admin, skipSessionRetry, ...rest } = options;
  const url = `${getApiBaseUrl()}${path}`;

  const requestHeaders: Record<string, string> = {
    "X-Trace-Id": traceId,
    ...(headers as Record<string, string> | undefined),
  };

  const sessionToken = useSessionStore.getState().sessionToken;
  if (sessionToken) requestHeaders.Authorization = `Session ${sessionToken}`;

  if (admin) {
    const adminToken = useAdminAuthStore.getState().adminToken;
    if (!adminToken) {
      throw new ApiError("Admin-токен не задан", 401, traceId, {
        code: "admin_unauthorized",
        message: "Admin-токен не задан",
      });
    }
    requestHeaders["X-Admin-Token"] = adminToken;
  }

  let requestBody: BodyInit | undefined;
  if (isFormData(body)) {
    requestBody = body;
  } else if (body !== undefined) {
    requestHeaders["Content-Type"] = "application/json";
    requestBody = JSON.stringify(body);
  }

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
      headers: requestHeaders,
      body: requestBody,
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
  const payload: unknown = contentType.includes("application/json")
    ? await response.json().catch(() => null)
    : null;

  if (!response.ok) {
    const errorBody = (payload ?? null) as ApiErrorBody | null;

    if (
      response.status === 401 &&
      errorBody?.code === "session_invalid" &&
      !skipSessionRetry
    ) {
      logger.info({
        scope: "api",
        event: "session_invalid_retry",
        traceId,
        data: { path },
      });
      useSessionStore.getState().clearSession();
      await ensureSession();
      return performFetch<T>(path, { ...options, skipSessionRetry: true }, traceId);
    }

    logger.error({
      scope: "api",
      event: "request_failed",
      traceId,
      data: { method, path, status: response.status, durationMs },
      error: errorBody,
    });
    throw new ApiError(
      errorBody?.message ?? `Запрос ${path} завершился ошибкой ${response.status}`,
      response.status,
      traceId,
      errorBody,
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

export async function apiFetch<T>(
  path: string,
  options: ApiFetchOptions = {},
): Promise<T> {
  const traceId = options.traceId ?? createTraceId();
  return performFetch<T>(path, options, traceId);
}

export const api = {
  get: <T>(path: string, options?: ApiFetchOptions) =>
    apiFetch<T>(path, { ...options, method: "GET" }),
  post: <T>(path: string, body?: unknown, options?: ApiFetchOptions) =>
    apiFetch<T>(path, { ...options, method: "POST", body }),
  put: <T>(path: string, body?: unknown, options?: ApiFetchOptions) =>
    apiFetch<T>(path, { ...options, method: "PUT", body }),
  patch: <T>(path: string, body?: unknown, options?: ApiFetchOptions) =>
    apiFetch<T>(path, { ...options, method: "PATCH", body }),
  delete: <T>(path: string, options?: ApiFetchOptions) =>
    apiFetch<T>(path, { ...options, method: "DELETE" }),
};

/** `?a=1&b=2` — пропускает undefined/пустую строку, не кодирует пустой набор в "?". */
export function buildQueryString(
  params: Record<string, string | number | undefined>,
): string {
  const search = new URLSearchParams();
  for (const [key, value] of Object.entries(params)) {
    if (value === undefined || value === "") continue;
    search.set(key, String(value));
  }
  const query = search.toString();
  return query ? `?${query}` : "";
}

interface CreateSessionResponse {
  session_token: string;
  session_id: string;
}

let sessionBootPromise: Promise<void> | null = null;

/**
 * Гарантирует, что в useSessionStore есть токен — создаёт анонимную сессию
 * (`POST /sessions`), если её ещё нет. Вызывается один раз на старте
 * приложения (`SessionBoot` в AppProviders.tsx) и переиспользуется для
 * восстановления после `401 session_invalid` выше. Конкурентные вызовы
 * (несколько запросов упали в 401 одновременно) шарят один и тот же промис,
 * не создают по сессии на каждый.
 */
export function ensureSession(): Promise<void> {
  if (useSessionStore.getState().sessionToken) return Promise.resolve();
  if (!sessionBootPromise) {
    sessionBootPromise = performFetch<CreateSessionResponse>(
      "/sessions",
      { method: "POST", skipSessionRetry: true },
      createTraceId(),
    )
      .then((res) => {
        useSessionStore.getState().setSession({
          sessionId: res.session_id,
          sessionToken: res.session_token,
        });
      })
      .finally(() => {
        sessionBootPromise = null;
      });
  }
  return sessionBootPromise;
}
