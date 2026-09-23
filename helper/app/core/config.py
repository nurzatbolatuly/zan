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

    def resolved_s3_public_endpoint(self) -> str:
        return self.s3_public_endpoint or self.s3_endpoint


def load_settings() -> Settings:
    return Settings()
