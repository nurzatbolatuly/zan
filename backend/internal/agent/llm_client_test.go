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
		model:      "gpt-4o",
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
		require.Equal(t, "Bearer test-key", r.Header.Get("authorization"))
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(openAIResponse{
			Choices: []openAIChoice{{Message: openAIMessage{Role: "assistant", Content: "hi"}, FinishReason: "stop"}},
		})
	}))
	defer server.Close()

	c := newTestLLMClient(t, server)
	resp, err := c.call(context.Background(), "system", []openAIMessage{{Role: "user", Content: "hi"}})

	require.NoError(t, err)
	require.Equal(t, "hi", resp.Choices[0].Message.Content)
	require.Equal(t, int32(1), calls.Load())
}

func TestLLMClient_Call_RetriesOn500ThenSucceeds(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":{"type":"server_error","message":"boom"}}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(openAIResponse{
			Choices: []openAIChoice{{Message: openAIMessage{Role: "assistant", Content: "ok"}, FinishReason: "stop"}},
		})
	}))
	defer server.Close()

	c := newTestLLMClient(t, server)
	resp, err := c.call(context.Background(), "system", nil)

	require.NoError(t, err)
	require.Equal(t, "ok", resp.Choices[0].Message.Content)
	require.Equal(t, int32(2), calls.Load())
}

func TestLLMClient_Call_DoesNotRetryOn401(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"type":"invalid_request_error","message":"invalid api key"}}`))
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

func TestLLMClient_Call_SendsSystemPromptAsFirstMessage(t *testing.T) {
	var seenReq openAIRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, json.NewDecoder(r.Body).Decode(&seenReq))
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(openAIResponse{
			Choices: []openAIChoice{{Message: openAIMessage{Content: "ok"}, FinishReason: "stop"}},
		})
	}))
	defer server.Close()

	c := newTestLLMClient(t, server)
	_, err := c.call(context.Background(), "be helpful", []openAIMessage{{Role: "user", Content: "hi"}})

	require.NoError(t, err)
	require.Equal(t, "json_object", seenReq.ResponseFormat.Type)
	require.Len(t, seenReq.Messages, 2)
	require.Equal(t, "system", seenReq.Messages[0].Role)
	require.Equal(t, "be helpful", seenReq.Messages[0].Content)
	require.Equal(t, "user", seenReq.Messages[1].Role)
}
