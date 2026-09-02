import { describe, expect, it, vi } from 'vitest';
import { withStepLogging } from './withStepLogging.js';
import type { Logger } from './createLogger.js';

function fakeLogger(): Logger {
  return {
    info: vi.fn(),
    warn: vi.fn(),
    error: vi.fn(),
    child: vi.fn(function (this: Logger) {
      return this;
    }),
  };
}

describe('withStepLogging', () => {
  it('логирует success и возвращает результат при успехе', async () => {
    const logger = fakeLogger();
    const result = await withStepLogging(logger, 'test.step', { foo: 'bar' }, async () => 42);

    expect(result).toBe(42);
    expect(logger.info).toHaveBeenCalledTimes(1);
    const [meta] = vi.mocked(logger.info).mock.calls[0]!;
    expect(meta).toMatchObject({ stage: 'test.step', status: 'success', meta: { foo: 'bar' } });
  });

  it('логирует failure и пробрасывает исходную ошибку', async () => {
    const logger = fakeLogger();
    const error = new Error('boom');

    await expect(
      withStepLogging(logger, 'test.step', {}, async () => {
        throw error;
      }),
    ).rejects.toBe(error);

    expect(logger.error).toHaveBeenCalledTimes(1);
    const [meta] = vi.mocked(logger.error).mock.calls[0]!;
    expect(meta).toMatchObject({ stage: 'test.step', status: 'failure', error: 'boom' });
  });
});
