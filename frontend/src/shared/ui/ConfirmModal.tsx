import { useState } from "react";
import { Modal } from "./Modal";
import { Button } from "./Button";
import { useConfirmModalStore } from "@/shared/stores/useConfirmModalStore";
import { useLangStore } from "@/shared/stores/useLangStore";
import { useToast } from "./toast/useToast";
import { logger } from "@/shared/lib/logger";
import { describeApiError } from "@/shared/lib/apiErrorMessages";

/**
 * Единственный инстанс на всё приложение — монтируется один раз в AppShell.
 * Страницы не рендерят свою модалку подтверждения, а зовут
 * useConfirmModalStore.getState().open({...}) (M1 из PLAN.md §1). `onConfirm`
 * — реальный сетевой вызов (Stage 6) — кнопка блокируется на время запроса,
 * модалка закрывается только при успехе (тот же паттерн, что PaymentModal).
 */
export function ConfirmModal() {
  const {
    isOpen,
    title,
    message,
    confirmLabel,
    cancelLabel,
    destructive,
    onConfirm,
    close,
  } = useConfirmModalStore();
  const lang = useLangStore((state) => state.lang);
  const toast = useToast();
  const [isPending, setIsPending] = useState(false);

  async function handleConfirm() {
    logger.info({ scope: "shared.confirm-modal", event: "confirmed", data: { title } });
    setIsPending(true);
    try {
      await onConfirm();
      close();
    } catch (error) {
      logger.error({ scope: "shared.confirm-modal", event: "confirm_failed", error });
      toast(describeApiError(error, lang), "error");
    } finally {
      setIsPending(false);
    }
  }

  function handleCancel() {
    if (isPending) return;
    logger.info({ scope: "shared.confirm-modal", event: "cancelled", data: { title } });
    close();
  }

  return (
    <Modal open={isOpen} onClose={handleCancel} ariaLabel={title} z="overlay-top">
      <div className="mb-2 text-h3 text-ink">{title}</div>
      <p className="mb-5 text-body-sm text-muted">{message}</p>
      <div className="flex gap-2">
        <Button
          variant="secondary"
          className="flex-1"
          onClick={handleCancel}
          disabled={isPending}
        >
          {cancelLabel}
        </Button>
        <Button
          variant={destructive ? "danger" : "primary"}
          className="flex-1"
          onClick={handleConfirm}
          disabled={isPending}
        >
          {isPending ? "…" : confirmLabel}
        </Button>
      </div>
    </Modal>
  );
}
