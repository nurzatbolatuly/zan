"""Раньше любого импорта из app.* — см. app/bootstrap_genproto.py: тесты
импортируют сгенерированный код (напрямую или транзитивно через app.main),
поэтому sys.path должен быть подготовлен ещё до сбора тестов."""

from collections.abc import Iterator

import pytest
import structlog

from app import bootstrap_genproto  # noqa: F401


@pytest.fixture(autouse=True)
def _reset_structlog_config() -> Iterator[None]:
    """tests/test_logging.py вызывает configure_logging(...), которая
    захватывает текущий sys.stdout в PrintLoggerFactory (app/core/logging.py)
    — capsys подменяет sys.stdout только на время своего теста, дальше поток
    закрывается. Без сброса конфигурация "протекает" в любой следующий тест,
    где код логирует через уже глобально закешированный logger (structlog
    cache_logger_on_first_use=True) — обнаружено на тестах gRPC-сервисеров
    (async grpc-интерцептор логирует после того, как test_logging.py уже
    переконфигурировал structlog в предыдущем тесте), падало с "I/O operation
    on closed file". reset_defaults() возвращает structlog к ленивой
    дефолтной конфигурации перед каждым тестом — тот же принцип, что
    monkeypatch: побочный эффект одного теста не должен переживать его сам."""
    yield
    structlog.reset_defaults()
