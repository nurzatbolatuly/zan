/**
 * 1:1 с `openapi/openapi.yaml#ApiErrorCode` (BACKEND_PLAN.md §6 п.6, закрыто
 * Stage 8) — полный каталог кодов, которые реально отдаёт backend/ в теле
 * `ApiError`. Меняется только вместе с openapi.yaml, не по догадке.
 */
export type ApiErrorCode =
  | "admin_blocked"
  | "admin_unauthorized"
  | "agent_type_not_found"
  | "document_generation_failed"
  | "document_not_generated"
  | "empty_transcript"
  | "file_not_found"
  | "file_rejected"
  | "file_too_large"
  | "internal_error"
  | "invalid_request"
  | "payment_not_found"
  | "payment_not_pending"
  | "payment_required"
  | "rate_limited"
  | "service_not_found"
  | "session_invalid"
  | "tariff_not_found"
  | "thread_busy"
  | "thread_cannot_cancel"
  | "thread_not_active"
  | "thread_not_found"
  | "unsupported_file_type"
  | "voice_transcribe_failed";

export interface ApiErrorBody {
  code: ApiErrorCode;
  message: string;
}
