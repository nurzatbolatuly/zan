package domain

import (
	"fmt"
	"time"
)

// AgentType — какой агент использует этот промпт (zan-backend-tz-v2.md
// §2.9: "agent_type enum: qa, document (уникальный)"). qa — Stage 5
// (internal/agent.Client), document — Stage 6 (генерация документа,
// переиспользует тот же LLM-клиент с другим промптом, BACKEND_PLAN.md
// Stage 6).
type AgentType string

const (
	AgentTypeQA       AgentType = "qa"
	AgentTypeDocument AgentType = "document"
)

// ParseAgentType валидирует сырое значение (из запроса/БД) как AgentType.
func ParseAgentType(s string) (AgentType, error) {
	switch v := AgentType(s); v {
	case AgentTypeQA, AgentTypeDocument:
		return v, nil
	default:
		return "", fmt.Errorf("domain: invalid agent type %q", s)
	}
}

// AgentPrompt — промпт агента, редактируемый через /admin/prompts и
// применяемый без долгого кеша (zan-backend-tz-v2.md §4.6: "промпт
// подтягивается из БД на каждый вызов, без долгого кеша, чтобы правки
// применялись сразу"). PromptText — только "человеческая" часть промпта
// (тон, охват, поведение) — технический контракт структурированного JSON-
// ответа ({answer_text, sources, findings, needs_clarification}) в
// PromptText не входит и через админку не редактируется, его добавляет
// internal/agent.buildSystemPrompt поверх этого текста (иначе правка
// промпта могла бы сломать парсинг ответа LLM).
type AgentPrompt struct {
	ID         string
	AgentType  AgentType
	PromptText string
	UpdatedAt  time.Time
}
