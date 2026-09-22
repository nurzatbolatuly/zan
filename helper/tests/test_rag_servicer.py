"""Сквозной тест на реальный grpc.aio-сервер + сгенерированный клиент
(BACKEND_CODING_STANDARDS.md §10), тот же приём, что tests/test_grpc_servicers.py —
отдельный файл, т.к. зависимости RagService (embeddings/vector store) не
пересекаются с FilesService/SttService/DocumentsService (нет storage/downloader)."""

from collections.abc import AsyncIterator

import grpc
import pytest

from app.domain.errors import RagUnavailableError
from app.domain.rag import SearchResult
from app.grpc.interceptors import AuthAndLoggingInterceptor
from app.grpc.servicers.rag import RagServicer
from app.services.rag_service import RagService, VectorStore
from zan.rpc.v1 import rag_pb2, rag_pb2_grpc

INTERNAL_SECRET = "test-internal-secret"
DEFAULT_TOP_K = 5


class FakeEmbeddingProvider:
    def embed(self, text: str) -> list[float]:
        return [0.1, 0.2, 0.3]


class FakeVectorStore:
    """Возвращает заранее заданные результаты независимо от embedding,
    но записывает top_k, с которым его вызвали — проверяет, что
    RagServicer.Search правильно прокидывает top_k из запроса (или дефолт,
    если top_k=0)."""

    def __init__(self, results: list[SearchResult]) -> None:
        self.results = results
        self.top_k_calls: list[int] = []

    async def search(self, embedding: list[float], top_k: int) -> list[SearchResult]:
        self.top_k_calls.append(top_k)
        return self.results


async def _start_server(store: VectorStore) -> tuple[grpc.aio.Server, int]:
    service = RagService(FakeEmbeddingProvider(), store)
    server = grpc.aio.server(interceptors=[AuthAndLoggingInterceptor(INTERNAL_SECRET)])
    rag_pb2_grpc.add_RagServiceServicer_to_server(RagServicer(service, DEFAULT_TOP_K), server)
    port = server.add_insecure_port("[::]:0")
    await server.start()
    return server, port


@pytest.fixture
def store() -> FakeVectorStore:
    return FakeVectorStore(results=[SearchResult(ref="ст. 1", quote="текст статьи", score=0.87)])


@pytest.fixture
async def channel(store: FakeVectorStore) -> AsyncIterator[grpc.aio.Channel]:
    server, port = await _start_server(store)
    try:
        async with grpc.aio.insecure_channel(f"localhost:{port}") as ch:
            yield ch
    finally:
        await server.stop(grace=None)


def _auth_metadata() -> list[tuple[str, str]]:
    return [("x-internal-secret", INTERNAL_SECRET), ("x-trace-id", "trace-123")]


async def test_search_rejects_call_without_internal_secret(channel: grpc.aio.Channel) -> None:
    stub = rag_pb2_grpc.RagServiceStub(channel)

    with pytest.raises(grpc.aio.AioRpcError) as exc_info:
        await stub.Search(rag_pb2.SearchRequest(query_text="вопрос", top_k=3))

    assert exc_info.value.code() == grpc.StatusCode.UNAUTHENTICATED


async def test_search_returns_matches(channel: grpc.aio.Channel) -> None:
    stub = rag_pb2_grpc.RagServiceStub(channel)

    resp = await stub.Search(
        rag_pb2.SearchRequest(query_text="какой срок исковой давности?", top_k=3),
        metadata=_auth_metadata(),
    )

    assert len(resp.matches) == 1
    assert resp.matches[0].ref == "ст. 1"
    assert resp.matches[0].quote == "текст статьи"
    assert resp.matches[0].score == pytest.approx(0.87)


async def test_search_uses_requested_top_k(
    channel: grpc.aio.Channel, store: FakeVectorStore
) -> None:
    stub = rag_pb2_grpc.RagServiceStub(channel)

    await stub.Search(
        rag_pb2.SearchRequest(query_text="вопрос", top_k=7), metadata=_auth_metadata()
    )

    assert store.top_k_calls == [7]


async def test_search_falls_back_to_default_top_k_when_unset(
    channel: grpc.aio.Channel, store: FakeVectorStore
) -> None:
    stub = rag_pb2_grpc.RagServiceStub(channel)

    await stub.Search(rag_pb2.SearchRequest(query_text="вопрос"), metadata=_auth_metadata())

    assert store.top_k_calls == [DEFAULT_TOP_K]


async def test_search_returns_empty_matches_when_store_finds_nothing() -> None:
    empty_store = FakeVectorStore(results=[])
    server, port = await _start_server(empty_store)
    try:
        async with grpc.aio.insecure_channel(f"localhost:{port}") as channel:
            stub = rag_pb2_grpc.RagServiceStub(channel)
            resp = await stub.Search(
                rag_pb2.SearchRequest(query_text="ничего похожего", top_k=5),
                metadata=_auth_metadata(),
            )
    finally:
        await server.stop(grace=None)

    assert list(resp.matches) == []


class UnavailableVectorStore:
    """Симулирует app.adapters.db.PgVectorStore(pool=None) — рантайм-
    состояние "пул к схеме rag не поднялся при старте" (BACKEND_LOG.md
    Stage 5)."""

    async def search(self, embedding: list[float], top_k: int) -> list[SearchResult]:
        raise RagUnavailableError("rag database pool was not initialized at startup")


async def test_search_maps_rag_unavailable_to_internal() -> None:
    server, port = await _start_server(UnavailableVectorStore())
    try:
        async with grpc.aio.insecure_channel(f"localhost:{port}") as channel:
            stub = rag_pb2_grpc.RagServiceStub(channel)
            with pytest.raises(grpc.aio.AioRpcError) as exc_info:
                await stub.Search(
                    rag_pb2.SearchRequest(query_text="вопрос", top_k=5),
                    metadata=_auth_metadata(),
                )
    finally:
        await server.stop(grace=None)

    assert exc_info.value.code() == grpc.StatusCode.INTERNAL
