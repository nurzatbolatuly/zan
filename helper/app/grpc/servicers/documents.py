"""Реализация DocumentsService (documents.proto, BACKEND_PLAN.md §3.1)."""

import grpc

from app.domain.documents import DocumentDraft, DocumentSection, RenderFormat
from app.grpc.error_mapping import abort_for_exception
from app.services.render_service import RenderService
from zan.rpc.v1 import documents_pb2, documents_pb2_grpc

_FORMATS = {
    documents_pb2.RENDER_FORMAT_PDF: RenderFormat.PDF,
    documents_pb2.RENDER_FORMAT_DOCX: RenderFormat.DOCX,
}


class DocumentsServicer(documents_pb2_grpc.DocumentsServiceServicer):
    """Тонкий gRPC-сервисер поверх RenderService."""

    def __init__(self, service: RenderService) -> None:
        self._service = service

    async def Render(
        self,
        request: documents_pb2.RenderRequest,
        context: "grpc.aio.ServicerContext[object, object]",
    ) -> documents_pb2.RenderResponse:
        fmt = _FORMATS.get(request.format)
        if fmt is None:
            await context.abort(
                grpc.StatusCode.INVALID_ARGUMENT, f"unsupported format: {request.format}"
            )
            raise AssertionError("unreachable — context.abort() raises")

        draft = DocumentDraft(
            title=request.title,
            sections=[DocumentSection(title=s.title, body=s.body) for s in request.sections],
        )
        try:
            result = await self._service.render(draft, fmt)
        except Exception as exc:
            await abort_for_exception(context, exc, rpc="DocumentsService/Render")
            raise
        return documents_pb2.RenderResponse(file_url=result.file_url, object_key=result.object_key)
