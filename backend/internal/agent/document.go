package agent

import (
	"context"
	"errors"
	"fmt"

	"zan-backend/internal/domain"
	"zan-backend/internal/service/document"
)

// ErrDocumentNeedsClarification — LLM запросила уточнение
// (needs_clarification=true) вместо готового документа. В отличие от Q&A
// (thread.AgentResultClarify — отдельный устойчивый статус треда), у
// генерации документа нет промежуточного состояния: недостаточно контекста
// в истории треда трактуется как неуспех операции (document.Service
// возвращает document.ErrGenerationFailed, кредит возвращается) — тот же
// принцип, что и у ErrModelRefused/невалидного JSON, см. document.go.
var ErrDocumentNeedsClarification = errors.New("agent: document generation needs clarification")

// GenerateDocument — document.Generator (Stage 6): AgentPrompt(document) +
// история треда поверх той же RAG/LLM-инфраструктуры, что Process (Stage 5,
// runLLM) — отдельный явный вызов, не через Process/toAgentType (генерация
// документа не идёт через thread.Agent, см. agent.go#toAgentType и
// zan-backend-tz-v2.md §4.4: "генерация документа — всегда отдельная
// операция"). Findings из ответа становятся секциями рендера
// (grpcclient.Client.RenderDocument), AnswerText — текстом сообщения,
// сохраняемого в чат (document.Service.Generate) — та же форма ответа, что
// уже кладёт в чат Q&A-агент (см. prompt.go: "RenderRequest.sections — та
// же форма, что domain.Finding в Go").
func (c *Client) GenerateDocument(ctx context.Context, history []domain.Message) (document.GenerateResult, error) {
	outcome, err := c.runLLM(ctx, domain.AgentTypeDocument, history)
	if err != nil {
		return document.GenerateResult{}, err
	}
	if outcome.Response.NeedsClarification {
		return document.GenerateResult{}, fmt.Errorf("agent: generate document: %w", ErrDocumentNeedsClarification)
	}

	return document.GenerateResult{
		AnswerText:        outcome.Response.AnswerText,
		Sources:           outcome.Response.Sources,
		Findings:          outcome.Response.Findings,
		UnverifiedSources: outcome.Unverified,
	}, nil
}
