import { useMemo, useRef, useState } from "react";
import { useLangStore } from "@/shared/stores/useLangStore";
import { useConfirmModalStore } from "@/shared/stores/useConfirmModalStore";
import { useToast } from "@/shared/ui";
import { logger } from "@/shared/lib/logger";
import { calcBundlePrice, findBundleUsingService } from "./bundlePricing";
import { settingsDictionary } from "./locales";
import { SERVICE_MOCKS, BUNDLE_MOCKS } from "./mocks";
import type { Bundle, BundleFormResult, Service, ServiceFormResult, ServiceId } from "./types";

type BundleModalState =
  { mode: "closed" } | { mode: "create" } | { mode: "edit"; bundle: Bundle };

/**
 * Данные — мок в локальном стейте до Stage 6 (`/tariffs` admin CRUD,
 * instructions.md «Открытые вопросы» — форма ответа для админки ещё не
 * согласована с бэком). Услуги/тарифы инициализируются один раз из мока
 * текущего языка (`useState`, без ленивого пересчёта по `lang`) — это
 * админские данные, которые пользователь может успеть отредактировать;
 * в отличие от контента чата (instructions.md «Конвенции»), сброс на смену
 * языка молча стёр бы несохранённые правки админа, а не просто перевёл текст.
 */
export function useAdminTariffs() {
  const lang = useLangStore((state) => state.lang);
  const t = settingsDictionary[lang];
  const toast = useToast();
  const openConfirm = useConfirmModalStore((state) => state.open);

  const [services, setServices] = useState<Service[]>(() => SERVICE_MOCKS[lang]);
  const [bundles, setBundles] = useState<Bundle[]>(() => BUNDLE_MOCKS[lang]);
  const [isServiceModalOpen, setServiceModalOpen] = useState(false);
  const [bundleModal, setBundleModal] = useState<BundleModalState>({ mode: "closed" });
  // Растёт при каждом открытии модалки — используется как React `key` на
  // BundleEditModal, чтобы форма пересобиралась с чистыми defaultValues каждый
  // раз (без этого повторное открытие того же тарифа после "Отмена" показало
  // бы недосохранённый черновик прошлой попытки — ключ по одному только id
  // тарифа не менялся бы между двумя открытиями одного и того же тарифа).
  const modalNonceRef = useRef(0);
  // Тот же приём для ServiceCreateModal — растёт на каждое открытие, идёт в
  // React `key`, чтобы повторное "Добавить услугу" после "Отмена" не
  // показывало недосохранённый черновик прошлой попытки.
  const serviceModalNonceRef = useRef(0);

  // Тот же паттерн, что и useTariffs.ts (Stage 3) — витрина считается в хуке
  // через useMemo, компоненты (BundleCard) только рендерят готовые значения
  // (FRONT_CODING_STANDARDS.md §1: "рендер-JSX не должен вперемешку содержать расчёты цены").
  // `includesItems` — массив строк (одна на позицию), не склеенная запятыми
  // строка: карточка тарифа теперь переиспользует вёрстку Stage 3
  // (features/tariffs/components/BundleCard.tsx), где состав — нумерованный список.
  const bundlesView = useMemo(
    () =>
      bundles.map((bundle) => ({
        bundle,
        includesItems: bundle.items
          .filter((item) => item.qty > 0)
          .map((item) => {
            const serviceName =
              services.find((service) => service.id === item.serviceId)?.name ??
              item.serviceId;
            return t.tariffs.qtyLabel(item.serviceId, item.qty, serviceName);
          }),
        discountLabel: t.tariffs.discountLabel(bundle.discountPercent),
        price: calcBundlePrice(bundle.items, services, bundle.discountPercent),
      })),
    [bundles, services, t],
  );

  function updateServiceDraft(
    id: ServiceId,
    patch: Partial<Pick<Service, "name" | "unitPriceTenge">>,
  ) {
    setServices((prev) =>
      prev.map((service) => (service.id === id ? { ...service, ...patch } : service)),
    );
  }

  /** Вызывается по `onBlur` поля — коммит правки как значимое админ-действие (FRONT_CODING_STANDARDS.md §4.3). */
  function commitServiceChange(service: Service) {
    logger.info({
      scope: "settings.tariffs",
      event: "service_updated",
      data: { serviceId: service.id },
    });
    toast(t.tariffs.serviceUpdatedToast, "success");
  }

  function openNewService() {
    serviceModalNonceRef.current += 1;
    setServiceModalOpen(true);
  }

  function closeServiceModal() {
    setServiceModalOpen(false);
  }

  function addService(values: ServiceFormResult) {
    const newService: Service = { id: crypto.randomUUID(), ...values };
    setServices((prev) => [...prev, newService]);
    logger.info({
      scope: "settings.tariffs",
      event: "service_created",
      data: { serviceId: newService.id },
    });
    toast(t.tariffs.serviceCreatedToast, "success");
    setServiceModalOpen(false);
  }

  /**
   * Услугу, которая используется хотя бы в одном тарифе, удалить нельзя — это
   * защита целостности цены (bundlePricing.ts#findBundleUsingService), а не
   * пользовательский выбор, поэтому сразу error-toast, без confirm-модалки.
   */
  function requestDeleteService(service: Service) {
    const blockingBundle = findBundleUsingService(bundles, service.id);
    if (blockingBundle) {
      logger.info({
        scope: "settings.tariffs",
        event: "service_delete_blocked",
        data: { serviceId: service.id, bundleId: blockingBundle.id },
      });
      toast(t.tariffs.serviceInUseError(blockingBundle.name), "error");
      return;
    }

    logger.info({
      scope: "settings.tariffs",
      event: "service_delete_requested",
      data: { serviceId: service.id },
    });
    openConfirm({
      title: t.tariffs.deleteConfirmTitle,
      message: t.tariffs.serviceDeleteConfirmMessage,
      confirmLabel: t.tariffs.deleteAction,
      cancelLabel: t.common.cancel,
      onConfirm: () => {
        setServices((prev) => prev.filter((item) => item.id !== service.id));
        logger.info({
          scope: "settings.tariffs",
          event: "service_deleted",
          data: { serviceId: service.id },
        });
        toast(t.tariffs.serviceDeletedToast, "success");
      },
    });
  }

  function openNewBundle() {
    modalNonceRef.current += 1;
    setBundleModal({ mode: "create" });
  }

  function openEditBundle(bundle: Bundle) {
    modalNonceRef.current += 1;
    setBundleModal({ mode: "edit", bundle });
  }

  function closeBundleModal() {
    setBundleModal({ mode: "closed" });
  }

  function saveBundle(values: BundleFormResult) {
    if (bundleModal.mode === "create") {
      const newBundle: Bundle = { id: crypto.randomUUID(), ...values };
      setBundles((prev) => [...prev, newBundle]);
      logger.info({
        scope: "settings.tariffs",
        event: "bundle_created",
        data: { bundleId: newBundle.id },
      });
    } else if (bundleModal.mode === "edit") {
      const bundleId = bundleModal.bundle.id;
      setBundles((prev) =>
        prev.map((bundle) =>
          bundle.id === bundleId ? { ...bundle, ...values } : bundle,
        ),
      );
      logger.info({
        scope: "settings.tariffs",
        event: "bundle_updated",
        data: { bundleId },
      });
    }
    toast(t.tariffs.savedToast, "success");
    closeBundleModal();
  }

  function requestDeleteBundle(bundle: Bundle) {
    logger.info({
      scope: "settings.tariffs",
      event: "bundle_delete_requested",
      data: { bundleId: bundle.id },
    });
    openConfirm({
      title: t.tariffs.deleteConfirmTitle,
      message: t.tariffs.deleteConfirmMessage,
      confirmLabel: t.tariffs.deleteAction,
      cancelLabel: t.common.cancel,
      onConfirm: () => {
        setBundles((prev) => prev.filter((item) => item.id !== bundle.id));
        logger.info({
          scope: "settings.tariffs",
          event: "bundle_deleted",
          data: { bundleId: bundle.id },
        });
        toast(t.tariffs.deleteToastSuccess, "success");
      },
    });
  }

  return {
    services,
    bundlesView,
    updateServiceDraft,
    commitServiceChange,
    isServiceModalOpen,
    serviceModalKey: `service-${serviceModalNonceRef.current}`,
    openNewService,
    closeServiceModal,
    addService,
    requestDeleteService,
    bundleModal,
    bundleModalKey: `${bundleModal.mode}-${modalNonceRef.current}`,
    openNewBundle,
    openEditBundle,
    closeBundleModal,
    saveBundle,
    requestDeleteBundle,
  };
}
