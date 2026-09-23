import { create } from "zustand";
import { persist } from "zustand/middleware";

interface SessionState {
  sessionId: string | null;
  sessionToken: string | null;
  setSession: (session: { sessionId: string; sessionToken: string }) => void;
  clearSession: () => void;
}

/**
 * Bearer-токен анонимной сессии (Stage 6, см. instructions.md «Общий словарь
 * FE↔BE» — `Authorization: Session <token>`, не httpOnly-cookie). Персистится
 * в localStorage, чтобы обновление страницы не создавало новую сессию каждый
 * раз. Баланс здесь больше не хранится — он серверный (`GET /balance`,
 * TanStack Query, см. `app/layout/BalanceIndicator.tsx`), дублировать его в
 * Zustand "для удобства" запрещено (`FRONT_CODING_STANDARDS.md` §2).
 */
export const useSessionStore = create<SessionState>()(
  persist(
    (set) => ({
      sessionId: null,
      sessionToken: null,
      setSession: ({ sessionId, sessionToken }) => set({ sessionId, sessionToken }),
      clearSession: () => set({ sessionId: null, sessionToken: null }),
    }),
    { name: "zan.session" },
  ),
);
