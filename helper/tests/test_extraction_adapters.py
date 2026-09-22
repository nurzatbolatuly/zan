"""Реальные PyMuPDF/python-docx на настоящих файлах — не моки библиотек
(BACKEND_CODING_STANDARDS.md §10: поведение, не форма вызова). Tesseract/
poppler (TesseractOcrProvider) требуют системных бинарей, которых нет в
локальном dev-окружении без Docker — проверяются в docker compose (см.
scripts/smoke-test.sh), не здесь."""

import io

import docx
import pymupdf
import pytest

from app.adapters.extraction import PyMuPdfExtractor, PythonDocxExtractor
from app.domain.errors import ExtractionError


def make_pdf_bytes(text: str) -> bytes:
    doc = pymupdf.open()
    page = doc.new_page()
    page.insert_text((72, 72), text)
    data: bytes = doc.tobytes()
    doc.close()
    return data


def make_docx_bytes(paragraphs: list[str]) -> bytes:
    document = docx.Document()
    for p in paragraphs:
        document.add_paragraph(p)
    buf = io.BytesIO()
    document.save(buf)
    return buf.getvalue()


def test_pymupdf_extractor_reads_text_layer() -> None:
    # ASCII, не кириллица: базовые PDF-шрифты (Helvetica и т.п., которые
    # insert_text использует по умолчанию без явного встраивания шрифта) не
    # несут кириллических глифов — это ограничение тестовой фикстуры, не
    # экстрактора (реальные PDF от пользователей всегда несут свои шрифты).
    pdf_bytes = make_pdf_bytes("Rental agreement No. 42")

    text = PyMuPdfExtractor().extract(pdf_bytes)

    assert "Rental agreement" in text


def test_pymupdf_extractor_returns_empty_string_for_blank_page() -> None:
    doc = pymupdf.open()
    doc.new_page()
    blank_pdf = doc.tobytes()
    doc.close()

    text = PyMuPdfExtractor().extract(blank_pdf)

    assert text.strip() == ""


def test_pymupdf_extractor_raises_on_corrupted_file() -> None:
    with pytest.raises(ExtractionError):
        PyMuPdfExtractor().extract(b"this is not a pdf")


def test_python_docx_extractor_reads_paragraphs() -> None:
    docx_bytes = make_docx_bytes(["Пункт 1: стороны", "Пункт 2: сроки"])

    text = PythonDocxExtractor().extract(docx_bytes)

    assert "Пункт 1: стороны" in text
    assert "Пункт 2: сроки" in text


def test_python_docx_extractor_raises_on_corrupted_file() -> None:
    with pytest.raises(ExtractionError):
        PythonDocxExtractor().extract(b"this is not a docx")
