import pytest

from app.core.config import Settings


def _required_env(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv("INTERNAL_SECRET", "test-internal-secret")
    monkeypatch.setenv("S3_ACCESS_KEY", "test-access-key")
    monkeypatch.setenv("S3_SECRET_KEY", "test-secret-key")


def test_settings_defaults(monkeypatch: pytest.MonkeyPatch) -> None:
    _required_env(monkeypatch)

    settings = Settings()

    assert settings.env == "dev"
    assert settings.grpc_port == 9090
    assert settings.health_http_port == 8001
    assert settings.log_level == "info"
    assert settings.s3_endpoint == "http://localhost:9000"
    assert settings.s3_public_endpoint == ""
    assert settings.s3_bucket == "zan-files"
    assert settings.whisper_model_size == "tiny"


def test_settings_reads_env_overrides(monkeypatch: pytest.MonkeyPatch) -> None:
    _required_env(monkeypatch)
    monkeypatch.setenv("ENV", "prod")
    monkeypatch.setenv("GRPC_PORT", "9999")
    monkeypatch.setenv("HEALTH_HTTP_PORT", "9998")
    monkeypatch.setenv("LOG_LEVEL", "warn")
    monkeypatch.setenv("S3_ENDPOINT", "http://minio:9000")
    monkeypatch.setenv("S3_PUBLIC_ENDPOINT", "http://localhost:9000")
    monkeypatch.setenv("WHISPER_MODEL_SIZE", "small")

    settings = Settings()

    assert settings.env == "prod"
    assert settings.grpc_port == 9999
    assert settings.health_http_port == 9998
    assert settings.log_level == "warn"
    assert settings.s3_endpoint == "http://minio:9000"
    assert settings.s3_public_endpoint == "http://localhost:9000"
    assert settings.whisper_model_size == "small"


def test_settings_missing_required_secret_raises(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv("S3_ACCESS_KEY", "test-access-key")
    monkeypatch.setenv("S3_SECRET_KEY", "test-secret-key")
    # INTERNAL_SECRET намеренно не задан.

    with pytest.raises(Exception):  # noqa: B017,PT011 — pydantic-settings ValidationError, тип не публичный API этого теста
        Settings()


def test_resolved_s3_public_endpoint_falls_back_to_internal(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    _required_env(monkeypatch)
    monkeypatch.setenv("S3_ENDPOINT", "http://minio:9000")

    settings = Settings()

    assert settings.resolved_s3_public_endpoint() == "http://minio:9000"
