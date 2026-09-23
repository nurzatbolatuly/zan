package agent

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"zan-backend/internal/platform/resilience"
)

// llmRetryPolicy — до 2 повторов сверх первой попытки, backoff — тот же
// принцип, что grpcclient.retryPolicy (backend-roadmap.md §5.2: "таймаут/
// 5xx — до 2 повторов с backoff").
var llmRetryPolicy = resilience.RetryPolicy{MaxAttempts: 3, BaseDelay: 500 * time.Millisecond}

// openAIMessage — Content: текст сообщения; Files — вложения, переданные
// модели целиком (только в запросе, в ответе их нет). Без Files content
// уходит строкой, с Files — массивом частей [text, file|image_url, ...]:
// Chat Completions принимает обе формы, текстовые запросы не меняются.
type openAIMessage struct {
	Role    string              `json:"role"`
	Content string              `json:"content"`
	Files   []openAIContentPart `json:"-"`
}

func (m openAIMessage) MarshalJSON() ([]byte, error) {
	if len(m.Files) == 0 {
		type plainMessage openAIMessage // без MarshalJSON — иначе рекурсия
		return json.Marshal(plainMessage(m))
	}
	parts := make([]openAIContentPart, 0, len(m.Files)+1)
	if m.Content != "" {
		parts = append(parts, openAIContentPart{Type: "text", Text: m.Content})
	}
	parts = append(parts, m.Files...)
	return json.Marshal(struct {
		Role    string              `json:"role"`
		Content []openAIContentPart `json:"content"`
	}{Role: m.Role, Content: parts})
}

// openAIContentPart — одна часть content: "text", "file" (PDF) или
// "image_url" (изображение); файл — base64 data-URL.
type openAIContentPart struct {
	Type     string          `json:"type"`
	Text     string          `json:"text,omitempty"`
	File     *openAIFile     `json:"file,omitempty"`
	ImageURL *openAIImageURL `json:"image_url,omitempty"`
}

type openAIFile struct {
	Filename string `json:"filename"`
	FileData string `json:"file_data"`
}

type openAIImageURL struct {
	URL string `json:"url"`
}

// openAIResponseFormat — `{"type": "json_object"}` (OpenAI Chat Completions
// API, "Structured Outputs"/JSON mode) — модель гарантированно возвращает
// синтаксически валидный JSON, не полагаемся только на инструкцию в
// промпте. Требует, чтобы слово "JSON" встречалось в messages (system или
// user) — jsonContractInstructions (prompt.go) этому условию удовлетворяет.
type openAIResponseFormat struct {
	Type string `json:"type"`
}

// ReasoningEffort — omitempty: у не-reasoning моделей (gpt-4o и т.п.)
// параметра нет, OpenAI отвечает на него 400 — пустое значение конфига
// означает "не отправлять".
type openAIRequest struct {
	Model               string               `json:"model"`
	MaxCompletionTokens int                  `json:"max_completion_tokens"`
	ReasoningEffort     string               `json:"reasoning_effort,omitempty"`
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
	// streamHTTPClient — отдельный клиент для streamCall БЕЗ
	// http.Client.Timeout (он режет и чтение тела ответа — для
	// длительного SSE-потока это обрезало бы ответ посередине). Дедлайн на
	// весь стрим — streamTimeout, применяется через context.WithTimeout
	// внутри streamCall, не через это поле.
	streamHTTPClient *http.Client
	streamTimeout    time.Duration
	baseURL          string
	apiKey           string
	model            string
	maxTokens        int
	reasoningEffort  string
	breaker          *resilience.Breaker
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
		ReasoningEffort:     c.reasoningEffort,
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

// openAIStreamRequest — тот же запрос, что openAIRequest, но без
// ResponseFormat (Q&A — обычный текст, не JSON-режим, см.
// prompt.go#qaAnswerInstructions) и с Stream:true.
type openAIStreamRequest struct {
	Model               string          `json:"model"`
	MaxCompletionTokens int             `json:"max_completion_tokens"`
	ReasoningEffort     string          `json:"reasoning_effort,omitempty"`
	Messages            []openAIMessage `json:"messages"`
	Stream              bool            `json:"stream"`
}

// openAIStreamChunk — один SSE-чанк Chat Completions API (`data: {...}`) в
// streaming-режиме — только то, что реально читает эта интеграция (тот же
// принцип, что openAIResponse).
type openAIStreamChunk struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
}

// errStreamPartial — сигнальная ошибка "стрим уже начал отдавать
// токены пользователю, дальнейший сбой этого вызова не ретраится". Обёртка,
// не самостоятельная причина сбоя — исходная ошибка остаётся доступна через
// errors.Unwrap/%w.
type errStreamPartial struct {
	cause error
}

func (e *errStreamPartial) Error() string {
	return fmt.Sprintf("agent: stream failed after partial output: %v", e.cause)
}
func (e *errStreamPartial) Unwrap() error { return e.cause }

// shouldRetryStream — как shouldRetryLLM, но никогда не ретраит
// errStreamPartial: частично показанные пользователю токены нельзя
// "отменить" повторным запросом с нуля — повтор молча продублировал бы или
// противоречил уже отрендеренному в UI началу ответа (см. streamCall).
func shouldRetryStream(err error) bool {
	var partial *errStreamPartial
	if errors.As(err, &partial) {
		return false
	}
	return shouldRetryLLM(err)
}

// streamCall — Q&A (agent.go#streamAnswer): один логический streaming-вызов
// LLM, circuit breaker снаружи (как call), ретраи с backoff внутри — но
// асимметрично (shouldRetryStream, не shouldRetryLLM): как только
// отправлена первая непустая дельта, дальнейший сбой того же вызова больше
// не ретраится. Возвращает полный накопленный текст ответа — он же
// финальный текст сообщения ассистента.
func (c *llmClient) streamCall(ctx context.Context, systemPrompt string, messages []openAIMessage, onDelta func(string)) (string, error) {
	var result string
	err := c.breaker.Execute(ctx, func(ctx context.Context) error {
		return llmRetryPolicy.Do(ctx, shouldRetryStream, func(ctx context.Context) error {
			text, err := c.doStreamRequest(ctx, systemPrompt, messages, onDelta)
			if err != nil {
				return err
			}
			result = text
			return nil
		})
	})
	if err != nil {
		return "", err
	}
	return result, nil
}

func (c *llmClient) doStreamRequest(ctx context.Context, systemPrompt string, messages []openAIMessage, onDelta func(string)) (string, error) {
	streamCtx := ctx
	var cancel context.CancelFunc
	if c.streamTimeout > 0 {
		streamCtx, cancel = context.WithTimeout(ctx, c.streamTimeout)
		defer cancel()
	}

	fullMessages := make([]openAIMessage, 0, len(messages)+1)
	fullMessages = append(fullMessages, openAIMessage{Role: "system", Content: systemPrompt})
	fullMessages = append(fullMessages, messages...)

	reqBody, err := json.Marshal(openAIStreamRequest{
		Model:               c.model,
		MaxCompletionTokens: c.maxTokens,
		ReasoningEffort:     c.reasoningEffort,
		Messages:            fullMessages,
		Stream:              true,
	})
	if err != nil {
		return "", fmt.Errorf("agent: marshal openai stream request: %w", err)
	}

	req, err := http.NewRequestWithContext(streamCtx, http.MethodPost, c.baseURL+"/v1/chat/completions", bytes.NewReader(reqBody))
	if err != nil {
		return "", fmt.Errorf("agent: build openai stream request: %w", err)
	}
	req.Header.Set("content-type", "application/json")
	req.Header.Set("accept", "text/event-stream")
	req.Header.Set("authorization", "Bearer "+c.apiKey)

	resp, err := c.streamHTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("agent: call openai stream: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		// Ничего ещё не отдано клиенту (Read тела ниже не начинался) —
		// обычный ретраи-путь, не оборачивается в errStreamPartial.
		body, _ := io.ReadAll(resp.Body)
		return "", &llmStatusError{StatusCode: resp.StatusCode, Body: string(body)}
	}

	var answer strings.Builder
	startedStreaming := false
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "[DONE]" {
			break
		}
		var chunk openAIStreamChunk
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			wrapped := fmt.Errorf("agent: unmarshal openai stream chunk: %w", err)
			if startedStreaming {
				return answer.String(), &errStreamPartial{cause: wrapped}
			}
			return "", wrapped
		}
		if len(chunk.Choices) == 0 {
			continue
		}
		if cause := finishReasonError(chunk.Choices[0].FinishReason); cause != nil {
			if startedStreaming {
				return answer.String(), &errStreamPartial{cause: cause}
			}
			return "", cause
		}
		if content := chunk.Choices[0].Delta.Content; content != "" {
			answer.WriteString(content)
			startedStreaming = true
			onDelta(content)
		}
	}
	if err := scanner.Err(); err != nil {
		wrapped := fmt.Errorf("agent: read openai stream: %w", err)
		if startedStreaming {
			return answer.String(), &errStreamPartial{cause: wrapped}
		}
		return "", wrapped
	}

	return answer.String(), nil
}

// finishReasonError — finish_reason, при котором ответ модели нельзя
// засчитать: отказ модерации или обрезка по max_completion_tokens. nil —
// штатное завершение ("stop") или поток ещё идёт (пустая строка).
func finishReasonError(finishReason string) error {
	switch finishReason {
	case refusalFinishReason:
		return ErrModelRefused
	case truncatedFinishReason:
		return ErrOutputTruncated
	default:
		return nil
	}
}

// isRequestRejected — OpenAI отклонил сам запрос (400/413): для вызова с
// вложениями целиком — сигнал повторить его на извлечённом тексте (PDF
// длиннее лимита страниц, неподдерживаемое изображение и т.п.).
func isRequestRejected(err error) bool {
	var statusErr *llmStatusError
	return errors.As(err, &statusErr) &&
		(statusErr.StatusCode == http.StatusBadRequest || statusErr.StatusCode == http.StatusRequestEntityTooLarge)
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
