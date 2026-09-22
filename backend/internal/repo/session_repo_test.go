package repo_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"

	"zan-backend/internal/domain"
	"zan-backend/internal/repo"
	"zan-backend/internal/service/session"
)

// setupTestDB поднимает одноразовый Postgres (тот же образ, что и прод —
// docker-compose.yml, pgvector/pgvector:pg16) в testcontainers, накатывает
// реальные миграции backend/migrations и возвращает готовый пул. Реальный
// Postgres, не моки SQL (BACKEND_CODING_STANDARDS.md §10).
func setupTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()

	container, err := tcpostgres.Run(ctx,
		"pgvector/pgvector:pg16",
		tcpostgres.WithDatabase("zan_test"),
		tcpostgres.WithUsername("zan"),
		tcpostgres.WithPassword("zan"),
		tcpostgres.BasicWaitStrategies(),
	)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, testcontainers.TerminateContainer(container))
	})

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	m, err := migrate.New("file://../../migrations", dsn)
	require.NoError(t, err)
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		require.NoError(t, err)
	}

	pool, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	return pool
}

func newTestSession(id string, now time.Time) domain.Session {
	return domain.Session{
		ID:             id,
		Language:       domain.LanguageRu,
		OnboardingSeen: false,
		CreatedAt:      now,
		LastSeenAt:     now,
		ExpiresAt:      now.Add(session.TTL),
	}
}

func TestSessionRepo_CreateAndGetByID(t *testing.T) {
	pool := setupTestDB(t)
	r := repo.NewSessionRepo(pool)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Microsecond)
	sess := newTestSession("3fa85f64-5717-4562-b3fc-2c963f66afa6", now)

	require.NoError(t, r.Create(ctx, sess))

	got, err := r.GetByID(ctx, sess.ID)
	require.NoError(t, err)
	require.Equal(t, sess.ID, got.ID)
	require.Equal(t, sess.Language, got.Language)
	require.Nil(t, got.Theme)
	require.False(t, got.OnboardingSeen)
	require.WithinDuration(t, sess.ExpiresAt, got.ExpiresAt, time.Second)
}

func TestSessionRepo_GetByID_NotFound(t *testing.T) {
	pool := setupTestDB(t)
	r := repo.NewSessionRepo(pool)

	_, err := r.GetByID(context.Background(), "00000000-0000-0000-0000-000000000000")

	require.ErrorIs(t, err, session.ErrNotFound)
}

func TestSessionRepo_Update_PersistsThemeAndOnboarding(t *testing.T) {
	pool := setupTestDB(t)
	r := repo.NewSessionRepo(pool)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Microsecond)
	sess := newTestSession("4fa85f64-5717-4562-b3fc-2c963f66afa7", now)
	require.NoError(t, r.Create(ctx, sess))

	dark := domain.ThemeDark
	sess.Theme = &dark
	sess.OnboardingSeen = true
	sess.Language = domain.LanguageKz
	later := now.Add(time.Hour)
	sess.LastSeenAt = later
	sess.ExpiresAt = later.Add(session.TTL)

	require.NoError(t, r.Update(ctx, sess))

	got, err := r.GetByID(ctx, sess.ID)
	require.NoError(t, err)
	require.Equal(t, domain.LanguageKz, got.Language)
	require.NotNil(t, got.Theme)
	require.Equal(t, domain.ThemeDark, *got.Theme)
	require.True(t, got.OnboardingSeen)
	require.WithinDuration(t, later, got.LastSeenAt, time.Second)
}
