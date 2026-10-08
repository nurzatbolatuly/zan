"""PDF-копия документа для просмотра (FilesService.ConvertToPdf): backend/
хранит DOCX-шаблоны документов и показывает их в админке как PDF без
возможности правки. Порт PdfConverter объявлен здесь, реализация
(headless LibreOffice) — в app/adapters/conversion.py.
"""

import asyncio
from typing import Protocol
from uuid import uuid4

from app.domain.errors import ConversionError
from app.domain.files import MIME_DOCX, MIME_PDF, ConvertedDocument

# _SOURCE_SUFFIXES — поддерживаемые входные MIME и расширение, по которому
# конвертер определяет формат исходника (LibreOffice смотрит на суффикс файла).
_SOURCE_SUFFIXES: dict[str, str] = {MIME_DOCX: ".docx"}


class Downloader(Protocol):
    """Порт скачивания файла по ссылке (app.adapters.storage.S3Storage)."""

    async def download(self, file_url: str) -> bytes: ...


class PdfConverter(Protocol):
    """Порт конвертации в PDF. Поднимает ConversionError, если исходник не
    удалось преобразовать (повреждён, не уложился в таймаут)."""

    def convert(self, data: bytes, source_suffix: str) -> bytes: ...


class Uploader(Protocol):
    """Порт загрузки готового файла (app.adapters.storage.S3Storage)."""

    async def upload_generated(
        self, key: str, data: bytes, content_type: str
    ) -> tuple[str, str]: ...


class ConversionService:
    def __init__(self, downloader: Downloader, converter: PdfConverter, uploader: Uploader) -> None:
        self._downloader = downloader
        self._converter = converter
        self._uploader = uploader

    async def convert_to_pdf(self, file_url: str, mime_type: str) -> ConvertedDocument:
        suffix = _SOURCE_SUFFIXES.get(mime_type)
        if suffix is None:
            raise ConversionError(f"unsupported mime type: {mime_type!r}")

        data = await self._downloader.download(file_url)
        pdf = await asyncio.to_thread(self._converter.convert, data, suffix)

        _, object_key = await self._uploader.upload_generated(
            f"converted/{uuid4()}.pdf", pdf, MIME_PDF
        )
        return ConvertedDocument(object_key=object_key)
