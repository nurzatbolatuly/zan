package thread

import (
	"context"

	"zan-backend/internal/domain"
)

// AgentRequest — вход Agent.Process. ServiceID сейчас всегда "qa" (генерация
// документа — отдельный флоу POST /threads/{id}/generate-document, Stage 6,
// со своим AgentPrompt(document) — zan-backend-tz-v2.md §4.6). History —
// вся история треда, включая только что сохранённое сообщение пользователя,
// от старых к новым.
type AgentRequest struct {
	ThreadID  string
	ServiceID string
	Language  domain.Language
	History   []domain.Message
}

// AgentResultStatus — исход вызова агента, явный тип вместо нескольких
// независимых bool/nullable-полей (BACKEND_CODING_STANDARDS.md §6.1 — тот
// же пример, что задаёт форму этого типа буквально).
type AgentResultStatus string

const (
	AgentResultDone    AgentResultStatus = "done"
	AgentResultClarify AgentResultStatus = "clarify"
	AgentResultError   AgentResultStatus = "error"
)

// AgentResult — структурированный ответ агента (zan-backend-tz-v2.md §4.6:
// `{answer_text, sources, findings, needs_clarification}`, здесь уже
// разобранный в AgentResultStatus вместо сырого needs_clarification bool).
// AnswerText/Sources/Findings/UnverifiedSources валидны только при Status
// AgentResultDone/AgentResultClarify; Err — только при AgentResultError и
// описывает бизнес-причину отказа (модерация, отказ модели) для лога, не
// для показа пользователю (BACKEND_CODING_STANDARDS.md §8). Отдельно от
// Err — сам возврат error из Process(...), это транспортный/инфраструктурный
// сбой (таймаут, 5xx, невалидный JSON), реализующий его internal/agent.Client
// (Stage 5) делает до 2 ретраев с backoff перед тем, как его вернуть
// (backend-roadmap.md §5.2), Stage 3 (StubAgent) никогда его не возвращает.
//
// UnverifiedSources — Stage 5: internal/agent.verifySources сверяет Sources
// с тем, что реально вернул RAG (backend-roadmap.md §1.4 — "риск"
// галлюцинации закона); thread.Service только переносит это значение в
// сохраняемый domain.Message, само вычисление ему не подконтрольно.
type AgentResult struct {
	Status            AgentResultStatus
	AnswerText        string
	Sources           []domain.Source
	Findings          []domain.Finding
	UnverifiedSources bool
	Err               error
}

// Agent — порт вызова LLM-агента, объявлен здесь (там, где используется —
// BACKEND_CODING_STANDARDS.md §1.1), а не в internal/agent, где живёт
// реальная реализация (internal/agent.Client, Stage 5).
type Agent interface {
	Process(ctx context.Context, req AgentRequest) (AgentResult, error)
}
