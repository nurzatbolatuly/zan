import { useEffect } from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { useLangStore } from "@/shared/stores/useLangStore";
import { useUnsavedChangesStore } from "@/shared/stores/useUnsavedChangesStore";
import { useUnsavedChangesGuard } from "@/shared/hooks/useUnsavedChangesGuard";
import { useToast } from "@/shared/ui";
import { logger } from "@/shared/lib/logger";
import { AGENT_PROMPT_MOCKS } from "./mocks";
import { settingsDictionary } from "./locales";
import type { AgentPromptKey } from "./types";

export type PromptsFormValues = Record<AgentPromptKey, string>;

// Мок сохранения — реальный `AgentPrompt` CRUD приходит в Stage 6
// (backend-roadmap.md:23), сейчас только имитация сетевой задержки, чтобы UI
// (disabled-состояние кнопки, "Сохранено") уже был рассчитан на асинхронность.
const MOCK_SAVE_DELAY_MS = 400;

/**
 * Вся логика вкладки «Промпты» (PLAN.md §5 Stage 4a): форма на react-hook-form
 * + zod (первое реальное применение стека, зафиксированного в PLAN.md §3 —
 * до этого RHF/Zod были в зависимостях, но не использовались, instructions.md
 * «Конвенции»), guard на несохранённые изменения при уходе со страницы.
 */
export function usePromptsForm() {
  const lang = useLangStore((state) => state.lang);
  const t = settingsDictionary[lang].prompts;
  const toast = useToast();
  const prompts = AGENT_PROMPT_MOCKS[lang];

  const schema = z.object({
    qa: z.string().trim().min(1, t.requiredError),
    documents: z.string().trim().min(1, t.requiredError),
  });

  const defaultValues: PromptsFormValues = {
    qa: prompts.find((p) => p.key === "qa")?.value ?? "",
    documents: prompts.find((p) => p.key === "documents")?.value ?? "",
  };

  const {
    register,
    handleSubmit,
    reset,
    formState: { errors, isDirty, isSubmitting },
  } = useForm<PromptsFormValues>({
    resolver: zodResolver(schema),
    defaultValues,
    // Смена языка не должна казаться "несохранённым изменением" — форма
    // переинициализируется на моки нового языка (см. эффект ниже).
  });

  // Переключение RU/KZ на этом экране показывает промпты другого языка — как
  // и Chat (instructions.md «Конвенции»: контент по языку достаётся заново,
  // а не хранится один раз при монтировании).
  useEffect(() => {
    reset(defaultValues);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [lang]);

  useEffect(() => {
    useUnsavedChangesStore
      .getState()
      .setDirty(isDirty, isDirty ? t.unsavedGuardMessage : "");
  }, [isDirty, t.unsavedGuardMessage]);
  useUnsavedChangesGuard(isDirty);

  // Сброс глобального флага, если вкладка ушла с экрана не через guard
  // (например, программная навигация мимо шапки) — не должен "залипать"
  // и блокировать переход в никак не связанном месте.
  useEffect(() => {
    return () => {
      useUnsavedChangesStore.getState().setDirty(false);
    };
  }, []);

  async function onSubmit(values: PromptsFormValues) {
    logger.info({ scope: "settings.prompts", event: "save_requested" });
    await new Promise((resolve) => setTimeout(resolve, MOCK_SAVE_DELAY_MS));
    logger.info({ scope: "settings.prompts", event: "save_succeeded" });
    toast(t.savedNote, "success");
    reset(values);
  }

  return {
    prompts,
    register,
    errors,
    isDirty,
    isSubmitting,
    submitForm: handleSubmit(onSubmit),
  };
}
