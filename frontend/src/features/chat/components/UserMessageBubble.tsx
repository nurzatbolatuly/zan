import { AttachmentChip } from "./AttachmentChip";
import type { UserChatMessage } from "../types";

export function UserMessageBubble({ message }: { message: UserChatMessage }) {
  return (
    <div className="ml-auto flex max-w-[85%] flex-col items-end gap-2">
      {message.attachment && <AttachmentChip attachment={message.attachment} />}
      {message.text && (
        <div className="rounded-2xl rounded-br-sm border border-line bg-accent-soft px-4 py-3 text-body text-ink">
          {message.text}
        </div>
      )}
    </div>
  );
}
