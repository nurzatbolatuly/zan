/**
 * Единственные домены, которые агентам разрешено открывать через веб-инструмент
 * (`ChatCompletionRequest.webSearch`, см. `chatClient.ts`) — enforced на стороне OpenAI через
 * `filters.allowed_domains`, не только текстовой инструкцией в промпте. Общий источник для всех
 * агентов, которым дают этот инструмент (Агент 1 — search, Агент 2 — verification), чтобы список
 * доменов не разъезжался между промптами по мере миграции остальных этапов.
 */
export const ALLOWED_LAW_DOMAINS = ['adilet.zan.kz', 'zan.gov.kz'];
