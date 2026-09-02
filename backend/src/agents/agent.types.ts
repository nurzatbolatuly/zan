/**
 * Контракты входа/выхода каждого агента. Оркестратор (../orchestrator/orchestrator.service.ts)
 * зависит только от этих интерфейсов и DI-токенов ниже — конкретная реализация подставляется
 * через AgentsModule без изменений в оркестраторе (Open/Closed, Dependency Inversion).
 *
 * Агент 1 (search) и Агент 2 (verification) читают действующее законодательство РК ЖИВЬЁМ, через
 * LLM со встроенным `web_search`, ограниченным adilet.zan.kz/zan.gov.kz — на каждый запрос
 * пользователя, без локального индекса/корпуса (см. INSTRUCTIONS.md §9: пробовали офлайн-индекс
 * с retrieval — решило латентность, но сам синк корпуса требует доступа к adilet.zan.kz из
 * инфраструктуры, куда он не всегда есть; вернулись к live-поиску и вместо этого убрали цикл
 * переспроса search↔verification, который и был основным вкладом в латентность).
 */

export interface SearchAgentInput {
  queryText: string;
}
export interface SearchAgentOutput {
  /**
   * Результат живого поиска по adilet.zan.kz/zan.gov.kz. Не адресован напрямую пользователю —
   * Агент 2 независимо перепроверяет его на корректность, Агент 3 сокращает до чёткого
   * структурированного ответа.
   */
  draftAnswer: string;
}
export interface SearchAgent {
  run(input: SearchAgentInput): Promise<SearchAgentOutput>;
}
export const SEARCH_AGENT = Symbol('SEARCH_AGENT');

export interface VerificationAgentInput {
  queryText: string;
  draftAnswer: string;
}
export interface VerificationAgentOutput {
  /** Независимо перепроверенная и, при необходимости, исправленная версия draftAnswer. */
  revisedAnswer: string;
  /** Расхождения/замечания. Пустой массив — их нет. */
  concerns: string[];
}
export interface VerificationAgent {
  run(input: VerificationAgentInput): Promise<VerificationAgentOutput>;
}
export const VERIFICATION_AGENT = Symbol('VERIFICATION_AGENT');

export interface EditorAgentInput {
  queryText: string;
  draftAnswer: string;
}
export interface EditorAgentOutput {
  summary: string;
}
export interface EditorAgent {
  run(input: EditorAgentInput): Promise<EditorAgentOutput>;
}
export const EDITOR_AGENT = Symbol('EDITOR_AGENT');

export interface DocumentAgentInput {
  queryText: string;
  draftAnswer: string;
  documentType: string;
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
