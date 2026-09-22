from dataclasses import dataclass
from enum import StrEnum


class RenderFormat(StrEnum):
    """Зеркалит zanv1.RenderFormat (proto/zan/rpc/v1/documents.proto) —
    домен не импортирует сгенерированный код (BACKEND_CODING_STANDARDS.md
    §1.2), конвертация происходит на границе app/grpc/servicers/documents.py."""

    PDF = "pdf"
    DOCX = "docx"


@dataclass(frozen=True)
class DocumentSection:
    """Одна структурированная находка (та же форма, что и domain.Finding
    в Go — zan-backend-tz-v2.md §2.7: `[{title, body}]`)."""

    title: str
    body: str


@dataclass(frozen=True)
class DocumentDraft:
    """Вход DocumentsService.Render — заголовок + список секций, из которых
    собирается DOCX (docxtpl) или PDF (WeasyPrint)."""

    title: str
    sections: list[DocumentSection]


@dataclass(frozen=True)
class RenderedDocument:
    """Результат рендера документа — всегда file_url, не бинарь
    (proto/zan/rpc/v1/documents.proto, §3.1). object_key — стабильная
    ссылка для того, кто должен пережить истечение presigned file_url и
    перевыпустить свежую позже (Stage 6)."""

    file_url: str
    object_key: str
