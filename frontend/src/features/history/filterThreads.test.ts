import { describe, expect, it } from "vitest";
import { filterByPeriod } from "./filterThreads";
import type { Thread } from "./types";

// status/search — теперь query-параметры GET /threads (см. useThreadHistory.ts),
// здесь тестируется только period — единственный фильтр, оставшийся на фронте
// (openapi.yaml не поддерживает period на бэке, см. filterThreads.ts).

function threadAt(id: string, updatedAt: string): Thread {
  return {
    id,
    title: id,
    preview: id,
    status: "done",
    updatedAt,
    messageCount: 1,
  };
}

describe("filterByPeriod", () => {
  const now = new Date("2026-09-20T12:00:00.000Z");

  const threads = [
    threadAt("today", "2026-09-20T08:00:00.000Z"),
    threadAt("yesterday", "2026-09-19T23:00:00.000Z"),
    threadAt("3-days-ago", "2026-09-17T12:00:00.000Z"),
    threadAt("10-days-ago", "2026-09-10T12:00:00.000Z"),
    threadAt("25-days-ago", "2026-08-26T12:00:00.000Z"),
    threadAt("40-days-ago", "2026-08-11T12:00:00.000Z"),
  ];

  it("не падает на пустом списке", () => {
    expect(filterByPeriod([], "today", now)).toEqual([]);
  });

  it("'all' не фильтрует по дате", () => {
    expect(filterByPeriod(threads, "all", now)).toHaveLength(threads.length);
  });

  it("'today' — только сегодняшний UTC-день", () => {
    expect(filterByPeriod(threads, "today", now).map((t) => t.id)).toEqual(["today"]);
  });

  it("'yesterday' — только вчерашний UTC-день", () => {
    expect(filterByPeriod(threads, "yesterday", now).map((t) => t.id)).toEqual([
      "yesterday",
    ]);
  });

  it("'7d' — включает сегодня/вчера/3 дня назад, но не 10 дней назад", () => {
    expect(filterByPeriod(threads, "7d", now).map((t) => t.id)).toEqual([
      "today",
      "yesterday",
      "3-days-ago",
    ]);
  });

  it("'30d' — включает всё до 30 дней, но не 40 дней назад", () => {
    expect(filterByPeriod(threads, "30d", now).map((t) => t.id)).toEqual([
      "today",
      "yesterday",
      "3-days-ago",
      "10-days-ago",
      "25-days-ago",
    ]);
  });
});
