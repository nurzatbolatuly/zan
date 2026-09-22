import type { Lang } from "@/shared/types/common";

/** Формат цены как в прототипе (Zan.dc.html: `toLocaleString("ru-RU") + " ₸"`). */
export function formatTenge(amount: number): string {
  return `${amount.toLocaleString("ru-RU")} ₸`;
}

/**
 * Формат даты `ДД.ММ.ГГГГ` — как в прототипе (Zan.dc.html:735-740), одинаково
 * для ru/kz (сами числа языка не меняют, только строки вокруг них), поэтому
 * без Intl/локали — общая утилита для History/Chat/Tariffs.
 *
 * Читает поля в UTC (не `getDate()`/локальный часовой пояс): бэк отдаёт дату
 * события, а не момент, который должен "плыть" по часовому поясу браузера —
 * иначе один и тот же тред показывал бы разную дату в разных таймзонах.
 */
export function formatDate(iso: string): string {
  const date = new Date(iso);
  const day = String(date.getUTCDate()).padStart(2, "0");
  const month = String(date.getUTCMonth() + 1).padStart(2, "0");
  return `${day}.${month}.${date.getUTCFullYear()}`;
}

/**
 * Склонение количественных существительных RU: 1 запрос / 2 запроса / 5 запросов.
 * Перенесено из Zan.dc.html:982-987 (там же было привязано к конкретному компоненту —
 * здесь общая утилита, чтобы Chat/Tariffs/History не реализовывали её каждый по-своему).
 */
export function pluralRu(
  count: number,
  forms: readonly [string, string, string],
): string {
  const mod10 = count % 10;
  const mod100 = count % 100;
  if (mod10 === 1 && mod100 !== 11) return forms[0];
  if (mod10 >= 2 && mod10 <= 4 && (mod100 < 12 || mod100 > 14)) return forms[1];
  return forms[2];
}

const FILE_SIZE_UNITS = ["B", "KB", "MB", "GB"] as const;
// KB/B без десятых — как в прототипе ("248 KB", Zan.dc.html:120), MB/GB с одной десятой.
const FILE_SIZE_DECIMALS: Record<(typeof FILE_SIZE_UNITS)[number], number> = {
  B: 0,
  KB: 0,
  MB: 1,
  GB: 1,
};

/** Читаемый размер файла для карточки вложения (Chat/History — везде, где есть файлы). */
export function formatFileSize(bytes: number): string {
  if (bytes <= 0) return "0 B";
  const exponent = Math.min(
    Math.floor(Math.log(bytes) / Math.log(1024)),
    FILE_SIZE_UNITS.length - 1,
  );
  // exponent зажат в границах FILE_SIZE_UNITS выше — индекс всегда валиден.
  const unit = FILE_SIZE_UNITS[exponent]!;
  const value = bytes / 1024 ** exponent;
  return `${value.toFixed(FILE_SIZE_DECIMALS[unit])} ${unit}`;
}

interface DurationUnits {
  hour: string;
  minute: string;
  second: string;
}

// Сокращения, не полные слова со склонением — тот же компактный стиль,
// что у остальных meta-полей карточки треда (дата, статус-бейдж).
const DURATION_UNITS: Record<Lang, DurationUnits> = {
  ru: { hour: "ч", minute: "мин", second: "с" },
  kz: { hour: "сағ", minute: "мин", second: "сек" },
};

/** Время обработки запроса (History/Chat) — "5 мин 12 с", "1 ч 05 мин", "34 с". */
export function formatDuration(totalSeconds: number, lang: Lang): string {
  const units = DURATION_UNITS[lang];
  const hours = Math.floor(totalSeconds / 3600);
  const minutes = Math.floor((totalSeconds % 3600) / 60);
  const seconds = totalSeconds % 60;

  if (hours > 0) {
    return `${hours} ${units.hour} ${String(minutes).padStart(2, "0")} ${units.minute}`;
  }
  if (minutes > 0) {
    return seconds > 0
      ? `${minutes} ${units.minute} ${seconds} ${units.second}`
      : `${minutes} ${units.minute}`;
  }
  return `${seconds} ${units.second}`;
}
