package agent

import "errors"

// ErrModelRefused — LLM отказалась отвечать / контент заблокирован
// модерацией провайдера (backend-roadmap.md §5.2: "без показа
// формулировки отказа модели пользователю") — бизнес-причина отказа для
// лога (thread.AgentResult.Err), не транспортная ошибка.
var ErrModelRefused = errors.New("agent: model refused to answer")

// refusalFinishReason — OpenAI Chat Completions API возвращает
// finish_reason="content_filter", когда ответ заблокирован модерацией
// провайдера (отдельно от обычного текстового отказа внутри content,
// который вместо этого не пройдёт парсинг JSON-контракта и уйдёт по пути
// "невалидный JSON" — оба случая ведут к одному и тому же пользовательскому
// исходу, thread.status=error, разница только в context.result лога).
const refusalFinishReason = "content_filter"
