package agent

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"zan-backend/internal/domain"
)

func TestBuildStructuredSystemPrompt_IncludesBasePromptAndJSONContract(t *testing.T) {
	prompt := buildStructuredSystemPrompt("Ты — юридический ассистент.")

	require.Contains(t, prompt, "Ты — юридический ассистент.")
	require.Contains(t, prompt, "answer_text")
	require.Contains(t, prompt, "needs_clarification")
}

func TestBuildQASystemPrompt_IncludesBasePromptAndPlainTextFormat(t *testing.T) {
	prompt := buildQASystemPrompt("  Ты — юридический ассистент.  ")

	require.True(t, strings.HasPrefix(prompt, "Ты — юридический ассистент."))
	require.Contains(t, prompt, qaAnswerInstructions)
	require.NotContains(t, prompt, "JSON", "Q&A answer is streamed as plain text, not JSON")
}

func TestBuildMessages_MapsSenderToRole(t *testing.T) {
	history := []domain.Message{
		{Sender: domain.MessageSenderUser, Text: "Вопрос"},
		{Sender: domain.MessageSenderAssistant, Text: "Ответ"},
	}

	messages := buildMessages(history, nil)

	require.Len(t, messages, 2)
	require.Equal(t, "user", messages[0].Role)
	require.Equal(t, "Вопрос", messages[0].Content)
	require.Equal(t, "assistant", messages[1].Role)
	require.Equal(t, "Ответ", messages[1].Content)
}

func TestBuildMessages_FileOnlyMessageGetsPlaceholderText(t *testing.T) {
	history := []domain.Message{
		{Sender: domain.MessageSenderUser, InputType: domain.MessageInputTypeFile, Text: ""},
	}

	messages := buildMessages(history, nil)

	require.Len(t, messages, 1)
	require.NotEmpty(t, messages[0].Content)
}

func TestBuildMessages_IncludesAttachmentText(t *testing.T) {
	extracted := "ДОГОВОР АРЕНДЫ\nСрок аренды — 11 месяцев."
	history := []domain.Message{{
		Sender: domain.MessageSenderUser, InputType: domain.MessageInputTypeFile, Text: "Проверьте договор",
		Attachments: []domain.FileAttachment{{
			OriginalName: "dogovor.pdf", ProcessingStatus: domain.FileProcessingStatusProcessed, ExtractedText: &extracted,
		}},
	}}

	content := buildMessages(history, nil)[0].Content

	require.True(t, strings.HasPrefix(content, "Проверьте договор"), "question text goes first")
	require.Contains(t, content, "dogovor.pdf")
	require.Contains(t, content, "Срок аренды — 11 месяцев.")
}

func TestBuildMessages_UnreadableAttachmentIsMarkedExplicitly(t *testing.T) {
	history := []domain.Message{{
		Sender: domain.MessageSenderUser, InputType: domain.MessageInputTypeFile,
		Attachments: []domain.FileAttachment{{
			OriginalName: "archive.zip", ProcessingStatus: domain.FileProcessingStatusError,
		}},
	}}

	content := buildMessages(history, nil)[0].Content

	require.Contains(t, content, "archive.zip")
	require.Contains(t, content, "прочитать не удалось")
}

func TestBuildMessages_TruncatesLongAttachment(t *testing.T) {
	extracted := strings.Repeat("x", maxAttachmentRunes+10)
	history := []domain.Message{{
		Sender: domain.MessageSenderUser,
		Attachments: []domain.FileAttachment{{
			OriginalName: "big.pdf", ProcessingStatus: domain.FileProcessingStatusProcessed, ExtractedText: &extracted,
		}},
	}}

	content := buildMessages(history, nil)[0].Content

	require.Equal(t, maxAttachmentRunes, strings.Count(content, "x"))
	require.Contains(t, content, "обрезан")
}

func TestBuildMessages_InlineAttachmentGoesAsFilePartWithLabel(t *testing.T) {
	extracted := "текст, который не должен дублироваться"
	history := []domain.Message{{
		Sender: domain.MessageSenderUser, Text: "Проверьте договор",
		Attachments: []domain.FileAttachment{{
			ID: "f1", OriginalName: "dogovor.pdf", MimeType: "application/pdf",
			ProcessingStatus: domain.FileProcessingStatusProcessed, ExtractedText: &extracted,
		}},
	}}
	part := openAIContentPart{Type: "file", File: &openAIFile{Filename: "dogovor.pdf", FileData: "data:application/pdf;base64,AA=="}}

	msg := buildMessages(history, map[string]openAIContentPart{"f1": part})[0]

	require.Equal(t, []openAIContentPart{part}, msg.Files)
	require.Contains(t, msg.Content, "Проверьте договор")
	require.Contains(t, msg.Content, "dogovor.pdf")
	require.NotContains(t, msg.Content, extracted, "inline file is not duplicated as extracted text")
}

func TestOpenAIMessage_MarshalJSON(t *testing.T) {
	plain, err := json.Marshal(openAIMessage{Role: "user", Content: "hi"})
	require.NoError(t, err)
	require.JSONEq(t, `{"role":"user","content":"hi"}`, string(plain), "text-only content stays a string")

	withFile, err := json.Marshal(openAIMessage{Role: "user", Content: "hi", Files: []openAIContentPart{
		{Type: "image_url", ImageURL: &openAIImageURL{URL: "data:image/png;base64,AA=="}},
	}})
	require.NoError(t, err)
	require.JSONEq(t, `{"role":"user","content":[
		{"type":"text","text":"hi"},
		{"type":"image_url","image_url":{"url":"data:image/png;base64,AA=="}}
	]}`, string(withFile))
}

func TestPromptVersionHash_DeterministicAndSensitiveToContent(t *testing.T) {
	h1 := promptVersionHash("prompt A")
	h2 := promptVersionHash("prompt A")
	h3 := promptVersionHash("prompt B")

	require.Equal(t, h1, h2)
	require.NotEqual(t, h1, h3)
	require.False(t, strings.Contains(h1, " "))
}
