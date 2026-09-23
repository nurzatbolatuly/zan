/**
 * Домен чата (Stage 6). Сообщения треда больше не хранятся в клиентском
 * состоянии — источник истины `useQuery(["thread", id])` (см. useChatThread.ts),
 * здесь остаётся только то, что действительно клиентское: черновик,
 * стейджинг вложения/записи голоса до отправки, локально развёрнутые
 * источники (UI-состояние, не серверное).
 */

export type AttachmentKind = "pdf" | "docx" | "image" | "other";

/**
 * Вложение проходит три стадии перед тем, как попасть в `file_ids` запроса:
 * `uploading` (идёт `POST /files/upload`) → `ready` (есть `fileId`) или
 * `error` (загрузка не удалась — composer снимает вложение и показывает тост,
 * этот статус в состоянии не задерживается).
 */
export type ChatAttachment =
  | {
      status: "uploading";
      id: string;
      name: string;
      mimeType: string;
      sizeBytes: number;
    }
  | {
      status: "ready";
      id: string;
      fileId: string;
      name: string;
      mimeType: string;
      sizeBytes: number;
    };

/** Ре-экспорт wire-типа сообщения — компоненты чата рендерят его напрямую, без адаптации. */
export type { MessageDto as ChatMessage } from "@/shared/types/api";

/** Откуда взят текущий текст в composer — определяет `input_type` при отправке
 * (voice, пока текст не тронут руками после распознавания; иначе text). Вложение
 * при наличии всегда даёт `input_type: "file"` — это решает useChatThread, не этот тип. */
export type DraftSource = "text" | "voice";
