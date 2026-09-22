import { create } from "zustand";
import { useConfirmModalStore } from "./useConfirmModalStore";

interface GuardCopy {
  title: string;
  confirmLabel: string;
  cancelLabel: string;
}

interface UnsavedChangesState {
  isDirty: boolean;
  message: string;
  /** Фича со своей формой сообщает сюда, есть ли несохранённое (и текст для confirm-модалки). */
  setDirty: (isDirty: boolean, message?: string) => void;
  /**
   * Общий шлюз для in-app переходов (нав-ссылка в шапке, смена вкладки в Settings):
   * если нет несохранённого — выполняет `action` сразу; если есть — спрашивает
   * подтверждение через общий `ConfirmModal` и выполняет `action` только после согласия.
   * `copy` — локализация конкретного вызова (у шапки и у вкладок Settings разный набор строк).
   */
  guard: (action: () => void, copy: GuardCopy) => void;
}

/**
 * Единственный источник "есть ли несохранённые изменения" на всё приложение —
 * позволяет шапке/таб-бару/вкладкам Settings спросить подтверждение перед уходом,
 * не зная ничего о конкретной форме, которая эти изменения держит (Settings→Prompts
 * сейчас — единственный вызывающий `setDirty`, PLAN.md §5 Stage 4a). Для закрытия/
 * обновления вкладки браузера — отдельно `shared/hooks/useUnsavedChangesGuard.ts`.
 */
export const useUnsavedChangesStore = create<UnsavedChangesState>((set, get) => ({
  isDirty: false,
  message: "",
  setDirty: (isDirty, message = "") => set({ isDirty, message: isDirty ? message : "" }),
  guard: (action, copy) => {
    const { isDirty, message } = get();
    if (!isDirty) {
      action();
      return;
    }
    useConfirmModalStore.getState().open({
      title: copy.title,
      message,
      confirmLabel: copy.confirmLabel,
      cancelLabel: copy.cancelLabel,
      destructive: false,
      onConfirm: () => {
        set({ isDirty: false, message: "" });
        action();
      },
    });
  },
}));
