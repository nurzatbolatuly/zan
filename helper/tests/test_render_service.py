import pytest

from app.domain.documents import DocumentDraft, DocumentSection, RenderedDocument, RenderFormat
from app.domain.errors import RenderError
from app.services.render_service import RenderService


class FakeRenderer:
    def __init__(self, data: bytes = b"rendered", raises: Exception | None = None) -> None:
        self.data = data
        self.raises = raises
        self.drafts: list[DocumentDraft] = []

    def render(self, draft: DocumentDraft) -> bytes:
        self.drafts.append(draft)
        if self.raises:
            raise self.raises
        return self.data


class FakeUploader:
    def __init__(self) -> None:
        self.uploads: list[tuple[str, bytes, str]] = []

    async def upload_generated(self, key: str, data: bytes, content_type: str) -> tuple[str, str]:
        self.uploads.append((key, data, content_type))
        return f"https://storage.public/{key}", key


def sample_draft() -> DocumentDraft:
    return DocumentDraft(
        title="Заявление", sections=[DocumentSection(title="Пункт 1", body="Текст")]
    )


async def test_render_pdf_uses_pdf_renderer_and_uploads() -> None:
    pdf_renderer = FakeRenderer(data=b"%PDF-1.4...")
    docx_renderer = FakeRenderer()
    uploader = FakeUploader()
    svc = RenderService(
        {RenderFormat.PDF: pdf_renderer, RenderFormat.DOCX: docx_renderer}, uploader
    )

    result = await svc.render(sample_draft(), RenderFormat.PDF)

    assert isinstance(result, RenderedDocument)
    assert result.file_url == "https://storage.public/" + result.object_key
    assert result.object_key.startswith("generated/")
    assert result.object_key.endswith(".pdf")
    assert len(pdf_renderer.drafts) == 1
    assert len(docx_renderer.drafts) == 0

    key, data, content_type = uploader.uploads[0]
    assert data == b"%PDF-1.4..."
    assert content_type == "application/pdf"
    assert key == result.object_key


async def test_render_docx_uses_docx_renderer() -> None:
    pdf_renderer = FakeRenderer()
    docx_renderer = FakeRenderer(data=b"PK\x03\x04docx bytes")
    uploader = FakeUploader()
    svc = RenderService(
        {RenderFormat.PDF: pdf_renderer, RenderFormat.DOCX: docx_renderer}, uploader
    )

    result = await svc.render(sample_draft(), RenderFormat.DOCX)

    assert result.object_key.endswith(".docx")
    assert len(docx_renderer.drafts) == 1
    assert uploader.uploads[0][2].endswith("wordprocessingml.document")


async def test_render_propagates_renderer_failure() -> None:
    renderer = FakeRenderer(raises=RenderError("broken template"))
    uploader = FakeUploader()
    svc = RenderService({RenderFormat.PDF: renderer, RenderFormat.DOCX: renderer}, uploader)

    with pytest.raises(RenderError):
        await svc.render(sample_draft(), RenderFormat.PDF)

    assert uploader.uploads == []
