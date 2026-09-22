package agent

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseAgentResponse_ValidJSON(t *testing.T) {
	resp := anthropicResponse{Content: []anthropicContentBlock{
		{Type: "text", Text: `{"answer_text":"Ответ","sources":[{"ref":"ст. 1","quote":"текст"}],"findings":[{"title":"T","body":"B"}],"needs_clarification":false}`},
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
	resp := anthropicResponse{Content: []anthropicContentBlock{
		{Type: "text", Text: "```json\n{\"answer_text\":\"Ответ\",\"needs_clarification\":false}\n```"},
	}}

	parsed, err := parseAgentResponse(resp)

	require.NoError(t, err)
	require.Equal(t, "Ответ", parsed.AnswerText)
}

func TestParseAgentResponse_ConcatenatesMultipleTextBlocks(t *testing.T) {
	resp := anthropicResponse{Content: []anthropicContentBlock{
		{Type: "text", Text: `{"answer_text":`},
		{Type: "text", Text: `"Ответ","needs_clarification":true}`},
	}}

	parsed, err := parseAgentResponse(resp)

	require.NoError(t, err)
	require.Equal(t, "Ответ", parsed.AnswerText)
	require.True(t, parsed.NeedsClarification)
}

func TestParseAgentResponse_IgnoresNonTextBlocks(t *testing.T) {
	resp := anthropicResponse{Content: []anthropicContentBlock{
		{Type: "thinking", Text: `{"answer_text":"should not be used"}`},
		{Type: "text", Text: `{"answer_text":"Ответ","needs_clarification":false}`},
	}}

	parsed, err := parseAgentResponse(resp)

	require.NoError(t, err)
	require.Equal(t, "Ответ", parsed.AnswerText)
}

func TestParseAgentResponse_InvalidJSON(t *testing.T) {
	resp := anthropicResponse{Content: []anthropicContentBlock{
		{Type: "text", Text: "к сожалению, я не могу ответить на этот вопрос в формате JSON"},
	}}

	_, err := parseAgentResponse(resp)

	require.Error(t, err)
}

func TestParseAgentResponse_EmptyAnswerText(t *testing.T) {
	resp := anthropicResponse{Content: []anthropicContentBlock{
		{Type: "text", Text: `{"answer_text":"","needs_clarification":false}`},
	}}

	_, err := parseAgentResponse(resp)

	require.ErrorIs(t, err, ErrEmptyAnswer)
}
