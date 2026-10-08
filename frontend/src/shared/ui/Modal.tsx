import { useEffect } from "react";
import { createPortal } from "react-dom";
import type { ReactNode } from "react";
import { useFocusTrap } from "@/shared/hooks/useFocusTrap";
import { cn } from "@/shared/lib/cn";

type ModalZ = "overlay" | "overlay-high" | "overlay-top";
type ModalSize = "md" | "lg";

interface ModalProps {
  open: boolean;
  onClose: () => void;
  children: ReactNode;
  ariaLabel: string;
  /** confirm-модалка всегда поверх остальных — overlay-top (FRONT_DESIGN_SYSTEM.md §6). */
  z?: ModalZ;
  /** "lg" — для просмотра документа (Settings → Шаблоны), всё остальное — "md". */
  size?: ModalSize;
}

// Классы должны быть статичными строками, чтобы Tailwind их увидел при сканировании —
// собирать "z-" + переменная в шаблонной строке нельзя (PLAN.md/DESIGN_SYSTEM правило про токены).
const Z_CLASS: Record<ModalZ, string> = {
  overlay: "z-overlay",
  "overlay-high": "z-overlay-high",
  "overlay-top": "z-overlay-top",
};

const SIZE_CLASS: Record<ModalSize, string> = {
  md: "max-w-[440px]",
  lg: "max-w-[960px]",
};

/**
 * Базовый примитив модалки: портал в body, focus-trap, Esc закрывает,
 * клик по оверлею закрывает, скролл body блокируется, aria-modal.
 * Всегда по центру экрана (по требованию пользователя — все модалки
 * приложения центрированы, не низовым шитом; см. instructions.md
 * «Конвенции»). ConfirmModal, PaymentModal, CustomOrderModal,
 * BundleEditModal, OnboardingModal строятся поверх него — не копируют эту
 * логику заново.
 */
export function Modal({
  open,
  onClose,
  children,
  ariaLabel,
  z = "overlay",
  size = "md",
}: ModalProps) {
  const containerRef = useFocusTrap<HTMLDivElement>(open);

  useEffect(() => {
    if (!open) return;
    function handleKeyDown(event: KeyboardEvent) {
      if (event.key === "Escape") onClose();
    }
    document.addEventListener("keydown", handleKeyDown);
    document.body.style.overflow = "hidden";
    return () => {
      document.removeEventListener("keydown", handleKeyDown);
      document.body.style.overflow = "";
    };
  }, [open, onClose]);

  if (!open) return null;

  return createPortal(
    <div
      className={cn(
        "fixed inset-0 flex items-center justify-center bg-black/45 p-4",
        Z_CLASS[z],
      )}
      onClick={(event) => {
        if (event.target === event.currentTarget) onClose();
      }}
    >
      <div
        ref={containerRef}
        role="dialog"
        aria-modal="true"
        aria-label={ariaLabel}
        tabIndex={-1}
        className={cn(
          "w-full rounded-2xl border border-line bg-surface p-6 shadow-modal outline-none",
          SIZE_CLASS[size],
        )}
      >
        {children}
      </div>
    </div>,
    document.body,
  );
}
