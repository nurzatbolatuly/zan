import { afterEach, describe, expect, it, vi } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { createTestQueryClient } from "@/test/queryClient";
import type { ThreadDetailDto } from "@/shared/types/api";

vi.mock("@/shared/lib/ws", () => ({
  openThreadSocket: vi.fn(),
}));
import { openThreadSocket } from "@/shared/lib/ws";
import type { ThreadSocketEvent } from "@/shared/lib/ws";
import { useThreadSocket } from "./useThreadSocket";

function makeThread(overrides: Partial<ThreadDetailDto> = {}): ThreadDetailDto {
  return {
    id: "t1",
    service_id: "qa",
    status: "processing",
    title: "Тред",
    preview_text: "Ассистент готовит ответ…",
    message_count: 1,
    created_at: new Date().toISOString(),
    last_message_at: new Date().toISOString(),
    messages: [],
    ...overrides,
  };
}

function renderWithClient(threadId: string | null, client: QueryClient) {
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  );
  return renderHook(({ id }) => useThreadSocket(id), {
    initialProps: { id: threadId },
    wrapper,
  });
}

afterEach(() => {
  vi.mocked(openThreadSocket).mockReset();
});

describe("useThreadSocket", () => {
  it("накапливает streamingText по мере прихода answer_delta", async () => {
    let onEvent: (event: ThreadSocketEvent) => void = () => {};
    vi.mocked(openThreadSocket).mockImplementation((_id, handlers) => {
      onEvent = handlers.onEvent;
      return { close: vi.fn() };
    });

    const client = createTestQueryClient();
    const { result } = renderWithClient("t1", client);

    onEvent({
      type: "thread_status",
      thread_id: "t1",
      seq: 1,
      status: "processing",
      preview_text: "...",
    });
    await waitFor(() => expect(result.current.isProcessing).toBe(true));

    onEvent({ type: "answer_delta", thread_id: "t1", seq: 2, delta: "При" });
    onEvent({ type: "answer_delta", thread_id: "t1", seq: 3, delta: "вет" });

    await waitFor(() => expect(result.current.streamingText).toBe("Привет"));
  });

  it("answer_done чистит streamingText и добавляет сообщение в кэш треда", async () => {
    let onEvent: (event: ThreadSocketEvent) => void = () => {};
    vi.mocked(openThreadSocket).mockImplementation((_id, handlers) => {
      onEvent = handlers.onEvent;
      return { close: vi.fn() };
    });

    const client = createTestQueryClient();
    client.setQueryData(["thread", "t1"], makeThread());
    const { result } = renderWithClient("t1", client);

    onEvent({ type: "answer_delta", thread_id: "t1", seq: 1, delta: "Готовый ответ" });
    await waitFor(() => expect(result.current.streamingText).toBe("Готовый ответ"));

    onEvent({
      type: "answer_done",
      thread_id: "t1",
      seq: 2,
      status: "done",
      message: {
        id: "m1",
        sender: "assistant",
        input_type: "text",
        text: "Готовый ответ",
        feedback: null,
        processing_time_ms: 500,
        created_at: new Date().toISOString(),
        attachments: [],
      },
    });

    await waitFor(() => {
      expect(result.current.streamingText).toBeNull();
      expect(result.current.isProcessing).toBe(false);
    });
    const cached = client.getQueryData<ThreadDetailDto>(["thread", "t1"]);
    expect(cached?.status).toBe("done");
    expect(cached?.messages).toHaveLength(1);
    expect(cached?.messages[0]?.text).toBe("Готовый ответ");
  });

  it("error чистит streamingText, ставит status:error, без нового сообщения", async () => {
    let onEvent: (event: ThreadSocketEvent) => void = () => {};
    vi.mocked(openThreadSocket).mockImplementation((_id, handlers) => {
      onEvent = handlers.onEvent;
      return { close: vi.fn() };
    });

    const client = createTestQueryClient();
    client.setQueryData(["thread", "t1"], makeThread());
    const { result } = renderWithClient("t1", client);

    onEvent({ type: "answer_delta", thread_id: "t1", seq: 1, delta: "Незаконченный " });
    await waitFor(() => expect(result.current.streamingText).toBe("Незаконченный "));

    onEvent({
      type: "error",
      thread_id: "t1",
      seq: 2,
      code: "internal_error",
      error_message: "Не удалось обработать запрос",
    });

    await waitFor(() => {
      expect(result.current.streamingText).toBeNull();
      expect(result.current.isProcessing).toBe(false);
    });
    const cached = client.getQueryData<ThreadDetailDto>(["thread", "t1"]);
    expect(cached?.status).toBe("error");
    expect(cached?.messages).toHaveLength(0);
  });

  it("answer_done сверяет тред с сервером — устаревший GET не перезапишет ответ", async () => {
    let onEvent: (event: ThreadSocketEvent) => void = () => {};
    vi.mocked(openThreadSocket).mockImplementation((_id, handlers) => {
      onEvent = handlers.onEvent;
      return { close: vi.fn() };
    });
    const client = createTestQueryClient();
    client.setQueryData(["thread", "t1"], makeThread());
    const invalidateSpy = vi.spyOn(client, "invalidateQueries");
    renderWithClient("t1", client);

    onEvent({
      type: "answer_done",
      thread_id: "t1",
      seq: 1,
      status: "done",
      message: {
        id: "m1",
        sender: "assistant",
        input_type: "text",
        text: "Готовый ответ",
        feedback: null,
        processing_time_ms: 10,
        created_at: new Date().toISOString(),
        attachments: [],
      },
    });

    await waitFor(() =>
      expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ["thread", "t1"] }),
    );
  });

  it("error перечитывает баланс — сервер вернул списанный кредит", async () => {
    let onEvent: (event: ThreadSocketEvent) => void = () => {};
    vi.mocked(openThreadSocket).mockImplementation((_id, handlers) => {
      onEvent = handlers.onEvent;
      return { close: vi.fn() };
    });
    const client = createTestQueryClient();
    const invalidateSpy = vi.spyOn(client, "invalidateQueries");
    renderWithClient("t1", client);

    onEvent({
      type: "error",
      thread_id: "t1",
      seq: 1,
      code: "internal_error",
      error_message: "Не удалось обработать запрос",
    });

    await waitFor(() =>
      expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ["balance"] }),
    );
  });

  it("закрывает сокет при размонтировании", () => {
    const close = vi.fn();
    vi.mocked(openThreadSocket).mockImplementation(() => ({ close }));

    const client = createTestQueryClient();
    const { unmount } = renderWithClient("t1", client);
    unmount();

    expect(close).toHaveBeenCalledTimes(1);
  });

  it("не открывает соединение, если threadId ещё нет", () => {
    const client = createTestQueryClient();
    renderWithClient(null, client);
    expect(openThreadSocket).not.toHaveBeenCalled();
  });
});
