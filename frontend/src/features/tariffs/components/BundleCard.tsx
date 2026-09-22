import { Badge, Button, Card } from "@/shared/ui";
import { formatTenge } from "@/shared/lib/format";
import type { BundlePrice } from "../pricing";
import type { TariffsDictionary } from "../locales";

interface BundleCardProps {
  name: string;
  includesItems: string[];
  price: BundlePrice;
  discountPercent: number;
  dictionary: TariffsDictionary;
  onBuy: () => void;
}

export function BundleCard({
  name,
  includesItems,
  price,
  discountPercent,
  dictionary,
  onBuy,
}: BundleCardProps) {
  return (
    <Card className="flex flex-col">
      <div className="mb-2.5 flex items-center justify-between gap-2">
        <div className="text-body-sm font-semibold text-ink">{name}</div>
        {price.hasDiscount && (
          <Badge tone="accent">{dictionary.discountLabel(discountPercent)}</Badge>
        )}
      </div>
      <ul className="mb-3.5 flex-1 list-disc space-y-0.5 pl-4 text-caption text-muted">
        {includesItems.map((item) => (
          <li key={item}>{item}</li>
        ))}
      </ul>
      <div className="mb-3.5 flex items-baseline gap-2">
        <div className="text-h2 font-bold text-ink">
          {formatTenge(price.hasDiscount ? price.totalTenge : price.subtotalTenge)}
        </div>
        {price.hasDiscount && (
          <div className="text-caption text-muted line-through">
            {formatTenge(price.subtotalTenge)}
          </div>
        )}
      </div>
      <Button onClick={onBuy} className="w-full">
        {dictionary.buy}
      </Button>
    </Card>
  );
}
