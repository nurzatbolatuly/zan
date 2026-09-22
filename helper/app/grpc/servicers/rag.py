"""Реализация RagService (proto/zan/rpc/v1/rag.proto, BACKEND_PLAN.md §3.1,
Stage 5): валидация protobuf-запроса, вызов app/services/rag_service.py,
маппинг исключения в grpc.StatusCode (BACKEND_CODING_STANDARDS.md §1.2, §8).
"""

import grpc

from app.grpc.error_mapping import abort_for_exception
from app.services.rag_service import RagService
from zan.rpc.v1 import rag_pb2, rag_pb2_grpc


class RagServicer(rag_pb2_grpc.RagServiceServicer):
    """Тонкий gRPC-сервисер поверх RagService. default_top_k — top_k=0 (не
    задано отправителем, proto3 default) не значит "ноль результатов", а
    "вызывающая сторона не указала" — подставляем settings.rag_search_top_k_default
    вместо пустого ответа. Go сейчас всегда передаёт top_k явно (см.
    internal/agent), этот путь — только страховка на случай будущих клиентов."""

    def __init__(self, service: RagService, default_top_k: int) -> None:
        self._service = service
        self._default_top_k = default_top_k

    async def Search(
        self, request: rag_pb2.SearchRequest, context: "grpc.aio.ServicerContext[object, object]"
    ) -> rag_pb2.SearchResponse:
        top_k = request.top_k or self._default_top_k
        try:
            results = await self._service.search(request.query_text, top_k)
        except Exception as exc:  # маппинг — единая точка, app/grpc/error_mapping.py
            await abort_for_exception(context, exc, rpc="RagService/Search")
            raise  # недостижимо — context.abort() поднимает исключение сам
        return rag_pb2.SearchResponse(
            matches=[rag_pb2.SearchMatch(ref=r.ref, quote=r.quote, score=r.score) for r in results]
        )
