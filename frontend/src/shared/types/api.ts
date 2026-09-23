/**
 * "Wire"-типы — 1:1 с `openapi/openapi.yaml` (snake_case как в реальном
 * JSON, без адаптации под фронтовый naming). Единственное место, где
 * дублируется форма ответов backend — фичи не объявляют свои копии этих
 * полей, а мапят wire-тип отсюда в свой доменный camelCase-тип (пример:
 * `features/history/types.ts#Thread` + мэппер поверх `ThreadDto`).
 * Нет `-Dto`-суффикса у самих JSON-полей (они и так snake_case, лишний
 * суффикс не добавляет ясности) — суффикс только у имени типа, где он
 * может столкнуться с одноимённым доменным типом фичи (`ThreadDto` vs
 * `Thread`), иначе имя как в openapi.yaml.
 */
import type { Lang, Theme, ThreadStatus } from "./common";
import type { ServiceId } from "./tariff";

export interface Session {
  session_id: string;
  language: Lang;
  theme: Theme | null;
  onboarding_seen: boolean;
}

export interface CreateSessionResponseDto {
  session_token: string;
  session_id: string;
}

export interface ServiceDto {
  id: ServiceId;
  type_label: string;
  name: string;
  price: number;
  is_active: boolean;
}

export interface BundleItemDto {
  service_id: ServiceId;
  qty: number;
}

export interface TariffDto {
  id: string;
  name: string;
  discount_percent: number;
  items: BundleItemDto[];
  subtotal: number;
  total: number;
  is_active: boolean;
}

export interface TariffWriteRequestDto {
  name: string;
  discount_percent: number;
  items: BundleItemDto[];
}

export interface BalanceEntryDto {
  service_id: ServiceId;
  quantity: number;
}

export type PaymentKind = "single_service" | "tariff" | "custom";
export type PaymentStatus = "pending" | "success" | "failed";

export interface CheckoutRequestDto {
  kind: PaymentKind;
  tariff_id?: string;
  items?: BundleItemDto[];
  thread_id?: string;
}

export interface CheckoutResponseDto {
  payment_id: string;
  amount: number;
}

export interface PaymentDto {
  id: string;
  kind: PaymentKind;
  amount: number;
  status: PaymentStatus;
  provider: string;
  paid_at: string | null;
}

export type AgentType = "qa" | "document";

export interface AgentPromptDto {
  agent_type: AgentType;
  prompt_text: string;
  updated_at: string;
}

export type MessageSender = "user" | "assistant";
export type MessageInputType = "text" | "voice" | "file";
export type MessageFeedbackValue = "like" | "dislike";

export interface SourceDto {
  ref: string;
  quote: string;
}

export interface FindingDto {
  title: string;
  body: string;
}

/** Карточка файла в сообщении (`openapi.yaml#MessageAttachment`) — без ссылки и текста. */
export interface MessageAttachmentDto {
  file_id: string;
  original_name: string;
  mime_type: string;
  size_bytes: number;
}

export interface MessageDto {
  id: string;
  sender: MessageSender;
  input_type: MessageInputType;
  text: string;
  sources?: SourceDto[];
  findings?: FindingDto[];
  feedback: MessageFeedbackValue | null;
  processing_time_ms: number | null;
  created_at: string;
  attachments: MessageAttachmentDto[];
}

export interface ThreadDto {
  id: string;
  service_id: ServiceId;
  status: ThreadStatus;
  title: string;
  preview_text: string;
  message_count: number;
  created_at: string;
  last_message_at: string | null;
}

export interface ThreadDetailDto extends ThreadDto {
  messages: MessageDto[];
}

export interface ThreadListResponseDto {
  items: ThreadDto[];
  page: number;
  page_size: number;
  total: number;
}

export interface CreateThreadRequestDto {
  service_id: ServiceId;
  text?: string;
  input_type: MessageInputType;
  file_ids?: string[];
}

export interface AddMessageRequestDto {
  text?: string;
  input_type: MessageInputType;
  file_ids?: string[];
}

export type FileProcessingStatus = "pending" | "processed" | "error";

export interface FileAttachmentDto {
  file_id: string;
  url: string;
  original_name: string;
  mime_type: string;
  size_bytes: number;
  processing_status: FileProcessingStatus;
}

export interface AnalyticsOverviewDto {
  total_threads: number;
  status_breakdown: Record<ThreadStatus, number>;
  avg_processing_time_sec?: number;
  satisfaction_rate?: number;
}
