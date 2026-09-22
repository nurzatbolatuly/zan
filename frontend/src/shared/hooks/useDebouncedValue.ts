import { useEffect, useState } from "react";

/**
 * Общий дебаунс значения (поиск по истории, будущие фильтры в других фичах —
 * PLAN.md Stage 2). Не привязан к домену, поэтому живёт в shared/hooks,
 * а не в features/history.
 */
export function useDebouncedValue<T>(value: T, delayMs: number): T {
  const [debounced, setDebounced] = useState(value);

  useEffect(() => {
    const timer = setTimeout(() => setDebounced(value), delayMs);
    return () => clearTimeout(timer);
  }, [value, delayMs]);

  return debounced;
}
