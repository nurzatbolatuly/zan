package agent

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"zan-backend/internal/domain"
)

func TestGenerateDocument_HappyPath_ReturnsAnswerAndFindings(t *testing.T) {
	llm := jsonLLMHandler(t, `{"answer_text":"Договор аренды подготовлен.","sources":[{"ref":"ст. 540 ГК РК","quote":"..."}],"findings":[{"title":"Стороны","body":"Арендодатель и арендатор..."}],"needs_clarification":false}`)
	client := newTestClient(t, llm, fakePromptProvider{text: "document base prompt"})

	result, err := client.GenerateDocument(context.Background(), historyWithQuestion("Составь договор аренды"))

	require.NoError(t, err)
	require.Equal(t, "Договор аренды подготовлен.", result.AnswerText)
	require.Len(t, result.Findings, 1)
	require.Equal(t, "Стороны", result.Findings[0].Title)
	require.Len(t, result.Sources, 1)
}

func TestGenerateDocument_NeedsClarification_ReturnsSentinelError(t *testing.T) {
	llm := jsonLLMHandler(t, `{"answer_text":"Уточните срок аренды","needs_clarification":true}`)
	client := newTestClient(t, llm, fakePromptProvider{text: "base"})

	_, err := client.GenerateDocument(context.Background(), historyWithQuestion("Составь договор аренды"))

	require.Error(t, err)
	require.ErrorIs(t, err, ErrDocumentNeedsClarification)
}

func TestGenerateDocument_ModelRefusal_ReturnsErrModelRefused(t *testing.T) {
	llm := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(openAIResponse{
			Choices: []openAIChoice{{FinishReason: refusalFinishReason}},
		})
	}
	client := newTestClient(t, llm, fakePromptProvider{text: "base"})

	_, err := client.GenerateDocument(context.Background(), historyWithQuestion("Составь договор"))

	require.Error(t, err)
	require.ErrorIs(t, err, ErrModelRefused)
}

// TestGenerateDocument_OutputTruncated_ReturnsErrOutputTruncated — JSON,
// обрезанный по max_completion_tokens, не доходит до парсинга: причина в
// логе — лимит токенов, а не "невалидный JSON".
func TestGenerateDocument_OutputTruncated_ReturnsErrOutputTruncated(t *testing.T) {
	llm := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(openAIResponse{
			Choices: []openAIChoice{{Message: openAIMessage{Role: "assistant", Content: `{"answer_text":"нач`}, FinishReason: truncatedFinishReason}},
		})
	}
	client := newTestClient(t, llm, fakePromptProvider{text: "base"})

	_, err := client.GenerateDocument(context.Background(), historyWithQuestion("Составь договор"))

	require.ErrorIs(t, err, ErrOutputTruncated)
}

func TestGenerateDocument_LLMTransportError_ReturnsError(t *testing.T) {
	llm := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"type":"authentication_error","message":"invalid x-api-key"}}`))
	}
	client := newTestClient(t, llm, fakePromptProvider{text: "base"})

	_, err := client.GenerateDocument(context.Background(), historyWithQuestion("Составь договор"))

	require.Error(t, err)
}

func TestGenerateDocument_PromptProviderFails_ReturnsError(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("LLM must not be called when prompt loading fails")
	}, fakePromptProvider{err: errors.New("db unavailable")})

	_, err := client.GenerateDocument(context.Background(), historyWithQuestion("Составь договор"))

	require.Error(t, err)
}

func TestGenerateDocument_UsesDocumentPromptType(t *testing.T) {
	var seenAgentType domain.AgentType
	prompts := fakePromptProviderFunc(func(_ context.Context, agentType domain.AgentType) (string, error) {
		seenAgentType = agentType
		return "document prompt", nil
	})
	llm := jsonLLMHandler(t, `{"answer_text":"ok","needs_clarification":false}`)
	client := newTestClient(t, llm, prompts)

	_, err := client.GenerateDocument(context.Background(), historyWithQuestion("Составь договор"))

	require.NoError(t, err)
	require.Equal(t, domain.AgentTypeDocument, seenAgentType)
}

type fakePromptProviderFunc func(context.Context, domain.AgentType) (string, error)

func (f fakePromptProviderFunc) GetPromptText(ctx context.Context, agentType domain.AgentType) (string, error) {
	return f(ctx, agentType)
}
