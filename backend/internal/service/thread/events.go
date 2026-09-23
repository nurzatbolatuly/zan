package thread

import (
	"context"

	"zan-backend/internal/domain"
)

// EventPublisher — порт публикации живых событий обработки треда для
// WS-подписчиков (Stage 9, WS-стриминг статуса и токенов ответа —
// internal/httpserver/ws_thread.go, GET /ws/threads/{id}). Реализация —
// internal/wshub.Hub, структурно удовлетворяет интерфейсу (BACKEND_CODING_
// STANDARDS.md §1.1 — интерфейс объявлен там, где используется). Методы
// ничего не возвращают: публикация — best-effort side effect обработки
// треда, отсутствие живых подписчиков (никто не открыл WS) — штатный
// случай, не ошибка, REST (GET /threads/{id}) остаётся источником правды
// независимо от того, слушает ли кто-то WS.
type EventPublisher interface {
	// PublishStatus — тред перешёл в новый статус (
	// activateFromBalance/finishWithAnswer/finishWithError).
	PublishStatus(ctx context.Context, threadID string, status domain.ThreadStatus, previewText string)

	// PublishAnswerDelta — очередной фрагмент текста ответа (streaming-вызов
	// LLM, см. AgentRequest.OnDelta).
	PublishAnswerDelta(ctx context.Context, threadID string, delta string)

	// PublishAnswerDone — раунд завершён успехом (Done), msg — уже
	// сохранённое сообщение ассистента.
	PublishAnswerDone(ctx context.Context, threadID string, msg domain.Message, status domain.ThreadStatus)

	// PublishError — раунд завершён ошибкой (тред переведён в Error).
	// message — уже безопасный для показа пользователю текст, не сырая
	// ошибка (BACKEND_CODING_STANDARDS.md §8).
	PublishError(ctx context.Context, threadID string, code, message string)
}
