import type { AnalyticsSummary, RequestDetails, RequestStatus, RequestSummary } from './types';

const API_BASE_URL = process.env.NEXT_PUBLIC_API_BASE_URL ?? 'http://localhost:3000';

export class ApiError extends Error {
  constructor(
    message: string,
    public readonly status: number,
  ) {
    super(message);
    this.name = 'ApiError';
  }
}

export interface CreateRequestInput {
  queryText: string;
  includeDocument: boolean;
  documentType: string | null;
}

export interface CreateRequestResponse {
  id: string;
  status: RequestStatus;
}

/**
 * Nest по умолчанию отдаёт ошибки как {statusCode, message, error}, где message — строка
 * либо массив строк (ошибки class-validator из validateDto, см. backend/src/common/validate-dto.ts).
 */
async function parseErrorMessage(response: Response): Promise<string> {
  try {
    const body: unknown = await response.json();
    if (body && typeof body === 'object' && 'message' in body) {
      const message = (body as { message: unknown }).message;
      if (Array.isArray(message)) return message.join('; ');
      if (typeof message === 'string') return message;
    }
  } catch {
    // Тело ответа не JSON — используем сообщение по статусу ниже.
  }
  return `Ошибка сервера (${response.status})`;
}

export async function createRequest(input: CreateRequestInput): Promise<CreateRequestResponse> {
  const response = await fetch(`${API_BASE_URL}/requests`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    // Нужно, чтобы браузер принял и потом отправлял cookie анонимной сессии (см.
    // backend/src/common/session.ts) — ей backend разграничивает доступ к чужим запросам.
    credentials: 'include',
    body: JSON.stringify(input),
  });
  if (!response.ok) {
    throw new ApiError(await parseErrorMessage(response), response.status);
  }
  return response.json() as Promise<CreateRequestResponse>;
}

export interface RequestDetailsResponse {
  data: RequestDetails;
  /** Момент ответа backend'а (заголовок Date, CORS-safelisted — доступен без настройки CORS
   *  на сервере). null, если заголовок почему-то отсутствует/не парсится. Используется на
   *  фронте, чтобы считать "прошло N сек" не от локальных часов клиента, а с поправкой на их
   *  рассинхрон с сервером (см. RequestStatusView.tsx). */
  serverTimeMs: number | null;
}

export async function fetchRequestDetails(id: string): Promise<RequestDetailsResponse> {
  const response = await fetch(`${API_BASE_URL}/requests/${id}`, {
    cache: 'no-store',
    credentials: 'include',
  });
  if (!response.ok) {
    throw new ApiError(await parseErrorMessage(response), response.status);
  }
  const dateHeader = response.headers.get('date');
  const parsedServerTimeMs = dateHeader ? Date.parse(dateHeader) : NaN;
  const data = (await response.json()) as RequestDetails;
  return { data, serverTimeMs: Number.isNaN(parsedServerTimeMs) ? null : parsedServerTimeMs };
}

/** Ровно один раунд уточнения (Этап 13) — см. backend/src/orchestrator/clarification-check.ts. */
export async function submitClarification(id: string, answer: string): Promise<void> {
  const response = await fetch(`${API_BASE_URL}/requests/${id}/clarification`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    credentials: 'include',
    body: JSON.stringify({ answer }),
  });
  if (!response.ok) {
    throw new ApiError(await parseErrorMessage(response), response.status);
  }
}

/** Soft-cancel (см. backend/src/requests/requests.service.ts) — отменить свой запрос, пока он
 *  ещё pending/processing/needs_clarification. */
export async function cancelRequest(id: string): Promise<void> {
  const response = await fetch(`${API_BASE_URL}/requests/${id}/cancel`, {
    method: 'POST',
    credentials: 'include',
  });
  if (!response.ok) {
    throw new ApiError(await parseErrorMessage(response), response.status);
  }
}

/** История обращений (Этап 13 — личный кабинет): запросы текущей анонимной сессии. */
export async function fetchRequestHistory(): Promise<RequestSummary[]> {
  const response = await fetch(`${API_BASE_URL}/requests`, {
    cache: 'no-store',
    credentials: 'include',
  });
  if (!response.ok) {
    throw new ApiError(await parseErrorMessage(response), response.status);
  }
  return response.json() as Promise<RequestSummary[]>;
}

export async function fetchAnalytics(): Promise<AnalyticsSummary> {
  const response = await fetch(`${API_BASE_URL}/analytics/topics`, { cache: 'no-store' });
  if (!response.ok) {
    throw new ApiError(await parseErrorMessage(response), response.status);
  }
  return response.json() as Promise<AnalyticsSummary>;
}
