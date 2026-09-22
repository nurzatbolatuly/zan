"""S3-совместимый клиент, тот же бакет, что и у backend/ (MinIO локально
— BACKEND_PLAN.md §1.1)."""

import asyncio
from typing import Protocol
from urllib.parse import urlparse

import boto3
import httpx
from botocore.config import Config as BotoConfig
from botocore.exceptions import ClientError

from app.domain.errors import DownloadError, UntrustedFileURLError


class HttpGetter(Protocol):
    """Порт HTTP-скачивания — узкий, чтобы юнит-тест S3Storage.download мог
    подменить его Protocol-based fake, не поднимая реальную сеть
    (BACKEND_CODING_STANDARDS.md §10)."""

    async def get(self, url: str) -> bytes: ...


class HttpxGetter:
    """Реализация HttpGetter поверх httpx.AsyncClient."""

    async def get(self, url: str) -> bytes:
        async with httpx.AsyncClient(timeout=30.0) as client:
            response = await client.get(url)
            response.raise_for_status()
            return response.content


class S3Storage:
    """Скачивание входных файлов (file_url, который прислал backend/, всегда
    указывает на этот же storage — см. _ensure_own_url, SSRF-защита
    BACKEND_PLAN.md §8) и загрузка сгенерированных (generated/-префикс, §3.1).

    Два S3-клиента с одними и теми же учётными данными, но разными
    endpoint_url — internal (реальные S3-операции, тот же docker-сеть, что и
    backend/) и public (presign для RenderResponse.file_url, который должен
    быть разыменован снаружи docker-сети — тот же приём и та же причина, что
    у internal/platform/storage.Client.PresignGetPublic в Go: presigned-
    ссылка на "minio:9000" не резолвится вне docker-сети).
    """

    def __init__(
        self,
        *,
        endpoint: str,
        public_endpoint: str,
        region: str,
        access_key: str,
        secret_key: str,
        bucket: str,
        http: HttpGetter,
    ) -> None:
        self._bucket = bucket
        self._allowed_host = urlparse(endpoint).netloc
        self._http = http
        boto_config = BotoConfig(signature_version="s3v4", s3={"addressing_style": "path"})
        self._client = boto3.client(
            "s3",
            endpoint_url=endpoint,
            region_name=region,
            aws_access_key_id=access_key,
            aws_secret_access_key=secret_key,
            config=boto_config,
        )
        self._public_client = boto3.client(
            "s3",
            endpoint_url=public_endpoint,
            region_name=region,
            aws_access_key_id=access_key,
            aws_secret_access_key=secret_key,
            config=boto_config,
        )

    async def ensure_bucket(self) -> None:
        """Idempotent bootstrap — тот же приём, что и
        internal/platform/storage.Client.ensureBucket в Go: backend/ и
        helper/ стартуют в docker-compose без гарантированного порядка друг
        относительно друга, оба должны быть в состоянии создать бакет сами,
        если он ещё не существует (BACKEND_PLAN.md Stage 0 DoD — "docker
        compose up" без ручных шагов). Не вызывается из __init__ намеренно —
        конструктор не должен делать сетевые вызовы (упрощает юниты, которые
        строят S3Storage для проверки чистой логики вроде _ensure_own_url,
        не поднимая реальный S3); composition root (app/main.py) вызывает
        этот метод один раз при старте.
        """
        try:
            await asyncio.to_thread(self._client.head_bucket, Bucket=self._bucket)
        except ClientError as exc:
            error_code = exc.response.get("Error", {}).get("Code")
            if error_code not in ("404", "NoSuchBucket"):
                raise
            await asyncio.to_thread(self._client.create_bucket, Bucket=self._bucket)

    async def download(self, file_url: str) -> bytes:
        self._ensure_own_url(file_url)
        try:
            return await self._http.get(file_url)
        except Exception as exc:
            raise DownloadError(f"failed to download {file_url!r}: {exc}") from exc

    def _ensure_own_url(self, file_url: str) -> None:
        host = urlparse(file_url).netloc
        if host != self._allowed_host:
            raise UntrustedFileURLError(f"refusing to fetch file_url with untrusted host {host!r}")

    async def upload_generated(self, key: str, data: bytes, content_type: str) -> tuple[str, str]:
        """Загружает готовый файл под generated/-префиксом, возвращает
        (file_url presigned, object_key)."""
        await asyncio.to_thread(
            self._client.put_object,
            Bucket=self._bucket,
            Key=key,
            Body=data,
            ContentType=content_type,
        )
        url = await asyncio.to_thread(
            self._public_client.generate_presigned_url,
            "get_object",
            Params={"Bucket": self._bucket, "Key": key},
            ExpiresIn=86400,
        )
        return url, key
