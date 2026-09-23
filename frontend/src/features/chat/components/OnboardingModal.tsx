import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Modal } from "@/shared/ui/Modal";
import { Button } from "@/shared/ui/Button";
import { api } from "@/shared/lib/api";
import { logger } from "@/shared/lib/logger";
import type { ChatDictionary } from "../locales";
import type { Session } from "@/shared/types/api";

interface OnboardingModalProps {
  t: ChatDictionary;
}

/**
 * M5 — только на Chat, показывается один раз. Stage 6: источник истины —
 * `Session.onboarding_seen` (openapi.yaml), не `localStorage` — сессия уже
 * анонимная и персистентная (bearer-токен), реальный флаг переживает смену
 * устройства/браузерного профиля так же, как переживал бы localStorage на
 * одном устройстве, но синхронно с остальным состоянием сессии (язык/тема).
 */
export function OnboardingModal({ t }: OnboardingModalProps) {
  const queryClient = useQueryClient();
  const sessionQuery = useQuery({
    queryKey: ["session"],
    queryFn: () => api.get<Session>("/sessions/me"),
  });

  const isOpen = sessionQuery.data?.onboarding_seen === false;

  async function handleClose() {
    logger.info({ scope: "chat.onboarding", event: "dismissed" });
    // Оптимистично — не ждём ответ сети, чтобы закрыть модалку: сбой этого
    // конкретного POST не критичен, худший случай — онбординг покажется ещё
    // раз в следующей сессии.
    queryClient.setQueryData<Session>(["session"], (prev) =>
      prev ? { ...prev, onboarding_seen: true } : prev,
    );
    try {
      await api.post("/sessions/onboarding-seen");
    } catch (error) {
      logger.error({ scope: "chat.onboarding", event: "mark_seen_failed", error });
    }
  }

  return (
    <Modal
      open={isOpen}
      onClose={() => void handleClose()}
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

      <Button onClick={() => void handleClose()} className="w-full">
        {t.onboardingCta}
      </Button>
    </Modal>
  );
}
