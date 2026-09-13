import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { SupabaseClient } from '@supabase/supabase-js';
import { PgRequestsRepository } from './requests.repository.js';
import type { CreateRequestInput } from './request.types.js';

/**
 * Живой инцидент (2026-09-13): Supabase-проект после паузы отдал 504 ("Gateway Timeout") на
 * INSERT — единственный синхронный шаг POST /requests (см. RequestsService.create). Этот сбой не
 * несёт `code` (Postgres SQLSTATE) в отличие от настоящей ошибки БД, поэтому createRequest теперь
 * ретраит один раз именно такие сбои (см. isTransientSupabaseError в requests.repository.ts).
 */

const INPUT: CreateRequestInput = {
  queryText: 'Вопрос пользователя длиной больше десяти символов',
  includeDocument: false,
  documentType: null,
  ownerTokenHash: null,
};

const ROW = {
  id: 'req-1',
  status: 'pending',
  query_text: INPUT.queryText,
  include_document: false,
  document_type: null,
  result_summary: null,
  error_message: null,
  clarification_question: null,
  clarification_answer: null,
  created_at: '2026-01-01T00:00:00.000Z',
  updated_at: '2026-01-01T00:00:00.000Z',
};

function fakeSupabase(responses: Array<{ data: unknown; error: unknown }>): SupabaseClient {
  let call = 0;
  const single = vi.fn(async () => responses[Math.min(call++, responses.length - 1)]);
  const select = vi.fn(() => ({ single }));
  const insert = vi.fn(() => ({ select }));
  const from = vi.fn(() => ({ insert }));
  return { from } as unknown as SupabaseClient;
}

describe('PgRequestsRepository.createRequest', () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it('повторяет вставку один раз при транспортном сбое (без code) и возвращает успех со второй попытки', async () => {
    const supabase = fakeSupabase([
      { data: null, error: { message: 'Gateway Timeout' } },
      { data: ROW, error: null },
    ]);
    const repo = new PgRequestsRepository(supabase);

    const pending = repo.createRequest(INPUT);
    await vi.advanceTimersByTimeAsync(500);
    const result = await pending;

    expect(result.id).toBe('req-1');
    expect(supabase.from).toHaveBeenCalledTimes(2);
  });

  it('не повторяет вставку при настоящей ошибке БД (есть code) — сразу бросает', async () => {
    const dbError = { message: 'duplicate key value violates unique constraint', code: '23505' };
    const supabase = fakeSupabase([{ data: null, error: dbError }]);
    const repo = new PgRequestsRepository(supabase);

    await expect(repo.createRequest(INPUT)).rejects.toBe(dbError);
    expect(supabase.from).toHaveBeenCalledTimes(1);
  });

  it('бросает исходную ошибку, если транспортный сбой повторяется дважды подряд', async () => {
    const timeoutError = { message: 'Gateway Timeout' };
    const supabase = fakeSupabase([
      { data: null, error: timeoutError },
      { data: null, error: timeoutError },
    ]);
    const repo = new PgRequestsRepository(supabase);

    const pending = repo.createRequest(INPUT).catch((error: unknown) => error);
    await vi.advanceTimersByTimeAsync(500);
    const error = await pending;

    expect(error).toBe(timeoutError);
    expect(supabase.from).toHaveBeenCalledTimes(2);
  });
});
