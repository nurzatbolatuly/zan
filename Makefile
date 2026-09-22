# Точка входа для всего бэкенда (backend/ + helper/ + proto/) — см.
# BACKEND_PLAN.md, Stage 0. Разработчик/CI работает через `make`, не
# вспоминает по памяти пути к venv/buf/golangci-lint под каждый сервис.

# DATABASE_URL — роль zan_core (Stage 8, не бутстрап-суперпользователь
# POSTGRES_USER) — заводится scripts/postgres-init/02-core-role.sh, тот же
# приём, что RAG_DATABASE_URL/zan_rag ниже (BACKEND_PLAN.md §1.3).
DATABASE_URL ?= postgres://zan_core:devcoredbpasswordchangeme@localhost:5432/zan?sslmode=disable
# RAG_DATABASE_URL — роль zan_rag (не DATABASE_URL) — заводится
# scripts/postgres-init/01-rag-role.sh (Stage 5, BACKEND_PLAN.md §1.3).
RAG_DATABASE_URL ?= postgres://zan_rag:devragdbpasswordchangeme@localhost:5432/zan
HELPER_VENV_MARKER := helper/.venv/bin/pytest

.PHONY: up down logs smoke load-test proto migrate migrate-down rag-migrate rag-migrate-down rag-index test test-backend test-helper lint lint-backend lint-helper helper-install verify

## --- docker-compose ---

up:
	docker compose up -d --build

down:
	docker compose down

logs:
	docker compose logs -f

## Полная сквозная проверка Stage 0 (docker compose up + healthz + grpcurl +
## pgvector) одной командой — см. scripts/smoke-test.sh. Сам поднимает и
## гарантированно опускает стек, ничего не оставляет висеть после себя.
smoke:
	./scripts/smoke-test.sh

## Нагрузочный smoke-тест POST /threads -> LLM -> ответ (Stage 8,
## BACKEND_PLAN.md §7) — умеренная конкурентная нагрузка на реальный
## docker-compose стек, не бенчмарк. См. scripts/load-test.sh.
load-test:
	./scripts/load-test.sh

## Всё, что нужно перед PR, одной командой: статический анализ + юнит-тесты
## + реальный докер-стек. Тот же набор шагов гоняет CI (backend-ci.yml).
verify: lint test smoke

## --- proto (BACKEND_PLAN.md §3.2) ---
## Go — через buf (proto/buf.gen.yaml). Python — отдельным вызовом
## grpc_tools.protoc (не через buf, см. proto/buf.gen.yaml — офлайн-совместимо,
## не требует buf remote-плагинов).

proto: $(HELPER_VENV_MARKER)
	cd proto && buf lint && buf generate
	cd helper && .venv/bin/python -m grpc_tools.protoc \
		-I ../proto \
		--python_out=app/genproto \
		--grpc_python_out=app/genproto \
		--pyi_out=app/genproto \
		$$(find ../proto -name '*.proto')

## --- migrations (golang-migrate CLI — BACKEND_PLAN.md §1.1, не встроено в бинарь) ---

migrate:
	migrate -path backend/migrations -database "$(DATABASE_URL)" up

migrate-down:
	migrate -path backend/migrations -database "$(DATABASE_URL)" down 1

## --- rag schema migrations (Alembic — BACKEND_PLAN.md §1.3, Stage 5) ---
## Отдельный CLI-шаг от рантайма helper/, тот же принцип, что migrate/
## migrate-down выше, но своим инструментом (схему rag мигрирует только
## Python) и своей ролью (RAG_DATABASE_URL, не DATABASE_URL).

rag-migrate: $(HELPER_VENV_MARKER)
	cd helper && RAG_DATABASE_URL="$(RAG_DATABASE_URL)" .venv/bin/alembic upgrade head

rag-migrate-down: $(HELPER_VENV_MARKER)
	cd helper && RAG_DATABASE_URL="$(RAG_DATABASE_URL)" .venv/bin/alembic downgrade -1

## Индексация сид-корпуса НПА (scripts/fixtures/rag_seed_corpus.json) —
## batch job, не RPC (BACKEND_PLAN.md §3.1). Требует rag-migrate заранее.
rag-index: $(HELPER_VENV_MARKER)
	cd helper && RAG_DATABASE_URL="$(RAG_DATABASE_URL)" .venv/bin/python scripts/index_corpus.py

## --- helper venv (dev-инструменты: pytest/ruff/mypy/grpc_tools) ---

$(HELPER_VENV_MARKER): helper/pyproject.toml
	cd helper && python3 -m venv .venv
	cd helper && .venv/bin/pip install -q --upgrade pip
	cd helper && .venv/bin/pip install -q -e ".[dev]"

helper-install: $(HELPER_VENV_MARKER)

## --- test ---

test: test-backend test-helper

test-backend:
	cd backend && go test ./...

test-helper: helper-install
	cd helper && .venv/bin/python -m pytest

## --- lint ---

lint: lint-backend lint-helper

lint-backend:
	cd backend && go vet ./...
	cd backend && golangci-lint run ./...

lint-helper: helper-install
	cd helper && .venv/bin/ruff check .
	cd helper && .venv/bin/ruff format --check .
	cd helper && .venv/bin/mypy app tests
