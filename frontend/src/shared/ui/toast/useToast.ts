import { useToastStore } from "./useToastStore";

/** `const toast = useToast(); toast("Сохранено")` / `toast("Не удалось сохранить", "error")` */
export function useToast() {
  return useToastStore((state) => state.push);
}
