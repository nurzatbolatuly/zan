import { BundleCard } from "./components/BundleCard";
import { CustomOrderCard } from "./components/CustomOrderCard";
import { CustomOrderModal } from "./components/CustomOrderModal";
import { CUSTOM_ORDER_MAX_QTY, SERVICE_PRICES } from "./mocks";
import { useTariffs } from "./useTariffs";

/**
 * Stage 3 (PLAN.md §5) — заменяет плейсхолдер Stage 0 целиком
 * (FRONT_CODING_STANDARDS.md §3, «чистая замена, а не наслоение»).
 * Зависит только от Stage 0 (`shared/ui`, `usePaymentModalStore`, `useSessionStore`).
 */
export function TariffsPage() {
  const {
    dictionary,
    bundles,
    serviceIds,
    isCustomOrderOpen,
    openCustomOrder,
    closeCustomOrder,
    customQuantities,
    setCustomQuantity,
    customTotalTenge,
    canConfirmCustomOrder,
    confirmCustomOrder,
    buyBundle,
  } = useTariffs();

  return (
    <div className="max-w-[900px] pt-6">
      <h1 className="mb-1 text-h1 text-ink">{dictionary.title}</h1>
      <p className="mb-5 text-body-sm text-muted">{dictionary.subtitle}</p>

      <div className="mb-5 grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
        {bundles.map(({ bundle, name, includesItems, price }) => (
          <BundleCard
            key={bundle.id}
            name={name}
            includesItems={includesItems}
            price={price}
            discountPercent={bundle.discountPercent}
            dictionary={dictionary}
            onBuy={() =>
              buyBundle(
                bundle,
                name,
                price.hasDiscount ? price.totalTenge : price.subtotalTenge,
              )
            }
          />
        ))}
        <CustomOrderCard dictionary={dictionary} onOpen={openCustomOrder} />
      </div>

      <CustomOrderModal
        open={isCustomOrderOpen}
        onClose={closeCustomOrder}
        dictionary={dictionary}
        serviceIds={serviceIds}
        quantities={customQuantities}
        onQuantityChange={setCustomQuantity}
        totalTenge={customTotalTenge}
        canConfirm={canConfirmCustomOrder}
        onConfirm={confirmCustomOrder}
        maxQty={CUSTOM_ORDER_MAX_QTY}
        unitPrices={SERVICE_PRICES}
      />
    </div>
  );
}
