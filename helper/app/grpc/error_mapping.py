"""Единая таблица маппинга доменных исключений (app/domain/errors.py) на
grpc.StatusCode — BACKEND_CODING_STANDARDS.md §8: "ни одно исключение из
app/services/app/adapters не долетает до клиента как есть", реализуется один
раз здесь, не в каждом сервисере отдельно. Коды — BACKEND_PLAN.md §3.3:
INVALID_ARGUMENT (невалидный/неотрабатываемый вход — Go не ретраит),
INTERNAL (непредвиденное — Go логирует ERROR, полный traceback только здесь).
"""

import grpc

from app.core.logging import get_logger
from app.domain.errors import (
    ConversionError,
    DownloadError,
    ExtractionError,
    RenderError,
    SttError,
    UntrustedFileURLError,
)

# _INVALID_ARGUMENT_ERRORS — контент/вход, который в принципе не обработать
# повторной попыткой того же запроса (BACKEND_PLAN.md §3.3: "невалидный
# запрос — не ретраится"). UntrustedFileURLError — отдельная ветка ниже,
# логируется как security-инцидент, не просто WARN.
_INVALID_ARGUMENT_ERRORS: tuple[type[Exception], ...] = (
    ExtractionError,
    SttError,
    RenderError,
    ConversionError,
)


async def abort_for_exception(
    context: "grpc.aio.ServicerContext[object, object]", exc: Exception, *, rpc: str
) -> None:
    """Абортит RPC кодом, соответствующим типу exc. context.abort() сам
    поднимает исключение (не возвращает управление) — вызывающий сервисер
    может звать эту функцию и сразу считать RPC завершённым."""
    logger = get_logger()

    if isinstance(exc, UntrustedFileURLError):
        # SSRF-попытка (BACKEND_PLAN.md §8) — не просто "плохой запрос",
        # достойно отдельного события для последующего разбора инцидента.
        logger.warning("ssrf_attempt_blocked", context={"rpc": rpc, "error": str(exc)})
        await context.abort(grpc.StatusCode.INVALID_ARGUMENT, str(exc))
        return

    if isinstance(exc, _INVALID_ARGUMENT_ERRORS):
        logger.warning(f"{rpc}_rejected", context={"error": str(exc)})
        await context.abort(grpc.StatusCode.INVALID_ARGUMENT, str(exc))
        return

    if isinstance(exc, DownloadError):
        # Собственный storage недоступен — инфраструктурный сбой на стороне
        # Python, не проблема запроса (BACKEND_PLAN.md §3.3: "INTERNAL —
        # необработанный сбой — логируется как ERROR"; повторные INTERNAL
        # всё равно копятся в circuit breaker на стороне Go, resilience.Breaker
        # считает неудачей любую ошибку, не только UNAVAILABLE).
        logger.error(f"{rpc}_storage_unavailable", context={"error": str(exc)})
        await context.abort(grpc.StatusCode.INTERNAL, "storage unavailable")
        return

    logger.critical(f"{rpc}_panicked", exc_info=True, context={"error": str(exc)})
    await context.abort(grpc.StatusCode.INTERNAL, "internal error")
