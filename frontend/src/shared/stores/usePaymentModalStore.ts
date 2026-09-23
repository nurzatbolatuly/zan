import { create } from "zustand";

/** Шаг обработки оплаты, который модалка показывает пользователю, пока ждёт onConfirm. */
export type PaymentStep = "checkout" | "confirm";

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
  /** Шаги, которые пройдёт onConfirm, в порядке выполнения — модалка рисует их списком. */
  steps: PaymentStep[];
  /** Реальный сетевой вызов (checkout+confirm, Stage 6) — модалка ждёт
   * промис, показывает прогресс по `steps` и закрывается только при успехе;
   * ошибка остаётся видимой (модалка не закрывается), см. PaymentModal.tsx.
   * `reportStep` — переключить подсвеченный шаг перед началом его выполнения. */
  onConfirm: (reportStep: (step: PaymentStep) => void) => Promise<void>;
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
  steps: ["checkout", "confirm"],
  onConfirm: () => Promise.resolve(),
};

/**
 * Единственный экземпляр оплаты на всё приложение (M3 из PLAN.md §1) —
 * Chat/Tariffs/History зовут его отсюда, компонент-рендерер — PaymentModal
 * в shared/ui, смонтирован один раз в AppShell. `onConfirm` — реальный
 * `POST /payments/checkout` + `.../confirm` (Stage 6); провайдер оплаты на
 * бэке сам называется "mock" (openapi.yaml#Payment.provider) — карта
 * реально не списывается, но запрос/начисление баланса настоящие, не
 * фронтовая имитация.
 */
export const usePaymentModalStore = create<PaymentModalState>((set) => ({
  ...DEFAULTS,
  isOpen: false,
  open: (options) => set({ ...DEFAULTS, ...options, isOpen: true }),
  close: () => set({ isOpen: false }),
}));
