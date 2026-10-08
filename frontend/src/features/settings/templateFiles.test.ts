import { describe, expect, it } from "vitest";
import {
  countTemplatesByType,
  groupTemplatesByType,
  isSupportedTemplateFile,
  titleFromFileName,
} from "./templateFiles";
import type { DocumentTemplate, DocumentType } from "./types";

function makeTemplate(id: string, typeId: string): DocumentTemplate {
  return {
    id,
    typeId,
    title: id,
    originalName: `${id}.pdf`,
    fileKind: "pdf",
    sizeBytes: 1024,
    previewUrl: `https://storage/${id}`,
    updatedAt: "2026-10-06T10:00:00Z",
  };
}

const TYPES: DocumentType[] = [
  { id: "contract", name: "Договор" },
  { id: "lawsuit", name: "Исковое заявление" },
  { id: "order", name: "Приказ" },
];

describe("isSupportedTemplateFile", () => {
  it.each([
    ["lease.pdf", true],
    ["Lease.DOCX", true],
    ["lease.doc", false],
    ["lease.pdf.exe", false],
    ["scan.png", false],
  ])("%s → %s", (name, expected) => {
    expect(isSupportedTemplateFile(name)).toBe(expected);
  });
});

describe("titleFromFileName", () => {
  it("drops the extension", () => {
    expect(titleFromFileName("Договор аренды.v2.docx")).toBe("Договор аренды.v2");
  });

  it("keeps names without extension and dotfiles as is", () => {
    expect(titleFromFileName("Приказ")).toBe("Приказ");
    expect(titleFromFileName(".pdf")).toBe(".pdf");
  });
});

describe("groupTemplatesByType", () => {
  it("follows the dictionary order and skips empty types", () => {
    const templates = [
      makeTemplate("order-1", "order"),
      makeTemplate("contract-2", "contract"),
      makeTemplate("contract-1", "contract"),
    ];

    const groups = groupTemplatesByType(templates, TYPES);

    expect(groups.map((group) => group.type.id)).toEqual(["contract", "order"]);
    expect(groups[0]?.templates.map((t) => t.id)).toEqual(["contract-2", "contract-1"]);
  });
});

describe("countTemplatesByType", () => {
  it("counts templates per type", () => {
    const counts = countTemplatesByType([
      makeTemplate("a", "contract"),
      makeTemplate("b", "contract"),
      makeTemplate("c", "order"),
    ]);

    expect(counts.get("contract")).toBe(2);
    expect(counts.get("order")).toBe(1);
    expect(counts.get("lawsuit")).toBeUndefined();
  });
});
