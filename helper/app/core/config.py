"""Единственная точка чтения конфигурации helper/ из ENV — собирается один
раз в app.main при старте процесса, передаётся дальше явным пробросом
(BACKEND_CODING_STANDARDS.md §2), не читается напрямую из сервисов/адаптеров.

Список переменных растёт по стадиям (BACKEND_PLAN.md) — Stage 4 добавляет
S3/internal-secret/whisper, ровно то, что реально начинает читать код в этом
PR, не заранее.
"""

from pydantic_settings import BaseSettings, SettingsConfigDict


class Settings(BaseSettings):
    model_config = SettingsConfigDict(case_sensitive=False)

    env: str = "dev"
    grpc_port: int = 9090
    health_http_port: int = 8001
    log_level: str = "info"

    # internal_secret — общий с backend/ секрет gRPC-метадаты x-internal-secret
    # (Stage 4, BACKEND_PLAN.md §1: "shared secret — вторая линия обороны"),
    # проверяется серверным interceptor'ом (app/grpc/interceptors.py). Без
    # дефолта намеренно — тот же принцип, что ADMIN_TOKEN на стороне Go.
    internal_secret: str

    # --- S3-совместимое хранилище (Stage 4, BACKEND_PLAN.md §3.1) ---

    # s3_endpoint — внутри docker-сети (используется для реальных S3-операций
    # и для скачивания по file_url, который прислал backend — тот же адрес,
    # раз оба сервиса в одной сети).
    s3_endpoint: str = "http://localhost:9000"
    # s3_public_endpoint — presigned-ссылка, которую видит снаружи docker-сети
    # (например, `go test`, запущенный на хосте, скачивающий результат
    # DocumentsService.Render в контрактном тесте) — пусто -> тот же
    # s3_endpoint (прод с одним публичным адресом на оба случая).
    s3_public_endpoint: str = ""
    s3_region: str = "us-east-1"
    s3_access_key: str
    s3_secret_key: str
    s3_bucket: str = "zan-files"

    # --- Провайдеры (Stage 4, BACKEND_PLAN.md §6 п.10) ---

    # whisper_model_size — self-hosted faster-whisper за WhisperSttProvider
    # (Strategy — замена на внешний API не требует переписывания вызывающего
    # кода). "tiny" по умолчанию — быстрый старт/CI, для реального
    # распознавания в проде — крупнее ("small"/"medium"), решается по метрикам
    # точности, не заводится "про запас" сейчас.
    whisper_model_size: str = "tiny"

    # --- RAG (Stage 5, BACKEND_PLAN.md §1.3, §3) ---

    # rag_database_url — DSN схемы rag, под отдельной, ограниченной ролью
    # Postgres (не той, что использует backend/DATABASE_URL — роль не имеет
    # прав на схему core и наоборот, см. helper/migrations/env.py и
    # scripts/postgres-init/, BACKEND_PLAN.md §1.3: "предотвращает случайную
    # связность мимо gRPC-контракта"). Без дефолта намеренно — тот же
    # принцип, что internal_secret/s3_access_key.
    rag_database_url: str

    # embedding_model — self-hosted fastembed (ONNX), см.
    # app/adapters/embeddings.FastEmbedProvider. Многоязычная модель,
    # покрывает ru (kz — слабее, см. модуль) — конфигурируемо тем же
    # принципом, что whisper_model_size: смена без переписывания кода.
    embedding_model: str = "sentence-transformers/paraphrase-multilingual-MiniLM-L12-v2"

    # rag_search_top_k_default — top_k, если вызывающая сторона его не
    # передала (RagServicer._DEFAULT_TOP_K использует это значение через
    # settings, не хардкодит отдельно — см. app/main.py).
    rag_search_top_k_default: int = 5

    def resolved_s3_public_endpoint(self) -> str:
        return self.s3_public_endpoint or self.s3_endpoint


def load_settings() -> Settings:
    return Settings()
