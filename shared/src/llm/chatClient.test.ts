import { describe, expect, it, vi } from 'vitest';

const chatCompletionsCreate = vi.fn();
const responsesCreate = vi.fn();

vi.mock('openai', () => ({
  default: class {
    chat = { completions: { create: chatCompletionsCreate } };
    responses = { create: responsesCreate };
  },
}));

const { OpenAiChatClient } = await import('./chatClient.js');
import type { LlmCallInfo } from './chatClient.js';

describe('OpenAiChatClient', () => {
  it('репортит успешный вызов chat.completions через onCall с моделью и operation=chat', async () => {
    chatCompletionsCreate.mockResolvedValueOnce({
      choices: [{ message: { content: 'ответ' } }],
    });
    const onCall = vi.fn<(info: LlmCallInfo) => void>();
    const client = new OpenAiChatClient('key', onCall);

    const result = await client.complete({
      system: 'sys',
      user: 'user',
      model: 'gpt-5.6-sol',
    });

    expect(result).toBe('ответ');
    expect(onCall).toHaveBeenCalledTimes(1);
    const [info] = onCall.mock.calls[0]!;
    expect(info.model).toBe('gpt-5.6-sol');
    expect(info.operation).toBe('chat');
    expect(info.outcome).toBe('success');
    expect(info.durationMs).toBeGreaterThanOrEqual(0);
  });

  it('репортит ошибку chat.completions через onCall с outcome=error, но пробрасывает исключение', async () => {
    chatCompletionsCreate.mockRejectedValueOnce(new Error('network boom'));
    const onCall = vi.fn<(info: LlmCallInfo) => void>();
    const client = new OpenAiChatClient('key', onCall);

    await expect(client.complete({ system: 'sys', user: 'user' })).rejects.toThrow('network boom');

    expect(onCall).toHaveBeenCalledTimes(1);
    expect(onCall.mock.calls[0]![0]).toMatchObject({ operation: 'chat', outcome: 'error' });
  });

  it('репортит пустой ответ chat.completions как outcome=error', async () => {
    chatCompletionsCreate.mockResolvedValueOnce({ choices: [{ message: { content: '  ' } }] });
    const onCall = vi.fn<(info: LlmCallInfo) => void>();
    const client = new OpenAiChatClient('key', onCall);

    await expect(client.complete({ system: 'sys', user: 'user' })).rejects.toThrow();
    expect(onCall.mock.calls[0]![0]).toMatchObject({ operation: 'chat', outcome: 'error' });
  });

  it('репортит вызов через Responses API (web search) с operation=responses', async () => {
    responsesCreate.mockResolvedValueOnce({ output_text: 'ответ с поиском', output: [] });
    const onCall = vi.fn<(info: LlmCallInfo) => void>();
    const client = new OpenAiChatClient('key', onCall);

    const result = await client.complete({
      system: 'sys',
      user: 'user',
      model: 'gpt-5.6-sol',
      webSearch: { allowedDomains: ['adilet.zan.kz'] },
    });

    expect(result).toBe('ответ с поиском');
    expect(onCall.mock.calls[0]![0]).toMatchObject({
      model: 'gpt-5.6-sol',
      operation: 'responses',
      outcome: 'success',
    });
  });

  it('работает без onCall (необязательный колбэк)', async () => {
    chatCompletionsCreate.mockResolvedValueOnce({ choices: [{ message: { content: 'ok' } }] });
    const client = new OpenAiChatClient('key');

    await expect(client.complete({ system: 'sys', user: 'user' })).resolves.toBe('ok');
  });
});
