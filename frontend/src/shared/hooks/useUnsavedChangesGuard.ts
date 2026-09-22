import { useEffect } from "react";

/**
 * Предупреждает нативным диалогом браузера при закрытии/обновлении вкладки,
 * пока есть несохранённые изменения (Settings→Prompts, PLAN.md §5 Stage 4a).
 *
 * Не блокирует SPA-переход между роутами сам по себе: роутинг собран на
 * декларативном `<Routes>` (`app/router.tsx`), а не на data-роутере
 * (`createBrowserRouter`), поэтому `useBlocker` из react-router недоступен —
 * он кидает ошибку вне `RouterProvider`. In-app переход (клик по нав-ссылке,
 * смена вкладки) перехватывается отдельно через `useUnsavedChangesStore`.
 */
export function useUnsavedChangesGuard(isDirty: boolean): void {
  useEffect(() => {
    if (!isDirty) return;
    function handleBeforeUnload(event: BeforeUnloadEvent) {
      event.preventDefault();
      event.returnValue = "";
    }
    window.addEventListener("beforeunload", handleBeforeUnload);
    return () => window.removeEventListener("beforeunload", handleBeforeUnload);
  }, [isDirty]);
}
