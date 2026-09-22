import { useLangStore } from "@/shared/stores/useLangStore";
import { settingsDictionary } from "../locales";
import { useAdminTariffs } from "../useAdminTariffs";
import { ServicesSection } from "./ServicesSection";
import { BundlesSection } from "./BundlesSection";

/** Settings → Тарифы (админ) — PLAN.md §5 Stage 4b. */
export function TariffsTab() {
  const lang = useLangStore((state) => state.lang);
  const t = settingsDictionary[lang].tariffs;
  const {
    services,
    bundlesView,
    updateServiceDraft,
    commitServiceChange,
    isServiceModalOpen,
    serviceModalKey,
    openNewService,
    closeServiceModal,
    addService,
    requestDeleteService,
    bundleModal,
    bundleModalKey,
    openNewBundle,
    openEditBundle,
    closeBundleModal,
    saveBundle,
    requestDeleteBundle,
  } = useAdminTariffs();

  return (
    <div className="flex flex-col gap-3.5">
      <ServicesSection
        title={t.servicesTitle}
        subtitle={t.servicesSub}
        typeFieldLabel={t.fServiceType}
        nameFieldLabel={t.fServiceName}
        priceFieldLabel={t.fUnitPrice}
        addLabel={t.addServiceLabel}
        deleteLabel={t.deleteAction}
        services={services}
        onChange={updateServiceDraft}
        onCommit={commitServiceChange}
        onDelete={requestDeleteService}
        isCreateModalOpen={isServiceModalOpen}
        createModalKey={serviceModalKey}
        modalTitle={t.newServiceTitle}
        modalLabels={{
          save: settingsDictionary[lang].common.save,
          cancel: settingsDictionary[lang].common.cancel,
          typeRequiredError: t.serviceTypeRequiredError,
          nameRequiredError: t.serviceNameRequiredError,
          priceRequiredError: t.servicePriceRequiredError,
        }}
        onOpenCreate={openNewService}
        onCloseCreate={closeServiceModal}
        onCreate={addService}
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
        modalTitles={{ create: t.newTariffTitle, edit: t.editTariffTitle }}
        modalLabels={{
          fName: t.fName,
          fDiscount: t.fDiscount,
          total: t.total,
          save: settingsDictionary[lang].common.save,
          cancel: settingsDictionary[lang].common.cancel,
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
