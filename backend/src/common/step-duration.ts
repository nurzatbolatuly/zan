export interface StepTimestamps {
  started_at: string | null;
  finished_at: string | null;
}

/**
 * Длительность работы пайплайна по границам шагов: от старта самого раннего шага до финиша
 * самого позднего — не created_at→updated_at запроса, которое включало бы и время ожидания
 * ответа пользователя на уточнение (needs_clarification тоже трогает updated_at, см.
 * orchestrator.service.ts). Используется и в аналитике (среднее по всем завершённым запросам,
 * см. analytics.service.ts), и в истории обращений (за один конкретный запрос, см.
 * requests.repository.ts) — единая формула, не дублируется по месту использования.
 *
 * null, если ни один шаг ещё не имеет обеих меток (пайплайн не завершил ни один шаг) — типично
 * для запроса, который ещё обрабатывается или упал на самом первом шаге.
 */
export function computeStepsDurationMs(steps: StepTimestamps[]): number | null {
  const startTimes = steps
    .map((s) => s.started_at)
    .filter((v): v is string => v !== null)
    .map((v) => new Date(v).getTime());
  const finishTimes = steps
    .map((s) => s.finished_at)
    .filter((v): v is string => v !== null)
    .map((v) => new Date(v).getTime());
  if (startTimes.length === 0 || finishTimes.length === 0) return null;
  return Math.max(...finishTimes) - Math.min(...startTimes);
}
