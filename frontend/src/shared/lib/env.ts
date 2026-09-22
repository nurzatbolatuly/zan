/**
 * Единственное место чтения `VITE_API_BASE_URL` — не читать `import.meta.env`
 * напрямую в других файлах (см. instructions.md, раздел «Деплой»).
 */

function normalizeBaseUrl(value: string): string {
  // без хвостового слэша — не плодим двойные слэши при склейке с путём в api.ts
  return value.replace(/\/+$/, "");
}

/**
 * Бросает понятную ошибку в момент первого реального похода в сеть, а не
 * молча собирает URL вида "undefined/threads" — так ошибка конфигурации
 * (забыли задать env на хостинге) видна сразу и по делу, а не как мистический
 * 404/CORS-провал через полчаса дебага после деплоя.
 */
export function getApiBaseUrl(): string {
  const raw = import.meta.env.VITE_API_BASE_URL;
  if (!raw) {
    throw new Error(
      "VITE_API_BASE_URL не задан. Локально — .env (см. .env.example), на хостинге — " +
        "переменные окружения проекта. См. instructions.md, раздел «Деплой».",
    );
  }
  return normalizeBaseUrl(raw);
}

/** Для необязательных мест (логирование на /logs/client) — не бросает, просто null. */
export function getApiBaseUrlSafe(): string | null {
  const raw = import.meta.env.VITE_API_BASE_URL;
  return raw ? normalizeBaseUrl(raw) : null;
}
