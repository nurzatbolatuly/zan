"""Сквозной тест на реальный grpc.aio-сервер + сгенерированный клиент
(BACKEND_CODING_STANDARDS.md §10), не на прямой вызов методов сервисера —
так проверяется и interceptor (auth/logging), которого прямой вызов бы
не затронул."""

import io
from collections.abc import AsyncIterator

import docx
import grpc
import pymupdf
import pytest

from app.adapters.extraction import PyMuPdfExtractor, PythonDocxExtractor
from app.adapters.render import DocxTplRenderer
from app.domain.documents import RenderFormat
from app.domain.errors import ConversionError
from app.domain.files import MIME_DOCX
from app.grpc.interceptors import AuthAndLoggingInterceptor
from app.grpc.servicers.documents import DocumentsServicer
from app.grpc.servicers.files import FilesServicer
from app.grpc.servicers.stt import SttServicer
from app.services.conversion_service import ConversionService
from app.services.extraction_service import ExtractionService
from app.services.render_service import RenderService
from app.services.stt_service import SttService
from zan.rpc.v1 import (
    common_pb2,
    documents_pb2,
    documents_pb2_grpc,
    files_pb2,
    files_pb2_grpc,
    stt_pb2,
    stt_pb2_grpc,
)

INTERNAL_SECRET = "test-internal-secret"


class FakeDownloader:
    """Возвращает заранее заданный контент независимо от file_url — реальная
    сеть подменена, реальные PyMuPDF/python-docx/docxtpl — нет."""

    def __init__(self) -> None:
        self.content_by_url: dict[str, bytes] = {}
        self.default_content = b""

    async def download(self, file_url: str) -> bytes:
        return self.content_by_url.get(file_url, self.default_content)


class NeverCalledOcr:
    """OCR не должен вызываться ни в одном из тестов ниже — оба фикстурных
    файла несут текстовый слой/валидны без фолбэка."""

    def ocr_image(self, data: bytes) -> str:
        raise AssertionError("ocr_image must not be called in these tests")

    def ocr_pdf(self, data: bytes) -> str:
        raise AssertionError("ocr_pdf must not be called in these tests")


class FakeSttProvider:
    def transcribe(self, audio: bytes, lang: str) -> str:
        return "привет"


class FakeUploader:
    async def upload_generated(self, key: str, data: bytes, content_type: str) -> tuple[str, str]:
        return f"https://storage.public/{key}", key


class FakePdfConverter:
    """Реальный LibreOffice в юнит-тестах не поднимается (системный пакет есть
    только в Docker-образе) — пустой вход имитирует повреждённый файл."""

    def convert(self, data: bytes, source_suffix: str) -> bytes:
        if not data:
            raise ConversionError("empty source")
        return b"%PDF-1.7 converted"


def _make_pdf_bytes(text: str) -> bytes:
    doc = pymupdf.open()
    page = doc.new_page()
    page.insert_text((72, 72), text)
    data: bytes = doc.tobytes()
    doc.close()
    return data


def _make_docx_bytes(text: str) -> bytes:
    document = docx.Document()
    document.add_paragraph(text)
    buf = io.BytesIO()
    document.save(buf)
    return buf.getvalue()


async def _start_server(downloader: FakeDownloader) -> tuple[grpc.aio.Server, int]:
    extraction_service = ExtractionService(
        downloader, PyMuPdfExtractor(), PythonDocxExtractor(), NeverCalledOcr()
    )
    conversion_service = ConversionService(downloader, FakePdfConverter(), FakeUploader())
    stt_service = SttService(downloader, FakeSttProvider())
    render_service = RenderService(
        {RenderFormat.DOCX: DocxTplRenderer(), RenderFormat.PDF: DocxTplRenderer()},
        uploader=FakeUploader(),
    )

    server = grpc.aio.server(interceptors=[AuthAndLoggingInterceptor(INTERNAL_SECRET)])
    files_pb2_grpc.add_FilesServiceServicer_to_server(
        FilesServicer(extraction_service, conversion_service), server
    )
    stt_pb2_grpc.add_SttServiceServicer_to_server(SttServicer(stt_service), server)
    documents_pb2_grpc.add_DocumentsServiceServicer_to_server(
        DocumentsServicer(render_service), server
    )
    port = server.add_insecure_port("[::]:0")
    await server.start()
    return server, port


@pytest.fixture
async def downloader() -> FakeDownloader:
    return FakeDownloader()


@pytest.fixture
async def channel(downloader: FakeDownloader) -> AsyncIterator[grpc.aio.Channel]:
    server, port = await _start_server(downloader)
    try:
        async with grpc.aio.insecure_channel(f"localhost:{port}") as ch:
            yield ch
    finally:
        await server.stop(grace=None)


def _auth_metadata() -> list[tuple[str, str]]:
    return [("x-internal-secret", INTERNAL_SECRET), ("x-trace-id", "trace-123")]


async def test_extract_rejects_call_without_internal_secret(channel: grpc.aio.Channel) -> None:
    stub = files_pb2_grpc.FilesServiceStub(channel)

    with pytest.raises(grpc.aio.AioRpcError) as exc_info:
        await stub.Extract(
            files_pb2.ExtractRequest(file_url="https://x/y", mime_type="application/pdf")
        )

    assert exc_info.value.code() == grpc.StatusCode.UNAUTHENTICATED


async def test_extract_rejects_wrong_internal_secret(channel: grpc.aio.Channel) -> None:
    stub = files_pb2_grpc.FilesServiceStub(channel)

    with pytest.raises(grpc.aio.AioRpcError) as exc_info:
        await stub.Extract(
            files_pb2.ExtractRequest(file_url="https://x/y", mime_type="application/pdf"),
            metadata=[("x-internal-secret", "wrong-secret")],
        )

    assert exc_info.value.code() == grpc.StatusCode.UNAUTHENTICATED


async def test_extract_pdf_succeeds_with_valid_secret(
    channel: grpc.aio.Channel, downloader: FakeDownloader
) -> None:
    downloader.content_by_url["https://x/contract.pdf"] = _make_pdf_bytes("Rental agreement")
    stub = files_pb2_grpc.FilesServiceStub(channel)

    response = await stub.Extract(
        files_pb2.ExtractRequest(file_url="https://x/contract.pdf", mime_type="application/pdf"),
        metadata=_auth_metadata(),
    )

    assert "Rental agreement" in response.text


async def test_extract_docx_succeeds(channel: grpc.aio.Channel, downloader: FakeDownloader) -> None:
    downloader.content_by_url["https://x/contract.docx"] = _make_docx_bytes("Договор №1")
    stub = files_pb2_grpc.FilesServiceStub(channel)

    response = await stub.Extract(
        files_pb2.ExtractRequest(
            file_url="https://x/contract.docx",
            mime_type="application/vnd.openxmlformats-officedocument.wordprocessingml.document",
        ),
        metadata=_auth_metadata(),
    )

    assert "Договор №1" in response.text


async def test_extract_corrupted_docx_is_invalid_argument(channel: grpc.aio.Channel) -> None:
    stub = files_pb2_grpc.FilesServiceStub(channel)

    with pytest.raises(grpc.aio.AioRpcError) as exc_info:
        await stub.Extract(
            files_pb2.ExtractRequest(
                file_url="https://x/not-really.docx",
                mime_type="application/vnd.openxmlformats-officedocument.wordprocessingml.document",
            ),
            metadata=_auth_metadata(),
        )

    assert exc_info.value.code() == grpc.StatusCode.INVALID_ARGUMENT


async def test_extract_unsupported_mime_type_is_invalid_argument(
    channel: grpc.aio.Channel,
) -> None:
    stub = files_pb2_grpc.FilesServiceStub(channel)

    with pytest.raises(grpc.aio.AioRpcError) as exc_info:
        await stub.Extract(
            files_pb2.ExtractRequest(file_url="https://x/y", mime_type="application/zip"),
            metadata=_auth_metadata(),
        )

    assert exc_info.value.code() == grpc.StatusCode.INVALID_ARGUMENT


async def test_transcribe_succeeds_and_returns_text(channel: grpc.aio.Channel) -> None:
    stub = stt_pb2_grpc.SttServiceStub(channel)

    response = await stub.Transcribe(
        stt_pb2.TranscribeRequest(
            file_url="https://x/audio.webm", mime_type="audio/webm", lang=common_pb2.LANG_RU
        ),
        metadata=_auth_metadata(),
    )

    assert response.text == "привет"


async def test_render_docx_succeeds_and_returns_file_url(channel: grpc.aio.Channel) -> None:
    stub = documents_pb2_grpc.DocumentsServiceStub(channel)

    response = await stub.Render(
        documents_pb2.RenderRequest(
            title="Заявление",
            sections=[documents_pb2.DocumentSection(title="Пункт 1", body="Текст")],
            format=documents_pb2.RENDER_FORMAT_DOCX,
        ),
        metadata=_auth_metadata(),
    )

    assert response.file_url.startswith("https://storage.public/generated/")
    assert response.object_key.endswith(".docx")


async def test_render_rejects_unspecified_format(channel: grpc.aio.Channel) -> None:
    stub = documents_pb2_grpc.DocumentsServiceStub(channel)

    with pytest.raises(grpc.aio.AioRpcError) as exc_info:
        await stub.Render(
            documents_pb2.RenderRequest(title="T", sections=[]),
            metadata=_auth_metadata(),
        )

    assert exc_info.value.code() == grpc.StatusCode.INVALID_ARGUMENT


async def test_convert_to_pdf_returns_object_key(
    channel: grpc.aio.Channel, downloader: FakeDownloader
) -> None:
    downloader.content_by_url["https://x/template.docx"] = _make_docx_bytes("Договор")
    stub = files_pb2_grpc.FilesServiceStub(channel)

    response = await stub.ConvertToPdf(
        files_pb2.ConvertToPdfRequest(file_url="https://x/template.docx", mime_type=MIME_DOCX),
        metadata=_auth_metadata(),
    )

    assert response.object_key.startswith("converted/")
    assert response.object_key.endswith(".pdf")


async def test_convert_to_pdf_failed_conversion_is_invalid_argument(
    channel: grpc.aio.Channel,
) -> None:
    stub = files_pb2_grpc.FilesServiceStub(channel)

    with pytest.raises(grpc.aio.AioRpcError) as exc_info:
        await stub.ConvertToPdf(
            files_pb2.ConvertToPdfRequest(file_url="https://x/empty.docx", mime_type=MIME_DOCX),
            metadata=_auth_metadata(),
        )

    assert exc_info.value.code() == grpc.StatusCode.INVALID_ARGUMENT
