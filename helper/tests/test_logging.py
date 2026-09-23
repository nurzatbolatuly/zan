import json

import pytest

from app.core.logging import configure_logging, get_logger


def test_log_record_matches_contract(capsys: pytest.CaptureFixture[str]) -> None:
    configure_logging("debug")
    logger = get_logger()

    logger.info("file_extract_completed", context={"extracted_chars": 1024})

    out = capsys.readouterr().out.strip()
    record = json.loads(out)

    assert record["service"] == "python-agent"
    assert record["level"] == "INFO"
    assert record["message"] == "file_extract_completed"
    assert "timestamp" in record
    assert record["context"] == {"extracted_chars": 1024}


def test_warning_level_is_warn_not_warning(capsys: pytest.CaptureFixture[str]) -> None:
    configure_logging("debug")
    logger = get_logger()

    logger.warning("sources_degraded")

    record = json.loads(capsys.readouterr().out.strip())
    assert record["level"] == "WARN"


def test_min_level_filters_debug(capsys: pytest.CaptureFixture[str]) -> None:
    configure_logging("info")
    logger = get_logger()

    logger.debug("should not appear")

    assert capsys.readouterr().out.strip() == ""


def test_critical_with_exc_info_renders_traceback(capsys: pytest.CaptureFixture[str]) -> None:
    configure_logging("debug")
    logger = get_logger()

    try:
        raise ValueError("boom")
    except ValueError:
        logger.critical("grpc_call_panicked", exc_info=True)

    record = json.loads(capsys.readouterr().out.strip())
    assert record["level"] == "CRITICAL"
    assert "ValueError: boom" in record["exception"]
