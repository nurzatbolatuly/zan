/** @type {import('node-pg-migrate').ColumnDefinitions | undefined} */
exports.shorthands = undefined;

/**
 * Гибридный поиск (вектор + полнотекст) вместо чисто векторного (см. law_corpus.cjs). Проблема,
 * которую это решает: чисто векторный top-k может не найти статью, которая реально есть в
 * проиндексированном документе, если формулировка вопроса пользователя семантически далека от
 * текста статьи (редкий термин, номер статьи, специфичное название) — документ "в базе", но
 * нужный кусок не попал в top-8 по косинусной близости. Полнотекстовый поиск по точным словам
 * ловит именно такие случаи, векторный — перефразированные вопросы без точных терминов; вместе
 * они взаимно перекрывают слабые места друг друга (стандартный паттерн для production RAG).
 *
 * content_tsv — generated column (не пересчитывается вручную при каждой записи, Postgres делает
 * это сам при INSERT), 'russian' text search config — встроенный в Postgres, отдельной настройки
 * не требует.
 */
exports.up = (pgm) => {
  pgm.sql(`
    ALTER TABLE law_chunks ADD COLUMN content_tsv tsvector
      GENERATED ALWAYS AS (to_tsvector('russian', content)) STORED;

    CREATE INDEX law_chunks_content_tsv_idx ON law_chunks USING gin (content_tsv);

    -- CREATE OR REPLACE не заменяет функцию с ДРУГОЙ сигнатурой параметров (новый query_text) —
    -- он бы просто добавил вторую перегрузку match_law_chunks и оставил старую 3-аргументную
    -- версию (из law_corpus.cjs) висеть в базе неиспользуемой. Явный DROP убирает старую версию.
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
  `);
};

exports.down = (pgm) => {
  pgm.sql(`
    DROP FUNCTION IF EXISTS match_law_chunks(VECTOR(1536), TEXT, INTEGER, FLOAT);

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

    DROP INDEX IF EXISTS law_chunks_content_tsv_idx;
    ALTER TABLE law_chunks DROP COLUMN IF EXISTS content_tsv;
  `);
};
