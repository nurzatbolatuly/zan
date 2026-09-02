import type { Logger } from './createLogger.js';

/**
 * Оборачивает шаг пайплайна логированием success/failure/duration в едином формате
 * (см. "Логирование" в INSTRUCTIONS.md). Используется ingestion при обработке каждого
 * документа и backend-агентами при обработке каждого шага запроса.
 */
export async function withStepLogging<T>(
  logger: Logger,
  stage: string,
  meta: Record<string, unknown>,
  fn: () => Promise<T>,
): Promise<T> {
  const startedAt = Date.now();
  try {
    const result = await fn();
    logger.info({ stage, status: 'success', duration_ms: Date.now() - startedAt, meta }, stage);
    return result;
  } catch (error) {
    logger.error(
      {
        stage,
        status: 'failure',
        duration_ms: Date.now() - startedAt,
        meta,
        error: error instanceof Error ? error.message : String(error),
      },
      stage,
    );
    throw error;
  }
}
