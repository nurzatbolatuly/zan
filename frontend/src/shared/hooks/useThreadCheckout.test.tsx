import { afterEach, describe, expect, it, vi } from "vitest";
import { renderHook } from "@testing-library/react";
import { QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { createTestQueryClient } from "@/test/queryClient";
import { usePaymentModalStore } from "@/shared/stores/usePaymentModalStore";
import type { PaymentStep } from "@/shared/stores/usePaymentModalStore";
import type { CheckoutResponseDto, PaymentDto } from "@/shared/types/api";

vi.mock("@/shared/lib/api", async () => {
  const actual =
    await vi.importActual<typeof import("@/shared/lib/api")>("@/shared/lib/api");
  return {
    ...actual,
    api: { get: vi.fn(), post: vi.fn(), put: vi.fn(), patch: vi.fn(), delete: vi.fn() },
  };
});

import { api } from "@/shared/lib/api";
import { useThreadCheckout } from "./useThreadCheckout";

afterEach(() => {
  vi.mocked(api.get).mockReset();
  vi.mocked(api.post).mockReset();
  usePaymentModalStore.setState({ isOpen: false });
});

describe("useThreadCheckout — payForThread", () => {
  it("проходит checkout -> confirm и сразу обновляет тред и баланс, не дожидаясь ответа ассистента", async () => {
    const calls: string[] = [];
    vi.mocked(api.post).mockImplementation((path: string) => {
      calls.push(`POST ${path}`);
      if (path === "/payments/checkout")
        return Promise.resolve({
          payment_id: "pay-1",
          amount: 2900,
        } satisfies CheckoutResponseDto);
      if (path === "/payments/pay-1/confirm")
        return Promise.resolve({
          id: "pay-1",
          kind: "single_service",
          amount: 2900,
          status: "success",
          provider: "mock",
          paid_at: null,
        } satisfies PaymentDto);
      throw new Error(`unexpected POST ${path}`);
    });

    const client = createTestQueryClient();
    const invalidateSpy = vi.spyOn(client, "invalidateQueries");
    const wrapper = ({ children }: { children: ReactNode }) => (
      <QueryClientProvider client={client}>{children}</QueryClientProvider>
    );
    const { result } = renderHook(() => useThreadCheckout(), { wrapper });

    result.current.payForThread({
      threadId: "t1",
      serviceId: "qa",
      amountTenge: 2900,
      copy: {
        title: "t",
        description: "d",
        amountField: "a",
        confirm: "c",
        cancel: "x",
        note: "n",
      },
    });

    const { steps, onConfirm } = usePaymentModalStore.getState();
    const reported: PaymentStep[] = [];
    await onConfirm((step) => reported.push(step));

    expect(steps).toEqual(["checkout", "confirm"]);
    expect(reported).toEqual(["checkout", "confirm"]);
    expect(calls).toEqual(["POST /payments/checkout", "POST /payments/pay-1/confirm"]);
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ["thread", "t1"] });
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ["balance"] });
  });

  it("ошибка confirm — тред не перечитывается, ошибка уходит модалке", async () => {
    vi.mocked(api.post).mockImplementation((path: string) => {
      if (path === "/payments/checkout")
        return Promise.resolve({
          payment_id: "pay-1",
          amount: 2900,
        } satisfies CheckoutResponseDto);
      return Promise.reject(new Error("payment failed"));
    });

    const client = createTestQueryClient();
    const wrapper = ({ children }: { children: ReactNode }) => (
      <QueryClientProvider client={client}>{children}</QueryClientProvider>
    );
    const invalidateSpy = vi.spyOn(client, "invalidateQueries");
    const { result } = renderHook(() => useThreadCheckout(), { wrapper });

    result.current.payForThread({
      threadId: "t1",
      serviceId: "qa",
      amountTenge: 2900,
      copy: {
        title: "t",
        description: "d",
        amountField: "a",
        confirm: "c",
        cancel: "x",
        note: "n",
      },
    });

    await expect(usePaymentModalStore.getState().onConfirm(() => {})).rejects.toThrow(
      "payment failed",
    );
    expect(invalidateSpy).not.toHaveBeenCalled();
  });
});
