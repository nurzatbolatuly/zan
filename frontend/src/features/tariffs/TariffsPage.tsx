import { Button, EmptyState, Skeleton } from "@/shared/ui";
import { BundleCard } from "./components/BundleCard";
import { CustomOrderCard } from "./components/CustomOrderCard";
import { CustomOrderModal } from "./components/CustomOrderModal";
import { useTariffs } from "./useTariffs";

/**
 * Stage 3 (PLAN.md §5), подключена к реальному бэку в Stage 6
 * (FRONT_CODING_STANDARDS.md §3, «чистая замена, а не наслоение»).
 */
export function TariffsPage() {
  const {
    dictionary,
    isLoading,
    isError,
    retry,
    bundles,
    serviceIds,
    unitPrices,
    customOrderMaxQty,
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

      {isError ? (
        <EmptyState
          title={dictionary.loadError}
          action={<Button onClick={retry}>{dictionary.retry}</Button>}
        />
      ) : isLoading ? (
        <div className="mb-5 grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
          {Array.from({ length: 5 }, (_, index) => (
            <Skeleton key={index} className="h-56" />
          ))}
        </div>
      ) : (
        <div className="mb-5 grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
          {bundles.map(({ tariff, includesItems, price }) => (
            <BundleCard
              key={tariff.id}
              name={tariff.name}
              includesItems={includesItems}
              price={price}
              discountPercent={tariff.discount_percent}
              dictionary={dictionary}
              onBuy={() =>
                buyBundle(
                  tariff.id,
                  tariff.name,
                  price.hasDiscount ? price.totalTenge : price.subtotalTenge,
                )
              }
            />
          ))}
          <CustomOrderCard dictionary={dictionary} onOpen={openCustomOrder} />
        </div>
      )}

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
        maxQty={customOrderMaxQty}
        unitPrices={unitPrices}
      />
    </div>
  );
}
