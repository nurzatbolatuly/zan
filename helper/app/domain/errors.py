"""Доменные исключения helper/ (BACKEND_CODING_STANDARDS.md §6.2): каждый
`app/services/*` поднимает один из этих типов на известный, ожидаемый сбой —
`app/grpc/servicers/*` ловит их на границе и мапит на `grpc.StatusCode`
явной таблицей (§8), не пробрасывает `Exception` как есть наружу.
"""


class UntrustedFileURLError(Exception):
    """file_url не указывает на собственный storage-эндпоинт (SSRF-защита,
    BACKEND_PLAN.md §8) — INVALID_ARGUMENT, не ретраится."""


class DownloadError(Exception):
    """Не удалось скачать содержимое по file_url (сеть, 4xx/5xx от storage)."""


class ExtractionError(Exception):
    """FilesService.Extract не смог прочитать файл (повреждён, плохой скан,
    неподдерживаемый MIME) — zan-backend-tz-v3.md §5.3."""


class SttError(Exception):
    """SttService.Transcribe не смог распознать речь (провайдер отказал,
    неподдерживаемый формат) — zan-backend-tz-v3.md §5.4."""


class RenderError(Exception):
    """DocumentsService.Render не смог собрать файл из шаблона."""


class RagUnavailableError(Exception):
    """RagService.Search не может выполниться — схема rag недоступна (пул
    соединений не удалось создать при старте процесса, см. app.main —
    ошибка при бутстрапе не должна ронять весь helper/, он обслуживает ещё
    три несвязанных сервиса). Маппится на INTERNAL (app/grpc/error_mapping.py)
    — Go-сторона трактует любую ошибку RagService.Search как деградацию, не
    блокировку (BACKEND_PLAN.md §5.2: "Go продолжает без источников")."""
