import { useNavigate } from "react-router-dom";
import { Button, EmptyState, Skeleton } from "@/shared/ui";
import { useLangStore } from "@/shared/stores/useLangStore";
import { HistoryFilters } from "./HistoryFilters";
import { ThreadCard } from "./ThreadCard";
import { useThreadHistory } from "./useThreadHistory";

/**
 * Stage 2 (PLAN.md §5), подключена к реальному `GET /threads` в Stage 6
 * (FRONT_CODING_STANDARDS.md §3, «чистая замена, а не наслоение»).
 */
export function HistoryPage() {
  const navigate = useNavigate();
  const lang = useLangStore((state) => state.lang);
  const {
    dictionary,
    threads,
    isLoading,
    isError,
    retry,
    hasAnyThreads,
    hasMore,
    isLoadingMore,
    loadMore,
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

      {isError && (
        <EmptyState
          title={dictionary.loadError}
          action={<Button onClick={retry}>{dictionary.retry}</Button>}
        />
      )}

      {!isError && isLoading && (
        <div className="flex flex-col gap-2.5">
          {Array.from({ length: 3 }, (_, index) => (
            <Skeleton key={index} className="h-24" />
          ))}
        </div>
      )}

      {!isError && !isLoading && !hasAnyThreads && (
        <EmptyState
          title={dictionary.emptyNoThreadsTitle}
          description={dictionary.emptyNoThreadsBody}
          action={<Button onClick={() => navigate("/")}>{dictionary.askQuestion}</Button>}
        />
      )}

      {!isError && !isLoading && hasAnyThreads && threads.length === 0 && (
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

      {!isError && !isLoading && threads.length > 0 && (
        <div className="flex flex-col gap-2.5">
          {threads.map((thread) => (
            <ThreadCard
              key={thread.id}
              thread={thread}
              lang={lang}
              onDelete={requestDelete}
            />
          ))}
          {hasMore && (
            <Button
              variant="secondary"
              onClick={loadMore}
              disabled={isLoadingMore}
              className="self-center"
            >
              {isLoadingMore ? "…" : dictionary.loadMore}
            </Button>
          )}
        </div>
      )}
    </div>
  );
}
