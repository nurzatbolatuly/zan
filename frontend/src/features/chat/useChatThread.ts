import { useReducer, useRef, useState } from "react";
import { useSearchParams } from "react-router-dom";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useLangStore } from "@/shared/stores/useLangStore";
import { useThreadCheckout } from "@/shared/hooks/useThreadCheckout";
import { useElapsedMs } from "@/shared/hooks/useElapsedMs";
import { useToast } from "@/shared/ui";
import { logger } from "@/shared/lib/logger";
import { api } from "@/shared/lib/api";
import { describeApiError } from "@/shared/lib/apiErrorMessages";
import { getFileMaxSizeBytes } from "@/shared/lib/env";
import { formatFileSize } from "@/shared/lib/format";
import { chatReducer, initialChatState } from "./chatReducer";
import { createAttachmentFromFile, toMessageAttachment } from "./attachments";
import { chatDictionary } from "./locales";
import type { ChatDictionary } from "./locales";
import { useThreadSocket } from "./useThreadSocket";
import { resolveReplyProgressStep } from "./replyProgress";
import type { ChatMessage } from "./types";
import type { MessageFeedbackValue } from "@/shared/types/api";
import type {
  FileAttachmentDto,
  MessageInputType,
  ServiceDto,
  ThreadDetailDto,
} from "@/shared/types/api";
import type { ServiceId } from "@/shared/types/tariff";

/**
 * Вся бизнес-логика экрана чата (FRONT_CODING_STANDARDS.md §1). Тред — не
 * клиентское состояние: `useQuery(["thread", id])` (Stage 6, `GET
 * /threads/{id}`), id берётся из `?thread=` (History теперь реально ведёт
 * сюда). Composer-стейт (черновик/вложение/запись) — локальный reducer,
 * ничего из этого не дублирует сервер.
 *
 * `POST /threads`/`POST /threads/{id}/messages`/`POST /threads/{id}/resume`
 * не содержат готовый ответ ассистента — только тред сразу после перехода
 * в `processing` либо в `awaiting_payment` (баланса нет — тогда открывается
 * модалка оплаты; отказался — тред ждёт в истории, баланс пополняется в
 * «Тарифах», вопрос перезапускается кнопкой или новым сообщением). Сам ответ (статус живьём + токен-стрим текста) приходит через
 * `useThreadSocket` (`GET /ws/threads/{id}`) и реконсилируется в тот же
 * кэш `["thread", id]` — см. `useThreadSocket.ts`.
 *
 * Отправка оптимистична: сообщение пользователя видно сразу по нажатию,
 * не после ответа POST (он делает несколько запросов к БД — заметная
 * пауза). Сообщение "в полёте" — локальный `pendingMessage`, НЕ запись в
 * кэш `["thread", id]`: у нового треда id ещё нет, а серверные данные
 * остаются единственным источником истины — ответ POST целиком заменяет
 * pending (в нём уже есть это сообщение), при ошибке pending просто
 * исчезает, откатывать кэш не нужно.
 */
export function useChatThread() {
  const [state, dispatch] = useReducer(chatReducer, initialChatState);
  const lang = useLangStore((s) => s.lang);
  const t = chatDictionary[lang];
  const toast = useToast();
  const [searchParams, setSearchParams] = useSearchParams();
  const { payForThread } = useThreadCheckout();
  const queryClient = useQueryClient();

  const threadId = searchParams.get("thread");

  const threadQuery = useQuery({
    queryKey: ["thread", threadId],
    queryFn: () => api.get<ThreadDetailDto>(`/threads/${threadId}`),
    enabled: threadId !== null,
  });
  const servicesQuery = useQuery({
    queryKey: ["services"],
    queryFn: () => api.get<ServiceDto[]>("/services"),
  });

  const [pendingMessage, setPendingMessage] = useState<ChatMessage | null>(null);
  const isSending = pendingMessage !== null;
  const [isResuming, setIsResuming] = useState(false);

  const mediaRecorderRef = useRef<MediaRecorder | null>(null);
  const mediaStreamRef = useRef<MediaStream | null>(null);
  const recordedChunksRef = useRef<Blob[]>([]);

  const thread = threadQuery.data ?? null;
  const { streamingText, isProcessing } = useThreadSocket(threadId);
  const streamingMessageId = streamingText !== null ? "streaming" : null;
  const messages = pendingMessage
    ? [...(thread?.messages ?? []), pendingMessage]
    : (thread?.messages ?? []);
  // Ожидание ответа — сразу по отправке, не только с WS-события processing
  // (оно приходит лишь после ответа POST), и до первого токена стрима.
  const isAwaitingReply = (isProcessing || isSending) && streamingText === null;
  const replyElapsedMs = useElapsedMs(isAwaitingReply);
  const replyProgress = isAwaitingReply
    ? resolveReplyProgressStep({
        isSending,
        hasFile:
          messages.filter((m) => m.sender === "user").at(-1)?.input_type === "file",
        elapsedMs: replyElapsedMs,
      })
    : null;
  const canSend =
    (state.draft.trim().length > 0 || state.attachment?.status === "ready") &&
    state.attachment?.status !== "uploading" &&
    !isProcessing;

  function openThreadPayment(id: string, serviceId: ServiceId) {
    const amountTenge = servicesQuery.data?.find((s) => s.id === serviceId)?.price ?? 0;
    logger.info({
      scope: "chat.payment",
      event: "payment_modal_opened",
      data: { threadId: id, serviceId },
    });
    payForThread({
      threadId: id,
      serviceId,
      amountTenge,
      copy: {
        title: t.payTitleConsultation,
        description: t.payDescriptionConsultation,
        amountField: t.payAmountField,
        confirm: t.payConfirm,
        cancel: t.payCancel,
        note: t.payNote,
      },
    });
  }

  async function sendMessage(): Promise<void> {
    if (!canSend || isSending) return;
    const text = state.draft.trim();
    const readyAttachments =
      state.attachment?.status === "ready" ? [state.attachment] : [];
    const fileIds = readyAttachments.map((attachment) => attachment.fileId);
    const inputType: MessageInputType =
      fileIds.length > 0 ? "file" : state.draftSource === "voice" ? "voice" : "text";

    const composerSnapshot = {
      draft: state.draft,
      draftSource: state.draftSource,
      attachment: state.attachment,
    };
    setPendingMessage({
      id: "pending",
      sender: "user",
      input_type: inputType,
      text,
      feedback: null,
      processing_time_ms: null,
      created_at: new Date().toISOString(),
      attachments: readyAttachments.map(toMessageAttachment),
    });
    dispatch({ type: "MESSAGE_SENT" });
    try {
      if (!threadId) {
        const created = await api.post<ThreadDetailDto>("/threads", {
          service_id: "qa",
          text,
          input_type: inputType,
          file_ids: fileIds.length > 0 ? fileIds : undefined,
        });
        logger.info({
          scope: "chat.composer",
          event: "thread_created",
          data: { threadId: created.id, status: created.status },
        });
        queryClient.setQueryData(["thread", created.id], created);
        // Создание треда списывает кредит на сервере (или нет — баланс 0,
        // тред ждёт оплаты): баланс в шапке перечитывается в любом случае.
        void queryClient.invalidateQueries({ queryKey: ["balance"] });
        setSearchParams({ thread: created.id });
        if (created.status === "awaiting_payment")
          openThreadPayment(created.id, created.service_id);
      } else {
        const updated = await api.post<ThreadDetailDto>(`/threads/${threadId}/messages`, {
          text,
          input_type: inputType,
          file_ids: fileIds.length > 0 ? fileIds : undefined,
        });
        logger.info({
          scope: "chat.composer",
          event: "message_sent",
          data: { threadId },
        });
        queryClient.setQueryData(["thread", threadId], updated);
        // Новый вопрос в неоплаченном треде списывает кредит с баланса.
        void queryClient.invalidateQueries({ queryKey: ["balance"] });
        if (updated.status === "awaiting_payment")
          openThreadPayment(threadId, updated.service_id);
      }
    } catch (error) {
      dispatch({ type: "SEND_FAILED", ...composerSnapshot });
      logger.error({ scope: "chat.composer", event: "send_failed", error });
      toast(describeApiError(error, lang), "error");
    } finally {
      setPendingMessage(null);
    }
  }

  /** «Перезапустить вопрос» неоплаченного треда — после пополнения баланса в «Тарифах». */
  async function resumeQuestion(): Promise<void> {
    if (!threadId || isResuming) return;
    setIsResuming(true);
    try {
      const resumed = await api.post<ThreadDetailDto>(`/threads/${threadId}/resume`);
      logger.info({
        scope: "chat.thread",
        event: "question_resumed",
        data: { threadId, status: resumed.status },
      });
      queryClient.setQueryData(["thread", threadId], resumed);
      void queryClient.invalidateQueries({ queryKey: ["balance"] });
      if (resumed.status === "awaiting_payment")
        openThreadPayment(threadId, resumed.service_id);
    } catch (error) {
      logger.error({ scope: "chat.thread", event: "resume_failed", error });
      toast(describeApiError(error, lang), "error");
    } finally {
      setIsResuming(false);
    }
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
    const maxSizeBytes = getFileMaxSizeBytes();
    if (file.size > maxSizeBytes) {
      logger.warn({
        scope: "chat.composer",
        event: "file_too_large",
        data: { sizeBytes: file.size, maxSizeBytes },
      });
      toast(t.fileTooLarge(formatFileSize(maxSizeBytes)), "error");
      return;
    }

    const staged = createAttachmentFromFile(file);
    dispatch({ type: "SET_ATTACHMENT", attachment: staged });
    try {
      const formData = new FormData();
      formData.append("file", file);
      const uploaded = await api.post<FileAttachmentDto>("/files/upload", formData);
      logger.info({
        scope: "chat.composer",
        event: "file_attached",
        data: { mimeType: staged.mimeType, sizeBytes: staged.sizeBytes },
      });
      dispatch({
        type: "SET_ATTACHMENT",
        attachment: { ...staged, status: "ready", fileId: uploaded.file_id },
      });
    } catch (error) {
      logger.error({ scope: "chat.composer", event: "file_upload_failed", error });
      dispatch({ type: "SET_ATTACHMENT", attachment: null });
      toast(describeApiError(error, lang), "error");
    }
  }

  function removeAttachment(): void {
    logger.info({ scope: "chat.composer", event: "file_removed" });
    dispatch({ type: "SET_ATTACHMENT", attachment: null });
  }

  async function startRecording(): Promise<void> {
    if (!navigator.mediaDevices?.getUserMedia) return;
    try {
      const stream = await navigator.mediaDevices.getUserMedia({ audio: true });
      const recorder = new MediaRecorder(stream);
      recordedChunksRef.current = [];
      recorder.addEventListener("dataavailable", (event) => {
        if (event.data.size > 0) recordedChunksRef.current.push(event.data);
      });
      recorder.start();
      mediaRecorderRef.current = recorder;
      mediaStreamRef.current = stream;
      logger.info({ scope: "chat.composer", event: "recording_started" });
      dispatch({ type: "START_RECORDING" });
    } catch (error) {
      logger.warn({
        scope: "chat.composer",
        event: "microphone_permission_denied",
        data: { error: String(error) },
      });
    }
  }

  async function stopRecording(): Promise<void> {
    const recorder = mediaRecorderRef.current;
    if (!recorder) return;

    const blob = await new Promise<Blob>((resolve) => {
      recorder.addEventListener(
        "stop",
        () => resolve(new Blob(recordedChunksRef.current, { type: recorder.mimeType })),
        { once: true },
      );
      recorder.stop();
    });
    mediaStreamRef.current?.getTracks().forEach((track) => track.stop());
    mediaRecorderRef.current = null;
    logger.info({ scope: "chat.composer", event: "recording_stopped" });
    dispatch({ type: "STOP_RECORDING" });

    try {
      const formData = new FormData();
      formData.append("audio", blob, "voice.webm");
      const { text } = await api.post<{ text: string }>("/voice/transcribe", formData);
      dispatch({ type: "VOICE_TRANSCRIBED", text });
    } catch (error) {
      logger.error({ scope: "chat.composer", event: "voice_transcribe_failed", error });
      toast(describeApiError(error, lang), "error");
    }
  }

  function toggleSources(messageId: string): void {
    dispatch({ type: "TOGGLE_SOURCES", messageId });
  }

  async function voteMessage(
    messageId: string,
    vote: MessageFeedbackValue,
  ): Promise<void> {
    if (!threadId) return;
    try {
      await api.post(`/messages/${messageId}/feedback`, { value: vote });
      logger.info({ scope: "chat.thread", event: "message_voted", data: { vote } });
      await queryClient.invalidateQueries({ queryKey: ["thread", threadId] });
    } catch (error) {
      logger.error({ scope: "chat.thread", event: "vote_failed", error });
      toast(describeApiError(error, lang), "error");
    }
  }

  return {
    threadId,
    thread,
    messages,
    replyProgress,
    isLoadingThread: threadQuery.isLoading,
    isThreadError: threadQuery.isError,
    retryThread: () => void threadQuery.refetch(),
    isAwaitingPayment: thread?.status === "awaiting_payment",
    statusNotice: thread ? resolveStatusNotice(thread, t) : null,
    resumeQuestion,
    isResuming,
    streamingText,
    streamingMessageId,
    draft: state.draft,
    attachment: state.attachment,
    isRecording: state.isRecording,
    setDraft,
    applyQuickTopic,
    attachFile,
    removeAttachment,
    startRecording,
    stopRecording,
    canSend,
    isSending,
    sendMessage,
    expandedSourceMessageIds: state.expandedSourceMessageIds,
    toggleSources,
    voteMessage,
  };
}

/** Подпись под тредом, когда продолжить его обычным образом нельзя или нужно действие. */
function resolveStatusNotice(thread: ThreadDetailDto, t: ChatDictionary): string | null {
  switch (thread.status) {
    case "error":
      return t.threadErrorNotice;
    case "canceled":
      return t.threadCanceledNotice;
    case "awaiting_payment":
      return t.awaitingPaymentNotice;
    default:
      return null;
  }
}
