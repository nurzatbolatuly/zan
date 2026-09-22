#!/usr/bin/env bash
# docker-entrypoint-initdb.d — выполняется официальным образом postgres
# РОВНО ОДИН РАЗ, при первой инициализации ПУСТОГО тома. Заводит отдельную,
# непривилегированную роль zan_core — под ней теперь ходит backend/
# (DATABASE_URL) и golang-migrate — вместо бутстрап-суперпользователя
# POSTGRES_USER, которым backend/ пользовался до Stage 8.
#
# Зачем нужен именно этот шаг (не просто "поменять пароль в DATABASE_URL"):
# POSTGRES_USER официального образа postgres — суперпользователь, он
# обходит вообще любые GRANT/REVOKE, включая "REVOKE ALL ON SCHEMA rag FROM
# PUBLIC" из 01-rag-role.sh. Пока backend/ ходил под этим суперпользователем,
# заявленная в BACKEND_PLAN.md §1.3 изоляция "core не имеет прав на rag и
# наоборот" была фактической только в одну сторону (Python не мог выйти за
# rag, но Go физически мог пойти куда угодно, будучи суперпользователем) —
# см. BACKEND_LOG.md, запись Stage 5, "отложено до Stage 8 hardening".
# zan_core — обычная LOGIN-роль без каких-либо дополнительных прав, поэтому
# REVOKE ALL ON SCHEMA rag FROM PUBLIC (01-rag-role.sh) реально её касается.
#
# Если у вас уже есть локальный docker-том с Postgres от прошлых стадий —
# этот скрипт НЕ применится автоматически (initdb.d гоняется только на
# пустом томе, тот же нюанс, что и у 01-rag-role.sh) — нужно один раз
# пересоздать том (`docker compose down -v` — см. предупреждение там же и в
# BACKEND_LOG.md перед тем, как это делать, если в томе есть данные, которые
# жалко терять).
set -euo pipefail

: "${CORE_DB_PASSWORD:?CORE_DB_PASSWORD must be set}"

psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" <<-SQL
    DO \$\$
    BEGIN
        IF NOT EXISTS (SELECT FROM pg_catalog.pg_roles WHERE rolname = 'zan_core') THEN
            CREATE ROLE zan_core LOGIN PASSWORD '${CORE_DB_PASSWORD}';
        END IF;
    END
    \$\$;

    CREATE SCHEMA IF NOT EXISTS core AUTHORIZATION zan_core;
    -- Тот же приём, что REVOKE в 01-rag-role.sh — без него дефолтные
    -- гранты Postgres на схему её владельцу сделали бы "core изолирована
    -- от rag" фикцией с другой стороны.
    REVOKE ALL ON SCHEMA core FROM PUBLIC;
    ALTER ROLE zan_core SET search_path = core;
    -- GRANT CREATE ON DATABASE — без него уже первая миграция
    -- (000002_core_schema.up.sql: "CREATE SCHEMA IF NOT EXISTS core;",
    -- нужен второй раз ради testcontainers-пути internal/repo, см.
    -- комментарий в самой миграции) падает с "permission denied for
    -- database" ДАЖЕ когда схема уже существует — Postgres проверяет
    -- привилегию CREATE на уровне базы раньше, чем проверку "IF NOT
    -- EXISTS" (проверено вручную при реализации Stage 8). Не открывает
    -- доступ к схеме rag — то, что действительно изолирует core от rag,
    -- это REVOKE ALL ON SCHEMA rag FROM PUBLIC в 01-rag-role.sh, а не
    -- отсутствие этого GRANT.
    GRANT CREATE ON DATABASE "$POSTGRES_DB" TO zan_core;
SQL
