package agent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"zan-backend/internal/domain"
	"zan-backend/internal/grpcclient"
	"zan-backend/internal/platform/logger"
	"zan-backend/internal/platform/resilience"
	"zan-backend/internal/service/thread"
)

// Config — конфигурация Client (BACKEND_PLAN.md Stage 5, читается один раз
// в cmd/api из internal/config.Config — BACKEND_CODING_STANDARDS.md §2).
type Config struct {
	BaseURL     string
	APIKey      string
	Model       string
	MaxTokens   int
	HTTPTimeout time.Duration
	RagTopK     int
	Breaker     resilience.BreakerConfig
}

// Client — реализация thread.Agent (Stage 5), заменяет thread.StubAgent
// (Stage 3) за тем же портом, без изменений в internal/service/thread или
// internal/httpserver (BACKEND_PLAN.md — "Strategy, тот же приём, что
// SttProvider").
type Client struct {
	prompts PromptProvider
	rag     RagSearcher
	llm     *llmClient
	ragTopK int
}

// NewClient собирает Client с внедрёнными зависимостями.
func NewClient(prompts PromptProvider, rag RagSearcher, cfg Config) *Client {
	return &Client{
		prompts: prompts,
		rag:     rag,
		llm: &llmClient{
			httpClient: &http.Client{Timeout: cfg.HTTPTimeout},
			baseURL:    cfg.BaseURL,
			apiKey:     cfg.APIKey,
			model:      cfg.Model,
			maxTokens:  cfg.MaxTokens,
			breaker:    resilience.NewBreaker(cfg.Breaker),
		},
		ragTopK: cfg.RagTopK,
	}
}

// Process — thread.Agent.Process (backend-roadmap.md §1.4, пошаговый флоу):
// 1) промпт из БД, 2) RAG-источники (деградация без блокировки при
// недоступности), 3) вызов LLM, 4) верификация источников, 5) структурный
// разбор ответа. Общая часть (промпт+RAG+вызов LLM+разбор ответа) — runLLM,
// переиспользуется GenerateDocument (Stage 6, document.go) — разница только
// в том, что Process делает со `parsed.NeedsClarification` (статус треда
// clarify) и как оборачивает ErrModelRefused (thread.AgentResult{Status:Error},
// без транспортной ошибки — GenerateDocument пробрасывает его как есть, у
// генерации документа нет отдельного "статуса", см. document.go).
func (c *Client) Process(ctx context.Context, req thread.AgentRequest) (thread.AgentResult, error) {
	agentType, err := toAgentType(req.ServiceID)
	if err != nil {
		return thread.AgentResult{}, fmt.Errorf("agent: %w", err)
	}

	outcome, err := c.runLLM(ctx, agentType, req.History)
	if err != nil {
		if errors.Is(err, ErrModelRefused) {
			return thread.AgentResult{Status: thread.AgentResultError, Err: ErrModelRefused}, nil
		}
		return thread.AgentResult{}, err
	}

	status := thread.AgentResultDone
	if outcome.Response.NeedsClarification {
		status = thread.AgentResultClarify
	}
	return thread.AgentResult{
		Status:            status,
		AnswerText:        outcome.Response.AnswerText,
		Sources:           outcome.Response.Sources,
		Findings:          outcome.Response.Findings,
		UnverifiedSources: outcome.Unverified,
	}, nil
}

// llmOutcome — результат runLLM: структурированный ответ LLM + вычисленный
// unverified (verifySources нужен и Process, и GenerateDocument, оба реально
// используют его только вместе с логом llm_call_completed — см. runLLM).
type llmOutcome struct {
	Response   llmJSONResponse
	Unverified bool
}

// runLLM — общая часть Process/GenerateDocument (Stage 6): промпт(agentType)
// из БД + RAG-источники (деградация без блокировки) + вызов LLM +
// верификация источников + разбор структурированного ответа. Логирует этапы
// 7/8/9 backend-roadmap.md §6.2 (rag_search_*/llm_call_*) — единственное
// место, где это делается для ОБОИХ агентов (qa/document), не задублировано.
// ErrModelRefused возвращается как обычная ошибка (не завёрнута в результат)
// — оба вызывающих метода решают сами, как её показать наружу.
func (c *Client) runLLM(ctx context.Context, agentType domain.AgentType, history []domain.Message) (llmOutcome, error) {
	l := logger.FromContext(ctx)

	basePrompt, err := c.prompts.GetPromptText(ctx, agentType)
	if err != nil {
		return llmOutcome{}, fmt.Errorf("agent: load prompt: %w", err)
	}

	matches := c.searchSources(ctx, l, history)

	systemPrompt := buildSystemPrompt(basePrompt, matches)
	messages := buildMessages(history)

	l.Info("llm_call_started", slog.Group("context",
		slog.String("agent_type", string(agentType)),
		slog.String("prompt_version", promptVersionHash(systemPrompt)),
		slog.Int("sources_count_in_context", len(matches)),
	))
	started := time.Now()
	resp, err := c.llm.call(ctx, systemPrompt, messages)
	latencyMs := time.Since(started).Milliseconds()

	if err != nil {
		l.Error("llm_call_completed", slog.Group("context",
			slog.Int64("latency_ms", latencyMs),
			slog.String("result", "fail"),
			slog.String("error", err.Error()),
		))
		return llmOutcome{}, fmt.Errorf("agent: call llm: %w", err)
	}

	if len(resp.Choices) > 0 && resp.Choices[0].FinishReason == refusalFinishReason {
		l.Error("llm_call_completed", slog.Group("context",
			slog.Int64("latency_ms", latencyMs),
			slog.String("result", "refused"),
		))
		return llmOutcome{}, ErrModelRefused
	}

	parsed, err := parseAgentResponse(resp)
	if err != nil {
		l.Error("llm_call_completed", slog.Group("context",
			slog.Int64("latency_ms", latencyMs),
			slog.String("result", "invalid_json"),
			slog.String("error", err.Error()),
		))
		return llmOutcome{}, fmt.Errorf("agent: parse response: %w", err)
	}

	unverified := verifySources(parsed.Sources, matches)
	l.Info("llm_call_completed", slog.Group("context",
		slog.Int64("latency_ms", latencyMs),
		slog.String("result", "success"),
		slog.Bool("needs_clarification", parsed.NeedsClarification),
		slog.Bool("unverified_sources", unverified),
	))

	return llmOutcome{Response: parsed, Unverified: unverified}, nil
}

// searchSources — RagService.Search (backend-roadmap.md §1.4 шаг 2) перед
// вызовом LLM. Недоступность RAG — деградация, не блокировка (backend-
// roadmap.md §5.2: "Go продолжает без источников, помечает
// sources_degraded=true в логе") — ЛЮБАЯ ошибка (не только
// grpcclient.IsUnavailable) трактуется так же: RAG никогда не должен быть
// единственной причиной, по которой пользователь не получил ответ.
func (c *Client) searchSources(ctx context.Context, l *slog.Logger, history []domain.Message) []grpcclient.RagMatch {
	query := lastUserMessageText(history)
	if query == "" {
		return nil
	}

	l.Info("rag_search_started", slog.Group("context", slog.String("query_text_hash", queryTextHash(query))))
	started := time.Now()
	matches, err := c.rag.SearchSources(ctx, query, c.ragTopK)
	latencyMs := time.Since(started).Milliseconds()

	if err != nil {
		l.Warn("rag_search_completed", slog.Group("context",
			slog.Int64("latency_ms", latencyMs),
			slog.Int("matches_count", 0),
			slog.Bool("degraded", true),
			slog.String("error", err.Error()),
		))
		return nil
	}
	l.Info("rag_search_completed", slog.Group("context",
		slog.Int64("latency_ms", latencyMs),
		slog.Int("matches_count", len(matches)),
		slog.Bool("degraded", false),
	))
	return matches
}

// toAgentType — AgentRequest.ServiceID (thread.Service, домен "qa"/"doc" —
// каталог услуг) не совпадает буквально с domain.AgentType ("qa"/"document"
// — таблица промптов, zan-backend-tz-v2.md §2.9). Stage 5 подключает только
// Q&A-флоу (thread.AgentRequest.ServiceID всегда "qa" — см. doc-комментарий
// AgentRequest, internal/service/thread/agent.go); "document" получит
// собственный явный вызов (не через этот маппинг) в Stage 6 — генерация
// документа не идёт через thread.Service.CreateThread/AddMessage.
func toAgentType(serviceID string) (domain.AgentType, error) {
	if serviceID != string(domain.AgentTypeQA) {
		return "", fmt.Errorf("unexpected service_id %q for qa agent (doc generation is a separate Stage 6 flow)", serviceID)
	}
	return domain.AgentTypeQA, nil
}
