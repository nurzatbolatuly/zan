-- Полная очистка базы перед повторным накатом миграций с нуля.
--
-- Зачем: 2026-09-13 из ingestion/migrations/ убраны law_corpus.cjs/law_chunks_fulltext.cjs
-- (мёртвая retrieval-инфраструктура — код, который её использовал, был удалён ещё 2026-08-28,
-- но сами таблицы/расширение/функция в БД оставались). node-pg-migrate трекает применённые
-- миграции по имени файла в таблице pgmigrations — раз файлы удалены из репозитория, продолжать
-- обычный `npm run migrate` на базе, где они уже были применены, нельзя (история разъедется).
-- Проще всего для прототипа: снести всё руками этим скриптом и накатить актуальный набор
-- миграций (ingestion/migrations/*.cjs или ingestion/supabase-migrations.sql) с нуля.
--
-- Запуск: Supabase Dashboard → SQL Editor → New query → вставить целиком → Run.
-- Для локального docker-compose postgres — psql "$DATABASE_URL" -f ingestion/reset-database.sql
--
-- ВНИМАНИЕ: необратимо удаляет ВСЕ данные приложения (requests/request_steps/documents) и
-- оставшиеся объекты retrieval-слоя, если он ещё физически существует в этой базе.

DROP FUNCTION IF EXISTS match_law_chunks(VECTOR(1536), TEXT, INTEGER, FLOAT);
DROP FUNCTION IF EXISTS match_law_chunks(VECTOR(1536), INTEGER, FLOAT);

DROP TABLE IF EXISTS law_chunks CASCADE;
DROP TABLE IF EXISTS law_articles CASCADE;
DROP TABLE IF EXISTS law_documents CASCADE;

DROP TABLE IF EXISTS documents CASCADE;
DROP TABLE IF EXISTS request_steps CASCADE;
DROP TABLE IF EXISTS requests CASCADE;

DROP TYPE IF EXISTS request_status;
DROP TYPE IF EXISTS request_step_status;

-- pgvector extension: снимается последней, только если её больше никто не использует
-- (безопасно — она заводилась исключительно под law_chunks.embedding выше).
DROP EXTENSION IF EXISTS vector;

-- node-pg-migrate хранит здесь список уже применённых миграций — без сброса он решит, что
-- requests.cjs..request_cancelled.cjs уже накатаны, и не выполнит их заново после этого скрипта.
DROP TABLE IF EXISTS pgmigrations CASCADE;
