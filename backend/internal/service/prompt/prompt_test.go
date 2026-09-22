package prompt_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"zan-backend/internal/domain"
	"zan-backend/internal/platform/clock"
	"zan-backend/internal/service/prompt"
)

type fakeRepo struct {
	prompts map[domain.AgentType]domain.AgentPrompt
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		prompts: map[domain.AgentType]domain.AgentPrompt{
			domain.AgentTypeQA:       {ID: "1", AgentType: domain.AgentTypeQA, PromptText: "qa prompt"},
			domain.AgentTypeDocument: {ID: "2", AgentType: domain.AgentTypeDocument, PromptText: "document prompt"},
		},
	}
}

func (r *fakeRepo) GetByAgentType(_ context.Context, agentType domain.AgentType) (domain.AgentPrompt, error) {
	p, ok := r.prompts[agentType]
	if !ok {
		return domain.AgentPrompt{}, prompt.ErrNotFound
	}
	return p, nil
}

func (r *fakeRepo) List(context.Context) ([]domain.AgentPrompt, error) {
	items := make([]domain.AgentPrompt, 0, len(r.prompts))
	for _, p := range r.prompts {
		items = append(items, p)
	}
	return items, nil
}

func (r *fakeRepo) Update(_ context.Context, agentType domain.AgentType, promptText string, updatedAt time.Time) (domain.AgentPrompt, error) {
	p, ok := r.prompts[agentType]
	if !ok {
		return domain.AgentPrompt{}, prompt.ErrNotFound
	}
	p.PromptText = promptText
	p.UpdatedAt = updatedAt
	r.prompts[agentType] = p
	return p, nil
}

func TestGetPromptText_ReturnsStoredText(t *testing.T) {
	svc := prompt.New(newFakeRepo(), clock.Real{})

	text, err := svc.GetPromptText(context.Background(), domain.AgentTypeQA)

	require.NoError(t, err)
	require.Equal(t, "qa prompt", text)
}

func TestGetPromptText_NotFound(t *testing.T) {
	repo := newFakeRepo()
	delete(repo.prompts, domain.AgentTypeQA)
	svc := prompt.New(repo, clock.Real{})

	_, err := svc.GetPromptText(context.Background(), domain.AgentTypeQA)

	require.ErrorIs(t, err, prompt.ErrNotFound)
}

func TestList_ReturnsBothPrompts(t *testing.T) {
	svc := prompt.New(newFakeRepo(), clock.Real{})

	items, err := svc.List(context.Background())

	require.NoError(t, err)
	require.Len(t, items, 2)
}

func TestUpdate_TrimsAndPersistsText(t *testing.T) {
	svc := prompt.New(newFakeRepo(), clock.Real{})

	updated, err := svc.Update(context.Background(), domain.AgentTypeQA, "  новый текст промпта  ")

	require.NoError(t, err)
	require.Equal(t, "новый текст промпта", updated.PromptText)

	text, err := svc.GetPromptText(context.Background(), domain.AgentTypeQA)
	require.NoError(t, err)
	require.Equal(t, "новый текст промпта", text)
}

func TestUpdate_EmptyTextRejected(t *testing.T) {
	svc := prompt.New(newFakeRepo(), clock.Real{})

	_, err := svc.Update(context.Background(), domain.AgentTypeQA, "   ")

	require.ErrorIs(t, err, prompt.ErrEmptyText)
}

func TestUpdate_UnknownAgentType(t *testing.T) {
	svc := prompt.New(newFakeRepo(), clock.Real{})

	_, err := svc.Update(context.Background(), domain.AgentType("bogus"), "text")

	require.ErrorIs(t, err, prompt.ErrNotFound)
}
