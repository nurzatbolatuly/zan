import { formatFileSize } from "@/shared/lib/format";
import type { AttachmentKind, ChatAttachment } from "./types";

/** Расширить карточку вложения типом файла (gap из PLAN.md §2, brief 3.2). */
export function detectAttachmentKind(file: File): AttachmentKind {
  const name = file.name.toLowerCase();
  if (file.type === "application/pdf" || name.endsWith(".pdf")) return "pdf";
  if (file.type.includes("word") || name.endsWith(".doc") || name.endsWith(".docx")) {
    return "docx";
  }
  if (file.type.startsWith("image/")) return "image";
  return "other";
}

export function createAttachmentFromFile(file: File): ChatAttachment {
  return {
    id: crypto.randomUUID(),
    name: file.name,
    kind: detectAttachmentKind(file),
    sizeLabel: formatFileSize(file.size),
  };
}
