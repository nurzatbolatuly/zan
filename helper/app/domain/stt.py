from dataclasses import dataclass


@dataclass(frozen=True)
class Transcript:
    """Результат распознавания речи (SttService.Transcribe, Stage 4,
    BACKEND_PLAN.md). Пустая строка (тишина) — валидное значение здесь,
    решение о том, что с ней делать — на стороне вызывающего кода
    (zan-backend-tz-v3.md §5.4)."""

    text: str
