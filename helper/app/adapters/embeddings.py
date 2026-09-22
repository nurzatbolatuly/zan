"""Обёртка над self-hosted fastembed (ONNX, без torch) — за портом
rag_service.EmbeddingProvider, тем же Strategy-приёмом, что
adapters/providers.WhisperSttProvider (BACKEND_PLAN.md §6 п.10: self-hosted
по умолчанию, замена на внешний API — без переписывания вызывающего кода).

Модель — sentence-transformers/paraphrase-multilingual-MiniLM-L12-v2
(384 измерения, fastembed — ONNX-рантайм, без зависимости от torch, которую
уже тянет faster-whisper транзитивно не полностью — см. requirements).
Многоязычная (ru покрыт хорошо; kz — слабее, общая проблема всех
доступных self-hosted мультиязычных embedding-моделей на эту пару языков,
пересмотреть при росте корпуса/жалобах на качество поиска по kz-запросам,
тот же принцип, что WHISPER_MODEL_SIZE)."""

from typing import cast

from fastembed import TextEmbedding


class FastEmbedProvider:
    """Реализация rag_service.EmbeddingProvider. Модель грузится один раз в
    конструкторе (composition root, app.main) — тот же принцип, что
    WhisperSttProvider: cold-start модели платится при старте процесса, не
    на случайном пользовательском запросе."""

    def __init__(self, model_name: str) -> None:
        self._model = TextEmbedding(model_name=model_name)

    def embed(self, text: str) -> list[float]:
        # fastembed.embed() — генератор, на один текст — один вектор
        # (numpy.ndarray без строгих типов-стабов — явный cast).
        (vector,) = self._model.embed([text])
        return cast(list[float], vector.tolist())
