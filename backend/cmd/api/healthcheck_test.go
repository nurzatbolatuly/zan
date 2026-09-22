package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHealthcheckURL(t *testing.T) {
	tests := []struct {
		name string
		addr string
		want string
	}{
		{"wildcard host, only port", ":8080", "http://localhost:8080/healthz"},
		{"explicit host:port", "backend:8080", "http://backend:8080/healthz"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, healthcheckURL(tt.addr))
		})
	}
}

func TestRunHealthcheck_OKOnHTTP200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	require.Equal(t, 0, runHealthcheck(srv.Listener.Addr().String()))
}

func TestRunHealthcheck_FailsOnNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	require.Equal(t, 1, runHealthcheck(srv.Listener.Addr().String()))
}

func TestRunHealthcheck_FailsWhenUnreachable(t *testing.T) {
	require.Equal(t, 1, runHealthcheck("127.0.0.1:1"))
}
