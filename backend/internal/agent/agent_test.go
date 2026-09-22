package agent

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"zan-backend/internal/domain"
	"zan-backend/internal/grpcclient"
	"zan-backend/internal/platform/resilience"
	"zan-backend/internal/service/thread"
)

type fakePromptProvider struct {
	text string
	err  error
}

func (p fakePromptProvider) GetPromptText(context.Context, domain.AgentType) (string, error) {
	return p.text, p.err
}

type fakeRagSearcher struct {
	matches []grpcclient.RagMatch
	err     error
}

func (s fakeRagSearcher) SearchSources(context.Context, string, int) ([]grpcclient.RagMatch, error) {
	return s.matches, s.err
}

func newTestClient(t *testing.T, llmHandler http.HandlerFunc, prompts PromptProvider, rag RagSearcher) *Client {
	t.Helper()
	server := httptest.NewServer(llmHandler)
	t.Cleanup(server.Close)

	return NewClient(prompts, rag, Config{
		BaseURL:     server.URL,
		APIKey:      "test-key",
		Model:       "gpt-4o",
		MaxTokens:   1024,
		HTTPTimeout: 5 * time.Second,
		RagTopK:     5,
		Breaker:     resilience.BreakerConfig{Name: "test", ConsecutiveFailuresToTrip: 10, OpenTimeout: time.Minute},
	})
}

func jsonLLMHandler(t *testing.T, body string) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(openAIResponse{
			Choices: []openAIChoice{{Message: openAIMessage{Content: body}, FinishReason: "stop"}},
		})
	}
}

func historyWithQuestion(text string) []domain.Message {
	return []domain.Message{{Sender: domain.MessageSenderUser, InputType: domain.MessageInputTypeText, Text: text}}
}

func TestProcess_HappyPath_ReturnsDoneWithVerifiedSources(t *testing.T) {
	llm := jsonLLMHandler(t, `{"answer_text":"Вы можете уволиться, предупредив за месяц.","sources":[{"ref":"ст. 157 ТК РК","quote":"..."}],"findings":[],"needs_clarification":false}`)
	rag := fakeRagSearcher{matches: []grpcclient.RagMatch{{Ref: "ст. 157 ТК РК", Quote: "работник вправе...", Score: 0.9}}}
	client := newTestClient(t, llm, fakePromptProvider{text: "base prompt"}, rag)

	result, err := client.Process(context.Background(), thread.AgentRequest{
		ServiceID: "qa",
		History:   historyWithQuestion("Как мне уволиться?"),
	})

	require.NoError(t, err)
	require.Equal(t, thread.AgentResultDone, result.Status)
	require.Equal(t, "Вы можете уволиться, предупредив за месяц.", result.AnswerText)
	require.Len(t, result.Sources, 1)
	require.False(t, result.UnverifiedSources)
}

func TestProcess_NeedsClarification_ReturnsClarifyStatus(t *testing.T) {
	llm := jsonLLMHandler(t, `{"answer_text":"Уточните, пожалуйста, о каком договоре речь?","needs_clarification":true}`)
	client := newTestClient(t, llm, fakePromptProvider{text: "base"}, fakeRagSearcher{})

	result, err := client.Process(context.Background(), thread.AgentRequest{
		ServiceID: "qa",
		History:   historyWithQuestion("Расторгнуть договор"),
	})

	require.NoError(t, err)
	require.Equal(t, thread.AgentResultClarify, result.Status)
}

func TestProcess_LLMCitesUnknownSource_MarksUnverified(t *testing.T) {
	llm := jsonLLMHandler(t, `{"answer_text":"Ответ","sources":[{"ref":"ст. 999 Придуманного кодекса","quote":"x"}],"needs_clarification":false}`)
	rag := fakeRagSearcher{matches: []grpcclient.RagMatch{{Ref: "ст. 157 ТК РК", Quote: "..."}}}
	client := newTestClient(t, llm, fakePromptProvider{text: "base"}, rag)

	result, err := client.Process(context.Background(), thread.AgentRequest{
		ServiceID: "qa",
		History:   historyWithQuestion("Вопрос"),
	})

	require.NoError(t, err)
	require.True(t, result.UnverifiedSources)
}

func TestProcess_RagUnavailable_DegradesWithoutBlocking(t *testing.T) {
	var seenSystemPrompt string
	llm := func(w http.ResponseWriter, r *http.Request) {
		var req openAIRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		require.NotEmpty(t, req.Messages)
		seenSystemPrompt = req.Messages[0].Content
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(openAIResponse{
			Choices: []openAIChoice{{Message: openAIMessage{Content: `{"answer_text":"Ответ без источников","needs_clarification":false}`}, FinishReason: "stop"}},
		})
	}
	rag := fakeRagSearcher{err: errors.New("helper: connection refused")}
	client := newTestClient(t, llm, fakePromptProvider{text: "base"}, rag)

	result, err := client.Process(context.Background(), thread.AgentRequest{
		ServiceID: "qa",
		History:   historyWithQuestion("Вопрос"),
	})

	require.NoError(t, err)
	require.Equal(t, thread.AgentResultDone, result.Status)
	require.Equal(t, "Ответ без источников", result.AnswerText)
	require.Contains(t, seenSystemPrompt, "не найдено ни одной подходящей статьи")
}

func TestProcess_PromptProviderFails_ReturnsError(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("LLM must not be called when prompt loading fails")
	}, fakePromptProvider{err: errors.New("db unavailable")}, fakeRagSearcher{})

	_, err := client.Process(context.Background(), thread.AgentRequest{
		ServiceID: "qa",
		History:   historyWithQuestion("Вопрос"),
	})

	require.Error(t, err)
}

func TestProcess_LLMTransportError_ReturnsPlainErrorNotAgentResultError(t *testing.T) {
	llm := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"type":"authentication_error","message":"invalid x-api-key"}}`))
	}
	client := newTestClient(t, llm, fakePromptProvider{text: "base"}, fakeRagSearcher{})

	result, err := client.Process(context.Background(), thread.AgentRequest{
		ServiceID: "qa",
		History:   historyWithQuestion("Вопрос"),
	})

	require.Error(t, err)
	require.Empty(t, result.Status, "on transport error, thread.Service reads err, not result.Status")
}

func TestProcess_InvalidJSONFromLLM_ReturnsPlainError(t *testing.T) {
	llm := jsonLLMHandler(t, "извините, но я не могу ответить в требуемом формате")
	client := newTestClient(t, llm, fakePromptProvider{text: "base"}, fakeRagSearcher{})

	_, err := client.Process(context.Background(), thread.AgentRequest{
		ServiceID: "qa",
		History:   historyWithQuestion("Вопрос"),
	})

	require.Error(t, err)
}

func TestProcess_ModelRefusal_ReturnsAgentResultError(t *testing.T) {
	llm := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(openAIResponse{
			Choices: []openAIChoice{{FinishReason: refusalFinishReason}},
		})
	}
	client := newTestClient(t, llm, fakePromptProvider{text: "base"}, fakeRagSearcher{})

	result, err := client.Process(context.Background(), thread.AgentRequest{
		ServiceID: "qa",
		History:   historyWithQuestion("Вопрос"),
	})

	require.NoError(t, err)
	require.Equal(t, thread.AgentResultError, result.Status)
	require.ErrorIs(t, result.Err, ErrModelRefused)
}

func TestProcess_UnexpectedServiceID_ReturnsError(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("LLM must not be called for an unexpected service_id")
	}, fakePromptProvider{text: "base"}, fakeRagSearcher{})

	_, err := client.Process(context.Background(), thread.AgentRequest{
		ServiceID: "doc",
		History:   historyWithQuestion("Вопрос"),
	})

	require.Error(t, err)
}

func TestProcess_EmptyHistory_SkipsRagSearchButStillCallsLLM(t *testing.T) {
	called := false
	rag := fakeRagSearcherFunc(func(context.Context, string, int) ([]grpcclient.RagMatch, error) {
		called = true
		return nil, nil
	})
	llm := jsonLLMHandler(t, `{"answer_text":"Ответ","needs_clarification":false}`)
	client := newTestClient(t, llm, fakePromptProvider{text: "base"}, rag)

	_, err := client.Process(context.Background(), thread.AgentRequest{ServiceID: "qa", History: nil})

	require.NoError(t, err)
	require.False(t, called, "no user message in history -> nothing to search for")
}

type fakeRagSearcherFunc func(context.Context, string, int) ([]grpcclient.RagMatch, error)

func (f fakeRagSearcherFunc) SearchSources(ctx context.Context, q string, k int) ([]grpcclient.RagMatch, error) {
	return f(ctx, q, k)
}
