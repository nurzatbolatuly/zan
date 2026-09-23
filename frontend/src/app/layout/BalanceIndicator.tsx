import { useQuery } from "@tanstack/react-query";
import { api } from "@/shared/lib/api";
import { navDictionary } from "@/shared/locales/nav";
import type { Lang } from "@/shared/types/common";
import type { BalanceEntryDto } from "@/shared/types/api";

/** Баланс — счётчик только для `qa` (brief §3.4), см. instructions.md «Конвенции». */
export function BalanceIndicator({ lang }: { lang: Lang }) {
  const { data } = useQuery({
    queryKey: ["balance"],
    queryFn: () => api.get<BalanceEntryDto[]>("/balance"),
  });
  const t = navDictionary[lang];
  const qaBalance = data?.find((entry) => entry.service_id === "qa")?.quantity ?? 0;

  return (
    <div
      title={t.balance}
      className="hidden items-center rounded-pill border border-line bg-surface px-3 py-1.5 font-mono text-micro text-muted sm:flex"
    >
      {t.consultationsLeft(qaBalance)}
    </div>
  );
}
