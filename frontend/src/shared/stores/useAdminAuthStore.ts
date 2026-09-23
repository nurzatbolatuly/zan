import { create } from "zustand";
import { persist } from "zustand/middleware";

interface AdminAuthState {
  adminToken: string | null;
  setAdminToken: (token: string) => void;
  clearAdminToken: () => void;
}

/**
 * `X-Admin-Token` для `/admin/*` (Settings, Stage 6) — не привязан к сессии
 * пользователя (BACKEND_PLAN.md §6 п.3, `useRole()` остаётся отдельной
 * фронтовой заглушкой, см. instructions.md «Общий словарь FE↔BE»). Вводится
 * один раз через `AdminGate` (`features/settings/components/AdminGate.tsx`),
 * персистится в localStorage. Очищается при `401 admin_unauthorized`/
 * `429 admin_blocked` — гейт запросит токен заново.
 */
export const useAdminAuthStore = create<AdminAuthState>()(
  persist(
    (set) => ({
      adminToken: null,
      setAdminToken: (adminToken) => set({ adminToken }),
      clearAdminToken: () => set({ adminToken: null }),
    }),
    { name: "zan.adminToken" },
  ),
);
