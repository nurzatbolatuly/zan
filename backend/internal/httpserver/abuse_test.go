package httpserver_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"zan-backend/internal/httpserver"
	"zan-backend/internal/platform/bruteforce"
	"zan-backend/internal/platform/clock"
	"zan-backend/internal/platform/logger"
	"zan-backend/internal/platform/ratelimit"
)

// newRouterWithDeps — та же сборка, что newTestRouter, но принимает уже
// собранный Deps (позволяет переопределить конкретные лимиты/сторы под
// тест, не трогая newTestDeps(), которым пользуются все остальные тесты
// этого пакета на заведомо не блокирующих значениях).
func newRouterWithDeps(t *testing.T, deps httpserver.Deps) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	l := logger.New(&bytes.Buffer{}, slog.LevelDebug)
	return httpserver.NewRouter(l, deps)
}

// zan-backend-tz-v3.md §5.7: "подбор/перебор admin-secret — rate limiting +
// блокировка IP после N неудачных попыток на /admin/*".
func TestAdminAuth_BlocksIPAfterConsecutiveFailures(t *testing.T) {
	deps := newTestDeps()
	deps.AdminBruteforce = bruteforce.NewStore(3, time.Minute, clock.Real{})
	router := newRouterWithDeps(t, deps)

	adminPing := func(token string) int {
		req := httptest.NewRequest(http.MethodGet, "/admin/ping", nil)
		if token != "" {
			req.Header.Set("X-Admin-Token", token)
		}
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec.Code
	}

	for range 3 {
		require.Equal(t, http.StatusUnauthorized, adminPing("wrong-token"))
	}

	// 4-я попытка — заблокирован, даже с ПРАВИЛЬНЫМ токеном.
	req := httptest.NewRequest(http.MethodGet, "/admin/ping", nil)
	req.Header.Set("X-Admin-Token", testAdminToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusTooManyRequests, rec.Code)
	require.Contains(t, rec.Body.String(), "admin_blocked")
}

func TestAdminAuth_SuccessResetsFailureCounter(t *testing.T) {
	deps := newTestDeps()
	deps.AdminBruteforce = bruteforce.NewStore(3, time.Minute, clock.Real{})
	router := newRouterWithDeps(t, deps)

	adminPing := func(token string) int {
		req := httptest.NewRequest(http.MethodGet, "/admin/ping", nil)
		if token != "" {
			req.Header.Set("X-Admin-Token", token)
		}
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec.Code
	}

	require.Equal(t, http.StatusUnauthorized, adminPing("wrong-token"))
	require.Equal(t, http.StatusUnauthorized, adminPing("wrong-token"))
	require.Equal(t, http.StatusOK, adminPing(testAdminToken), "correct token resets the counter")
	require.Equal(t, http.StatusUnauthorized, adminPing("wrong-token"))
	require.Equal(t, http.StatusUnauthorized, adminPing("wrong-token"))
	require.Equal(t, http.StatusOK, adminPing(testAdminToken), "still not blocked — counter was reset in between")
}

// zan-backend-tz-v3.md §5.7: "массовое создание... тредов с одного IP —
// rate limiting на POST /threads по IP + по session_id".
func TestCreateThread_RateLimitedByIP(t *testing.T) {
	deps := newTestDeps()
	deps.ThreadRateLimit = ratelimit.NewStore(0, 1) // burst=1: второй запрос с того же IP всегда 429
	router := newRouterWithDeps(t, deps)
	_, token, _ := createTestSession(t, router)

	first := createThread(t, router, token, map[string]any{"service_id": "qa", "text": "первый", "input_type": "text"})
	require.NotEqual(t, http.StatusTooManyRequests, first.Code)

	second := createThread(t, router, token, map[string]any{"service_id": "qa", "text": "второй", "input_type": "text"})
	require.Equal(t, http.StatusTooManyRequests, second.Code)
	require.Contains(t, second.Body.String(), "rate_limited")
}

func TestCreateThread_RateLimitedBySession_DifferentIPsSameSessionStillBlocked(t *testing.T) {
	deps := newTestDeps()
	deps.ThreadSessionRateLimit = ratelimit.NewStore(0, 1) // burst=1 per session_id
	router := newRouterWithDeps(t, deps)
	_, token, _ := createTestSession(t, router)

	postFromIP := func(ip string) int {
		body, err := json.Marshal(map[string]any{"service_id": "qa", "text": "вопрос", "input_type": "text"})
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, "/threads", bytes.NewReader(body))
		req.Header.Set("Authorization", "Session "+token)
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = ip + ":12345"
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec.Code
	}

	require.NotEqual(t, http.StatusTooManyRequests, postFromIP("1.1.1.1"))
	// Тот же session_id, другой IP — session-барьер всё равно срабатывает.
	require.Equal(t, http.StatusTooManyRequests, postFromIP("2.2.2.2"))
}

func TestPostClientLog_RateLimitedByIP(t *testing.T) {
	deps := newTestDeps()
	deps.ClientLogRateLimit = ratelimit.NewStore(0, 1)
	router := newRouterWithDeps(t, deps)

	payload := map[string]any{"level": "INFO", "event": "x", "message": "y"}

	first := postClientLog(t, router, payload)
	require.NotEqual(t, http.StatusTooManyRequests, first.Code)

	second := postClientLog(t, router, payload)
	require.Equal(t, http.StatusTooManyRequests, second.Code)
}
