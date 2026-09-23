package agent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"zan-backend/internal/domain"
	"zan-backend/internal/platform/logger"
	"zan-backend/internal/platform/resilience"
	"zan-backend/internal/service/thread"
)

// Config — конфигурация Client (читается один раз в cmd/api из
// internal/config.Config — BACKEND_CODING_STANDARDS.md §2).
type Config struct {
	BaseURL     string
	APIKey      string
	Model       string
	MaxTokens   int
	HTTPTimeout time.Duration
	Breaker     resilience.BreakerConfig

	// ReasoningEffort — reasoning_effort запроса
	// (config.Config.OpenAIReasoningEffort); пусто — параметр не
	// отправляется (не-reasoning модели его не принимают).
	ReasoningEffort string

	// StreamHTTPTimeout — дедлайн на весь streaming-вызов Q&A
	// (config.Config.LLMStreamTimeout) — отдельно от HTTPTimeout, который
	// применяется к не-streaming вызову GenerateDocument. См.
	// llmClient.streamHTTPClient — у него нет собственного
	// http.Client.Timeout (обрезал бы SSE-поток посередине), дедлайн
	// применяется через context.WithTimeout вокруг конкретно streamCall.
	StreamHTTPTimeout time.Duration
}

// Client — реализация thread.Agent (Q&A) и document.Generator (генерация
// документа) поверх OpenAI Chat Completions API.
type Client struct {
	prompts PromptProvider
	files   FileLoader
	llm     *llmClient
}

// NewClient собирает Client с внедрёнными зависимостями.
func NewClient(prompts PromptProvider, files FileLoader, cfg Config) *Client {
	return &Client{
		prompts: prompts,
		files:   files,
		llm: &llmClient{
			httpClient: &http.Client{Timeout: cfg.HTTPTimeout},
			// streamHTTPClient — без Timeout намеренно, см. Config.StreamHTTPTimeout.
			streamHTTPClient: &http.Client{},
			streamTimeout:    cfg.StreamHTTPTimeout,
			baseURL:          cfg.BaseURL,
			apiKey:           cfg.APIKey,
			model:            cfg.Model,
			maxTokens:        cfg.MaxTokens,
			reasoningEffort:  cfg.ReasoningEffort,
			breaker:          resilience.NewBreaker(cfg.Breaker),
		},
	}
}

// Process — thread.Agent.Process (Q&A): промпт AgentPrompt(qa) из БД +
// история треда (последнее сообщение — вопрос пользователя) уходят в
// OpenAI одним streaming-вызовом, текст ответа льётся в req.OnDelta по мере
// генерации (WS, answer_delta). Ни поиска источников, ни второго
// структурирующего вызова — ответ пользователю это ровно сгенерированный
// текст, answer_done уходит сразу после конца потока.
//
// PDF/изображения текущего вопроса уходят модели целиком (loadInlineFiles).
// Если OpenAI отклонил такой запрос (isRequestRejected — например, PDF
// длиннее лимита страниц), вызов один раз повторяется на извлечённом тексте:
// отказ приходит до первой дельты, пользователь ещё ничего не видел.
//
// ErrModelRefused (модерация провайдера) оборачивается в
// thread.AgentResult{Status: Error} — бизнес-исход, не транспортный сбой;
// всё остальное (таймаут, 5xx, пустой ответ) возвращается как error.
func (c *Client) Process(ctx context.Context, req thread.AgentRequest) (thread.AgentResult, error) {
	agentType, err := toAgentType(req.ServiceID)
	if err != nil {
		return thread.AgentResult{}, fmt.Errorf("agent: %w", err)
	}

	l := logger.FromContext(ctx)
	inline := c.loadInlineFiles(ctx, l, req.History)
	answerText, err := c.streamAnswer(ctx, agentType, buildMessages(req.History, inline), len(inline), req.OnDelta)
	if err != nil && len(inline) > 0 && isRequestRejected(err) {
		l.Warn("llm_inline_files_rejected", slog.Group("context",
			slog.Int("inline_files", len(inline)),
			slog.String("error", err.Error()),
		))
		answerText, err = c.streamAnswer(ctx, agentType, buildMessages(req.History, nil), 0, req.OnDelta)
	}
	if err != nil {
		if errors.Is(err, ErrModelRefused) {
			return thread.AgentResult{Status: thread.AgentResultError, Err: ErrModelRefused}, nil
		}
		return thread.AgentResult{}, err
	}
	return thread.AgentResult{Status: thread.AgentResultDone, AnswerText: answerText}, nil
}

// streamAnswer — единственный вызов LLM Q&A-раунда, логирует этапы
// llm_call_started/llm_call_completed (instructions.md §3.4).
func (c *Client) streamAnswer(ctx context.Context, agentType domain.AgentType, messages []openAIMessage, inlineFiles int, onDelta func(string)) (string, error) {
	l := logger.FromContext(ctx)

	basePrompt, err := c.prompts.GetPromptText(ctx, agentType)
	if err != nil {
		return "", fmt.Errorf("agent: load prompt: %w", err)
	}
	if onDelta == nil {
		onDelta = func(string) {}
	}

	systemPrompt := buildQASystemPrompt(basePrompt)
	l.Info("llm_call_started", slog.Group("context",
		slog.String("agent_type", string(agentType)),
		slog.String("prompt_version", promptVersionHash(systemPrompt)),
		slog.Int("inline_files", inlineFiles),
	))
	started := time.Now()
	// first_token_ms — сколько пользователь смотрел на индикатор ожидания до
	// первого текста; у reasoning-моделей это почти вся задержка ответа
	// (скрытые рассуждения до первого токена, instructions.md §8).
	firstTokenMs := int64(-1)
	answerText, err := c.llm.streamCall(ctx, systemPrompt, messages, func(delta string) {
		if firstTokenMs < 0 {
			firstTokenMs = time.Since(started).Milliseconds()
		}
		onDelta(delta)
	})
	latencyMs := time.Since(started).Milliseconds()

	if err != nil {
		l.Error("llm_call_completed", slog.Group("context",
			slog.Int64("latency_ms", latencyMs),
			slog.String("result", "fail"),
			slog.String("error", err.Error()),
		))
		return "", fmt.Errorf("agent: stream call llm: %w", err)
	}
	if strings.TrimSpace(answerText) == "" {
		l.Error("llm_call_completed", slog.Group("context",
			slog.Int64("latency_ms", latencyMs),
			slog.String("result", "empty_answer"),
		))
		return "", ErrEmptyAnswer
	}
	l.Info("llm_call_completed", slog.Group("context",
		slog.Int64("latency_ms", latencyMs),
		slog.Int64("first_token_ms", firstTokenMs),
		slog.String("result", "success"),
	))
	return answerText, nil
}

// toAgentType — AgentRequest.ServiceID (каталог услуг, "qa"/"doc") не
// совпадает буквально с domain.AgentType ("qa"/"document" — таблица
// промптов). Через thread.Agent идёт только Q&A: генерация документа —
// отдельный явный вызов GenerateDocument (document.go), не через
// thread.Service.CreateThread/AddMessage.
func toAgentType(serviceID string) (domain.AgentType, error) {
	if serviceID != string(domain.AgentTypeQA) {
		return "", fmt.Errorf("unexpected service_id %q for qa agent (doc generation is a separate flow)", serviceID)
	}
	return domain.AgentTypeQA, nil
}
