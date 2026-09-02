import type { RequestStatus } from '../requests/request.types.js';

export interface AnalyticsSummary {
  requestsByStatus: { status: RequestStatus; count: number }[];
  /** null, если ни один запрос ещё не завершился успешно. */
  avgProcessingMs: number | null;
}
