"""Alembic env.py — миграции схемы rag (BACKEND_PLAN.md §1.3, Stage 5).
Асинхронный движок (SQLAlchemy + asyncpg-диалект — тот же пакет asyncpg,
что app/adapters/db.py, без добавления psycopg2 отдельной зависимостью
только ради CLI-миграций) — стандартный рецепт Alembic для async-драйверов.

target_metadata = None намеренно: миграции — raw SQL (op.execute(...)), не
ORM-модели (helper/ не использует SQLAlchemy ORM нигде в рантайме,
BACKEND_CODING_STANDARDS.md §1.2 — "raw SQL", не ORM) — autogenerate не
нужен и не будет работать без деклараций моделей.
"""

import asyncio
import os
from logging.config import fileConfig

from alembic import context
from sqlalchemy.engine import Connection
from sqlalchemy.ext.asyncio import async_engine_from_config

config = context.config

if config.config_file_name is not None:
    fileConfig(config.config_file_name)

target_metadata = None

# RAG_DATABASE_URL — тот же DSN, что app.core.config.Settings.rag_database_url
# (роль zan_rag, владеющая схемой rag — scripts/postgres-init/). Alembic сам
# ожидает синхронный "postgresql://" в sqlalchemy.url секции ini, но здесь
# мы подменяем на async-диалект явно (async_engine_from_config ниже читает
# "sqlalchemy.url" из этого же dict).
db_url = os.environ["RAG_DATABASE_URL"].replace("postgresql://", "postgresql+asyncpg://", 1)
config.set_main_option("sqlalchemy.url", db_url)


def run_migrations_offline() -> None:
    context.configure(
        url=db_url,
        target_metadata=target_metadata,
        literal_binds=True,
        dialect_opts={"paramstyle": "named"},
        version_table_schema="rag",
    )
    with context.begin_transaction():
        context.run_migrations()


def do_run_migrations(connection: Connection) -> None:
    # version_table_schema="rag" — таблица alembic_version живёт внутри
    # схемы rag, не в public (на который у роли zan_rag нет прав, см.
    # scripts/postgres-init/ REVOKE ALL ... FROM PUBLIC).
    context.configure(
        connection=connection, target_metadata=target_metadata, version_table_schema="rag"
    )
    with context.begin_transaction():
        context.run_migrations()


async def run_migrations_online() -> None:
    connectable = async_engine_from_config(
        config.get_section(config.config_ini_section, {}),
        prefix="sqlalchemy.",
    )
    async with connectable.connect() as connection:
        await connection.run_sync(do_run_migrations)
    await connectable.dispose()


if context.is_offline_mode():
    run_migrations_offline()
else:
    asyncio.run(run_migrations_online())
