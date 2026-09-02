/** @type {import('node-pg-migrate').ColumnDefinitions | undefined} */
exports.shorthands = undefined;

/**
 * Этап 13 (бэклог) — многоходовой диалог, ограниченный ОДНИМ раундом уточнения перед поиском
 * (не полноценный чат): оркестратор перед Агентом 1 задаёт вопрос, если исходного запроса
 * пользователя недостаточно, останавливает пайплайн и ждёт ответ (см.
 * backend/src/orchestrator/clarification-check.ts). Реализовано двумя nullable-колонками на
 * requests, а не отдельной таблицей сообщений — раунд ровно один, полноценная история
 * переписки не нужна.
 */
exports.up = (pgm) => {
  pgm.sql(`
    ALTER TYPE request_status ADD VALUE IF NOT EXISTS 'needs_clarification';
    ALTER TABLE requests ADD COLUMN clarification_question TEXT;
    ALTER TABLE requests ADD COLUMN clarification_answer TEXT;
  `);
};

exports.down = (pgm) => {
  // Postgres не поддерживает удаление значения enum без пересоздания типа — для локальной
  // разработки откат этой части не реализован, только колонки.
  pgm.sql(`
    ALTER TABLE requests DROP COLUMN IF EXISTS clarification_answer;
    ALTER TABLE requests DROP COLUMN IF EXISTS clarification_question;
  `);
};
