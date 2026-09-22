package agent

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"zan-backend/internal/domain"
	"zan-backend/internal/grpcclient"
)

func TestBuildSystemPrompt_IncludesBasePromptAndJSONContract(t *testing.T) {
	prompt := buildSystemPrompt("Ты — юридический ассистент.", nil)

	require.Contains(t, prompt, "Ты — юридический ассистент.")
	require.Contains(t, prompt, "answer_text")
	require.Contains(t, prompt, "needs_clarification")
}

func TestBuildSystemPrompt_NoMatches_SaysSoExplicitly(t *testing.T) {
	prompt := buildSystemPrompt("base", nil)

	require.Contains(t, prompt, "не найдено ни одной подходящей статьи")
}

func TestBuildSystemPrompt_ListsMatchesAsOnlyCitableSources(t *testing.T) {
	matches := []grpcclient.RagMatch{
		{Ref: "ст. 157 ТК РК", Quote: "работник вправе уволиться..."},
	}

	prompt := buildSystemPrompt("base", matches)

	require.Contains(t, prompt, "ст. 157 ТК РК")
	require.Contains(t, prompt, "работник вправе уволиться...")
}

func TestBuildMessages_MapsSenderToRole(t *testing.T) {
	history := []domain.Message{
		{Sender: domain.MessageSenderUser, Text: "Вопрос"},
		{Sender: domain.MessageSenderAssistant, Text: "Ответ"},
	}

	messages := buildMessages(history)

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

	messages := buildMessages(history)

	require.Len(t, messages, 1)
	require.NotEmpty(t, messages[0].Content)
}

func TestLastUserMessageText_ReturnsMostRecentUserMessage(t *testing.T) {
	history := []domain.Message{
		{Sender: domain.MessageSenderUser, Text: "первый вопрос"},
		{Sender: domain.MessageSenderAssistant, Text: "ответ"},
		{Sender: domain.MessageSenderUser, Text: "  уточнение  "},
	}

	require.Equal(t, "уточнение", lastUserMessageText(history))
}

func TestLastUserMessageText_EmptyHistory(t *testing.T) {
	require.Equal(t, "", lastUserMessageText(nil))
}

func TestPromptVersionHash_DeterministicAndSensitiveToContent(t *testing.T) {
	h1 := promptVersionHash("prompt A")
	h2 := promptVersionHash("prompt A")
	h3 := promptVersionHash("prompt B")

	require.Equal(t, h1, h2)
	require.NotEqual(t, h1, h3)
	require.False(t, strings.Contains(h1, " "))
}

func TestQueryTextHash_DoesNotLeakRawText(t *testing.T) {
	hash := queryTextHash("персональные данные пользователя")

	require.NotContains(t, hash, "персональные")
	require.NotEmpty(t, hash)
}
