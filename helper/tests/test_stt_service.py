from app.domain.stt import Transcript
from app.services.stt_service import SttService


class FakeDownloader:
    def __init__(self, content: bytes = b"audio bytes") -> None:
        self.content = content
        self.requested: list[str] = []

    async def download(self, file_url: str) -> bytes:
        self.requested.append(file_url)
        return self.content


class FakeSttProvider:
    def __init__(self, text: str = "") -> None:
        self.text = text
        self.calls: list[tuple[bytes, str]] = []

    def transcribe(self, audio: bytes, lang: str) -> str:
        self.calls.append((audio, lang))
        return self.text


async def test_transcribe_returns_trimmed_text() -> None:
    provider = FakeSttProvider(text="  привет мир  ")
    svc = SttService(FakeDownloader(), provider)

    result = await svc.transcribe("https://storage.internal/a.webm", "ru")

    assert result == Transcript(text="привет мир")
    assert provider.calls == [(b"audio bytes", "ru")]


async def test_transcribe_returns_empty_transcript_on_silence() -> None:
    svc = SttService(FakeDownloader(), FakeSttProvider(text=""))

    result = await svc.transcribe("https://storage.internal/silence.webm", "kz")

    assert result == Transcript(text="")


async def test_transcribe_downloads_from_given_url() -> None:
    downloader = FakeDownloader()
    svc = SttService(downloader, FakeSttProvider(text="x"))

    await svc.transcribe("https://storage.internal/specific.webm", "ru")

    assert downloader.requested == ["https://storage.internal/specific.webm"]
