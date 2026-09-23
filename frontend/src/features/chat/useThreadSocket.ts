import { useEffect, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { openThreadSocket } from "@/shared/lib/ws";
import { logger } from "@/shared/lib/logger";
import type { ThreadSocketEvent } from "@/shared/lib/ws";
import type { ThreadDetailDto } from "@/shared/types/api";

interface UseThreadSocketResult {
  /** Накопленный текст стрима текущего раунда; null — ничего не льётся. */
  streamingText: string | null;
  /** true между thread_status(processing) и answer_done/error этого раунда. */
  isProcessing: boolean;
  latestStatusEvent: ThreadSocketEvent | null;
}

/**
 * Один WS-канал на активно открытый тред (FRONT_CODING_STANDARDS.md —
 * решение "один канал на активный тред", не общий сокет на сессию,
 * подтверждено пользователем при планировании). Открывается на монтировании/
 * смене threadId, закрывается на размонтировании — используется внутри
 * useChatThread.ts, не как провайдер верхнего уровня (SessionBoot уже
 * гарантирует токен к моменту, когда ChatPage монтируется).
 *
 * `streamingText` — локальный React-стейт, осознанное исключение из правила
 * "не дублировать серверный стейт в useState/Zustand для удобства" (§3.2
 * instructions.md): это не копия ничего "durable" на сервере — стирается в
 * момент answer_done/error, когда итоговый MessageDto уже лёг в кэш
 * TanStack Query (единственный источник истины для messages).
 */
export function useThreadSocket(threadId: string | null): UseThreadSocketResult {
  const queryClient = useQueryClient();
  const [streamingText, setStreamingText] = useState<string | null>(null);
  const [isProcessing, setIsProcessing] = useState(false);
  const [latestStatusEvent, setLatestStatusEvent] = useState<ThreadSocketEvent | null>(
    null,
  );

  useEffect(() => {
    if (!threadId) return;
    setStreamingText(null);
    setIsProcessing(false);
    setLatestStatusEvent(null);

    // Сервер сохраняет итог раунда в БД ДО отправки answer_done/error, но
    // GET /threads/{id}, начатый раньше этого (монтирование, возврат фокуса
    // во вкладку), мог вернуться уже после события и перезаписать кэш
    // состоянием без ответа. invalidateQueries отменяет такой запрос (без
    // отката данных) и перечитывает тред — кэш сходится с сервером.
    const reconcileThread = () =>
      void queryClient.invalidateQueries({ queryKey: ["thread", threadId] });

    const handle = openThreadSocket(threadId, {
      onEvent: (event) => {
        setLatestStatusEvent(event);

        switch (event.type) {
          case "thread_status": {
            // "processing" — единственный статус, при котором ассистент
            // реально готовит ответ прямо сейчас; "awaiting_payment"
            // (неоплаченный тред) покрыт отдельным UI
            // (useChatThread.ts#isAwaitingPayment), остальные — терминальны
            // для этого раунда.
            setIsProcessing(event.status === "processing");
            break;
          }
          case "answer_delta": {
            setStreamingText((prev) => (prev ?? "") + event.delta);
            break;
          }
          case "answer_done": {
            setIsProcessing(false);
            setStreamingText(null);
            queryClient.setQueryData<ThreadDetailDto>(["thread", threadId], (prev) =>
              prev
                ? {
                    ...prev,
                    status: event.status,
                    messages: [...prev.messages, event.message],
                  }
                : prev,
            );
            reconcileThread();
            break;
          }
          case "error": {
            setIsProcessing(false);
            setStreamingText(null); // не оставлять обрубленный текст как финальный
            logger.error({
              scope: "chat.ws",
              event: "thread_processing_failed",
              data: { threadId, code: event.code },
              error: event.error_message,
            });
            queryClient.setQueryData<ThreadDetailDto>(["thread", threadId], (prev) =>
              prev ? { ...prev, status: "error" } : prev,
            );
            reconcileThread();
            // Сбой обработки возвращает списанный кредит на сервере.
            void queryClient.invalidateQueries({ queryKey: ["balance"] });
            break;
          }
        }
      },
    });

    return () => handle.close();
  }, [threadId, queryClient]);

  return { streamingText, isProcessing, latestStatusEvent };
}
