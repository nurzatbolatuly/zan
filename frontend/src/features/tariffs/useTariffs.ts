import { useMemo, useState } from "react";
import { useLangStore } from "@/shared/stores/useLangStore";
import { usePaymentModalStore } from "@/shared/stores/usePaymentModalStore";
import { useSessionStore } from "@/shared/stores/useSessionStore";
import { useToast } from "@/shared/ui";
import { logger } from "@/shared/lib/logger";
import { formatTenge } from "@/shared/lib/format";
import { tariffsDictionary } from "./locales";
import { BUNDLES, SERVICE_PRICES } from "./mocks";
import { computeBundlePrice, computeCustomOrderPrice, quantityOf } from "./pricing";
import type { BundleTariff, BuiltInServiceId, CustomOrderQuantities } from "./types";

const SERVICE_IDS: BuiltInServiceId[] = ["qa", "doc"];
const DEFAULT_CUSTOM_ORDER: CustomOrderQuantities = { qa: 1, doc: 1 };

/**
 * Вся бизнес-логика экрана «Тарифы» (FRONT_CODING_STANDARDS.md §1). Пакеты и цены —
 * мок (mocks.ts) до Stage 6 (`/tariffs`). Оплата — общая PaymentModal (M3, PLAN.md
 * §1), тот же стор, что уже открывает Chat (useChatThread.ts) — второй реальный
 * потребитель usePaymentModalStore, паттерн не меняется.
 */
export function useTariffs() {
  const lang = useLangStore((state) => state.lang);
  const dictionary = tariffsDictionary[lang];
  const toast = useToast();
  const openPayment = usePaymentModalStore((state) => state.open);

  const [isCustomOrderOpen, setCustomOrderOpen] = useState(false);
  const [customQuantities, setCustomQuantities] =
    useState<CustomOrderQuantities>(DEFAULT_CUSTOM_ORDER);

  const bundles = useMemo(
    () =>
      BUNDLES.map((bundle) => ({
        bundle,
        name: dictionary.bundleName[bundle.id] ?? bundle.id,
        // `BundleItem.serviceId` — открытый shared-тип (Settings может завести
        // произвольную услугу), но эта страница знает, что её собственный
        // BUNDLES-мок (mocks.ts) всегда состоит только из "qa"/"doc".
        includesItems: bundle.items.map((item) =>
          dictionary.qtyLabel(item.serviceId as BuiltInServiceId, item.qty),
        ),
        price: computeBundlePrice(bundle, SERVICE_PRICES),
      })),
    [dictionary],
  );

  const customTotalTenge = useMemo(
    () => computeCustomOrderPrice(customQuantities, SERVICE_PRICES),
    [customQuantities],
  );

  // Баланс в шапке считает только консультации (qa) — документы там не отражаются
  // (useSessionStore.balance — мок-заглушка Stage 0 под один счётчик, не кошелёк по услугам).
  function finalizePurchase(label: string, qaQtyPurchased: number): void {
    logger.info({
      scope: "tariffs.payment",
      event: "purchase_confirmed",
      data: { label, qaQtyPurchased },
    });
    if (qaQtyPurchased > 0) {
      const current = useSessionStore.getState().balance;
      useSessionStore.getState().setBalance(current + qaQtyPurchased);
    }
    toast(dictionary.purchaseSuccessToast(label), "success");
  }

  function buyBundle(bundle: BundleTariff, name: string, totalTenge: number): void {
    logger.info({
      scope: "tariffs.payment",
      event: "payment_modal_opened",
      data: { bundleId: bundle.id },
    });
    openPayment({
      title: dictionary.payTitleBundle(name),
      description: dictionary.payDescriptionBundle,
      amountLabel: formatTenge(totalTenge),
      amountFieldLabel: dictionary.payAmountField,
      confirmLabel: dictionary.payConfirm,
      cancelLabel: dictionary.payCancel,
      note: dictionary.payNote,
      onConfirm: () => finalizePurchase(name, quantityOf(bundle.items, "qa")),
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

  function setCustomQuantity(serviceId: BuiltInServiceId, qty: number): void {
    setCustomQuantities((prev) => ({ ...prev, [serviceId]: qty }));
  }

  function confirmCustomOrder(): void {
    setCustomOrderOpen(false);
    logger.info({
      scope: "tariffs.payment",
      event: "payment_modal_opened",
      data: { kind: "custom", quantities: customQuantities },
    });
    openPayment({
      title: dictionary.payTitleCustom,
      description: dictionary.payDescriptionCustom,
      amountLabel: formatTenge(customTotalTenge),
      amountFieldLabel: dictionary.payAmountField,
      confirmLabel: dictionary.payConfirm,
      cancelLabel: dictionary.payCancel,
      note: dictionary.payNote,
      onConfirm: () => finalizePurchase(dictionary.customTitle, customQuantities.qa),
    });
  }

  return {
    dictionary,
    bundles,
    serviceIds: SERVICE_IDS,
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
