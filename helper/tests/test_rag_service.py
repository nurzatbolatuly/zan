from app.domain.rag import SearchResult
from app.services.rag_service import RagService


class FakeEmbeddingProvider:
    def __init__(self, vector: list[float] | None = None) -> None:
        self.vector = vector if vector is not None else [0.1, 0.2, 0.3]
        self.calls: list[str] = []

    def embed(self, text: str) -> list[float]:
        self.calls.append(text)
        return self.vector


class FakeVectorStore:
    def __init__(self, results: list[SearchResult] | None = None) -> None:
        self.results = results if results is not None else []
        self.calls: list[tuple[list[float], int]] = []

    async def search(self, embedding: list[float], top_k: int) -> list[SearchResult]:
        self.calls.append((embedding, top_k))
        return self.results


async def test_search_embeds_query_then_searches_store() -> None:
    embeddings = FakeEmbeddingProvider(vector=[1.0, 2.0])
    expected = [SearchResult(ref="ст. 1", quote="текст", score=0.9)]
    store = FakeVectorStore(results=expected)
    svc = RagService(embeddings, store)

    results = await svc.search("какой срок исковой давности?", top_k=3)

    assert results == expected
    assert embeddings.calls == ["какой срок исковой давности?"]
    assert store.calls == [([1.0, 2.0], 3)]


async def test_search_returns_empty_list_when_nothing_found() -> None:
    svc = RagService(FakeEmbeddingProvider(), FakeVectorStore(results=[]))

    results = await svc.search("что-то, чего нет в корпусе", top_k=5)

    assert results == []
