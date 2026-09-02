/**
 * Формы данных, как они реально приходят по HTTP от backend (@zan/backend), а не копия его
 * внутренних типов: даты сериализуются в JSON как строки (не Date), а поля вроде
 * RequestStepRecord.input/output backend типизирует как `unknown` и фронтенд их не использует
 * (статус шага рендерится по agentName/status, без интроспекции содержимого).
 */

export type RequestStatus =
  'pending' | 'processing' | 'needs_clarification' | 'completed' | 'failed' | 'cancelled';
export type RequestStepStatus = 'pending' | 'running' | 'success' | 'failed';
export type AgentName = 'search' | 'verification' | 'editor' | 'document';

export interface RequestStepRecord {
  id: string;
  agentName: AgentName;
  status: RequestStepStatus;
  errorMessage: string | null;
}

export interface DocumentRecord {
  id: string;
  documentType: string;
  title: string;
  fileFormat: string;
  /** Содержимое файла в base64 (см. backend/src/agents/document/law-document.agent.ts). */
  content: string;
}

export interface RequestDetails {
  id: string;
  status: RequestStatus;
  queryText: string;
  includeDocument: boolean;
  documentType: string | null;
  resultSummary: string | null;
  errorMessage: string | null;
  /** Один раунд уточнения перед поиском (Этап 13) — не диалог. */
  clarificationQuestion: string | null;
  clarificationAnswer: string | null;
  steps: RequestStepRecord[];
  document: DocumentRecord | null;
  createdAt: string;
  updatedAt: string;
}

/** Карточка запроса для истории обращений (Этап 13 — личный кабинет, GET /requests). */
export interface RequestSummary {
  id: string;
  status: RequestStatus;
  queryText: string;
  includeDocument: boolean;
  createdAt: string;
  /** Длительность работы пайплайна в мс — null, пока не завершился ни один шаг. */
  processingMs: number | null;
}

/** Аналитика по частым темам обращений (Этап 13, GET /analytics/topics). */
export interface AnalyticsSummary {
  requestsByStatus: { status: RequestStatus; count: number }[];
  avgProcessingMs: number | null;
}
