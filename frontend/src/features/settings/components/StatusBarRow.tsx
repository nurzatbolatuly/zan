import { cn } from "@/shared/lib/cn";
import { THREAD_STATUS_BAR_CLASS } from "@/shared/lib/threadStatusTone";
import type { ThreadStatus } from "@/shared/types/common";

interface StatusBarRowProps {
  status: ThreadStatus;
  label: string;
  count: number;
  /** Доля от максимального count среди всех строк — ширина полосы. */
  ratio: number;
}

/** Одна строка "по статусам" в Settings → Аналитика (PLAN.md §5 Stage 4c). */
export function StatusBarRow({ status, label, count, ratio }: StatusBarRowProps) {
  const barClass = THREAD_STATUS_BAR_CLASS[status];
  return (
    <div className="flex items-center gap-2.5">
      <span
        className={cn("h-2 w-2 flex-none rounded-full", barClass)}
        aria-hidden="true"
      />
      <span className="w-32 flex-none text-body-sm text-ink">{label}</span>
      <div className="h-2 flex-1 overflow-hidden rounded-sm bg-surface-2">
        <div
          className={cn("h-full rounded-sm", barClass)}
          style={{ width: `${Math.round(ratio * 100)}%` }}
        />
      </div>
      <span className="w-8 flex-none text-right text-body-sm font-semibold text-ink">
        {count}
      </span>
    </div>
  );
}
