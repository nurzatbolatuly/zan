package agent

import (
	"context"

	"zan-backend/internal/domain"
	"zan-backend/internal/grpcclient"
)

// PromptProvider — то немногое, что Client нужно от промптов: текст
// текущего промпта агента, читается заново на каждый вызов, без долгого
// кеша (zan-backend-tz-v2.md §4.6). Порт объявлен здесь, где используется
// (BACKEND_CODING_STANDARDS.md §1.1) — *prompt.Service удовлетворяет ему
// структурно, без импорта пакета agent пакетом prompt.
type PromptProvider interface {
	GetPromptText(ctx context.Context, agentType domain.AgentType) (string, error)
}

// RagSearcher — то немногое, что Client нужно от RAG: кандидаты-источники
// под текст запроса. *grpcclient.Client удовлетворяет этому структурно.
type RagSearcher interface {
	SearchSources(ctx context.Context, queryText string, topK int) ([]grpcclient.RagMatch, error)
}
