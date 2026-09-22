package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"zan-backend/internal/domain"
)

// ErrEmptyAnswer — LLM вернула валидный JSON, но answer_text пуст —
// технически парсится, но бессмысленно показывать пользователю, тот же
// класс проблемы, что "невалидный JSON" (backend-roadmap.md §5.2).
var ErrEmptyAnswer = errors.New("agent: answer_text is empty")

// llmJSONResponse — структурированный ответ агента (zan-backend-tz-v2.md
// §4.6: "{answer_text, sources, findings, needs_clarification}"). Прямая
// unmarshal в domain.Source/domain.Finding работает благодаря
// регистронезависимому фолбэку encoding/json (поля без тегов "Ref"/"Quote"
// матчатся с JSON-ключами "ref"/"quote") — отдельные wire-DTO под это не
// заводятся (BACKEND_CODING_STANDARDS.md §1.2 не требует их там, где
// стандартной библиотеки достаточно и нет расхождения имён/типов полей).
type llmJSONResponse struct {
	AnswerText         string           `json:"answer_text"`
	Sources            []domain.Source  `json:"sources"`
	Findings           []domain.Finding `json:"findings"`
	NeedsClarification bool             `json:"needs_clarification"`
}

// parseAgentResponse — разбирает сырой ответ Anthropic (объединяет все
// text-блоки content, снимает возможную markdown-обёртку — модель иногда
// оборачивает JSON в ```json несмотря на инструкцию в промпте) в
// llmJSONResponse. Ошибка здесь — тот самый "невалидный JSON" из каталога
// ошибок (backend-roadmap.md §5.2), возвращается как error, не как
// AgentResult{Status: AgentResultError} — решение уже сделано портом
// thread.Agent (internal/service/thread/agent.go).
func parseAgentResponse(resp anthropicResponse) (llmJSONResponse, error) {
	raw := stripMarkdownFence(extractText(resp))

	var parsed llmJSONResponse
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return llmJSONResponse{}, fmt.Errorf("agent: unmarshal agent response: %w", err)
	}
	if strings.TrimSpace(parsed.AnswerText) == "" {
		return llmJSONResponse{}, ErrEmptyAnswer
	}
	return parsed, nil
}

func extractText(resp anthropicResponse) string {
	var b strings.Builder
	for _, block := range resp.Content {
		if block.Type == "text" {
			b.WriteString(block.Text)
		}
	}
	return b.String()
}

// stripMarkdownFence — снимает ```json ... ``` / ``` ... ```, если модель
// обернула JSON в code fence вопреки явной инструкции промпта отвечать
// "строго JSON без markdown-разметки" — защита от типичного отклонения
// LLM от формата, не часть основного контракта.
func stripMarkdownFence(s string) string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "```") {
		return s
	}
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	return strings.TrimSpace(s)
}
