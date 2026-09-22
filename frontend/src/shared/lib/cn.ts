type ClassValue =
  string | number | null | false | undefined | Record<string, boolean | undefined>;

/**
 * Небольшой merge классов без сторонней зависимости (без tailwind-merge) — используется всем UI-китом.
 *
 * Важно: это простая конкатенация строк, а не разрешение конфликтов. Если снаружи передать
 * className с той же утилитой, что уже задаёт компонент (например, свой `h-8` поверх
 * внутреннего `h-11`), выигрывает не порядок в строке класса, а порядок в сгенерированном
 * Tailwind CSS — то есть непредсказуемо для читающего код. Поэтому `className`-проп у
 * компонентов UI-кита — только для добавления (отступы снаружи, ширина, flex), не для
 * переопределения внутренних размеров/цветов. Нужен другой размер — это новый variant/size
 * пропс у компонента, не className-хак.
 */
export function cn(...values: ClassValue[]): string {
  const classes: string[] = [];
  for (const value of values) {
    if (!value) continue;
    if (typeof value === "string" || typeof value === "number") {
      classes.push(String(value));
      continue;
    }
    for (const [key, enabled] of Object.entries(value)) {
      if (enabled) classes.push(key);
    }
  }
  return classes.join(" ");
}
