"""Доступ к схеме rag (asyncpg, без ORM — тот же принцип, что pgx напрямую
в Go) — реализует rag_service.VectorStore (BACKEND_CODING_STANDARDS.md
§1.2). Схемой rag владеет и мигрирует только Python (Alembic,
helper/migrations/), не Go (BACKEND_PLAN.md §1.3).
"""

import asyncpg

from app.domain.errors import RagUnavailableError
from app.domain.rag import SearchResult


class PgVectorStore:
    """Реализация rag_service.VectorStore поверх rag.chunks (pgvector,
    HNSW-индекс по cosine distance — см. helper/migrations/versions,
    выбор индекса обоснован там же). Пул соединений создаётся один раз в
    composition root (app.main), передаётся сюда конструктором — тот же
    принцип, что *pgxpool.Pool в Go (BACKEND_CODING_STANDARDS.md §2).

    pool=None — app.main не смог создать пул при старте (роль zan_rag/схема
    rag ещё не забутстрапены — например, старый том Postgres без
    scripts/postgres-init/, см. BACKEND_LOG.md Stage 5) — не роняет весь
    процесс (Files/Stt/Documents от rag не зависят), но каждый вызов
    search() честно и сразу падает, а не зависает/паникует."""

    def __init__(self, pool: asyncpg.Pool | None) -> None:
        self._pool = pool

    async def search(self, embedding: list[float], top_k: int) -> list[SearchResult]:
        if self._pool is None:
            raise RagUnavailableError("rag database pool was not initialized at startup")

        # pgvector принимает текстовый литерал вида "[0.1,0.2,...]" с
        # явным приведением ::vector — не тянем отдельно пакет pgvector
        # (codec для asyncpg) ради одного этого запроса.
        vector_literal = "[" + ",".join(repr(v) for v in embedding) + "]"
        rows = await self._pool.fetch(
            """
            SELECT ref, quote, 1 - (embedding <=> $1::vector) AS score
            FROM rag.chunks
            ORDER BY embedding <=> $1::vector
            LIMIT $2
            """,
            vector_literal,
            top_k,
        )
        return [
            SearchResult(ref=row["ref"], quote=row["quote"], score=row["score"]) for row in rows
        ]
