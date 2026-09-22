package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"zan-backend/internal/platform/resilience"
)

func newTestLLMClient(t *testing.T, server *httptest.Server) *llmClient {
	t.Helper()
	return &llmClient{
		httpClient: server.Client(),
		baseURL:    server.URL,
		apiKey:     "test-key",
		model:      "claude-sonnet-5",
		maxTokens:  1024,
		breaker: resilience.NewBreaker(resilience.BreakerConfig{
			Name:                      "test",
			ConsecutiveFailuresToTrip: 3,
			OpenTimeout:               time.Minute,
		}),
	}
}

func TestLLMClient_Call_SuccessOnFirstTry(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		require.Equal(t, "test-key", r.Header.Get("x-api-key"))
		require.Equal(t, anthropicVersion, r.Header.Get("anthropic-version"))
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(anthropicResponse{
			Content: []anthropicContentBlock{{Type: "text", Text: "hi"}},
		})
	}))
	defer server.Close()

	c := newTestLLMClient(t, server)
	resp, err := c.call(context.Background(), "system", []anthropicMessage{{Role: "user", Content: "hi"}})

	require.NoError(t, err)
	require.Equal(t, "hi", resp.Content[0].Text)
	require.Equal(t, int32(1), calls.Load())
}

func TestLLMClient_Call_RetriesOn500ThenSucceeds(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":{"type":"api_error","message":"boom"}}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(anthropicResponse{Content: []anthropicContentBlock{{Type: "text", Text: "ok"}}})
	}))
	defer server.Close()

	c := newTestLLMClient(t, server)
	resp, err := c.call(context.Background(), "system", nil)

	require.NoError(t, err)
	require.Equal(t, "ok", resp.Content[0].Text)
	require.Equal(t, int32(2), calls.Load())
}

func TestLLMClient_Call_DoesNotRetryOn401(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"type":"authentication_error","message":"invalid x-api-key"}}`))
	}))
	defer server.Close()

	c := newTestLLMClient(t, server)
	_, err := c.call(context.Background(), "system", nil)

	require.Error(t, err)
	require.Equal(t, int32(1), calls.Load())
}

func TestLLMClient_Call_ExhaustsRetriesOnPersistent5xx(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	c := newTestLLMClient(t, server)
	c.breaker = resilience.NewBreaker(resilience.BreakerConfig{Name: "test", ConsecutiveFailuresToTrip: 100, OpenTimeout: time.Minute})
	llmRetryPolicyOriginal := llmRetryPolicy
	llmRetryPolicy = resilience.RetryPolicy{MaxAttempts: 3, BaseDelay: time.Millisecond}
	defer func() { llmRetryPolicy = llmRetryPolicyOriginal }()

	_, err := c.call(context.Background(), "system", nil)

	require.Error(t, err)
	require.Equal(t, int32(3), calls.Load())
}

func TestLLMClient_Call_BreakerOpensAfterConsecutiveFailures(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	c := newTestLLMClient(t, server)
	c.breaker = resilience.NewBreaker(resilience.BreakerConfig{Name: "test", ConsecutiveFailuresToTrip: 2, OpenTimeout: time.Minute})

	_, err1 := c.call(context.Background(), "system", nil)
	_, err2 := c.call(context.Background(), "system", nil)
	_, err3 := c.call(context.Background(), "system", nil)

	require.Error(t, err1)
	require.Error(t, err2)
	require.Error(t, err3)
	require.ErrorIs(t, err3, resilience.ErrBreakerOpen)
	// Третий вызов не должен был дойти до сервера — breaker уже открыт.
	require.Equal(t, int32(2), calls.Load())
}
