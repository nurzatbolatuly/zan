import { describe, expect, it } from "vitest";
import { chatReducer, initialChatState } from "./chatReducer";
import type { ChatAttachment, OutgoingChatRequest } from "./types";

const textOnlyRequest: OutgoingChatRequest = {
  text: "Работодатель не отдаёт трудовую книжку",
  attachment: null,
};

const attachment: ChatAttachment = {
  id: "att-1",
  name: "dogovor.pdf",
  kind: "pdf",
  sizeLabel: "248 KB",
};

describe("chatReducer / SEND_MESSAGE", () => {
  it("добавляет сообщение пользователя и запускает сценарий вопрос-ответ", () => {
    const state = chatReducer(initialChatState, {
      type: "SEND_MESSAGE",
      request: textOnlyRequest,
    });

    expect(state.messages).toHaveLength(1);
    expect(state.messages[0]).toMatchObject({ role: "user", text: textOnlyRequest.text });
    expect(state.isAssistantTyping).toBe(true);
    expect(state.pendingReplyKind).toBe("qa");
    expect(state.draft).toBe("");
  });

  it("выбирает сценарий проверки документа при наличии вложения", () => {
    const state = chatReducer(initialChatState, {
      type: "SEND_MESSAGE",
      request: { ...textOnlyRequest, attachment },
    });

    expect(state.pendingReplyKind).toBe("contract-review");
  });

  it("выбирает сценарий подготовки документа по ключевым словам в тексте", () => {
    const state = chatReducer(initialChatState, {
      type: "SEND_MESSAGE",
      request: { ...textOnlyRequest, text: "Подготовьте претензию работодателю" },
    });

    expect(state.pendingReplyKind).toBe("document");
  });

  it("очищает вложение после отправки", () => {
    const withAttachment = { ...initialChatState, attachment };
    const state = chatReducer(withAttachment, {
      type: "SEND_MESSAGE",
      request: { ...textOnlyRequest, attachment },
    });

    expect(state.attachment).toBeNull();
  });
});

describe("chatReducer / RECEIVE_ASSISTANT_REPLY", () => {
  it("добавляет ответ ассистента и снимает индикатор набора", () => {
    const pending = chatReducer(initialChatState, {
      type: "SEND_MESSAGE",
      request: textOnlyRequest,
    });

    const state = chatReducer(pending, {
      type: "RECEIVE_ASSISTANT_REPLY",
      id: "assist-1",
      createdAt: 123,
    });

    expect(state.isAssistantTyping).toBe(false);
    expect(state.pendingReplyKind).toBeNull();
    expect(state.messages).toHaveLength(2);
    expect(state.messages[1]).toMatchObject({
      role: "assistant",
      replyKind: "qa",
      vote: null,
    });
  });

  it("ничего не делает, если ответа не ждали", () => {
    const state = chatReducer(initialChatState, {
      type: "RECEIVE_ASSISTANT_REPLY",
      id: "assist-1",
      createdAt: 123,
    });

    expect(state).toBe(initialChatState);
  });
});

describe("chatReducer / вложение", () => {
  it("ATTACH_FILE сохраняет вложение", () => {
    const state = chatReducer(initialChatState, { type: "ATTACH_FILE", attachment });
    expect(state.attachment).toEqual(attachment);
  });

  it("REMOVE_ATTACHMENT убирает вложение", () => {
    const withAttachment = { ...initialChatState, attachment };
    const state = chatReducer(withAttachment, { type: "REMOVE_ATTACHMENT" });
    expect(state.attachment).toBeNull();
  });
});

describe("chatReducer / STOP_RECORDING", () => {
  it("вставляет транскрипт в пустой черновик", () => {
    const state = chatReducer(initialChatState, {
      type: "STOP_RECORDING",
      transcript: "Нужна консультация",
    });
    expect(state.draft).toBe("Нужна консультация");
    expect(state.isRecording).toBe(false);
  });

  it("дописывает транскрипт через пробел к непустому черновику", () => {
    const withDraft = { ...initialChatState, draft: "Добрый день." };
    const state = chatReducer(withDraft, {
      type: "STOP_RECORDING",
      transcript: "Нужна консультация",
    });
    expect(state.draft).toBe("Добрый день. Нужна консультация");
  });
});

describe("chatReducer / TOGGLE_SOURCES", () => {
  it("открывает и закрывает источники по id сообщения", () => {
    const opened = chatReducer(initialChatState, {
      type: "TOGGLE_SOURCES",
      messageId: "msg-1",
    });
    expect(opened.expandedSourceMessageIds.has("msg-1")).toBe(true);

    const closed = chatReducer(opened, { type: "TOGGLE_SOURCES", messageId: "msg-1" });
    expect(closed.expandedSourceMessageIds.has("msg-1")).toBe(false);
  });
});

describe("chatReducer / VOTE", () => {
  it("выставляет голос ассистентскому сообщению", () => {
    const pending = chatReducer(initialChatState, {
      type: "SEND_MESSAGE",
      request: textOnlyRequest,
    });
    const withReply = chatReducer(pending, {
      type: "RECEIVE_ASSISTANT_REPLY",
      id: "assist-1",
      createdAt: 1,
    });

    const voted = chatReducer(withReply, {
      type: "VOTE",
      messageId: "assist-1",
      vote: "up",
    });
    expect(voted.messages[1]).toMatchObject({ vote: "up" });

    const toggledOff = chatReducer(voted, {
      type: "VOTE",
      messageId: "assist-1",
      vote: "up",
    });
    expect(toggledOff.messages[1]).toMatchObject({ vote: null });
  });
});
