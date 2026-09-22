import { describe, expect, it } from "vitest";
import { createAttachmentFromFile, detectAttachmentKind } from "./attachments";

function makeFile(name: string, type: string, sizeBytes: number): File {
  return new File([new Uint8Array(sizeBytes)], name, { type });
}

describe("detectAttachmentKind", () => {
  it("распознаёт PDF по MIME", () => {
    expect(detectAttachmentKind(makeFile("dogovor.pdf", "application/pdf", 10))).toBe(
      "pdf",
    );
  });

  it("распознаёт PDF по расширению, если MIME пуст", () => {
    expect(detectAttachmentKind(makeFile("dogovor.PDF", "", 10))).toBe("pdf");
  });

  it("распознаёт DOCX по MIME", () => {
    const type =
      "application/vnd.openxmlformats-officedocument.wordprocessingml.document";
    expect(detectAttachmentKind(makeFile("dogovor.docx", type, 10))).toBe("docx");
  });

  it("распознаёт изображение по MIME", () => {
    expect(detectAttachmentKind(makeFile("photo.jpg", "image/jpeg", 10))).toBe("image");
  });

  it("возвращает other для нераспознанного типа", () => {
    expect(detectAttachmentKind(makeFile("data.zip", "application/zip", 10))).toBe(
      "other",
    );
  });
});

describe("createAttachmentFromFile", () => {
  it("собирает вложение с читаемым размером", () => {
    const attachment = createAttachmentFromFile(
      makeFile("dogovor-arenda.pdf", "application/pdf", 253952),
    );
    expect(attachment.name).toBe("dogovor-arenda.pdf");
    expect(attachment.kind).toBe("pdf");
    expect(attachment.sizeLabel).toBe("248 KB");
    expect(attachment.id).toEqual(expect.any(String));
  });
});
