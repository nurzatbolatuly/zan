import { describe, expect, it, vi } from 'vitest';
import JSZip from 'jszip';
import type { ChatClient, ChatCompletionRequest } from '@zan/shared';
import { DOCX_MIME_TYPE, LawDocumentAgent } from './law-document.agent.js';

function fakeChat(response: string): ChatClient {
  return {
    complete: vi.fn(async () => response),
    completeWithWebSearchMeta: vi.fn(async () => ({
      content: response,
      responseId: 'resp_document_1',
      citedUrls: [],
    })),
    completeWithWebSearchMetaStream: vi.fn(),
  };
}

const validContent = JSON.stringify({
  title: 'Заявление о расторжении трудового договора',
  recipientLines: ['Директору ТОО "Ромашка"', 'от [ФИО]'],
  bodyParagraphs: ['Прошу расторгнуть трудовой договор со мной.', 'Основание указано ниже.'],
});

describe('LawDocumentAgent', () => {
  it('генерирует документ на основе JSON-содержимого от LLM, опираясь на черновой ответ', async () => {
    const chat = fakeChat(validContent);
    const agent = new LawDocumentAgent(chat);

    const output = await agent.run({
      queryText: 'Меня уволили без предупреждения',
      draftAnswer: 'Черновой ответ (Трудовой кодекс РК, статья 52).',
      documentType: 'заявление',
      previousResponseId: 'resp_verify_1',
      requestId: 'req-1',
    });

    expect(output.title).toBe('Заявление о расторжении трудового договора');
    expect(output.fileFormat).toBe(DOCX_MIME_TYPE);
    expect(Buffer.from(output.content, 'base64').subarray(0, 2).toString('latin1')).toBe('PK');

    const request = (chat.complete as ReturnType<typeof vi.fn>).mock
      .calls[0]![0] as ChatCompletionRequest;
    expect(request.user).toContain('Черновой ответ (Трудовой кодекс РК, статья 52).');
    expect(request.system).toMatch(/Правило цитирования норм/);
  });

  it('НЕ даёт web_search для типа документа без вероятной официальной формы ("заявление")', async () => {
    const chat = fakeChat(validContent);
    const agent = new LawDocumentAgent(chat);

    await agent.run({
      queryText: 'вопрос',
      draftAnswer: 'ответ',
      documentType: 'заявление',
      previousResponseId: 'resp_verify_1',
      requestId: 'req-1',
    });

    expect(chat.completeWithWebSearchMeta).not.toHaveBeenCalled();
    const request = (chat.complete as ReturnType<typeof vi.fn>).mock
      .calls[0]![0] as ChatCompletionRequest;
    expect(request.webSearch).toBeUndefined();
    expect(request.system).not.toMatch(/У тебя есть инструмент/);
  });

  it('даёт web_search для процессуальных типов документа ("исковое заявление") и продолжает цепочку Агента "answer"', async () => {
    const chat = fakeChat(validContent);
    const agent = new LawDocumentAgent(chat);

    await agent.run({
      queryText: 'вопрос',
      draftAnswer: 'ответ',
      documentType: 'исковое заявление',
      previousResponseId: 'resp_verify_1',
      requestId: 'req-1',
    });

    expect(chat.complete).not.toHaveBeenCalled();
    const meta = chat.completeWithWebSearchMeta as ReturnType<typeof vi.fn>;
    const request = meta.mock.calls[0]![0] as ChatCompletionRequest;
    const webSearch = meta.mock.calls[0]![1] as { allowedDomains: string[] };
    expect(webSearch).toEqual({ allowedDomains: ['adilet.zan.kz', 'zan.gov.kz'] });
    expect(request.previousResponseId).toBe('resp_verify_1');
    expect(request.system).toMatch(/У тебя есть инструмент/);
  });

  it('применяет formatting от LLM, если оно найдено в законе', async () => {
    const chat = fakeChat(
      JSON.stringify({
        title: 'Исковое заявление',
        recipientLines: ['В районный суд №2 г. Астаны'],
        bodyParagraphs: ['Прошу взыскать сумму долга.'],
        formatting: { fontFamily: 'Times New Roman', fontSizePt: 14, lineSpacingMultiplier: 1.5 },
      }),
    );
    const agent = new LawDocumentAgent(chat);

    const output = await agent.run({
      queryText: 'вопрос',
      draftAnswer: 'ответ',
      documentType: 'исковое заявление',
      previousResponseId: 'resp_verify_1',
      requestId: 'req-1',
    });

    const zip = await JSZip.loadAsync(Buffer.from(output.content, 'base64'));
    const stylesXml = await zip.file('word/styles.xml')?.async('string');
    expect(stylesXml).toContain('Times New Roman');
  });

  it(
    'падает с ошибкой, если LLM вернула formatting с полем неверного типа (защита от мусора ' +
      'в docx-template.ts)',
    async () => {
      const chat = fakeChat(
        JSON.stringify({
          title: 'Заявление',
          recipientLines: [],
          bodyParagraphs: ['Пункт.'],
          formatting: { fontSizePt: '14' },
        }),
      );
      const agent = new LawDocumentAgent(chat);

      await expect(
        agent.run({
          queryText: 'вопрос',
          draftAnswer: 'ответ',
          documentType: 'заявление',
          previousResponseId: 'resp_verify_1',
          requestId: 'req-1',
        }),
      ).rejects.toThrow(/не соответствует ожидаемой структуре/i);
    },
  );

  it('падает с ошибкой, если LLM вернул невалидный JSON', async () => {
    const chat = fakeChat('это не JSON');
    const agent = new LawDocumentAgent(chat);

    await expect(
      agent.run({
        queryText: 'вопрос',
        draftAnswer: 'ответ',
        documentType: 'заявление',
        previousResponseId: 'resp_verify_1',
        requestId: 'req-1',
      }),
    ).rejects.toThrow(/невалидный JSON/i);
  });

  it('падает с ошибкой, если JSON валиден, но не соответствует ожидаемой структуре', async () => {
    const chat = fakeChat(JSON.stringify({ title: 'Заявление' }));
    const agent = new LawDocumentAgent(chat);

    await expect(
      agent.run({
        queryText: 'вопрос',
        draftAnswer: 'ответ',
        documentType: 'заявление',
        previousResponseId: 'resp_verify_1',
        requestId: 'req-1',
      }),
    ).rejects.toThrow(/не соответствует ожидаемой структуре/i);
  });

  it('падает с ошибкой вместо генерации документа, если LLM решает, что вопрос вне темы права РК', async () => {
    const chat = fakeChat(
      JSON.stringify({
        title: 'OUT_OF_TOPIC',
        recipientLines: [],
        bodyParagraphs: ['Документ не может быть составлен вне сферы права РК.'],
      }),
    );
    const agent = new LawDocumentAgent(chat);

    await expect(
      agent.run({
        queryText: 'как приготовить бешбармак?',
        draftAnswer: 'черновик',
        documentType: 'заявление',
        previousResponseId: 'resp_verify_1',
        requestId: 'req-1',
      }),
    ).rejects.toThrow('Документ не может быть составлен вне сферы права РК.');
  });

  it('падает с ошибкой вместо генерации документа, если LLM решает, что фактов не хватает', async () => {
    const chat = fakeChat(
      JSON.stringify({
        title: 'INSUFFICIENT_CONTEXT',
        recipientLines: [],
        bodyParagraphs: ['Не хватает даты увольнения и названия работодателя.'],
      }),
    );
    const agent = new LawDocumentAgent(chat);

    await expect(
      agent.run({
        queryText: 'составьте мне заявление',
        draftAnswer: 'черновик без конкретики',
        documentType: 'заявление',
        previousResponseId: 'resp_verify_1',
        requestId: 'req-1',
      }),
    ).rejects.toThrow('Не хватает даты увольнения и названия работодателя.');
  });
});
