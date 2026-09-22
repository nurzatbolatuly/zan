export type Lang = "ru" | "kz";
export type Theme = "light" | "dark";

/** Заглушка до реальной авторизации — см. shared/hooks/useRole.ts */
export type Role = "user" | "admin";

/** Статусная модель треда (backend-roadmap.md, brief §3.3) */
export type ThreadStatus =
  "queued" | "processing" | "clarify" | "done" | "error" | "canceled";
