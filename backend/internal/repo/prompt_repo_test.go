package repo_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"zan-backend/internal/domain"
	"zan-backend/internal/repo"
	"zan-backend/internal/service/prompt"
)

func TestPromptRepo_GetByAgentType_SeededByMigration(t *testing.T) {
	pool := setupTestDB(t)
	r := repo.NewPromptRepo(pool)

	qa, err := r.GetByAgentType(context.Background(), domain.AgentTypeQA)

	require.NoError(t, err) // migrations/000005_stage5_agent.up.sql — сид qa/document
	require.Equal(t, domain.AgentTypeQA, qa.AgentType)
	require.NotEmpty(t, qa.PromptText)

	doc, err := r.GetByAgentType(context.Background(), domain.AgentTypeDocument)
	require.NoError(t, err)
	require.Equal(t, domain.AgentTypeDocument, doc.AgentType)
	require.NotEmpty(t, doc.PromptText)
}

func TestPromptRepo_List_ReturnsBothSeededRows(t *testing.T) {
	pool := setupTestDB(t)
	r := repo.NewPromptRepo(pool)

	items, err := r.List(context.Background())

	require.NoError(t, err)
	require.Len(t, items, 2)
}

func TestPromptRepo_Update_PersistsNewTextAndTimestamp(t *testing.T) {
	pool := setupTestDB(t)
	r := repo.NewPromptRepo(pool)

	before, err := r.GetByAgentType(context.Background(), domain.AgentTypeQA)
	require.NoError(t, err)

	updated, err := r.Update(context.Background(), domain.AgentTypeQA, "новый текст промпта", before.UpdatedAt.Add(time.Hour))
	require.NoError(t, err)
	require.Equal(t, "новый текст промпта", updated.PromptText)

	reloaded, err := r.GetByAgentType(context.Background(), domain.AgentTypeQA)
	require.NoError(t, err)
	require.Equal(t, "новый текст промпта", reloaded.PromptText)
}

func TestPromptRepo_GetByAgentType_UnknownReturnsNotFound(t *testing.T) {
	pool := setupTestDB(t)
	r := repo.NewPromptRepo(pool)

	_, err := r.GetByAgentType(context.Background(), domain.AgentType("bogus"))

	require.ErrorIs(t, err, prompt.ErrNotFound)
}
