import { useRef } from "react";
import type { ChangeEvent, FocusEvent } from "react";
import { Power, PowerOff } from "lucide-react";
import { Badge, IconButton, Input } from "@/shared/ui";
import type { Service } from "../types";

interface ServiceRowProps {
  service: Service;
  typeFieldLabel: string;
  nameFieldLabel: string;
  priceFieldLabel: string;
  activeLabel: string;
  inactiveLabel: string;
  toggleActiveLabel: (serviceName: string, willBeActive: boolean) => string;
  onChange: (patch: Partial<Pick<Service, "name" | "unitPriceTenge">>) => void;
  /** Значимое действие — логируется/показывает тост, только если значение реально изменилось (FRONT_CODING_STANDARDS.md §4.3). */
  onCommit: () => void;
  onToggleActive: (isActive: boolean) => void;
}

/**
 * Инлайн-редактирование цены/названия услуги — коммит на `blur`, а не на
 * каждое нажатие клавиши. "Изменилось ли значение" решается здесь, локально
 * в компоненте (значение на момент фокуса — чисто UI-состояние поля до
 * сабмита, FRONT_CODING_STANDARDS.md §2), а не в хуке. Каталог услуг
 * фиксирован миграцией на бэке (Stage 6) — вместо удаления услуги здесь
 * теперь переключатель активности (`PUT /admin/services/{id}` умеет только
 * `price`/`is_active`, не create/delete).
 */
export function ServiceRow({
  service,
  typeFieldLabel,
  nameFieldLabel,
  priceFieldLabel,
  activeLabel,
  inactiveLabel,
  toggleActiveLabel,
  onChange,
  onCommit,
  onToggleActive,
}: ServiceRowProps) {
  const valueAtFocusRef = useRef<string | number>("");

  function handleFocus(event: FocusEvent<HTMLInputElement>) {
    valueAtFocusRef.current = event.target.value;
  }

  function handleBlur(event: FocusEvent<HTMLInputElement>) {
    if (event.target.value !== String(valueAtFocusRef.current)) onCommit();
  }

  return (
    <div className="grid items-end gap-3 rounded-lg border border-line p-3.5 sm:grid-cols-[1fr_1.4fr_140px_auto_44px]">
      <div>
        {/* Тот же стиль лейбла, что у Input.tsx — раньше этот лейбл был мельче/серее
            (font-mono uppercase micro), выбивался рядом с соседними полями строки. */}
        <span className="mb-1 block text-body-sm font-semibold text-ink">
          {typeFieldLabel}
        </span>
        <div className="flex h-11 items-center text-body-sm text-ink">
          {service.typeLabel}
        </div>
      </div>
      <Input
        label={nameFieldLabel}
        value={service.name}
        onFocus={handleFocus}
        onChange={(event: ChangeEvent<HTMLInputElement>) =>
          onChange({ name: event.target.value })
        }
        onBlur={handleBlur}
      />
      <Input
        type="number"
        inputMode="numeric"
        label={priceFieldLabel}
        value={service.unitPriceTenge}
        onFocus={handleFocus}
        onChange={(event: ChangeEvent<HTMLInputElement>) =>
          onChange({ unitPriceTenge: Number(event.target.value) || 0 })
        }
        onBlur={handleBlur}
      />
      <div className="flex h-11 items-center">
        <Badge tone={service.isActive ? "accent" : "muted"}>
          {service.isActive ? activeLabel : inactiveLabel}
        </Badge>
      </div>
      <IconButton
        aria-label={toggleActiveLabel(service.name, !service.isActive)}
        variant={service.isActive ? "ghost-danger" : "ghost"}
        onClick={() => onToggleActive(!service.isActive)}
      >
        {service.isActive ? (
          <Power size={14} aria-hidden="true" />
        ) : (
          <PowerOff size={14} aria-hidden="true" />
        )}
      </IconButton>
    </div>
  );
}
