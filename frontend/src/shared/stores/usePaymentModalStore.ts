import { create } from "zustand";

interface PaymentOptions {
  title: string;
  description: string;
  amountLabel: string;
  /** Локализуемые надписи вокруг суммы/кнопок — своя копия по языку задаётся вызывающей фичей
   * (тот же паттерн, что confirmLabel/cancelLabel в useConfirmModalStore), сам PaymentModal
   * языка не знает. */
  amountFieldLabel: string;
  confirmLabel: string;
  cancelLabel: string;
  note: string;
  onConfirm: () => void;
}

interface PaymentModalState extends PaymentOptions {
  isOpen: boolean;
  open: (
    options: Partial<PaymentOptions> & Pick<PaymentOptions, "amountLabel" | "onConfirm">,
  ) => void;
  close: () => void;
}

const DEFAULTS: PaymentOptions = {
  title: "Оплата",
  description: "",
  amountLabel: "",
  amountFieldLabel: "К оплате",
  confirmLabel: "Оплатить картой",
  cancelLabel: "Отмена",
  note: "Демонстрационная оплата — деньги не списываются.",
  onConfirm: () => {},
};

/**
 * Единственный экземпляр оплаты на всё приложение (M3 из PLAN.md §1) —
 * Chat и Tariffs зовут его отсюда, компонент-рендерер — PaymentModal
 * в shared/ui, смонтирован один раз в AppShell. Оплата — заглушка/mock
 * (brief §3.4/§5) — onConfirm просто выполняет то, что нужно вызывающей странице.
 */
export const usePaymentModalStore = create<PaymentModalState>((set) => ({
  ...DEFAULTS,
  isOpen: false,
  open: (options) => set({ ...DEFAULTS, ...options, isOpen: true }),
  close: () => set({ isOpen: false }),
}));
