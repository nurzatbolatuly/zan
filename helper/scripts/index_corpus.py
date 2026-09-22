"""Офлайн/админский batch job — индексация корпуса НПА в rag.chunks
(BACKEND_PLAN.md §3.1, Stage 5: "отдельный офлайн/админский скрипт
индексации... не RPC"). Не часть рантайма helper/ — запускается вручную
(`make rag-index`) после `make rag-migrate`, читает JSON-файл вида
`[{"ref": "...", "quote": "..."}]`, считает embedding тем же провайдером,
что и рантайм (app.adapters.embeddings.FastEmbedProvider — одна и та же
модель на индексации и на поиске, иначе векторы несравнимы), пишет в БД.

ВНИМАНИЕ: заменяет содержимое rag.chunks целиком (TRUNCATE + INSERT) — это
admin-инструмент для (пере)заливки всего корпуса разом, не инкрементальный
апдейт по одной статье. Сид-корпус по умолчанию (scripts/fixtures/
rag_seed_corpus.json) — небольшой иллюстративный набор для тестов/DoD
Stage 5 (BACKEND_PLAN.md: "хотя бы небольшим сид-корпусом"), не полная база
нормативных актов РК — реальное наполнение корпуса вне скоупа этого этапа.
"""

import asyncio
import json
import os
import sys
import uuid
from pathlib import Path

import asyncpg

from app.adapters.embeddings import FastEmbedProvider

# Читает RAG_DATABASE_URL/EMBEDDING_MODEL напрямую из os.environ, не через
# app.core.config.Settings — Settings требует ещё и S3/internal_secret
# (нужны рантайму gRPC-сервера, не этому отдельному batch-скрипту), заводить
# их только ради вызова этого CLI было бы лишней связанностью (тот же
# принцип, что отдельный DATABASE_URL у Go-шного golang-migrate CLI, не
# полный config.Config приложения).
_DEFAULT_EMBEDDING_MODEL = "sentence-transformers/paraphrase-multilingual-MiniLM-L12-v2"


async def index_corpus(corpus_path: Path) -> None:
    entries = json.loads(corpus_path.read_text(encoding="utf-8"))
    if not entries:
        print(f"WARN: {corpus_path} is empty, nothing to index")
        return

    rag_database_url = os.environ["RAG_DATABASE_URL"]
    embedding_model = os.environ.get("EMBEDDING_MODEL", _DEFAULT_EMBEDDING_MODEL)

    embeddings = FastEmbedProvider(embedding_model)
    pool = await asyncpg.create_pool(rag_database_url)
    try:
        async with pool.acquire() as conn, conn.transaction():
            await conn.execute("TRUNCATE rag.chunks")
            for entry in entries:
                vector = embeddings.embed(entry["quote"])
                vector_literal = "[" + ",".join(repr(v) for v in vector) + "]"
                await conn.execute(
                    "INSERT INTO rag.chunks (id, ref, quote, embedding) "
                    "VALUES ($1, $2, $3, $4::vector)",
                    uuid.uuid4(),
                    entry["ref"],
                    entry["quote"],
                    vector_literal,
                )
        print(f"Indexed {len(entries)} chunks from {corpus_path} into rag.chunks")
    finally:
        await pool.close()


_REPO_ROOT = Path(__file__).resolve().parents[2]
_DEFAULT_CORPUS_PATH = _REPO_ROOT / "scripts" / "fixtures" / "rag_seed_corpus.json"


def main() -> None:
    corpus_path = Path(sys.argv[1]) if len(sys.argv) > 1 else _DEFAULT_CORPUS_PATH
    if not corpus_path.exists():
        raise SystemExit(f"corpus file not found: {corpus_path}")
    asyncio.run(index_corpus(corpus_path))


if __name__ == "__main__":
    main()
