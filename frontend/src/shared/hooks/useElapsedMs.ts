import { useEffect, useState } from "react";

/**
 * Сколько миллисекунд прошло с момента, когда `isRunning` стал true;
 * 0, пока не запущен. Каждый новый запуск считается с нуля. Время берётся
 * из `Date.now()`, а не из числа тиков — фоновая вкладка троттлит
 * `setInterval`, и счёт по тикам отставал бы от реального.
 */
export function useElapsedMs(isRunning: boolean, tickMs = 1_000): number {
  const [elapsedMs, setElapsedMs] = useState(0);

  useEffect(() => {
    if (!isRunning) return;
    const startedAt = Date.now();
    const timer = setInterval(() => setElapsedMs(Date.now() - startedAt), tickMs);
    return () => {
      clearInterval(timer);
      setElapsedMs(0);
    };
  }, [isRunning, tickMs]);

  return elapsedMs;
}
