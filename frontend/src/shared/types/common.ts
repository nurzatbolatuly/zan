export type Lang = "ru" | "kz";
export type Theme = "light" | "dark";

/** Заглушка до реальной авторизации — см. shared/hooks/useRole.ts */
export type Role = "user" | "admin";

/** Статусная модель треда (openapi.yaml#ThreadStatus). `awaiting_payment` —
 * вопрос сохранён, но не оплачен: ждёт пополнения баланса в «Тарифах». */
export type ThreadStatus =
  "awaiting_payment" | "processing" | "done" | "error" | "canceled";
