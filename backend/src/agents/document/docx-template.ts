import {
  AlignmentType,
  convertMillimetersToTwip,
  Document,
  LineRuleType,
  Packer,
  Paragraph,
  TextRun,
  type IPageMarginAttributes,
  type IPageSizeAttributes,
  type IParagraphPropertiesOptions,
  type IRunStylePropertiesOptions,
} from 'docx';

/**
 * Ни одно из этих полей не имеет значения по умолчанию в этом файле — раньше здесь были
 * захардкожены "стандартные" A4/Times New Roman 14/красная строка 1,25 см/полуторный интервал по
 * аналогии с общим делопроизводством (ГОСТ Р 7.0.97), но эти цифры никогда не были проверены на
 * то, что их действительно требует законодательство РК для составляемых сервисом документов
 * (заявление, претензия, иск и т.п.) — а для большинства из них закон, скорее всего, вообще не
 * регламентирует визуальное оформление, только структуру/реквизиты.
 *
 * Поэтому источник этих значений — не эта функция, а Агент "document" (см. law-document.agent.ts):
 * когда у него в этом обращении есть инструмент web_search по adilet.zan.kz/zan.gov.kz, он
 * дополнительно ищет, регламентирует ли закон визуальное оформление для конкретного типа
 * документа, и заполняет здесь только то, что реально нашёл. Шаблонизатор ничего не предполагает
 * сам — не заданное поле означает "закон это не регламентирует", и docx применяет свой обычный
 * дефолт (см. комментарии у каждого поля ниже), а не выдуманное сервисом "типовое" значение.
 */
export interface DocumentFormatting {
  /** Например, "Times New Roman" — если не задано, используется дефолт docx/Word. */
  fontFamily?: string;
  /** В pt (например, 14) — если не задано, используется дефолт docx/Word. */
  fontSizePt?: number;
  /** Множитель межстрочного интервала (1 — одинарный, 1.5 — полуторный, 2 — двойной). */
  lineSpacingMultiplier?: number;
  /** Абзацный отступ первой строки ("красная строка") у пунктов основного текста, в см. */
  firstLineIndentCm?: number;
  /** Размер страницы, мм — оба поля обязательны вместе. Дефолт docx уже равен A4 (210×297мм). */
  pageWidthMm?: number;
  pageHeightMm?: number;
  marginTopMm?: number;
  marginBottomMm?: number;
  marginLeftMm?: number;
  marginRightMm?: number;
}

export interface DocumentTemplateInput {
  title: string;
  /** Шапка справа сверху — кому адресован документ, от кого и т.д. */
  recipientLines: string[];
  /** Пункты основного текста — нумеруются автоматически шаблонизатором. */
  bodyParagraphs: string[];
  /** См. DocumentFormatting — только то, что агент нашёл в законе, ничего сверх этого. */
  formatting?: DocumentFormatting;
}

function buildRunDefaults(
  formatting: DocumentFormatting | undefined,
): IRunStylePropertiesOptions | undefined {
  const font = formatting?.fontFamily;
  const size = formatting?.fontSizePt;
  if (font === undefined && size === undefined) return undefined;
  return {
    ...(font !== undefined ? { font } : {}),
    // docx использует полупункты (14pt закона → 28).
    ...(size !== undefined ? { size: Math.round(size * 2) } : {}),
  };
}

function buildLineSpacing(
  formatting: DocumentFormatting | undefined,
): { line: number; lineRule: (typeof LineRuleType)['AUTO'] } | undefined {
  const multiplier = formatting?.lineSpacingMultiplier;
  if (multiplier === undefined) return undefined;
  // OOXML `w:spacing w:line` при lineRule="auto" — в 240-х долях строки (240 = одинарный).
  return { line: Math.round(240 * multiplier), lineRule: LineRuleType.AUTO };
}

function buildFirstLineIndent(
  formatting: DocumentFormatting | undefined,
): IParagraphPropertiesOptions['indent'] {
  const cm = formatting?.firstLineIndentCm;
  if (cm === undefined) return undefined;
  return { firstLine: convertMillimetersToTwip(cm * 10) };
}

function buildPageSize(
  formatting: DocumentFormatting | undefined,
): Partial<IPageSizeAttributes> | undefined {
  const { pageWidthMm, pageHeightMm } = formatting ?? {};
  if (pageWidthMm === undefined || pageHeightMm === undefined) return undefined;
  return {
    width: convertMillimetersToTwip(pageWidthMm),
    height: convertMillimetersToTwip(pageHeightMm),
  };
}

function buildPageMargin(
  formatting: DocumentFormatting | undefined,
): IPageMarginAttributes | undefined {
  const { marginTopMm, marginBottomMm, marginLeftMm, marginRightMm } = formatting ?? {};
  if (
    marginTopMm === undefined &&
    marginBottomMm === undefined &&
    marginLeftMm === undefined &&
    marginRightMm === undefined
  ) {
    return undefined;
  }
  return {
    ...(marginTopMm !== undefined ? { top: convertMillimetersToTwip(marginTopMm) } : {}),
    ...(marginBottomMm !== undefined ? { bottom: convertMillimetersToTwip(marginBottomMm) } : {}),
    ...(marginLeftMm !== undefined ? { left: convertMillimetersToTwip(marginLeftMm) } : {}),
    ...(marginRightMm !== undefined ? { right: convertMillimetersToTwip(marginRightMm) } : {}),
  };
}

export async function buildDocumentBuffer(input: DocumentTemplateInput): Promise<Buffer> {
  const lineSpacing = buildLineSpacing(input.formatting);
  const firstLineIndent = buildFirstLineIndent(input.formatting);
  const runDefaults = buildRunDefaults(input.formatting);
  const pageSize = buildPageSize(input.formatting);
  const pageMargin = buildPageMargin(input.formatting);

  const headerParagraphs = input.recipientLines.map(
    (line) =>
      new Paragraph({
        alignment: AlignmentType.RIGHT,
        ...(lineSpacing ? { spacing: lineSpacing } : {}),
        children: [new TextRun(line)],
      }),
  );

  const titleParagraph = new Paragraph({
    alignment: AlignmentType.CENTER,
    spacing: { before: 400, after: 400, ...lineSpacing },
    children: [new TextRun({ text: input.title.toUpperCase(), bold: true })],
  });

  const bodyParagraphs = input.bodyParagraphs.map(
    (text, index) =>
      new Paragraph({
        alignment: AlignmentType.JUSTIFIED,
        ...(firstLineIndent ? { indent: firstLineIndent } : {}),
        spacing: { after: 200, ...lineSpacing },
        children: [new TextRun(`${index + 1}. ${text}`)],
      }),
  );

  const footerParagraphs = [
    new Paragraph({
      spacing: { before: 600, ...lineSpacing },
      children: [new TextRun('Дата: «___» ____________ ______ г.')],
    }),
    new Paragraph({
      spacing: { before: 400, ...lineSpacing },
      children: [new TextRun('Подпись: _____________ /_____________/')],
    }),
  ];

  const document = new Document({
    ...(runDefaults ? { styles: { default: { document: { run: runDefaults } } } } : {}),
    sections: [
      {
        ...((pageSize ?? pageMargin)
          ? {
              properties: {
                page: {
                  ...(pageSize ? { size: pageSize } : {}),
                  ...(pageMargin ? { margin: pageMargin } : {}),
                },
              },
            }
          : {}),
        children: [...headerParagraphs, titleParagraph, ...bodyParagraphs, ...footerParagraphs],
      },
    ],
  });

  return Packer.toBuffer(document);
}
