import { describe, expect, it, vi } from 'vitest';
import type { ChatClient, ChatCompletionRequest } from '@zan/shared';
import { LawVerificationAgent } from './law-verification.agent.js';

function fakeChat(response = ''): ChatClient {
  return { complete: vi.fn(async () => response) };
}

describe('LawVerificationAgent — независимая LLM-перепроверка с web_search', () => {
  it('возвращает исправленный ответ и замечания от независимой перепроверки', async () => {
    const chat = fakeChat(
      JSON.stringify({
        revisedAnswer: 'исправленный ответ (Трудовой кодекс РК, статья 52)',
        concerns: ['уточнена формулировка про сроки уведомления'],
      }),
    );
    const agent = new LawVerificationAgent(chat);

    const output = await agent.run({
      queryText: 'Меня уволили без предупреждения',
      draftAnswer: 'черновой ответ',
    });

    expect(output.revisedAnswer).toBe('исправленный ответ (Трудовой кодекс РК, статья 52)');
    expect(output.concerns).toEqual(['уточнена формулировка про сроки уведомления']);

    const request = (chat.complete as ReturnType<typeof vi.fn>).mock
      .calls[0]![0] as ChatCompletionRequest;
    expect(request.user).toContain('черновой ответ');
    expect(request.system).toMatch(/независимый юрист-эксперт/i);
    expect(request.webSearch).toEqual({ allowedDomains: ['adilet.zan.kz', 'zan.gov.kz'] });
  });

  it('не находит замечаний, если черновик подтверждён проверкой', async () => {
    const chat = fakeChat(
      JSON.stringify({
        revisedAnswer: 'подтверждённый ответ (Трудовой кодекс РК, статья 52)',
        concerns: [],
      }),
    );
    const agent = new LawVerificationAgent(chat);

    const output = await agent.run({
      queryText: 'вопрос',
      draftAnswer: 'подтверждённый ответ (Трудовой кодекс РК, статья 52)',
    });

    expect(output.concerns).toEqual([]);
  });

  it('fail-open: при невалидном JSON от LLM возвращает исходный черновой ответ с пометкой о сбое', async () => {
    const chat = fakeChat('это не JSON');
    const agent = new LawVerificationAgent(chat);

    const output = await agent.run({ queryText: 'вопрос', draftAnswer: 'исходный черновик' });

    expect(output.revisedAnswer).toBe('исходный черновик');
    expect(output.concerns).toEqual([expect.stringMatching(/не удалось выполнить/i)]);
  });

  it('fail-open: при исключении из вызова LLM возвращает исходный черновик без замечаний', async () => {
    const chat: ChatClient = {
      complete: vi.fn(async () => {
        throw new Error('OpenAI недоступен');
      }),
    };
    const agent = new LawVerificationAgent(chat);

    const output = await agent.run({ queryText: 'вопрос', draftAnswer: 'исходный черновик' });

    expect(output.revisedAnswer).toBe('исходный черновик');
    expect(output.concerns).toEqual([]);
  });
});
