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

// newStreamTestLLMClient — как newTestLLMClient, но со streamHTTPClient/
// streamTimeout — тесты streamCall изолированно, без промпта/thread.Agent
// сверху (это покрыто agent_test.go).
func newStreamTestLLMClient(t *testing.T, server *httptest.Server, streamTimeout time.Duration) *llmClient {
	t.Helper()
	return &llmClient{
		httpClient:       &http.Client{Timeout: 5 * time.Second},
		streamHTTPClient: &http.Client{},
		streamTimeout:    streamTimeout,
		baseURL:          server.URL,
		apiKey:           "test-key",
		model:            "gpt-4o",
		maxTokens:        1024,
		breaker:          resilience.NewBreaker(resilience.BreakerConfig{Name: "test", ConsecutiveFailuresToTrip: 10, OpenTimeout: time.Minute}),
	}
}

func TestLLMClient_StreamCall_InvokesOnDeltaInOrderAndReturnsConcatenatedText(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		streaming, _ := isStreamRequest(t, r)
		require.True(t, streaming)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		writeSSEChunk(w, "Привет", "")
		writeSSEChunk(w, ", ", "")
		writeSSEChunk(w, "мир", "")
		writeSSEDone(w)
	}))
	t.Cleanup(server.Close)
	c := newStreamTestLLMClient(t, server, 5*time.Second)

	var deltas []string
	text, err := c.streamCall(context.Background(), "system", nil, func(d string) { deltas = append(deltas, d) })

	require.NoError(t, err)
	require.Equal(t, "Привет, мир", text)
	require.Equal(t, []string{"Привет", ", ", "мир"}, deltas)
}

// TestLLMClient_StreamCall_RetriesOnFailureBeforeAnyDelta — сбой ДО первой
// дельты (здесь — non-2xx статус целиком, ничего не начало литься) идёт по
// обычному retry-пути (shouldRetryLLM), как и call(); асимметрия касается
// только того, что происходит ПОСЛЕ первой дельты (см. тест ниже).
func TestLLMClient_StreamCall_RetriesOnFailureBeforeAnyDelta(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := attempts.Add(1)
		if n == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":"boom"}`))
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		writeSSEChunk(w, "ok", "")
		writeSSEDone(w)
	}))
	t.Cleanup(server.Close)
	c := newStreamTestLLMClient(t, server, 5*time.Second)

	text, err := c.streamCall(context.Background(), "system", nil, func(string) {})

	require.NoError(t, err)
	require.Equal(t, "ok", text)
	require.Equal(t, int32(2), attempts.Load(), "первая попытка 500, вторая (ретрай) успешна")
}

// TestLLMClient_StreamCall_DoesNotRetryAfterPartialDelta — ключевой инвариант асимметричного ретрая (llm_client.go#shouldRetryStream):
// как только пользователю уже показана хотя бы одна дельта, дальнейший сбой
// того же вызова НЕ ретраится, даже если исходная ошибка сама по себе
// выглядела бы ретраибельной для shouldRetryLLM (здесь — context.DeadlineExceeded
// от истёкшего streamTimeout, shouldRetryLLM.errors.Is(err, context.DeadlineExceeded)
// вернул бы true для НЕ-partial ошибки).
func TestLLMClient_StreamCall_DoesNotRetryAfterPartialDelta(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		writeSSEChunk(w, "начало", "")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		// Зависаем дольше клиентского streamTimeout, не отправляя [DONE] —
		// context.WithTimeout вокруг streamCall (см. doStreamRequest) должен
		// оборвать чтение с context.DeadlineExceeded ПОСЛЕ уже отправленной
		// дельты — ровно сценарий "partial".
		time.Sleep(300 * time.Millisecond)
	}))
	t.Cleanup(server.Close)
	c := newStreamTestLLMClient(t, server, 30*time.Millisecond)

	var deltas []string
	_, err := c.streamCall(context.Background(), "system", nil, func(d string) { deltas = append(deltas, d) })

	require.Error(t, err)
	require.Equal(t, []string{"начало"}, deltas, "дельта до обрыва должна была уйти в onDelta")
	require.Equal(t, int32(1), attempts.Load(), "сбой после частичного вывода не должен ретраиться")
}

func TestLLMClient_StreamCall_SendsStreamRequestWithoutJSONMode(t *testing.T) {
	var seenBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, json.NewDecoder(r.Body).Decode(&seenBody))
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		writeSSEChunk(w, "ok", "")
		writeSSEDone(w)
	}))
	t.Cleanup(server.Close)
	c := newStreamTestLLMClient(t, server, 5*time.Second)

	_, err := c.streamCall(context.Background(), "be helpful", []openAIMessage{{Role: "user", Content: "hi"}}, func(string) {})

	require.NoError(t, err)
	require.Equal(t, true, seenBody["stream"])
	require.NotContains(t, seenBody, "response_format", "стрим читаемого текста несовместим с JSON-режимом")
	messages, ok := seenBody["messages"].([]any)
	require.True(t, ok)
	require.Len(t, messages, 2)
	require.Equal(t, "system", messages[0].(map[string]any)["role"])
	require.Equal(t, "be helpful", messages[0].(map[string]any)["content"])
}

// TestLLMClient_StreamCall_TruncatedBeforeAnyText_ReturnsErrOutputTruncated —
// реальный сценарий gpt-5 с малым max_completion_tokens: весь бюджет ушёл
// на скрытое reasoning, поток закрывается finish_reason="length" без единой
// дельты текста. Это ошибка конфигурации, не транспортный сбой — повтор
// того же запроса дал бы тот же результат, ретрая нет.
func TestLLMClient_StreamCall_TruncatedBeforeAnyText_ReturnsErrOutputTruncated(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		writeSSEChunk(w, "", truncatedFinishReason)
		writeSSEDone(w)
	}))
	t.Cleanup(server.Close)
	c := newStreamTestLLMClient(t, server, 5*time.Second)

	_, err := c.streamCall(context.Background(), "system", nil, func(string) {})

	require.ErrorIs(t, err, ErrOutputTruncated)
	require.Equal(t, int32(1), attempts.Load())
}

// TestLLMClient_StreamCall_TruncatedAfterPartialText_ReturnsErrOutputTruncated —
// обрезанный посередине ответ не засчитывается как успешный, хотя начало
// пользователь уже видел.
func TestLLMClient_StreamCall_TruncatedAfterPartialText_ReturnsErrOutputTruncated(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		writeSSEChunk(w, "начало ответа", "")
		writeSSEChunk(w, "", truncatedFinishReason)
		writeSSEDone(w)
	}))
	t.Cleanup(server.Close)
	c := newStreamTestLLMClient(t, server, 5*time.Second)

	_, err := c.streamCall(context.Background(), "system", nil, func(string) {})

	require.ErrorIs(t, err, ErrOutputTruncated)
}

func TestLLMClient_StreamCall_ReasoningEffort(t *testing.T) {
	for _, tc := range []struct {
		name   string
		effort string
	}{
		{name: "sent when configured", effort: "low"},
		{name: "omitted when empty (non-reasoning models reject it)", effort: ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var seenBody map[string]any
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.NoError(t, json.NewDecoder(r.Body).Decode(&seenBody))
				w.Header().Set("Content-Type", "text/event-stream")
				w.WriteHeader(http.StatusOK)
				writeSSEChunk(w, "ok", "")
				writeSSEDone(w)
			}))
			t.Cleanup(server.Close)
			c := newStreamTestLLMClient(t, server, 5*time.Second)
			c.reasoningEffort = tc.effort

			_, err := c.streamCall(context.Background(), "system", nil, func(string) {})

			require.NoError(t, err)
			if tc.effort == "" {
				require.NotContains(t, seenBody, "reasoning_effort")
			} else {
				require.Equal(t, tc.effort, seenBody["reasoning_effort"])
			}
		})
	}
}
