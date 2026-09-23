package agent

import "errors"

// ErrModelRefused — LLM отказалась отвечать / контент заблокирован
// модерацией провайдера (без показа формулировки отказа модели
// пользователю) — бизнес-причина отказа для лога (thread.AgentResult.Err),
// не транспортная ошибка.
var ErrModelRefused = errors.New("agent: model refused to answer")

// refusalFinishReason — OpenAI Chat Completions API возвращает
// finish_reason="content_filter", когда ответ заблокирован модерацией
// провайдера. Обычный текстовый отказ внутри content сюда не попадает: в
// Q&A он просто становится текстом ответа, в генерации документа — не
// пройдёт парсинг JSON-контракта (путь "невалидный JSON").
const refusalFinishReason = "content_filter"

// ErrOutputTruncated — модель упёрлась в max_completion_tokens
// (finish_reason="length") раньше, чем закончила ответ. У reasoning-моделей
// (gpt-5, o-серия) лимит общий на скрытые рассуждения и видимый текст —
// при слишком малом OPENAI_MAX_TOKENS весь бюджет уходит на reasoning и
// текст ответа пуст. Обрезанный ответ пользователю не засчитывается
// (status=error, кредит возвращается) — тот же исход, что пустой ответ, но
// отдельная причина в логе: лечится конфигурацией (OPENAI_MAX_TOKENS/
// OPENAI_REASONING_EFFORT), не ретраем.
var ErrOutputTruncated = errors.New("agent: model output truncated by max_completion_tokens")

// truncatedFinishReason — finish_reason OpenAI Chat Completions API при
// исчерпании max_completion_tokens.
const truncatedFinishReason = "length"
