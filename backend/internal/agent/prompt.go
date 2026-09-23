// Package agent — сборка промпта и HTTP-клиент к LLM-провайдеру,
// имплементит thread.Agent и document.Generator (BACKEND_CODING_STANDARDS.md
// §1.1). Ретраи и circuit breaker — через internal/platform/resilience, тот
// же паттерн, что у internal/grpcclient.
package agent

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"zan-backend/internal/domain"
)

// qaAnswerInstructions — техническая часть системного промпта Q&A: ответ
// стримится пользователю токен за токеном, поэтому это обычный читаемый
// текст, не JSON (частичный JSON посреди генерации не читаем как текст).
// Как и jsonContractInstructions, намеренно НЕ хранится в
// core.agent_prompts — правка текста промпта через /admin/prompts не
// должна иметь возможности сломать формат ответа.
const qaAnswerInstructions = `Ответь пользователю обычным текстом — без markdown-разметки, без вступлений вида "Конечно, вот ответ", сразу по существу. Если для содержательного ответа не хватает информации — задай один уточняющий вопрос вместо ответа.`

// jsonContractInstructions — техническая часть системного промпта
// генерации документа (формат структурированного ответа). Намеренно НЕ
// хранится в core.agent_prompts и не редактируется через /admin/prompts —
// редактирование текста промпта через админку не должно иметь возможности
// случайно сломать парсинг ответа (response.go рассчитывает ровно на эту
// форму).
const jsonContractInstructions = `Ответь СТРОГО одним JSON-объектом без markdown-разметки (без ` + "```" + `) и без текста вне JSON, ровно с полями:
{
  "answer_text": string — текст ответа пользователю (или уточняющий вопрос, если needs_clarification=true),
  "sources": [{"ref": string, "quote": string}] — статьи закона, на которые опирается ответ; пустой список, если ни одной не процитировано,
  "findings": [{"title": string, "body": string}] — структурированные пункты анализа, если уместно; иначе пустой список,
  "needs_clarification": boolean — true, если для содержательного ответа не хватает информации и вместо ответа нужно задать один уточняющий вопрос
}
Указывай в "sources" только нормы законодательства Республики Казахстан, в существовании и номере которых ты уверен — если не уверен, не указывай статью вовсе, не придумывай номер.`

// buildQASystemPrompt — [промпт из БД] + [формат ответа Q&A].
func buildQASystemPrompt(basePromptText string) string {
	return strings.TrimSpace(basePromptText) + "\n\n" + qaAnswerInstructions
}

// buildStructuredSystemPrompt — [промпт из БД] + [технический контракт
// JSON-ответа].
func buildStructuredSystemPrompt(basePromptText string) string {
	return strings.TrimSpace(basePromptText) + "\n\n" + jsonContractInstructions
}

// buildMessages — история треда (вся переписка, последнее сообщение —
// текущий вопрос пользователя) в формате OpenAI Chat Completions API
// (system-сообщение добавляется отдельно, llmClient). Порядок — как есть:
// агент вызывается сразу после каждого сообщения пользователя, до
// следующего (инвариант статус-машины thread.Service). Вложения идут в то
// же сообщение после текста: из inline (file ID → часть content) — файлом
// целиком, остальные — извлечённым текстом (attachmentBlock).
func buildMessages(history []domain.Message, inline map[string]openAIContentPart) []openAIMessage {
	messages := make([]openAIMessage, 0, len(history))
	for _, m := range history {
		role := "user"
		if m.Sender == domain.MessageSenderAssistant {
			role = "assistant"
		}
		messages = append(messages, buildMessage(role, m, inline))
	}
	return messages
}

func buildMessage(role string, m domain.Message, inline map[string]openAIContentPart) openAIMessage {
	msg := openAIMessage{Role: role}
	texts := make([]string, 0, 1+len(m.Attachments))
	if text := strings.TrimSpace(m.Text); text != "" {
		texts = append(texts, text)
	}
	for _, f := range m.Attachments {
		if part, ok := inline[f.ID]; ok {
			texts = append(texts, fmt.Sprintf("[Вложение «%s» — файл приложен к сообщению]", f.OriginalName))
			msg.Files = append(msg.Files, part)
			continue
		}
		texts = append(texts, attachmentBlock(f))
	}
	if len(texts) == 0 {
		// input_type=file без текста, вложение не найдено — не отправляем пустой content.
		texts = append(texts, "(сообщение без текста, только вложение)")
	}
	msg.Content = strings.Join(texts, "\n\n")
	return msg
}

// promptVersionHash — значение context.prompt_version в логе
// llm_call_started — короткий, стабильный отпечаток фактического
// системного промпта, не сам текст (полный текст на INFO+ не логируется,
// instructions.md §3.4).
func promptVersionHash(systemPrompt string) string {
	sum := sha256.Sum256([]byte(systemPrompt))
	return hex.EncodeToString(sum[:])[:12]
}
