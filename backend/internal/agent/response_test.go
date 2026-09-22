package agent

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseAgentResponse_ValidJSON(t *testing.T) {
	resp := openAIResponse{Choices: []openAIChoice{
		{Message: openAIMessage{Content: `{"answer_text":"Ответ","sources":[{"ref":"ст. 1","quote":"текст"}],"findings":[{"title":"T","body":"B"}],"needs_clarification":false}`}},
	}}

	parsed, err := parseAgentResponse(resp)

	require.NoError(t, err)
	require.Equal(t, "Ответ", parsed.AnswerText)
	require.Equal(t, "ст. 1", parsed.Sources[0].Ref)
	require.Equal(t, "текст", parsed.Sources[0].Quote)
	require.Equal(t, "T", parsed.Findings[0].Title)
	require.False(t, parsed.NeedsClarification)
}

func TestParseAgentResponse_StripsMarkdownFence(t *testing.T) {
	resp := openAIResponse{Choices: []openAIChoice{
		{Message: openAIMessage{Content: "```json\n{\"answer_text\":\"Ответ\",\"needs_clarification\":false}\n```"}},
	}}

	parsed, err := parseAgentResponse(resp)

	require.NoError(t, err)
	require.Equal(t, "Ответ", parsed.AnswerText)
}

func TestParseAgentResponse_InvalidJSON(t *testing.T) {
	resp := openAIResponse{Choices: []openAIChoice{
		{Message: openAIMessage{Content: "к сожалению, я не могу ответить на этот вопрос в формате JSON"}},
	}}

	_, err := parseAgentResponse(resp)

	require.Error(t, err)
}

func TestParseAgentResponse_EmptyAnswerText(t *testing.T) {
	resp := openAIResponse{Choices: []openAIChoice{
		{Message: openAIMessage{Content: `{"answer_text":"","needs_clarification":false}`}},
	}}

	_, err := parseAgentResponse(resp)

	require.ErrorIs(t, err, ErrEmptyAnswer)
}

func TestParseAgentResponse_NoChoices(t *testing.T) {
	_, err := parseAgentResponse(openAIResponse{})

	require.Error(t, err)
}
