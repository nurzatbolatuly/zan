import type { HistoryPeriodFilter, Thread } from "./types";

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
 * Stage 6: `status`/`search` ушли на бэк как query-параметры `GET /threads`
 * (см. `useThreadHistory.ts`) — здесь остался только период, у него нет
 * серверного эквивалента (openapi.yaml поддерживает только `status`/
 * `search`/`page`). Фильтрует то, что уже загружено на экран (текущую
 * страницу), не весь список тредов сессии — задокументированное
 * ограничение, см. instructions.md, расхождение №6 плана интеграции.
 * Чистая функция — тестируется отдельно от React (FRONT_CODING_STANDARDS.md §10).
 */
export function filterByPeriod(
  threads: Thread[],
  period: HistoryPeriodFilter,
  now: Date = new Date(),
): Thread[] {
  if (period === "all") return threads;
  return threads.filter((thread) => matchesPeriod(thread.updatedAt, period, now));
}
