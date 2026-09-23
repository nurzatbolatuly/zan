import type { ChatAttachment, DraftSource } from "./types";

/**
 * Только клиентское composer-состояние (Stage 6) — сами сообщения треда
 * больше не здесь, а в `useQuery(["thread", id])` (useChatThread.ts):
 * дублировать серверные данные в reducer "для удобства" запрещено
 * (FRONT_CODING_STANDARDS.md §2). Раньше сюда же входили messages/
 * isAssistantTyping/pendingReplyKind — это было демонстрацией мок-ответа,
 * не реальным клиентским состоянием, убрано целиком вместе с моками.
 */
export interface ChatState {
  draft: string;
  draftSource: DraftSource;
  attachment: ChatAttachment | null;
  isRecording: boolean;
  expandedSourceMessageIds: ReadonlySet<string>;
}

export const initialChatState: ChatState = {
  draft: "",
  draftSource: "text",
  attachment: null,
  isRecording: false,
  expandedSourceMessageIds: new Set(),
};

export type ChatAction =
  | { type: "SET_DRAFT"; text: string }
  | { type: "SET_ATTACHMENT"; attachment: ChatAttachment | null }
  | { type: "START_RECORDING" }
  | { type: "STOP_RECORDING" }
  | { type: "VOICE_TRANSCRIBED"; text: string }
  | { type: "MESSAGE_SENT" }
  | {
      type: "SEND_FAILED";
      draft: string;
      draftSource: DraftSource;
      attachment: ChatAttachment | null;
    }
  | { type: "TOGGLE_SOURCES"; messageId: string };

export function chatReducer(state: ChatState, action: ChatAction): ChatState {
  switch (action.type) {
    case "SET_DRAFT":
      return { ...state, draft: action.text, draftSource: "text" };

    case "SET_ATTACHMENT":
      return { ...state, attachment: action.attachment };

    case "START_RECORDING":
      return { ...state, isRecording: true };

    case "STOP_RECORDING":
      return { ...state, isRecording: false };

    case "VOICE_TRANSCRIBED":
      return {
        ...state,
        draftSource: "voice",
        draft: state.draft ? `${state.draft} ${action.text}` : action.text,
      };

    // MESSAGE_SENT — сразу по нажатию "отправить" (оптимистично, до ответа
    // сервера); SEND_FAILED возвращает composer в состояние до отправки,
    // чтобы неудачный запрос не стирал набранный текст/вложение.
    case "MESSAGE_SENT":
      return { ...state, draft: "", draftSource: "text", attachment: null };

    case "SEND_FAILED":
      return {
        ...state,
        draft: action.draft,
        draftSource: action.draftSource,
        attachment: action.attachment,
      };

    case "TOGGLE_SOURCES": {
      const next = new Set(state.expandedSourceMessageIds);
      if (next.has(action.messageId)) next.delete(action.messageId);
      else next.add(action.messageId);
      return { ...state, expandedSourceMessageIds: next };
    }

    default: {
      const exhaustiveCheck: never = action;
      return exhaustiveCheck;
    }
  }
}
