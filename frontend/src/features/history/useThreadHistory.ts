import { useMemo, useState } from "react";
import {
  useInfiniteQuery,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { useDebouncedValue } from "@/shared/hooks/useDebouncedValue";
import { useConfirmModalStore } from "@/shared/stores/useConfirmModalStore";
import { useLangStore } from "@/shared/stores/useLangStore";
import { useToast } from "@/shared/ui";
import { logger } from "@/shared/lib/logger";
import { api, buildQueryString } from "@/shared/lib/api";
import { filterByPeriod } from "./filterThreads";
import { historyDictionary } from "./locales";
import type { HistoryPeriodFilter, HistoryStatusFilter, Thread } from "./types";
import type { ThreadDto, ThreadListResponseDto } from "@/shared/types/api";

const SEARCH_DEBOUNCE_MS = 300;

function mapThread(dto: ThreadDto): Thread {
  return {
    id: dto.id,
    title: dto.title,
    preview: dto.preview_text,
    status: dto.status,
    updatedAt: dto.last_message_at ?? dto.created_at,
    messageCount: dto.message_count,
  };
}

/**
 * Данные — `GET /threads` (Stage 6, TanStack Query). `status`/`search` уходят
 * на бэк как query-параметры; `period` не поддержан бэком (openapi.yaml) —
 * фильтруется на фронте поверх уже загруженных страниц (см. filterThreads.ts,
 * instructions.md, расхождение №6 плана интеграции). Отдельный лёгкий запрос
 * без фильтров (`threadsAnyQuery`) — только чтобы различить два пустых
 * состояния ("нет тредов вообще" / "нет результатов фильтра"), т.к.
 * `ThreadListResponse.total` самого списка теперь отражает уже
 * отфильтрованный на бэке результат, не общий счётчик.
 */
export function useThreadHistory() {
  const lang = useLangStore((state) => state.lang);
  const dictionary = historyDictionary[lang];
  const toast = useToast();
  const openConfirm = useConfirmModalStore((state) => state.open);
  const queryClient = useQueryClient();

  const [searchInput, setSearchInput] = useState("");
  const [statusFilter, setStatusFilter] = useState<HistoryStatusFilter>("all");
  const [periodFilter, setPeriodFilter] = useState<HistoryPeriodFilter>("all");
  const debouncedSearch = useDebouncedValue(searchInput, SEARCH_DEBOUNCE_MS);

  const threadsAnyQuery = useQuery({
    queryKey: ["threads", "any"],
    queryFn: () => api.get<ThreadListResponseDto>("/threads?page=1"),
  });

  const listQueryKey = [
    "threads",
    { status: statusFilter, search: debouncedSearch },
  ] as const;
  const listQuery = useInfiniteQuery({
    queryKey: listQueryKey,
    queryFn: ({ pageParam }) =>
      api.get<ThreadListResponseDto>(
        `/threads${buildQueryString({
          status: statusFilter === "all" ? undefined : statusFilter,
          search: debouncedSearch || undefined,
          page: pageParam,
        })}`,
      ),
    initialPageParam: 1,
    getNextPageParam: (lastPage) =>
      lastPage.page * lastPage.page_size < lastPage.total ? lastPage.page + 1 : undefined,
  });

  const loadedThreads = useMemo(
    () => (listQuery.data?.pages ?? []).flatMap((page) => page.items.map(mapThread)),
    [listQuery.data],
  );

  const threads = useMemo(
    () => filterByPeriod(loadedThreads, periodFilter),
    [loadedThreads, periodFilter],
  );

  const isFiltered =
    debouncedSearch.trim() !== "" || statusFilter !== "all" || periodFilter !== "all";

  function resetFilters() {
    setSearchInput("");
    setStatusFilter("all");
    setPeriodFilter("all");
  }

  const deleteMutation = useMutation({
    mutationFn: (threadId: string) => api.delete(`/threads/${threadId}`),
  });

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
      onConfirm: async () => {
        await deleteMutation.mutateAsync(thread.id);
        logger.info({
          scope: "history.delete",
          event: "delete_confirmed",
          data: { threadId: thread.id },
        });
        await queryClient.invalidateQueries({ queryKey: ["threads"] });
        toast(dictionary.deleteToastSuccess, "success");
      },
    });
  }

  return {
    dictionary,
    threads,
    isLoading: listQuery.isLoading,
    isError: listQuery.isError,
    retry: () => void listQuery.refetch(),
    hasAnyThreads: (threadsAnyQuery.data?.total ?? 0) > 0,
    hasMore: listQuery.hasNextPage,
    isLoadingMore: listQuery.isFetchingNextPage,
    loadMore: () => void listQuery.fetchNextPage(),
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
