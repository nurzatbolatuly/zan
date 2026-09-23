import { Card } from "@/shared/ui";
import type { Service, ServiceId } from "../types";
import { ServiceRow } from "./ServiceRow";

interface ServicesSectionProps {
  title: string;
  subtitle: string;
  typeFieldLabel: string;
  nameFieldLabel: string;
  priceFieldLabel: string;
  activeLabel: string;
  inactiveLabel: string;
  toggleActiveLabel: (serviceName: string, willBeActive: boolean) => string;
  services: Service[];
  onChange: (
    id: ServiceId,
    patch: Partial<Pick<Service, "name" | "unitPriceTenge">>,
  ) => void;
  onCommit: (service: Service) => void;
  onToggleActive: (service: Service, isActive: boolean) => void;
}

/**
 * Settings → Тарифы → список услуг (Stage 6). Каталог услуг фиксирован
 * миграцией на бэке (только qa/doc) — нет добавления/удаления, только
 * инлайн-правка цены/названия и переключатель активности.
 */
export function ServicesSection({
  title,
  subtitle,
  typeFieldLabel,
  nameFieldLabel,
  priceFieldLabel,
  activeLabel,
  inactiveLabel,
  toggleActiveLabel,
  services,
  onChange,
  onCommit,
  onToggleActive,
}: ServicesSectionProps) {
  return (
    <Card>
      <div className="mb-4">
        <h2 className="text-h3 text-ink">{title}</h2>
        <p className="text-caption text-muted">{subtitle}</p>
      </div>
      <div className="flex flex-col gap-2.5">
        {services.map((service) => (
          <ServiceRow
            key={service.id}
            service={service}
            typeFieldLabel={typeFieldLabel}
            nameFieldLabel={nameFieldLabel}
            priceFieldLabel={priceFieldLabel}
            activeLabel={activeLabel}
            inactiveLabel={inactiveLabel}
            toggleActiveLabel={toggleActiveLabel}
            onChange={(patch) => onChange(service.id, patch)}
            onCommit={() => onCommit(service)}
            onToggleActive={(isActive) => onToggleActive(service, isActive)}
          />
        ))}
      </div>
    </Card>
  );
}
