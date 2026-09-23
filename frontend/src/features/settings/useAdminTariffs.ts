import { useEffect, useMemo, useRef, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useLangStore } from "@/shared/stores/useLangStore";
import { useConfirmModalStore } from "@/shared/stores/useConfirmModalStore";
import { useToast } from "@/shared/ui";
import { logger } from "@/shared/lib/logger";
import { api } from "@/shared/lib/api";
import { describeApiError } from "@/shared/lib/apiErrorMessages";
import { calcBundlePrice } from "./bundlePricing";
import { settingsDictionary } from "./locales";
import type { Bundle, BundleFormResult, Service, ServiceId } from "./types";
import type { ServiceDto, TariffDto, TariffWriteRequestDto } from "@/shared/types/api";

type BundleModalState =
  { mode: "closed" } | { mode: "create" } | { mode: "edit"; bundle: Bundle };

function mapService(dto: ServiceDto): Service {
  return {
    id: dto.id,
    typeLabel: dto.type_label,
    name: dto.name,
    unitPriceTenge: dto.price,
    isActive: dto.is_active,
  };
}

function mapBundle(dto: TariffDto): Bundle {
  return {
    id: dto.id,
    name: dto.name,
    discountPercent: dto.discount_percent,
    items: dto.items.map((item) => ({ serviceId: item.service_id, qty: item.qty })),
  };
}

/**
 * Данные — реальный `/admin/services`/`/admin/tariffs` CRUD (Stage 6, требует
 * `X-Admin-Token` — см. `AdminGate.tsx`). Каталог услуг фиксирован миграцией
 * (только qa/doc, `PUT /admin/services/{id}` — только `price`/`is_active`,
 * бэк не даёт ни создавать, ни удалять услугу) — в отличие от Stage 4b на
 * моках, здесь нет `addService`/`requestDeleteService`, вместо удаления —
 * переключатель активности (`setServiceActive`).
 */
export function useAdminTariffs() {
  const lang = useLangStore((state) => state.lang);
  const t = settingsDictionary[lang];
  const toast = useToast();
  const openConfirm = useConfirmModalStore((state) => state.open);
  const queryClient = useQueryClient();

  const servicesQuery = useQuery({
    queryKey: ["admin", "services"],
    queryFn: () => api.get<ServiceDto[]>("/admin/services", { admin: true }),
  });
  const tariffsQuery = useQuery({
    queryKey: ["admin", "tariffs"],
    queryFn: () => api.get<TariffDto[]>("/admin/tariffs", { admin: true }),
  });

  // Локальная копия — инлайн-редактирование цены идёт по каждому нажатию
  // клавиши (UI-состояние поля до коммита на blur, FRONT_CODING_STANDARDS.md
  // §2), сервер — источник истины только на момент первой успешной загрузки
  // (повторный resync после ошибки — см. commitServiceChange).
  const [services, setServices] = useState<Service[] | null>(null);
  useEffect(() => {
    if (servicesQuery.data && services === null) {
      setServices(servicesQuery.data.map(mapService));
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [servicesQuery.data]);

  const bundles = useMemo(
    () => (tariffsQuery.data ?? []).map(mapBundle),
    [tariffsQuery.data],
  );

  const [isBundleModalPending, setBundleModalPending] = useState(false);
  const [bundleModal, setBundleModal] = useState<BundleModalState>({ mode: "closed" });
  // Растёт при каждом открытии модалки — используется как React `key` на
  // BundleEditModal, чтобы форма пересобиралась с чистыми defaultValues каждый
  // раз (без этого повторное открытие того же тарифа после "Отмена" показало
  // бы недосохранённый черновик прошлой попытки).
  const modalNonceRef = useRef(0);

  const bundlesView = useMemo(
    () =>
      bundles.map((bundle) => ({
        bundle,
        includesItems: bundle.items
          .filter((item) => item.qty > 0)
          .map((item) => {
            const serviceName =
              (services ?? []).find((service) => service.id === item.serviceId)?.name ??
              item.serviceId;
            return t.tariffs.qtyLabel(item.serviceId, item.qty, serviceName);
          }),
        discountLabel: t.tariffs.discountLabel(bundle.discountPercent),
        price: calcBundlePrice(bundle.items, services ?? [], bundle.discountPercent),
      })),
    [bundles, services, t],
  );

  function updateServiceDraft(
    id: ServiceId,
    patch: Partial<Pick<Service, "name" | "unitPriceTenge">>,
  ) {
    setServices((prev) =>
      (prev ?? []).map((service) =>
        service.id === id ? { ...service, ...patch } : service,
      ),
    );
  }

  async function putService(service: Service): Promise<void> {
    await api.put(
      `/admin/services/${service.id}`,
      { price: service.unitPriceTenge, is_active: service.isActive },
      { admin: true },
    );
  }

  /** Вызывается по `onBlur` поля — коммит правки как значимое админ-действие (FRONT_CODING_STANDARDS.md §4.3). */
  async function commitServiceChange(service: Service) {
    logger.info({
      scope: "settings.tariffs",
      event: "service_update_requested",
      data: { serviceId: service.id },
    });
    try {
      await putService(service);
      logger.info({
        scope: "settings.tariffs",
        event: "service_updated",
        data: { serviceId: service.id },
      });
      toast(t.tariffs.serviceUpdatedToast, "success");
    } catch (error) {
      logger.error({ scope: "settings.tariffs", event: "service_update_failed", error });
      toast(describeApiError(error, lang), "error");
      // Откатываемся к последней известной серверной правде — иначе поле
      // молча продолжит показывать значение, которое не сохранилось.
      setServices(null);
      void servicesQuery.refetch();
    }
  }

  async function setServiceActive(service: Service, isActive: boolean) {
    const updated = { ...service, isActive };
    setServices((prev) =>
      (prev ?? []).map((item) => (item.id === service.id ? updated : item)),
    );
    await commitServiceChange(updated);
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

  async function saveBundle(values: BundleFormResult) {
    const body: TariffWriteRequestDto = {
      name: values.name,
      discount_percent: values.discountPercent,
      items: values.items.map((item) => ({ service_id: item.serviceId, qty: item.qty })),
    };
    setBundleModalPending(true);
    try {
      if (bundleModal.mode === "create") {
        await api.post("/admin/tariffs", body, { admin: true });
        logger.info({ scope: "settings.tariffs", event: "bundle_created" });
      } else if (bundleModal.mode === "edit") {
        await api.put(`/admin/tariffs/${bundleModal.bundle.id}`, body, { admin: true });
        logger.info({
          scope: "settings.tariffs",
          event: "bundle_updated",
          data: { bundleId: bundleModal.bundle.id },
        });
      }
      await queryClient.invalidateQueries({ queryKey: ["admin", "tariffs"] });
      await queryClient.invalidateQueries({ queryKey: ["tariffs"] });
      toast(t.tariffs.savedToast, "success");
      closeBundleModal();
    } catch (error) {
      logger.error({ scope: "settings.tariffs", event: "bundle_save_failed", error });
      toast(describeApiError(error, lang), "error");
    } finally {
      setBundleModalPending(false);
    }
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
      onConfirm: async () => {
        await api.delete(`/admin/tariffs/${bundle.id}`, { admin: true });
        logger.info({
          scope: "settings.tariffs",
          event: "bundle_deleted",
          data: { bundleId: bundle.id },
        });
        await queryClient.invalidateQueries({ queryKey: ["admin", "tariffs"] });
        await queryClient.invalidateQueries({ queryKey: ["tariffs"] });
        toast(t.tariffs.deleteToastSuccess, "success");
      },
    });
  }

  return {
    isLoading: servicesQuery.isLoading || tariffsQuery.isLoading,
    isError: servicesQuery.isError || tariffsQuery.isError,
    retry: () => {
      void servicesQuery.refetch();
      void tariffsQuery.refetch();
    },
    services: services ?? [],
    bundlesView,
    updateServiceDraft,
    commitServiceChange,
    setServiceActive,
    bundleModal,
    bundleModalKey: `${bundleModal.mode}-${modalNonceRef.current}`,
    isBundleModalPending,
    openNewBundle,
    openEditBundle,
    closeBundleModal,
    saveBundle,
    requestDeleteBundle,
  };
}
