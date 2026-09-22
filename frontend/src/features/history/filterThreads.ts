import type { HistoryPeriodFilter, HistoryStatusFilter, Thread } from "./types";

interface FilterParams {
  threads: Thread[];
  search: string;
  status: HistoryStatusFilter;
  period?: HistoryPeriodFilter;
  /** Точка отсчёта для period-фильтра — параметр ради детерминированных тестов, не `new Date()` внутри. */
  now?: Date;
}

const DAY_MS = 24 * 60 * 60 * 1000;

// UTC, не локальная дата браузера — тот же принцип, что у formatDate
// (shared/lib/format.ts): "сегодня"/"вчера" не должны "плыть" по часовому поясу.
function utcDateKey(date: Date): string {
  return date.toISOString().slice(0, 10);
}

function matchesPeriod(
  updatedAtIso: string,
  period: HistoryPeriodFilter,
  now: Date,
): boolean {
  if (period === "all") return true;

  const updatedAt = new Date(updatedAtIso);

  if (period === "today") return utcDateKey(updatedAt) === utcDateKey(now);
  if (period === "yesterday") {
    return utcDateKey(updatedAt) === utcDateKey(new Date(now.getTime() - DAY_MS));
  }

  const elapsedMs = now.getTime() - updatedAt.getTime();
  const windowDays = period === "7d" ? 7 : 30;
  return elapsedMs >= 0 && elapsedMs <= windowDays * DAY_MS;
}

/**
 * Чистая функция — вынесена из хука, чтобы тестировать бизнес-логику
 * фильтра отдельно от React (FRONT_CODING_STANDARDS.md §10).
 */
export function filterThreads({
  threads,
  search,
  status,
  period = "all",
  now = new Date(),
}: FilterParams): Thread[] {
  const query = search.trim().toLowerCase();

  return threads.filter((thread) => {
    if (status !== "all" && thread.status !== status) return false;
    if (!matchesPeriod(thread.updatedAt, period, now)) return false;
    if (!query) return true;
    return (
      thread.title.toLowerCase().includes(query) ||
      thread.preview.toLowerCase().includes(query)
    );
  });
}
