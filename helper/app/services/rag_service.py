"""RAG-поиск источников (RagService.Search, BACKEND_PLAN.md §3, Stage 5).
Порты (VectorStore/EmbeddingProvider) объявлены здесь же, где используются,
реализации — в app/adapters/db.py и app/adapters/embeddings.py
(BACKEND_CODING_STANDARDS.md §1.2).
"""

from typing import Protocol

from app.domain.rag import SearchResult


class EmbeddingProvider(Protocol):
    """Порт получения векторного представления текста запроса. Self-hosted
    по умолчанию (app/adapters/embeddings.FastEmbedProvider) — тот же
    Strategy-приём, что SttProvider (BACKEND_PLAN.md §6 п.10): замена на
    внешний API (OpenAI/Voyage) не требует переписывания RagService."""

    def embed(self, text: str) -> list[float]:
        """Синхронный вызов намеренно (не async) — модель грузится в
        память процесса и считает на CPU без сетевого I/O, оборачивать в
        asyncio.to_thread — забота вызывающей стороны (см.
        app/grpc/servicers/rag.py), не самого порта."""
        ...


class VectorStore(Protocol):
    """Порт доступа к pgvector-индексу схемы rag (BACKEND_PLAN.md §1.3:
    схемой rag владеет и мигрирует только Python)."""

    async def search(self, embedding: list[float], top_k: int) -> list[SearchResult]:
        """Возвращает до top_k кандидатов, отсортированных по убыванию
        релевантности (cosine similarity). Пустой список — легитимный
        исход (корпус пуст/ничего похожего не нашлось), не ошибка."""
        ...


class RagService:
    """Бизнес-логика RagService.Search: embed(query) -> store.search(...).
    Никакой дополнительной фильтрации/обрезки здесь — top_k и то, что с
    результатом делать дальше (что положить в промпт), решает вызывающая
    сторона (Go, BACKEND_PLAN.md §1.1)."""

    def __init__(self, embeddings: EmbeddingProvider, store: VectorStore) -> None:
        self._embeddings = embeddings
        self._store = store

    async def search(self, query_text: str, top_k: int) -> list[SearchResult]:
        embedding = self._embeddings.embed(query_text)
        return await self._store.search(embedding, top_k)
