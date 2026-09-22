"""Доменные модели RAG-поиска — НЕ те же классы, что сгенерированы из
.proto (BACKEND_CODING_STANDARDS.md §1.2). Конвертация в/из wire-типов
происходит на границе app/grpc/servicers/rag.py, бизнес-логика
(app/services/rag_service.py) работает с этими моделями.
"""

from dataclasses import dataclass


@dataclass(frozen=True)
class SearchResult:
    """Один кандидат-источник (rag.chunks) — та же форма, что
    zanv1.SearchMatch (ref/quote/score), но не сам сгенерированный класс
    (Stage 5, BACKEND_PLAN.md)."""

    ref: str
    quote: str
    score: float
