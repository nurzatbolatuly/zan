import type { MessageAttachmentDto } from "@/shared/types/api";
import type { AttachmentKind, ChatAttachment } from "./types";

/** Иконка/подпись вложения по типу файла — одинаково в composer и в истории сообщений. */
export function detectAttachmentKind(name: string, mimeType: string): AttachmentKind {
  const lowerName = name.toLowerCase();
  if (mimeType === "application/pdf" || lowerName.endsWith(".pdf")) return "pdf";
  if (
    mimeType.includes("word") ||
    lowerName.endsWith(".doc") ||
    lowerName.endsWith(".docx")
  ) {
    return "docx";
  }
  if (mimeType.startsWith("image/")) return "image";
  return "other";
}

/** Стартовое (`uploading`) состояние вложения — useChatThread.ts переводит его
 * в `ready`/убирает при ошибке после реального `POST /files/upload` (Stage 6). */
export function createAttachmentFromFile(file: File): ChatAttachment {
  return {
    status: "uploading",
    id: crypto.randomUUID(),
    name: file.name,
    mimeType: file.type,
    sizeBytes: file.size,
  };
}

/** Загруженное вложение в wire-форме сообщения — для оптимистичного pending-сообщения,
 * чтобы файл был виден в чате сразу, а не только после ответа сервера. */
export function toMessageAttachment(
  attachment: Extract<ChatAttachment, { status: "ready" }>,
): MessageAttachmentDto {
  return {
    file_id: attachment.fileId,
    original_name: attachment.name,
    mime_type: attachment.mimeType,
    size_bytes: attachment.sizeBytes,
  };
}
