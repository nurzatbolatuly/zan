import { describe, expect, it } from "vitest";
import {
  formatDate,
  formatDuration,
  formatFileSize,
  formatTenge,
  pluralRu,
} from "./format";

// ru-RU группирует тысячи неразрывным пробелом (U+00A0) — нормализуем перед сравнением.
function normalizeSpaces(value: string): string {
  return value.replace(/\s/g, " ");
}

describe("formatTenge", () => {
  it("форматирует сумму с разделителями тысяч и знаком тенге", () => {
    expect(normalizeSpaces(formatTenge(2900))).toBe("2 900 ₸");
  });

  it("не ломается на нуле", () => {
    expect(formatTenge(0)).toBe("0 ₸");
  });
});

describe("formatDate", () => {
  it("форматирует ISO-дату как ДД.ММ.ГГГГ", () => {
    expect(formatDate("2026-09-14T10:00:00.000Z")).toBe("14.09.2026");
  });

  it("дополняет день/месяц нулём", () => {
    expect(formatDate("2026-01-05T00:00:00.000Z")).toBe("05.01.2026");
  });
});

describe("formatFileSize", () => {
  it("не ломается на нуле", () => {
    expect(formatFileSize(0)).toBe("0 B");
  });

  it("показывает байты без десятых", () => {
    expect(formatFileSize(500)).toBe("500 B");
  });

  it("показывает килобайты без десятых", () => {
    expect(formatFileSize(253952)).toBe("248 KB");
  });

  it("показывает мегабайты с одной десятой", () => {
    expect(formatFileSize(1572864)).toBe("1.5 MB");
  });
});

describe("formatDuration", () => {
  it("показывает только секунды, если меньше минуты", () => {
    expect(formatDuration(34, "ru")).toBe("34 с");
  });

  it("не ломается на нуле", () => {
    expect(formatDuration(0, "ru")).toBe("0 с");
  });

  it("показывает минуты без секунд, если секунд ровно 0", () => {
    expect(formatDuration(120, "ru")).toBe("2 мин");
  });

  it("показывает минуты и секунды вместе", () => {
    expect(formatDuration(134, "ru")).toBe("2 мин 14 с");
  });

  it("показывает часы с минутами, дополненными нулём", () => {
    expect(formatDuration(3900, "ru")).toBe("1 ч 05 мин");
  });

  it("использует казахские сокращения для kz", () => {
    expect(formatDuration(134, "kz")).toBe("2 мин 14 сек");
  });
});

describe("pluralRu", () => {
  const forms: [string, string, string] = ["запрос", "запроса", "запросов"];

  it.each([
    [1, "запрос"],
    [2, "запроса"],
    [3, "запроса"],
    [5, "запросов"],
    [11, "запросов"],
    [21, "запрос"],
  ])("pluralRu(%i) === %s", (count, expected) => {
    expect(pluralRu(count, forms)).toBe(expected);
  });
});
