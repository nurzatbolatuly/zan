import pytest

from app.domain.errors import ExtractionError
from app.domain.files import ExtractedDocument
from app.services.extraction_service import MIME_DOCX, MIME_JPEG, MIME_PDF, ExtractionService


class FakeDownloader:
    def __init__(self, content: bytes = b"data") -> None:
        self.content = content
        self.requested: list[str] = []

    async def download(self, file_url: str) -> bytes:
        self.requested.append(file_url)
        return self.content


class FakePdfExtractor:
    def __init__(self, text: str = "") -> None:
        self.text = text
        self.calls = 0

    def extract(self, data: bytes) -> str:
        self.calls += 1
        return self.text


class FakeDocxExtractor:
    def __init__(self, text: str = "") -> None:
        self.text = text

    def extract(self, data: bytes) -> str:
        return self.text


class FakeOcr:
    def __init__(self, text: str = "", raises: Exception | None = None) -> None:
        self.text = text
        self.raises = raises
        self.pdf_calls = 0
        self.image_calls = 0

    def ocr_image(self, data: bytes) -> str:
        self.image_calls += 1
        if self.raises:
            raise self.raises
        return self.text

    def ocr_pdf(self, data: bytes) -> str:
        self.pdf_calls += 1
        if self.raises:
            raise self.raises
        return self.text


def new_service(
    pdf: FakePdfExtractor | None = None,
    docx: FakeDocxExtractor | None = None,
    ocr: FakeOcr | None = None,
    downloader: FakeDownloader | None = None,
) -> tuple[ExtractionService, FakeDownloader]:
    dl = downloader or FakeDownloader()
    svc = ExtractionService(
        dl, pdf or FakePdfExtractor(), docx or FakeDocxExtractor(), ocr or FakeOcr()
    )
    return svc, dl


async def test_extract_rejects_unsupported_mime_type() -> None:
    svc, _ = new_service()

    with pytest.raises(ExtractionError):
        await svc.extract("https://storage.internal/x", "application/zip")


async def test_extract_pdf_uses_pdf_extractor_when_text_layer_present() -> None:
    svc, dl = new_service(pdf=FakePdfExtractor(text="hello world"))

    result = await svc.extract("https://storage.internal/a.pdf", MIME_PDF)

    assert result == ExtractedDocument(text="hello world")
    assert dl.requested == ["https://storage.internal/a.pdf"]


async def test_extract_pdf_falls_back_to_ocr_when_no_text_layer() -> None:
    ocr = FakeOcr(text="scanned text")
    svc, _ = new_service(pdf=FakePdfExtractor(text=""), ocr=ocr)

    result = await svc.extract("https://storage.internal/scan.pdf", MIME_PDF)

    assert result.text == "scanned text"
    assert ocr.pdf_calls == 1


async def test_extract_docx_uses_docx_extractor() -> None:
    svc, _ = new_service(docx=FakeDocxExtractor(text="contract text"))

    result = await svc.extract("https://storage.internal/a.docx", MIME_DOCX)

    assert result.text == "contract text"


async def test_extract_image_uses_ocr() -> None:
    ocr = FakeOcr(text="text from image")
    svc, _ = new_service(ocr=ocr)

    result = await svc.extract("https://storage.internal/a.jpg", MIME_JPEG)

    assert result.text == "text from image"
    assert ocr.image_calls == 1


async def test_extract_raises_when_result_is_empty() -> None:
    svc, _ = new_service(pdf=FakePdfExtractor(text=""), ocr=FakeOcr(text="   "))

    with pytest.raises(ExtractionError):
        await svc.extract("https://storage.internal/blank.pdf", MIME_PDF)


async def test_extract_propagates_ocr_failure() -> None:
    svc, _ = new_service(ocr=FakeOcr(raises=ExtractionError("ocr binary crashed")))

    with pytest.raises(ExtractionError):
        await svc.extract("https://storage.internal/a.jpg", MIME_JPEG)
