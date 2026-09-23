// Package wshub — pub/sub-хаб живых WS-подписчиков на события обработки
// треда (Stage 9, WS-стриминг статуса и токенов ответа — переоткрытое
// решение Stage 6, см. instructions.md §9). Один Hub на процесс (сконструирован
// в cmd/api, не package-level var — BACKEND_CODING_STANDARDS.md §2), один
// топик на thread_id, живёт, пока есть хотя бы один подписчик или тред
// обрабатывается. Пакет не знает про gin/JSON/WS-протокол — только домен
// (domain.ThreadStatus/domain.Message), конвертация в wire-JSON — на границе
// internal/httpserver/ws_thread.go.
package wshub

import (
	"context"
	"log/slog"
	"sync"

	"zan-backend/internal/domain"
	"zan-backend/internal/platform/logger"
)

// EventType — тип события, зеркалит "type" в wire-протоколе
// GET /ws/threads/{id} (см. internal/httpserver/ws_thread.go).
type EventType string

const (
	// EventThreadStatus — переход статуса треда (Repository.UpdateStatus) —
	// единственное событие, гарантированно отправляемое
	// первым любому новому подписчику (снапшот).
	EventThreadStatus EventType = "thread_status"
	// EventAnswerDelta — очередной фрагмент текста ответа (Фаза 1
	// streaming-вызова, internal/agent/streaming.go).
	EventAnswerDelta EventType = "answer_delta"
	// EventAnswerDone — раунд обработки завершён успехом (Done),
	// Message — уже сохранённое сообщение ассистента.
	EventAnswerDone EventType = "answer_done"
	// EventError — раунд обработки завершён ошибкой (тред переведён в
	// Error) — Code/ErrorMsg зеркалят apierror-словарь, не сырую ошибку
	// (BACKEND_CODING_STANDARDS.md §8).
	EventError EventType = "error"
)

// Event — одно событие обработки треда. Какие поля заполнены — зависит от
// Type (см. константы выше и Publish*-методы Hub).
type Event struct {
	Type        EventType
	ThreadID    string
	Seq         int
	Status      domain.ThreadStatus
	PreviewText string
	Delta       string
	Message     domain.Message
	Code        string
	ErrorMsg    string
}

// subscriberBufferSize — сколько событий может накопиться непрочитанными у
// одного подписчика, прежде чем Publish* сочтёт его медленным и отключит
// (см. topic.broadcastLocked) — один зависший WS-клиент не должен
// притормаживать рассылку остальным.
const subscriberBufferSize = 64

// subscriber — один живой WS-коннекшен на конкретный thread_id.
type subscriber struct {
	send chan Event
}

// Subscription — хендл подписки, возвращённый Hub.Subscribe. Events() —
// единственный способ читать события; Hub.Unsubscribe вызывается ровно один
// раз (обычно defer сразу после Subscribe), когда подписчик перестаёт читать.
type Subscription struct {
	threadID string
	sub      *subscriber
}

// Events — канал живых событий данного топика после снапшота, отданного
// Subscribe. Закрывается Hub.Unsubscribe.
func (s *Subscription) Events() <-chan Event {
	return s.sub.send
}

// topic — состояние одного треда: текущий снапшот (для будущих Subscribe) +
// живые подписчики. Один мьютекс на топик — Subscribe и Publish* берут его
// строго последовательно, поэтому между "прочитать снапшот" и "начать
// слушать живые события" не может потеряться ни одно событие: новый
// подписчик либо успевает попасть в fan-out конкретного Publish (после),
// либо получает уже обновлённый снапшот, включающий его эффект (до) — но
// никогда не оказывается в промежутке между ними.
type topic struct {
	mu          sync.Mutex
	seq         int
	status      domain.ThreadStatus
	previewText string
	partialText string
	subs        map[*subscriber]struct{}
}

// Hub — реестр топиков, один на процесс. Реализует thread.EventPublisher
// (internal/service/thread/events.go) структурно.
type Hub struct {
	mu     sync.Mutex
	topics map[string]*topic
}

// NewHub — конструктор, вызывается один раз в cmd/api (composition root).
func NewHub() *Hub {
	return &Hub{topics: make(map[string]*topic)}
}

func (h *Hub) topicFor(threadID string) *topic {
	h.mu.Lock()
	defer h.mu.Unlock()
	t, ok := h.topics[threadID]
	if !ok {
		t = &topic{subs: make(map[*subscriber]struct{})}
		h.topics[threadID] = t
	}
	return t
}

// Subscribe регистрирует нового подписчика на threadID и атомарно (под тем
// же мьютексом топика, что и Publish*) возвращает текущий снапшот в виде
// упорядоченного списка событий: всегда ThreadStatus первым, плюс
// AnswerDelta с уже накопленным текстом текущего раунда, если стрим уже
// начался и ещё не завершён (partialText непуст). Пустой topic (ни одного
// Publish* ещё не было) — снапшот пуст, вызывающий код (ws_thread.go)
// в этом случае ничего не пишет клиенту до первого живого события.
func (h *Hub) Subscribe(threadID string) (*Subscription, []Event) {
	t := h.topicFor(threadID)
	t.mu.Lock()
	defer t.mu.Unlock()

	sub := &subscriber{send: make(chan Event, subscriberBufferSize)}
	t.subs[sub] = struct{}{}

	var snapshot []Event
	if t.status != "" {
		snapshot = append(snapshot, Event{
			Type: EventThreadStatus, ThreadID: threadID, Seq: t.seq,
			Status: t.status, PreviewText: t.previewText,
		})
	}
	if t.partialText != "" {
		snapshot = append(snapshot, Event{
			Type: EventAnswerDelta, ThreadID: threadID, Seq: t.seq, Delta: t.partialText,
		})
	}
	return &Subscription{threadID: threadID, sub: sub}, snapshot
}

// Unsubscribe отключает подписчика: закрывает канал событий (Events()
// возвращает закрытый канал — читающий цикл вызывающего кода завершается
// range'ом) и, если топик опустел и не в процессе обработки (status не
// Processing), удаляет его из реестра — чтобы карта топиков не росла
// неограниченно за время жизни процесса.
func (h *Hub) Unsubscribe(threadID string, sub *Subscription) {
	t := h.topicFor(threadID)
	t.mu.Lock()
	if _, ok := t.subs[sub.sub]; ok {
		delete(t.subs, sub.sub)
		close(sub.sub.send)
	}
	empty := len(t.subs) == 0 && t.status != domain.ThreadStatusProcessing
	t.mu.Unlock()

	if empty {
		h.mu.Lock()
		if cur, ok := h.topics[threadID]; ok && cur == t {
			delete(h.topics, threadID)
		}
		h.mu.Unlock()
	}
}

// broadcastLocked рассылает ev всем текущим подписчикам топика,
// НИКОГДА не блокируясь: канал с заполненным буфером — подписчик считается
// медленным, отключается (закрывается его канал, убирается из subs) вместо
// того, чтобы тормозить рассылку остальным (и обработку самого треда —
// Publish* вызывается синхронно из internal/service/thread). Вызывается под
// t.mu — сам вызов не блокирующий (select/default), поэтому удержание
// мьютекса на время рассылки безопасно и не бьёт по конкурентности.
func (t *topic) broadcastLocked(ctx context.Context, threadID string, ev Event) {
	l := logger.FromContext(ctx)
	for sub := range t.subs {
		select {
		case sub.send <- ev:
		default:
			l.Warn("slow_ws_subscriber_dropped", slog.Group("context",
				slog.String("thread_id", threadID),
				slog.String("event_type", string(ev.Type)),
			))
			delete(t.subs, sub)
			close(sub.send)
		}
	}
}

// PublishStatus — тред перешёл в новый статус (Repository.UpdateStatus).
// previewText — тот же короткий текст, что уже лежит в
// domain.Thread.PreviewText (thread.previewAwaitingPayment/previewProcessing/...).
func (h *Hub) PublishStatus(ctx context.Context, threadID string, status domain.ThreadStatus, previewText string) {
	t := h.topicFor(threadID)
	t.mu.Lock()
	defer t.mu.Unlock()

	t.seq++
	t.status = status
	t.previewText = previewText
	if status != domain.ThreadStatusProcessing {
		// Новый раунд ещё не начался (AwaitingPayment) либо раунд уже завершён
		// (Done/Error/Canceled) — накопленный текст предыдущего
		// раунда больше не актуален как "текущий стрим".
		t.partialText = ""
	}
	t.broadcastLocked(ctx, threadID, Event{
		Type: EventThreadStatus, ThreadID: threadID, Seq: t.seq,
		Status: status, PreviewText: previewText,
	})
}

// PublishAnswerDelta — очередной фрагмент текста ответа (Фаза 1
// streaming-вызова). Накапливается в topic.partialText, чтобы Subscribe,
// пришедший посреди стрима, мог отдать уже сгенерированный текст одним
// снапшотным событием (см. Subscribe).
func (h *Hub) PublishAnswerDelta(ctx context.Context, threadID string, delta string) {
	if delta == "" {
		return
	}
	t := h.topicFor(threadID)
	t.mu.Lock()
	defer t.mu.Unlock()

	t.seq++
	t.partialText += delta
	t.broadcastLocked(ctx, threadID, Event{
		Type: EventAnswerDelta, ThreadID: threadID, Seq: t.seq, Delta: delta,
	})
}

// PublishAnswerDone — раунд обработки завершён успехом (Done), msg —
// уже сохранённое сообщение ассистента (та же форма, что легла в БД).
func (h *Hub) PublishAnswerDone(ctx context.Context, threadID string, msg domain.Message, status domain.ThreadStatus) {
	t := h.topicFor(threadID)
	t.mu.Lock()
	defer t.mu.Unlock()

	t.seq++
	t.status = status
	t.partialText = ""
	t.broadcastLocked(ctx, threadID, Event{
		Type: EventAnswerDone, ThreadID: threadID, Seq: t.seq, Status: status, Message: msg,
	})
}

// PublishError — раунд обработки завершён ошибкой, тред переведён в Error.
// message — уже безопасный для показа пользователю текст (previewError и
// т.п.), не сырая ошибка (BACKEND_CODING_STANDARDS.md §8).
func (h *Hub) PublishError(ctx context.Context, threadID string, code, message string) {
	t := h.topicFor(threadID)
	t.mu.Lock()
	defer t.mu.Unlock()

	t.seq++
	t.status = domain.ThreadStatusError
	t.partialText = ""
	t.broadcastLocked(ctx, threadID, Event{
		Type: EventError, ThreadID: threadID, Seq: t.seq, Code: code, ErrorMsg: message,
	})
}
