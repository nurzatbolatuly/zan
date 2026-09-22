"""Рендер документа из шаблона (DocumentsService.Render, BACKEND_PLAN.md
§3) — всегда возвращает file_url, не бинарь (§3.1). Порт Renderer
объявлен здесь, реализации (docxtpl/WeasyPrint) — в app/adapters.
"""

import asyncio
from typing import Protocol
from uuid import uuid4

from app.domain.documents import DocumentDraft, RenderedDocument, RenderFormat

_CONTENT_TYPES: dict[RenderFormat, str] = {
    RenderFormat.PDF: "application/pdf",
    RenderFormat.DOCX: "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
}


class Renderer(Protocol):
    """Порт рендера одного конкретного формата — RenderService держит по
    одному Renderer на каждый RenderFormat (DocxTplRenderer/WeasyPrintRenderer,
    app/adapters/render.py), не один универсальный объект с if/else внутри."""

    def render(self, draft: DocumentDraft) -> bytes: ...


class Uploader(Protocol):
    """Порт загрузки готового файла (app.adapters.storage.S3Storage)."""

    async def upload_generated(
        self, key: str, data: bytes, content_type: str
    ) -> tuple[str, str]: ...


class RenderService:
    def __init__(self, renderers: dict[RenderFormat, Renderer], uploader: Uploader) -> None:
        self._renderers = renderers
        self._uploader = uploader

    async def render(self, draft: DocumentDraft, fmt: RenderFormat) -> RenderedDocument:
        renderer = self._renderers[fmt]
        data = await asyncio.to_thread(renderer.render, draft)

        key = f"generated/{uuid4()}.{fmt.value}"
        file_url, object_key = await self._uploader.upload_generated(key, data, _CONTENT_TYPES[fmt])
        return RenderedDocument(file_url=file_url, object_key=object_key)
