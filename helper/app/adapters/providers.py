"""Обёртка над self-hosted faster-whisper — за портом
app/services/stt_service.py, чтобы провайдер менялся без переписывания
вызывающего кода (zan-backend-tz-v3.md §8.3, BACKEND_PLAN.md §1.2, §6 п.10).

OCR-фолбэк (tesseract) — в app/adapters/extraction.py, не здесь: он часть
пайплайна извлечения текста (используется тем же ExtractionService, что и
PyMuPDF/python-docx), а не самостоятельный "провайдер" наравне с STT.
"""

import io

from faster_whisper import WhisperModel

from app.domain.errors import SttError

# _LANG_HINTS — Lang (common.proto) -> код языка faster-whisper/Whisper.
# Пусто/неизвестное значение -> None (faster-whisper определяет язык сам).
_LANG_HINTS = {"ru": "ru", "kz": "kk"}  # ISO 639-1 казахского — "kk", не "kz"


class WhisperSttProvider:
    """Реализация stt_service.SttProvider поверх self-hosted faster-whisper.
    Модель грузится один раз в конструкторе (composition root, app.main) —
    первый реальный запрос не платит cold-start модели, а неверный
    WHISPER_MODEL_SIZE/недоступность весов роняет процесс на старте, не на
    случайном пользовательском запросе."""

    def __init__(self, model_size: str, device: str = "cpu", compute_type: str = "int8") -> None:
        self._model = WhisperModel(model_size, device=device, compute_type=compute_type)

    def transcribe(self, audio: bytes, lang: str) -> str:
        language = _LANG_HINTS.get(lang)
        try:
            segments, _info = self._model.transcribe(io.BytesIO(audio), language=language)
            return "".join(segment.text for segment in segments)
        except Exception as exc:
            raise SttError(f"transcription failed: {exc}") from exc
