package httpserver_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func createTestSession(t *testing.T, router http.Handler) (sessionID, token string, cookie *http.Cookie) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/sessions", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code)

	var body struct {
		SessionToken string `json:"session_token"`
		SessionID    string `json:"session_id"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.NotEmpty(t, body.SessionToken)
	require.NotEmpty(t, body.SessionID)

	res := http.Response{Header: rec.Header()}
	for _, c := range res.Cookies() {
		if c.Name == "session_token" {
			cookie = c
		}
	}
	require.NotNil(t, cookie, "POST /sessions must set session_token cookie")

	return body.SessionID, body.SessionToken, cookie
}

func TestSessions_CreateThenGetMe_WithBearerToken(t *testing.T) {
	router, _ := newTestRouter(t)

	sessionID, token, _ := createTestSession(t, router)

	req := httptest.NewRequest(http.MethodGet, "/sessions/me", nil)
	req.Header.Set("Authorization", "Session "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var body struct {
		SessionID      string `json:"session_id"`
		Language       string `json:"language"`
		OnboardingSeen bool   `json:"onboarding_seen"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, sessionID, body.SessionID)
	require.Equal(t, "ru", body.Language)
	require.False(t, body.OnboardingSeen)
}

func TestSessions_GetMe_WithCookie(t *testing.T) {
	router, _ := newTestRouter(t)

	sessionID, _, cookie := createTestSession(t, router)

	req := httptest.NewRequest(http.MethodGet, "/sessions/me", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), sessionID)
}

func TestSessions_GetMe_WithoutToken_Returns401(t *testing.T) {
	router, _ := newTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/sessions/me", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusUnauthorized, rec.Code)
	require.Contains(t, rec.Body.String(), "session_invalid")
}

func TestSessions_GetMe_WithForgedToken_Returns401WithoutAutoCreate(t *testing.T) {
	router, _ := newTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/sessions/me", nil)
	req.Header.Set("Authorization", "Session 00000000-0000-0000-0000-000000000000.deadbeef")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestSessions_PatchMe_UpdatesLanguageAndTheme(t *testing.T) {
	router, _ := newTestRouter(t)
	_, token, _ := createTestSession(t, router)

	body, err := json.Marshal(map[string]string{"language": "kz", "theme": "dark"})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPatch, "/sessions/me", bytes.NewReader(body))
	req.Header.Set("Authorization", "Session "+token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"language":"kz"`)
	require.Contains(t, rec.Body.String(), `"theme":"dark"`)
}

func TestSessions_PatchMe_RejectsInvalidLanguage(t *testing.T) {
	router, _ := newTestRouter(t)
	_, token, _ := createTestSession(t, router)

	body, err := json.Marshal(map[string]string{"language": "en"})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPatch, "/sessions/me", bytes.NewReader(body))
	req.Header.Set("Authorization", "Session "+token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "invalid_request")
}

func TestSessions_OnboardingSeen_MarksTrue(t *testing.T) {
	router, _ := newTestRouter(t)
	_, token, _ := createTestSession(t, router)

	req := httptest.NewRequest(http.MethodPost, "/sessions/onboarding-seen", nil)
	req.Header.Set("Authorization", "Session "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"onboarding_seen":true`)
}

func TestAdmin_Ping_WithoutToken_Returns401(t *testing.T) {
	router, _ := newTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/admin/ping", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestAdmin_Ping_WithValidToken_Returns200(t *testing.T) {
	router, _ := newTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/admin/ping", nil)
	req.Header.Set("X-Admin-Token", testAdminToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
}

func TestAdmin_Ping_WithWrongToken_Returns401(t *testing.T) {
	router, _ := newTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/admin/ping", nil)
	req.Header.Set("X-Admin-Token", "wrong-token")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusUnauthorized, rec.Code)
}
