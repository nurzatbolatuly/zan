-- Stage 1 (BACKEND_PLAN.md): полная схема core — все таблицы v2 §2,
-- адаптированные под Session вместо User (v3 §2.2/§3). Схему core владеет
-- и мигрирует только Go (BACKEND_PLAN.md §1.3); схема rag (pgvector) —
-- отдельно, только Python, появится в Stage 5.
--
-- CREATE SCHEMA IF NOT EXISTS ниже — по-прежнему нужен, хотя с Stage 8
-- docker-compose.yml/scripts/postgres-init/02-core-role.sh уже создают
-- схему core (авторизованную на непривилегированную роль zan_core) до
-- первого запуска golang-migrate: тестовый Postgres в internal/repo
-- (testcontainers, без init-скриптов, см. session_repo_test.go) по-прежнему
-- поднимается "с нуля" под ролью-бутстрапом и ожидает, что первая же
-- миграция создаст схему сама.
--
-- Все id, которые сущность получает не от админа (Service.id — slug,
-- задаётся вручную), генерируются в Go (internal/platform/idgen) и
-- приходят уже готовыми — DEFAULT gen_random_uuid() здесь намеренно нет.
--
-- Enum-подобные поля — text + CHECK, а не нативный Postgres ENUM: с
-- native ENUM пришлось бы регистрировать каждый тип в pgx (pgtype.Map)
-- на старте процесса и заново на каждый новый тип по мере роста схемы;
-- text+CHECK даёт ту же гарантию валидности на уровне БД без этой
-- церемонии, а типобезопасность на стороне Go — через domain-типы
-- (internal/domain), не через драйвер.

CREATE SCHEMA IF NOT EXISTS core;

CREATE TABLE core.sessions (
    id              uuid PRIMARY KEY,
    language        text NOT NULL DEFAULT 'ru' CHECK (language IN ('ru', 'kz')),
    theme           text CHECK (theme IN ('light', 'dark')),
    onboarding_seen boolean NOT NULL DEFAULT false,
    created_at      timestamptz NOT NULL DEFAULT now(),
    last_seen_at    timestamptz NOT NULL DEFAULT now(),
    expires_at      timestamptz NOT NULL
);

CREATE TABLE core.services (
    id          text PRIMARY KEY,
    type_label  text NOT NULL,
    name        text NOT NULL,
    price       numeric(12, 2) NOT NULL,
    is_active   boolean NOT NULL DEFAULT true,
    updated_at  timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE core.tariffs (
    id                uuid PRIMARY KEY,
    name              text NOT NULL,
    discount_percent  int NOT NULL CHECK (discount_percent BETWEEN 0 AND 90),
    items             jsonb NOT NULL,
    is_active         boolean NOT NULL DEFAULT true,
    sort_order        int NOT NULL DEFAULT 0,
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE core.user_credits (
    id          uuid PRIMARY KEY,
    session_id  uuid NOT NULL REFERENCES core.sessions (id),
    service_id  text NOT NULL REFERENCES core.services (id),
    quantity    int NOT NULL DEFAULT 0,
    updated_at  timestamptz NOT NULL DEFAULT now(),
    UNIQUE (session_id, service_id) -- BACKEND_PLAN.md §2, индекс критичен под UPSERT списания
);

CREATE TABLE core.payments (
    id          uuid PRIMARY KEY,
    session_id  uuid NOT NULL REFERENCES core.sessions (id),
    kind        text NOT NULL CHECK (kind IN ('single_service', 'tariff', 'custom')),
    tariff_id   uuid REFERENCES core.tariffs (id),
    items       jsonb NOT NULL,
    amount      numeric(12, 2) NOT NULL,
    status      text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'success', 'failed')),
    provider    text NOT NULL DEFAULT 'mock',
    created_at  timestamptz NOT NULL DEFAULT now(),
    paid_at     timestamptz
);

CREATE TABLE core.threads (
    id              uuid PRIMARY KEY,
    session_id      uuid NOT NULL REFERENCES core.sessions (id),
    service_id      text NOT NULL REFERENCES core.services (id),
    status          text NOT NULL DEFAULT 'queued'
                        CHECK (status IN ('queued', 'processing', 'clarify', 'done', 'error', 'canceled')),
    title           text NOT NULL DEFAULT '',
    preview_text    text NOT NULL DEFAULT '',
    message_count   int NOT NULL DEFAULT 0,
    is_paid         boolean NOT NULL DEFAULT false,
    paid_at         timestamptz,
    free_until      timestamptz,
    created_at      timestamptz NOT NULL DEFAULT now(),
    last_message_at timestamptz,
    closed_at       timestamptz,
    deleted_at      timestamptz -- soft delete, v2 §4.5
);

-- BACKEND_PLAN.md §2 — индексы, критичные с первого дня, не "оптимизация потом".
CREATE INDEX idx_threads_session_status_deleted ON core.threads (session_id, status, deleted_at);
CREATE INDEX idx_threads_session_created ON core.threads (session_id, created_at);

CREATE TABLE core.messages (
    id                 uuid PRIMARY KEY,
    thread_id          uuid NOT NULL REFERENCES core.threads (id),
    sender             text NOT NULL CHECK (sender IN ('user', 'assistant')),
    input_type         text NOT NULL CHECK (input_type IN ('text', 'voice', 'file')),
    text               text NOT NULL DEFAULT '',
    sources            jsonb,
    findings           jsonb,
    feedback           text CHECK (feedback IN ('like', 'dislike')),
    processing_time_ms int,
    created_at         timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE core.file_attachments (
    id                 uuid PRIMARY KEY,
    message_id         uuid NOT NULL REFERENCES core.messages (id),
    file_url           text NOT NULL,
    original_name      text NOT NULL,
    mime_type          text NOT NULL,
    size_bytes         bigint NOT NULL,
    purpose            text NOT NULL CHECK (purpose IN ('analysis_input', 'generated_output')),
    output_formats     jsonb,
    processing_status  text NOT NULL DEFAULT 'pending' CHECK (processing_status IN ('pending', 'processed', 'error')),
    created_at         timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE core.agent_prompts (
    id          uuid PRIMARY KEY,
    agent_type  text NOT NULL UNIQUE CHECK (agent_type IN ('qa', 'document')),
    prompt_text text NOT NULL,
    updated_at  timestamptz NOT NULL DEFAULT now()
    -- updated_by сознательно убран — v3 §3: "нет персональных аккаунтов,
    -- оставить как 'кто последний правил' не критично без ролей".
);
