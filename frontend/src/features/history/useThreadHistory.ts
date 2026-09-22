import { useMemo, useState } from "react";
import { useDebouncedValue } from "@/shared/hooks/useDebouncedValue";
import { useConfirmModalStore } from "@/shared/stores/useConfirmModalStore";
import { useLangStore } from "@/shared/stores/useLangStore";
import { useToast } from "@/shared/ui";
import { logger } from "@/shared/lib/logger";
import { filterThreads } from "./filterThreads";
import { historyDictionary } from "./locales";
import { THREAD_MOCKS } from "./threads.mocks";
import type { HistoryPeriodFilter, HistoryStatusFilter, Thread } from "./types";

const SEARCH_DEBOUNCE_MS = 300;

/**
 * Данные — мок в локальном стейте до Stage 6 (реальный `/threads` через
 * TanStack Query заменит THREAD_MOCKS/setThreads целиком, не дополнит).
 */
export function useThreadHistory() {
  const lang = useLangStore((state) => state.lang);
  const dictionary = historyDictionary[lang];
  const toast = useToast();
  const openConfirm = useConfirmModalStore((state) => state.open);

  const [threads, setThreads] = useState<Thread[]>(THREAD_MOCKS);
  const [searchInput, setSearchInput] = useState("");
  const [statusFilter, setStatusFilter] = useState<HistoryStatusFilter>("all");
  const [periodFilter, setPeriodFilter] = useState<HistoryPeriodFilter>("all");
  const debouncedSearch = useDebouncedValue(searchInput, SEARCH_DEBOUNCE_MS);

  const filteredThreads = useMemo(
    () =>
      filterThreads({
        threads,
        search: debouncedSearch,
        status: statusFilter,
        period: periodFilter,
      }),
    [threads, debouncedSearch, statusFilter, periodFilter],
  );

  const isFiltered =
    debouncedSearch.trim() !== "" || statusFilter !== "all" || periodFilter !== "all";

  function resetFilters() {
    setSearchInput("");
    setStatusFilter("all");
    setPeriodFilter("all");
  }

  function requestDelete(thread: Thread) {
    logger.info({
      scope: "history.delete",
      event: "delete_requested",
      data: { threadId: thread.id },
    });
    openConfirm({
      title: dictionary.deleteConfirmTitle,
      message: dictionary.deleteConfirmMessage,
      confirmLabel: dictionary.deleteAction,
      cancelLabel: dictionary.cancel,
      onConfirm: () => {
        setThreads((prev) => prev.filter((item) => item.id !== thread.id));
        logger.info({
          scope: "history.delete",
          event: "delete_confirmed",
          data: { threadId: thread.id },
        });
        toast(dictionary.deleteToastSuccess, "success");
      },
    });
  }

  return {
    dictionary,
    threads: filteredThreads,
    hasAnyThreads: threads.length > 0,
    isFiltered,
    searchInput,
    setSearchInput,
    statusFilter,
    setStatusFilter,
    periodFilter,
    setPeriodFilter,
    resetFilters,
    requestDelete,
  };
}
