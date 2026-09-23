import { describe, expect, it } from "vitest";
import { chatReducer, initialChatState } from "./chatReducer";
import type { ChatAttachment } from "./types";

const READY_ATTACHMENT: ChatAttachment = {
  status: "ready",
  id: "a1",
  fileId: "file-1",
  name: "dogovor.pdf",
  mimeType: "application/pdf",
  sizeBytes: 253952,
};

describe("chatReducer", () => {
  it("SET_DRAFT обновляет текст и сбрасывает draftSource на text", () => {
    const withVoice = { ...initialChatState, draftSource: "voice" as const };
    const state = chatReducer(withVoice, { type: "SET_DRAFT", text: "новый текст" });
    expect(state.draft).toBe("новый текст");
    expect(state.draftSource).toBe("text");
  });

  it("SET_ATTACHMENT сохраняет вложение", () => {
    const state = chatReducer(initialChatState, {
      type: "SET_ATTACHMENT",
      attachment: READY_ATTACHMENT,
    });
    expect(state.attachment).toEqual(READY_ATTACHMENT);
  });

  it("SET_ATTACHMENT с null снимает вложение", () => {
    const withAttachment = { ...initialChatState, attachment: READY_ATTACHMENT };
    const state = chatReducer(withAttachment, {
      type: "SET_ATTACHMENT",
      attachment: null,
    });
    expect(state.attachment).toBeNull();
  });

  it("START_RECORDING/STOP_RECORDING переключают isRecording", () => {
    const recording = chatReducer(initialChatState, { type: "START_RECORDING" });
    expect(recording.isRecording).toBe(true);
    const stopped = chatReducer(recording, { type: "STOP_RECORDING" });
    expect(stopped.isRecording).toBe(false);
  });

  it("VOICE_TRANSCRIBED дописывает текст и помечает источник voice", () => {
    const state = chatReducer(initialChatState, {
      type: "VOICE_TRANSCRIBED",
      text: "распознанный текст",
    });
    expect(state.draft).toBe("распознанный текст");
    expect(state.draftSource).toBe("voice");
  });

  it("VOICE_TRANSCRIBED дописывает через пробел к уже введённому черновику", () => {
    const withDraft = { ...initialChatState, draft: "у меня вопрос" };
    const state = chatReducer(withDraft, {
      type: "VOICE_TRANSCRIBED",
      text: "про аренду",
    });
    expect(state.draft).toBe("у меня вопрос про аренду");
  });

  it("MESSAGE_SENT очищает черновик, источник и вложение", () => {
    const filled = {
      ...initialChatState,
      draft: "текст",
      draftSource: "voice" as const,
      attachment: READY_ATTACHMENT,
    };
    const state = chatReducer(filled, { type: "MESSAGE_SENT" });
    expect(state.draft).toBe("");
    expect(state.draftSource).toBe("text");
    expect(state.attachment).toBeNull();
  });

  it("SEND_FAILED возвращает черновик, источник и вложение, стёртые оптимистичной отправкой", () => {
    const sent = chatReducer(
      {
        ...initialChatState,
        draft: "текст",
        draftSource: "voice",
        attachment: READY_ATTACHMENT,
      },
      { type: "MESSAGE_SENT" },
    );
    const state = chatReducer(sent, {
      type: "SEND_FAILED",
      draft: "текст",
      draftSource: "voice",
      attachment: READY_ATTACHMENT,
    });
    expect(state.draft).toBe("текст");
    expect(state.draftSource).toBe("voice");
    expect(state.attachment).toEqual(READY_ATTACHMENT);
  });

  it("TOGGLE_SOURCES добавляет и убирает id из набора развёрнутых источников", () => {
    const opened = chatReducer(initialChatState, {
      type: "TOGGLE_SOURCES",
      messageId: "m1",
    });
    expect(opened.expandedSourceMessageIds.has("m1")).toBe(true);

    const closed = chatReducer(opened, { type: "TOGGLE_SOURCES", messageId: "m1" });
    expect(closed.expandedSourceMessageIds.has("m1")).toBe(false);
  });
});
