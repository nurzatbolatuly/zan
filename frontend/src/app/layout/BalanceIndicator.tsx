import { useSessionStore } from "@/shared/stores/useSessionStore";
import { navDictionary } from "@/shared/locales/nav";
import type { Lang } from "@/shared/types/common";

/** Gap из PLAN.md §2 (brief §3.4) — баланс мок до Stage 6, но виден пользователю уже сейчас. */
export function BalanceIndicator({ lang }: { lang: Lang }) {
  const balance = useSessionStore((state) => state.balance);
  const t = navDictionary[lang];

  return (
    <div
      title={t.balance}
      className="hidden items-center rounded-pill border border-line bg-surface px-3 py-1.5 font-mono text-micro text-muted sm:flex"
    >
      {t.consultationsLeft(balance)}
    </div>
  );
}
