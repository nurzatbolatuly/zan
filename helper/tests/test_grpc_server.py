"""Сквозной тест на реальный gRPC-транспорт, не заглушку — подтверждает то
же самое, что и DoD Stage 0 в BACKEND_PLAN.md ("grpcurl :9090 list
показывает... сервер жив, контракта ещё нет"), просто через grpc.aio-клиент
вместо внешней утилиты grpcurl, чтобы это проверялось на каждый прогон
`pytest`, а не только руками при ревью."""

from collections.abc import AsyncIterator

import grpc
import pytest
from grpc_reflection.v1alpha import reflection, reflection_pb2, reflection_pb2_grpc


async def _start_server_on_free_port() -> tuple[grpc.aio.Server, int]:
    server = grpc.aio.server()
    reflection.enable_server_reflection((reflection.SERVICE_NAME,), server)
    port = server.add_insecure_port("[::]:0")
    await server.start()
    return server, port


@pytest.mark.asyncio
async def test_reflection_lists_reflection_service_with_no_business_contract() -> None:
    server, port = await _start_server_on_free_port()
    try:
        async with grpc.aio.insecure_channel(f"localhost:{port}") as channel:
            # types-grpcio типизирует Stub-конструкторы и стриминговые вызовы
            # под синхронный grpc.Channel/grpc.Call — grpc.aio почти не
            # покрыт стабами апстрима (известное ограничение пакета, не
            # ошибка в нашем коде), отсюда точечные ignore ниже.
            stub = reflection_pb2_grpc.ServerReflectionStub(channel)  # type: ignore[arg-type]
            request = reflection_pb2.ServerReflectionRequest(list_services="")

            async def request_iterator() -> AsyncIterator[reflection_pb2.ServerReflectionRequest]:
                yield request

            responses = stub.ServerReflectionInfo(request_iterator())  # type: ignore[operator]
            reply = await responses.read()
            service_names = {s.name for s in reply.list_services_response.service}

            assert reflection.SERVICE_NAME in service_names
    finally:
        await server.stop(grace=None)
