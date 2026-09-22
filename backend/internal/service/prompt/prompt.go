// Package prompt — юзкейсы редактирования и выдачи промптов агента
// (zan-backend-tz-v2.md §3.7 "/admin/prompts", §4.6: "применяются сразу,
// без кеша"). GetPromptText — то немногое, что internal/agent реально
// нужно от этого пакета (порт agent.PromptProvider,
// BACKEND_CODING_STANDARDS.md §1.1, тот же приём, что thread.BalanceService
// удовлетворяется *billing.Service структурно).
package prompt

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"zan-backend/internal/domain"
	"zan-backend/internal/platform/clock"
)

// ErrNotFound — записи с таким agent_type нет (в норме недостижимо после
// сид-данных миграции 000005 — обе строки, qa/document, заведены там же,
// где таблица наполняется в первый раз, см. её комментарий).
var ErrNotFound = errors.New("prompt: not found")

// ErrEmptyText — PUT /admin/prompts/{agent_type} с пустым prompt_text.
var ErrEmptyText = errors.New("prompt: text must not be empty")

// Repository — порт доступа к core.agent_prompts (ровно 2 строки,
// zan-backend-tz-v2.md §2.9).
type Repository interface {
	GetByAgentType(ctx context.Context, agentType domain.AgentType) (domain.AgentPrompt, error)
	List(ctx context.Context) ([]domain.AgentPrompt, error)
	Update(ctx context.Context, agentType domain.AgentType, promptText string, updatedAt time.Time) (domain.AgentPrompt, error)
}

// Service — бизнес-логика промптов.
type Service struct {
	repo  Repository
	clock clock.Clock
}

// New собирает Service с внедрённой зависимостью.
func New(repo Repository, clk clock.Clock) *Service {
	return &Service{repo: repo, clock: clk}
}

// GetPromptText — internal/agent.Client дёргает это на каждый вызов LLM
// (zan-backend-tz-v2.md §4.6: "без долгого кеша"), не сам Repository —
// единая точка, где формируется ErrNotFound из сентинела репозитория.
func (s *Service) GetPromptText(ctx context.Context, agentType domain.AgentType) (string, error) {
	p, err := s.repo.GetByAgentType(ctx, agentType)
	if err != nil {
		return "", err
	}
	return p.PromptText, nil
}

// List — GET /admin/prompts (zan-backend-tz-v2.md §3.7: "обе записи").
func (s *Service) List(ctx context.Context) ([]domain.AgentPrompt, error) {
	items, err := s.repo.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("prompt: list: %w", err)
	}
	return items, nil
}

// Update — PUT /admin/prompts/{agent_type} (zan-backend-tz-v2.md §3.7:
// "{prompt_text}").
func (s *Service) Update(ctx context.Context, agentType domain.AgentType, promptText string) (domain.AgentPrompt, error) {
	text := strings.TrimSpace(promptText)
	if text == "" {
		return domain.AgentPrompt{}, ErrEmptyText
	}
	updated, err := s.repo.Update(ctx, agentType, text, s.clock.Now())
	if err != nil {
		return domain.AgentPrompt{}, fmt.Errorf("prompt: update: %w", err)
	}
	return updated, nil
}
