/**
 * Контракты входа/выхода каждого агента. Оркестратор (../orchestrator/orchestrator.service.ts)
 * зависит только от этих интерфейсов и DI-токенов ниже — конкретная реализация подставляется
 * через AgentsModule без изменений в оркестраторе (Open/Closed, Dependency Inversion).
 *
 * Этап 15 (2026-09-12, см. INSTRUCTIONS.md §9.10): Агенты 1–3 (search/verification/editor) слиты
 * в одного — Агента "answer" — один вызов LLM с `web_search` вместо трёх последовательных.
 * Причина: пользователь видел поток "думает…" бо́льшую часть времени ответа, а промежуточный
 * стриминг Агента 1 (эксперимент того же дня, откачен) сбивал с толку — на экране появлялся
 * черновик, который потом менялся Агентом 2 и переписывался Агентом 3. Одному агенту нечего
 * менять после себя — то, что он стримит, и есть финальный ответ.
 *
 * Читает действующее законодательство РК ЖИВЬЁМ, через LLM со встроенным `web_search`,
 * ограниченным adilet.zan.kz/zan.gov.kz — на каждый запрос пользователя, без локального
 * индекса/корпуса. Офлайн-корпус принципиально не рассматривается (см. §9.6/9.8) — доступа к
 * adilet.zan.kz нет ни у разработчика, ни у прода, единственный канал к содержимому закона — сам
 * агент через web_search OpenAI.
 */

export interface AnswerAgentInput {
  queryText: string;
  /** Только для наблюдаемости (см. ChatCompletionRequest.requestId в chatClient.ts) — привязывает
   *  сырой вызов OpenAI к этому запросу в логах, независимо от Prometheus-метрик. */
  requestId: string;
  /**
   * Колбэк на каждый кусок текста ответа по мере генерации (см.
   * ChatClient.completeWithWebSearchMetaStream) — то, что приходит через него, это уже финальный
   * ответ пользователю (после этого агента в пайплайне ответ больше никто не переписывает, см.
   * orchestrator.service.ts), поэтому стримить его в реальном времени честно, в отличие от
   * промежуточного черновика в прежней архитектуре. Необязателен — тесты и вызовы не через
   * оркестратор просто не передают его.
   */
  onToken?: (delta: string) => void;
}
export interface AnswerAgentOutput {
  /** Финальный ответ пользователю — по существу, с самопроверенными ссылками на нормы, в тёплом
   *  человеческом тоне (см. SYSTEM_PROMPT в law-answer.agent.ts). */
  answer: string;
  /**
   * Responses API response id этого вызова (см. WebSearchResult в chatClient.ts) — передаётся
   * Агенту 4 (document), если тому нужен web_search, чтобы продолжить тот же диалог вместо
   * нового поиска с нуля (см. orchestrator.service.ts).
   */
  responseId: string;
  /** Реальные URL, которые агент открыл через web_search. */
  citedUrls: string[];
}
export interface AnswerAgent {
  run(input: AnswerAgentInput): Promise<AnswerAgentOutput>;
}
export const ANSWER_AGENT = Symbol('ANSWER_AGENT');

export interface DocumentAgentInput {
  queryText: string;
  draftAnswer: string;
  documentType: string;
  /** Только для наблюдаемости — см. AnswerAgentInput.requestId. */
  requestId: string;
  /**
   * previousResponseId от Агента "answer" — используется только когда документу реально нужен
   * web_search (см. mayHaveOfficialForm в law-document.agent.ts), чтобы продолжить ту же цепочку
   * вместо нового поиска с нуля.
   */
  previousResponseId: string;
}
export interface DocumentAgentOutput {
  title: string;
  fileFormat: string;
  content: string;
}
export interface DocumentAgent {
  run(input: DocumentAgentInput): Promise<DocumentAgentOutput>;
}
export const DOCUMENT_AGENT = Symbol('DOCUMENT_AGENT');
