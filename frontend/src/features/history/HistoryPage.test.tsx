import { afterEach, describe, expect, it } from "vitest";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { HistoryPage } from "./HistoryPage";
import { ConfirmModal } from "@/shared/ui/ConfirmModal";
import { ToastViewport } from "@/shared/ui/toast/ToastViewport";
import { useLangStore } from "@/shared/stores/useLangStore";
import { THREAD_MOCKS } from "./threads.mocks";

function renderHistoryPage() {
  return render(
    <MemoryRouter>
      <HistoryPage />
      <ConfirmModal />
      <ToastViewport />
    </MemoryRouter>,
  );
}

afterEach(() => {
  useLangStore.setState({ lang: "ru" });
});

describe("HistoryPage", () => {
  it("показывает все треды из мока при первой загрузке", () => {
    renderHistoryPage();

    for (const thread of THREAD_MOCKS) {
      expect(screen.getByText(thread.title)).toBeInTheDocument();
    }
  });

  it("фильтрует список по поиску (с дебаунсом)", async () => {
    const user = userEvent.setup();
    renderHistoryPage();

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

  it("фильтрует список по статусу", async () => {
    const user = userEvent.setup();
    renderHistoryPage();

    await user.selectOptions(screen.getByRole("combobox", { name: "Статус" }), "done");

    expect(
      screen.getByText("Работодатель не отдаёт трудовую книжку"),
    ).toBeInTheDocument();
    expect(screen.queryByText("Раздел имущества при разводе")).not.toBeInTheDocument();
  });

  it("фильтрует список по периоду ('Сегодня')", async () => {
    const user = userEvent.setup();
    renderHistoryPage();

    await user.selectOptions(screen.getByRole("combobox", { name: "Период" }), "today");

    // Сегодня: "done" (2ч назад) и "processing" (1ч назад) из threads.mocks.ts.
    expect(
      screen.getByText("Работодатель не отдаёт трудовую книжку"),
    ).toBeInTheDocument();
    expect(
      screen.getByText("Регистрация ИП: какой налоговый режим выбрать"),
    ).toBeInTheDocument();
    // "canceled" — 25 дней назад, точно не сегодня.
    expect(
      screen.queryByText("Оформление сотрудника-иностранца"),
    ).not.toBeInTheDocument();
  });

  it("показывает пустое состояние 'ничего не найдено' и сбрасывает фильтр", async () => {
    const user = userEvent.setup();
    renderHistoryPage();

    await user.type(
      screen.getByRole("searchbox", { name: "Поиск по обращениям" }),
      "несуществующий запрос",
    );

    await waitFor(() => {
      expect(screen.getByText("Ничего не найдено")).toBeInTheDocument();
    });

    await user.click(screen.getByRole("button", { name: "Сбросить фильтры" }));

    await waitFor(() => {
      expect(
        screen.getByText("Работодатель не отдаёт трудовую книжку"),
      ).toBeInTheDocument();
    });
  });

  it("удаляет тред через ConfirmModal и показывает toast", async () => {
    const user = userEvent.setup();
    renderHistoryPage();

    const target = THREAD_MOCKS[0]!;
    await user.click(screen.getByRole("button", { name: `Удалить: ${target.title}` }));

    const dialog = screen.getByRole("dialog", { name: "Подтвердите удаление" });
    await user.click(within(dialog).getByRole("button", { name: "Удалить" }));

    await waitFor(() => {
      expect(screen.queryByText(target.title)).not.toBeInTheDocument();
    });
    expect(await screen.findByText("Обращение удалено")).toBeInTheDocument();
  });

  it("отмена в ConfirmModal не удаляет тред", async () => {
    const user = userEvent.setup();
    renderHistoryPage();

    const target = THREAD_MOCKS[0]!;
    await user.click(screen.getByRole("button", { name: `Удалить: ${target.title}` }));

    const dialog = screen.getByRole("dialog", { name: "Подтвердите удаление" });
    await user.click(within(dialog).getByRole("button", { name: "Отмена" }));

    expect(screen.getByText(target.title)).toBeInTheDocument();
  });
});
