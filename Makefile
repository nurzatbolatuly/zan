# Точка входа для всего бэкенда (backend/ + helper/ + proto/) — см.
# BACKEND_PLAN.md, Stage 0. Разработчик/CI работает через `make`, не
# вспоминает по памяти пути к venv/buf/golangci-lint под каждый сервис.

HELPER_VENV_MARKER := helper/.venv/bin/pytest

.PHONY: proto migrate migrate-down require-database-url test test-backend test-helper lint lint-backend lint-helper helper-install verify

## Статический анализ + юнит-тесты одной командой. Тот же набор шагов
## гоняет CI (backend-ci.yml).
verify: lint test

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

## DATABASE_URL — роль zan_core (не суперпользователь), без дефолта: локальной
## БД в репозитории больше нет, адрес задаётся явно (`make migrate DATABASE_URL=...`).
require-database-url:
	@test -n "$(DATABASE_URL)" || (echo "DATABASE_URL is required" >&2; exit 1)

migrate: require-database-url
	migrate -path backend/migrations -database "$(DATABASE_URL)" up

migrate-down: require-database-url
	migrate -path backend/migrations -database "$(DATABASE_URL)" down 1

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
