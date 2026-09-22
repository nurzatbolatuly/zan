import { useNavigate } from "react-router-dom";
import { Button, EmptyState } from "@/shared/ui";
import { useLangStore } from "@/shared/stores/useLangStore";
import { HistoryFilters } from "./HistoryFilters";
import { ThreadCard } from "./ThreadCard";
import { useThreadHistory } from "./useThreadHistory";

/**
 * Stage 2 (PLAN.md §5) — заменяет плейсхолдер Stage 0 целиком
 * (FRONT_CODING_STANDARDS.md §3, «чистая замена, а не наслоение»).
 * Зависит только от Stage 0 (`shared/ui`, `useConfirmModalStore`).
 */
export function HistoryPage() {
  const navigate = useNavigate();
  const lang = useLangStore((state) => state.lang);
  const {
    dictionary,
    threads,
    hasAnyThreads,
    isFiltered,
    searchInput,
    setSearchInput,
    statusFilter,
    setStatusFilter,
    periodFilter,
    setPeriodFilter,
    resetFilters,
    requestDelete,
  } = useThreadHistory();

  return (
    <div className="max-w-[860px] pt-6">
      <h1 className="mb-1.5 text-h1 text-ink">{dictionary.title}</h1>
      <p className="mb-5 text-body text-muted">{dictionary.subtitle}</p>

      {hasAnyThreads && (
        <HistoryFilters
          lang={lang}
          search={searchInput}
          onSearchChange={setSearchInput}
          status={statusFilter}
          onStatusChange={setStatusFilter}
          period={periodFilter}
          onPeriodChange={setPeriodFilter}
        />
      )}

      {!hasAnyThreads && (
        <EmptyState
          title={dictionary.emptyNoThreadsTitle}
          description={dictionary.emptyNoThreadsBody}
          action={<Button onClick={() => navigate("/")}>{dictionary.askQuestion}</Button>}
        />
      )}

      {hasAnyThreads && threads.length === 0 && (
        <EmptyState
          title={dictionary.emptyNoResultsTitle}
          description={dictionary.emptyNoResultsBody}
          action={
            isFiltered ? (
              <Button variant="secondary" onClick={resetFilters}>
                {dictionary.resetFilters}
              </Button>
            ) : undefined
          }
        />
      )}

      {threads.length > 0 && (
        <div className="flex flex-col gap-2.5">
          {threads.map((thread) => (
            <ThreadCard
              key={thread.id}
              thread={thread}
              lang={lang}
              onDelete={requestDelete}
            />
          ))}
        </div>
      )}
    </div>
  );
}
