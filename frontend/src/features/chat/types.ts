/**
 * Домен чата (PLAN.md «Stage 1 — Chat»). Ассистентские сообщения не хранят
 * переведённый текст внутри себя — только `replyKind` (какой из мок-сценариев
 * сработал). Сам текст/источники/находки достаются из mocks.ts по (kind, lang)
 * в момент рендера — иначе при переключении языка старые сообщения треда
 * остались бы на предыдущем языке или пришлось бы дублировать контент на
 * оба языка внутри каждого сообщения.
 */

export type AttachmentKind = "pdf" | "docx" | "image" | "other";

export interface ChatAttachment {
  id: string;
  name: string;
  kind: AttachmentKind;
  sizeLabel: string;
}

export interface ChatSource {
  ref: string;
  quote: string;
}

export interface ChatFinding {
  title: string;
  body: string;
}

export interface ChatDocument {
  title: string;
  description: string;
}

export type MessageVote = "up" | "down" | null;

/** Какой мок-сценарий ответа выбран (gap: явный toggle "документ" + вложение файла из brief 3.1-3.2). */
export type AssistantReplyKind = "qa" | "contract-review" | "document";

export interface UserChatMessage {
  id: string;
  role: "user";
  text: string;
  attachment: ChatAttachment | null;
  createdAt: number;
}

export interface AssistantChatMessage {
  id: string;
  role: "assistant";
  replyKind: AssistantReplyKind;
  vote: MessageVote;
  createdAt: number;
}

export type ChatMessage = UserChatMessage | AssistantChatMessage;

/**
 * То, что уходит "на сервер" при отправке — реальным API станет в Stage 6.
 * Явного флага "нужен документ" здесь нет: бэк определяет это сам по контексту
 * сообщения (решение от 2026-09-20 — см. features/chat/documentIntent.ts).
 */
export interface OutgoingChatRequest {
  text: string;
  attachment: ChatAttachment | null;
}
