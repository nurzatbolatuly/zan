"""Извлечение текста из файлов (FilesService.Extract, BACKEND_PLAN.md §3).
Порты объявлены здесь, реализации (PyMuPDF/python-docx/OCR-фолбэк)
— в app/adapters (BACKEND_CODING_STANDARDS.md §1.2).
"""

import asyncio
from typing import Protocol

from app.domain.errors import ExtractionError
from app.domain.files import ExtractedDocument

MIME_PDF = "application/pdf"
MIME_DOCX = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
MIME_PNG = "image/png"
MIME_JPEG = "image/jpeg"

# SUPPORTED_MIME_TYPES — зеркалит file.extractableMimeTypes на стороне Go
# (backend/internal/service/file/file.go) — загрузить можно файл любого типа,
# но Go не вызывает Extract для остальных; это проверка defense-in-depth,
# не первичная линия.
SUPPORTED_MIME_TYPES = frozenset({MIME_PDF, MIME_DOCX, MIME_PNG, MIME_JPEG})


class Downloader(Protocol):
    """Порт скачивания файла по ссылке (app.adapters.storage.S3Storage)."""

    async def download(self, file_url: str) -> bytes: ...


class PdfTextExtractor(Protocol):
    """Извлечение текстового слоя PDF (PyMuPDF). Пустая строка — валидный
    результат для сканов без текстового слоя, вызывающий код (ExtractionService)
    сам решает идти ли в OCR-фолбэк."""

    def extract(self, data: bytes) -> str: ...


class DocxTextExtractor(Protocol):
    """Извлечение текста DOCX (python-docx)."""

    def extract(self, data: bytes) -> str: ...


class OcrProvider(Protocol):
    """OCR-фолбэк для сканов/изображений (tesseract). Поднимает
    ExtractionError, если файл повреждён/не читается — не возвращает пустую
    строку молча (zan-backend-tz-v3.md §5.3: "не смог прочитать файл")."""

    def ocr_image(self, data: bytes) -> str: ...
    def ocr_pdf(self, data: bytes) -> str: ...


class ExtractionService:
    def __init__(
        self,
        downloader: Downloader,
        pdf_extractor: PdfTextExtractor,
        docx_extractor: DocxTextExtractor,
        ocr: OcrProvider,
    ) -> None:
        self._downloader = downloader
        self._pdf_extractor = pdf_extractor
        self._docx_extractor = docx_extractor
        self._ocr = ocr

    async def extract(self, file_url: str, mime_type: str) -> ExtractedDocument:
        if mime_type not in SUPPORTED_MIME_TYPES:
            raise ExtractionError(f"unsupported mime type: {mime_type!r}")

        data = await self._downloader.download(file_url)

        text = await self._extract_by_mime_type(data, mime_type)
        if not text.strip():
            raise ExtractionError("extraction produced no text (corrupted file or blank scan)")
        return ExtractedDocument(text=text.strip())

    async def _extract_by_mime_type(self, data: bytes, mime_type: str) -> str:
        if mime_type == MIME_PDF:
            text = await asyncio.to_thread(self._pdf_extractor.extract, data)
            if text.strip():
                return text
            # Скан без текстового слоя — OCR-фолбэк (zan-backend-tz-v3.md §5.3).
            return await asyncio.to_thread(self._ocr.ocr_pdf, data)
        if mime_type == MIME_DOCX:
            return await asyncio.to_thread(self._docx_extractor.extract, data)
        # MIME_PNG/MIME_JPEG — единственные оставшиеся варианты в SUPPORTED_MIME_TYPES.
        return await asyncio.to_thread(self._ocr.ocr_image, data)
