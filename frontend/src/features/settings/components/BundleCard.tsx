import { Pencil, Trash2 } from "lucide-react";
import { Badge, Card, IconButton } from "@/shared/ui";
import { formatTenge } from "@/shared/lib/format";
import type { BundlePrice } from "@/shared/lib/tariffPricing";
import type { Bundle } from "../types";

interface BundleCardProps {
  bundle: Bundle;
  includesItems: string[];
  price: BundlePrice;
  discountLabel: string;
  editLabel: string;
  deleteLabel: string;
  onEdit: () => void;
  onDelete: () => void;
}

/**
 * Тот же блок, что и у Stage 3 (features/tariffs/components/BundleCard.tsx —
 * название/скидка сверху, маркированный список состава, цена снизу), без кнопки
 * «Купить»: вместо неё изменение/удаление — просто иконки без рамки/фона
 * (`IconButton variant="ghost"`/`"ghost-danger"`, размер иконки уменьшен до
 * 14px), а не кнопки в отдельном обрамлённом блоке поверх карточки, как было
 * раньше. `ghost-danger` — тот же вариант, что уже завела History (Stage 5)
 * для своей кнопки удаления внутри карточки, не новый.
 */
export function BundleCard({
  bundle,
  includesItems,
  price,
  discountLabel,
  editLabel,
  deleteLabel,
  onEdit,
  onDelete,
}: BundleCardProps) {
  return (
    <Card className="flex flex-col">
      <div className="mb-2.5 flex items-start justify-between gap-2">
        <div className="flex min-w-0 flex-wrap items-center gap-2">
          <div className="text-body-sm font-semibold text-ink">{bundle.name}</div>
          {price.hasDiscount && <Badge tone="accent">{discountLabel}</Badge>}
        </div>
        <div className="-mr-2 -mt-2 flex flex-none items-center gap-0.5">
          <IconButton
            aria-label={`${editLabel}: ${bundle.name}`}
            variant="ghost"
            onClick={onEdit}
          >
            <Pencil size={14} aria-hidden="true" />
          </IconButton>
          <IconButton
            aria-label={`${deleteLabel}: ${bundle.name}`}
            variant="ghost-danger"
            onClick={onDelete}
          >
            <Trash2 size={14} aria-hidden="true" />
          </IconButton>
        </div>
      </div>

      <ul className="mb-3.5 flex-1 list-disc space-y-0.5 pl-4 text-caption text-muted">
        {includesItems.map((item) => (
          <li key={item}>{item}</li>
        ))}
      </ul>

      <div className="flex items-baseline gap-2">
        <div className="text-h2 font-bold text-ink">{formatTenge(price.totalTenge)}</div>
        {price.hasDiscount && (
          <div className="text-caption text-muted line-through">
            {formatTenge(price.subtotalTenge)}
          </div>
        )}
      </div>
    </Card>
  );
}
