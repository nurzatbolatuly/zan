/** @type {import('node-pg-migrate').ColumnDefinitions | undefined} */
exports.shorthands = undefined;

/**
 * Этап 6 (Агент 4) генерирует заголовок документа (documentOutput.title), но оркестратор
 * его нигде не сохранял — persisted была только исходная строка documentType от пользователя.
 * Этап 7 (frontend) показывает и скачивает документ по осмысленному названию, поэтому title
 * нужен в БД.
 */
exports.up = (pgm) => {
  pgm.sql(`
    ALTER TABLE documents ADD COLUMN title TEXT NOT NULL DEFAULT '';
    ALTER TABLE documents ALTER COLUMN title DROP DEFAULT;
  `);
};

exports.down = (pgm) => {
  pgm.sql(`
    ALTER TABLE documents DROP COLUMN IF EXISTS title;
  `);
};
