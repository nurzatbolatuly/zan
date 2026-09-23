import { Button, EmptyState, Skeleton } from "@/shared/ui";
import { useLangStore } from "@/shared/stores/useLangStore";
import { settingsDictionary } from "../locales";
import { useAdminTariffs } from "../useAdminTariffs";
import { ServicesSection } from "./ServicesSection";
import { BundlesSection } from "./BundlesSection";

/** Settings → Тарифы (админ) — PLAN.md §5 Stage 4b, Stage 6 — реальный `/admin/*` CRUD. */
export function TariffsTab() {
  const lang = useLangStore((state) => state.lang);
  const t = settingsDictionary[lang].tariffs;
  const common = settingsDictionary[lang].common;
  const {
    isLoading,
    isError,
    retry,
    services,
    bundlesView,
    updateServiceDraft,
    commitServiceChange,
    setServiceActive,
    bundleModal,
    bundleModalKey,
    isBundleModalPending,
    openNewBundle,
    openEditBundle,
    closeBundleModal,
    saveBundle,
    requestDeleteBundle,
  } = useAdminTariffs();

  if (isError) {
    return (
      <EmptyState
        title={common.loadError}
        action={<Button onClick={retry}>{common.retry}</Button>}
      />
    );
  }

  if (isLoading) {
    return (
      <div className="flex flex-col gap-3.5">
        <Skeleton className="h-40" />
        <Skeleton className="h-56" />
      </div>
    );
  }

  return (
    <div className="flex flex-col gap-3.5">
      <ServicesSection
        title={t.servicesTitle}
        subtitle={t.servicesSub}
        typeFieldLabel={t.fServiceType}
        nameFieldLabel={t.fServiceName}
        priceFieldLabel={t.fUnitPrice}
        activeLabel={t.serviceActiveLabel}
        inactiveLabel={t.serviceInactiveLabel}
        toggleActiveLabel={t.serviceToggleActiveLabel}
        services={services}
        onChange={updateServiceDraft}
        onCommit={commitServiceChange}
        onToggleActive={setServiceActive}
      />

      <BundlesSection
        title={t.bundlesTitle}
        subtitle={t.bundlesSub}
        addLabel={t.addTariff}
        editLabel={t.editAction}
        deleteLabel={t.deleteAction}
        bundlesView={bundlesView}
        services={services}
        bundleModal={bundleModal}
        bundleModalKey={bundleModalKey}
        isBundleModalPending={isBundleModalPending}
        modalTitles={{ create: t.newTariffTitle, edit: t.editTariffTitle }}
        modalLabels={{
          fName: t.fName,
          fDiscount: t.fDiscount,
          total: t.total,
          save: common.save,
          cancel: common.cancel,
          nameRequiredError: t.nameRequiredError,
          atLeastOneItemError: t.atLeastOneItemError,
          decreaseQuantityLabel: t.decreaseQuantityLabel,
          increaseQuantityLabel: t.increaseQuantityLabel,
        }}
        onOpenNew={openNewBundle}
        onOpenEdit={openEditBundle}
        onDelete={requestDeleteBundle}
        onSave={saveBundle}
        onCloseModal={closeBundleModal}
      />
    </div>
  );
}
