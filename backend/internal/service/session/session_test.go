package session_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"zan-backend/internal/domain"
	"zan-backend/internal/platform/clock"
	"zan-backend/internal/platform/idgen"
	"zan-backend/internal/service/session"
)

type fakeRepo struct {
	sessions map[string]domain.Session
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{sessions: make(map[string]domain.Session)}
}

func (r *fakeRepo) Create(_ context.Context, s domain.Session) error {
	r.sessions[s.ID] = s
	return nil
}

func (r *fakeRepo) GetByID(_ context.Context, id string) (domain.Session, error) {
	s, ok := r.sessions[id]
	if !ok {
		return domain.Session{}, session.ErrNotFound
	}
	return s, nil
}

func (r *fakeRepo) Update(_ context.Context, s domain.Session) error {
	if _, ok := r.sessions[s.ID]; !ok {
		return session.ErrNotFound
	}
	r.sessions[s.ID] = s
	return nil
}

const testSessionID = "3fa85f64-5717-4562-b3fc-2c963f66afa6"

func newTestService(repo *fakeRepo, now time.Time) *session.Service {
	return session.New(
		repo,
		clock.Fake{T: now},
		&idgen.Fake{IDs: []string{testSessionID}},
		session.NewTokenSigner("test-secret"),
	)
}

func TestService_Create_ReturnsSessionAndValidToken(t *testing.T) {
	repo := newFakeRepo()
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	svc := newTestService(repo, now)

	sess, token, err := svc.Create(context.Background())

	require.NoError(t, err)
	require.Equal(t, testSessionID, sess.ID)
	require.Equal(t, domain.LanguageRu, sess.Language)
	require.False(t, sess.OnboardingSeen)
	require.Equal(t, now.Add(session.TTL), sess.ExpiresAt)

	authenticated, err := svc.Authenticate(context.Background(), token)
	require.NoError(t, err)
	require.Equal(t, sess.ID, authenticated.ID)
}

func TestService_Authenticate_ExtendsSlidingWindow(t *testing.T) {
	repo := newFakeRepo()
	createdAt := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	svc := newTestService(repo, createdAt)

	_, token, err := svc.Create(context.Background())
	require.NoError(t, err)

	later := createdAt.Add(30 * 24 * time.Hour)
	svcLater := session.New(repo, clock.Fake{T: later}, &idgen.Fake{}, session.NewTokenSigner("test-secret"))

	sess, err := svcLater.Authenticate(context.Background(), token)
	require.NoError(t, err)
	require.Equal(t, later, sess.LastSeenAt)
	require.Equal(t, later.Add(session.TTL), sess.ExpiresAt)
}

func TestService_Authenticate_RejectsExpiredSession(t *testing.T) {
	repo := newFakeRepo()
	createdAt := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	svc := newTestService(repo, createdAt)

	_, token, err := svc.Create(context.Background())
	require.NoError(t, err)

	afterExpiry := createdAt.Add(session.TTL + time.Second)
	svcLater := session.New(repo, clock.Fake{T: afterExpiry}, &idgen.Fake{}, session.NewTokenSigner("test-secret"))

	_, err = svcLater.Authenticate(context.Background(), token)
	require.ErrorIs(t, err, session.ErrExpired)
}

func TestService_Authenticate_RejectsForgedToken(t *testing.T) {
	repo := newFakeRepo()
	svc := newTestService(repo, time.Now())

	_, err := svc.Authenticate(context.Background(), "3fa85f64-5717-4562-b3fc-2c963f66afa6.deadbeef")

	require.ErrorIs(t, err, session.ErrInvalidToken)
}

func TestService_Authenticate_RejectsUnknownSessionID(t *testing.T) {
	repo := newFakeRepo()
	svc := newTestService(repo, time.Now())
	signer := session.NewTokenSigner("test-secret")

	unknownToken := signer.Sign("00000000-0000-0000-0000-000000000000")

	_, err := svc.Authenticate(context.Background(), unknownToken)

	require.ErrorIs(t, err, session.ErrInvalidToken)
}

func TestService_UpdatePreferences_OnlyChangesProvidedFields(t *testing.T) {
	repo := newFakeRepo()
	svc := newTestService(repo, time.Now())
	sess, _, err := svc.Create(context.Background())
	require.NoError(t, err)

	dark := domain.ThemeDark
	updated, err := svc.UpdatePreferences(context.Background(), sess.ID, nil, &dark)
	require.NoError(t, err)
	require.Equal(t, domain.LanguageRu, updated.Language) // не тронуто
	require.Equal(t, &dark, updated.Theme)

	kz := domain.LanguageKz
	updated, err = svc.UpdatePreferences(context.Background(), sess.ID, &kz, nil)
	require.NoError(t, err)
	require.Equal(t, domain.LanguageKz, updated.Language)
	require.Equal(t, &dark, updated.Theme) // не тронуто предыдущим вызовом
}

func TestService_MarkOnboardingSeen(t *testing.T) {
	repo := newFakeRepo()
	svc := newTestService(repo, time.Now())
	sess, _, err := svc.Create(context.Background())
	require.NoError(t, err)
	require.False(t, sess.OnboardingSeen)

	updated, err := svc.MarkOnboardingSeen(context.Background(), sess.ID)

	require.NoError(t, err)
	require.True(t, updated.OnboardingSeen)
}
