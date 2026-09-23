import { useQueryClient } from "@tanstack/react-query";
import { usePaymentModalStore } from "@/shared/stores/usePaymentModalStore";
import { api } from "@/shared/lib/api";
import { formatTenge } from "@/shared/lib/format";
import { logger } from "@/shared/lib/logger";
import type { ServiceId } from "@/shared/types/tariff";
import type { CheckoutResponseDto, PaymentDto } from "@/shared/types/api";

interface PayForThreadCopy {
  title: string;
  description: string;
  amountField: string;
  confirm: string;
  cancel: string;
  note: string;
}

interface PayForThreadArgs {
  threadId: string;
  serviceId: ServiceId;
  amountTenge: number;
  copy: PayForThreadCopy;
}

/**
 * Общая логика "оплатить уже созданный, но ещё не оплаченный тред"
 * (`kind: "single_service"`, `thread_id` — openapi.yaml#CheckoutRequest) —
 * модалка оплаты в Chat, когда отправка/перезапуск вопроса оставили тред в
 * `awaiting_payment` (баланса не было). Отказ от модалки — тред остаётся в
 * истории «Ожидает оплаты», баланс пополняется в «Тарифах». Живёт в
 * `shared/hooks` как общая логика оплаты треда (single_service + thread_id).
 *
 * Модалка закрывается сразу после успешного `confirm` — ни рефетч кэша,
 * ни ответ ассистента её не задерживают: `confirm` запускает обработку
 * треда в фоне на бэке (`thread.Service.Resume`), ответ стримится в чат по
 * WS (`useThreadSocket`), открытому на треде независимо от оплаты, а
 * инвалидация баланса/треда идёт параллельно, не блокируя UI.
 */
export function useThreadCheckout() {
  const openPayment = usePaymentModalStore((state) => state.open);
  const queryClient = useQueryClient();

  function payForThread({ threadId, serviceId, amountTenge, copy }: PayForThreadArgs) {
    logger.info({
      scope: "billing.thread-payment",
      event: "payment_modal_opened",
      data: { threadId, serviceId },
    });
    openPayment({
      title: copy.title,
      description: copy.description,
      amountLabel: formatTenge(amountTenge),
      amountFieldLabel: copy.amountField,
      confirmLabel: copy.confirm,
      cancelLabel: copy.cancel,
      note: copy.note,
      steps: ["checkout", "confirm"],
      onConfirm: async (reportStep) => {
        reportStep("checkout");
        const checkout = await api.post<CheckoutResponseDto>("/payments/checkout", {
          kind: "single_service",
          items: [{ service_id: serviceId, qty: 1 }],
          thread_id: threadId,
        });
        reportStep("confirm");
        await api.post<PaymentDto>(`/payments/${checkout.payment_id}/confirm`);
        logger.info({
          scope: "billing.thread-payment",
          event: "payment_confirmed",
          data: { threadId, serviceId },
        });
        void queryClient.invalidateQueries({ queryKey: ["balance"] });
        void queryClient.invalidateQueries({ queryKey: ["threads"] });
        void queryClient.invalidateQueries({ queryKey: ["thread", threadId] });
      },
    });
  }

  return { payForThread };
}
