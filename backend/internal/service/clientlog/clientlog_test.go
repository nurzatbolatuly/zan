package clientlog_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"

	"zan-backend/internal/platform/logger"
	"zan-backend/internal/service/clientlog"
)

func TestParseLevel_AcceptsKnownValues(t *testing.T) {
	for _, raw := range []string{"INFO", "WARN", "ERROR"} {
		level, err := clientlog.ParseLevel(raw)
		require.NoError(t, err)
		require.Equal(t, clientlog.Level(raw), level)
	}
}

func TestParseLevel_RejectsUnknownValues(t *testing.T) {
	for _, raw := range []string{"", "DEBUG", "CRITICAL", "info"} {
		_, err := clientlog.ParseLevel(raw)
		require.ErrorIs(t, err, clientlog.ErrInvalidLevel)
	}
}

func newTestContext(buf *bytes.Buffer) context.Context {
	l := logger.New(buf, slog.LevelDebug)
	return logger.WithContext(context.Background(), l)
}

func TestLog_EmptyEvent_ReturnsError(t *testing.T) {
	svc := clientlog.New()

	err := svc.Log(newTestContext(&bytes.Buffer{}), clientlog.Entry{Level: clientlog.LevelInfo, Event: "  ", Message: "x"})

	require.ErrorIs(t, err, clientlog.ErrEmptyEvent)
}

func TestLog_EmptyMessage_ReturnsError(t *testing.T) {
	svc := clientlog.New()

	err := svc.Log(newTestContext(&bytes.Buffer{}), clientlog.Entry{Level: clientlog.LevelInfo, Event: "x", Message: " "})

	require.ErrorIs(t, err, clientlog.ErrEmptyMessage)
}

func TestLog_WritesStructuredRecordAtRequestedLevel(t *testing.T) {
	var buf bytes.Buffer
	svc := clientlog.New()

	err := svc.Log(newTestContext(&buf), clientlog.Entry{
		SessionID: "sess-1",
		TraceID:   "trace-from-the-page",
		Level:     clientlog.LevelWarn,
		Event:     "mic_permission_denied",
		Message:   "User denied microphone access",
		Context:   map[string]any{"screen": "chat"},
	})
	require.NoError(t, err)

	var record map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &record))
	require.Equal(t, "WARN", record["level"])
	require.Equal(t, "client_event", record["message"])
	require.Equal(t, "sess-1", record["session_id"])

	ctx := record["context"].(map[string]any)
	require.Equal(t, "mic_permission_denied", ctx["event"])
	require.Equal(t, "User denied microphone access", ctx["client_message"])
	require.Equal(t, "trace-from-the-page", ctx["client_trace_id"])
	clientCtx := ctx["client_context"].(map[string]any)
	require.Equal(t, "chat", clientCtx["screen"])
}

func TestLog_ErrorLevel_MapsToErrorSeverity(t *testing.T) {
	var buf bytes.Buffer
	svc := clientlog.New()

	require.NoError(t, svc.Log(newTestContext(&buf), clientlog.Entry{
		Level: clientlog.LevelError, Event: "api_call_failed", Message: "POST /threads failed",
	}))

	var record map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &record))
	require.Equal(t, "ERROR", record["level"])
}

func TestLog_WithoutSessionID_OmitsField(t *testing.T) {
	var buf bytes.Buffer
	svc := clientlog.New()

	require.NoError(t, svc.Log(newTestContext(&buf), clientlog.Entry{
		Level: clientlog.LevelInfo, Event: "render_error", Message: "before any session exists",
	}))

	var record map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &record))
	_, hasSessionID := record["session_id"]
	require.False(t, hasSessionID)
}
