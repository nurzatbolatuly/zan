"""Реализация SttService (stt.proto, BACKEND_PLAN.md §3.1)."""

import grpc

from app.grpc.error_mapping import abort_for_exception
from app.services.stt_service import SttService as SttServiceImpl
from zan.rpc.v1 import common_pb2, stt_pb2, stt_pb2_grpc

# Lang (common.proto — значения enum'а генерируются в common_pb2, не в
# stt_pb2, хотя используются полем TranscribeRequest.lang) -> код языка для
# app/services/stt_service.SttService.
_LANG_CODES = {
    common_pb2.LANG_RU: "ru",
    common_pb2.LANG_KZ: "kz",
}


class SttServicer(stt_pb2_grpc.SttServiceServicer):
    """Тонкий gRPC-сервисер поверх SttService."""

    def __init__(self, service: SttServiceImpl) -> None:
        self._service = service

    async def Transcribe(
        self,
        request: stt_pb2.TranscribeRequest,
        context: "grpc.aio.ServicerContext[object, object]",
    ) -> stt_pb2.TranscribeResponse:
        lang = _LANG_CODES.get(request.lang, "")
        try:
            result = await self._service.transcribe(request.file_url, lang)
        except Exception as exc:
            await abort_for_exception(context, exc, rpc="SttService/Transcribe")
            raise
        return stt_pb2.TranscribeResponse(text=result.text)
