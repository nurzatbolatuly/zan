"""Серверный gRPC-interceptor — единая точка сквозных забот (BACKEND_CODING_STANDARDS.md
§7): проверка x-internal-secret (BACKEND_PLAN.md §1 — "shared secret, вторая
линия обороны"), привязка trace_id/session_id в structlog.contextvars (§4.1),
логирование старта/итога каждого входящего вызова (§4.2) — сервисеры сами
этого не делают.
"""

# grpc.RpcMethodHandler — обычный (не Generic) класс в рантайме grpcio,
# субскрипт RpcMethodHandler[Any, Any] ниже существует только для types-grpcio
# на этапе статической проверки. Python 3.12 (в отличие от 3.14+, где PEP 649
# отложенной оценки аннотаций стал умолчанием) вычисляет аннотации сигнатур
# eagerly при определении класса — без этого импорта весь модуль падал бы
# `TypeError: type 'RpcMethodHandler' is not subscriptable` при старте
# процесса на python:3.12-slim (обнаружено при первом запуске Docker-образа,
# Stage 4 — прошло мимо локального pytest именно из-за разницы версий Python).
from __future__ import annotations

import hmac
import time
from collections.abc import Awaitable, Callable
from typing import Any

import grpc
import structlog

from app.core.logging import get_logger

_INTERNAL_SECRET_KEY = "x-internal-secret"
_TRACE_ID_KEY = "x-trace-id"


class AuthAndLoggingInterceptor(grpc.aio.ServerInterceptor):
    def __init__(self, internal_secret: str) -> None:
        self._internal_secret = internal_secret

    async def intercept_service(
        self,
        continuation: Callable[
            [grpc.HandlerCallDetails], Awaitable[grpc.RpcMethodHandler[Any, Any] | None]
        ],
        handler_call_details: grpc.HandlerCallDetails,
    ) -> grpc.RpcMethodHandler[Any, Any] | None:
        handler = await continuation(handler_call_details)
        if handler is None or not handler.unary_unary:
            # Stage 4/5 — только unary-unary RPC во всём контракте
            # (BACKEND_PLAN.md §3.1: "стриминг — только если появится
            # измеримая причина, не заводится про запас").
            return handler

        method = handler_call_details.method
        # types-grpcio типизирует unary_unary под синхронный
        # grpc.ServicerContext, не grpc.aio — известное ограничение пакета
        # (см. tests/test_grpc_server.py, тот же комментарий про
        # ServerReflectionStub) — cast, не ignore на весь модуль.
        inner: Callable[[Any, Any], Awaitable[Any]] = handler.unary_unary

        async def wrapped(request: object, context: grpc.aio.ServicerContext[Any, Any]) -> object:
            metadata = dict(context.invocation_metadata() or [])
            got_secret = metadata.get(_INTERNAL_SECRET_KEY, "")
            if not isinstance(got_secret, str) or not hmac.compare_digest(
                got_secret, self._internal_secret
            ):
                await context.abort(grpc.StatusCode.UNAUTHENTICATED, "invalid internal secret")
                return None

            token = structlog.contextvars.bind_contextvars(trace_id=metadata.get(_TRACE_ID_KEY, ""))
            logger = get_logger()
            started = time.monotonic()
            logger.info("grpc_call_started", context={"method": method})
            try:
                response = await inner(request, context)
            except grpc.aio.AbortError:
                # Сервисер/эта же функция уже вызвали context.abort() —
                # решение и код статуса приняты там, здесь просто фиксируем
                # итог по факту (WARN — ожидаемый бизнес-исход, не баг).
                logger.warning(
                    "grpc_call_aborted",
                    context={
                        "method": method,
                        "code": context.code().name if context.code() else None,
                        "duration_ms": int((time.monotonic() - started) * 1000),
                    },
                )
                raise
            except Exception as exc:
                # Защитная сетка на случай, если что-то прорвалось мимо
                # собственного try/except сервисера (app/grpc/error_mapping.py)
                # — тот же принцип, что httpserver.Recovery на стороне Go.
                logger.critical(
                    "grpc_call_panicked",
                    exc_info=True,
                    context={
                        "method": method,
                        "duration_ms": int((time.monotonic() - started) * 1000),
                    },
                )
                await context.abort(grpc.StatusCode.INTERNAL, "internal error")
                raise AssertionError("unreachable — context.abort() raises") from exc
            else:
                logger.info(
                    "grpc_call_succeeded",
                    context={
                        "method": method,
                        "duration_ms": int((time.monotonic() - started) * 1000),
                    },
                )
                return response
            finally:
                structlog.contextvars.reset_contextvars(**token)

        return grpc.unary_unary_rpc_method_handler(
            wrapped,
            request_deserializer=handler.request_deserializer,
            response_serializer=handler.response_serializer,
        )
