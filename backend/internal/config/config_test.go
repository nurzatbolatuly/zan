package config_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"zan-backend/internal/config"
)

// Секреты ниже — 33+ символов (Stage 8, minSecretLength в config.go требует
// не меньше 32) и не совпадают ни с одним значением weakDefaultSecrets.
const (
	testAdminToken        = "test-admin-token-0123456789abcdef"
	testSessionHMACSecret = "test-session-hmac-secret-0123456789"
	testInternalSecret    = "test-internal-secret-0123456789abc"
)

// setRequiredSecrets — ADMIN_TOKEN/SESSION_HMAC_SECRET обязательны (Stage 1,
// без envDefault намеренно, см. config.go) — тесты, не проверяющие сами эти
// поля, всё равно должны их задать, иначе Load() падает с ошибкой конфига.
func setRequiredSecrets(t *testing.T) {
	t.Helper()
	t.Setenv("ADMIN_TOKEN", testAdminToken)
	t.Setenv("SESSION_HMAC_SECRET", testSessionHMACSecret)
	t.Setenv("INTERNAL_SECRET", testInternalSecret)
	t.Setenv("S3_ACCESS_KEY", "test-s3-access-key")
	t.Setenv("S3_SECRET_KEY", "test-s3-secret-key")
	t.Setenv("OPENAI_API_KEY", "test-openai-api-key")
}

func TestLoad_Defaults(t *testing.T) {
	setRequiredSecrets(t)

	cfg, err := config.Load()
	require.NoError(t, err)

	require.Equal(t, "dev", cfg.Env)
	require.Equal(t, ":8080", cfg.HTTPAddr)
	require.Equal(t, "info", cfg.LogLevel)
	require.Equal(t, "localhost:9090", cfg.GRPCHelperTarget)
	require.Equal(t, "postgres://zan:zan@localhost:5432/zan?sslmode=disable", cfg.DatabaseURL)
	require.Equal(t, 72*time.Hour, cfg.ThreadFreeUntil)
	require.Equal(t, "http://localhost:9000", cfg.S3Endpoint)
	require.Empty(t, cfg.S3PublicEndpoint)
	require.Equal(t, "us-east-1", cfg.S3Region)
	require.Equal(t, "zan-files", cfg.S3Bucket)
	require.Equal(t, int64(20*1024*1024), cfg.FileMaxSizeBytes)
	require.Equal(t, int64(15*1024*1024), cfg.VoiceMaxSizeBytes)
	require.Equal(t, "gpt-4o", cfg.OpenAIModel)
	require.Equal(t, "https://api.openai.com", cfg.OpenAIBaseURL)
	require.Equal(t, 2048, cfg.OpenAIMaxTokens)
	require.Equal(t, 30*time.Second, cfg.LLMTimeout)
	require.Equal(t, 5, cfg.RagTopK)
}

func TestLoad_OverridesFromEnv(t *testing.T) {
	setRequiredSecrets(t)
	t.Setenv("ENV", "prod")
	t.Setenv("HTTP_ADDR", ":9000")
	t.Setenv("LOG_LEVEL", "warn")
	t.Setenv("GRPC_HELPER_TARGET", "helper:9090")
	t.Setenv("DATABASE_URL", "postgres://x:x@db:5432/x")
	t.Setenv("THREAD_FREE_UNTIL", "24h")
	t.Setenv("OPENAI_MODEL", "gpt-4o-mini")
	t.Setenv("OPENAI_BASE_URL", "http://localhost:9999")
	t.Setenv("OPENAI_MAX_TOKENS", "4096")
	t.Setenv("LLM_TIMEOUT", "10s")
	t.Setenv("RAG_TOP_K", "8")

	cfg, err := config.Load()
	require.NoError(t, err)

	require.Equal(t, "prod", cfg.Env)
	require.Equal(t, ":9000", cfg.HTTPAddr)
	require.Equal(t, "warn", cfg.LogLevel)
	require.Equal(t, "helper:9090", cfg.GRPCHelperTarget)
	require.Equal(t, "postgres://x:x@db:5432/x", cfg.DatabaseURL)
	require.Equal(t, 24*time.Hour, cfg.ThreadFreeUntil)
	require.Equal(t, "gpt-4o-mini", cfg.OpenAIModel)
	require.Equal(t, "http://localhost:9999", cfg.OpenAIBaseURL)
	require.Equal(t, 4096, cfg.OpenAIMaxTokens)
	require.Equal(t, 10*time.Second, cfg.LLMTimeout)
	require.Equal(t, 8, cfg.RagTopK)
}

func TestLoad_MissingRequiredSecret(t *testing.T) {
	t.Setenv("SESSION_HMAC_SECRET", testSessionHMACSecret)
	// ADMIN_TOKEN намеренно не задан.

	_, err := config.Load()
	require.Error(t, err)
}

func TestLoad_MissingS3Credentials(t *testing.T) {
	t.Setenv("ADMIN_TOKEN", testAdminToken)
	t.Setenv("SESSION_HMAC_SECRET", testSessionHMACSecret)
	t.Setenv("INTERNAL_SECRET", testInternalSecret)
	t.Setenv("OPENAI_API_KEY", "test-openai-api-key")
	// S3_ACCESS_KEY/S3_SECRET_KEY намеренно не заданы.

	_, err := config.Load()
	require.Error(t, err)
}

func TestLoad_MissingOpenAIAPIKey(t *testing.T) {
	t.Setenv("ADMIN_TOKEN", testAdminToken)
	t.Setenv("SESSION_HMAC_SECRET", testSessionHMACSecret)
	t.Setenv("INTERNAL_SECRET", testInternalSecret)
	t.Setenv("S3_ACCESS_KEY", "test-s3-access-key")
	t.Setenv("S3_SECRET_KEY", "test-s3-secret-key")
	// OPENAI_API_KEY намеренно не задан.

	_, err := config.Load()
	require.Error(t, err)
}

// Stage 8 (BACKEND_PLAN.md §6 "сила admin-secret и x-internal-secret") —
// validateSecrets в config.go.

func TestLoad_RejectsShortSecret(t *testing.T) {
	setRequiredSecrets(t)
	t.Setenv("ADMIN_TOKEN", "too-short")

	_, err := config.Load()
	require.ErrorContains(t, err, "ADMIN_TOKEN")
}

func TestLoad_RejectsKnownDevPlaceholder(t *testing.T) {
	setRequiredSecrets(t)
	t.Setenv("INTERNAL_SECRET", "devinternalsecretchangemedevinternalsecretchangeme1")

	_, err := config.Load()
	require.ErrorContains(t, err, "INTERNAL_SECRET")
}
