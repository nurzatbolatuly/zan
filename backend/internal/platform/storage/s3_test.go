package storage_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcminio "github.com/testcontainers/testcontainers-go/modules/minio"

	"zan-backend/internal/platform/storage"
)

// setupTestClient поднимает одноразовый MinIO в testcontainers (тот же
// образ семейства, что и docker-compose.yml) — реальный S3 API, не мок
// AWS SDK (BACKEND_CODING_STANDARDS.md §10: поведение, не форма вызова).
// PublicEndpoint намеренно оставлен пустым — единственный адрес контейнера
// в тесте одинаково доступен и "изнутри", и "снаружи" (никакой отдельной
// docker-сети между тестом и контейнером, как в docker-compose.yml, тут
// нет), так что оба presign-метода должны отдавать рабочую ссылку.
func setupTestClient(t *testing.T) *storage.Client {
	t.Helper()
	ctx := context.Background()

	container, err := tcminio.Run(ctx, "quay.io/minio/minio:latest")
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, testcontainers.TerminateContainer(container))
	})

	endpoint, err := container.ConnectionString(ctx)
	require.NoError(t, err)

	client, err := storage.New(ctx, storage.Config{
		Endpoint:  "http://" + endpoint,
		Region:    "us-east-1",
		AccessKey: container.Username,
		SecretKey: container.Password,
		Bucket:    "zan-files-test",
	})
	require.NoError(t, err)
	return client
}

func TestClient_PutAndPresignGet_RoundTrip(t *testing.T) {
	client := setupTestClient(t)
	ctx := context.Background()

	content := []byte("hello from stage 4")
	require.NoError(t, client.PutBytes(ctx, "uploads/test.txt", content, "text/plain"))

	url, err := client.PresignGetPublic(ctx, "uploads/test.txt", time.Minute)
	require.NoError(t, err)

	resp, err := http.Get(url) //nolint:gosec // url подписан этим же тестом, не пользовательский ввод
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	got, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, content, got)
}

func TestClient_PresignGetInternal_AlsoWorks(t *testing.T) {
	client := setupTestClient(t)
	ctx := context.Background()

	require.NoError(t, client.PutBytes(ctx, "generated/report.txt", []byte("report"), "text/plain"))

	url, err := client.PresignGetInternal(ctx, "generated/report.txt", time.Minute)
	require.NoError(t, err)

	resp, err := http.Get(url) //nolint:gosec // url подписан этим же тестом
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestClient_Delete_RemovesObject(t *testing.T) {
	client := setupTestClient(t)
	ctx := context.Background()

	require.NoError(t, client.PutBytes(ctx, "uploads/voice.webm", []byte("audio"), "audio/webm"))
	require.NoError(t, client.Delete(ctx, "uploads/voice.webm"))

	url, err := client.PresignGetInternal(ctx, "uploads/voice.webm", time.Minute)
	require.NoError(t, err)

	resp, err := http.Get(url) //nolint:gosec // url подписан этим же тестом
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestClient_New_CreatesBucketWhenMissing(t *testing.T) {
	// Покрывает ensureBucket: New не падает на пустом MinIO без
	// заранее созданного бакета (BACKEND_PLAN.md Stage 0 DoD — "docker
	// compose up" без ручных шагов).
	client := setupTestClient(t)
	ctx := context.Background()

	err := client.PutBytes(ctx, fmt.Sprintf("uploads/%s.txt", strings.Repeat("a", 4)), []byte("x"), "text/plain")
	require.NoError(t, err)
}
