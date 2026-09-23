import { afterEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { QueryClientProvider } from "@tanstack/react-query";
import { HistoryPage } from "./HistoryPage";
import { ConfirmModal } from "@/shared/ui/ConfirmModal";
import { PaymentModal } from "@/shared/ui/PaymentModal";
import { ToastViewport } from "@/shared/ui/toast/ToastViewport";
import { useLangStore } from "@/shared/stores/useLangStore";
import { createTestQueryClient } from "@/test/queryClient";
import type { ServiceDto, ThreadDto } from "@/shared/types/api";

vi.mock("@/shared/lib/api", async () => {
  const actual =
    await vi.importActual<typeof import("@/shared/lib/api")>("@/shared/lib/api");
  return {
    ...actual,
    api: { get: vi.fn(), post: vi.fn(), put: vi.fn(), patch: vi.fn(), delete: vi.fn() },
  };
});
import { api } from "@/shared/lib/api";

function hoursAgo(hours: number): string {
  return new Date(Date.now() - hours * 60 * 60 * 1000).toISOString();
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

function makeThread(overrides: Partial<ThreadDto>): ThreadDto {
  return {
    id: "thread-id",
    service_id: "qa",
    status: "done",
    title: "Тред",
    preview_text: "Превью",
    message_count: 1,
    created_at: hoursAgo(5),
    last_message_at: hoursAgo(2),
    ...overrides,
  };
}

let threads: ThreadDto[] = [];

function threadsResponse(items: ThreadDto[]) {
  return { items, page: 1, page_size: 20, total: items.length };
}

function mockApi() {
  vi.mocked(api.get).mockImplementation((path: string) => {
    if (path.startsWith("/threads/")) {
      const id = path.replace("/threads/", "");
      const thread = threads.find((t) => t.id === id);
      if (!thread) throw new Error(`thread not found: ${id}`);
      return Promise.resolve({ ...thread, messages: [] });
    }
    if (path.startsWith("/threads")) {
      const url = new URL(`http://x${path}`);
      const status = url.searchParams.get("status");
      const search = url.searchParams.get("search")?.toLowerCase();
      let items = threads;
      if (status) items = items.filter((t) => t.status === status);
      if (search) {
        items = items.filter(
          (t) =>
            t.title.toLowerCase().includes(search) ||
            t.preview_text.toLowerCase().includes(search),
        );
      }
      return Promise.resolve(threadsResponse(items));
    }
    if (path === "/services") return Promise.resolve(SERVICES);
    throw new Error(`unexpected GET ${path}`);
  });
  vi.mocked(api.delete).mockImplementation((path: string) => {
    const id = path.replace("/threads/", "");
    threads = threads.filter((t) => t.id !== id);
    return Promise.resolve(undefined);
  });
}

function renderHistoryPage() {
  const queryClient = createTestQueryClient();
  return render(
    <MemoryRouter>
      <QueryClientProvider client={queryClient}>
        <HistoryPage />
        <ConfirmModal />
        <PaymentModal />
        <ToastViewport />
      </QueryClientProvider>
    </MemoryRouter>,
  );
}

afterEach(() => {
  useLangStore.setState({ lang: "ru" });
  vi.mocked(api.get).mockReset();
  vi.mocked(api.post).mockReset();
  vi.mocked(api.delete).mockReset();
});

describe("HistoryPage", () => {
  it("показывает все треды с бэка при первой загрузке", async () => {
    threads = [
      makeThread({ id: "t1", title: "Работодатель не отдаёт трудовую книжку" }),
      makeThread({ id: "t2", title: "Раздел имущества при разводе" }),
    ];
    mockApi();
    renderHistoryPage();

    expect(
      await screen.findByText("Работодатель не отдаёт трудовую книжку"),
    ).toBeInTheDocument();
    expect(screen.getByText("Раздел имущества при разводе")).toBeInTheDocument();
  });

  it("фильтрует список по поиску (с дебаунсом, запрос уходит на бэк)", async () => {
    threads = [
      makeThread({ id: "t1", title: "Работодатель не отдаёт трудовую книжку" }),
      makeThread({ id: "t2", title: "Раздел имущества при разводе" }),
    ];
    mockApi();
    const user = userEvent.setup();
    renderHistoryPage();
    await screen.findByText("Работодатель не отдаёт трудовую книжку");

    await user.type(
      screen.getByRole("searchbox", { name: "Поиск по обращениям" }),
      "трудовую книжку",
    );

    await waitFor(() => {
      expect(
        screen.getByText("Работодатель не отдаёт трудовую книжку"),
      ).toBeInTheDocument();
      expect(screen.queryByText("Раздел имущества при разводе")).not.toBeInTheDocument();
    });
  });

  it("фильтрует по статусу", async () => {
    threads = [
      makeThread({ id: "t1", title: "Готовый тред", status: "done" }),
      makeThread({
        id: "t2",
        title: "Ожидающий тред",
        status: "awaiting_payment",
      }),
    ];
    mockApi();
    const user = userEvent.setup();
    renderHistoryPage();
    await screen.findByText("Готовый тред");

    await user.selectOptions(screen.getByRole("combobox", { name: "Статус" }), "done");

    await waitFor(() => {
      expect(screen.getByText("Готовый тред")).toBeInTheDocument();
      expect(screen.queryByText("Ожидающий тред")).not.toBeInTheDocument();
    });
  });

  it("фильтрует по периоду ('Сегодня') на уже загруженной странице", async () => {
    threads = [
      makeThread({ id: "t1", title: "Сегодняшний", last_message_at: hoursAgo(1) }),
      makeThread({ id: "t2", title: "Старый", last_message_at: hoursAgo(25 * 24) }),
    ];
    mockApi();
    const user = userEvent.setup();
    renderHistoryPage();
    await screen.findByText("Сегодняшний");

    await user.selectOptions(screen.getByRole("combobox", { name: "Период" }), "today");

    expect(screen.getByText("Сегодняшний")).toBeInTheDocument();
    expect(screen.queryByText("Старый")).not.toBeInTheDocument();
  });

  it("показывает пустое состояние 'нет обращений' и отдельное 'ничего не найдено'", async () => {
    threads = [];
    mockApi();
    renderHistoryPage();

    expect(await screen.findByText("Пока нет обращений")).toBeInTheDocument();
  });

  it("показывает 'ничего не найдено' и сбрасывает фильтр, когда обращения есть, но фильтр пуст", async () => {
    threads = [makeThread({ id: "t1", title: "Единственный тред" })];
    mockApi();
    const user = userEvent.setup();
    renderHistoryPage();
    await screen.findByText("Единственный тред");

    await user.type(
      screen.getByRole("searchbox", { name: "Поиск по обращениям" }),
      "несуществующий запрос",
    );

    await waitFor(() => {
      expect(screen.getByText("Ничего не найдено")).toBeInTheDocument();
    });

    await user.click(screen.getByRole("button", { name: "Сбросить фильтры" }));

    await waitFor(() => {
      expect(screen.getByText("Единственный тред")).toBeInTheDocument();
    });
  });

  it("удаляет тред через ConfirmModal (DELETE /threads/:id) и показывает toast", async () => {
    threads = [makeThread({ id: "t1", title: "Тред на удаление" })];
    mockApi();
    const user = userEvent.setup();
    renderHistoryPage();
    await screen.findByText("Тред на удаление");

    await user.click(screen.getByRole("button", { name: "Удалить: Тред на удаление" }));
    const dialog = screen.getByRole("dialog", { name: "Подтвердите удаление" });
    await user.click(within(dialog).getByRole("button", { name: "Удалить" }));

    await waitFor(() => {
      expect(screen.queryByText("Тред на удаление")).not.toBeInTheDocument();
    });
    expect(await screen.findByText("Обращение удалено")).toBeInTheDocument();
    expect(api.delete).toHaveBeenCalledWith("/threads/t1");
  });

  it("отмена в ConfirmModal не удаляет тред", async () => {
    threads = [makeThread({ id: "t1", title: "Тред остаётся" })];
    mockApi();
    const user = userEvent.setup();
    renderHistoryPage();
    await screen.findByText("Тред остаётся");

    await user.click(screen.getByRole("button", { name: "Удалить: Тред остаётся" }));
    const dialog = screen.getByRole("dialog", { name: "Подтвердите удаление" });
    await user.click(within(dialog).getByRole("button", { name: "Отмена" }));

    expect(screen.getByText("Тред остаётся")).toBeInTheDocument();
  });

  it("неоплаченный тред показан со статусом «Ожидает оплаты», без кнопки оплаты — платят в «Тарифах»", async () => {
    threads = [
      makeThread({
        id: "t1",
        title: "Неоплаченный тред",
        status: "awaiting_payment",
      }),
    ];
    mockApi();
    renderHistoryPage();
    await screen.findByText("Неоплаченный тред");

    const card = screen.getByRole("link", { name: "Открыть тред: Неоплаченный тред" });
    expect(within(card).getByText("Ожидает оплаты")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Оплатить" })).not.toBeInTheDocument();
  });
});
