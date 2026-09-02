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
-- Версия: 1787130553000 — law_corpus.cjs
-- Retrieval-слой (локальный корпус законодательства РК) — см. README §"Retrieval-слой".
-- ============================================================================
CREATE EXTENSION IF NOT EXISTS vector;

CREATE TABLE IF NOT EXISTS law_documents (
  id TEXT PRIMARY KEY,
  title TEXT NOT NULL,
  url TEXT NOT NULL,
  is_active BOOLEAN NOT NULL DEFAULT true,
  status_note TEXT,
  content_hash TEXT,
  fetched_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS law_articles (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  document_id TEXT NOT NULL REFERENCES law_documents(id) ON DELETE CASCADE,
  article_number TEXT NOT NULL,
  anchor TEXT,
  heading TEXT,
  is_active BOOLEAN NOT NULL DEFAULT true,
  last_amended_at DATE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (document_id, article_number)
);

CREATE TABLE IF NOT EXISTS law_chunks (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  article_id UUID NOT NULL REFERENCES law_articles(id) ON DELETE CASCADE,
  document_id TEXT NOT NULL REFERENCES law_documents(id) ON DELETE CASCADE,
  chunk_index INTEGER NOT NULL,
  content TEXT NOT NULL,
  -- text-embedding-3-small, см. shared/src/llm/embeddingClient.ts — 1536 измерений.
  embedding VECTOR(1536) NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (article_id, chunk_index)
);

CREATE INDEX IF NOT EXISTS law_chunks_document_id_idx ON law_chunks (document_id);
CREATE INDEX IF NOT EXISTS law_articles_document_id_idx ON law_articles (document_id);
CREATE INDEX IF NOT EXISTS law_chunks_embedding_idx ON law_chunks
  USING hnsw (embedding vector_cosine_ops);

CREATE OR REPLACE FUNCTION match_law_chunks(
  query_embedding VECTOR(1536),
  match_count INTEGER DEFAULT 8,
  min_similarity FLOAT DEFAULT 0.3
)
RETURNS TABLE (
  chunk_id UUID,
  article_id UUID,
  document_id TEXT,
  document_title TEXT,
  article_number TEXT,
  heading TEXT,
  content TEXT,
  similarity FLOAT
)
LANGUAGE sql STABLE AS $$
  SELECT
    c.id AS chunk_id,
    c.article_id,
    c.document_id,
    d.title AS document_title,
    a.article_number,
    a.heading,
    c.content,
    1 - (c.embedding <=> query_embedding) AS similarity
  FROM law_chunks c
  JOIN law_articles a ON a.id = c.article_id
  JOIN law_documents d ON d.id = c.document_id
  WHERE d.is_active AND a.is_active
    AND 1 - (c.embedding <=> query_embedding) >= min_similarity
  ORDER BY c.embedding <=> query_embedding
  LIMIT match_count;
$$;

-- ============================================================================
-- Версия: 1787130554000 — law_chunks_fulltext.cjs
-- Гибридный поиск (вектор + полнотекст) — см. README §"Retrieval-слой".
-- ============================================================================
ALTER TABLE law_chunks ADD COLUMN IF NOT EXISTS content_tsv tsvector
  GENERATED ALWAYS AS (to_tsvector('russian', content)) STORED;

CREATE INDEX IF NOT EXISTS law_chunks_content_tsv_idx ON law_chunks USING gin (content_tsv);

-- CREATE OR REPLACE не заменяет функцию с другой сигнатурой параметров (добавлен query_text) —
-- явный DROP убирает 3-аргументную версию из предыдущего блока миграции выше, иначе она осталась
-- бы висеть в базе как отдельная неиспользуемая перегрузка.
DROP FUNCTION IF EXISTS match_law_chunks(VECTOR(1536), INTEGER, FLOAT);

CREATE OR REPLACE FUNCTION match_law_chunks(
  query_embedding VECTOR(1536),
  query_text TEXT,
  match_count INTEGER DEFAULT 8,
  min_similarity FLOAT DEFAULT 0.3
)
RETURNS TABLE (
  chunk_id UUID,
  article_id UUID,
  document_id TEXT,
  document_title TEXT,
  article_number TEXT,
  heading TEXT,
  content TEXT,
  similarity FLOAT
)
LANGUAGE sql STABLE AS $$
  WITH vector_matches AS (
    SELECT c.id, 1 - (c.embedding <=> query_embedding) AS vector_score
    FROM law_chunks c
    ORDER BY c.embedding <=> query_embedding
    LIMIT match_count * 3
  ),
  keyword_matches AS (
    SELECT c.id, ts_rank(c.content_tsv, websearch_to_tsquery('russian', query_text)) AS keyword_score
    FROM law_chunks c
    WHERE c.content_tsv @@ websearch_to_tsquery('russian', query_text)
    ORDER BY keyword_score DESC
    LIMIT match_count * 3
  ),
  combined AS (
    SELECT
      COALESCE(v.id, k.id) AS chunk_id,
      COALESCE(v.vector_score, 0) AS vector_score,
      COALESCE(k.keyword_score, 0) AS keyword_score
    FROM vector_matches v
    FULL OUTER JOIN keyword_matches k ON v.id = k.id
  )
  SELECT
    c.id AS chunk_id,
    c.article_id,
    c.document_id,
    d.title AS document_title,
    a.article_number,
    a.heading,
    c.content,
    combined.vector_score AS similarity
  FROM combined
  JOIN law_chunks c ON c.id = combined.chunk_id
  JOIN law_articles a ON a.id = c.article_id
  JOIN law_documents d ON d.id = c.document_id
  WHERE d.is_active AND a.is_active
    AND (combined.vector_score >= min_similarity OR combined.keyword_score > 0)
  ORDER BY (combined.vector_score * 0.7 + LEAST(combined.keyword_score, 1) * 0.3) DESC
  LIMIT match_count;
$$;

-- ============================================================================
-- Следующая миграция — добавляй новый блок "-- Версия: ..." ниже этой строки (в идемпотентной
-- форме, см. правила в шапке файла).
-- ============================================================================
