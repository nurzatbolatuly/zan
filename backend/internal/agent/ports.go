package agent

import (
	"context"

	"zan-backend/internal/domain"
)

// PromptProvider — то немногое, что Client нужно от промптов: текст
// текущего промпта агента, читается заново на каждый вызов, без долгого
// кеша (правки в /admin/prompts применяются сразу). Порт объявлен здесь,
// где используется (BACKEND_CODING_STANDARDS.md §1.1) — *prompt.Service
// удовлетворяет ему структурно, без импорта пакета agent пакетом prompt.
type PromptProvider interface {
	GetPromptText(ctx context.Context, agentType domain.AgentType) (string, error)
}

// FileLoader — байты вложения из хранилища, чтобы передать PDF/изображение
// модели целиком (base64), а не только извлечённый текст.
// *storage.Client удовлетворяет ему структурно.
type FileLoader interface {
	Get(ctx context.Context, key string) ([]byte, error)
}
