package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"zan-backend/internal/platform/resilience"
)

// llmRetryPolicy — до 2 повторов сверх первой попытки, backoff — тот же
// принцип, что grpcclient.retryPolicy (backend-roadmap.md §5.2: "таймаут/
// 5xx — до 2 повторов с backoff").
var llmRetryPolicy = resilience.RetryPolicy{MaxAttempts: 3, BaseDelay: 500 * time.Millisecond}

type openAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// openAIResponseFormat — `{"type": "json_object"}` (OpenAI Chat Completions
// API, "Structured Outputs"/JSON mode) — модель гарантированно возвращает
// синтаксически валидный JSON, не полагаемся только на инструкцию в
// промпте. Требует, чтобы слово "JSON" встречалось в messages (system или
// user) — jsonContractInstructions (prompt.go) этому условию удовлетворяет.
type openAIResponseFormat struct {
	Type string `json:"type"`
}

type openAIRequest struct {
	Model               string               `json:"model"`
	MaxCompletionTokens int                  `json:"max_completion_tokens"`
	Messages            []openAIMessage      `json:"messages"`
	ResponseFormat      openAIResponseFormat `json:"response_format"`
}

type openAIChoice struct {
	Message      openAIMessage `json:"message"`
	FinishReason string        `json:"finish_reason"`
}

// openAIResponse — только то, что реально читает эта интеграция — узкое
// подмножество полного ответа Chat Completions API (usage/id/model и т.п.
// здесь не нужны, BACKEND_CODING_STANDARDS.md §1.2: не тянуть wire-контракт
// шире, чем реально используется).
type openAIResponse struct {
	Choices []openAIChoice `json:"choices"`
}

// llmStatusError — non-2xx ответ OpenAI API. StatusCode решает, ретраится
// ли вызов (shouldRetryLLM), Body уходит только в лог (BACKEND_CODING_STANDARDS.md
// §8 — не показывается пользователю).
type llmStatusError struct {
	StatusCode int
	Body       string
}

func (e *llmStatusError) Error() string {
	return fmt.Sprintf("agent: openai http %d: %s", e.StatusCode, e.Body)
}

// llmClient — тонкий HTTP-клиент к OpenAI Chat Completions API
// (BACKEND_PLAN.md Stage 5/Stage 8: "HTTP-клиент к LLM-провайдеру"), не
// официальный SDK — ретраи/backoff/circuit breaker идут через
// internal/platform/resilience, тот же состав, что grpcclient.Client, ради
// одного эндпоинта не оправдан отдельный SDK как зависимость.
type llmClient struct {
	httpClient *http.Client
	baseURL    string
	apiKey     string
	model      string
	maxTokens  int
	breaker    *resilience.Breaker
}

// call — один логический вызов LLM: circuit breaker снаружи, ретраи с
// backoff внутри (та же композиция, что grpcclient.Client.call).
// systemPrompt собирается в первое сообщение с role="system" — OpenAI Chat
// Completions API (в отличие от Anthropic Messages API) не выделяет system
// в отдельное поле верхнего уровня запроса.
func (c *llmClient) call(ctx context.Context, systemPrompt string, messages []openAIMessage) (openAIResponse, error) {
	var result openAIResponse
	err := c.breaker.Execute(ctx, func(ctx context.Context) error {
		return llmRetryPolicy.Do(ctx, shouldRetryLLM, func(ctx context.Context) error {
			resp, err := c.doRequest(ctx, systemPrompt, messages)
			if err != nil {
				return err
			}
			result = resp
			return nil
		})
	})
	if err != nil {
		return openAIResponse{}, err
	}
	return result, nil
}

func (c *llmClient) doRequest(ctx context.Context, systemPrompt string, messages []openAIMessage) (openAIResponse, error) {
	fullMessages := make([]openAIMessage, 0, len(messages)+1)
	fullMessages = append(fullMessages, openAIMessage{Role: "system", Content: systemPrompt})
	fullMessages = append(fullMessages, messages...)

	reqBody, err := json.Marshal(openAIRequest{
		Model:               c.model,
		MaxCompletionTokens: c.maxTokens,
		Messages:            fullMessages,
		ResponseFormat:      openAIResponseFormat{Type: "json_object"},
	})
	if err != nil {
		return openAIResponse{}, fmt.Errorf("agent: marshal openai request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/chat/completions", bytes.NewReader(reqBody))
	if err != nil {
		return openAIResponse{}, fmt.Errorf("agent: build openai request: %w", err)
	}
	req.Header.Set("content-type", "application/json")
	req.Header.Set("authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return openAIResponse{}, fmt.Errorf("agent: call openai: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return openAIResponse{}, fmt.Errorf("agent: read openai response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return openAIResponse{}, &llmStatusError{StatusCode: resp.StatusCode, Body: string(body)}
	}

	var parsed openAIResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return openAIResponse{}, fmt.Errorf("agent: unmarshal openai response: %w", err)
	}
	return parsed, nil
}

// shouldRetryLLM — ретраится таймаут/сетевая ошибка и 429/5xx (backend-
// roadmap.md §5.2: "таймаут/5xx — до 2 повторов"); остальные 4xx (401
// неверный ключ, 400 невалидный запрос) — не ретраятся, повтор того же
// запроса даст тот же результат.
func shouldRetryLLM(err error) bool {
	var statusErr *llmStatusError
	if errors.As(err, &statusErr) {
		switch statusErr.StatusCode {
		case http.StatusTooManyRequests, http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
			return true
		default:
			return false
		}
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return netErr.Timeout()
	}
	return false
}
