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

// anthropicVersion — фиксированная версия Anthropic Messages API
// (обязательный заголовок anthropic-version). Меняется отдельным PR при
// осознанном апгрейде контракта, не "плавает" неявно.
const anthropicVersion = "2023-06-01"

// llmRetryPolicy — до 2 повторов сверх первой попытки, backoff — тот же
// принцип, что grpcclient.retryPolicy (backend-roadmap.md §5.2: "таймаут/
// 5xx — до 2 повторов с backoff").
var llmRetryPolicy = resilience.RetryPolicy{MaxAttempts: 3, BaseDelay: 500 * time.Millisecond}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type anthropicRequest struct {
	Model     string              `json:"model"`
	MaxTokens int                 `json:"max_tokens"`
	System    string              `json:"system"`
	Messages  []anthropicMessage  `json:"messages"`
}

type anthropicContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// anthropicResponse — только то, что реально читает эта интеграция —
// узкое подмножество полного ответа Anthropic Messages API (usage/id/model
// и т.п. здесь не нужны, BACKEND_CODING_STANDARDS.md §1.2: не тянуть wire-
// контракт шире, чем реально используется).
type anthropicResponse struct {
	Content    []anthropicContentBlock `json:"content"`
	StopReason string                  `json:"stop_reason"`
}

// llmStatusError — non-2xx ответ Anthropic API. StatusCode решает,
// ретраится ли вызов (shouldRetryLLM), Body уходит только в лог
// (BACKEND_CODING_STANDARDS.md §8 — не показывается пользователю).
type llmStatusError struct {
	StatusCode int
	Body       string
}

func (e *llmStatusError) Error() string {
	return fmt.Sprintf("agent: anthropic http %d: %s", e.StatusCode, e.Body)
}

// llmClient — тонкий HTTP-клиент к Anthropic Messages API (BACKEND_PLAN.md
// Stage 5: "HTTP-клиент к LLM-провайдеру"), не официальный SDK — ретраи/
// backoff/circuit breaker идут через internal/platform/resilience, тот же
// состав, что grpcclient.Client, ради одного эндпоинта не оправдан
// отдельный SDK как зависимость.
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
func (c *llmClient) call(ctx context.Context, systemPrompt string, messages []anthropicMessage) (anthropicResponse, error) {
	var result anthropicResponse
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
		return anthropicResponse{}, err
	}
	return result, nil
}

func (c *llmClient) doRequest(ctx context.Context, systemPrompt string, messages []anthropicMessage) (anthropicResponse, error) {
	reqBody, err := json.Marshal(anthropicRequest{
		Model:     c.model,
		MaxTokens: c.maxTokens,
		System:    systemPrompt,
		Messages:  messages,
	})
	if err != nil {
		return anthropicResponse{}, fmt.Errorf("agent: marshal anthropic request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/messages", bytes.NewReader(reqBody))
	if err != nil {
		return anthropicResponse{}, fmt.Errorf("agent: build anthropic request: %w", err)
	}
	req.Header.Set("content-type", "application/json")
	req.Header.Set("x-api-key", c.apiKey) //nolint:gosec // имя заголовка Anthropic API, не хардкод секрета
	req.Header.Set("anthropic-version", anthropicVersion)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return anthropicResponse{}, fmt.Errorf("agent: call anthropic: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return anthropicResponse{}, fmt.Errorf("agent: read anthropic response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return anthropicResponse{}, &llmStatusError{StatusCode: resp.StatusCode, Body: string(body)}
	}

	var parsed anthropicResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return anthropicResponse{}, fmt.Errorf("agent: unmarshal anthropic response: %w", err)
	}
	return parsed, nil
}

// statusOverloaded — Anthropic-специфичный код "модель перегружена"
// (не стандартный HTTP-код из net/http, поэтому не в этом пакете констант).
const statusOverloaded = 529

// shouldRetryLLM — ретраится таймаут/сетевая ошибка и 429/5xx (backend-
// roadmap.md §5.2: "таймаут/5xx — до 2 повторов"); остальные 4xx (401
// неверный ключ, 400 невалидный запрос) — не ретраятся, повтор того же
// запроса даст тот же результат.
func shouldRetryLLM(err error) bool {
	var statusErr *llmStatusError
	if errors.As(err, &statusErr) {
		switch statusErr.StatusCode {
		case http.StatusTooManyRequests, http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, statusOverloaded:
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
