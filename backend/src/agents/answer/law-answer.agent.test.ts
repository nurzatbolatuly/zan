import { describe, expect, it, vi } from 'vitest';
import type { ChatClient, ChatCompletionRequest, WebSearchResult } from '@zan/shared';
import { LawAnswerAgent } from './law-answer.agent.js';

function fakeChat(content: string, meta: Partial<WebSearchResult> = {}): ChatClient {
  return {
    complete: vi.fn(),
    completeWithWebSearchMeta: vi.fn(),
    completeWithWebSearchMetaStream: vi.fn(
      async (
        _request: unknown,
        _webSearch: unknown,
        onDelta: (delta: string) => void,
      ): Promise<WebSearchResult> => {
        // Режем на произвольные куски (не по словам/границам префикса), чтобы проверить, что
        // буферизация OUT_OF_TOPIC работает независимо от того, как OpenAI режет дельты.
        for (const chunk of content.match(/.{1,3}/g) ?? []) {
          onDelta(chunk);
        }
        return {
          content,
          responseId: 'resp_answer_1',
          citedUrls: ['https://adilet.zan.kz/rus/docs/K1000000000#z52'],
          ...meta,
        };
      },
    ),
  };
}

describe('LawAnswerAgent — один вызов с web_search вместо search→verification→editor', () => {
  it('запрашивает web_search, ограниченный adilet.zan.kz/zan.gov.kz, и возвращает финальный ответ + метаданные диалога', async () => {
    const chat = fakeChat(
      '## Что говорит закон\nТК РК, статья 52 (Трудовой кодекс РК, статья 52).',
    );
    const agent = new LawAnswerAgent(chat);

    const output = await agent.run({
      queryText: 'Меня уволили без предупреждения, законно ли это?',
      requestId: 'req-1',
    });

    expect(output.answer).toContain('статья 52');
    expect(output.responseId).toBe('resp_answer_1');
    expect(output.citedUrls).toEqual(['https://adilet.zan.kz/rus/docs/K1000000000#z52']);

    const meta = chat.completeWithWebSearchMetaStream as ReturnType<typeof vi.fn>;
    const request = meta.mock.calls[0]![0] as ChatCompletionRequest;
    const webSearch = meta.mock.calls[0]![1] as { allowedDomains: string[] };
    expect(request.user).toContain('Меня уволили без предупреждения, законно ли это?');
    expect(request.reasoningEffort).toBe('medium');
    expect(request.requestId).toBe('req-1');
    expect(webSearch).toEqual({ allowedDomains: ['adilet.zan.kz', 'zan.gov.kz'] });
  });

  it('пробрасывает потоковые куски текста в onToken по мере генерации, склеиваясь в исходный текст', async () => {
    const fullText = 'Согласно ТК РК, увольнение возможно... (Трудовой кодекс РК, статья 52)';
    const chat = fakeChat(fullText);
    const agent = new LawAnswerAgent(chat);
    const received: string[] = [];

    await agent.run({
      queryText: 'вопрос',
      requestId: 'req-1',
      onToken: (delta) => received.push(delta),
    });

    expect(received.join('')).toBe(fullText);
  });

  it('бросает ошибку с пояснением, если модель решает, что вопрос вне темы права РК', async () => {
    const chat = fakeChat('OUT_OF_TOPIC: это не юридический вопрос.');
    const agent = new LawAnswerAgent(chat);

    await expect(
      agent.run({ queryText: 'как приготовить бешбармак?', requestId: 'req-1' }),
    ).rejects.toThrow('это не юридический вопрос.');
  });

  it('НЕ пропускает в onToken ни одного куска, если ответ начинается с OUT_OF_TOPIC (пользователь не должен увидеть служебный префикс)', async () => {
    const chat = fakeChat('OUT_OF_TOPIC: это не юридический вопрос.');
    const agent = new LawAnswerAgent(chat);
    const onToken = vi.fn();

    await expect(agent.run({ queryText: 'вопрос', requestId: 'req-1', onToken })).rejects.toThrow();

    expect(onToken).not.toHaveBeenCalled();
  });
});
