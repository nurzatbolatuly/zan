"""Реализация FilesService (files.proto, BACKEND_PLAN.md §3.1)."""

import grpc

from app.grpc.error_mapping import abort_for_exception
from app.services.conversion_service import ConversionService
from app.services.extraction_service import ExtractionService
from zan.rpc.v1 import files_pb2, files_pb2_grpc


class FilesServicer(files_pb2_grpc.FilesServiceServicer):
    """Тонкий gRPC-сервисер поверх ExtractionService/ConversionService:
    валидация protobuf-запроса, вызов сервиса, маппинг исключения в
    grpc.StatusCode (BACKEND_CODING_STANDARDS.md §1.2)."""

    def __init__(self, extraction: ExtractionService, conversion: ConversionService) -> None:
        self._extraction = extraction
        self._conversion = conversion

    async def Extract(
        self, request: files_pb2.ExtractRequest, context: "grpc.aio.ServicerContext[object, object]"
    ) -> files_pb2.ExtractResponse:
        try:
            result = await self._extraction.extract(request.file_url, request.mime_type)
        except Exception as exc:  # маппинг — единая точка, app/grpc/error_mapping.py
            await abort_for_exception(context, exc, rpc="FilesService/Extract")
            raise  # недостижимо — context.abort() поднимает исключение сам
        return files_pb2.ExtractResponse(text=result.text)

    async def ConvertToPdf(
        self,
        request: files_pb2.ConvertToPdfRequest,
        context: "grpc.aio.ServicerContext[object, object]",
    ) -> files_pb2.ConvertToPdfResponse:
        try:
            result = await self._conversion.convert_to_pdf(request.file_url, request.mime_type)
        except Exception as exc:
            await abort_for_exception(context, exc, rpc="FilesService/ConvertToPdf")
            raise
        return files_pb2.ConvertToPdfResponse(object_key=result.object_key)
