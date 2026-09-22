import { create } from "zustand";

interface SessionState {
  sessionId: string | null;
  /** Мок до Stage 6 — реальный баланс придёт с бэка (brief §3.4, gap PLAN.md §2). */
  balance: number;
  setBalance: (balance: number) => void;
}

export const useSessionStore = create<SessionState>((set) => ({
  sessionId: null,
  balance: 3,
  setBalance: (balance) => set({ balance }),
}));
