"""rag.chunks — Stage 5 (BACKEND_PLAN.md §1.3, §3): корпус НПА для
RagService.Search. embedding — vector(384), совпадает с размерностью
sentence-transformers/paraphrase-multilingual-MiniLM-L12-v2
(app/adapters/embeddings.py, settings.embedding_model) — если модель
меняется на другую размерность, эта колонка и индекс пересоздаются новой
миграцией (смена модели embeddings — breaking change для уже
проиндексированного корпуса, переиндексация обязательна, не только смена
конфига).

Индекс — HNSW (не IVFFlat): не требует настройки `lists` под заранее
известный размер корпуса и не деградирует по качеству, пока корпус растёт
(в отличие от IVFFlat, где `lists` считается от ожидаемого числа строк на
момент создания индекса) — разумный дефолт для MVP с небольшим стартовым
сид-корпусом, который будет расти (BACKEND_PLAN.md §1.3: "выбор — на этапе
Stage 5, зависит от размера корпуса", корпус на старте — единицы/десятки
записей, пересмотреть при росте на порядки). vector_cosine_ops — та же
метрика (cosine distance), что PgVectorStore.search использует в запросе
(`embedding <=> $1::vector`).

Revision ID: 0001
Revises:
Create Date: 2026-09-21
"""

from collections.abc import Sequence

from alembic import op

revision: str = "0001"
down_revision: str | None = None
branch_labels: str | Sequence[str] | None = None
depends_on: str | Sequence[str] | None = None

# EMBEDDING_DIM — совпадает с sentence-transformers/paraphrase-multilingual-MiniLM-L12-v2
# (см. docstring модуля выше). Raw SQL, не sa.Table/ORM-декларация — helper/
# не использует pgvector.sqlalchemy (лишняя зависимость только ради типа
# колонки в одной миграции), тот же принцип "raw SQL, без ORM", что и
# app/adapters/db.py в рантайме (BACKEND_CODING_STANDARDS.md §1.2).
EMBEDDING_DIM = 384


def upgrade() -> None:
    # CREATE EXTENSION/SCHEMA — уже сделаны scripts/postgres-init/ (роль
    # zan_rag владеет схемой rag к моменту первого запуска alembic), здесь
    # IF NOT EXISTS — только страховка на случай ручного прогона против
    # окружения, где init-скрипт ещё не применялся.
    op.execute("CREATE EXTENSION IF NOT EXISTS vector")
    op.execute("CREATE SCHEMA IF NOT EXISTS rag")

    op.execute(
        f"""
        CREATE TABLE rag.chunks (
            id         uuid PRIMARY KEY,
            -- ref — то, что LLM обязана процитировать дословно (сверяется в
            -- internal/agent.verifySources со ссылками из ответа LLM),
            -- напр. "ст. 157 Трудового кодекса РК".
            ref        text NOT NULL,
            -- quote — сам текст фрагмента, передаётся в промпт LLM как контекст.
            quote      text NOT NULL,
            embedding  vector({EMBEDDING_DIM}) NOT NULL,
            created_at timestamptz NOT NULL DEFAULT now()
        )
        """
    )
    op.execute(
        "CREATE INDEX idx_chunks_embedding_hnsw ON rag.chunks "
        "USING hnsw (embedding vector_cosine_ops)"
    )


def downgrade() -> None:
    op.execute("DROP TABLE IF EXISTS rag.chunks")
