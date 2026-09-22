import { describe, expect, it } from "vitest";
import { filterThreads } from "./filterThreads";
import { THREAD_MOCKS } from "./threads.mocks";

describe("filterThreads", () => {
  it("без запроса и с фильтром 'all' возвращает все треды", () => {
    expect(
      filterThreads({ threads: THREAD_MOCKS, search: "", status: "all" }),
    ).toHaveLength(THREAD_MOCKS.length);
  });

  it("фильтрует по статусу", () => {
    const result = filterThreads({ threads: THREAD_MOCKS, search: "", status: "done" });
    expect(result).toHaveLength(1);
    expect(result[0]?.status).toBe("done");
  });

  it("ищет по заголовку без учёта регистра", () => {
    const result = filterThreads({
      threads: THREAD_MOCKS,
      search: "трудовую книжку",
      status: "all",
    });
    expect(result).toHaveLength(1);
    expect(result[0]?.id).toBe("thread-done-1");
  });

  it("ищет и по превью, не только по заголовку", () => {
    const result = filterThreads({
      threads: THREAD_MOCKS,
      search: "оплата возвращена",
      status: "all",
    });
    expect(result).toHaveLength(1);
    expect(result[0]?.id).toBe("thread-error-1");
  });

  it("применяет статус и поиск одновременно (AND, не OR)", () => {
    const result = filterThreads({
      threads: THREAD_MOCKS,
      search: "договора",
      status: "error",
    });
    expect(result).toHaveLength(0);
  });

  it("не падает на пустом списке", () => {
    expect(filterThreads({ threads: [], search: "что угодно", status: "all" })).toEqual(
      [],
    );
  });
});

describe("filterThreads — period", () => {
  const now = new Date("2026-09-20T12:00:00.000Z");

  function threadAt(id: string, updatedAt: string) {
    return {
      id,
      title: id,
      preview: id,
      status: "done" as const,
      updatedAt,
      processingTimeSeconds: 0,
    };
  }

  const threads = [
    threadAt("today", "2026-09-20T08:00:00.000Z"),
    threadAt("yesterday", "2026-09-19T23:00:00.000Z"),
    threadAt("3-days-ago", "2026-09-17T12:00:00.000Z"),
    threadAt("10-days-ago", "2026-09-10T12:00:00.000Z"),
    threadAt("25-days-ago", "2026-08-26T12:00:00.000Z"),
    threadAt("40-days-ago", "2026-08-11T12:00:00.000Z"),
  ];

  it("period 'all' не фильтрует по дате", () => {
    expect(
      filterThreads({ threads, search: "", status: "all", period: "all", now }),
    ).toHaveLength(threads.length);
  });

  it("period 'today' — только сегодняшний UTC-день", () => {
    const result = filterThreads({
      threads,
      search: "",
      status: "all",
      period: "today",
      now,
    });
    expect(result.map((t) => t.id)).toEqual(["today"]);
  });

  it("period 'yesterday' — только вчерашний UTC-день", () => {
    const result = filterThreads({
      threads,
      search: "",
      status: "all",
      period: "yesterday",
      now,
    });
    expect(result.map((t) => t.id)).toEqual(["yesterday"]);
  });

  it("period '7d' — включает сегодня/вчера/3 дня назад, но не 10 дней назад", () => {
    const result = filterThreads({
      threads,
      search: "",
      status: "all",
      period: "7d",
      now,
    });
    expect(result.map((t) => t.id)).toEqual(["today", "yesterday", "3-days-ago"]);
  });

  it("period '30d' — включает всё до 30 дней, но не 40 дней назад", () => {
    const result = filterThreads({
      threads,
      search: "",
      status: "all",
      period: "30d",
      now,
    });
    expect(result.map((t) => t.id)).toEqual([
      "today",
      "yesterday",
      "3-days-ago",
      "10-days-ago",
      "25-days-ago",
    ]);
  });

  it("период и статус применяются одновременно (AND)", () => {
    const mixedStatus = [
      { ...threads[0]!, status: "done" as const },
      { ...threads[1]!, status: "error" as const },
    ];
    const result = filterThreads({
      threads: mixedStatus,
      search: "",
      status: "done",
      period: "7d",
      now,
    });
    expect(result.map((t) => t.id)).toEqual(["today"]);
  });
});
