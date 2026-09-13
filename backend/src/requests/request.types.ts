export type RequestStatus =
  'pending' | 'processing' | 'needs_clarification' | 'completed' | 'failed' | 'cancelled';
export type RequestStepStatus = 'pending' | 'running' | 'success' | 'failed';
/**
 * До Этапа 15 (2026-09-12, см. INSTRUCTIONS.md §9.10) здесь было 4 значения — 'search' |
 * 'verification' | 'editor' | 'document': поиск, независимая перепроверка и подготовка ответа
 * были отдельными LLM-вызовами. Слиты в один — 'answer'. Исторические request_steps старых
 * запросов, обработанных до этого дня, могут иметь в БД старые значения ('search' и т.д.) — это
 * просто TEXT-колонка (см. ingestion/migrations/1787130547000_requests.cjs), без БД-миграции;
 * такие записи не проходят через этот union-тип заново, только читаются как есть.
 */
export type AgentName = 'answer' | 'document';

export interface RequestRecord {
  id: string;
  status: RequestStatus;
  queryText: string;
  includeDocument: boolean;
  documentType: string | null;
  resultSummary: string | null;
  errorMessage: string | null;
  /** Один раунд уточнения перед Агентом 1 (см. orchestrator/clarification-check.ts) — не диалог. */
  clarificationQuestion: string | null;
  clarificationAnswer: string | null;
  createdAt: Date;
  updatedAt: Date;
}

/** Облегчённая карточка запроса для истории обращений (личный кабинет, GET /requests). */
export interface RequestSummary {
  id: string;
  status: RequestStatus;
  queryText: string;
  includeDocument: boolean;
  createdAt: Date;
  /** Длительность работы пайплайна (см. common/step-duration.ts) — null, пока не завершился ни
   *  один шаг (запрос ещё в очереди/обрабатывается или упал на самом первом шаге). */
  processingMs: number | null;
}

export interface RequestStepRecord {
  id: string;
  requestId: string;
  agentName: AgentName;
  ordinal: number;
  status: RequestStepStatus;
  input: unknown;
  output: unknown;
  errorMessage: string | null;
  startedAt: Date | null;
  finishedAt: Date | null;
}

export interface DocumentRecord {
  id: string;
  requestId: string;
  documentType: string;
  title: string;
  fileFormat: string;
  content: string;
  createdAt: Date;
}

export interface CreateRequestInput {
  queryText: string;
  includeDocument: boolean;
  documentType: string | null;
  /** SHA-256 хэш анонимного токена сессии владельца (см. common/session.ts), не сам токен. */
  ownerTokenHash: string | null;
}

export interface RequestWithDetails extends RequestRecord {
  steps: RequestStepRecord[];
  document: DocumentRecord | null;
}
