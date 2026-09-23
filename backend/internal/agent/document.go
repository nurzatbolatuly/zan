package agent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"zan-backend/internal/domain"
	"zan-backend/internal/platform/logger"
	"zan-backend/internal/service/document"
)

// ErrDocumentNeedsClarification — LLM запросила уточнение
// (needs_clarification=true) вместо готового документа. У генерации
// документа нет промежуточного состояния: недостаточно контекста в истории
// треда трактуется как неуспех операции (document.Service возвращает
// document.ErrGenerationFailed, кредит возвращается) — тот же принцип, что
// и у ErrModelRefused/невалидного JSON.
var ErrDocumentNeedsClarification = errors.New("agent: document generation needs clarification")

// GenerateDocument — document.Generator: AgentPrompt(document) + история
// треда, один не-streaming вызов LLM в JSON-режиме (документ рендерится
// целиком, стримить его пользователю нечего). Findings из ответа становятся
// секциями рендера (grpcclient.Client.RenderDocument), AnswerText — текстом
// сообщения, сохраняемого в чат (document.Service.Generate).
func (c *Client) GenerateDocument(ctx context.Context, history []domain.Message) (document.GenerateResult, error) {
	parsed, err := c.generateStructured(ctx, domain.AgentTypeDocument, history)
	if err != nil {
		return document.GenerateResult{}, err
	}
	if parsed.NeedsClarification {
		return document.GenerateResult{}, fmt.Errorf("agent: generate document: %w", ErrDocumentNeedsClarification)
	}

	return document.GenerateResult{
		AnswerText: parsed.AnswerText,
		Sources:    parsed.Sources,
		Findings:   parsed.Findings,
	}, nil
}

// generateStructured — промпт(agentType) из БД + технический JSON-контракт
// + вызов LLM + разбор структурированного ответа. Вложения — только
// извлечённым текстом: документ строится по уже состоявшемуся диалогу, в
// котором модель видела файлы целиком в момент вопроса. Логирует
// llm_call_started/llm_call_completed (instructions.md §3.4).
// ErrModelRefused возвращается как обычная ошибка — вызывающий метод
// решает сам, как её показать наружу.
func (c *Client) generateStructured(ctx context.Context, agentType domain.AgentType, history []domain.Message) (llmJSONResponse, error) {
	l := logger.FromContext(ctx)

	basePrompt, err := c.prompts.GetPromptText(ctx, agentType)
	if err != nil {
		return llmJSONResponse{}, fmt.Errorf("agent: load prompt: %w", err)
	}

	systemPrompt := buildStructuredSystemPrompt(basePrompt)
	l.Info("llm_call_started", slog.Group("context",
		slog.String("agent_type", string(agentType)),
		slog.String("prompt_version", promptVersionHash(systemPrompt)),
	))
	started := time.Now()
	resp, err := c.llm.call(ctx, systemPrompt, buildMessages(history, nil))
	latencyMs := time.Since(started).Milliseconds()

	if err != nil {
		l.Error("llm_call_completed", slog.Group("context",
			slog.Int64("latency_ms", latencyMs),
			slog.String("result", "fail"),
			slog.String("error", err.Error()),
		))
		return llmJSONResponse{}, fmt.Errorf("agent: call llm: %w", err)
	}

	if len(resp.Choices) > 0 {
		if cause := finishReasonError(resp.Choices[0].FinishReason); cause != nil {
			l.Error("llm_call_completed", slog.Group("context",
				slog.Int64("latency_ms", latencyMs),
				slog.String("result", "fail"),
				slog.String("error", cause.Error()),
			))
			return llmJSONResponse{}, cause
		}
	}

	parsed, err := parseAgentResponse(resp)
	if err != nil {
		l.Error("llm_call_completed", slog.Group("context",
			slog.Int64("latency_ms", latencyMs),
			slog.String("result", "invalid_json"),
			slog.String("error", err.Error()),
		))
		return llmJSONResponse{}, fmt.Errorf("agent: parse response: %w", err)
	}

	l.Info("llm_call_completed", slog.Group("context",
		slog.Int64("latency_ms", latencyMs),
		slog.String("result", "success"),
		slog.Bool("needs_clarification", parsed.NeedsClarification),
	))
	return parsed, nil
}
