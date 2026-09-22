package domain

import (
	"fmt"
	"time"
)

// Thread — обращение/консультация (zan-backend-tz-v2.md §2.6), со
// статус-машиной (thread_status.go). ServiceID — какая услуга была/будет
// списана на открытие (`qa`/`doc`). Title/PreviewText/MessageCount —
// денормализация для истории (§4.5), поддерживается на каждой мутации
// вызывающим кодом (internal/service/thread), не триггером БД.
type Thread struct {
	ID            string
	SessionID     string
	ServiceID     string
	Status        ThreadStatus
	Title         string
	PreviewText   string
	MessageCount  int
	IsPaid        bool
	PaidAt        *time.Time
	FreeUntil     *time.Time
	CreatedAt     time.Time
	LastMessageAt *time.Time
	ClosedAt      *time.Time
	DeletedAt     *time.Time
}

// IsDeleted — soft-deleted (zan-backend-tz-v2.md §4.5), исключается из
// GET /threads и недоступен по GET /threads/{id}.
func (t Thread) IsDeleted() bool {
	return t.DeletedAt != nil
}

// IsClosed — free_until истёк (§4.3): новое сообщение по теме требует
// нового треда (новое списание), в этот же тред больше нельзя писать.
func (t Thread) IsClosed(now time.Time) bool {
	if t.ClosedAt != nil {
		return true
	}
	return t.FreeUntil != nil && !now.Before(*t.FreeUntil)
}

// MessageSender — кто написал сообщение (zan-backend-tz-v2.md §2.7).
type MessageSender string

const (
	MessageSenderUser      MessageSender = "user"
	MessageSenderAssistant MessageSender = "assistant"
)

// ParseMessageSender валидирует сырое значение (из БД) как MessageSender.
func ParseMessageSender(s string) (MessageSender, error) {
	switch v := MessageSender(s); v {
	case MessageSenderUser, MessageSenderAssistant:
		return v, nil
	default:
		return "", fmt.Errorf("domain: invalid message sender %q", s)
	}
}

// MessageInputType — как пользователь ввёл сообщение (zan-backend-tz-v2.md
// §2.7). Voice/File заведены в схеме и enum'е с первого дня (Stage 1
// полная схема), но реально обрабатываются только с Stage 4
// (STT/извлечение файлов) — internal/service/thread до тех пор принимает
// только Text, отклоняя остальные явной ошибкой (не тихой заглушкой).
type MessageInputType string

const (
	MessageInputTypeText  MessageInputType = "text"
	MessageInputTypeVoice MessageInputType = "voice"
	MessageInputTypeFile  MessageInputType = "file"
)

// ParseMessageInputType валидирует сырое значение (из запроса/БД) как MessageInputType.
func ParseMessageInputType(s string) (MessageInputType, error) {
	switch v := MessageInputType(s); v {
	case MessageInputTypeText, MessageInputTypeVoice, MessageInputTypeFile:
		return v, nil
	default:
		return "", fmt.Errorf("domain: invalid message input type %q", s)
	}
}

// MessageFeedback — оценка сообщения ассистента (zan-backend-tz-v2.md §2.7/§3.6).
type MessageFeedback string

const (
	MessageFeedbackLike    MessageFeedback = "like"
	MessageFeedbackDislike MessageFeedback = "dislike"
)

// ParseMessageFeedback валидирует сырое значение (из тела запроса) как MessageFeedback.
func ParseMessageFeedback(s string) (MessageFeedback, error) {
	switch v := MessageFeedback(s); v {
	case MessageFeedbackLike, MessageFeedbackDislike:
		return v, nil
	default:
		return "", fmt.Errorf("domain: invalid message feedback %q", s)
	}
}

// Source — статья закона, привязанная к ответу (zan-backend-tz-v2.md
// §2.7/§4.6: `[{ref, quote}]`). Верификация источников (сверка с реальным
// RAG-корпусом) — Stage 5, здесь только форма значения.
type Source struct {
	Ref   string
	Quote string
}

// Finding — структурированный пункт анализа (zan-backend-tz-v2.md §2.7:
// `[{title, body}]`, для ответов по документам).
type Finding struct {
	Title string
	Body  string
}

// Message — сообщение внутри Thread (zan-backend-tz-v2.md §2.7).
// UnverifiedSources — Stage 5 (backend-roadmap.md §6 открытый вопрос №11:
// "бэк-часть — флаг unverified_sources в ответе — реализуется в любом
// случае"), считается internal/agent.verifySources сразу после ответа LLM,
// валиден только для Sender=Assistant (у пользовательских сообщений всегда
// false) — не проверка "хорошего тона", а сигнал, что LLM процитировала
// источник вне того, что реально вернул RAG (подозрение на галлюцинацию
// закона, backend-roadmap.md §1.4).
type Message struct {
	ID                string
	ThreadID          string
	Sender            MessageSender
	InputType         MessageInputType
	Text              string
	Sources           []Source
	Findings          []Finding
	UnverifiedSources bool
	Feedback          *MessageFeedback
	ProcessingTimeMs  *int
	CreatedAt         time.Time
}
