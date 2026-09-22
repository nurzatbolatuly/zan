import { useEffect, useReducer } from "react";
import { useLangStore } from "@/shared/stores/useLangStore";
import { usePaymentModalStore } from "@/shared/stores/usePaymentModalStore";
import { logger } from "@/shared/lib/logger";
import { formatTenge } from "@/shared/lib/format";
import { chatReducer, initialChatState } from "./chatReducer";
import { createAttachmentFromFile } from "./attachments";
import { looksLikeDocumentRequest } from "./documentIntent";
import { chatDictionary } from "./locales";
import {
  CONSULTATION_PRICE_TENGE,
  DOCUMENT_PRICE_TENGE,
  VOICE_DEMO_TRANSCRIPT,
} from "./mocks";
import type { MessageVote } from "./types";

// Пауза перед мок-ответом ассистента — только для ощущения "печатает", не реальная задержка сети.
const ASSISTANT_REPLY_DELAY_MS = 700;

/**
 * Вся бизнес-логика экрана чата (FRONT_CODING_STANDARDS.md §1 — "логика в
 * хуках, компонент только рендерит"). Состояние — локальное для страницы
 * (useReducer), не Zustand: тред не нужен нигде за пределами ChatPage, пока
 * не появится реальный бэкенд (Stage 6) с TanStack Query.
 */
export function useChatThread() {
  const [state, dispatch] = useReducer(chatReducer, initialChatState);
  const lang = useLangStore((s) => s.lang);
  const t = chatDictionary[lang];

  useEffect(() => {
    if (!state.pendingReplyKind) return;
    const timeoutId = window.setTimeout(() => {
      logger.info({
        scope: "chat.thread",
        event: "assistant_reply_received",
        data: { replyKind: state.pendingReplyKind },
      });
      dispatch({
        type: "RECEIVE_ASSISTANT_REPLY",
        id: crypto.randomUUID(),
        createdAt: Date.now(),
      });
    }, ASSISTANT_REPLY_DELAY_MS);
    return () => window.clearTimeout(timeoutId);
  }, [state.pendingReplyKind]);

  const canSend = state.draft.trim().length > 0 || state.attachment !== null;

  function sendMessage(): void {
    if (!canSend) return;
    const request = { text: state.draft.trim(), attachment: state.attachment };
    const isFirstMessage = state.messages.length === 0;
    // "Нужен документ" не приходит от пользователя явно (toggle снят решением
    // от 2026-09-20) — на реальном бэке это определит контекст (Stage 6), здесь
    // только мок-эвристика по тексту, нужная сейчас лишь для цены оплаты
    // (сам сценарий мок-ответа reducer выбирает этой же функцией).
    const willPrepareDocument = looksLikeDocumentRequest(request.text);

    const dispatchSend = () => {
      logger.info({
        scope: "chat.composer",
        event: "message_sent",
        data: {
          isFirstMessage,
          willPrepareDocument,
          hasAttachment: request.attachment !== null,
        },
      });
      dispatch({ type: "SEND_MESSAGE", request });
    };

    if (!isFirstMessage) {
      dispatchSend();
      return;
    }

    // Оплата — за тред, не за сообщение (brief 3.4): спрашиваем деньги только
    // на первом сообщении нового треда, дальше — бесплатные уточнения.
    const amount = willPrepareDocument ? DOCUMENT_PRICE_TENGE : CONSULTATION_PRICE_TENGE;
    logger.info({
      scope: "chat.composer",
      event: "payment_modal_opened",
      data: { willPrepareDocument },
    });
    usePaymentModalStore.getState().open({
      title: willPrepareDocument ? t.payTitleDocument : t.payTitleConsultation,
      description: willPrepareDocument
        ? t.payDescriptionDocument
        : t.payDescriptionConsultation,
      amountLabel: formatTenge(amount),
      amountFieldLabel: t.payAmountField,
      confirmLabel: t.payConfirm,
      cancelLabel: t.payCancel,
      note: t.payNote,
      onConfirm: dispatchSend,
    });
  }

  function setDraft(text: string): void {
    dispatch({ type: "SET_DRAFT", text });
  }

  function applyQuickTopic(topicLabel: string, draftText: string): void {
    logger.info({
      scope: "chat.composer",
      event: "quick_topic_selected",
      data: { topicLabel },
    });
    dispatch({ type: "SET_DRAFT", text: draftText });
  }

  async function attachFile(file: File): Promise<void> {
    const attachment = createAttachmentFromFile(file);
    logger.info({
      scope: "chat.composer",
      event: "file_attached",
      data: { kind: attachment.kind, sizeLabel: attachment.sizeLabel },
    });
    dispatch({ type: "ATTACH_FILE", attachment });
  }

  function removeAttachment(): void {
    logger.info({ scope: "chat.composer", event: "file_removed" });
    dispatch({ type: "REMOVE_ATTACHMENT" });
  }

  async function startRecording(): Promise<void> {
    // Реального STT нет до Stage 6 (`/voice/transcribe`) — запрос разрешения
    // на микрофон нужен только для честного UX (пользователь видит системный
    // промпт, как в реальном голосовом вводе), сам поток сразу останавливается.
    if (navigator.mediaDevices?.getUserMedia) {
      try {
        const stream = await navigator.mediaDevices.getUserMedia({ audio: true });
        stream.getTracks().forEach((track) => track.stop());
      } catch (error) {
        logger.warn({
          scope: "chat.composer",
          event: "microphone_permission_denied",
          data: { error: String(error) },
        });
      }
    }
    logger.info({ scope: "chat.composer", event: "recording_started" });
    dispatch({ type: "START_RECORDING" });
  }

  function stopRecording(): void {
    logger.info({ scope: "chat.composer", event: "recording_stopped" });
    dispatch({ type: "STOP_RECORDING", transcript: VOICE_DEMO_TRANSCRIPT[lang] });
  }

  function toggleSources(messageId: string): void {
    dispatch({ type: "TOGGLE_SOURCES", messageId });
  }

  function voteMessage(messageId: string, vote: NonNullable<MessageVote>): void {
    logger.info({ scope: "chat.thread", event: "message_voted", data: { vote } });
    dispatch({ type: "VOTE", messageId, vote });
  }

  return {
    ...state,
    canSend,
    sendMessage,
    setDraft,
    applyQuickTopic,
    attachFile,
    removeAttachment,
    startRecording,
    stopRecording,
    toggleSources,
    voteMessage,
  };
}
