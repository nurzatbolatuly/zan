import { create } from "zustand";

interface ConfirmOptions {
  title: string;
  message: string;
  confirmLabel: string;
  cancelLabel: string;
  /** true → кнопка подтверждения красная (удаление и т.п.) */
  destructive: boolean;
  /** Реальный сетевой вызов (Stage 6: DELETE /threads/{id}, /admin/tariffs/{id}
   * и т.п.) — модалка ждёт промис и закрывается только при успехе, см.
   * ConfirmModal.tsx. */
  onConfirm: () => Promise<void>;
}

interface ConfirmModalState extends ConfirmOptions {
  isOpen: boolean;
  open: (
    options: Partial<ConfirmOptions> &
      Pick<ConfirmOptions, "title" | "message" | "onConfirm">,
  ) => void;
  close: () => void;
}

const DEFAULTS: ConfirmOptions = {
  title: "",
  message: "",
  confirmLabel: "Удалить",
  cancelLabel: "Отмена",
  destructive: true,
  onConfirm: () => Promise.resolve(),
};

/**
 * Единственный экземпляр подтверждения на всё приложение (M1 из PLAN.md §1) —
 * History и Settings→Tariffs зовут его отсюда, компонент-рендерер — ConfirmModal
 * в shared/ui, смонтирован один раз в AppShell.
 */
export const useConfirmModalStore = create<ConfirmModalState>((set) => ({
  ...DEFAULTS,
  isOpen: false,
  open: (options) => set({ ...DEFAULTS, ...options, isOpen: true }),
  close: () => set({ isOpen: false }),
}));
