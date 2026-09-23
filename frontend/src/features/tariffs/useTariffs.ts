import { useMemo, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useLangStore } from "@/shared/stores/useLangStore";
import { usePaymentModalStore } from "@/shared/stores/usePaymentModalStore";
import type { PaymentStep } from "@/shared/stores/usePaymentModalStore";
import { useToast } from "@/shared/ui";
import { logger } from "@/shared/lib/logger";
import { formatTenge } from "@/shared/lib/format";
import { api } from "@/shared/lib/api";
import { tariffsDictionary } from "./locales";
import { computeCustomOrderPrice } from "./pricing";
import type { ServiceId, CustomOrderQuantities } from "./types";
import type {
  ServiceDto,
  TariffDto,
  CheckoutRequestDto,
  CheckoutResponseDto,
  PaymentDto,
} from "@/shared/types/api";

const SERVICE_IDS: ServiceId[] = ["qa", "doc"];
const DEFAULT_CUSTOM_ORDER: CustomOrderQuantities = { qa: 1, doc: 1 };
const CUSTOM_ORDER_MAX_QTY = 20;

/**
 * Вся бизнес-логика экрана «Тарифы» (FRONT_CODING_STANDARDS.md §1). Каталог
 * услуг/тарифов — `GET /services`/`GET /tariffs` (Stage 6, TanStack Query) —
 * цена/скидка уже посчитаны бэком (`Tariff.subtotal/total`), фронт их не
 * пересчитывает для витрины (только для live-превью своего набора, где
 * ответа сервера ещё нет, см. `pricing.ts#computeCustomOrderPrice`). Оплата
 * — общая PaymentModal (M3, PLAN.md §1): `POST /payments/checkout` +
 * `.../confirm`, баланс в шапке обновляется инвалидацией `["balance"]`, не
 * ручным инкрементом (баланс — серверная сущность, не Zustand-мок).
 */
export function useTariffs() {
  const lang = useLangStore((state) => state.lang);
  const dictionary = tariffsDictionary[lang];
  const toast = useToast();
  const openPayment = usePaymentModalStore((state) => state.open);
  const queryClient = useQueryClient();

  const servicesQuery = useQuery({
    queryKey: ["services"],
    queryFn: () => api.get<ServiceDto[]>("/services"),
  });
  const tariffsQuery = useQuery({
    queryKey: ["tariffs"],
    queryFn: () => api.get<TariffDto[]>("/tariffs"),
  });

  const [isCustomOrderOpen, setCustomOrderOpen] = useState(false);
  const [customQuantities, setCustomQuantities] =
    useState<CustomOrderQuantities>(DEFAULT_CUSTOM_ORDER);

  const unitPrices = useMemo<Record<ServiceId, number>>(() => {
    const prices: Record<ServiceId, number> = { qa: 0, doc: 0 };
    for (const service of servicesQuery.data ?? []) {
      prices[service.id] = service.price;
    }
    return prices;
  }, [servicesQuery.data]);

  const bundles = useMemo(
    () =>
      (tariffsQuery.data ?? []).map((tariff) => ({
        tariff,
        includesItems: tariff.items.map((item) =>
          dictionary.qtyLabel(item.service_id, item.qty),
        ),
        price: {
          subtotalTenge: tariff.subtotal,
          totalTenge: tariff.total,
          hasDiscount: tariff.discount_percent > 0,
        },
      })),
    [tariffsQuery.data, dictionary],
  );

  const customTotalTenge = useMemo(
    () => computeCustomOrderPrice(customQuantities, unitPrices),
    [customQuantities, unitPrices],
  );

  async function checkoutAndConfirm(
    body: CheckoutRequestDto,
    reportStep: (step: PaymentStep) => void,
  ): Promise<void> {
    reportStep("checkout");
    const checkout = await api.post<CheckoutResponseDto>("/payments/checkout", body);
    reportStep("confirm");
    await api.post<PaymentDto>(`/payments/${checkout.payment_id}/confirm`);
    await queryClient.invalidateQueries({ queryKey: ["balance"] });
  }

  function buyBundle(tariffId: string, name: string, totalTenge: number): void {
    logger.info({
      scope: "tariffs.payment",
      event: "payment_modal_opened",
      data: { tariffId },
    });
    openPayment({
      title: dictionary.payTitleBundle(name),
      description: dictionary.payDescriptionBundle,
      amountLabel: formatTenge(totalTenge),
      amountFieldLabel: dictionary.payAmountField,
      confirmLabel: dictionary.payConfirm,
      cancelLabel: dictionary.payCancel,
      note: dictionary.payNote,
      onConfirm: async (reportStep) => {
        await checkoutAndConfirm({ kind: "tariff", tariff_id: tariffId }, reportStep);
        logger.info({
          scope: "tariffs.payment",
          event: "purchase_confirmed",
          data: { tariffId },
        });
        toast(dictionary.purchaseSuccessToast(name), "success");
      },
    });
  }

  function openCustomOrder(): void {
    logger.info({ scope: "tariffs.custom_order", event: "opened" });
    setCustomQuantities(DEFAULT_CUSTOM_ORDER);
    setCustomOrderOpen(true);
  }

  function closeCustomOrder(): void {
    setCustomOrderOpen(false);
  }

  function setCustomQuantity(serviceId: ServiceId, qty: number): void {
    setCustomQuantities((prev) => ({ ...prev, [serviceId]: qty }));
  }

  function confirmCustomOrder(): void {
    setCustomOrderOpen(false);
    const items = SERVICE_IDS.filter((id) => customQuantities[id] > 0).map((id) => ({
      service_id: id,
      qty: customQuantities[id],
    }));
    logger.info({
      scope: "tariffs.payment",
      event: "payment_modal_opened",
      data: { kind: "custom", items },
    });
    openPayment({
      title: dictionary.payTitleCustom,
      description: dictionary.payDescriptionCustom,
      amountLabel: formatTenge(customTotalTenge),
      amountFieldLabel: dictionary.payAmountField,
      confirmLabel: dictionary.payConfirm,
      cancelLabel: dictionary.payCancel,
      note: dictionary.payNote,
      onConfirm: async (reportStep) => {
        await checkoutAndConfirm({ kind: "custom", items }, reportStep);
        logger.info({
          scope: "tariffs.payment",
          event: "purchase_confirmed",
          data: { items },
        });
        toast(dictionary.purchaseSuccessToast(dictionary.customTitle), "success");
      },
    });
  }

  return {
    dictionary,
    isLoading: servicesQuery.isLoading || tariffsQuery.isLoading,
    isError: servicesQuery.isError || tariffsQuery.isError,
    retry: () => {
      void servicesQuery.refetch();
      void tariffsQuery.refetch();
    },
    bundles,
    serviceIds: SERVICE_IDS,
    unitPrices,
    customOrderMaxQty: CUSTOM_ORDER_MAX_QTY,
    isCustomOrderOpen,
    openCustomOrder,
    closeCustomOrder,
    customQuantities,
    setCustomQuantity,
    customTotalTenge,
    canConfirmCustomOrder: customTotalTenge > 0,
    confirmCustomOrder,
    buyBundle,
  };
}
