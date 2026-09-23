# syntax=docker/dockerfile:1

FROM python:3.12-slim AS base
WORKDIR /src
ENV PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1

# Системные зависимости Stage 4 (BACKEND_PLAN.md §1.2), без dev-заголовков —
# только рантайм-библиотеки:
#   tesseract-ocr(-rus)  — OCR-фолбэк для сканов (app/adapters/extraction.py,
#                          TesseractOcrProvider, lang="rus+eng")
#   poppler-utils        — pdf2image рендерит страницы PDF в изображения перед OCR
#   libcairo2/libpango.../libgdk-pixbuf... — WeasyPrint (рендер PDF из HTML/CSS,
#                          DocumentsService.Render) не работает без них
#                          (см. BACKEND_LOG.md, Stage 4 — падает на импорте без
#                          понятного сообщения, если их нет)
#   fonts-dejavu-core     — шрифт, на который явно ссылается CSS в
#                          app/adapters/render.py (_PDF_TEMPLATE)
RUN apt-get update && apt-get install --no-install-recommends -y \
        tesseract-ocr \
        tesseract-ocr-rus \
        poppler-utils \
        libcairo2 \
        libpango-1.0-0 \
        libpangocairo-1.0-0 \
        libgdk-pixbuf-2.0-0 \
        fonts-dejavu-core \
    && rm -rf /var/lib/apt/lists/*

# Слой зависимостей отдельно от кода — requirements.txt меняется реже, чем
# app/. Не "pip install ." из pyproject.toml: hatchling потребовал бы
# исходники app/ уже на этом шаге, что сломало бы кэширование слоя
# (см. requirements.txt — почему два места с версиями и как их синхронить).
COPY requirements.txt ./
RUN pip install --no-cache-dir -r requirements.txt

COPY app ./app
COPY healthcheck.py ./

RUN useradd --create-home --uid 10001 helper
USER 10001

# WHISPER_MODEL_SIZE — должен совпадать с рантайм-ENV того же имени
# (Variables сервиса на хостинге), иначе на старте контейнера
# WhisperSttProvider всё равно скачает другую модель заново, теряя смысл
# этого шага. Скачивание/кеширование весов на этапе сборки — не на первом
# пользовательском запросе — избавляет прод от рантайм-зависимости от сети
# HuggingFace (BACKEND_PLAN.md §6 п.10: self-hosted, а не внешний API).
ARG WHISPER_MODEL_SIZE=tiny
RUN python -c "from faster_whisper import WhisperModel; WhisperModel('${WHISPER_MODEL_SIZE}', device='cpu', compute_type='int8')"

EXPOSE 9090 8001
HEALTHCHECK --interval=5s --timeout=3s --retries=5 CMD ["python", "healthcheck.py"]
ENTRYPOINT ["python", "-m", "app.main"]
