"""LibreOfficePdfConverter на реальном LibreOffice — системный пакет есть в
Docker-образе helper/ (Dockerfile), локально тест пропускается, если soffice
не установлен."""

import io
import shutil

import docx
import pytest

from app.adapters.conversion import LibreOfficePdfConverter
from app.domain.errors import ConversionError

requires_soffice = pytest.mark.skipif(
    shutil.which("soffice") is None, reason="LibreOffice not installed"
)


def _make_docx_bytes(text: str) -> bytes:
    document = docx.Document()
    document.add_paragraph(text)
    buf = io.BytesIO()
    document.save(buf)
    return buf.getvalue()


@requires_soffice
def test_converts_docx_to_pdf() -> None:
    pdf = LibreOfficePdfConverter().convert(_make_docx_bytes("Договор аренды"), ".docx")

    assert pdf.startswith(b"%PDF")


@requires_soffice
@pytest.mark.parametrize("data", [b"not a docx", b"PK\x03\x04broken zip"])
def test_corrupted_docx_raises_conversion_error(data: bytes) -> None:
    # Без явного входного фильтра LibreOffice открыл бы это как текст и
    # вернул «PDF» с мусором.
    with pytest.raises(ConversionError):
        LibreOfficePdfConverter().convert(data, ".docx")


def test_unsupported_suffix_raises_conversion_error() -> None:
    with pytest.raises(ConversionError):
        LibreOfficePdfConverter().convert(b"data", ".odt")
