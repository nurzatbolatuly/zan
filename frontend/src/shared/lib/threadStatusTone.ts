import type { ThreadStatus } from "@/shared/types/common";
import type { BadgeProps } from "@/shared/ui";

/**
 * Цвет статуса треда — 1:1 STATUS_COLOR из прототипа (Zan.dc.html:804).
 * Изначально жил только в `features/history` (Stage 2); перенесено сюда,
 * когда Settings→Analytics (Stage 4c, PLAN.md §5) стал вторым потребителем
 * той же раскраски статусов — для полос "по статусам" (FRONT_CODING_STANDARDS.md §5,
 * "переезжает в shared, когда нужен второй фиче").
 *
 * Тонов у Badge всего 4 (accent/warn/danger/muted) на 6 статусов — совпадает
 * с прототипом, не ошибка маппинга.
 */
export const THREAD_STATUS_TONE: Record<ThreadStatus, NonNullable<BadgeProps["tone"]>> = {
  queued: "muted",
  processing: "accent",
  clarify: "warn",
  done: "accent",
  error: "danger",
  canceled: "muted",
};

// Классы должны быть статичными строками, чтобы Tailwind их увидел при сканировании —
// не собирать "bg-" + tone в шаблонной строке (то же правило, что в shared/ui/Modal.tsx).
const TONE_BAR_CLASS: Record<NonNullable<BadgeProps["tone"]>, string> = {
  accent: "bg-accent",
  warn: "bg-warn",
  danger: "bg-danger",
  muted: "bg-muted",
};

/** Цвет полосы статус-бара в аналитике (Settings→Analytics) — тот же тон, что у бейджа статуса. */
export const THREAD_STATUS_BAR_CLASS: Record<ThreadStatus, string> = Object.fromEntries(
  Object.entries(THREAD_STATUS_TONE).map(([status, tone]) => [
    status,
    TONE_BAR_CLASS[tone],
  ]),
) as Record<ThreadStatus, string>;
