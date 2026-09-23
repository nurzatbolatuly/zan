import time
from collections.abc import Iterator

import boto3
import httpx
import pytest
from testcontainers.core.container import DockerContainer

from app.adapters.storage import HttpGetter, S3Storage
from app.domain.errors import DownloadError, UntrustedFileURLError

INTERNAL_ENDPOINT = "http://minio:9000"
PUBLIC_ENDPOINT = "http://localhost:9000"


class FakeHttpGetter:
    def __init__(self, content: bytes | None = None, error: Exception | None = None) -> None:
        self.content = content
        self.error = error
        self.requested_urls: list[str] = []

    async def get(self, url: str) -> bytes:
        self.requested_urls.append(url)
        if self.error is not None:
            raise self.error
        assert self.content is not None
        return self.content


def new_storage(
    http: HttpGetter, endpoint: str, bucket: str, public_endpoint: str = PUBLIC_ENDPOINT
) -> S3Storage:
    return S3Storage(
        endpoint=endpoint,
        public_endpoint=public_endpoint,
        region="us-east-1",
        access_key="minioadmin",
        secret_key="minioadmin",
        bucket=bucket,
        http=http,
    )


async def test_download_rejects_untrusted_host() -> None:
    storage = new_storage(FakeHttpGetter(content=b"x"), INTERNAL_ENDPOINT, "zan-files-test")

    with pytest.raises(UntrustedFileURLError):
        await storage.download("https://evil.example.com/steal-me.pdf")


async def test_download_accepts_own_host() -> None:
    http = FakeHttpGetter(content=b"pdf bytes")
    storage = new_storage(http, INTERNAL_ENDPOINT, "zan-files-test")

    data = await storage.download(f"{INTERNAL_ENDPOINT}/zan-files-test/uploads/a.pdf")

    assert data == b"pdf bytes"
    assert http.requested_urls == [f"{INTERNAL_ENDPOINT}/zan-files-test/uploads/a.pdf"]


async def test_download_wraps_http_errors() -> None:
    storage = new_storage(
        FakeHttpGetter(error=RuntimeError("connection refused")),
        INTERNAL_ENDPOINT,
        "zan-files-test",
    )

    with pytest.raises(DownloadError):
        await storage.download(f"{INTERNAL_ENDPOINT}/zan-files-test/uploads/a.pdf")


@pytest.fixture(scope="module")
def minio_endpoint() -> Iterator[str]:
    """Реальный MinIO в testcontainers — не мок S3 API: moto не поддерживает кастомный endpoint_url, который
    нужен для MinIO (проверено при реализации Stage 4), см. pyproject.toml.
    """
    container = (
        DockerContainer("quay.io/minio/minio:latest")
        .with_command("server /data")
        .with_env("MINIO_ROOT_USER", "minioadmin")
        .with_env("MINIO_ROOT_PASSWORD", "minioadmin")
        .with_exposed_ports(9000)
    )

    with container:
        host = container.get_container_host_ip()
        port = container.get_exposed_port(9000)
        endpoint = f"http://{host}:{port}"

        deadline = time.monotonic() + 30
        while time.monotonic() < deadline:
            try:
                resp = httpx.get(f"{endpoint}/minio/health/live", timeout=1.0)
                if resp.status_code == 200:
                    break
            except httpx.HTTPError:
                pass
            time.sleep(0.5)
        else:
            pytest.fail("minio did not become healthy in time")

        yield endpoint


async def test_ensure_bucket_creates_bucket_when_missing(minio_endpoint: str) -> None:
    storage = new_storage(
        FakeHttpGetter(), minio_endpoint, "zan-files-test-ensure", public_endpoint=minio_endpoint
    )

    await storage.ensure_bucket()
    await storage.ensure_bucket()  # идемпотентно — второй вызов не должен падать

    buckets = boto3.client(
        "s3",
        endpoint_url=minio_endpoint,
        region_name="us-east-1",
        aws_access_key_id="minioadmin",
        aws_secret_access_key="minioadmin",
    ).list_buckets()
    assert "zan-files-test-ensure" in [b["Name"] for b in buckets["Buckets"]]


async def test_upload_generated_puts_object_and_returns_presigned_url(minio_endpoint: str) -> None:
    bucket = "zan-files-test"
    # public_endpoint == minio_endpoint здесь намеренно: этот тест проверяет,
    # что presigned-ссылка реально скачивается, а не только строится — split
    # internal/public (BACKEND_LOG.md, Stage 4) проверяется на уровне
    # конфигурации (test_config.py), не здесь.
    storage = new_storage(FakeHttpGetter(), minio_endpoint, bucket, public_endpoint=minio_endpoint)
    await storage.ensure_bucket()

    url, key = await storage.upload_generated(
        "generated/report.pdf", b"pdf content", "application/pdf"
    )
    assert key == "generated/report.pdf"

    resp = httpx.get(url, timeout=5.0)
    resp.raise_for_status()
    assert resp.content == b"pdf content"
