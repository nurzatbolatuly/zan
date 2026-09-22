package logger_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"

	"zan-backend/internal/platform/logger"
)

func TestNew_ContractFieldNames(t *testing.T) {
	var buf bytes.Buffer
	l := logger.New(&buf, slog.LevelDebug)

	l.Info("thread_status_changed", slog.Group("context", slog.String("from", "queued"), slog.String("to", "processing")))

	var record map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &record))

	require.Equal(t, "go-core", record["service"])
	require.Equal(t, "INFO", record["level"])
	require.Equal(t, "thread_status_changed", record["message"])
	require.Contains(t, record, "timestamp")
	require.NotContains(t, record, "time")
	require.NotContains(t, record, "msg")

	ctx, ok := record["context"].(map[string]any)
	require.True(t, ok, "context group must be a nested object")
	require.Equal(t, "queued", ctx["from"])
	require.Equal(t, "processing", ctx["to"])
}

func TestNew_CriticalLevelName(t *testing.T) {
	var buf bytes.Buffer
	l := logger.New(&buf, slog.LevelDebug)

	l.Log(context.Background(), logger.LevelCritical, "database unreachable")

	var record map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &record))
	require.Equal(t, "CRITICAL", record["level"])
}

func TestNew_RespectsMinimumLevel(t *testing.T) {
	var buf bytes.Buffer
	l := logger.New(&buf, slog.LevelWarn)

	l.Debug("should not appear")
	l.Info("should not appear either")

	require.Empty(t, buf.String())
}

func TestParseLevel(t *testing.T) {
	tests := []struct {
		in      string
		want    slog.Level
		wantErr bool
	}{
		{"debug", slog.LevelDebug, false},
		{"info", slog.LevelInfo, false},
		{"warn", slog.LevelWarn, false},
		{"error", slog.LevelError, false},
		{"not-a-level", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := logger.ParseLevel(tt.in)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestFromContext_FallsBackToDefault(t *testing.T) {
	l := logger.FromContext(context.Background())
	require.NotNil(t, l)
}

func TestWithContext_RoundTrip(t *testing.T) {
	var buf bytes.Buffer
	l := logger.New(&buf, slog.LevelDebug)

	ctx := logger.WithContext(context.Background(), l)
	got := logger.FromContext(ctx)

	got.Info("from context")
	require.Contains(t, buf.String(), "from context")
}
