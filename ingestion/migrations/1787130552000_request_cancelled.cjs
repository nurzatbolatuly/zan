/** @type {import('node-pg-migrate').ColumnDefinitions | undefined} */
exports.shorthands = undefined;

/**
 * Soft-cancel: пользователь может отменить свой запрос, пока он ещё в 'pending'/'processing'/
 * 'needs_clarification'. Отдельной колонки не заводим — 'cancelled' просто ещё одно терминальное
 * значение request_status, оркестратор проверяет его между шагами пайплайна (см.
 * orchestrator.service.ts) и останавливается, не перезаписывая статус.
 */
exports.up = (pgm) => {
  pgm.sql(`ALTER TYPE request_status ADD VALUE IF NOT EXISTS 'cancelled';`);
};

exports.down = () => {
  // Postgres не поддерживает удаление значения enum без пересоздания типа — не реализовано,
  // как и для предыдущего добавленного значения (см. 1787130551000_request_clarification.cjs).
};
