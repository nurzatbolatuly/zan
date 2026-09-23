import { AttachmentChip } from "./AttachmentChip";
import type { ChatMessage } from "../types";

/** Сообщение пользователя: приложенные файлы (карточки) и текст вопроса. */
export function UserMessageBubble({ message }: { message: ChatMessage }) {
  if (!message.text && message.attachments.length === 0) return null;
  return (
    <div className="ml-auto flex max-w-[85%] flex-col items-end gap-2">
      {message.attachments.map((attachment) => (
        <AttachmentChip
          key={attachment.file_id}
          name={attachment.original_name}
          mimeType={attachment.mime_type}
          sizeBytes={attachment.size_bytes}
        />
      ))}
      {message.text && (
        <div className="rounded-2xl rounded-br-sm border border-line bg-accent-soft px-4 py-3 text-body text-ink">
          {message.text}
        </div>
      )}
    </div>
  );
}
