import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useSessionStore } from "@/shared/stores/useSessionStore";

/**
 * jsdom не даёт реального WebSocket-сервера — подменяем глобальный
 * конструктор фейковым классом, управляемым руками из теста (та же идея,
 * что и `vi.mock("@/shared/lib/api")` для fetch — контролируемый двойник
 * вместо реальной сети, FRONT_CODING_STANDARDS.md §... тестирование).
 */
class FakeWebSocket {
  static instances: FakeWebSocket[] = [];
  url: string;
  onopen: (() => void) | null = null;
  onclose: (() => void) | null = null;
  onerror: (() => void) | null = null;
  onmessage: ((event: { data: string }) => void) | null = null;
  closeCalls = 0;

  constructor(url: string) {
    this.url = url;
    FakeWebSocket.instances.push(this);
  }

  close(): void {
    this.closeCalls += 1;
    this.onclose?.();
  }

  emitOpen(): void {
    this.onopen?.();
  }

  emitMessage(data: unknown): void {
    this.onmessage?.({ data: JSON.stringify(data) });
  }
}

beforeEach(() => {
  FakeWebSocket.instances = [];
  vi.stubGlobal("WebSocket", FakeWebSocket);
  useSessionStore.setState({ sessionId: "s1", sessionToken: "tok-1" });
});

afterEach(() => {
  vi.unstubAllGlobals();
  vi.useRealTimers();
});

describe("openThreadSocket", () => {
  it("открывает соединение на /ws/threads/{id} с токеном в query", async () => {
    const { openThreadSocket } = await import("./ws");
    openThreadSocket("t1", { onEvent: vi.fn() });

    expect(FakeWebSocket.instances).toHaveLength(1);
    expect(FakeWebSocket.instances[0]!.url).toContain("/ws/threads/t1?token=tok-1");
  });

  it("диспатчит события по type в onEvent", async () => {
    const { openThreadSocket } = await import("./ws");
    const onEvent = vi.fn();
    openThreadSocket("t1", { onEvent });
    const socket = FakeWebSocket.instances[0]!;

    socket.emitMessage({
      type: "thread_status",
      thread_id: "t1",
      seq: 1,
      status: "processing",
      preview_text: "...",
    });
    socket.emitMessage({
      type: "answer_delta",
      thread_id: "t1",
      seq: 2,
      delta: "Привет",
    });
    socket.emitMessage({
      type: "answer_done",
      thread_id: "t1",
      seq: 3,
      status: "done",
      message: {
        id: "m1",
        sender: "assistant",
        input_type: "text",
        text: "Привет",
        feedback: null,
        processing_time_ms: 100,
        created_at: new Date().toISOString(),
        attachments: [],
      },
    });

    expect(onEvent).toHaveBeenCalledTimes(3);
    expect(onEvent.mock.calls[0]![0]).toMatchObject({
      type: "thread_status",
      status: "processing",
    });
    expect(onEvent.mock.calls[1]![0]).toMatchObject({
      type: "answer_delta",
      delta: "Привет",
    });
    expect(onEvent.mock.calls[2]![0]).toMatchObject({
      type: "answer_done",
      status: "done",
    });
  });

  it("close() реально закрывает сокет и не переподключается", async () => {
    vi.useFakeTimers();
    const { openThreadSocket } = await import("./ws");
    const handle = openThreadSocket("t1", { onEvent: vi.fn() });
    const socket = FakeWebSocket.instances[0]!;

    handle.close();

    expect(socket.closeCalls).toBe(1);
    await vi.advanceTimersByTimeAsync(10_000);
    expect(FakeWebSocket.instances).toHaveLength(1); // не создал новый сокет — реконнект не сработал после явного close()
  });

  it("переподключается с backoff при обрыве соединения не по инициативе клиента", async () => {
    vi.useFakeTimers();
    const { openThreadSocket } = await import("./ws");
    openThreadSocket("t1", { onEvent: vi.fn() });
    const first = FakeWebSocket.instances[0]!;

    first.onclose?.(); // сервер разорвал соединение сам (не handle.close())
    await vi.advanceTimersByTimeAsync(1000);

    expect(FakeWebSocket.instances.length).toBeGreaterThan(1);
  });
});
