package httpserver_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func postClientLog(t *testing.T, router http.Handler, payload map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(payload)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/logs/client", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestPostClientLog_WithoutSession_Succeeds(t *testing.T) {
	router, _ := newTestRouter(t)

	rec := postClientLog(t, router, map[string]any{
		"level":   "WARN",
		"event":   "mic_permission_denied",
		"message": "User denied microphone access",
	})

	require.Equal(t, http.StatusNoContent, rec.Code, "POST /logs/client requires no session — event may predate one")
}

func TestPostClientLog_WritesToServerLog(t *testing.T) {
	router, logBuf := newTestRouter(t)

	rec := postClientLog(t, router, map[string]any{
		"session_id": "sess-abc",
		"trace_id":   "trace-from-page",
		"level":      "ERROR",
		"event":      "api_call_failed",
		"message":    "POST /threads failed",
		"context":    map[string]any{"screen": "chat"},
	})
	require.Equal(t, http.StatusNoContent, rec.Code)

	record := findLogRecord(t, logBuf, "client_event")
	require.Equal(t, "ERROR", record["level"])
	require.Equal(t, "sess-abc", record["session_id"])
	ctx := record["context"].(map[string]any)
	require.Equal(t, "api_call_failed", ctx["event"])
	require.Equal(t, "trace-from-page", ctx["client_trace_id"])
}

func TestPostClientLog_InvalidLevel_Returns400(t *testing.T) {
	router, _ := newTestRouter(t)

	rec := postClientLog(t, router, map[string]any{
		"level":   "CRITICAL",
		"event":   "x",
		"message": "y",
	})

	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestPostClientLog_MissingEvent_Returns400(t *testing.T) {
	router, _ := newTestRouter(t)

	rec := postClientLog(t, router, map[string]any{
		"level":   "INFO",
		"message": "y",
	})

	require.Equal(t, http.StatusBadRequest, rec.Code)
}
