package agent

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"zan-backend/internal/domain"
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

// fakeFileLoader — хранилище вложений в памяти, ключ — object key.
type fakeFileLoader map[string][]byte

func (f fakeFileLoader) Get(_ context.Context, key string) ([]byte, error) {
	data, ok := f[key]
	if !ok {
		return nil, errors.New("object not found")
	}
	return data, nil
}

func newTestClient(t *testing.T, llmHandler http.HandlerFunc, prompts PromptProvider) *Client {
	t.Helper()
	return newTestClientWithFiles(t, llmHandler, prompts, fakeFileLoader{})
}

func newTestClientWithFiles(t *testing.T, llmHandler http.HandlerFunc, prompts PromptProvider, files FileLoader) *Client {
	t.Helper()
	server := httptest.NewServer(llmHandler)
	t.Cleanup(server.Close)

	return NewClient(prompts, files, Config{
		BaseURL:           server.URL,
		APIKey:            "test-key",
		Model:             "gpt-4o",
		MaxTokens:         1024,
		HTTPTimeout:       5 * time.Second,
		StreamHTTPTimeout: 5 * time.Second,
		Breaker:           resilience.BreakerConfig{Name: "test", ConsecutiveFailuresToTrip: 10, OpenTimeout: time.Minute},
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

// isStreamRequest — различает streaming-вызов (Q&A, `"stream":true`, без
// response_format) и не-streaming (генерация документа, JSON-режим) по
// телу запроса, которое реально шлёт llmClient.
func isStreamRequest(t *testing.T, r *http.Request) (bool, []byte) {
	t.Helper()
	body, err := io.ReadAll(r.Body)
	require.NoError(t, err)
	var probe struct {
		Stream bool `json:"stream"`
	}
	require.NoError(t, json.Unmarshal(body, &probe))
	return probe.Stream, body
}

func writeSSEChunk(w http.ResponseWriter, content, finishReason string) {
	_, _ = fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":%q},\"finish_reason\":%q}]}\n\n", content, finishReason)
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}

func writeSSEDone(w http.ResponseWriter) {
	_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}

// streamLLMHandler — fake OpenAI для Process: отвечает SSE-потоком из
// chunks (по одному чанку на элемент) и требует, чтобы запрос был
// streaming — Q&A не должен ходить в LLM не-streaming вызовом.
func streamLLMHandler(t *testing.T, chunks ...string) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		streaming, _ := isStreamRequest(t, r)
		require.True(t, streaming, "Q&A must call LLM in streaming mode")
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		for _, c := range chunks {
			writeSSEChunk(w, c, "")
		}
		writeSSEDone(w)
	}
}

func historyWithQuestion(text string) []domain.Message {
	return []domain.Message{{Sender: domain.MessageSenderUser, InputType: domain.MessageInputTypeText, Text: text}}
}

// TestProcess_StreamsAnswerInSingleLLMCall — ключевой контракт Q&A: ровно
// один вызов LLM, дельты уходят в OnDelta в порядке генерации и до
// возврата Process, финальный текст — их конкатенация.
func TestProcess_StreamsAnswerInSingleLLMCall(t *testing.T) {
	var requests atomic.Int32
	stream := streamLLMHandler(t, "Вы можете уволиться, ", "предупредив за месяц.")
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		stream(w, r)
	}, fakePromptProvider{text: "base prompt"})

	var deltas []string
	result, err := client.Process(context.Background(), thread.AgentRequest{
		ServiceID: "qa",
		History:   historyWithQuestion("Как мне уволиться?"),
		OnDelta:   func(d string) { deltas = append(deltas, d) },
	})

	require.NoError(t, err)
	require.Equal(t, thread.AgentResultDone, result.Status)
	require.Equal(t, "Вы можете уволиться, предупредив за месяц.", result.AnswerText)
	require.Equal(t, []string{"Вы можете уволиться, ", "предупредив за месяц."}, deltas)
	require.Equal(t, int32(1), requests.Load())
}

// TestProcess_SendsPromptAndUserQuestion — системный промпт = промпт
// AgentPrompt(qa) из БД + формат ответа, за ним история треда с вопросом
// пользователя последним сообщением.
func TestProcess_SendsPromptAndUserQuestion(t *testing.T) {
	var seen openAIStreamRequest
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, body := isStreamRequest(t, r)
		require.NoError(t, json.Unmarshal(body, &seen))
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		writeSSEChunk(w, "Ответ", "")
		writeSSEDone(w)
	}, fakePromptProvider{text: "Ты — юридический ассистент."})

	history := []domain.Message{
		{Sender: domain.MessageSenderUser, Text: "первый вопрос"},
		{Sender: domain.MessageSenderAssistant, Text: "первый ответ"},
		{Sender: domain.MessageSenderUser, Text: "уточнение"},
	}
	_, err := client.Process(context.Background(), thread.AgentRequest{ServiceID: "qa", History: history})

	require.NoError(t, err)
	require.Len(t, seen.Messages, 4)
	require.Equal(t, "system", seen.Messages[0].Role)
	require.Contains(t, seen.Messages[0].Content, "Ты — юридический ассистент.")
	require.Contains(t, seen.Messages[0].Content, qaAnswerInstructions)
	require.Equal(t, openAIMessage{Role: "user", Content: "уточнение"}, seen.Messages[3])
}

func TestProcess_PromptProviderFails_ReturnsError(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("LLM must not be called when prompt loading fails")
	}, fakePromptProvider{err: errors.New("db unavailable")})

	_, err := client.Process(context.Background(), thread.AgentRequest{
		ServiceID: "qa",
		History:   historyWithQuestion("Вопрос"),
	})

	require.Error(t, err)
}

func TestProcess_LLMTransportError_ReturnsPlainErrorNotAgentResultError(t *testing.T) {
	llm := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"type":"authentication_error","message":"invalid api key"}}`))
	}
	client := newTestClient(t, llm, fakePromptProvider{text: "base"})

	result, err := client.Process(context.Background(), thread.AgentRequest{
		ServiceID: "qa",
		History:   historyWithQuestion("Вопрос"),
	})

	require.Error(t, err)
	require.Empty(t, result.Status, "on transport error, thread.Service reads err, not result.Status")
}

func TestProcess_EmptyAnswer_ReturnsError(t *testing.T) {
	client := newTestClient(t, streamLLMHandler(t, "   "), fakePromptProvider{text: "base"})

	_, err := client.Process(context.Background(), thread.AgentRequest{
		ServiceID: "qa",
		History:   historyWithQuestion("Вопрос"),
	})

	require.ErrorIs(t, err, ErrEmptyAnswer)
}

// TestProcess_ModelRefusal_ReturnsAgentResultError — первый же SSE-чанк
// несёт finish_reason=content_filter без единого символа контента (до
// начала реального стрима), поэтому ErrModelRefused возвращается НЕ
// обёрнутым в errStreamPartial и становится бизнес-исходом Error.
func TestProcess_ModelRefusal_ReturnsAgentResultError(t *testing.T) {
	llm := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		writeSSEChunk(w, "", refusalFinishReason)
		writeSSEDone(w)
	}
	client := newTestClient(t, llm, fakePromptProvider{text: "base"})

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
	}, fakePromptProvider{text: "base"})

	_, err := client.Process(context.Background(), thread.AgentRequest{
		ServiceID: "doc",
		History:   historyWithQuestion("Вопрос"),
	})

	require.Error(t, err)
}

// contractQuestion — вопрос с PDF-вложением после уже состоявшегося вопроса
// с фото: фото прошлого вопроса должно уйти текстом, PDF текущего — файлом.
func contractQuestion() []domain.Message {
	photoText := "текст фото"
	contractText := "ДОГОВОР АРЕНДЫ"
	return []domain.Message{
		{ID: "m1", Sender: domain.MessageSenderUser, Text: "Что на фото?", Attachments: []domain.FileAttachment{{
			ID: "f-photo", ObjectKey: "uploads/s/f-photo", OriginalName: "photo.jpg", MimeType: "image/jpeg",
			ProcessingStatus: domain.FileProcessingStatusProcessed, ExtractedText: &photoText,
		}}},
		{ID: "m2", Sender: domain.MessageSenderAssistant, Text: "Это расписка."},
		{ID: "m3", Sender: domain.MessageSenderUser, Text: "Проверьте договор", Attachments: []domain.FileAttachment{{
			ID: "f-contract", ObjectKey: "uploads/s/f-contract", OriginalName: "dogovor.pdf", MimeType: "application/pdf",
			ProcessingStatus: domain.FileProcessingStatusProcessed, ExtractedText: &contractText,
		}}},
	}
}

// rawRequestMessages — messages запроса как сырой JSON: content с файлом —
// массив частей, в openAIMessage (Content string) он не декодируется.
func rawRequestMessages(t *testing.T, body []byte) []json.RawMessage {
	t.Helper()
	var req struct {
		Messages []json.RawMessage `json:"messages"`
	}
	require.NoError(t, json.Unmarshal(body, &req))
	return req.Messages
}

func streamOK(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/event-stream")
	writeSSEChunk(w, "Ответ", "stop")
	writeSSEDone(w)
}

func TestProcess_SendsCurrentQuestionPDFInlineAndPastFilesAsText(t *testing.T) {
	var body []byte
	files := fakeFileLoader{"uploads/s/f-contract": []byte("%PDF-1.7"), "uploads/s/f-photo": []byte("jpeg")}
	c := newTestClientWithFiles(t, func(w http.ResponseWriter, r *http.Request) {
		_, body = isStreamRequest(t, r)
		streamOK(w)
	}, fakePromptProvider{text: "Ты — юридический ассистент."}, files)

	result, err := c.Process(context.Background(), thread.AgentRequest{ServiceID: "qa", History: contractQuestion()})
	require.NoError(t, err)
	require.Equal(t, thread.AgentResultDone, result.Status)

	messages := rawRequestMessages(t, body)
	require.Len(t, messages, 4) // system + 3
	require.Contains(t, string(messages[1]), "текст фото", "past question attachment goes as extracted text")
	require.NotContains(t, string(messages[1]), "base64")
	require.Contains(t, string(messages[3]), `"type":"file"`)
	require.Contains(t, string(messages[3]), "data:application/pdf;base64,"+base64.StdEncoding.EncodeToString([]byte("%PDF-1.7")))
	require.NotContains(t, string(messages[3]), "ДОГОВОР АРЕНДЫ", "inline file is not duplicated as text")
}

func TestProcess_InlineFileRejectedRetriesOnExtractedText(t *testing.T) {
	var calls atomic.Int32
	var retryBody []byte
	files := fakeFileLoader{"uploads/s/f-contract": []byte("%PDF-1.7")}
	c := newTestClientWithFiles(t, func(w http.ResponseWriter, r *http.Request) {
		_, body := isStreamRequest(t, r)
		if calls.Add(1) == 1 {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":{"message":"PDF exceeds page limit"}}`))
			return
		}
		retryBody = body
		streamOK(w)
	}, fakePromptProvider{text: "prompt"}, files)

	result, err := c.Process(context.Background(), thread.AgentRequest{ServiceID: "qa", History: contractQuestion()})
	require.NoError(t, err)
	require.Equal(t, thread.AgentResultDone, result.Status)
	require.Equal(t, int32(2), calls.Load())
	require.NotContains(t, string(retryBody), "base64")
	require.Contains(t, string(retryBody), "ДОГОВОР АРЕНДЫ")
}

func TestProcess_InlineFileLoadFailureFallsBackToText(t *testing.T) {
	var body []byte
	c := newTestClientWithFiles(t, func(w http.ResponseWriter, r *http.Request) {
		_, body = isStreamRequest(t, r)
		streamOK(w)
	}, fakePromptProvider{text: "prompt"}, fakeFileLoader{})

	_, err := c.Process(context.Background(), thread.AgentRequest{ServiceID: "qa", History: contractQuestion()})
	require.NoError(t, err)
	require.NotContains(t, string(body), "base64")
	require.Contains(t, string(body), "ДОГОВОР АРЕНДЫ")
}
