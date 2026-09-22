import { useEffect, useRef } from "react";

const MAX_HEIGHT_PX = 120; // Zan.dc.html:415 — max-height:120px композера

/**
 * Textarea композера растёт по контенту до MAX_HEIGHT_PX, дальше — свой скролл.
 * Высота — рантайм-значение (зависит от текста, не от темы/брейкпоинта), поэтому
 * считается в JS, а не берётся из design-токенов (FRONT_DESIGN_SYSTEM.md — токены
 * фиксируют статичные значения, не то, что вычисляется по контенту).
 */
export function useAutoResizeTextarea(value: string) {
  const ref = useRef<HTMLTextAreaElement>(null);

  useEffect(() => {
    const node = ref.current;
    if (!node) return;
    node.style.height = "auto";
    node.style.height = `${Math.min(node.scrollHeight, MAX_HEIGHT_PX)}px`;
  }, [value]);

  return ref;
}
