"""Реализации services.extraction_service — PyMuPDF (PDF), python-docx
(DOCX), pytesseract+pdf2image (OCR-фолбэк для сканов/изображений). Обёртки
над блокирующими C-библиотеками — вызываются через asyncio.to_thread на
границе ExtractionService, здесь синхронный код (BACKEND_CODING_STANDARDS.md
§6.2)."""

import io

import docx
import pymupdf
import pytesseract
from pdf2image import convert_from_bytes
from PIL import Image

from app.domain.errors import ExtractionError

# MAX_OCR_PAGES — предохранитель от многостраничного скана, растягивающего
# синхронный gRPC-вызов на минуты (Extract — синхронный RPC в пределах
# одного HTTP-запроса Go, BACKEND_PLAN.md Stage 4 DoD). Значение —
# рекомендация архитектора для MVP, пересмотреть по метрикам прод.
MAX_OCR_PAGES = 20


class PyMuPdfExtractor:
    """Реализация extraction_service.PdfTextExtractor."""

    def extract(self, data: bytes) -> str:
        try:
            # pymupdf.open — недотипизированный алиас на класс Document внутри
            # самого пакета (`open = Document` в pymupdf/__init__.py), несмотря
            # на py.typed (проверено при реализации Stage 4).
            with pymupdf.open(stream=data, filetype="pdf") as doc:  # type: ignore[no-untyped-call]
                return "\n\n".join(page.get_text() for page in doc)
        except pymupdf.FileDataError as exc:
            raise ExtractionError(f"corrupted pdf: {exc}") from exc


class PythonDocxExtractor:
    """Реализация extraction_service.DocxTextExtractor."""

    def extract(self, data: bytes) -> str:
        try:
            document = docx.Document(io.BytesIO(data))
        except Exception as exc:  # python-docx поднимает разные типы (zipfile.BadZipFile и т.п.)
            raise ExtractionError(f"corrupted docx: {exc}") from exc
        return "\n".join(p.text for p in document.paragraphs)


class TesseractOcrProvider:
    """Реализация extraction_service.OcrProvider поверх системного tesseract
    (см. helper/Dockerfile — tesseract-ocr/poppler-utils apt-пакеты) —
    OCR-фолбэк для сканов, используется, когда PyMuPDF не находит текстовый
    слой (Stage 4, BACKEND_PLAN.md)."""

    def __init__(self, lang: str = "rus+eng") -> None:
        self._lang = lang

    def ocr_image(self, data: bytes) -> str:
        try:
            image = Image.open(io.BytesIO(data))
            return str(pytesseract.image_to_string(image, lang=self._lang))
        except Exception as exc:
            raise ExtractionError(f"ocr failed: {exc}") from exc

    def ocr_pdf(self, data: bytes) -> str:
        try:
            pages = convert_from_bytes(data)
        except Exception as exc:
            raise ExtractionError(f"failed to rasterize pdf for ocr: {exc}") from exc

        texts = [
            str(pytesseract.image_to_string(page, lang=self._lang))
            for page in pages[:MAX_OCR_PAGES]
        ]
        return "\n\n".join(texts)
