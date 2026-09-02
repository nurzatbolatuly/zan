import {
  AlignmentType,
  convertMillimetersToTwip,
  Document,
  Packer,
  Paragraph,
  TextRun,
} from 'docx';

export interface DocumentTemplateInput {
  title: string;
  /** Шапка справа сверху — кому адресован документ, от кого и т.д. */
  recipientLines: string[];
  /** Пункты основного текста — нумеруются автоматически шаблонизатором. */
  bodyParagraphs: string[];
}

const A4_WIDTH_MM = 210;
const A4_HEIGHT_MM = 297;
const MARGIN_TOP_MM = 20;
const MARGIN_BOTTOM_MM = 20;
const MARGIN_LEFT_MM = 30;
const MARGIN_RIGHT_MM = 15;

const FONT = 'Times New Roman';
/** docx использует полупункты: 14pt = 28. */
const FONT_SIZE_HALF_POINTS = 28;

/**
 * Формальные требования РК к оформлению документов (A4, Times New Roman 14, шапка справа
 * сверху, нумерация пунктов) реализованы здесь программно, а не текстовой просьбой к LLM —
 * так соответствие формату гарантировано независимо от того, что вернула модель (см.
 * "Технологический стек" в ../../../../INSTRUCTIONS.md). LLM отвечает только за содержание:
 * заголовок, шапку и текст пунктов, переданные сюда через DocumentTemplateInput.
 */
export async function buildDocumentBuffer(input: DocumentTemplateInput): Promise<Buffer> {
  const headerParagraphs = input.recipientLines.map(
    (line) => new Paragraph({ alignment: AlignmentType.RIGHT, children: [new TextRun(line)] }),
  );

  const titleParagraph = new Paragraph({
    alignment: AlignmentType.CENTER,
    spacing: { before: 400, after: 400 },
    children: [new TextRun({ text: input.title.toUpperCase(), bold: true })],
  });

  const bodyParagraphs = input.bodyParagraphs.map(
    (text, index) =>
      new Paragraph({
        alignment: AlignmentType.JUSTIFIED,
        spacing: { after: 200 },
        children: [new TextRun(`${index + 1}. ${text}`)],
      }),
  );

  const footerParagraphs = [
    new Paragraph({
      spacing: { before: 600 },
      children: [new TextRun('Дата: «___» ____________ ______ г.')],
    }),
    new Paragraph({
      spacing: { before: 400 },
      children: [new TextRun('Подпись: _____________ /_____________/')],
    }),
  ];

  const document = new Document({
    styles: {
      default: {
        document: {
          run: { font: FONT, size: FONT_SIZE_HALF_POINTS },
        },
      },
    },
    sections: [
      {
        properties: {
          page: {
            size: {
              width: convertMillimetersToTwip(A4_WIDTH_MM),
              height: convertMillimetersToTwip(A4_HEIGHT_MM),
            },
            margin: {
              top: convertMillimetersToTwip(MARGIN_TOP_MM),
              bottom: convertMillimetersToTwip(MARGIN_BOTTOM_MM),
              left: convertMillimetersToTwip(MARGIN_LEFT_MM),
              right: convertMillimetersToTwip(MARGIN_RIGHT_MM),
            },
          },
        },
        children: [...headerParagraphs, titleParagraph, ...bodyParagraphs, ...footerParagraphs],
      },
    ],
  });

  return Packer.toBuffer(document);
}
