import { afterEach, describe, expect, it, vi } from "vitest";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClientProvider } from "@tanstack/react-query";
import { TariffsPage } from "./TariffsPage";
import { PaymentModal } from "@/shared/ui/PaymentModal";
import { ToastViewport } from "@/shared/ui/toast/ToastViewport";
import { useLangStore } from "@/shared/stores/useLangStore";
import { createTestQueryClient } from "@/test/queryClient";
import type { ServiceDto, TariffDto } from "@/shared/types/api";

vi.mock("@/shared/lib/api", () => ({
  api: { get: vi.fn(), post: vi.fn(), put: vi.fn(), patch: vi.fn(), delete: vi.fn() },
}));
import { api } from "@/shared/lib/api";

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

const TARIFFS: TariffDto[] = [
  {
    id: "t1",
    name: "1 вопрос",
    discount_percent: 0,
    items: [{ service_id: "qa", qty: 1 }],
    subtotal: 2900,
    total: 2900,
    is_active: true,
  },
  {
    id: "t2",
    name: "Пакет вопросов",
    discount_percent: 15,
    items: [{ service_id: "qa", qty: 3 }],
    subtotal: 8700,
    total: 7400,
    is_active: true,
  },
];

// ru-RU группирует тысячи неразрывным пробелом (U+00A0/U+202F в разных рантаймах) —
// сравниваем регуляркой, а не строкой с обычным пробелом (см. format.test.ts).
function priceText(value: string): RegExp {
  return new RegExp(`^${value.replace(/ /g, "[\\s\\u00A0\\u202F]")}$`);
}

function renderTariffsPage() {
  const queryClient = createTestQueryClient();
  return render(
    <QueryClientProvider client={queryClient}>
      <TariffsPage />
      <PaymentModal />
      <ToastViewport />
    </QueryClientProvider>,
  );
}

afterEach(() => {
  useLangStore.setState({ lang: "ru" });
  vi.mocked(api.get).mockReset();
  vi.mocked(api.post).mockReset();
});

function mockCatalog() {
  vi.mocked(api.get).mockImplementation((path: string) => {
    if (path === "/services") return Promise.resolve(SERVICES);
    if (path === "/tariffs") return Promise.resolve(TARIFFS);
    throw new Error(`unexpected GET ${path}`);
  });
}

function mockCheckout() {
  vi.mocked(api.post).mockImplementation((path: string) => {
    if (path === "/payments/checkout") {
      return Promise.resolve({ payment_id: "pay-1", amount: 2900 });
    }
    if (path === "/payments/pay-1/confirm") {
      return Promise.resolve({
        id: "pay-1",
        kind: "tariff",
        amount: 2900,
        status: "success",
        provider: "mock",
        paid_at: null,
      });
    }
    throw new Error(`unexpected POST ${path}`);
  });
}

describe("TariffsPage", () => {
  it("показывает тарифы с бэка с ценой", async () => {
    mockCatalog();
    renderTariffsPage();

    expect(await screen.findByText("1 вопрос")).toBeInTheDocument();
    expect(screen.getByText("Пакет вопросов")).toBeInTheDocument();
    expect(screen.getByText(priceText("7 400 ₸"))).toBeInTheDocument();
  });

  it("покупка тарифа проходит checkout+confirm и показывает toast", async () => {
    mockCatalog();
    mockCheckout();
    const user = userEvent.setup();
    renderTariffsPage();

    await user.click((await screen.findAllByRole("button", { name: "Купить" }))[0]!);

    const dialog = await screen.findByRole("dialog", {
      name: "Оплата тарифа «1 вопрос»",
    });
    expect(within(dialog).getByText(priceText("2 900 ₸"))).toBeInTheDocument();

    await user.click(within(dialog).getByRole("button", { name: "Оплатить картой" }));

    expect(await screen.findByText("«1 вопрос» активирован")).toBeInTheDocument();
    expect(api.post).toHaveBeenCalledWith("/payments/checkout", {
      kind: "tariff",
      tariff_id: "t1",
    });
    expect(api.post).toHaveBeenCalledWith("/payments/pay-1/confirm");
  });

  it("свой набор: изменение количества пересчитывает сумму и покупка идёт через ту же оплату", async () => {
    mockCatalog();
    mockCheckout();
    const user = userEvent.setup();
    renderTariffsPage();

    await user.click(await screen.findByRole("button", { name: "Выбрать количество" }));

    const modal = await screen.findByRole("dialog", { name: "Свой набор запросов" });
    await user.click(
      within(modal).getByRole("button", { name: "Увеличить количество: Вопрос-ответ" }),
    );

    // qa: 2*2900 + doc: 1*4900 = 10700 ₸
    expect(within(modal).getByText(priceText("10 700 ₸"))).toBeInTheDocument();

    await user.click(within(modal).getByRole("button", { name: "Продолжить к оплате" }));

    const payDialog = await screen.findByRole("dialog", { name: "Оплата своего набора" });
    await user.click(within(payDialog).getByRole("button", { name: "Оплатить картой" }));

    expect(
      await screen.findByText("«Свой набор запросов» активирован"),
    ).toBeInTheDocument();
    expect(api.post).toHaveBeenCalledWith("/payments/checkout", {
      kind: "custom",
      items: [
        { service_id: "qa", qty: 2 },
        { service_id: "doc", qty: 1 },
      ],
    });
  });

  it("показывает ошибку загрузки с кнопкой повтора", async () => {
    vi.mocked(api.get).mockRejectedValue(new Error("network"));
    renderTariffsPage();

    expect(await screen.findByText("Не удалось загрузить тарифы.")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Повторить" })).toBeInTheDocument();
  });
});
