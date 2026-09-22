import { useState } from "react";
import { Modal } from "@/shared/ui/Modal";
import { Button } from "@/shared/ui/Button";
import { logger } from "@/shared/lib/logger";
import type { ChatDictionary } from "../locales";

const ONBOARDING_STORAGE_KEY = "zan.chat.onboarding-seen";

function hasSeenOnboarding(): boolean {
  try {
    return localStorage.getItem(ONBOARDING_STORAGE_KEY) === "1";
  } catch (error) {
    logger.error({ scope: "chat.onboarding", event: "storage_read_failed", error });
    return false;
  }
}

function markOnboardingSeen(): void {
  try {
    localStorage.setItem(ONBOARDING_STORAGE_KEY, "1");
  } catch (error) {
    logger.error({ scope: "chat.onboarding", event: "storage_write_failed", error });
  }
}

interface OnboardingModalProps {
  t: ChatDictionary;
}

/**
 * M5 — только на Chat, показывается один раз (флаг в localStorage, не в
 * проп, как было в прототипе Zan.dc.html:817 — см. PLAN.md Stage 1).
 */
export function OnboardingModal({ t }: OnboardingModalProps) {
  const [isOpen, setIsOpen] = useState(() => !hasSeenOnboarding());

  function handleClose() {
    logger.info({ scope: "chat.onboarding", event: "dismissed" });
    markOnboardingSeen();
    setIsOpen(false);
  }

  return (
    <Modal
      open={isOpen}
      onClose={handleClose}
      ariaLabel={t.onboardingTitle}
      z="overlay-high"
    >
      <div className="mb-4 h-8 w-8 rounded-md bg-accent" aria-hidden="true" />
      <h2 className="mb-2 text-h2 text-ink">{t.onboardingTitle}</h2>
      <p className="mb-5 text-body text-muted">{t.onboardingSubtitle}</p>

      <ol className="mb-5 flex flex-col gap-3">
        {t.onboardingSteps.map((step, index) => (
          <li key={step} className="flex items-start gap-3">
            <span className="grid h-6 w-6 flex-none place-items-center rounded-sm bg-accent-soft font-mono text-micro font-bold text-accent">
              {index + 1}
            </span>
            <span className="text-body-sm text-ink">{step}</span>
          </li>
        ))}
      </ol>

      <div className="mb-4 rounded-lg bg-surface-2 p-3.5 text-caption text-muted">
        {t.onboardingDisclaimer}
      </div>

      <Button onClick={handleClose} className="w-full">
        {t.onboardingCta}
      </Button>
    </Modal>
  );
}
