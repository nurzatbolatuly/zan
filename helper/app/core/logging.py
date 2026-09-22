"""Единственная точка настройки логирования в helper/
(BACKEND_CODING_STANDARDS.md §4.1). Формат записи и уровни — тот же
контракт, что и у backend/ (backend-roadmap.md §6.1/§6.4):

    {
      "timestamp": "...",
      "level": "INFO|WARN|ERROR|CRITICAL|DEBUG",
      "service": "python-agent",
      "trace_id": "...",
      "message": "...",
      "context": { ... }
    }

Домен-специфичные поля передаются через именованный аргумент ``context``
(словарь) при вызове — так они лягут вложенным объектом "context", как и
``slog.Group("context", ...)`` на стороне Go (см. internal/platform/logger
в backend/), а не расползутся по верхнему уровню записи:

    logger.info("rag_search_completed", context={"matches_count": 3})

``trace_id``/``session_id`` — не статические поля, а contextvars
(structlog.contextvars), которые серверный gRPC-интерцептор биндит на вход
каждого запроса (появляется вместе с первым реальным RPC, Stage 4/5) — на
Stage 0 их взять неоткуда, подставлять пустые значения было бы ложью в
логах.
"""

import logging
import sys
from typing import cast

import structlog

SERVICE_NAME = "python-agent"

_LEVEL_RENAMES = {
    "warning": "WARN",
    "critical": "CRITICAL",
}


def _normalize_level(
    _logger: object, _method_name: str, event_dict: structlog.types.EventDict
) -> structlog.types.EventDict:
    """Контракт §6.4 требует ровно "WARN", не "WARNING" (умолчание Python
    logging/structlog) — без этого шага уровень WARNING не совпадал бы с
    зафиксированным в backend-roadmap.md enum."""
    level = event_dict.get("level")
    if isinstance(level, str):
        event_dict["level"] = _LEVEL_RENAMES.get(level, level.upper())
    return event_dict


def configure_logging(level: str = "info") -> None:
    numeric_level = logging.getLevelNamesMapping().get(level.upper(), logging.INFO)

    structlog.configure(
        processors=[
            structlog.contextvars.merge_contextvars,
            structlog.processors.add_log_level,
            _normalize_level,
            structlog.processors.TimeStamper(fmt="iso", key="timestamp"),
            # format_exc_info — рендерит exc_info=True в текстовый traceback
            # (Go-эквивалент: httpserver.Recovery пишет "CRITICAL с полным
            # стеком" на панику, BACKEND_CODING_STANDARDS.md §4.3) — единая
            # точка, вызывающий код (app/grpc/error_mapping.py) не форматирует
            # traceback вручную.
            structlog.processors.format_exc_info,
            structlog.processors.EventRenamer(to="message"),
            structlog.processors.JSONRenderer(),
        ],
        wrapper_class=structlog.make_filtering_bound_logger(numeric_level),
        context_class=dict,
        logger_factory=structlog.PrintLoggerFactory(sys.stdout),
        cache_logger_on_first_use=True,
    )


def get_logger() -> structlog.typing.FilteringBoundLogger:
    # structlog.get_logger()/.bind() возвращают Any в апстримных тайп-хинтах
    # (structlog сам определяет реальный тип лениво, через configure()) —
    # cast, а не игнор, чтобы не потерять проверку сигнатуры вызывающего кода.
    logger = cast("structlog.typing.FilteringBoundLogger", structlog.get_logger())
    return logger.bind(service=SERVICE_NAME)
