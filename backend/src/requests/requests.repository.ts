import { Inject, Injectable } from '@nestjs/common';
import type { SupabaseClient } from '@supabase/supabase-js';
import { SUPABASE_ADMIN } from '../db/db.constants.js';
import { computeStepsDurationMs, type StepTimestamps } from '../common/step-duration.js';
import type {
  AgentName,
  CreateRequestInput,
  DocumentRecord,
  RequestRecord,
  RequestStatus,
  RequestStepRecord,
  RequestStepStatus,
  RequestSummary,
  RequestWithDetails,
} from './request.types.js';

export const REQUESTS_REPOSITORY = Symbol('REQUESTS_REPOSITORY');

export interface RequestsRepository {
  createRequest(input: CreateRequestInput): Promise<RequestRecord>;
  findRequestWithDetails(id: string): Promise<RequestWithDetails | null>;
  /**
   * true, если запись существует и owner_token_hash совпадает с переданным хэшем (оба null —
   * тоже совпадение: запрос без владельца доступен только тому, кто тоже пришёл без cookie
   * сессии). Не раскрывает сам хэш наружу — только результат сравнения, чтобы owner_token_hash
   * никогда не покидал репозиторий.
   */
  isOwnedBy(id: string, ownerTokenHash: string | null): Promise<boolean>;
  /** История обращений (личный кабинет) — только запросы текущей анонимной сессии, новые сверху. */
  listByOwner(ownerTokenHash: string, limit: number): Promise<RequestSummary[]>;
  updateRequestStatus(
    id: string,
    status: RequestStatus,
    patch?: { resultSummary?: string; errorMessage?: string },
  ): Promise<void>;
  /** Лёгкая проверка статуса (без steps/document) — используется оркестратором между шагами
   *  пайплайна, чтобы вовремя заметить отмену (см. orchestrator.service.ts). */
  getStatus(id: string): Promise<RequestStatus | null>;
  /**
   * Атомарно переводит запрос в 'cancelled', только если он ещё активен (pending/processing/
   * needs_clarification) — условие в самом UPDATE, а не отдельным SELECT+UPDATE, чтобы не
   * потерять гонку с оркестратором. Возвращает false, если запрос уже в терминальном статусе.
   */
  cancelIfActive(id: string): Promise<boolean>;
  setClarificationQuestion(id: string, question: string): Promise<void>;
  setClarificationAnswer(id: string, answer: string): Promise<void>;
  /**
   * Этап 18: помечает уже завершённый запрос как "нужен документ" (см.
   * RequestsService.requestDocument) — includeDocument/documentType раньше выставлялись только
   * при создании запроса (см. CreateRequestDto), теперь также задним числом, когда пользователь
   * решает приложить документ уже после того, как увидел ответ. Статус самого запроса намеренно
   * не трогаем (остаётся 'completed') — см. OrchestratorService.processDocumentOnlyResume.
   */
  setDocumentRequested(id: string, documentType: string): Promise<void>;
  createStep(requestId: string, agentName: AgentName, ordinal: number): Promise<RequestStepRecord>;
  startStep(stepId: string, input: unknown): Promise<void>;
  finishStep(
    stepId: string,
    status: RequestStepStatus,
    patch: { output?: unknown; errorMessage?: string },
  ): Promise<void>;
  createDocument(
    requestId: string,
    documentType: string,
    title: string,
    fileFormat: string,
    content: string,
  ): Promise<DocumentRecord>;
}

/**
 * Транспортный сбой перед PostgREST (например, 504 от gateway Supabase на "просыпающемся" после
 * паузы проекте — так и обнаружили: `error.message === 'Gateway Timeout'` без единого другого
 * поля) — в отличие от настоящей ошибки БД (нарушение constraint, невалидный запрос и т.п.),
 * которую PostgREST всегда возвращает вместе с `code` (Postgres SQLSTATE). Повтор второй ошибки
 * её не исправит — тот же constraint нарушится снова; повтор первой вполне может проскочить,
 * если к этому моменту сервис уже "проснулся"/сеть восстановилась.
 */
function isTransientSupabaseError(error: unknown): boolean {
  if (!error || typeof error !== 'object') return false;
  const code = (error as { code?: unknown }).code;
  return typeof code !== 'string' || code.length === 0;
}

function delay(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

interface RequestRow {
  id: string;
  status: RequestStatus;
  query_text: string;
  include_document: boolean;
  document_type: string | null;
  result_summary: string | null;
  error_message: string | null;
  clarification_question: string | null;
  clarification_answer: string | null;
  created_at: string;
  updated_at: string;
  // Присутствует в select('*') из requests, но намеренно не попадает в RequestRecord/mapRequest —
  // владением распоряжается только isOwnedBy(), хэш никогда не должен покидать репозиторий.
  owner_token_hash?: string | null;
}

interface RequestStepRow {
  id: string;
  request_id: string;
  agent_name: AgentName;
  ordinal: number;
  status: RequestStepStatus;
  input: unknown;
  output: unknown;
  error_message: string | null;
  started_at: string | null;
  finished_at: string | null;
}

interface DocumentRow {
  id: string;
  request_id: string;
  document_type: string;
  title: string;
  file_format: string;
  content: string;
  created_at: string;
}

@Injectable()
export class PgRequestsRepository implements RequestsRepository {
  constructor(@Inject(SUPABASE_ADMIN) private readonly supabase: SupabaseClient) {}

  /**
   * Один повтор при транспортном сбое (см. isTransientSupabaseError) — это единственный
   * синхронный шаг на пути POST /requests (до ответа пользователю, см. RequestsService.create),
   * и живой инцидент показал, что "просыпающийся" после паузы Supabase-проект может отдать 504
   * на первый запрос за несколько секунд, а на повтор почти сразу — раньше это было для
   * пользователя мгновенным 500 без единого шанса на успех. Не ретраим настоящие ошибки БД (те, у
   * которых есть `code`) — повтор их не исправит.
   */
  async createRequest(input: CreateRequestInput): Promise<RequestRecord> {
    const insert = () =>
      this.supabase
        .from('requests')
        .insert({
          query_text: input.queryText,
          include_document: input.includeDocument,
          document_type: input.documentType,
          owner_token_hash: input.ownerTokenHash,
        })
        .select()
        .single<RequestRow>();

    let result = await insert();
    if (result.error && isTransientSupabaseError(result.error)) {
      await delay(500);
      result = await insert();
    }
    if (result.error) throw result.error;
    return mapRequest(result.data);
  }

  async isOwnedBy(id: string, ownerTokenHash: string | null): Promise<boolean> {
    const { data, error } = await this.supabase
      .from('requests')
      .select('owner_token_hash')
      .eq('id', id)
      .maybeSingle<Pick<RequestRow, 'owner_token_hash'>>();
    if (error) throw error;
    if (!data) return false;
    // Раньше owner_token_hash === null трактовался как "ничей — доступен любому", включая
    // запросивших с валидной cookie-сессией (ownerTokenHash !== null). ensureSessionTokenHash
    // сейчас всегда проставляет hash при создании — NULL остаётся только у записей без
    // владельца, и такая запись должна быть доступна лишь тому, кто тоже пришёл без cookie
    // (ownerTokenHash === null), а не вообще всем, кто узнал id.
    return data.owner_token_hash === ownerTokenHash;
  }

  async listByOwner(ownerTokenHash: string, limit: number): Promise<RequestSummary[]> {
    const { data, error } = await this.supabase
      .from('requests')
      .select(
        'id, status, query_text, include_document, created_at, request_steps(started_at, finished_at)',
      )
      .eq('owner_token_hash', ownerTokenHash)
      .order('created_at', { ascending: false })
      .limit(limit)
      .returns<
        (Pick<RequestRow, 'id' | 'status' | 'query_text' | 'include_document' | 'created_at'> & {
          request_steps: StepTimestamps[];
        })[]
      >();
    if (error) throw error;
    return (data ?? []).map((row) => ({
      id: row.id,
      status: row.status,
      queryText: row.query_text,
      includeDocument: row.include_document,
      createdAt: new Date(row.created_at),
      processingMs: computeStepsDurationMs(row.request_steps),
    }));
  }

  async findRequestWithDetails(id: string): Promise<RequestWithDetails | null> {
    const { data: requestRow, error: requestError } = await this.supabase
      .from('requests')
      .select('*')
      .eq('id', id)
      .maybeSingle<RequestRow>();
    if (requestError) throw requestError;
    if (!requestRow) return null;

    const [stepsResult, documentResult] = await Promise.all([
      this.supabase
        .from('request_steps')
        .select('*')
        .eq('request_id', id)
        .order('ordinal', { ascending: true })
        .returns<RequestStepRow[]>(),
      this.supabase
        .from('documents')
        .select('*')
        .eq('request_id', id)
        .order('created_at', { ascending: false })
        .limit(1)
        .returns<DocumentRow[]>(),
    ]);
    if (stepsResult.error) throw stepsResult.error;
    if (documentResult.error) throw documentResult.error;

    return {
      ...mapRequest(requestRow),
      steps: (stepsResult.data ?? []).map(mapStep),
      document: documentResult.data?.[0] ? mapDocument(documentResult.data[0]) : null,
    };
  }

  async updateRequestStatus(
    id: string,
    status: RequestStatus,
    patch: { resultSummary?: string; errorMessage?: string } = {},
  ): Promise<void> {
    const update: Record<string, unknown> = { status, updated_at: new Date().toISOString() };
    if (patch.resultSummary !== undefined) update.result_summary = patch.resultSummary;
    if (patch.errorMessage !== undefined) update.error_message = patch.errorMessage;
    const { error } = await this.supabase.from('requests').update(update).eq('id', id);
    if (error) throw error;
  }

  async getStatus(id: string): Promise<RequestStatus | null> {
    const { data, error } = await this.supabase
      .from('requests')
      .select('status')
      .eq('id', id)
      .maybeSingle<Pick<RequestRow, 'status'>>();
    if (error) throw error;
    return data?.status ?? null;
  }

  async cancelIfActive(id: string): Promise<boolean> {
    const { data, error } = await this.supabase
      .from('requests')
      .update({ status: 'cancelled', updated_at: new Date().toISOString() })
      .eq('id', id)
      .in('status', ['pending', 'processing', 'needs_clarification'])
      .select('id');
    if (error) throw error;
    return (data ?? []).length > 0;
  }

  async setClarificationQuestion(id: string, question: string): Promise<void> {
    const { error } = await this.supabase
      .from('requests')
      .update({ clarification_question: question, updated_at: new Date().toISOString() })
      .eq('id', id);
    if (error) throw error;
  }

  async setClarificationAnswer(id: string, answer: string): Promise<void> {
    const { error } = await this.supabase
      .from('requests')
      .update({ clarification_answer: answer, updated_at: new Date().toISOString() })
      .eq('id', id);
    if (error) throw error;
  }

  async setDocumentRequested(id: string, documentType: string): Promise<void> {
    const { error } = await this.supabase
      .from('requests')
      .update({
        include_document: true,
        document_type: documentType,
        updated_at: new Date().toISOString(),
      })
      .eq('id', id);
    if (error) throw error;
  }

  async createStep(
    requestId: string,
    agentName: AgentName,
    ordinal: number,
  ): Promise<RequestStepRecord> {
    // upsert, а не insert: если BullMQ повторно доставит "зависшую" джобу после падения воркера
    // (stalled job — см. request-processing.processor.ts) и оркестратор начнёт пайплайн заново
    // с тем же requestId, ordinal снова стартует с 1 — plain insert упёрся бы в
    // UNIQUE(request_id, ordinal) и уронил бы запрос в 'failed' вместо того, чтобы дать пайплайну
    // повторную попытку. startStep()/finishStep() ниже всё равно сбрасывают статус/input/output
    // до актуальных значений этого прогона.
    const { data, error } = await this.supabase
      .from('request_steps')
      .upsert(
        { request_id: requestId, agent_name: agentName, ordinal },
        { onConflict: 'request_id,ordinal' },
      )
      .select()
      .single<RequestStepRow>();
    if (error) throw error;
    return mapStep(data);
  }

  async startStep(stepId: string, input: unknown): Promise<void> {
    const { error } = await this.supabase
      .from('request_steps')
      .update({ status: 'running', input, started_at: new Date().toISOString() })
      .eq('id', stepId);
    if (error) throw error;
  }

  async finishStep(
    stepId: string,
    status: RequestStepStatus,
    patch: { output?: unknown; errorMessage?: string },
  ): Promise<void> {
    const { error } = await this.supabase
      .from('request_steps')
      .update({
        status,
        output: patch.output ?? null,
        error_message: patch.errorMessage ?? null,
        finished_at: new Date().toISOString(),
      })
      .eq('id', stepId);
    if (error) throw error;
  }

  async createDocument(
    requestId: string,
    documentType: string,
    title: string,
    fileFormat: string,
    content: string,
  ): Promise<DocumentRecord> {
    const { data, error } = await this.supabase
      .from('documents')
      .insert({
        request_id: requestId,
        document_type: documentType,
        title,
        file_format: fileFormat,
        content,
      })
      .select()
      .single<DocumentRow>();
    if (error) throw error;
    return mapDocument(data);
  }
}

function mapRequest(row: RequestRow): RequestRecord {
  return {
    id: row.id,
    status: row.status,
    queryText: row.query_text,
    includeDocument: row.include_document,
    documentType: row.document_type,
    resultSummary: row.result_summary,
    errorMessage: row.error_message,
    clarificationQuestion: row.clarification_question,
    clarificationAnswer: row.clarification_answer,
    createdAt: new Date(row.created_at),
    updatedAt: new Date(row.updated_at),
  };
}

function mapStep(row: RequestStepRow): RequestStepRecord {
  return {
    id: row.id,
    requestId: row.request_id,
    agentName: row.agent_name,
    ordinal: row.ordinal,
    status: row.status,
    input: row.input,
    output: row.output,
    errorMessage: row.error_message,
    startedAt: row.started_at ? new Date(row.started_at) : null,
    finishedAt: row.finished_at ? new Date(row.finished_at) : null,
  };
}

function mapDocument(row: DocumentRow): DocumentRecord {
  return {
    id: row.id,
    requestId: row.request_id,
    documentType: row.document_type,
    title: row.title,
    fileFormat: row.file_format,
    content: row.content,
    createdAt: new Date(row.created_at),
  };
}
