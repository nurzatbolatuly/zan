/**
 * Один traceId на пользовательское действие, передаётся сквозь весь запрос
 * и уходит в заголовок X-Trace-Id к бэку (backend-roadmap.md:44).
 * См. FRONT_CODING_STANDARDS.md §4.4 — не создавать новый id на каждый
 * внутренний вызов одной операции, генерировать один раз в точке входа.
 */
export function createTraceId(): string {
  if (typeof crypto !== "undefined" && "randomUUID" in crypto) {
    return crypto.randomUUID();
  }
  return `${Date.now()}-${Math.random().toString(16).slice(2)}`;
}
