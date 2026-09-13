-- Копия миграций для ручного запуска в Supabase SQL Editor (Dashboard → SQL Editor → New query).
--
-- Источник правды — версионированные миграции в ingestion/migrations/*.cjs (node-pg-migrate);
-- этот файл — их ручная копия для вставки в Supabase, потому что DATABASE_URL в .env указывает
-- прямо на прод Supabase Postgres (db.<project>.supabase.co), а не на локальный docker-compose
-- postgres — см. INSTRUCTIONS.md §7.1.
--
-- ИДЕМПОТЕНТНОСТЬ — обязательное свойство этого файла (в отличие от ingestion/migrations/*.cjs,
-- которые накатываются node-pg-migrate ровно один раз и трекаются в pgmigrations): весь файл
-- целиком безопасно выполнять повторно на базе, где часть объектов уже существует (типичный
-- случай — часть таблиц уже создана руками раньше). Поэтому:
--   * CREATE TABLE  → CREATE TABLE IF NOT EXISTS
--   * CREATE INDEX  → CREATE INDEX IF NOT EXISTS
--   * ALTER TABLE ... ADD COLUMN → ADD COLUMN IF NOT EXISTS
--   * CREATE TYPE (enum) → обёрнут в DO $$ ... EXCEPTION WHEN duplicate_object THEN NULL; END $$;
--     (Postgres не поддерживает "CREATE TYPE IF NOT EXISTS" напрямую)
--   * CREATE FUNCTION → CREATE OR REPLACE FUNCTION
--
-- ПРАВИЛО (см. INSTRUCTIONS.md §7.1): при добавлении новой миграции в ingestion/migrations/ —
-- добавляй её SQL (только up, down не нужен для этого файла) СЮДА ЖЕ, новым блоком снизу, с
-- комментарием-заголовком "-- Версия: <timestamp из имени файла> — <имя файла>", и в ТОЙ ЖЕ
-- идемпотентной форме, что и здесь (см. правила выше) — иначе следующий полный прогон файла
-- сломается на объектах, созданных предыдущим блоком.

-- ============================================================================
-- Версия: 1787130547000 — requests.cjs
-- ============================================================================
DO $$ BEGIN
  CREATE TYPE request_status AS ENUM ('pending', 'processing', 'completed', 'failed');
EXCEPTION
  WHEN duplicate_object THEN NULL;
END $$;

DO $$ BEGIN
  CREATE TYPE request_step_status AS ENUM ('pending', 'running', 'success', 'failed');
EXCEPTION
  WHEN duplicate_object THEN NULL;
END $$;

CREATE TABLE IF NOT EXISTS requests (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  status request_status NOT NULL DEFAULT 'pending',
  query_text TEXT NOT NULL,
  include_document BOOLEAN NOT NULL DEFAULT false,
  document_type TEXT,
  result_summary TEXT,
  error_message TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS request_steps (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  request_id UUID NOT NULL REFERENCES requests(id) ON DELETE CASCADE,
  agent_name TEXT NOT NULL,
  ordinal INTEGER NOT NULL,
  status request_step_status NOT NULL DEFAULT 'pending',
  input JSONB,
  output JSONB,
  error_message TEXT,
  started_at TIMESTAMPTZ,
  finished_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (request_id, ordinal)
);

CREATE TABLE IF NOT EXISTS documents (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  request_id UUID NOT NULL REFERENCES requests(id) ON DELETE CASCADE,
  document_type TEXT NOT NULL,
  file_format TEXT NOT NULL,
  content TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS request_steps_request_id_idx ON request_steps (request_id);
CREATE INDEX IF NOT EXISTS documents_request_id_idx ON documents (request_id);

-- ============================================================================
-- Версия: 1787130549000 — documents_title.cjs
-- ============================================================================
ALTER TABLE documents ADD COLUMN IF NOT EXISTS title TEXT NOT NULL DEFAULT '';
ALTER TABLE documents ALTER COLUMN title DROP DEFAULT;

-- ============================================================================
-- Версия: 1787130550000 — requests_owner_token.cjs
-- ============================================================================
ALTER TABLE requests ADD COLUMN IF NOT EXISTS owner_token_hash TEXT;
CREATE INDEX IF NOT EXISTS requests_owner_token_hash_idx ON requests (id, owner_token_hash);

-- ============================================================================
-- Версия: 1787130551000 — request_clarification.cjs
-- ============================================================================
ALTER TYPE request_status ADD VALUE IF NOT EXISTS 'needs_clarification';
ALTER TABLE requests ADD COLUMN IF NOT EXISTS clarification_question TEXT;
ALTER TABLE requests ADD COLUMN IF NOT EXISTS clarification_answer TEXT;

-- ============================================================================
-- Версия: 1787130552000 — request_cancelled.cjs
-- ============================================================================
ALTER TYPE request_status ADD VALUE IF NOT EXISTS 'cancelled';

-- ============================================================================
-- Следующая миграция — добавляй новый блок "-- Версия: ..." ниже этой строки (в идемпотентной
-- форме, см. правила в шапке файла).
-- ============================================================================
