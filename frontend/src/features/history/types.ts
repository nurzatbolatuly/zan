import type { ThreadStatus } from "@/shared/types/common";

/**
 * Только в этой фиче — как только тред понадобится Chat (Stage 1) для роутинга
 * `/?thread=:id`, переносится в `shared/types` (FRONT_CODING_STANDARDS.md §5).
 */
export interface Thread {
  id: string;
  title: string;
  preview: string;
  status: ThreadStatus;
  /** ISO-строка, форматируется через `shared/lib/format.ts#formatDate`. */
  updatedAt: string;
  /** Время обработки запроса, секунды — `shared/lib/format.ts#formatDuration`. */
  processingTimeSeconds: number;
}

export type HistoryStatusFilter = ThreadStatus | "all";

/** "По дням" (today/yesterday) и "по промежуткам" (7d/30d) — один Select, обе оси сразу. */
export type HistoryPeriodFilter = "all" | "today" | "yesterday" | "7d" | "30d";
