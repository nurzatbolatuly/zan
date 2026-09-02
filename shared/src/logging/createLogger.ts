import pino from 'pino';

/**
 * Единый структурированный логгер для всех сервисов (ingestion, backend).
 * Формат полей фиксирован в INSTRUCTIONS.md — меняется только здесь (DRY).
 */
export interface Logger {
  info(meta: Record<string, unknown>, message: string): void;
  warn(meta: Record<string, unknown>, message: string): void;
  error(meta: Record<string, unknown>, message: string): void;
  child(bindings: Record<string, unknown>): Logger;
}

export function createLogger(service: string): Logger {
  const level = process.env.LOG_LEVEL ?? 'info';
  return pino({
    level,
    base: { service },
    timestamp: pino.stdTimeFunctions.isoTime,
  });
}
