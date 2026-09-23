import { useState } from "react";
import { Check, Circle, Loader2 } from "lucide-react";
import { Modal } from "./Modal";
import { Button } from "./Button";
import { usePaymentModalStore } from "@/shared/stores/usePaymentModalStore";
import type { PaymentStep } from "@/shared/stores/usePaymentModalStore";
import { useLangStore } from "@/shared/stores/useLangStore";
import { useToast } from "./toast/useToast";
import { cn } from "@/shared/lib/cn";
import { logger } from "@/shared/lib/logger";
import { describeApiError } from "@/shared/lib/apiErrorMessages";
import type { Lang } from "@/shared/types/common";

const STEP_LABELS: Record<PaymentStep, Record<Lang, string>> = {
  checkout: { ru: "Создаём платёж", kz: "Төлем жасалуда" },
  confirm: { ru: "Подтверждаем оплату", kz: "Төлем расталуда" },
};

/**
 * Единственный инстанс на всё приложение — монтируется один раз в AppShell.
 * Chat/Tariffs/History зовут usePaymentModalStore.getState().open({...})
 * (M3 из PLAN.md §1). `onConfirm` — реальный сетевой вызов (Stage 6): пока
 * он идёт, модалка показывает прогресс по шагам (`steps`, текущий шаг
 * сообщает сам onConfirm через reportStep) и не закрывается; закрывается
 * только при успехе — при ошибке остаётся открытой с тостом, повторный клик
 * возможен без заново открытой модалки.
 */
export function PaymentModal() {
  const {
    isOpen,
    title,
    description,
    amountLabel,
    amountFieldLabel,
    confirmLabel,
    cancelLabel,
    note,
    steps,
    onConfirm,
    close,
  } = usePaymentModalStore();
  const lang = useLangStore((state) => state.lang);
  const toast = useToast();
  const [activeStep, setActiveStep] = useState<PaymentStep | null>(null);
  const isPending = activeStep !== null;
  const activeIndex = activeStep ? steps.indexOf(activeStep) : -1;

  async function handlePay() {
    logger.info({
      scope: "shared.payment-modal",
      event: "pay_confirmed",
      data: { title, amountLabel },
    });
    setActiveStep(steps[0] ?? "checkout");
    try {
      await onConfirm(setActiveStep);
      close();
    } catch (error) {
      logger.error({ scope: "shared.payment-modal", event: "pay_failed", error });
      toast(describeApiError(error, lang), "error");
    } finally {
      setActiveStep(null);
    }
  }

  function handleCancel() {
    if (isPending) return;
    logger.info({ scope: "shared.payment-modal", event: "cancelled", data: { title } });
    close();
  }

  return (
    <Modal open={isOpen} onClose={handleCancel} ariaLabel={title}>
      <div className="mb-1.5 text-h2 text-ink">{title}</div>
      {description && <p className="mb-4 text-body-sm text-muted">{description}</p>}
      <div className="mb-4 flex items-center justify-between rounded-lg border border-line bg-bg p-3.5">
        <span className="text-body-sm text-ink">{amountFieldLabel}</span>
        <strong className="text-h3 text-ink">{amountLabel}</strong>
      </div>
      {isPending && (
        <ol className="mb-4 flex flex-col gap-2" aria-live="polite">
          {steps.map((step, index) => {
            const isDone = index < activeIndex;
            const isActive = index === activeIndex;
            return (
              <li
                key={step}
                className={cn(
                  "flex items-center gap-2 text-body-sm",
                  isDone || isActive ? "text-ink" : "text-muted",
                )}
              >
                {isDone ? (
                  <Check size={16} className="text-accent" aria-hidden="true" />
                ) : isActive ? (
                  <Loader2
                    size={16}
                    className="animate-spin text-accent"
                    aria-hidden="true"
                  />
                ) : (
                  <Circle size={16} aria-hidden="true" />
                )}
                {STEP_LABELS[step][lang]}
              </li>
            );
          })}
        </ol>
      )}
      <div className="flex flex-col gap-2">
        <Button onClick={handlePay} disabled={isPending}>
          {activeStep ? STEP_LABELS[activeStep][lang] : confirmLabel}
        </Button>
        <Button variant="ghost" onClick={handleCancel} disabled={isPending}>
          {cancelLabel}
        </Button>
      </div>
      <p className="mt-3 text-center text-micro font-mono text-muted">{note}</p>
    </Modal>
  );
}
