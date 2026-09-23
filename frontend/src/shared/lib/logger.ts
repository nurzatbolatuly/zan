/**
 * Единственная точка логирования во всём приложении (FRONT_CODING_STANDARDS.md §4).
 * Нигде за пределами этого файла не должно быть сырого console.* —
 * правило включено в eslint.config.js как "no-console": "error".
 *
 * dev:  все уровни идут в консоль.
 * prod: debug отбрасывается; info не отправляется, но копится в кольцевом
 *       буфере и прикладывается как контекст к следующему warn/error —
 *       так по одной ошибке видно, что происходило непосредственно перед ней;
 *       warn/error всегда уходят в консоль и на /logs/client.
 */

import { getApiBaseUrlSafe } from "./env";
import { useSessionStore } from "@/shared/stores/useSessionStore";

type LogLevel = "debug" | "info" | "warn" | "error";

export interface LogPayload {
  /** Откуда событие: "chat.composer", "tariffs.payment", "history.delete"... */
  scope: string;
  /** Что произошло: "message_send_started", "payment_failed"... */
  event: string;
  data?: Record<string, unknown>;
  /** Тот же id, что уходит в X-Trace-Id к бэку — см. traceId.ts */
  traceId?: string;
}

interface LogRecord extends LogPayload {
  level: LogLevel;
  ts: string;
}

const RING_BUFFER_SIZE = 20;
const recentInfo: LogRecord[] = [];

const isDev = import.meta.env.DEV;
const verbose = import.meta.env.VITE_ENABLE_VERBOSE_LOGS === "true";

function toRecord(level: LogLevel, payload: LogPayload): LogRecord {
  return { level, ts: new Date().toISOString(), ...payload };
}

function pushToRingBuffer(record: LogRecord): void {
  recentInfo.push(record);
  if (recentInfo.length > RING_BUFFER_SIZE) recentInfo.shift();
}

function printToConsole(record: LogRecord): void {
  const label = `[${record.scope}] ${record.event}`;
  const details = { traceId: record.traceId, ...record.data };
  // no-console выключен для этого файла целиком в eslint.config.js — единственное разрешённое место.
  console[record.level](label, details);
}

function serializeError(error: unknown): Record<string, unknown> {
  if (error instanceof Error) {
    return { name: error.name, message: error.message, stack: error.stack };
  }
  return { value: error };
}

/** POST /logs/client требует "message" (не "scope"/"data") и LEVEL заглавными
 * буквами (openapi.yaml#ClientLogRequest) — record.level/scope/data остаются
 * внутренним форматом фронта, здесь только маппинг на контракт бэка. */
function toClientLogRequest(record: LogRecord) {
  return {
    session_id: useSessionStore.getState().sessionId ?? undefined,
    trace_id: record.traceId,
    level: record.level.toUpperCase() as "WARN" | "ERROR",
    event: record.event,
    message: `[${record.scope}] ${record.event}`,
    context: { ...record.data, recent: recentInfo.slice() },
  };
}

function sendToServer(record: LogRecord): void {
  const apiBaseUrl = getApiBaseUrlSafe();
  // логирование — best-effort и никогда не бросает: нет URL (например, забыли
  // задать env на хостинге) — просто не отправляем, запись уже ушла в консоль выше.
  if (typeof fetch === "undefined" || !apiBaseUrl) return;
  const body = JSON.stringify(toClientLogRequest(record));
  const url = `${apiBaseUrl}/logs/client`;
  // keepalive — чтобы запрос не оборвался, если ошибка произошла перед уходом со страницы
  void fetch(url, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body,
    keepalive: true,
  }).catch(() => {
    // намеренно без логирования: логирование ошибки логирования — риск цикла
  });
}

export const logger = {
  debug(payload: LogPayload): void {
    if (!isDev && !verbose) return;
    printToConsole(toRecord("debug", payload));
  },

  info(payload: LogPayload): void {
    const record = toRecord("info", payload);
    if (isDev) printToConsole(record);
    pushToRingBuffer(record);
  },

  warn(payload: LogPayload): void {
    const record = toRecord("warn", payload);
    printToConsole(record);
    if (!isDev) sendToServer(record);
  },

  error(payload: LogPayload & { error: unknown }): void {
    const { error, ...rest } = payload;
    const record = toRecord("error", {
      ...rest,
      data: { ...rest.data, error: serializeError(error) },
    });
    printToConsole(record);
    if (!isDev) sendToServer(record);
  },
};
