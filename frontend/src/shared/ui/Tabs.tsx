import { cn } from "@/shared/lib/cn";

export interface TabItem {
  key: string;
  label: string;
}

interface TabsProps {
  items: TabItem[];
  activeKey: string;
  onChange: (key: string) => void;
  className?: string;
  /**
   * "column" (по умолчанию) — вертикальный список, как было.
   * "sidebar" — горизонтальный скроллящийся ряд на мобильном (нет места под
   * боковую колонку), вертикальная боковая колонка от `lg` — Settings, PLAN.md
   * §7 (breakpoints: "lg — Settings: боковые табы + контент рядом"). Новый
   * вариант, а не className-переопределение layout (правило «className — только
   * для добавления», см. instructions.md «Конвенции»).
   */
  layout?: "column" | "sidebar";
}

export function Tabs({
  items,
  activeKey,
  onChange,
  className,
  layout = "column",
}: TabsProps) {
  return (
    <div
      role="tablist"
      className={cn(
        "flex gap-1.5 overflow-x-auto",
        layout === "column" && "flex-col gap-0.5",
        layout === "sidebar" &&
          "lg:w-[172px] lg:flex-none lg:flex-col lg:gap-0.5 lg:overflow-visible",
        className,
      )}
    >
      {items.map((item) => {
        const isActive = item.key === activeKey;
        return (
          <button
            key={item.key}
            type="button"
            role="tab"
            aria-selected={isActive}
            onClick={() => onChange(item.key)}
            className={cn(
              "h-11 flex-none rounded-md px-3.5 text-left text-body-sm font-semibold transition-colors",
              isActive
                ? "bg-accent-soft text-accent"
                : "bg-transparent text-muted hover:text-ink",
            )}
          >
            {item.label}
          </button>
        );
      })}
    </div>
  );
}
