package domain

import (
	"fmt"
	"time"
)

// FilePurpose — что за роль у вложения (zan-backend-tz-v2.md §2.8).
type FilePurpose string

const (
	// FilePurposeAnalysisInput — файл, который пользователь прикрепил к
	// сообщению для анализа (POST /files/upload -> file_ids в
	// POST /threads|/threads/{id}/messages).
	FilePurposeAnalysisInput FilePurpose = "analysis_input"
	// FilePurposeGeneratedOutput — результат генерации документа
	// (Stage 6: FileAttachment создаётся backend'ом самим, не загрузкой).
	FilePurposeGeneratedOutput FilePurpose = "generated_output"
)

// ParseFilePurpose валидирует сырое значение (из БД) как FilePurpose.
func ParseFilePurpose(s string) (FilePurpose, error) {
	switch v := FilePurpose(s); v {
	case FilePurposeAnalysisInput, FilePurposeGeneratedOutput:
		return v, nil
	default:
		return "", fmt.Errorf("domain: invalid file purpose %q", s)
	}
}

// FileProcessingStatus — прогресс FilesService.Extract над этим вложением
// (zan-backend-tz-v2.md §2.8, zan-backend-tz-v3.md §5.3). Error — не
// блокирует сохранение файла: он всё равно доступен по GET /files/{id} и
// может быть приложен к сообщению, просто без извлечённого текста (Stage 5
// решает, что делать с сообщением, у которого файл "не читается", —
// needs_clarification, а не ошибка треда).
type FileProcessingStatus string

const (
	FileProcessingStatusPending   FileProcessingStatus = "pending"
	FileProcessingStatusProcessed FileProcessingStatus = "processed"
	FileProcessingStatusError     FileProcessingStatus = "error"
)

// ParseFileProcessingStatus валидирует сырое значение (из БД) как FileProcessingStatus.
func ParseFileProcessingStatus(s string) (FileProcessingStatus, error) {
	switch v := FileProcessingStatus(s); v {
	case FileProcessingStatusPending, FileProcessingStatusProcessed, FileProcessingStatusError:
		return v, nil
	default:
		return "", fmt.Errorf("domain: invalid file processing status %q", s)
	}
}

// FileAttachment — файл, прикреплённый к Thread/Message, хранится в
// S3-совместимом сторадже по ссылке (zan-backend-tz-v2.md §2.8).
//
// ObjectKey — ключ объекта в бакете ("uploads/{session_id}/{id}.{ext}"),
// не готовая ссылка: presigned-ссылка на скачивание всегда перевыпускается
// заново на момент ответа (POST /files/upload, GET /files/{id}) —
// internal/service/file, не хранится в БД, потому что presigned-ссылка
// имеет срок жизни и протухла бы задолго до того, как пользователь снова
// откроет историю треда (см. internal/platform/storage.Client.PresignGetPublic).
//
// MessageID — nullable: файл существует с момента успешной загрузки, ещё
// до того, как он привязан к конкретному сообщению
// (zan-backend-tz-v2.md §3.2: "{..., file_ids?}" — сначала upload,
// потом ссылка на file_id в теле POST /threads|/messages).
//
// ThreadID — Stage 6, только для Purpose=FilePurposeGeneratedOutput: тред,
// для которого сгенерирован документ (internal/service/document читает по
// нему GET /threads/{id}/document?format=, не через JOIN messages). NULL
// для FilePurposeAnalysisInput.
//
// ExtractedText — результат FilesService.Extract, nullable: пусто, пока
// ProcessingStatus=Pending, и остаётся пустым при ProcessingStatus=Error
// (zan-backend-tz-v3.md §5.3 — "файл не удалось прочитать", не пустая
// success-строка). Поле в v2-схеме отсутствовало — добавлено на Stage 4:
// без него извлечённый текст было бы негде хранить для последующей
// передачи агенту (Stage 5), не сохранять его вообще означало бы повторно
// вызывать Extract на каждый Q&A-запрос по тому же файлу.
type FileAttachment struct {
	ID               string
	SessionID        string
	MessageID        *string
	ThreadID         *string
	ObjectKey        string
	OriginalName     string
	MimeType         string
	SizeBytes        int64
	Purpose          FilePurpose
	OutputFormats    []string
	ProcessingStatus FileProcessingStatus
	ExtractedText    *string
	CreatedAt        time.Time
}

// IsAttached — файл уже привязан к сообщению (MessageID заполнен) и не
// может быть привязан повторно к другому.
func (f FileAttachment) IsAttached() bool {
	return f.MessageID != nil
}
