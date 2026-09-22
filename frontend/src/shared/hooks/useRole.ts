import type { Role } from "@/shared/types/common";

/**
 * Заглушка до реальной роли/авторизации (brief §3.6, PLAN.md §2 и §9).
 * `/settings` уже написан через этот хук, а не через "видно всем" —
 * когда появится настоящая проверка роли, меняется только эта функция.
 */
export function useRole(): Role {
  return "admin";
}
