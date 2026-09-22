import { Minus, Plus } from "lucide-react";
import { IconButton } from "./IconButton";

interface QuantityStepperProps {
  value: number;
  min?: number;
  max?: number;
  onChange: (value: number) => void;
  /** Полный aria-label кнопки уменьшения — локализуется вызывающей фичей, не собирается
   * здесь префиксом "Уменьшить: " (тот префикс был захардкожен на русском — см. instructions.md,
   * этот компонент до Stage 3 не имел реальных потребителей, поэтому это чистая замена, а не наслоение). */
  decreaseLabel: string;
  /** Полный aria-label кнопки увеличения — тот же принцип, см. decreaseLabel. */
  increaseLabel: string;
  /** aria-label самого поля ввода. */
  inputLabel: string;
}

/**
 * Используется в BundleEditModal (Settings→Tariffs, Stage 4b) и CustomOrderModal
 * (Tariffs, Stage 3) — общий атом, а не две похожие реализации.
 * Кнопки 44×44px — крупнее прототипа (Zan.dc.html:462, 30px) намеренно,
 * см. IconButton.
 */
export function QuantityStepper({
  value,
  min = 0,
  max = 99,
  onChange,
  decreaseLabel,
  increaseLabel,
  inputLabel,
}: QuantityStepperProps) {
  const dec = () => onChange(Math.max(min, value - 1));
  const inc = () => onChange(Math.min(max, value + 1));

  return (
    <div className="flex items-center gap-2">
      <IconButton aria-label={decreaseLabel} onClick={dec} disabled={value <= min}>
        <Minus size={16} aria-hidden="true" />
      </IconButton>
      <input
        type="number"
        inputMode="numeric"
        value={value}
        min={min}
        max={max}
        onChange={(event) =>
          onChange(Math.max(min, Math.min(max, Number(event.target.value) || 0)))
        }
        aria-label={inputLabel}
        className="h-11 w-14 rounded-md border border-line bg-bg text-center text-body-sm text-ink outline-none focus:border-accent"
      />
      <IconButton aria-label={increaseLabel} onClick={inc} disabled={value >= max}>
        <Plus size={16} aria-hidden="true" />
      </IconButton>
    </div>
  );
}
