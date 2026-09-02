import { describe, expect, it, vi } from 'vitest';
import type { ChatClient, ChatCompletionRequest } from '@zan/shared';
import { LawSearchAgent } from './law-search.agent.js';

function fakeChat(content: string): ChatClient {
  return { complete: vi.fn(async () => content) };
}

describe('LawSearchAgent — живой поиск (web_search на каждый вызов)', () => {
  it('запрашивает web_search, ограниченный adilet.zan.kz/zan.gov.kz, и возвращает ответ модели', async () => {
    const chat = fakeChat('Согласно ТК РК, увольнение возможно... (Трудовой кодекс РК, статья 52)');
    const agent = new LawSearchAgent(chat);

    const output = await agent.run({
      queryText: 'Меня уволили без предупреждения, законно ли это?',
    });

    expect(output.draftAnswer).toBe(
      'Согласно ТК РК, увольнение возможно... (Трудовой кодекс РК, статья 52)',
    );

    const request = (chat.complete as ReturnType<typeof vi.fn>).mock
      .calls[0]![0] as ChatCompletionRequest;
    expect(request.user).toContain('Меня уволили без предупреждения, законно ли это?');
    expect(request.webSearch).toEqual({ allowedDomains: ['adilet.zan.kz', 'zan.gov.kz'] });
  });

  it('бросает ошибку с пояснением, если модель решает, что вопрос вне темы права РК', async () => {
    const chat = fakeChat('OUT_OF_TOPIC: это не юридический вопрос.');
    const agent = new LawSearchAgent(chat);

    await expect(agent.run({ queryText: 'как приготовить бешбармак?' })).rejects.toThrow(
      'это не юридический вопрос.',
    );
  });
});
