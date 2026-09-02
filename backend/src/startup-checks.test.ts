import { describe, expect, it } from 'vitest';
import { evaluateStartupChecks, formatStartupFailure } from './startup-checks.js';

function neverResolves(): Promise<void> {
  return new Promise(() => {});
}

describe('evaluateStartupChecks', () => {
  it('возвращает ok для всех проверок, если все run() успешно резолвятся', async () => {
    const results = await evaluateStartupChecks([
      { service: 'A', run: async () => {} },
      { service: 'B', run: async () => {} },
    ]);

    expect(results).toEqual([
      { service: 'A', ok: true },
      { service: 'B', ok: true },
    ]);
  });

  it('фиксирует конкретную ошибку для упавшей проверки, не трогая остальные', async () => {
    const results = await evaluateStartupChecks([
      { service: 'A', run: async () => {} },
      {
        service: 'B',
        run: async () => {
          throw new Error('неверный ключ');
        },
      },
    ]);

    expect(results).toEqual([
      { service: 'A', ok: true },
      { service: 'B', ok: false, error: 'неверный ключ' },
    ]);
  });

  it('считает "зависшую" (никогда не резолвящуюся) проверку failed по тайм-ауту, а не висит вечно', async () => {
    const results = await evaluateStartupChecks([{ service: 'Stuck', run: neverResolves }]);

    expect(results).toHaveLength(1);
    expect(results[0]!.ok).toBe(false);
    expect(results[0]!.error).toMatch(/тайм-аут/i);
  }, 7000);
});

describe('formatStartupFailure', () => {
  it('возвращает null, если все проверки прошли', () => {
    expect(formatStartupFailure([{ service: 'A', ok: true }])).toBeNull();
  });

  it('перечисляет все упавшие сервисы с их причинами в одном сообщении', () => {
    const message = formatStartupFailure([
      { service: 'A', ok: true },
      { service: 'B', ok: false, error: 'нет соединения' },
      { service: 'C', ok: false, error: '401' },
    ]);

    expect(message).toContain('B (нет соединения)');
    expect(message).toContain('C (401)');
    expect(message).not.toContain('A');
  });
});
