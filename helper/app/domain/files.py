from dataclasses import dataclass


@dataclass(frozen=True)
class ExtractedDocument:
    """Результат извлечения текста из файла (FilesService.Extract,
    Stage 4, BACKEND_PLAN.md)."""

    text: str
