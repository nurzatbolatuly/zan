package httpserver_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

// Stage 8 (BACKEND_PLAN.md §6 п.3 "Деплой": "CORS — со стороны бэкенда").

func TestCORS_AllowedOrigin_SetsCredentialedHeaders(t *testing.T) {
	deps := newTestDeps()
	deps.CORSAllowedOrigins = []string{"https://app.example.kz"}
	router := newRouterWithDeps(t, deps)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.Header.Set("Origin", "https://app.example.kz")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, "https://app.example.kz", rec.Header().Get("Access-Control-Allow-Origin"))
	require.Equal(t, "true", rec.Header().Get("Access-Control-Allow-Credentials"))
	require.Equal(t, "Origin", rec.Header().Get("Vary"))
}

func TestCORS_UnlistedOrigin_NoCORSHeaders(t *testing.T) {
	deps := newTestDeps()
	deps.CORSAllowedOrigins = []string{"https://app.example.kz"}
	router := newRouterWithDeps(t, deps)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.Header.Set("Origin", "https://evil.example.com")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Empty(t, rec.Header().Get("Access-Control-Allow-Origin"))
	require.Empty(t, rec.Header().Get("Access-Control-Allow-Credentials"))
}

func TestCORS_EmptyAllowlist_NoCORSHeadersEvenForSameRequest(t *testing.T) {
	deps := newTestDeps()
	deps.CORSAllowedOrigins = nil
	router := newRouterWithDeps(t, deps)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.Header.Set("Origin", "https://app.example.kz")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Empty(t, rec.Header().Get("Access-Control-Allow-Origin"))
}

func TestCORS_Preflight_RespondsNoContentWithMethodsAndHeaders(t *testing.T) {
	deps := newTestDeps()
	deps.CORSAllowedOrigins = []string{"https://app.example.kz"}
	router := newRouterWithDeps(t, deps)

	req := httptest.NewRequest(http.MethodOptions, "/threads", nil)
	req.Header.Set("Origin", "https://app.example.kz")
	req.Header.Set("Access-Control-Request-Method", "POST")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNoContent, rec.Code)
	require.Equal(t, "https://app.example.kz", rec.Header().Get("Access-Control-Allow-Origin"))
	require.Contains(t, rec.Header().Get("Access-Control-Allow-Methods"), "POST")
	require.Contains(t, rec.Header().Get("Access-Control-Allow-Headers"), "Authorization")
}
