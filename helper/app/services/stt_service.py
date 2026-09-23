"""Speech-to-text (SttService.Transcribe, BACKEND_PLAN.md §3). Порт
SttProvider — Strategy-паттерн для self-hosted faster-whisper или внешнего
API (zan-backend-tz-v3.md §8.3, открытый вопрос продукта не закрыт).
"""

import asyncio
from typing import Protocol

from app.domain.stt import Transcript


class Downloader(Protocol):
    """Порт скачивания файла по ссылке (app.adapters.storage.S3Storage)."""

    async def download(self, file_url: str) -> bytes: ...


class SttProvider(Protocol):
    """Порт распознавания речи. lang — язык сессии ("ru"/"kz"/пусто), только
    подсказка: реализация вправе определять язык по самой речи и игнорировать
    её (так делает WhisperSttProvider). Поднимает SttError на непригодный для
    распознавания вход (BACKEND_CODING_STANDARDS.md §6.2) — пустой результат
    (тишина) не ошибка, см. app.domain.stt.Transcript."""

    def transcribe(self, audio: bytes, lang: str) -> str: ...


class SttService:
    def __init__(self, downloader: Downloader, provider: SttProvider) -> None:
        self._downloader = downloader
        self._provider = provider

    async def transcribe(self, file_url: str, lang: str) -> Transcript:
        audio = await self._downloader.download(file_url)
        text = await asyncio.to_thread(self._provider.transcribe, audio, lang)
        return Transcript(text=text.strip())
