#!/usr/bin/env bash
# Нагрузочный smoke-тест пути POST /threads -> LLM -> ответ (Stage 8,
# BACKEND_PLAN.md §7: "Нагрузочный smoke-тест пути POST /threads -> LLM ->
# ответ"). Не бенчмарк и не нагрузочный тест в духе k6/hey — сознательно
# без сторонней зависимости (bash+curl, тот же приём, что и
# scripts/smoke-test.sh), проверяет, что путь держит умеренную
# конкурентную нагрузку РЕАЛЬНОГО стека (не моков) и не роняет процесс, не
# зависает, не отвечает 5xx там, где ожидается управляемый отказ.
#
# Масштаб теста ограничен НАМЕРЕННО самим API, не ленью: POST /sessions
# лимитирован per-IP (burst 5, дальше 1/10s — internal/config через
# cmd/api/main.go, sessionCreateRateLimit), поэтому SESSIONS ниже не
# больше 5 — иначе тест сам себе гарантирует 429 на создании сессий и
# ничего не проверяет про путь POST /threads. THREADS_PER_SESSION — в
# пределах burst для POST /threads по session_id (burst 3,
# threadCreateRateBurstSession) — тот же принцип.
set -euo pipefail

cd "$(dirname "$0")/.."

if [ -f .env ]; then
  set -a
  # shellcheck disable=SC1091
  source .env
  set +a
fi

SESSIONS="${LOAD_TEST_SESSIONS:-5}"
THREADS_PER_SESSION="${LOAD_TEST_THREADS_PER_SESSION:-3}"
BASE_URL="${LOAD_TEST_BASE_URL:-http://localhost:8080}"

WORKDIR="$(mktemp -d)"
cleanup() {
  echo "--- docker compose down ---"
  docker compose down >/dev/null 2>&1 || true
  rm -rf "$WORKDIR"
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
      return 1
    fi
    sleep 2
  done
}

echo "--- docker compose up ---"
docker compose up -d --build

for svc in postgres minio backend helper; do
  wait_healthy "$svc" || fail "$svc never became healthy"
done
wait_healthy "clamav" 300 || fail "clamav never became healthy"

if command -v migrate >/dev/null 2>&1; then
  CORE_DB_PASSWORD="${CORE_DB_PASSWORD:-devcoredbpasswordchangeme}"
  migrate -path backend/migrations \
    -database "postgres://zan_core:${CORE_DB_PASSWORD}@localhost:5432/${POSTGRES_DB:-zan}?sslmode=disable" \
    up || fail "migrate up"
else
  fail "golang-migrate CLI не установлен — обязателен для load-test.sh (в отличие от smoke-test.sh, здесь без graceful skip: без схемы core тест ничего не измеряет)"
fi

# run_session — один "виртуальный пользователь": своя сессия, разовая
# оплата на THREADS_PER_SESSION тредов, затем THREADS_PER_SESSION
# последовательных POST /threads (реальный вызов LLM-агента внутри
# каждого — thread.Service.Process синхронен в пределах одного HTTP-
# запроса, BACKEND_LOG.md Stage 5). Пишет "<http_status> <latency_s>" по
# одной строке на тред в свой файл — параллельные сессии пишут в разные
# файлы, гонки за один файл не нужно.
run_session() {
  local idx="$1"
  local outfile="$2"
  local session_token
  session_token="$(curl -sSf -X POST "$BASE_URL/sessions" | python3 -c "import json,sys; print(json.load(sys.stdin)['session_token'])")" \
    || { echo "session_create_failed" >>"$outfile"; return; }

  local checkout_response payment_id
  checkout_response="$(curl -sSf -H "Authorization: Session $session_token" -H "Content-Type: application/json" \
    -d "{\"kind\":\"single_service\",\"items\":[{\"service_id\":\"qa\",\"qty\":$THREADS_PER_SESSION}]}" \
    "$BASE_URL/payments/checkout")" || { echo "checkout_failed" >>"$outfile"; return; }
  payment_id="$(echo "$checkout_response" | python3 -c "import json,sys; print(json.load(sys.stdin)['payment_id'])" 2>/dev/null)" \
    || { echo "checkout_bad_response" >>"$outfile"; return; }
  curl -sSf -X POST -H "Authorization: Session $session_token" \
    "$BASE_URL/payments/$payment_id/confirm" >/dev/null || { echo "confirm_failed" >>"$outfile"; return; }

  local n
  for n in $(seq 1 "$THREADS_PER_SESSION"); do
    local started status_code elapsed
    started=$(date +%s.%N)
    status_code="$(curl -sS -o /dev/null -w '%{http_code}' \
      -H "Authorization: Session $session_token" -H "Content-Type: application/json" \
      -d "{\"service_id\":\"qa\",\"input_type\":\"text\",\"text\":\"load-test session=$idx thread=$n\"}" \
      "$BASE_URL/threads")"
    elapsed=$(echo "$(date +%s.%N) - $started" | bc)
    echo "$status_code $elapsed" >>"$outfile"
  done
}

echo "--- firing $SESSIONS concurrent sessions x $THREADS_PER_SESSION POST /threads each ---"
pids=()
for i in $(seq 1 "$SESSIONS"); do
  run_session "$i" "$WORKDIR/session-$i.out" &
  pids+=("$!")
done
for pid in "${pids[@]}"; do
  wait "$pid" || true # индивидуальные curl-ошибки уже записаны в outfile, здесь не проваливаем весь тест
done

all_results="$WORKDIR/all.out"
cat "$WORKDIR"/session-*.out >"$all_results" 2>/dev/null || true

total=$(wc -l <"$all_results" | tr -d ' ')
[ "$total" -gt 0 ] || fail "no results collected at all — every session failed before reaching POST /threads"

echo "--- results ($total requests across $SESSIONS sessions) ---"
echo "status code distribution:"
awk '{print $1}' "$all_results" | sort | uniq -c | sort -rn

created=$(awk '$1 == 201' "$all_results" | wc -l | tr -d ' ')
rate_limited=$(awk '$1 == 429' "$all_results" | wc -l | tr -d ' ')
server_errors=$(awk '$1 ~ /^5/ && $1 != 502' "$all_results" | wc -l | tr -d ' ')
# 502 (document_generation_failed) не применим к этому пути (только qa,
# не generate-document) — оставлено на будущее, если сценарий расширят.

echo "created=$created rate_limited(429)=$rate_limited unexpected_5xx=$server_errors"

awk '$2 != "" {sum+=$2; n++; if ($2>max || n==1) max=$2} END {if (n>0) printf "latency: avg=%.3fs max=%.3fs (n=%d)\n", sum/n, max, n}' "$all_results"

[ "$server_errors" -eq 0 ] || fail "$server_errors request(s) returned an unexpected 5xx — backend/ должен либо создать тред (201, status=error внутри — placeholder ANTHROPIC_API_KEY), либо корректно отклонить (429), не падать с internal_error"
[ "$created" -gt 0 ] || fail "zero requests returned 201 — path is fully broken, not just rate-limited"

echo "LOAD TEST PASSED"
