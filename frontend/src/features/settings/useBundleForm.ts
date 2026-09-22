import { useMemo } from "react";
import { useForm, useWatch } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { calcBundlePrice } from "./bundlePricing";
import type { Bundle, BundleFormResult, Service } from "./types";

export interface BundleFormValues {
  name: string;
  discountPercent: number;
  /** Ключ — `Service.id`, значение — количество в наборе (0 = не входит в тариф). */
  quantities: Record<string, number>;
}

interface UseBundleFormArgs {
  services: Service[];
  initialBundle: Bundle | null;
  nameRequiredError: string;
  atLeastOneItemError: string;
  onSave: (values: BundleFormResult) => void;
}

function buildDefaultValues(
  services: Service[],
  bundle: Bundle | null,
): BundleFormValues {
  const quantities: Record<string, number> = {};
  services.forEach((service) => {
    quantities[service.id] =
      bundle?.items.find((item) => item.serviceId === service.id)?.qty ?? 0;
  });
  return {
    name: bundle?.name ?? "",
    discountPercent: bundle?.discountPercent ?? 0,
    quantities,
  };
}

/**
 * Форма редактора тарифа (M2, PLAN.md §5 Stage 4b) — react-hook-form + zod,
 * live-пересчёт суммы через `useWatch` + `calcBundlePrice` (чистая функция,
 * протестирована отдельно в `bundlePricing.test.ts`), без глобального
 * `state`-объекта прототипа (Zan.dc.html:1024-1042).
 */
export function useBundleForm({
  services,
  initialBundle,
  nameRequiredError,
  atLeastOneItemError,
  onSave,
}: UseBundleFormArgs) {
  const schema = useMemo(
    () =>
      z
        .object({
          name: z.string().trim().min(1, nameRequiredError),
          discountPercent: z.number().min(0).max(90),
          quantities: z.record(z.string(), z.number().min(0).max(20)),
        })
        .refine((values) => Object.values(values.quantities).some((qty) => qty > 0), {
          message: atLeastOneItemError,
          path: ["quantities"],
        }),
    [nameRequiredError, atLeastOneItemError],
  );

  const form = useForm<BundleFormValues>({
    resolver: zodResolver(schema),
    // Ключ формы задаётся снаружи (BundleEditModal размонтирует/монтирует форму
    // заново при смене bundle — см. компонент), defaultValues читаются один раз при монтировании.
    defaultValues: buildDefaultValues(services, initialBundle),
  });

  const watched = useWatch({ control: form.control });

  const price = useMemo(() => {
    const items = services.map((service) => ({
      serviceId: service.id,
      qty: watched.quantities?.[service.id] ?? 0,
    }));
    return calcBundlePrice(items, services, watched.discountPercent ?? 0);
  }, [services, watched.quantities, watched.discountPercent]);

  function submit(values: BundleFormValues) {
    const items = services
      .map((service) => ({
        serviceId: service.id,
        qty: values.quantities[service.id] ?? 0,
      }))
      .filter((item) => item.qty > 0);
    onSave({ name: values.name.trim(), discountPercent: values.discountPercent, items });
  }

  return {
    ...form,
    price,
    submitForm: form.handleSubmit(submit),
  };
}
