import type {
  AssistantChatMessage,
  AssistantReplyKind,
  ChatAttachment,
  ChatMessage,
  MessageVote,
  OutgoingChatRequest,
  UserChatMessage,
} from "./types";
import { looksLikeDocumentRequest } from "./documentIntent";

export interface ChatState {
  messages: ChatMessage[];
  draft: string;
  attachment: ChatAttachment | null;
  isRecording: boolean;
  isAssistantTyping: boolean;
  pendingReplyKind: AssistantReplyKind | null;
  expandedSourceMessageIds: ReadonlySet<string>;
}

export const initialChatState: ChatState = {
  messages: [],
  draft: "",
  attachment: null,
  isRecording: false,
  isAssistantTyping: false,
  pendingReplyKind: null,
  expandedSourceMessageIds: new Set(),
};

export type ChatAction =
  | { type: "SEND_MESSAGE"; request: OutgoingChatRequest }
  | { type: "RECEIVE_ASSISTANT_REPLY"; id: string; createdAt: number }
  | { type: "SET_DRAFT"; text: string }
  | { type: "ATTACH_FILE"; attachment: ChatAttachment }
  | { type: "REMOVE_ATTACHMENT" }
  | { type: "START_RECORDING" }
  | { type: "STOP_RECORDING"; transcript: string }
  | { type: "TOGGLE_SOURCES"; messageId: string }
  | { type: "VOTE"; messageId: string; vote: NonNullable<MessageVote> };

/**
 * Что вернёт "ассистент" на этот запрос. Вложение файла — надёжный сигнал
 * (contract-review). "Нужен документ" явного toggle не имеет (снят решением
 * от 2026-09-20 — на реальном бэке это определяется по контексту, Stage 6);
 * пока бэка нет — мок-эвристика по ключевым словам в тексте, см. documentIntent.ts.
 */
function pickReplyKind(request: OutgoingChatRequest): AssistantReplyKind {
  if (looksLikeDocumentRequest(request.text)) return "document";
  if (request.attachment) return "contract-review";
  return "qa";
}

export function chatReducer(state: ChatState, action: ChatAction): ChatState {
  switch (action.type) {
    case "SEND_MESSAGE": {
      const userMessage: UserChatMessage = {
        id: crypto.randomUUID(),
        role: "user",
        text: action.request.text,
        attachment: action.request.attachment,
        createdAt: Date.now(),
      };
      return {
        ...state,
        messages: [...state.messages, userMessage],
        draft: "",
        attachment: null,
        isAssistantTyping: true,
        pendingReplyKind: pickReplyKind(action.request),
      };
    }

    case "RECEIVE_ASSISTANT_REPLY": {
      if (!state.pendingReplyKind) return state;
      const assistantMessage: AssistantChatMessage = {
        id: action.id,
        role: "assistant",
        replyKind: state.pendingReplyKind,
        vote: null,
        createdAt: action.createdAt,
      };
      return {
        ...state,
        messages: [...state.messages, assistantMessage],
        isAssistantTyping: false,
        pendingReplyKind: null,
      };
    }

    case "SET_DRAFT":
      return { ...state, draft: action.text };

    case "ATTACH_FILE":
      return { ...state, attachment: action.attachment };

    case "REMOVE_ATTACHMENT":
      return { ...state, attachment: null };

    case "START_RECORDING":
      return { ...state, isRecording: true };

    case "STOP_RECORDING":
      return {
        ...state,
        isRecording: false,
        draft: state.draft ? `${state.draft} ${action.transcript}` : action.transcript,
      };

    case "TOGGLE_SOURCES": {
      const next = new Set(state.expandedSourceMessageIds);
      if (next.has(action.messageId)) next.delete(action.messageId);
      else next.add(action.messageId);
      return { ...state, expandedSourceMessageIds: next };
    }

    case "VOTE": {
      const messages = state.messages.map((message) =>
        message.role === "assistant" && message.id === action.messageId
          ? { ...message, vote: message.vote === action.vote ? null : action.vote }
          : message,
      );
      return { ...state, messages };
    }

    default: {
      const exhaustiveCheck: never = action;
      return exhaustiveCheck;
    }
  }
}
