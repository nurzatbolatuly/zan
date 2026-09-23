import type { ChangeEvent } from "react";
import { Input, Select } from "@/shared/ui";
import type { HistoryPeriodFilter, HistoryStatusFilter } from "./types";
import { historyDictionary } from "./locales";
import type { Lang, ThreadStatus } from "@/shared/types/common";

const STATUS_ORDER: ThreadStatus[] = [
  "awaiting_payment",
  "processing",
  "done",
  "error",
  "canceled",
];

const PERIOD_ORDER: HistoryPeriodFilter[] = ["today", "yesterday", "7d", "30d"];

interface HistoryFiltersProps {
  lang: Lang;
  search: string;
  onSearchChange: (value: string) => void;
  status: HistoryStatusFilter;
  onStatusChange: (value: HistoryStatusFilter) => void;
  period: HistoryPeriodFilter;
  onPeriodChange: (value: HistoryPeriodFilter) => void;
}

export function HistoryFilters({
  lang,
  search,
  onSearchChange,
  status,
  onStatusChange,
  period,
  onPeriodChange,
}: HistoryFiltersProps) {
  const dictionary = historyDictionary[lang];

  return (
    <div className="mb-4 flex flex-wrap gap-3">
      <Input
        type="search"
        aria-label={dictionary.searchLabel}
        placeholder={dictionary.searchPlaceholder}
        value={search}
        onChange={(event: ChangeEvent<HTMLInputElement>) =>
          onSearchChange(event.target.value)
        }
        className="min-w-[220px] flex-1"
      />
      <Select
        aria-label={dictionary.filterLabel}
        value={status}
        onChange={(event) => onStatusChange(event.target.value as HistoryStatusFilter)}
        className="w-auto min-w-[180px]"
      >
        <option value="all">{dictionary.filterAll}</option>
        {STATUS_ORDER.map((statusKey) => (
          <option key={statusKey} value={statusKey}>
            {dictionary.statusLabel[statusKey]}
          </option>
        ))}
      </Select>
      <Select
        aria-label={dictionary.periodFilterLabel}
        value={period}
        onChange={(event) => onPeriodChange(event.target.value as HistoryPeriodFilter)}
        className="w-auto min-w-[160px]"
      >
        <option value="all">{dictionary.periodLabel.all}</option>
        {PERIOD_ORDER.map((periodKey) => (
          <option key={periodKey} value={periodKey}>
            {dictionary.periodLabel[periodKey]}
          </option>
        ))}
      </Select>
    </div>
  );
}
