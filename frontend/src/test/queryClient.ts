import { QueryClient } from "@tanstack/react-query";

/**
 * Общий QueryClient для тестов (Stage 6 — первый реальный `useQuery` в
 * фиче) — `retry: false` везде, иначе тест на ошибочный ответ ждал бы
 * встроенные ретраи TanStack Query по несколько секунд. Один хелпер на все
 * фичи, не пересоздавать конфиг в каждом тестовом файле.
 */
export function createTestQueryClient(): QueryClient {
  return new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  });
}
