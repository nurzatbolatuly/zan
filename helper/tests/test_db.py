import pytest

from app.adapters.db import PgVectorStore
from app.domain.errors import RagUnavailableError


async def test_search_raises_when_pool_is_none() -> None:
    """app.main не смог создать пул к схеме rag при старте (роль/схема ещё
    не забутстрапены) — PgVectorStore.search должен явно и сразу падать,
    не зависать/паниковать неявно (BACKEND_LOG.md Stage 5)."""
    store = PgVectorStore(pool=None)

    with pytest.raises(RagUnavailableError):
        await store.search([0.1, 0.2, 0.3], top_k=5)
