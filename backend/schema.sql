-- =============================================================================
-- Zan Backend — единый референс-файл текущей схемы БД (схема `core`).
--
-- ЭТО НЕ МИГРАЦИЯ. golang-migrate по-прежнему читает только
-- backend/migrations/*.up.sql/*.down.sql (BACKEND_PLAN.md §1.1) — этот файл
-- их не заменяет и не подключён ни к одному инструменту. Назначение —
-- единая точка, где видна ВСЯ схема целиком одним взглядом (а не по кускам
-- из N файлов миграций), для онбординга и код-ревью.
--
-- ПРАВИЛО ОБНОВЛЕНИЯ (BACKEND_CODING_STANDARDS.md §12): любая миграция,
-- меняющая схему `core`, обновляет этот файл в том же PR. Перед
-- изменяемым/новым объектом (таблица/колонка/индекс/constraint) — короткий
-- комментарий "что изменилось и почему" (или ссылка на номер миграции, если
-- обоснование уже развёрнуто там), сразу под ним — сам SQL в актуальном
-- (финальном) виде. Это не журнал ALTER'ов — файл всегда отражает текущее
-- состояние, история "как дошли до него" остаётся в BACKEND_LOG.md и в
-- самих файлах миграций.
-- =============================================================================

CREATE SCHEMA IF NOT EXISTS core;

-- -----------------------------------------------------------------------------
-- core.sessions — Stage 1 (000002_core_schema): анонимная сессия вместо
-- User (zan-backend-tz-v3.md §2.2), opaque-токен подписывается вне БД.
-- -----------------------------------------------------------------------------
CREATE TABLE core.sessions (
    id              uuid PRIMARY KEY,
    language        text NOT NULL DEFAULT 'ru' CHECK (language IN ('ru', 'kz')),
    theme           text CHECK (theme IN ('light', 'dark')),
    onboarding_seen boolean NOT NULL DEFAULT false,
    created_at      timestamptz NOT NULL DEFAULT now(),
    last_seen_at    timestamptz NOT NULL DEFAULT now(),
    expires_at      timestamptz NOT NULL
);

-- -----------------------------------------------------------------------------
-- core.services — Stage 1 (000002): базовая единица тарификации (zan-backend-tz-v2.md §2.2).
-- Stage 2 (000003_stage2_billing): `price` переведён numeric(12,2) -> integer
-- (BACKEND_PLAN.md §6 п.7 — цена всегда в целых тенге, не тиынах) + CHECK на
-- неотрицательность.
-- -----------------------------------------------------------------------------
CREATE TABLE core.services (
    id          text PRIMARY KEY,
    type_label  text NOT NULL,
    name        text NOT NULL,
    price       integer NOT NULL CHECK (price >= 0),
    is_active   boolean NOT NULL DEFAULT true,
    updated_at  timestamptz NOT NULL DEFAULT now()
);

-- -----------------------------------------------------------------------------
-- core.tariffs — Stage 1 (000002): фиксированный бандл услуг со скидкой
-- (zan-backend-tz-v2.md §2.3). Цена не хранится здесь — считается на лету
-- (internal/service/pricing) по текущим ценам core.services.
-- -----------------------------------------------------------------------------
CREATE TABLE core.tariffs (
    id                uuid PRIMARY KEY,
    name              text NOT NULL,
    discount_percent  int NOT NULL CHECK (discount_percent BETWEEN 0 AND 90),
    items             jsonb NOT NULL, -- [{service_id, qty}], без FK на services (см. internal/service/catalog.ValidateItems)
    is_active         boolean NOT NULL DEFAULT true,
    sort_order        int NOT NULL DEFAULT 0,
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now()
);

-- -----------------------------------------------------------------------------
-- core.user_credits — Stage 1 (000002): остаток сессии по услуге, списывается
-- атомарно (SELECT ... FOR UPDATE — zan-backend-tz-v2.md §4.2,
-- internal/repo/billing_repo.go#DebitCredit).
-- -----------------------------------------------------------------------------
CREATE TABLE core.user_credits (
    id          uuid PRIMARY KEY,
    session_id  uuid NOT NULL REFERENCES core.sessions (id),
    service_id  text NOT NULL REFERENCES core.services (id),
    quantity    int NOT NULL DEFAULT 0,
    updated_at  timestamptz NOT NULL DEFAULT now(),
    UNIQUE (session_id, service_id) -- под UPSERT списания/начисления
);

-- -----------------------------------------------------------------------------
-- core.payments — Stage 1 (000002): мок-оплата (zan-backend-tz-v2.md §2.5).
-- Stage 2 (000003_stage2_billing): `amount` переведён numeric(12,2) ->
-- integer (та же причина, что у services.price) + CHECK на неотрицательность.
-- `thread_id` — см. ALTER сразу после core.threads ниже: FK на ещё не
-- созданную здесь таблицу, объявлен отдельно, чтобы файл оставался
-- исполняемым сверху вниз (порядок таблиц следует нумерации zan-backend-tz-v2.md §2,
-- где Payment идёт до Thread).
-- -----------------------------------------------------------------------------
CREATE TABLE core.payments (
    id          uuid PRIMARY KEY,
    session_id  uuid NOT NULL REFERENCES core.sessions (id),
    kind        text NOT NULL CHECK (kind IN ('single_service', 'tariff', 'custom')),
    tariff_id   uuid REFERENCES core.tariffs (id),
    items       jsonb NOT NULL, -- [{service_id, qty}] — копия состава на момент покупки
    amount      integer NOT NULL CHECK (amount >= 0),
    status      text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'success', 'failed')),
    provider    text NOT NULL DEFAULT 'mock',
    created_at  timestamptz NOT NULL DEFAULT now(),
    paid_at     timestamptz
);

-- -----------------------------------------------------------------------------
-- core.threads — Stage 1 (000002): обращение/консультация (zan-backend-tz-v2.md §2.6).
-- Статус-машина — internal/domain/thread_status.go (Stage 3).
-- -----------------------------------------------------------------------------
CREATE TABLE core.threads (
    id              uuid PRIMARY KEY,
    session_id      uuid NOT NULL REFERENCES core.sessions (id),
    service_id      text NOT NULL REFERENCES core.services (id),
    -- clarify удалён 000008_drop_clarify_status; queued заменён на
    -- awaiting_payment (тред сохранён, но не оплачен) — 000009_awaiting_payment_status.
    status          text NOT NULL DEFAULT 'awaiting_payment'
                        CHECK (status IN ('awaiting_payment', 'processing', 'done', 'error', 'canceled')),
    title           text NOT NULL DEFAULT '',
    preview_text    text NOT NULL DEFAULT '',
    message_count   int NOT NULL DEFAULT 0,
    -- is_paid/paid_at/free_until/closed_at удалены 000010_per_question_billing:
    -- один вопрос = одна консультация, оплата — по status, история — в core.payments.
    created_at      timestamptz NOT NULL DEFAULT now(),
    last_message_at timestamptz,
    deleted_at      timestamptz -- soft delete, v2 §4.5
);

-- Индексы, критичные с первого дня (BACKEND_PLAN.md §2), не "оптимизация потом".
CREATE INDEX idx_threads_session_status_deleted ON core.threads (session_id, status, deleted_at);
CREATE INDEX idx_threads_session_created ON core.threads (session_id, created_at);

-- Stage 2 (000003_stage2_billing): core.payments.thread_id (nullable FK) —
-- под checkout {..., thread_id?}, когда баланс кончился в момент открытия
-- треда (zan-backend-tz-v2.md §4.2 п.2). Оплата вопроса треда по этому
-- платежу — thread.Service.Resume, здесь только сама связь в БД.
ALTER TABLE core.payments
    ADD COLUMN thread_id uuid REFERENCES core.threads (id);

-- -----------------------------------------------------------------------------
-- core.messages — Stage 1 (000002): сообщение в треде (zan-backend-tz-v2.md §2.7).
-- -----------------------------------------------------------------------------
CREATE TABLE core.messages (
    id                 uuid PRIMARY KEY,
    thread_id          uuid NOT NULL REFERENCES core.threads (id),
    sender             text NOT NULL CHECK (sender IN ('user', 'assistant')),
    input_type         text NOT NULL CHECK (input_type IN ('text', 'voice', 'file')),
    text               text NOT NULL DEFAULT '',
    sources            jsonb,
    findings           jsonb,
    -- unverified_sources (Stage 5, 000005) удалён 000007_remove_rag — сверять
    -- процитированные источники больше не с чем (RAG удалён).
    feedback           text CHECK (feedback IN ('like', 'dislike')),
    processing_time_ms int,
    created_at         timestamptz NOT NULL DEFAULT now()
);

-- -----------------------------------------------------------------------------
-- core.file_attachments — Stage 1 (000002): вложение сообщения (zan-backend-tz-v2.md §2.8).
-- Stage 4 (000004_stage4_files): message_id стал nullable (файл существует
-- с момента загрузки, до привязки к сообщению — v2 §3.2 "file_ids?"),
-- добавлены session_id (владение файлом до привязки) и extracted_text
-- (результат FilesService.Extract, места для которого не было в v2-схеме).
-- file_url переименован в object_key — хранит ключ объекта в бакете, не
-- готовую ссылку (presigned-ссылка перевыпускается на каждый ответ заново,
-- see internal/platform/storage.Client.PresignGet*).
-- Stage 6 (000006_stage6_documents): добавлен thread_id — прямая ссылка на
-- тред для purpose=generated_output (генерация документа рендерит и pdf, и
-- docx одним вызовом POST /threads/{id}/generate-document, каждый формат —
-- своя строка; output_formats=["pdf","docx"] на обеих строках декларирует
-- полный набор форматов документа). NULL для purpose=analysis_input.
-- -----------------------------------------------------------------------------
CREATE TABLE core.file_attachments (
    id                 uuid PRIMARY KEY,
    session_id         uuid NOT NULL REFERENCES core.sessions (id),
    message_id         uuid REFERENCES core.messages (id),
    thread_id          uuid REFERENCES core.threads (id),
    object_key         text NOT NULL,
    original_name      text NOT NULL,
    mime_type          text NOT NULL,
    size_bytes         bigint NOT NULL,
    purpose            text NOT NULL CHECK (purpose IN ('analysis_input', 'generated_output')),
    output_formats     jsonb,
    processing_status  text NOT NULL DEFAULT 'pending' CHECK (processing_status IN ('pending', 'processed', 'error')),
    extracted_text     text,
    created_at         timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX file_attachments_thread_id_idx
    ON core.file_attachments (thread_id, mime_type, created_at DESC)
    WHERE thread_id IS NOT NULL;

-- -----------------------------------------------------------------------------
-- core.agent_prompts — Stage 1 (000002): ровно 2 строки, по одной на тип
-- агента (zan-backend-tz-v2.md §2.9). `updated_by` сознательно отсутствует —
-- v3 §3: нет персональных аккаунтов, "кто последний правил" не критично без ролей.
-- -----------------------------------------------------------------------------
CREATE TABLE core.agent_prompts (
    id          uuid PRIMARY KEY,
    agent_type  text NOT NULL UNIQUE CHECK (agent_type IN ('qa', 'document')),
    prompt_text text NOT NULL,
    updated_at  timestamptz NOT NULL DEFAULT now()
);

-- -----------------------------------------------------------------------------
-- Стартовые данные — Stage 2 (000003_stage2_billing): единственный способ
-- завести core.services, т.к. /admin/services не даёт POST (zan-backend-tz-v2.md
-- §3.7 — только GET список + PUT price/is_active). Цены 1:1 с демо-данными
-- фронта (frontend/src/features/settings/mocks.ts).
-- -----------------------------------------------------------------------------
INSERT INTO core.services (id, type_label, name, price, is_active) VALUES
    ('qa', 'Консультация', 'Вопрос-ответ', 2900, true),
    ('doc', 'Документ', 'Подготовка документа', 4900, true)
ON CONFLICT (id) DO NOTHING;
