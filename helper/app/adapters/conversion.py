"""Реализация services.conversion_service.PdfConverter поверх headless
LibreOffice (системный пакет, см. helper/Dockerfile). Синхронный код —
вызывается через asyncio.to_thread на границе ConversionService."""

import subprocess
import tempfile
from pathlib import Path

from app.domain.errors import ConversionError

# CONVERSION_TIMEOUT_SECONDS — предохранитель от документа, на котором
# LibreOffice зависает: ConvertToPdf — синхронный RPC в пределах одного
# HTTP-запроса Go. Значение — рекомендация для MVP (шаблон в десятки страниц
# конвертируется за единицы секунд), пересмотреть по метрикам прод.
CONVERSION_TIMEOUT_SECONDS = 90

_STDERR_LOG_LIMIT = 500

# _INPUT_FILTERS — входной фильтр LibreOffice по расширению исходника. Без
# явного фильтра LibreOffice угадывает формат сам и молча открывает битый
# файл как простой текст — получается «PDF» с мусором вместо ошибки.
_INPUT_FILTERS: dict[str, str] = {".docx": "MS Word 2007 XML"}


class LibreOfficePdfConverter:
    """Реализация conversion_service.PdfConverter."""

    def __init__(self, binary: str = "soffice") -> None:
        self._binary = binary

    def convert(self, data: bytes, source_suffix: str) -> bytes:
        input_filter = _INPUT_FILTERS.get(source_suffix)
        if input_filter is None:
            raise ConversionError(f"unsupported source format: {source_suffix!r}")

        with tempfile.TemporaryDirectory(prefix="zan-convert-") as workdir:
            work = Path(workdir)
            source = work / f"source{source_suffix}"
            source.write_bytes(data)
            command = [
                self._binary,
                # Свой профиль на каждый вызов: LibreOffice блокирует профиль
                # пользователя, параллельные конвертации с общим профилем
                # падают или зависают.
                f"-env:UserInstallation={(work / 'profile').as_uri()}",
                "--headless",
                "--norestore",
                f"--infilter={input_filter}",
                "--convert-to",
                "pdf",
                "--outdir",
                str(work),
                str(source),
            ]
            try:
                completed = subprocess.run(
                    command,
                    capture_output=True,
                    timeout=CONVERSION_TIMEOUT_SECONDS,
                    check=False,
                )
            except subprocess.TimeoutExpired as exc:
                raise ConversionError(
                    f"conversion timed out after {CONVERSION_TIMEOUT_SECONDS}s"
                ) from exc

            # Код возврата 0 не гарантирует результат: на повреждённом файле
            # LibreOffice завершается успешно, просто не создав PDF.
            output = source.with_suffix(".pdf")
            if completed.returncode != 0 or not output.exists():
                stderr = completed.stderr.decode(errors="replace")[:_STDERR_LOG_LIMIT]
                raise ConversionError(
                    f"conversion failed (exit code {completed.returncode}): {stderr}"
                )
            return output.read_bytes()
