import { Modal } from "./Modal";
import { Button } from "./Button";
import { useConfirmModalStore } from "@/shared/stores/useConfirmModalStore";
import { logger } from "@/shared/lib/logger";

/**
 * Единственный инстанс на всё приложение — монтируется один раз в AppShell.
 * Страницы не рендерят свою модалку подтверждения, а зовут
 * useConfirmModalStore.getState().open({...}) (M1 из PLAN.md §1).
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

  function handleConfirm() {
    logger.info({ scope: "shared.confirm-modal", event: "confirmed", data: { title } });
    onConfirm();
    close();
  }

  function handleCancel() {
    logger.info({ scope: "shared.confirm-modal", event: "cancelled", data: { title } });
    close();
  }

  return (
    <Modal open={isOpen} onClose={handleCancel} ariaLabel={title} z="overlay-top">
      <div className="mb-2 text-h3 text-ink">{title}</div>
      <p className="mb-5 text-body-sm text-muted">{message}</p>
      <div className="flex gap-2">
        <Button variant="secondary" className="flex-1" onClick={handleCancel}>
          {cancelLabel}
        </Button>
        <Button
          variant={destructive ? "danger" : "primary"}
          className="flex-1"
          onClick={handleConfirm}
        >
          {confirmLabel}
        </Button>
      </div>
    </Modal>
  );
}
