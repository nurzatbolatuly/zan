import pytest

from app.domain.errors import ConversionError
from app.domain.files import MIME_DOCX, MIME_PDF
from app.services.conversion_service import ConversionService


class FakeDownloader:
    def __init__(self, content: bytes = b"docx-bytes") -> None:
        self.content = content
        self.requested: list[str] = []

    async def download(self, file_url: str) -> bytes:
        self.requested.append(file_url)
        return self.content


class FakePdfConverter:
    def __init__(self, raises: Exception | None = None) -> None:
        self.raises = raises
        self.calls: list[tuple[bytes, str]] = []

    def convert(self, data: bytes, source_suffix: str) -> bytes:
        self.calls.append((data, source_suffix))
        if self.raises:
            raise self.raises
        return b"%PDF-1.7"


class FakeUploader:
    def __init__(self) -> None:
        self.uploads: list[tuple[str, bytes, str]] = []

    async def upload_generated(self, key: str, data: bytes, content_type: str) -> tuple[str, str]:
        self.uploads.append((key, data, content_type))
        return f"https://storage.public/{key}", key


async def test_convert_docx_uploads_pdf_under_converted_prefix() -> None:
    downloader, converter, uploader = FakeDownloader(), FakePdfConverter(), FakeUploader()
    svc = ConversionService(downloader, converter, uploader)

    result = await svc.convert_to_pdf("https://storage.internal/t", MIME_DOCX)

    assert downloader.requested == ["https://storage.internal/t"]
    assert converter.calls == [(b"docx-bytes", ".docx")]
    key, data, content_type = uploader.uploads[0]
    assert key.startswith("converted/")
    assert key.endswith(".pdf")
    assert data == b"%PDF-1.7"
    assert content_type == MIME_PDF
    assert result.object_key == key


async def test_unsupported_mime_type_is_rejected_before_download() -> None:
    downloader, uploader = FakeDownloader(), FakeUploader()
    svc = ConversionService(downloader, FakePdfConverter(), uploader)

    with pytest.raises(ConversionError):
        await svc.convert_to_pdf("https://storage.internal/t", MIME_PDF)

    assert downloader.requested == []
    assert uploader.uploads == []


async def test_converter_failure_propagates_and_nothing_is_uploaded() -> None:
    uploader = FakeUploader()
    svc = ConversionService(
        FakeDownloader(), FakePdfConverter(raises=ConversionError("broken")), uploader
    )

    with pytest.raises(ConversionError):
        await svc.convert_to_pdf("https://storage.internal/t", MIME_DOCX)

    assert uploader.uploads == []
