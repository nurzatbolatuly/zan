import { describe, expect, it } from 'vitest';
import JSZip from 'jszip';
import { buildDocumentBuffer } from './docx-template.js';

/** DOCX — это ZIP/OOXML; текст пунктов и их форматирование лежат в word/document.xml. */
async function readDocumentXml(buffer: Buffer): Promise<string> {
  const zip = await JSZip.loadAsync(buffer);
  const xml = await zip.file('word/document.xml')?.async('string');
  if (xml === undefined) throw new Error('word/document.xml отсутствует в сгенерированном DOCX');
  return xml;
}

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

  it(
    'без formatting не проставляет ни отступ первой строки, ни межстрочный интервал — ' +
      'шаблонизатор ничего не придумывает сам, только то, что нашёл агент в законе (см. law-document.agent.ts)',
    async () => {
      const buffer = await buildDocumentBuffer({
        title: 'Заявление',
        recipientLines: ['Директору ТОО "Ромашка"'],
        bodyParagraphs: ['Прошу расторгнуть трудовой договор.'],
      });
      const xml = await readDocumentXml(buffer);

      expect(xml).not.toContain('w:firstLine');
      expect(xml).not.toContain('w:lineRule');
    },
  );

  it('применяет ровно то форматирование, которое передал агент (formatting), и ничего сверх этого', async () => {
    const buffer = await buildDocumentBuffer({
      title: 'Исковое заявление',
      recipientLines: ['В районный суд №2 г. Астаны'],
      bodyParagraphs: ['Прошу взыскать сумму долга.'],
      formatting: {
        fontFamily: 'Times New Roman',
        fontSizePt: 14,
        lineSpacingMultiplier: 1.5,
        firstLineIndentCm: 1.25,
        marginLeftMm: 30,
      },
    });
    const xml = await readDocumentXml(buffer);
    const stylesXml = await (
      await JSZip.loadAsync(buffer)
    )
      .file('word/styles.xml')
      ?.async('string');

    // 1,25 см = 12.5мм ≈ 708 твипов (1мм ≈ 56.6929 твипа) — docx округляет до целого.
    expect(xml).toMatch(/<w:ind[^>]*w:firstLine="70[0-9]"/);
    // 240 * 1.5 = 360 (полуторный интервал в 240-х долях строки).
    expect(xml).toMatch(/<w:spacing[^>]*w:line="360"[^>]*w:lineRule="auto"/);
    // Шрифт/размер по умолчанию документа — в word/styles.xml (w:docDefaults), не в document.xml.
    expect(stylesXml).toContain('Times New Roman');
    expect(stylesXml).toMatch(/w:sz w:val="28"/); // 14pt → 28 полупунктов.
  });

  it('не требует marginTopMm/marginBottomMm/marginRightMm, если задан только marginLeftMm', async () => {
    const buffer = await buildDocumentBuffer({
      title: 'Заявление',
      recipientLines: [],
      bodyParagraphs: ['Единственный пункт документа.'],
      formatting: { marginLeftMm: 30 },
    });

    expect(buffer.subarray(0, 2).toString('latin1')).toBe('PK');
  });
});
