import { describe, expect, it, vi } from 'vitest';
import type { ChatClient, ChatCompletionRequest } from '@zan/shared';
import { LawEditorAgent } from './law-editor.agent.js';

function fakeChat(response = 'финальный ответ'): ChatClient {
  return {
    complete: vi.fn(async () => response),
  };
}

describe('LawEditorAgent', () => {
  it('формирует финальный ответ на основе чернового ответа Агента 1', async () => {
    const chat = fakeChat('## Суть вопроса\n...\n(Трудовой кодекс РК, статья 52)');
    const agent = new LawEditorAgent(chat);

    const output = await agent.run({
      queryText: 'Меня уволили без предупреждения, законно ли это?',
      draftAnswer: 'Черновой ответ, ссылается на статью 52.',
    });

    expect(output.summary).toContain('статья 52');

    const request = (chat.complete as ReturnType<typeof vi.fn>).mock
      .calls[0]![0] as ChatCompletionRequest;
    expect(request.user).toContain('Черновой ответ, ссылается на статью 52.');
    expect(request.system).toMatch(/живом разговоре/i);
    expect(request.system).toMatch(/Что говорит закон/);
    expect(request.system).toMatch(/Что можно сделать/);
    expect(request.system).toMatch(/OUT_OF_TOPIC:/);
    expect(request.reasoningEffort).toBe('medium');
  });
});
