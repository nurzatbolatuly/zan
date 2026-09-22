import { describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Button } from "./Button";

describe("Button", () => {
  it("рендерит текст и обрабатывает клик", async () => {
    const onClick = vi.fn();
    render(<Button onClick={onClick}>Сохранить</Button>);

    await userEvent.click(screen.getByRole("button", { name: "Сохранить" }));

    expect(onClick).toHaveBeenCalledTimes(1);
  });

  it("не вызывает onClick, когда disabled", async () => {
    const onClick = vi.fn();
    render(
      <Button onClick={onClick} disabled>
        Сохранить
      </Button>,
    );

    await userEvent.click(screen.getByRole("button", { name: "Сохранить" }));

    expect(onClick).not.toHaveBeenCalled();
  });
});
