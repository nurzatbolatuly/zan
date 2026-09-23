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

/**
 * WS-адрес выводится из `VITE_API_BASE_URL` (http→ws, https→wss) — отдельного
 * `VITE_WS_*` нет: тот же origin, что REST, соответствует текущей модели
 * деплоя одним доменом (см. instructions.md, раздел «Деплой»).
 */
export function getWsBaseUrl(): string {
  return getApiBaseUrl().replace(/^http/, "ws");
}

/**
 * Лимит размера вложения — зеркало `FILE_MAX_SIZE_BYTES` бэкенда (значения
 * держать равными). Проверяется до `POST /files/upload`: заведомо большой файл
 * иначе ушёл бы на сервер целиком, а тот обрывает приём на лимите — вместо
 * понятного `413 file_too_large` браузер мог бы показать сетевую ошибку.
 * Как и `getApiBaseUrl`, при кривой конфигурации бросает, а не подставляет
 * число молча.
 */
export function getFileMaxSizeBytes(): number {
  const raw = import.meta.env.VITE_FILE_MAX_SIZE_BYTES;
  const value = Number(raw);
  if (!raw || !Number.isSafeInteger(value) || value <= 0) {
    throw new Error(
      "VITE_FILE_MAX_SIZE_BYTES не задан или не положительное целое число байт. " +
        "Локально — .env (см. .env.example), на хостинге — переменные окружения проекта.",
    );
  }
  return value;
}
