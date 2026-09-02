import { describe, expect, it } from 'vitest';
import { buildDocumentBuffer } from './docx-template.js';

describe('buildDocumentBuffer', () => {
  it('генерирует валидный DOCX (OOXML/ZIP) достаточного размера', async () => {
    const buffer = await buildDocumentBuffer({
      title: 'Заявление о расторжении трудового договора',
      recipientLines: ['Директору ТОО "Ромашка"', 'от [ФИО],', 'проживающего по адресу [адрес]'],
      bodyParagraphs: ['Прошу расторгнуть трудовой договор.', 'Основание: статья 52 ТК РК.'],
    });

    // DOCX — это ZIP-архив (OOXML); первые байты — сигнатура ZIP "PK".
    expect(buffer.subarray(0, 2).toString('latin1')).toBe('PK');
    expect(buffer.length).toBeGreaterThan(1000);
  });

  it('не падает, если пунктов основного текста один и шапка пустая', async () => {
    const buffer = await buildDocumentBuffer({
      title: 'Заявление',
      recipientLines: [],
      bodyParagraphs: ['Единственный пункт документа.'],
    });

    expect(buffer.subarray(0, 2).toString('latin1')).toBe('PK');
  });
});
