import { Modal } from "./Modal";
import { Button } from "./Button";
import { usePaymentModalStore } from "@/shared/stores/usePaymentModalStore";
import { logger } from "@/shared/lib/logger";

/**
 * Единственный инстанс на всё приложение — монтируется один раз в AppShell.
 * Chat и Tariffs зовут usePaymentModalStore.getState().open({...}) (M3 из PLAN.md §1).
 * Оплата — заглушка/mock (brief §3.4/§5), реальной интеграции нет ни здесь, ни позже.
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
    onConfirm,
    close,
  } = usePaymentModalStore();

  function handlePay() {
    logger.info({
      scope: "shared.payment-modal",
      event: "pay_confirmed",
      data: { title, amountLabel },
    });
    onConfirm();
    close();
  }

  function handleCancel() {
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
      <div className="flex flex-col gap-2">
        <Button onClick={handlePay}>{confirmLabel}</Button>
        <Button variant="ghost" onClick={handleCancel}>
          {cancelLabel}
        </Button>
      </div>
      <p className="mt-3 text-center text-micro font-mono text-muted">{note}</p>
    </Modal>
  );
}
