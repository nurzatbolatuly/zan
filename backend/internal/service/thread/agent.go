package thread

import (
	"context"

	"zan-backend/internal/domain"
)

// AgentRequest — вход Agent.Process. ServiceID сейчас всегда "qa" (генерация
// документа — отдельный флоу POST /threads/{id}/generate-document, Stage 6,
// со своим AgentPrompt(document) — zan-backend-tz-v2.md §4.6). History —
// вся история треда, включая только что сохранённое сообщение пользователя,
// от старых к новым, с вложениями (Repository.GetConversation).
type AgentRequest struct {
	ThreadID  string
	ServiceID string
	Language  domain.Language
	History   []domain.Message

	// OnDelta — колбэк на каждый фрагмент текста ответа по мере генерации
	// (streaming-вызов LLM, internal/agent). nil-safe: вызывающий код
	// (runAgentAndFinish) всегда передаёт непустой колбэк.
	OnDelta func(delta string)
}

// AgentResultStatus — исход вызова агента, явный тип вместо нескольких
// независимых bool/nullable-полей (BACKEND_CODING_STANDARDS.md §6.1).
type AgentResultStatus string

const (
	AgentResultDone  AgentResultStatus = "done"
	AgentResultError AgentResultStatus = "error"
)

// AgentResult — ответ агента. AnswerText валиден только при Status
// AgentResultDone; Err — только при AgentResultError и описывает
// бизнес-причину отказа (модерация, отказ модели) для лога, не для показа
// пользователю (BACKEND_CODING_STANDARDS.md §8). Отдельно от Err — сам
// возврат error из Process(...): транспортный/инфраструктурный сбой
// (таймаут, 5xx, пустой ответ), реализация (internal/agent.Client) делает
// до 2 ретраев с backoff перед тем, как его вернуть.
type AgentResult struct {
	Status     AgentResultStatus
	AnswerText string
	Err        error
}

// Agent — порт вызова LLM-агента, объявлен здесь (там, где используется —
// BACKEND_CODING_STANDARDS.md §1.1), а не в internal/agent, где живёт
// реальная реализация (internal/agent.Client).
type Agent interface {
	Process(ctx context.Context, req AgentRequest) (AgentResult, error)
}
