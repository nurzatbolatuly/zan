from dataclasses import dataclass

MIME_PDF = "application/pdf"
MIME_DOCX = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
MIME_PNG = "image/png"
MIME_JPEG = "image/jpeg"


@dataclass(frozen=True)
class ExtractedDocument:
    """Результат извлечения текста из файла (FilesService.Extract,
    Stage 4, BACKEND_PLAN.md)."""

    text: str


@dataclass(frozen=True)
class ConvertedDocument:
    """Результат FilesService.ConvertToPdf — ключ готового PDF в бакете
    (не ссылка: объектом дальше владеет backend/, proto/zan/rpc/v1/files.proto)."""

    object_key: str
