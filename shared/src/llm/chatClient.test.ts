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

  it('эхо requestId в LlmCallInfo, если он передан в запросе — не отправляется в OpenAI', async () => {
    chatCompletionsCreate.mockResolvedValueOnce({
      choices: [{ message: { content: 'ответ' } }],
    });
    const onCall = vi.fn<(info: LlmCallInfo) => void>();
    const client = new OpenAiChatClient('key', onCall);

    await client.complete({ system: 'sys', user: 'user', requestId: 'req-42' });

    expect(onCall.mock.calls[0]![0]).toMatchObject({ requestId: 'req-42' });
    expect(chatCompletionsCreate.mock.calls.at(-1)![0]).not.toHaveProperty('requestId');
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

  it('completeWithWebSearchMeta возвращает responseId и citedUrls из url_citation-аннотаций', async () => {
    responsesCreate.mockResolvedValueOnce({
      id: 'resp_abc123',
      output_text: 'ответ с поиском',
      output: [
        {
          type: 'message',
          content: [
            {
              type: 'output_text',
              text: 'ответ с поиском',
              annotations: [
                { type: 'url_citation', url: 'https://adilet.zan.kz/rus/docs/K1000000000#z52' },
                { type: 'url_citation', url: 'https://adilet.zan.kz/rus/docs/K1000000000#z52' },
                { type: 'url_citation', url: 'https://adilet.zan.kz/rus/docs/K2000000000#z1' },
              ],
            },
          ],
        },
      ],
    });
    const client = new OpenAiChatClient('key');

    const result = await client.completeWithWebSearchMeta(
      { system: 'sys', user: 'user' },
      { allowedDomains: ['adilet.zan.kz'] },
    );

    expect(result).toEqual({
      content: 'ответ с поиском',
      responseId: 'resp_abc123',
      citedUrls: [
        'https://adilet.zan.kz/rus/docs/K1000000000#z52',
        'https://adilet.zan.kz/rus/docs/K2000000000#z1',
      ],
    });
  });

  it('completeWithWebSearchMeta прокидывает previousResponseId в OpenAI как previous_response_id', async () => {
    responsesCreate.mockResolvedValueOnce({ id: 'resp_2', output_text: 'ответ', output: [] });
    const client = new OpenAiChatClient('key');

    await client.completeWithWebSearchMeta(
      { system: 'sys', user: 'user', previousResponseId: 'resp_1' },
      { allowedDomains: ['adilet.zan.kz'] },
    );

    expect(responsesCreate.mock.calls.at(-1)![0]).toMatchObject({
      previous_response_id: 'resp_1',
    });
  });

  it('completeWithWebSearchMeta возвращает пустой citedUrls, если аннотаций нет (не считается ошибкой)', async () => {
    responsesCreate.mockResolvedValueOnce({ id: 'resp_3', output_text: 'ответ', output: [] });
    const client = new OpenAiChatClient('key');

    const result = await client.completeWithWebSearchMeta(
      { system: 'sys', user: 'user' },
      { allowedDomains: ['adilet.zan.kz'] },
    );

    expect(result.citedUrls).toEqual([]);
  });

  it('completeWithWebSearchMetaStream вызывает onDelta на текстовые дельты и возвращает id/citedUrls из response.completed', async () => {
    async function* fakeResponseStream() {
      yield { type: 'response.output_text.delta', delta: 'При' };
      yield { type: 'response.output_text.delta', delta: 'вет' };
      yield {
        type: 'response.completed',
        response: {
          id: 'resp_stream_1',
          output: [
            {
              type: 'message',
              content: [
                {
                  type: 'output_text',
                  text: 'Привет',
                  annotations: [
                    { type: 'url_citation', url: 'https://adilet.zan.kz/rus/docs/K1#z1' },
                  ],
                },
              ],
            },
          ],
        },
      };
    }
    responsesCreate.mockResolvedValueOnce(fakeResponseStream());
    const client = new OpenAiChatClient('key');
    const deltas: string[] = [];

    const result = await client.completeWithWebSearchMetaStream(
      { system: 'sys', user: 'user' },
      { allowedDomains: ['adilet.zan.kz'] },
      (delta) => deltas.push(delta),
    );

    expect(deltas).toEqual(['При', 'вет']);
    expect(result).toEqual({
      content: 'Привет',
      responseId: 'resp_stream_1',
      citedUrls: ['https://adilet.zan.kz/rus/docs/K1#z1'],
    });
    expect(responsesCreate.mock.calls.at(-1)![0]).toMatchObject({ stream: true });
  });

  it('completeWithWebSearchMetaStream бросает исключение, если не пришло ни текстовых дельт, ни output_text в финальном ответе', async () => {
    async function* emptyResponseStream() {
      yield { type: 'response.completed', response: { id: 'resp_empty', output: [] } };
    }
    responsesCreate.mockResolvedValueOnce(emptyResponseStream());
    const client = new OpenAiChatClient('key');

    await expect(
      client.completeWithWebSearchMetaStream(
        { system: 'sys', user: 'user' },
        { allowedDomains: ['adilet.zan.kz'] },
        () => {},
      ),
    ).rejects.toThrow();
  });

  it('работает без onCall (необязательный колбэк)', async () => {
    chatCompletionsCreate.mockResolvedValueOnce({ choices: [{ message: { content: 'ok' } }] });
    const client = new OpenAiChatClient('key');

    await expect(client.complete({ system: 'sys', user: 'user' })).resolves.toBe('ok');
  });
});
