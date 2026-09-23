import type { ThreadStatus } from "@/shared/types/common";

/**
 * Только в этой фиче — Chat (Stage 6) читает тред целиком через
 * `ThreadDetailDto` (shared/types/api.ts), не через этот укороченный
 * список-тип (список и деталь треда — разные формы ответа бэка, не одна
 * сущность на двух экранах).
 */
export interface Thread {
  id: string;
  title: string;
  preview: string;
  status: ThreadStatus;
  /** ISO-строка (`last_message_at ?? created_at`), форматируется через `shared/lib/format.ts#formatDate`. */
  updatedAt: string;
  messageCount: number;
}

export type HistoryStatusFilter = ThreadStatus | "all";

/** "По дням" (today/yesterday) и "по промежуткам" (7d/30d) — один Select, обе оси сразу. */
export type HistoryPeriodFilter = "all" | "today" | "yesterday" | "7d" | "30d";
