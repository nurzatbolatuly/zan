import { afterEach, describe, expect, it } from "vitest";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { TariffsPage } from "./TariffsPage";
import { PaymentModal } from "@/shared/ui/PaymentModal";
import { ToastViewport } from "@/shared/ui/toast/ToastViewport";
import { useLangStore } from "@/shared/stores/useLangStore";
import { useSessionStore } from "@/shared/stores/useSessionStore";

// ru-RU группирует тысячи неразрывным пробелом (U+00A0/U+202F в разных рантаймах) —
// сравниваем регуляркой, а не строкой с обычным пробелом (см. format.test.ts).
function priceText(value: string): RegExp {
  return new RegExp(`^${value.replace(/ /g, "[\\s\\u00A0\\u202F]")}$`);
}

function renderTariffsPage() {
  return render(
    <>
      <TariffsPage />
      <PaymentModal />
      <ToastViewport />
    </>,
  );
}

afterEach(() => {
  useLangStore.setState({ lang: "ru" });
  useSessionStore.setState({ balance: 3 });
});

describe("TariffsPage", () => {
  it("показывает все пакеты из мока с ценой", () => {
    renderTariffsPage();

    expect(screen.getByText("1 вопрос")).toBeInTheDocument();
    expect(screen.getByText("Пакет вопросов")).toBeInTheDocument();
    expect(screen.getByText("Вопрос + документ")).toBeInTheDocument();
    expect(screen.getByText("Для бизнеса")).toBeInTheDocument();
    // b2: 3 запроса по 2900, скидка 15% -> 7400 ₸ (округление до 10, см. pricing.test.ts)
    expect(screen.getByText(priceText("7 400 ₸"))).toBeInTheDocument();
  });

  it("покупка пакета открывает оплату и пополняет баланс после подтверждения", async () => {
    const user = userEvent.setup();
    renderTariffsPage();

    // Пакеты рендерятся в порядке BUNDLES (mocks.ts) — b1 "1 вопрос" первый.
    await user.click(screen.getAllByRole("button", { name: "Купить" })[0]!);

    const dialog = await screen.findByRole("dialog", {
      name: "Оплата тарифа «1 вопрос»",
    });
    expect(within(dialog).getByText(priceText("2 900 ₸"))).toBeInTheDocument();

    await user.click(within(dialog).getByRole("button", { name: "Оплатить картой" }));

    expect(await screen.findByText("«1 вопрос» активирован")).toBeInTheDocument();
    expect(useSessionStore.getState().balance).toBe(4);
  });

  it("свой набор: изменение количества пересчитывает сумму и покупка проходит через общую оплату", async () => {
    const user = userEvent.setup();
    renderTariffsPage();

    await user.click(screen.getByRole("button", { name: "Выбрать количество" }));

    const modal = await screen.findByRole("dialog", { name: "Свой набор запросов" });
    await user.click(
      within(modal).getByRole("button", { name: "Увеличить количество: Вопрос-ответ" }),
    );

    // qa: 2*2900 + doc: 1*4900 = 10700 ₸
    expect(within(modal).getByText(priceText("10 700 ₸"))).toBeInTheDocument();

    await user.click(within(modal).getByRole("button", { name: "Продолжить к оплате" }));

    const payDialog = await screen.findByRole("dialog", { name: "Оплата своего набора" });
    expect(within(payDialog).getByText(priceText("10 700 ₸"))).toBeInTheDocument();

    await user.click(within(payDialog).getByRole("button", { name: "Оплатить картой" }));

    expect(
      await screen.findByText("«Свой набор запросов» активирован"),
    ).toBeInTheDocument();
    expect(useSessionStore.getState().balance).toBe(5);
  });
});
