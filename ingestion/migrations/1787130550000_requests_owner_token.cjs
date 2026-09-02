/** @type {import('node-pg-migrate').ColumnDefinitions | undefined} */
exports.shorthands = undefined;

/**
 * Этап 8 (безопасность) — "пользователь видит только свои запросы". Полноценных аккаунтов
 * пока нет (см. Этап 13 бэклог — личный кабинет), поэтому используем анонимную сессию:
 * backend выдаёт браузеру httpOnly-cookie со случайным токеном и хранит только SHA-256 хэш
 * этого токена на записи запроса — сам токен нигде на сервере не хранится и не логируется,
 * сравнение выполняется по хэшу. NULL допустим для случая, если запрос создан без cookie
 * (клиент отключил cookies) — такой запрос не будет доступен через GET по дизайну.
 */
exports.up = (pgm) => {
  pgm.sql(`
    ALTER TABLE requests ADD COLUMN owner_token_hash TEXT;
    CREATE INDEX requests_owner_token_hash_idx ON requests (id, owner_token_hash);
  `);
};

exports.down = (pgm) => {
  pgm.sql(`
    DROP INDEX IF EXISTS requests_owner_token_hash_idx;
    ALTER TABLE requests DROP COLUMN IF EXISTS owner_token_hash;
  `);
};
