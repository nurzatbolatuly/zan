package httpserver_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"zan-backend/internal/domain"
	"zan-backend/internal/httpserver"
	"zan-backend/internal/platform/bruteforce"
	"zan-backend/internal/platform/clock"
	"zan-backend/internal/platform/idgen"
	"zan-backend/internal/platform/logger"
	"zan-backend/internal/platform/ratelimit"
	"zan-backend/internal/service/analytics"
	"zan-backend/internal/service/billing"
	"zan-backend/internal/service/catalog"
	"zan-backend/internal/service/clientlog"
	"zan-backend/internal/service/document"
	"zan-backend/internal/service/prompt"
	"zan-backend/internal/service/session"
	"zan-backend/internal/service/template"
	"zan-backend/internal/service/thread"
	"zan-backend/internal/wshub"
)

// fakeSessionRepo — in-memory реализация session.Repository для юнит-тестов
// httpserver, без реального Postgres (тот тестируется отдельно,
// internal/repo — интеграционными тестами на testcontainers).
type fakeSessionRepo struct {
	mu       sync.Mutex
	sessions map[string]domain.Session
}

func newFakeSessionRepo() *fakeSessionRepo {
	return &fakeSessionRepo{sessions: make(map[string]domain.Session)}
}

func (r *fakeSessionRepo) Create(_ context.Context, s domain.Session) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sessions[s.ID] = s
	return nil
}

func (r *fakeSessionRepo) GetByID(_ context.Context, id string) (domain.Session, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.sessions[id]
	if !ok {
		return domain.Session{}, session.ErrNotFound
	}
	return s, nil
}

func (r *fakeSessionRepo) Update(_ context.Context, s domain.Session) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.sessions[s.ID]; !ok {
		return session.ErrNotFound
	}
	r.sessions[s.ID] = s
	return nil
}

const (
	testAdminToken = "test-admin-token"
	// testFileMaxSizeBytes — лимит загрузок в тестах (вложения, голос, шаблоны).
	testFileMaxSizeBytes = 1 << 20
	// seededDocumentTypeID — тип документа, заранее заведённый в fakeTemplateRepo.
	seededDocumentTypeID = "00000000-0000-0000-0001-000000000001"
)

// noopFileAttacher — thread.FileAttacher для тестов, не касающихся file_ids
// (Stage 4) — ни один из них не передаёт file_ids, поэтому оба метода
// недостижимы (thread.Service вызывает их только при непустом списке).
type noopFileAttacher struct{}

func (noopFileAttacher) ValidateAvailable(context.Context, string, []string) error { return nil }
func (noopFileAttacher) AttachToMessage(context.Context, string, []string) error   { return nil }

// fakeAgent — thread.Agent для тестов httpserver, которым не важно
// поведение реального internal/agent.Client (Stage 5) — ни один тест в
// этом файле не проверяет содержимое ответа ассистента, только маршрутизацию/
// авторизацию/сериализацию остальных роутов, поэтому детерминированная
// заглушка здесь достаточна (то, что раньше делал thread.StubAgent, Stage 3,
// удалённый вместе с подключением реального агента).
type fakeAgent struct{}

func (fakeAgent) Process(context.Context, thread.AgentRequest) (thread.AgentResult, error) {
	return thread.AgentResult{Status: thread.AgentResultDone, AnswerText: "ok"}, nil
}

func newTestDeps() httpserver.Deps {
	sessions := session.New(newFakeSessionRepo(), clock.Real{}, idgen.UUIDGenerator{}, session.NewTokenSigner("test-secret"))
	cat := catalog.New(newFakeCatalogRepo(), idgen.UUIDGenerator{})
	bill := billing.New(newFakeBillingRepo(), cat, clock.Real{}, idgen.UUIDGenerator{})
	threadRepo := newFakeThreadRepo()
	// hub — Stage 9, реальный *wshub.Hub (не фейк — сам уже покрыт
	// internal/wshub/hub_test.go), общий для thread.Service (EventPublisher)
	// и httpserver.Deps.Hub, как в cmd/api/main.go.
	hub := wshub.NewHub()
	th := thread.New(threadRepo, bill, cat, noopFileAttacher{}, fakeAgent{}, hub, clock.Real{}, idgen.UUIDGenerator{})
	prompts := prompt.New(newFakePromptRepo(), clock.Real{})
	// documentSvc — Stage 6, реиспользует тот же threadRepo, что и th (fake
	// удовлетворяет document.ThreadStore структурно — тот же приём, что и в
	// cmd/api/main.go с repo.ThreadRepo), чтобы тред, созданный через
	// createThreadHandler в тесте, был виден generate-document/GET document.
	documentSvc := document.New(threadRepo, newFakeDocumentFileStore(), bill, cat, newFakeDocumentGenerator(), &fakeDocumentRenderer{}, fakeDocumentStorage{}, clock.Real{}, idgen.UUIDGenerator{})
	templateSvc := template.New(newFakeTemplateRepo(), fakeTemplateStorage{}, fakeTemplateConverter{}, clock.Real{}, idgen.UUIDGenerator{}, testFileMaxSizeBytes)
	analyticsSvc := analytics.New(newFakeAnalyticsRepo())
	clientLogSvc := clientlog.New()
	return httpserver.Deps{
		Sessions:            sessions,
		Catalog:             cat,
		Billing:             bill,
		Thread:              th,
		Hub:                 hub,
		Documents:           documentSvc,
		Prompts:             prompts,
		Templates:           templateSvc,
		Analytics:           analyticsSvc,
		FileMaxSizeBytes:    testFileMaxSizeBytes,
		ClientLogs:          clientLogSvc,
		AdminToken:          testAdminToken,
		SessionCookieSecure: false,
		// Лимиты в тестах — заведомо выше, чем любой сценарий одного теста
		// может реально задеть (тестируется само наличие барьера — отдельные
		// тесты ниже, с собственными router/Deps на малых лимитах, не через
		// этот общий newTestDeps).
		SessionRateLimit:       ratelimit.NewStore(1000, 1000),
		ThreadRateLimit:        ratelimit.NewStore(1000, 1000),
		ThreadSessionRateLimit: ratelimit.NewStore(1000, 1000),
		ClientLogRateLimit:     ratelimit.NewStore(1000, 1000),
		AdminBruteforce:        bruteforce.NewStore(1000, time.Minute, clock.Real{}),
	}
}

// parseLogRecords разбирает буфер JSON-логов (одна запись — одна строка,
// как и пишет slog.JSONHandler) на отдельные записи — нужно там, где за
// один запрос пишется больше одной строки лога (AccessLog + Recovery).
func parseLogRecords(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var records []map[string]any
	scanner := bufio.NewScanner(bytes.NewReader(buf.Bytes()))
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var record map[string]any
		require.NoError(t, json.Unmarshal(line, &record))
		records = append(records, record)
	}
	require.NoError(t, scanner.Err())
	return records
}

func findLogRecord(t *testing.T, buf *bytes.Buffer, message string) map[string]any {
	t.Helper()
	for _, record := range parseLogRecords(t, buf) {
		if record["message"] == message {
			return record
		}
	}
	t.Fatalf("no log record with message %q found in:\n%s", message, buf.String())
	return nil
}

func newTestRouter(t *testing.T) (*gin.Engine, *bytes.Buffer) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	var logBuf bytes.Buffer
	l := logger.New(&logBuf, slog.LevelDebug)
	return httpserver.NewRouter(l, newTestDeps()), &logBuf
}

func TestHealthz_ReturnsOK(t *testing.T) {
	router, _ := newTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var body map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, "ok", body["status"])
}

func TestHealthz_GeneratesTraceIDWhenMissing(t *testing.T) {
	router, logBuf := newTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	traceID := rec.Header().Get("X-Trace-Id")
	require.NotEmpty(t, traceID)
	require.Contains(t, logBuf.String(), traceID)
}

func TestHealthz_PropagatesIncomingTraceID(t *testing.T) {
	router, _ := newTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.Header.Set("X-Trace-Id", "trace-from-frontend")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, "trace-from-frontend", rec.Header().Get("X-Trace-Id"))
}

func TestAccessLog_LogsMethodPathStatus(t *testing.T) {
	router, logBuf := newTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	record := findLogRecord(t, logBuf, "http_request_completed")
	ctx := record["context"].(map[string]any)
	require.Equal(t, "GET", ctx["method"])
	require.Equal(t, "/healthz", ctx["path"])
	require.EqualValues(t, http.StatusOK, ctx["status"])
}

func TestRecovery_ConvertsPanicToInternalError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var logBuf bytes.Buffer
	l := logger.New(&logBuf, slog.LevelDebug)

	router := gin.New()
	router.Use(httpserver.RequestContext(l), httpserver.AccessLog(), httpserver.Recovery())
	router.GET("/boom", func(c *gin.Context) {
		panic("something exploded")
	})

	req := httptest.NewRequest(http.MethodGet, "/boom", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusInternalServerError, rec.Code)
	require.NotContains(t, rec.Body.String(), "something exploded", "клиент не должен видеть сырой текст паники")

	record := findLogRecord(t, &logBuf, "panic_recovered")
	require.Equal(t, "CRITICAL", record["level"])
}
