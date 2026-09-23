import { afterEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { QueryClientProvider } from "@tanstack/react-query";
import { ChatPage } from "./ChatPage";
import { PaymentModal } from "@/shared/ui/PaymentModal";
import { ToastViewport } from "@/shared/ui/toast/ToastViewport";
import { useLangStore } from "@/shared/stores/useLangStore";
import { createTestQueryClient } from "@/test/queryClient";
import type { ServiceDto, Session, ThreadDetailDto } from "@/shared/types/api";

vi.mock("@/shared/lib/api", async () => {
  const actual =
    await vi.importActual<typeof import("@/shared/lib/api")>("@/shared/lib/api");
  return {
    ...actual,
    api: { get: vi.fn(), post: vi.fn(), put: vi.fn(), patch: vi.fn(), delete: vi.fn() },
  };
});
vi.mock("@/shared/lib/ws", () => ({ openThreadSocket: vi.fn() }));
import { api } from "@/shared/lib/api";
import { openThreadSocket } from "@/shared/lib/ws";
import type { ThreadSocketHandlers } from "@/shared/lib/ws";

/**
 * POST /threads и POST /threads/{id}/messages больше не возвращают готовый
 * ответ ассистента (см. useChatThread.ts) — он приходит через WS
 * (useThreadSocket). Тесты симулируют это через мок openThreadSocket:
 * регистрируют handlers по threadId, тесты сами шлют answer_done/error
 * через socketHandlersByThread.get(id)!.onEvent(...).
 */
let socketHandlersByThread: Map<string, ThreadSocketHandlers>;

function setupSocketMock() {
  socketHandlersByThread = new Map();
  vi.mocked(openThreadSocket).mockImplementation((threadId, handlers) => {
    socketHandlersByThread.set(threadId, handlers);
    return { close: vi.fn() };
  });
}

/**
 * Повторяет порядок реального сервера: итог раунда сперва сохраняется в БД
 * (threadStore — "БД" фейкового сервера), только потом уходит answer_done —
 * клиент после события перечитывает тред (useThreadSocket#reconcileThread).
 */
function emitAnswerDone(
  threadStore: Record<string, ThreadDetailDto>,
  threadId: string,
  message: ThreadDetailDto["messages"][number],
) {
  const saved = threadStore[threadId]!;
  threadStore[threadId] = {
    ...saved,
    status: "done",
    messages: [...saved.messages, message],
  };
  socketHandlersByThread.get(threadId)?.onEvent({
    type: "answer_done",
    thread_id: threadId,
    seq: 1,
    status: "done",
    message,
  });
}

const SERVICES: ServiceDto[] = [
  {
    id: "qa",
    type_label: "Консультация",
    name: "Вопрос-ответ",
    price: 2900,
    is_active: true,
  },
  {
    id: "doc",
    type_label: "Документ",
    name: "Подготовка документа",
    price: 4900,
    is_active: true,
  },
];

const SESSION: Session = {
  session_id: "s1",
  language: "ru",
  theme: null,
  onboarding_seen: true,
};

function makeThread(overrides: Partial<ThreadDetailDto>): ThreadDetailDto {
  return {
    id: "t1",
    service_id: "qa",
    status: "done",
    title: "Тред",
    preview_text: "Превью",
    message_count: 1,
    created_at: new Date().toISOString(),
    last_message_at: new Date().toISOString(),
    messages: [],
    ...overrides,
  };
}

function renderChatPage(initialPath = "/", queryClient = createTestQueryClient()) {
  return render(
    <MemoryRouter initialEntries={[initialPath]}>
      <QueryClientProvider client={queryClient}>
        <ChatPage />
        <PaymentModal />
        <ToastViewport />
      </QueryClientProvider>
    </MemoryRouter>,
  );
}

afterEach(() => {
  vi.unstubAllEnvs();
  useLangStore.setState({ lang: "ru" });
  vi.mocked(api.get).mockReset();
  vi.mocked(api.post).mockReset();
  vi.mocked(openThreadSocket).mockReset();
});

function baseGetMock(threadStore: Record<string, ThreadDetailDto>) {
  return (path: string) => {
    if (path === "/services") return Promise.resolve(SERVICES);
    if (path === "/sessions/me") return Promise.resolve(SESSION);
    const threadMatch = /^\/threads\/([^/]+)$/.exec(path);
    if (threadMatch) {
      const thread = threadStore[threadMatch[1]!];
      if (!thread) throw new Error(`thread not found: ${path}`);
      return Promise.resolve(thread);
    }
    throw new Error(`unexpected GET ${path}`);
  };
}

describe("ChatPage", () => {
  it("приложенный файл виден в отправленном сообщении, а не пропадает из чата", async () => {
    vi.stubEnv("VITE_FILE_MAX_SIZE_BYTES", "15728640");
    setupSocketMock();
    const threadStore: Record<string, ThreadDetailDto> = {};
    vi.mocked(api.get).mockImplementation(baseGetMock(threadStore));
    const attachment = {
      file_id: "file-1",
      original_name: "dogovor.pdf",
      mime_type: "application/pdf",
      size_bytes: 2048,
    };
    vi.mocked(api.post).mockImplementation((path: string, body?: unknown) => {
      if (path === "/files/upload") {
        return Promise.resolve({
          ...attachment,
          url: "https://storage/file-1",
          processing_status: "processed",
        });
      }
      if (path === "/threads") {
        const created = makeThread({
          status: "processing",
          messages: [
            {
              id: "m-user",
              sender: "user",
              input_type: "file",
              text: (body as { text: string }).text,
              feedback: null,
              processing_time_ms: null,
              created_at: new Date().toISOString(),
              attachments: [attachment],
            },
          ],
        });
        threadStore[created.id] = created;
        return Promise.resolve(created);
      }
      throw new Error(`unexpected POST ${path}`);
    });

    const user = userEvent.setup();
    const { container } = renderChatPage();

    // Нативный input[type=file] скрыт (открывается кнопкой-скрепкой) — роли/подписи у него нет.
    const fileInput = container.querySelector<HTMLInputElement>('input[type="file"]')!;
    await user.upload(
      fileInput,
      new File([new Uint8Array(2048)], "dogovor.pdf", { type: "application/pdf" }),
    );
    await user.type(
      screen.getByPlaceholderText("Опишите ситуацию…"),
      "Проверьте договор",
    );
    await waitFor(() =>
      expect(screen.getByRole("button", { name: "Отправить сообщение" })).toBeEnabled(),
    );
    await user.click(screen.getByRole("button", { name: "Отправить сообщение" }));

    expect(api.post).toHaveBeenCalledWith(
      "/threads",
      expect.objectContaining({ file_ids: ["file-1"], input_type: "file" }),
    );
    await waitFor(() => expect(socketHandlersByThread.has("t1")).toBe(true));
    // Вложение ушло из composer (кнопки «Убрать вложение» нет) и осталось в сообщении.
    expect(
      screen.queryByRole("button", { name: "Убрать вложение" }),
    ).not.toBeInTheDocument();
    expect(screen.getByText("dogovor.pdf")).toBeInTheDocument();
    expect(screen.getByText("Проверьте договор")).toBeInTheDocument();
  });

  it("пустое состояние: быстрая тема подставляет черновик в композер", async () => {
    vi.mocked(api.get).mockImplementation(baseGetMock({}));
    const user = userEvent.setup();
    renderChatPage();

    await user.click(screen.getByRole("button", { name: "Трудовой спор" }));

    expect(screen.getByPlaceholderText("Опишите ситуацию…")).toHaveValue(
      "Работодатель не выплатил зарплату вовремя. Что мне делать?",
    );
  });

  it("отправка первого сообщения создаёт тред (processing) и показывает ответ, когда его доставит WS", async () => {
    setupSocketMock();
    const threadStore: Record<string, ThreadDetailDto> = {};
    vi.mocked(api.get).mockImplementation(baseGetMock(threadStore));
    vi.mocked(api.post).mockImplementation((path: string, body?: unknown) => {
      if (path === "/threads") {
        const created = makeThread({
          status: "processing",
          messages: [
            {
              id: "m-user",
              sender: "user",
              input_type: "text",
              text: (body as { text: string }).text,
              feedback: null,
              processing_time_ms: null,
              created_at: new Date().toISOString(),
              attachments: [],
            },
          ],
        });
        threadStore[created.id] = created;
        return Promise.resolve(created);
      }
      throw new Error(`unexpected POST ${path}`);
    });

    const user = userEvent.setup();
    renderChatPage();

    await user.type(
      screen.getByPlaceholderText("Опишите ситуацию…"),
      "У меня вопрос про отпуск",
    );
    await user.click(screen.getByRole("button", { name: "Отправить сообщение" }));

    await waitFor(() => expect(socketHandlersByThread.has("t1")).toBe(true));
    emitAnswerDone(threadStore, "t1", {
      id: "m-assistant",
      sender: "assistant",
      input_type: "text",
      text: "Ответ ассистента по вашему вопросу.",
      sources: [{ ref: "ТК РК, ст. 1", quote: "..." }],
      feedback: null,
      processing_time_ms: 500,
      created_at: new Date().toISOString(),
      attachments: [],
    });

    expect(
      await screen.findByText("Ответ ассистента по вашему вопросу."),
    ).toBeInTheDocument();
    expect(api.post).toHaveBeenCalledWith("/threads", {
      service_id: "qa",
      text: "У меня вопрос про отпуск",
      input_type: "text",
      file_ids: undefined,
    });
  });

  it("сообщение видно сразу по нажатию, до ответа сервера, вместе с индикатором ответа", async () => {
    setupSocketMock();
    vi.mocked(api.get).mockImplementation(baseGetMock({}));
    vi.mocked(api.post).mockImplementation(() => new Promise(() => {}));

    const user = userEvent.setup();
    renderChatPage();

    const composer = screen.getByPlaceholderText("Опишите ситуацию…");
    await user.type(composer, "У меня вопрос про отпуск");
    await user.click(screen.getByRole("button", { name: "Отправить сообщение" }));

    expect(screen.getByText("У меня вопрос про отпуск")).toBeInTheDocument();
    expect(screen.getByText("Отправляем ваш вопрос…")).toBeInTheDocument();
    expect(composer).toHaveValue("");
  });

  it("ошибка отправки убирает сообщение и возвращает текст в композер", async () => {
    setupSocketMock();
    vi.mocked(api.get).mockImplementation(baseGetMock({}));
    vi.mocked(api.post).mockRejectedValue(new Error("network down"));

    const user = userEvent.setup();
    renderChatPage();

    const composer = screen.getByPlaceholderText("Опишите ситуацию…");
    await user.type(composer, "У меня вопрос про отпуск");
    await user.click(screen.getByRole("button", { name: "Отправить сообщение" }));

    await waitFor(() => expect(composer).toHaveValue("У меня вопрос про отпуск"));
    expect(screen.queryByText("Отправляем ваш вопрос…")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Трудовой спор" })).toBeInTheDocument();
  });

  it("создание треда перечитывает баланс — кредит списан на сервере", async () => {
    setupSocketMock();
    vi.mocked(api.get).mockImplementation(baseGetMock({}));
    vi.mocked(api.post).mockResolvedValue(makeThread({ status: "processing" }));
    const queryClient = createTestQueryClient();
    const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries");

    const user = userEvent.setup();
    renderChatPage("/", queryClient);

    await user.type(
      screen.getByPlaceholderText("Опишите ситуацию…"),
      "У меня вопрос про отпуск",
    );
    await user.click(screen.getByRole("button", { name: "Отправить сообщение" }));

    await waitFor(() =>
      expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ["balance"] }),
    );
  });

  it("если баланса не было — предлагает оплату, после подтверждения тред обрабатывается", async () => {
    setupSocketMock();
    const threadStore: Record<string, ThreadDetailDto> = {};
    vi.mocked(api.get).mockImplementation(baseGetMock(threadStore));
    vi.mocked(api.post).mockImplementation((path: string) => {
      if (path === "/threads") {
        const created = makeThread({
          status: "awaiting_payment",
          messages: [],
        });
        threadStore[created.id] = created;
        return Promise.resolve(created);
      }
      if (path === "/payments/checkout") {
        return Promise.resolve({ payment_id: "pay-1", amount: 2900 });
      }
      if (path === "/payments/pay-1/confirm") {
        threadStore.t1 = makeThread({
          status: "done",
          messages: [
            {
              id: "m-assistant",
              sender: "assistant",
              input_type: "text",
              text: "Готовый ответ после оплаты.",
              feedback: null,
              processing_time_ms: 300,
              created_at: new Date().toISOString(),
              attachments: [],
            },
          ],
        });
        return Promise.resolve({
          id: "pay-1",
          kind: "single_service",
          amount: 2900,
          status: "success",
          provider: "mock",
          paid_at: null,
        });
      }
      throw new Error(`unexpected POST ${path}`);
    });

    const user = userEvent.setup();
    renderChatPage();

    await user.type(
      screen.getByPlaceholderText("Опишите ситуацию…"),
      "Вопрос без баланса",
    );
    await user.click(screen.getByRole("button", { name: "Отправить сообщение" }));

    const dialog = await screen.findByRole("dialog", { name: "Оплата консультации" });
    await user.click(within(dialog).getByRole("button", { name: "Оплатить картой" }));

    expect(await screen.findByText("Готовый ответ после оплаты.")).toBeInTheDocument();
  });

  it("отказ от оплаты при отправке — тред остаётся «ожидает оплаты» без кнопки «Оплатить»", async () => {
    setupSocketMock();
    const threadStore: Record<string, ThreadDetailDto> = {};
    vi.mocked(api.get).mockImplementation(baseGetMock(threadStore));
    vi.mocked(api.post).mockImplementation((path: string) => {
      if (path === "/threads") {
        const created = makeThread({
          status: "awaiting_payment",
          messages: [],
        });
        threadStore[created.id] = created;
        return Promise.resolve(created);
      }
      throw new Error(`unexpected POST ${path}`);
    });

    const user = userEvent.setup();
    renderChatPage();

    await user.type(
      screen.getByPlaceholderText("Опишите ситуацию…"),
      "Вопрос без баланса",
    );
    await user.click(screen.getByRole("button", { name: "Отправить сообщение" }));
    const dialog = await screen.findByRole("dialog", { name: "Оплата консультации" });
    await user.click(within(dialog).getByRole("button", { name: "Отмена" }));

    expect(await screen.findByText(/Обращение ожидает оплаты/)).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Перезапустить вопрос" }),
    ).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Пополнить баланс" })).toHaveAttribute(
      "href",
      "/tariffs",
    );
    expect(screen.queryByRole("button", { name: "Оплатить" })).not.toBeInTheDocument();
  });

  it("перезапуск вопроса после пополнения баланса — ответ приходит по WS", async () => {
    setupSocketMock();
    const threadStore: Record<string, ThreadDetailDto> = {
      t1: makeThread({ status: "awaiting_payment" }),
    };
    vi.mocked(api.get).mockImplementation(baseGetMock(threadStore));
    vi.mocked(api.post).mockImplementation((path: string) => {
      if (path === "/threads/t1/resume") {
        threadStore.t1 = makeThread({ status: "processing" });
        return Promise.resolve(threadStore.t1);
      }
      throw new Error(`unexpected POST ${path}`);
    });

    const user = userEvent.setup();
    renderChatPage("/?thread=t1");
    await user.click(await screen.findByRole("button", { name: "Перезапустить вопрос" }));

    await waitFor(() => expect(socketHandlersByThread.has("t1")).toBe(true));
    emitAnswerDone(threadStore, "t1", {
      id: "m-assistant",
      sender: "assistant",
      input_type: "text",
      text: "Ответ после перезапуска.",
      feedback: null,
      processing_time_ms: null,
      created_at: new Date().toISOString(),
      attachments: [],
    });

    expect(await screen.findByText("Ответ после перезапуска.")).toBeInTheDocument();
    expect(screen.queryByText(/Обращение ожидает оплаты/)).not.toBeInTheDocument();
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  it("перезапуск без баланса — снова предлагает оплату", async () => {
    setupSocketMock();
    const threadStore: Record<string, ThreadDetailDto> = {
      t1: makeThread({ status: "awaiting_payment" }),
    };
    vi.mocked(api.get).mockImplementation(baseGetMock(threadStore));
    vi.mocked(api.post).mockImplementation((path: string) => {
      if (path === "/threads/t1/resume") return Promise.resolve(threadStore.t1);
      throw new Error(`unexpected POST ${path}`);
    });

    const user = userEvent.setup();
    renderChatPage("/?thread=t1");
    await user.click(await screen.findByRole("button", { name: "Перезапустить вопрос" }));

    expect(
      await screen.findByRole("dialog", { name: "Оплата консультации" }),
    ).toBeInTheDocument();
  });

  it("новый вопрос в отвеченном треде без баланса — просит оплату, бесплатно не отправляется", async () => {
    setupSocketMock();
    const threadStore: Record<string, ThreadDetailDto> = { t1: makeThread({}) };
    vi.mocked(api.get).mockImplementation(baseGetMock(threadStore));
    vi.mocked(api.post).mockImplementation((path: string) => {
      if (path === "/threads/t1/messages") {
        threadStore.t1 = makeThread({ status: "awaiting_payment" });
        return Promise.resolve(threadStore.t1);
      }
      throw new Error(`unexpected POST ${path}`);
    });

    const user = userEvent.setup();
    renderChatPage("/?thread=t1");
    await user.type(
      await screen.findByPlaceholderText("Опишите ситуацию…"),
      "Второй вопрос",
    );
    await user.click(screen.getByRole("button", { name: "Отправить сообщение" }));

    expect(
      await screen.findByRole("dialog", { name: "Оплата консультации" }),
    ).toBeInTheDocument();
  });

  it("follow-up на существующем треде дописывает сообщение в тот же тред, ответ приходит по WS", async () => {
    setupSocketMock();
    const threadStore: Record<string, ThreadDetailDto> = {
      t1: makeThread({
        messages: [
          {
            id: "m1",
            sender: "assistant",
            input_type: "text",
            text: "Первый ответ",
            feedback: null,
            processing_time_ms: null,
            created_at: new Date().toISOString(),
            attachments: [],
          },
        ],
      }),
    };
    vi.mocked(api.get).mockImplementation(baseGetMock(threadStore));
    vi.mocked(api.post).mockImplementation((path: string, body?: unknown) => {
      if (path === "/threads/t1/messages") {
        const updated = makeThread({
          status: "processing",
          messages: [
            ...threadStore.t1!.messages,
            {
              id: "m2",
              sender: "user",
              input_type: "text",
              text: (body as { text: string }).text,
              feedback: null,
              processing_time_ms: null,
              created_at: new Date().toISOString(),
              attachments: [],
            },
          ],
        });
        threadStore.t1 = updated;
        return Promise.resolve(updated);
      }
      throw new Error(`unexpected POST ${path}`);
    });

    const user = userEvent.setup();
    renderChatPage("/?thread=t1");
    await screen.findByText("Первый ответ");

    await user.type(
      screen.getByPlaceholderText("Опишите ситуацию…"),
      "Уточняющий вопрос",
    );
    await user.click(screen.getByRole("button", { name: "Отправить сообщение" }));

    await waitFor(() => expect(socketHandlersByThread.has("t1")).toBe(true));
    emitAnswerDone(threadStore, "t1", {
      id: "m3",
      sender: "assistant",
      input_type: "text",
      text: "Второй ответ",
      feedback: null,
      processing_time_ms: null,
      created_at: new Date().toISOString(),
      attachments: [],
    });

    expect(await screen.findByText("Второй ответ")).toBeInTheDocument();
    expect(api.post).toHaveBeenCalledWith("/threads/t1/messages", {
      text: "Уточняющий вопрос",
      input_type: "text",
      file_ids: undefined,
    });
  });

  it("оценка ответа шлёт feedback и обновляет вид кнопки", async () => {
    setupSocketMock();
    const threadStore: Record<string, ThreadDetailDto> = {
      t1: makeThread({
        messages: [
          {
            id: "m1",
            sender: "assistant",
            input_type: "text",
            text: "Ответ для оценки",
            feedback: null,
            processing_time_ms: null,
            created_at: new Date().toISOString(),
            attachments: [],
          },
        ],
      }),
    };
    vi.mocked(api.get).mockImplementation(baseGetMock(threadStore));
    vi.mocked(api.post).mockImplementation((path: string) => {
      if (path === "/messages/m1/feedback") {
        threadStore.t1 = {
          ...threadStore.t1!,
          messages: [{ ...threadStore.t1!.messages[0]!, feedback: "like" }],
        };
        return Promise.resolve(threadStore.t1.messages[0]);
      }
      throw new Error(`unexpected POST ${path}`);
    });

    const user = userEvent.setup();
    renderChatPage("/?thread=t1");
    await screen.findByText("Ответ для оценки");

    await user.click(screen.getByRole("button", { name: "Полезно" }));

    await waitFor(() => {
      expect(screen.getByRole("button", { name: "Полезно" })).toHaveAttribute(
        "aria-pressed",
        "true",
      );
    });
    expect(api.post).toHaveBeenCalledWith("/messages/m1/feedback", { value: "like" });
  });
});
