"""Точка входа helper/ (composition root, BACKEND_CODING_STANDARDS.md §1.2):
поднимает gRPC-сервер (бизнес-контракт Go -> Python, BACKEND_PLAN.md §3) и
рядом — тонкий HTTP только под /healthz (для docker-compose/k8s liveness,
см. BACKEND_PLAN.md §1.2 — "gRPC health-checking protocol избыточен для
этого масштаба"). Оба — в одном процессе, одним event loop.
"""

import asyncio
import signal

import asyncpg
import grpc
import uvicorn
from grpc_reflection.v1alpha import reflection
from starlette.applications import Starlette
from starlette.requests import Request
from starlette.responses import JSONResponse
from starlette.routing import Route

# side effect: sys.path — должен идти раньше любого импорта из zan.*
from app import bootstrap_genproto  # noqa: F401
from app.adapters.db import PgVectorStore
from app.adapters.embeddings import FastEmbedProvider
from app.adapters.extraction import PyMuPdfExtractor, PythonDocxExtractor, TesseractOcrProvider
from app.adapters.providers import WhisperSttProvider
from app.adapters.render import DocxTplRenderer, WeasyPrintRenderer
from app.adapters.storage import HttpxGetter, S3Storage
from app.core.config import Settings, load_settings
from app.core.logging import configure_logging, get_logger
from app.domain.documents import RenderFormat
from app.grpc.interceptors import AuthAndLoggingInterceptor
from app.grpc.servicers.documents import DocumentsServicer
from app.grpc.servicers.files import FilesServicer
from app.grpc.servicers.rag import RagServicer
from app.grpc.servicers.stt import SttServicer
from app.services.extraction_service import ExtractionService
from app.services.rag_service import RagService
from app.services.render_service import RenderService
from app.services.stt_service import SttService
from zan.rpc.v1 import (
    documents_pb2,
    documents_pb2_grpc,
    files_pb2,
    files_pb2_grpc,
    rag_pb2,
    rag_pb2_grpc,
    stt_pb2,
    stt_pb2_grpc,
)

# MAX_MESSAGE_SIZE — тот же лимит и то же обоснование, что
# backend/internal/grpcclient.maxMessageSize (Go): дефолт grpc (4 МБ) мал для
# FilesService.Extract на большом PDF/скане, BACKEND_PLAN.md §8.
MAX_MESSAGE_SIZE = 16 * 1024 * 1024


async def _healthz(_request: Request) -> JSONResponse:
    return JSONResponse({"status": "ok"})


def build_http_app() -> Starlette:
    return Starlette(routes=[Route("/healthz", _healthz, methods=["GET"])])


async def _build_grpc_server(settings: Settings, rag_pool: asyncpg.Pool | None) -> grpc.aio.Server:
    storage = S3Storage(
        endpoint=settings.s3_endpoint,
        public_endpoint=settings.resolved_s3_public_endpoint(),
        region=settings.s3_region,
        access_key=settings.s3_access_key,
        secret_key=settings.s3_secret_key,
        bucket=settings.s3_bucket,
        http=HttpxGetter(),
    )
    await storage.ensure_bucket()

    extraction_service = ExtractionService(
        storage, PyMuPdfExtractor(), PythonDocxExtractor(), TesseractOcrProvider()
    )
    stt_service = SttService(storage, WhisperSttProvider(settings.whisper_model_size))
    render_service = RenderService(
        {RenderFormat.DOCX: DocxTplRenderer(), RenderFormat.PDF: WeasyPrintRenderer()}, storage
    )
    # FastEmbedProvider — self-hosted embedding model, грузится здесь (не в
    # RagService) — тот же принцип, что WhisperSttProvider(...) строится в
    # composition root, не внутри SttService (BACKEND_PLAN.md §6 п.10).
    rag_service = RagService(FastEmbedProvider(settings.embedding_model), PgVectorStore(rag_pool))

    server = grpc.aio.server(
        interceptors=[AuthAndLoggingInterceptor(settings.internal_secret)],
        options=[
            ("grpc.max_send_message_length", MAX_MESSAGE_SIZE),
            ("grpc.max_receive_message_length", MAX_MESSAGE_SIZE),
        ],
    )
    files_pb2_grpc.add_FilesServiceServicer_to_server(FilesServicer(extraction_service), server)
    stt_pb2_grpc.add_SttServiceServicer_to_server(SttServicer(stt_service), server)
    documents_pb2_grpc.add_DocumentsServiceServicer_to_server(
        DocumentsServicer(render_service), server
    )
    rag_pb2_grpc.add_RagServiceServicer_to_server(
        RagServicer(rag_service, settings.rag_search_top_k_default), server
    )

    reflection.enable_server_reflection(
        (
            files_pb2.DESCRIPTOR.services_by_name["FilesService"].full_name,
            stt_pb2.DESCRIPTOR.services_by_name["SttService"].full_name,
            documents_pb2.DESCRIPTOR.services_by_name["DocumentsService"].full_name,
            rag_pb2.DESCRIPTOR.services_by_name["RagService"].full_name,
            reflection.SERVICE_NAME,
        ),
        server,
    )
    return server


async def _serve_grpc(
    settings: Settings, rag_pool: asyncpg.Pool | None, stop_event: asyncio.Event
) -> None:
    server = await _build_grpc_server(settings, rag_pool)
    server.add_insecure_port(f"[::]:{settings.grpc_port}")

    logger = get_logger()
    await server.start()
    logger.info("grpc_server_started", context={"port": settings.grpc_port})

    await stop_event.wait()
    await server.stop(grace=5)
    logger.info("grpc_server_stopped")


async def _serve_http(port: int, stop_event: asyncio.Event) -> None:
    config = uvicorn.Config(
        build_http_app(),
        host="0.0.0.0",  # noqa: S104 — внутренний сервис в docker-сети, не за публичным ingress (BACKEND_PLAN.md §1.2)
        port=port,
        log_config=None,
        access_log=False,
    )
    server = uvicorn.Server(config)
    logger = get_logger()

    serve_task = asyncio.create_task(server.serve())
    logger.info("health_http_server_started", context={"port": port})

    await stop_event.wait()
    server.should_exit = True
    await serve_task
    logger.info("health_http_server_stopped")


async def _run(settings: Settings) -> None:
    logger = get_logger()
    logger.info(
        "starting",
        context={
            "env": settings.env,
            "grpc_port": settings.grpc_port,
            "health_http_port": settings.health_http_port,
        },
    )

    stop_event = asyncio.Event()
    loop = asyncio.get_running_loop()
    for sig in (signal.SIGINT, signal.SIGTERM):
        loop.add_signal_handler(sig, stop_event.set)

    # rag_pool — отдельный пул от роли, ограниченной схемой rag (не той,
    # что использует backend/DATABASE_URL — BACKEND_PLAN.md §1.3), создаётся
    # один раз в composition root, закрывается при остановке процесса.
    # Ошибка здесь НЕ фатальна для всего процесса: helper/ обслуживает ещё
    # три несвязанных с rag сервиса (Files/Stt/Documents), которые не
    # должны падать из-за не забутстрапленной схемы rag на старом томе
    # Postgres (см. scripts/postgres-init/, BACKEND_LOG.md Stage 5) —
    # RagService.Search в этом случае будет отвечать INTERNAL на каждый
    # вызов (app/adapters/db.PgVectorStore с pool=None), что Go-сторона уже
    # трактует как деградацию, не блокировку.
    rag_pool: asyncpg.Pool | None = None
    try:
        rag_pool = await asyncpg.create_pool(settings.rag_database_url)
    except Exception:
        logger.error("rag_database_pool_unavailable_at_startup", exc_info=True)

    try:
        await asyncio.gather(
            _serve_grpc(settings, rag_pool, stop_event),
            _serve_http(settings.health_http_port, stop_event),
        )
    finally:
        if rag_pool is not None:
            await rag_pool.close()
    logger.info("shutdown_complete")


def main() -> None:
    settings = load_settings()
    configure_logging(settings.log_level)
    asyncio.run(_run(settings))


if __name__ == "__main__":
    main()
