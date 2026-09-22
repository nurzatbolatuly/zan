// Package agent — сборка промпта и HTTP-клиент к LLM-провайдеру,
// имплементит thread.Agent (BACKEND_CODING_STANDARDS.md §1.1). Ретраи и
// circuit breaker — через internal/platform/resilience, тот же паттерн,
// что у internal/grpcclient (BACKEND_PLAN.md §1.1).
package agent

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"
	"strings"

	"zan-backend/internal/domain"
	"zan-backend/internal/grpcclient"
)

// jsonContractInstructions — техническая часть системного промпта
// (формат структурированного ответа, zan-backend-tz-v2.md §4.6). Намеренно
// НЕ хранится в core.agent_prompts и не редактируется через
// /admin/prompts — редактирование текста промпта через админку не должно
// иметь возможности случайно сломать парсинг ответа (response.go
// рассчитывает ровно на эту форму).
const jsonContractInstructions = `Ответь СТРОГО одним JSON-объектом без markdown-разметки (без ` + "```" + `) и без текста вне JSON, ровно с полями:
{
  "answer_text": string — текст ответа пользователю (или уточняющий вопрос, если needs_clarification=true),
  "sources": [{"ref": string, "quote": string}] — статьи закона, процитированные в ответе; пустой список, если ни одной не процитировано,
  "findings": [{"title": string, "body": string}] — структурированные пункты анализа, если уместно; иначе пустой список,
  "needs_clarification": boolean — true, если для содержательного ответа не хватает информации и вместо ответа нужно задать один уточняющий вопрос
}
Никогда не указывай в "sources" статью, которой нет в разделе "Источники" ниже — если нужной статьи там нет, явно скажи в answer_text, что не можешь сослаться на конкретную норму, не придумывай номер статьи.`

// buildSystemPrompt — [промпт из БД] + [технический контракт JSON-ответа] +
// [RAG-источники, единственные, которые LLM разрешено процитировать].
// matches=nil (RAG недоступен/ничего не нашёл) — явно сообщается модели,
// не молчаливое отсутствие раздела, чтобы модель не считала, что источники
// просто не были нужны.
func buildSystemPrompt(basePromptText string, matches []grpcclient.RagMatch) string {
	var b strings.Builder
	b.WriteString(strings.TrimSpace(basePromptText))
	b.WriteString("\n\n")
	b.WriteString(jsonContractInstructions)
	b.WriteString("\n\n")

	if len(matches) == 0 {
		b.WriteString("Источники: по этому запросу не найдено ни одной подходящей статьи закона в базе — отвечай на основе общих знаний, но не приписывай ответу конкретную статью.")
	} else {
		b.WriteString("Источники (единственные статьи, которые можно процитировать в \"sources\"):\n")
		for _, m := range matches {
			fmt.Fprintf(&b, "- %s: %s\n", m.Ref, m.Quote)
		}
	}
	return b.String()
}

// buildMessages — история треда (zan-backend-tz-v2.md §4.6: "в контекст —
// вся история сообщений треда") в формате Anthropic Messages API. Messages
// API требует строгого чередования user/assistant начиная с user —
// гарантируется статус-машиной thread.Service (агент вызывается синхронно
// сразу после каждого сообщения пользователя, до следующего сообщения),
// эта функция полагается на инвариант, а не проверяет его сама.
func buildMessages(history []domain.Message) []anthropicMessage {
	messages := make([]anthropicMessage, 0, len(history))
	for _, m := range history {
		role := "user"
		if m.Sender == domain.MessageSenderAssistant {
			role = "assistant"
		}
		text := m.Text
		if text == "" {
			// input_type=file без текста (Stage 4) — Messages API не
			// принимает пустой content-блок.
			text = "(сообщение без текста, только вложение)"
		}
		messages = append(messages, anthropicMessage{Role: role, Content: text})
	}
	return messages
}

// lastUserMessageText — текст RAG-запроса (backend-roadmap.md §1.4 шаг 2:
// "текст вопроса"). Намеренно только последнее сообщение пользователя, не
// склейка всей истории — вся история и так уходит в промпт LLM отдельным
// путём (buildMessages), сюда нужен только текст для эмбеддинга поискового
// запроса.
func lastUserMessageText(history []domain.Message) string {
	for _, m := range slices.Backward(history) {
		if m.Sender == domain.MessageSenderUser {
			return strings.TrimSpace(m.Text)
		}
	}
	return ""
}

// promptVersionHash — значение context.prompt_version в логе llm_call_started
// (backend-roadmap.md §6.2 п.8: "хэш промпта") — короткий, стабильный
// отпечаток фактического системного промпта (включая RAG-контекст этого
// конкретного вызова), не сам текст (§4.4: полный текст на INFO+ не
// логируется).
func promptVersionHash(systemPrompt string) string {
	sum := sha256.Sum256([]byte(systemPrompt))
	return hex.EncodeToString(sum[:])[:12]
}

// queryTextHash — context.query_text_hash в логе rag_search_started/completed
// (backend-roadmap.md §6.2 п.7) — тот же принцип, что promptVersionHash:
// текст вопроса не логируется на INFO+ как есть.
func queryTextHash(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])[:12]
}
