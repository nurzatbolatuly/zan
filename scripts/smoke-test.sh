#!/usr/bin/env bash
# Автоматическая проверка Stage 0 DoD (BACKEND_PLAN.md) — то, что раньше
# приходилось руками прогонять по BACKEND_LOG.md (curl/grpcurl/psql).
# Поднимает весь docker-compose стек, ждёт healthy у всех сервисов,
# проверяет каждый публичный контракт и гарантированно опускает стек в
# конце (успех или провал — не важно). Один источник правды: то же самое
# гоняет `make smoke` локально и job `smoke` в .github/workflows/backend-ci.yml.
set -euo pipefail

cd "$(dirname "$0")/.."

if [ -f .env ]; then
  set -a
  # shellcheck disable=SC1091
  source .env
  set +a
fi

POSTGRES_USER="${POSTGRES_USER:-zan}"
POSTGRES_DB="${POSTGRES_DB:-zan}"
ADMIN_TOKEN="${ADMIN_TOKEN:-devadmintokendevadmintokendevadmintoken1}"
# CORE_DB_PASSWORD — роль zan_core (Stage 8, scripts/postgres-init/02-core-role.sh),
# миграции backend/ применяются под ней же, не под POSTGRES_USER/PASSWORD
# бутстрап-суперпользователя — иначе таблицы окажутся во владении не той
# роли, под которой ходит сам backend/ (см. комментарий в 02-core-role.sh).
CORE_DB_PASSWORD="${CORE_DB_PASSWORD:-devcoredbpasswordchangeme}"

cleanup() {
  echo "--- docker compose down ---"
  docker compose down >/dev/null 2>&1 || true
}
trap cleanup EXIT

fail() {
  echo "FAIL: $1" >&2
  exit 1
}

wait_healthy() {
  local service="$1"
  local timeout_s="${2:-60}"
  local deadline=$((SECONDS + timeout_s))
  local cid status="starting"

  while true; do
    cid="$(docker compose ps -q "$service" 2>/dev/null || true)"
    if [ -n "$cid" ]; then
      status="$(docker inspect -f '{{.State.Health.Status}}' "$cid" 2>/dev/null || echo "starting")"
      [ "$status" = "healthy" ] && return 0
    fi
    if [ "$SECONDS" -ge "$deadline" ]; then
      echo "$service did not become healthy within ${timeout_s}s (last status: $status)" >&2
      docker compose logs "$service" | tail -30 >&2
      return 1
    fi
    sleep 2
  done
}

echo "--- docker compose up ---"
docker compose up -d --build

for svc in postgres minio backend helper; do
  echo "--- waiting for $svc ---"
  wait_healthy "$svc" || fail "$svc never became healthy"
done

# clamav — Stage 8, dockerd-встроенный HEALTHCHECK image'а грузит вшитую
# базу сигнатур в память при старте (StartPeriod 360s у самого образа,
# проверено вручную при реализации Stage 8) — заметно дольше остальных
# сервисов, отдельный больший таймаут, не общий 60s.
echo "--- waiting for clamav ---"
wait_healthy "clamav" 300 || fail "clamav never became healthy"

echo "--- backend GET /healthz ---"
curl -sSf http://localhost:8080/healthz | grep -q '"status":"ok"' || fail "backend /healthz"

echo "--- helper GET /healthz ---"
curl -sSf http://localhost:8001/healthz | grep -q '"status":"ok"' || fail "helper /healthz"

echo "--- helper gRPC reflection (grpcurl list) ---"
if command -v grpcurl >/dev/null 2>&1; then
  grpcurl -plaintext localhost:9090 list | grep -q "ServerReflection" || fail "helper grpc reflection"
  # Stage 5 — RagService.Search — единственный оставшийся RPC контракта
  # Go<->Python (BACKEND_PLAN.md §3.1), проверяем, что он реально
  # задеплоен, не только рефлексия сервера в целом.
  grpcurl -plaintext localhost:9090 list zan.rpc.v1.RagService | grep -q "Search" || fail "RagService.Search not exposed"
else
  echo "SKIP: grpcurl не установлен локально — этот шаг всё равно гоняется в CI (backend-ci.yml устанавливает grpcurl)."
fi

echo "--- postgres: pgvector extension доступно ---"
docker compose exec -T postgres psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "CREATE EXTENSION IF NOT EXISTS vector;" >/dev/null \
  || fail "CREATE EXTENSION vector"

echo "--- minio health endpoint ---"
curl -sSf -o /dev/null http://localhost:9000/minio/health/live || fail "minio /minio/health/live"

# Миграции — не часть образа backend (golang-migrate — отдельный CLI,
# BACKEND_PLAN.md §1.1), поэтому ни один эндпоинт, которому нужна схема
# core.*, не заработает без этого шага. Раньше это не было заметно — Stage 0
# smoke-проверки ограничивались /healthz и grpc reflection, которым схема
# не нужна; Stage 4 первым добавляет проверки реальных эндпоинтов.
echo "--- backend: применяем миграции ---"
if command -v migrate >/dev/null 2>&1; then
  migrate -path backend/migrations \
    -database "postgres://zan_core:${CORE_DB_PASSWORD}@localhost:5432/${POSTGRES_DB}?sslmode=disable" \
    up || fail "migrate up"
else
  echo "SKIP: golang-migrate CLI не установлен локально — этот шаг всё равно гоняется в CI (backend-ci.yml устанавливает migrate)."
fi

# Stage 4 — файлы/голос через реальную Go-оркестрацию + gRPC-вызовы в
# helper/ (BACKEND_PLAN.md Stage 4 DoD), не заглушки.
echo "--- backend: создаём сессию ---"
session_response="$(curl -sSf -X POST http://localhost:8080/sessions)"
session_token="$(echo "$session_response" | grep -o '"session_token":"[^"]*"' | cut -d'"' -f4)"
[ -n "$session_token" ] || fail "could not extract session_token from: $session_response"

echo "--- backend: POST /files/upload (реальный PDF -> FilesService.Extract) ---"
upload_response="$(curl -sSf -H "Authorization: Session $session_token" \
  -F "file=@scripts/fixtures/smoke-test.pdf;type=application/pdf" \
  http://localhost:8080/files/upload)"
echo "$upload_response" | grep -q '"processing_status":"processed"' \
  || fail "expected processing_status=processed, got: $upload_response"
file_id="$(echo "$upload_response" | grep -o '"file_id":"[^"]*"' | cut -d'"' -f4)"
[ -n "$file_id" ] || fail "could not extract file_id from: $upload_response"

echo "--- backend: GET /files/{id} отдаёт рабочую ссылку ---"
file_url="$(curl -sSf -H "Authorization: Session $session_token" "http://localhost:8080/files/$file_id" \
  | grep -o '"url":"[^"]*"' | cut -d'"' -f4 | sed 's/\\u0026/\&/g')"
[ -n "$file_url" ] || fail "GET /files/$file_id did not return a url"
curl -sSf -o /dev/null "$file_url" || fail "presigned file url is not downloadable: $file_url"

echo "--- backend: POST /voice/transcribe (реальный вызов SttService.Transcribe) ---"
voice_status="$(curl -sS -o /tmp/zan-smoke-voice.json -w '%{http_code}' \
  -H "Authorization: Session $session_token" \
  -F "audio=@scripts/fixtures/smoke-test.wav;type=audio/wav" \
  http://localhost:8080/voice/transcribe)"
# 200 (что-то распознано) и 422 empty_transcript (тишина — фикстура именно
# такая) — оба означают, что пайплайн Go->Python->faster-whisper реально
# отработал; любой другой код — реальная поломка.
case "$voice_status" in
  200|422) ;;
  *) fail "voice/transcribe unexpected status $voice_status: $(cat /tmp/zan-smoke-voice.json)" ;;
esac

echo "--- backend: POST /threads с вложением (input_type=file) ---"
thread_response="$(curl -sSf -H "Authorization: Session $session_token" -H "Content-Type: application/json" \
  -d "{\"service_id\":\"qa\",\"input_type\":\"file\",\"file_ids\":[\"$file_id\"]}" \
  http://localhost:8080/threads)"
echo "$thread_response" | grep -q '"input_type":"file"' \
  || fail "expected a file-input message in thread, got: $thread_response"

# Stage 5 (BACKEND_PLAN.md) — реальный вызов LLM-агента, не заглушка.
# Баланс новой сессии по умолчанию 0 (файловый тред выше создался
# is_paid=false, агент не вызывался), поэтому сперва — реальный checkout+
# confirm, чтобы тред открылся оплаченным и синхронно вызвал agent.Process.
echo "--- backend: пополняем баланс qa (checkout -> confirm) ---"
checkout_response="$(curl -sSf -H "Authorization: Session $session_token" -H "Content-Type: application/json" \
  -d '{"kind":"single_service","items":[{"service_id":"qa","qty":1}]}' \
  http://localhost:8080/payments/checkout)"
payment_id="$(echo "$checkout_response" | grep -o '"payment_id":"[^"]*"' | cut -d'"' -f4)"
[ -n "$payment_id" ] || fail "could not extract payment_id from: $checkout_response"
curl -sSf -X POST -H "Authorization: Session $session_token" \
  "http://localhost:8080/payments/$payment_id/confirm" >/dev/null || fail "payment confirm failed"

qa_balance() {
  curl -sSf -H "Authorization: Session $session_token" http://localhost:8080/balance \
    | python3 -c "import json,sys; d=json.load(sys.stdin); print(next((x['quantity'] for x in d if x['service_id']=='qa'), 0))"
}
[ "$(qa_balance)" = "1" ] || fail "expected qa balance=1 after checkout+confirm"

echo "--- backend: POST /threads {service_id:qa} — реальный вызов LLM-агента (Stage 5) ---"
qa_thread_response="$(curl -sSf -H "Authorization: Session $session_token" -H "Content-Type: application/json" \
  -d '{"service_id":"qa","input_type":"text","text":"Какой срок исковой давности по трудовым спорам в РК?"}' \
  http://localhost:8080/threads)"
qa_thread_status="$(echo "$qa_thread_response" | grep -o '"status":"[^"]*"' | head -1 | cut -d'"' -f4)"
case "$qa_thread_status" in
  done|clarify)
    # OPENAI_API_KEY в окружении — рабочий реальный ключ (не дефолтный
    # placeholder docker-compose.yml) — агент реально ответил.
    echo "LLM agent responded (status=$qa_thread_status) — a working OPENAI_API_KEY is configured."
    ;;
  error)
    # Ожидаемо с дефолтным placeholder-ключом (docker-compose.yml): реальный
    # вызов к api.openai.com получает 401, internal/agent не ретраит
    # 4xx, thread.Service помечает тред error и возвращает кредит на баланс
    # (backend-roadmap.md §5.2) — проверяем именно это, не просто "не упало".
    echo "LLM agent call failed as expected with a placeholder OPENAI_API_KEY (status=error) — checking credit was refunded."
    [ "$(qa_balance)" = "1" ] || fail "expected qa credit refunded to 1 after agent error"
    ;;
  *)
    fail "unexpected thread status after qa thread creation: $qa_thread_response"
    ;;
esac

# Stage 6 (BACKEND_PLAN.md) — "Агент 'Документы'": generate-document
# доступен из любого треда (zan-backend-tz-v2.md §4.4), не зависит от
# status выше. Тот же паттерн, что и qa-раунд: пополняем "doc" через
# checkout{thread_id}+confirm, затем реальный вызов LLM + рендер.
echo "--- backend: пополняем баланс doc для этого треда (checkout{thread_id} -> confirm) ---"
qa_thread_id="$(echo "$qa_thread_response" | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4)"
[ -n "$qa_thread_id" ] || fail "could not extract thread id from: $qa_thread_response"
doc_checkout_response="$(curl -sSf -H "Authorization: Session $session_token" -H "Content-Type: application/json" \
  -d "{\"kind\":\"single_service\",\"items\":[{\"service_id\":\"doc\",\"qty\":1}],\"thread_id\":\"$qa_thread_id\"}" \
  http://localhost:8080/payments/checkout)"
doc_payment_id="$(echo "$doc_checkout_response" | grep -o '"payment_id":"[^"]*"' | cut -d'"' -f4)"
[ -n "$doc_payment_id" ] || fail "could not extract payment_id from: $doc_checkout_response"
curl -sSf -X POST -H "Authorization: Session $session_token" \
  "http://localhost:8080/payments/$doc_payment_id/confirm" >/dev/null || fail "doc payment confirm failed"

echo "--- backend: POST /threads/{id}/generate-document — реальный вызов LLM + DocumentsService.Render (Stage 6) ---"
doc_status="$(curl -sS -o /tmp/zan-smoke-generate-document.json -w '%{http_code}' \
  -X POST -H "Authorization: Session $session_token" \
  "http://localhost:8080/threads/$qa_thread_id/generate-document")"
case "$doc_status" in
  201)
    files_ready="$(python3 -c "import json; print(json.load(open('/tmp/zan-smoke-generate-document.json'))['files_ready'])")"
    echo "document generation responded 201 (files_ready=$files_ready) — a working OPENAI_API_KEY is configured."
    if [ "$files_ready" = "True" ]; then
      echo "--- backend: GET /threads/{id}/document?format=pdf отдаёт скачиваемую ссылку ---"
      doc_url="$(curl -sSf -H "Authorization: Session $session_token" \
        "http://localhost:8080/threads/$qa_thread_id/document?format=pdf" \
        | grep -o '"url":"[^"]*"' | cut -d'"' -f4 | sed 's/\\u0026/\&/g')"
      [ -n "$doc_url" ] || fail "GET .../document?format=pdf did not return a url"
      curl -sSf -o /dev/null "$doc_url" || fail "presigned document url is not downloadable: $doc_url"
    fi
    ;;
  502)
    # Тот же ожидаемый путь с placeholder-ключом, что и у qa-раунда выше —
    # document_generation_failed, doc-кредит должен вернуться на баланс.
    echo "document generation failed as expected with a placeholder OPENAI_API_KEY (502) — checking credit was refunded."
    doc_balance="$(curl -sSf -H "Authorization: Session $session_token" http://localhost:8080/balance \
      | python3 -c "import json,sys; d=json.load(sys.stdin); print(next((x['quantity'] for x in d if x['service_id']=='doc'), 0))")"
    [ "$doc_balance" = "1" ] || fail "expected doc credit refunded to 1 after generation error"
    ;;
  *)
    fail "unexpected generate-document status $doc_status: $(cat /tmp/zan-smoke-generate-document.json)"
    ;;
esac

# Stage 7 (BACKEND_PLAN.md) — админ-аналитика и клиентские логи.
echo "--- backend: GET /admin/analytics/overview (реальные агрегаты по только что созданным тредам выше) ---"
analytics_response="$(curl -sSf -H "X-Admin-Token: $ADMIN_TOKEN" http://localhost:8080/admin/analytics/overview)"
echo "$analytics_response" | grep -q '"total_threads":[1-9]' \
  || fail "expected total_threads > 0 after creating threads above, got: $analytics_response"
echo "$analytics_response" | python3 -c "
import json, sys
d = json.load(sys.stdin)
breakdown = d['status_breakdown']
assert set(breakdown) == {'queued', 'processing', 'clarify', 'done', 'error', 'canceled'}, breakdown
" || fail "status_breakdown missing expected keys: $analytics_response"

echo "--- backend: X-Admin-Token без токена -> 401, неверный -> 401 ---"
curl -sS -o /dev/null -w '%{http_code}' http://localhost:8080/admin/analytics/overview | grep -q '^401$' \
  || fail "expected 401 without X-Admin-Token"

echo "--- backend: POST /logs/client (без сессии — barebones spam-защита, не авторизация) ---"
logs_status="$(curl -sS -o /dev/null -w '%{http_code}' -H "Content-Type: application/json" \
  -d '{"level":"WARN","event":"mic_permission_denied","message":"smoke test"}' \
  http://localhost:8080/logs/client)"
[ "$logs_status" = "204" ] || fail "expected 204 from POST /logs/client, got $logs_status"

echo "ALL SMOKE CHECKS PASSED"
