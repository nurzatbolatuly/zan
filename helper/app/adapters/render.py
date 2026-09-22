"""Реализации services.render_service.Renderer — DocxTplRenderer (DOCX,
docxtpl) и WeasyPrintRenderer (PDF, HTML/CSS через WeasyPrint) — BACKEND_PLAN.md
§1.2, §3.1. Обе используют один и тот же DocumentDraft (title + sections),
просто два разных механизма рендера, как зафиксировано в стеке.

WeasyPrint импортируется лениво, внутри render() — пакет требует системных
libcairo/libpango (см. helper/Dockerfile), которых нет в части dev/CI
окружений без Docker; модуль в целом обязан оставаться импортируемым (module
import errors тогда ловятся при первом реальном вызове Render, не при
запуске всего процесса — тот же принцип, что и у ExtractionService,
не пробрасывающего import-ошибку из-за одного неиспользуемого пути).
"""

import io
from pathlib import Path

import jinja2
from docxtpl import DocxTemplate

from app.domain.documents import DocumentDraft
from app.domain.errors import RenderError

_TEMPLATE_DIR = Path(__file__).resolve().parent / "templates"

_DOCX_TEMPLATE_PATH = _TEMPLATE_DIR / "document_template.docx"

_PDF_TEMPLATE = jinja2.Template("""
<!DOCTYPE html>
<html lang="ru">
<head>
<meta charset="utf-8">
<style>
  body { font-family: "DejaVu Sans", sans-serif; margin: 3cm 2cm; }
  h1 { font-size: 18pt; }
  h2 { font-size: 13pt; margin-top: 1.2em; }
  p { font-size: 11pt; line-height: 1.4; }
</style>
</head>
<body>
  <h1>{{ title }}</h1>
  {% for s in sections %}
  <h2>{{ s.title }}</h2>
  <p>{{ s.body }}</p>
  {% endfor %}
</body>
</html>
""")


class DocxTplRenderer:
    """Реализация render_service.Renderer для RenderFormat.DOCX."""

    def render(self, draft: DocumentDraft) -> bytes:
        try:
            tpl = DocxTemplate(_DOCX_TEMPLATE_PATH)
            tpl.render(
                {
                    "title": draft.title,
                    "sections": [{"title": s.title, "body": s.body} for s in draft.sections],
                }
            )
            buf = io.BytesIO()
            tpl.save(buf)
            return buf.getvalue()
        except Exception as exc:
            raise RenderError(f"docx render failed: {exc}") from exc


class WeasyPrintRenderer:
    """Реализация render_service.Renderer для RenderFormat.PDF."""

    def render(self, draft: DocumentDraft) -> bytes:
        try:
            from weasyprint import HTML

            html = _PDF_TEMPLATE.render(title=draft.title, sections=draft.sections)
            pdf_bytes: bytes = HTML(string=html).write_pdf()
            return pdf_bytes
        except Exception as exc:
            raise RenderError(f"pdf render failed: {exc}") from exc
