package repo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"zan-backend/internal/domain"
	"zan-backend/internal/service/prompt"
)

// PromptRepo — реализация prompt.Repository поверх core.agent_prompts
// (Stage 5, ровно 2 строки, zan-backend-tz-v2.md §2.9 — сид данные см.
// backend/migrations/000005_stage5_agent.up.sql).
type PromptRepo struct {
	db *pgxpool.Pool
}

// NewPromptRepo строит PromptRepo поверх общего пула соединений.
func NewPromptRepo(db *pgxpool.Pool) *PromptRepo {
	return &PromptRepo{db: db}
}

func (r *PromptRepo) GetByAgentType(ctx context.Context, agentType domain.AgentType) (domain.AgentPrompt, error) {
	row := r.db.QueryRow(ctx, `
		SELECT id, agent_type, prompt_text, updated_at
		FROM core.agent_prompts
		WHERE agent_type = $1
	`, string(agentType))

	var pr promptRow
	if err := pr.scan(row); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.AgentPrompt{}, prompt.ErrNotFound
		}
		return domain.AgentPrompt{}, fmt.Errorf("repo: get prompt: %w", err)
	}
	return pr.toDomain()
}

func (r *PromptRepo) List(ctx context.Context) ([]domain.AgentPrompt, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, agent_type, prompt_text, updated_at
		FROM core.agent_prompts
		ORDER BY agent_type
	`)
	if err != nil {
		return nil, fmt.Errorf("repo: list prompts: %w", err)
	}
	defer rows.Close()

	var items []domain.AgentPrompt
	for rows.Next() {
		var pr promptRow
		if err := pr.scan(rows); err != nil {
			return nil, fmt.Errorf("repo: list prompts: scan: %w", err)
		}
		p, err := pr.toDomain()
		if err != nil {
			return nil, err
		}
		items = append(items, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("repo: list prompts: %w", err)
	}
	return items, nil
}

func (r *PromptRepo) Update(ctx context.Context, agentType domain.AgentType, promptText string, updatedAt time.Time) (domain.AgentPrompt, error) {
	row := r.db.QueryRow(ctx, `
		UPDATE core.agent_prompts
		SET prompt_text = $2,
		    updated_at  = $3
		WHERE agent_type = $1
		RETURNING id, agent_type, prompt_text, updated_at
	`, string(agentType), promptText, toTimestamptz(updatedAt))

	var pr promptRow
	if err := pr.scan(row); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.AgentPrompt{}, prompt.ErrNotFound
		}
		return domain.AgentPrompt{}, fmt.Errorf("repo: update prompt: %w", err)
	}
	return pr.toDomain()
}

type promptRow struct {
	id         pgtype.UUID
	agentType  string
	promptText string
	updatedAt  pgtype.Timestamptz
}

func (pr *promptRow) scan(row rowScanner) error {
	return row.Scan(&pr.id, &pr.agentType, &pr.promptText, &pr.updatedAt)
}

func (pr promptRow) toDomain() (domain.AgentPrompt, error) {
	agentType, err := domain.ParseAgentType(pr.agentType)
	if err != nil {
		return domain.AgentPrompt{}, fmt.Errorf("repo: prompt row: %w", err)
	}
	return domain.AgentPrompt{
		ID:         fromPgUUID(pr.id),
		AgentType:  agentType,
		PromptText: pr.promptText,
		UpdatedAt:  pr.updatedAt.Time,
	}, nil
}
