package agent

import (
	"context"
	"encoding/base64"
	"fmt"
	"log/slog"
	"strings"

	"zan-backend/internal/domain"
)

const mimePDF = "application/pdf"

// inlineMimeTypes — типы, которые модель читает сама (Chat Completions:
// PDF — часть "file", изображения — "image_url"): сканы, таблицы и печати
// она видит лучше, чем их извлечённый текст.
var inlineMimeTypes = map[string]bool{
	mimePDF:      true,
	"image/png":  true,
	"image/jpeg": true,
}

// maxAttachmentRunes — потолок извлечённого текста одного вложения в
// промпте: 15 МБ PDF может дать миллионы символов и не влезть в контекст
// модели. Обрезка явная (пометка в тексте), чтобы модель не принимала часть
// документа за целый.
const maxAttachmentRunes = 100_000

// loadInlineFiles — вложения ТЕКУЩЕГО вопроса (последнее сообщение истории,
// если оно от пользователя) с типом из inlineMimeTypes: байты из хранилища →
// base64-часть content, ключ — file ID. Прошлые вопросы треда идут
// извлечённым текстом: иначе каждый новый вопрос заново оплачивал бы все
// ранее приложенные документы. Файл, который не удалось скачать, остаётся
// текстом — вызов из-за этого не проваливается.
func (c *Client) loadInlineFiles(ctx context.Context, l *slog.Logger, history []domain.Message) map[string]openAIContentPart {
	if len(history) == 0 {
		return nil
	}
	current := history[len(history)-1]
	if current.Sender != domain.MessageSenderUser {
		return nil
	}

	parts := make(map[string]openAIContentPart)
	for _, f := range current.Attachments {
		if !inlineMimeTypes[f.MimeType] {
			continue
		}
		data, err := c.files.Get(ctx, f.ObjectKey)
		if err != nil {
			l.Warn("llm_inline_file_load_failed", slog.Group("context",
				slog.String("file_id", f.ID),
				slog.String("error", err.Error()),
			))
			continue
		}
		parts[f.ID] = inlineFilePart(f, data)
	}
	return parts
}

func inlineFilePart(f domain.FileAttachment, data []byte) openAIContentPart {
	dataURL := "data:" + f.MimeType + ";base64," + base64.StdEncoding.EncodeToString(data)
	if f.MimeType == mimePDF {
		return openAIContentPart{Type: "file", File: &openAIFile{Filename: f.OriginalName, FileData: dataURL}}
	}
	return openAIContentPart{Type: "image_url", ImageURL: &openAIImageURL{URL: dataURL}}
}

// attachmentBlock — вложение извлечённым текстом. Файл без извлечённого
// текста (тип без извлечения, битый скан, helper/ недоступен) помечается
// явно — модель должна сказать, что не смогла прочитать файл, а не отвечать
// так, будто его не было.
func attachmentBlock(f domain.FileAttachment) string {
	if f.ProcessingStatus != domain.FileProcessingStatusProcessed || f.ExtractedText == nil {
		return fmt.Sprintf("[Вложение «%s»: текст файла прочитать не удалось]", f.OriginalName)
	}
	text := []rune(strings.TrimSpace(*f.ExtractedText))
	truncated := len(text) > maxAttachmentRunes
	if truncated {
		text = text[:maxAttachmentRunes]
	}
	block := fmt.Sprintf("[Вложение «%s»]\n%s", f.OriginalName, string(text))
	if truncated {
		block += "\n[Текст вложения обрезан — документ длиннее, чем помещается в запрос]"
	}
	return block
}
