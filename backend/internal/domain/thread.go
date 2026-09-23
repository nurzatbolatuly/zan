package domain

import (
	"fmt"
	"time"
)

// Thread — обращение/консультация (zan-backend-tz-v2.md §2.6), со
// статус-машиной (thread_status.go). ServiceID — какая услуга списывается
// с баланса за КАЖДЫЙ вопрос треда (`qa`/`doc`): один вопрос = одна
// консультация, бесплатных уточнений нет. Оплачен ли текущий вопрос —
// видно только по Status (awaiting_payment), отдельного флага нет. Title/PreviewText/MessageCount —
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
	CreatedAt     time.Time
	LastMessageAt *time.Time
	DeletedAt     *time.Time
}

// IsDeleted — soft-deleted (zan-backend-tz-v2.md §4.5), исключается из
// GET /threads и недоступен по GET /threads/{id}.
func (t Thread) IsDeleted() bool {
	return t.DeletedAt != nil
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
// §2.7/§4.6: `[{ref, quote}]`) — заполняется генерацией документа
// (структурированный JSON-ответ LLM); Q&A-ответ источников не несёт.
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
type Message struct {
	ID               string
	ThreadID         string
	Sender           MessageSender
	InputType        MessageInputType
	Text             string
	Sources          []Source
	Findings         []Finding
	Feedback         *MessageFeedback
	ProcessingTimeMs *int
	CreatedAt        time.Time

	// Attachments — файлы, прикреплённые пользователем к сообщению.
	// ExtractedText заполнен только в истории для LLM
	// (ThreadRepo.GetConversation); GetMessages (REST) отдаёт метаданные без
	// него — извлечённый текст не нужен клиенту и не тянется на каждое
	// чтение треда.
	Attachments []FileAttachment
}
