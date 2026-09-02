/** @type {import('node-pg-migrate').ColumnDefinitions | undefined} */
exports.shorthands = undefined;

/**
 * Схема локального корпуса законодательства РК для retrieval-слоя (см. корневой README —
 * пересмотр решения из PLAN.md §2: вместо live web_search на каждый запрос агенты читают из
 * заранее проиндексированного и периодически синкаемого корпуса, см. ingestion/src/sync.ts).
 *
 * law_documents — один документ adilet.zan.kz (кодекс/закон) целиком, is_active — снят ли он
 * целиком (документ признан утратившим силу целиком) — если true, retrieval никогда не отдаёт
 * его чанки, независимо от similarity.
 *
 * law_articles — одна статья внутри документа. is_active=false — статья фактически изъята
 * ("N) исключен Законом РК от ...", см. parseDocument.ts) — текста нет, только техническая
 * отметка, что было. last_amended_at — дата последней внесённой (уже ДЕЙСТВУЮЩЕЙ) поправки,
 * не будущей — используется в промпте search-агента, чтобы модель могла сказать "по состоянию
 * на редакцию от ...".
 *
 * law_chunks — статья может быть разбита на несколько чанков для эмбеддинга (длинные статьи).
 * Текст в content — уже ОЧИЩЕННЫЙ от ещё не вступивших в силу поправок (см. IN_FORCE_LAW_GUARD
 * в shared/src/security/userContent.ts — здесь тот же принцип обеспечен кодом парсера, а не
 * только текстом промпта).
 */
exports.up = (pgm) => {
  pgm.sql(`
    CREATE EXTENSION IF NOT EXISTS vector;

    CREATE TABLE law_documents (
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

    CREATE TABLE law_articles (
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

    CREATE TABLE law_chunks (
      id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
      article_id UUID NOT NULL REFERENCES law_articles(id) ON DELETE CASCADE,
      document_id TEXT NOT NULL REFERENCES law_documents(id) ON DELETE CASCADE,
      chunk_index INTEGER NOT NULL,
      content TEXT NOT NULL,
      -- text-embedding-3-small, см. shared/src/llm/embeddingClient.ts — 1536 измерений,
      -- выбран ради latency/стоимости, не -large (см. комментарий у DEFAULT_EMBEDDING_MODEL).
      embedding VECTOR(1536) NOT NULL,
      created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
      UNIQUE (article_id, chunk_index)
    );

    CREATE INDEX law_chunks_document_id_idx ON law_chunks (document_id);
    CREATE INDEX law_articles_document_id_idx ON law_articles (document_id);
    -- HNSW, не ivfflat: корпус на старте маленький (несколько кодексов) и будет расти
    -- инкрементально — ivfflat требует пересчёта списков (ANALYZE/REINDEX) при росте таблицы,
    -- HNSW строится инкрементально без этого шага, что подходит частым ресинкам.
    CREATE INDEX law_chunks_embedding_idx ON law_chunks
      USING hnsw (embedding vector_cosine_ops);

    -- Вызывается через Supabase PostgREST RPC (см. backend/src/retrieval/retrieval.service.ts) —
    -- backend не подключается к Postgres напрямую (см. db/db.module.ts), поэтому векторный поиск
    -- должен быть доступен как функция, а не сырой SQL с оператором <=>.
    -- SECURITY INVOKER (по умолчанию) — ok, вызывается только admin-клиентом с secret key,
    -- никаких RLS-политик на публичных таблицах законодательства не требуется.
    CREATE FUNCTION match_law_chunks(
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
  `);
};

exports.down = (pgm) => {
  pgm.sql(`
    DROP FUNCTION IF EXISTS match_law_chunks;
    DROP TABLE IF EXISTS law_chunks;
    DROP TABLE IF EXISTS law_articles;
    DROP TABLE IF EXISTS law_documents;
  `);
};
