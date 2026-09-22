import { Button, Card } from "@/shared/ui";
import type { TariffsDictionary } from "../locales";

interface CustomOrderCardProps {
  dictionary: TariffsDictionary;
  onOpen: () => void;
}

/**
 * «Свой набор» — та же форма карточки, что и BundleCard (Card, flex-col,
 * кнопка на всю ширину внизу), но с маркетинговым акцентом на фоне
 * (`Card tone="spotlight"` — анимированный градиент-микс, FRONT_DESIGN_SYSTEM.md
 * §1), чтобы выделить его среди готовых пакетов, не ломая ритм сетки отдельным блоком.
 */
export function CustomOrderCard({ dictionary, onOpen }: CustomOrderCardProps) {
  return (
    <Card tone="spotlight" className="flex flex-col">
      <div className="mb-2.5 text-body-sm font-semibold text-ink">
        {dictionary.customTitle}
      </div>
      <p className="mb-3.5 flex-1 text-caption text-muted">{dictionary.customSubtitle}</p>
      <Button onClick={onOpen} className="w-full">
        {dictionary.customCta}
      </Button>
    </Card>
  );
}
