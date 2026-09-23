import { describe, expect, it } from "vitest";
import {
  createAttachmentFromFile,
  detectAttachmentKind,
  toMessageAttachment,
} from "./attachments";

function makeFile(name: string, type: string, sizeBytes: number): File {
  return new File([new Uint8Array(sizeBytes)], name, { type });
}

describe("detectAttachmentKind", () => {
  it("распознаёт PDF по MIME", () => {
    expect(detectAttachmentKind("dogovor.pdf", "application/pdf")).toBe("pdf");
  });

  it("распознаёт PDF по расширению, если MIME пуст", () => {
    expect(detectAttachmentKind("dogovor.PDF", "")).toBe("pdf");
  });

  it("распознаёт DOCX по MIME", () => {
    const type =
      "application/vnd.openxmlformats-officedocument.wordprocessingml.document";
    expect(detectAttachmentKind("dogovor.docx", type)).toBe("docx");
  });

  it("распознаёт изображение по MIME", () => {
    expect(detectAttachmentKind("photo.jpg", "image/jpeg")).toBe("image");
  });

  it("возвращает other для нераспознанного типа", () => {
    expect(detectAttachmentKind("data.zip", "application/zip")).toBe("other");
  });
});

describe("createAttachmentFromFile", () => {
  it("собирает вложение из файла", () => {
    const attachment = createAttachmentFromFile(
      makeFile("dogovor-arenda.pdf", "application/pdf", 253952),
    );
    expect(attachment.status).toBe("uploading");
    expect(attachment.name).toBe("dogovor-arenda.pdf");
    expect(attachment.mimeType).toBe("application/pdf");
    expect(attachment.sizeBytes).toBe(253952);
    expect(attachment.id).toEqual(expect.any(String));
  });
});

describe("toMessageAttachment", () => {
  it("переводит загруженное вложение в wire-форму сообщения", () => {
    expect(
      toMessageAttachment({
        status: "ready",
        id: "a1",
        fileId: "file-1",
        name: "dogovor.pdf",
        mimeType: "application/pdf",
        sizeBytes: 2048,
      }),
    ).toEqual({
      file_id: "file-1",
      original_name: "dogovor.pdf",
      mime_type: "application/pdf",
      size_bytes: 2048,
    });
  });
});
