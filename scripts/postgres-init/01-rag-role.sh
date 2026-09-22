#!/usr/bin/env bash
# docker-entrypoint-initdb.d — выполняется официальным образом postgres
# РОВНО ОДИН РАЗ, при первой инициализации ПУСТОГО тома (BACKEND_PLAN.md
# §1.3: "Роль go_core не имеет прав на схему rag и наоборот"). Заводит
# отдельную, ограниченную роль zan_rag — под ней ходит helper/ (и Alembic-
# миграции схемы rag, см. helper/migrations/env.py), у неё нет прав на
# схему core (которой владеет backend/DATABASE_URL, тот же bootstrap-
# суперпользователь POSTGRES_USER, что и раньше — полная симметрия
# "core изолирована от rag" пока не реализована, см. BACKEND_LOG.md, запись
# Stage 5: retrofitting backend/ под отдельную непривилегированную роль —
# отдельная, более рискованная миграция существующих томов, осознанно
# отложена до Stage 8 hardening).
#
# Если у вас уже есть локальный docker-том с Postgres от прошлых стадий —
# этот скрипт НЕ применится автоматически (initdb.d гоняется только на
# пустом томе); нужно один раз пересоздать том Postgres (`docker compose down
# -v` затронет ТОЛЬКО контейнер postgres — см. предупреждение в BACKEND_LOG.md
# перед тем, как это делать, если в томе есть данные, которые жалко терять).
set -euo pipefail

: "${RAG_DB_PASSWORD:?RAG_DB_PASSWORD must be set}"

psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" <<-SQL
    CREATE EXTENSION IF NOT EXISTS vector;

    DO \$\$
    BEGIN
        IF NOT EXISTS (SELECT FROM pg_catalog.pg_roles WHERE rolname = 'zan_rag') THEN
            CREATE ROLE zan_rag LOGIN PASSWORD '${RAG_DB_PASSWORD}';
        END IF;
    END
    \$\$;

    CREATE SCHEMA IF NOT EXISTS rag AUTHORIZATION zan_rag;
    -- REVOKE ALL ... FROM PUBLIC — схема rag доступна только zan_rag, не
    -- любой роли, которая умеет подключиться к базе (по умолчанию Postgres
    -- даёт PUBLIC право CREATE в схемах, созданных её владельцем — здесь
    -- явно закрываем это, чтобы "изоляция через отдельную роль" не была
    -- фикцией из-за дефолтных грантов PUBLIC).
    REVOKE ALL ON SCHEMA rag FROM PUBLIC;
    ALTER ROLE zan_rag SET search_path = rag;
SQL
