"""Обёртка над self-hosted faster-whisper — за портом
app/services/stt_service.py, чтобы провайдер менялся без переписывания
вызывающего кода (zan-backend-tz-v3.md §8.3, BACKEND_PLAN.md §1.2, §6 п.10).

OCR-фолбэк (tesseract) — в app/adapters/extraction.py, не здесь: он часть
пайплайна извлечения текста (используется тем же ExtractionService, что и
PyMuPDF/python-docx), а не самостоятельный "провайдер" наравне с STT.
"""

import io
from collections.abc import Iterable, Sequence

from faster_whisper import WhisperModel, decode_audio

from app.domain.errors import SttError

# SUPPORTED_LANGUAGES — языки (коды Whisper, ISO 639-1), среди которых
# определяется язык речи. Язык не навязывается из настроек сессии: человек
# может говорить не на языке интерфейса. Но и не выбирается среди всех ~99
# языков Whisper — на коротких фразах он путает казахский с татарским/
# кыргызским, русский с украинским; ограничение набором продукта убирает
# этот класс ошибок. Казахский — "kk", не "kz".
SUPPORTED_LANGUAGES: tuple[str, ...] = ("kk", "ru", "en")


def choose_language(probabilities: Iterable[tuple[str, float]], supported: Sequence[str]) -> str:
    """Самый вероятный язык из supported по вероятностям Whisper
    (WhisperModel.detect_language). Языки вне supported игнорируются; если
    ни один из supported не получил вероятности — первый из supported."""
    best_language, best_probability = supported[0], -1.0
    for language, probability in probabilities:
        if language in supported and probability > best_probability:
            best_language, best_probability = language, probability
    return best_language


class WhisperSttProvider:
    """Реализация stt_service.SttProvider поверх self-hosted faster-whisper.
    Модель грузится один раз в конструкторе (composition root, app.main) —
    первый реальный запрос не платит cold-start модели, а неверный
    WHISPER_MODEL_SIZE/недоступность весов роняет процесс на старте, не на
    случайном пользовательском запросе."""

    def __init__(
        self,
        model_size: str,
        device: str = "cpu",
        compute_type: str = "int8",
        languages: Sequence[str] = SUPPORTED_LANGUAGES,
    ) -> None:
        self._model = WhisperModel(model_size, device=device, compute_type=compute_type)
        self._languages = languages

    def transcribe(self, audio: bytes, lang: str) -> str:
        """lang (язык сессии) не используется: язык определяется по самой
        речи среди self._languages, см. SUPPORTED_LANGUAGES."""
        try:
            # Декодируем один раз — тот же массив нужен и для определения
            # языка, и для распознавания.
            samples = decode_audio(io.BytesIO(audio))
            _, _, probabilities = self._model.detect_language(samples)
            language = choose_language(probabilities, self._languages)
            segments, _info = self._model.transcribe(samples, language=language)
            return "".join(segment.text for segment in segments)
        except Exception as exc:
            raise SttError(f"transcription failed: {exc}") from exc
