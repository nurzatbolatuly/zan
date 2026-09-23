import { useEffect } from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useLangStore } from "@/shared/stores/useLangStore";
import { useUnsavedChangesStore } from "@/shared/stores/useUnsavedChangesStore";
import { useUnsavedChangesGuard } from "@/shared/hooks/useUnsavedChangesGuard";
import { useToast } from "@/shared/ui";
import { logger } from "@/shared/lib/logger";
import { api } from "@/shared/lib/api";
import { describeApiError } from "@/shared/lib/apiErrorMessages";
import { settingsDictionary } from "./locales";
import type { AgentPromptKey } from "./types";
import type { AgentPromptDto } from "@/shared/types/api";

export type PromptsFormValues = Record<AgentPromptKey, string>;

const EMPTY_VALUES: PromptsFormValues = { qa: "", document: "" };

/**
 * Вся логика вкладки «Промпты» (PLAN.md §5 Stage 4a, подключена к реальному
 * `AgentPrompt` CRUD в Stage 6 — `GET/PUT /admin/prompts`, требует
 * `X-Admin-Token`, см. `AdminGate.tsx`). Сохраняются только реально
 * изменённые промпты (`formState.dirtyFields`), не оба сразу — незачем
 * перезаписывать `updated_at` промпта, который админ не трогал.
 */
export function usePromptsForm() {
  const lang = useLangStore((state) => state.lang);
  const t = settingsDictionary[lang].prompts;
  const toast = useToast();
  const queryClient = useQueryClient();

  const promptsQuery = useQuery({
    queryKey: ["admin", "prompts"],
    queryFn: () => api.get<AgentPromptDto[]>("/admin/prompts", { admin: true }),
  });

  const prompts = (["qa", "document"] as const).map((key) => ({
    key,
    ...t.labels[key],
    value: promptsQuery.data?.find((p) => p.agent_type === key)?.prompt_text ?? "",
  }));

  const schema = z.object({
    qa: z.string().trim().min(1, t.requiredError),
    document: z.string().trim().min(1, t.requiredError),
  });

  const {
    register,
    handleSubmit,
    reset,
    formState: { errors, isDirty, dirtyFields, isSubmitting },
  } = useForm<PromptsFormValues>({
    resolver: zodResolver(schema),
    defaultValues: EMPTY_VALUES,
  });

  // Как только промпты загрузились (или переключился язык — label/hint
  // локализованы, а не value, поэтому язык сам по себе не должен казаться
  // несохранённым изменением), форма переинициализируется реальными значениями.
  useEffect(() => {
    if (!promptsQuery.data) return;
    reset({
      qa: promptsQuery.data.find((p) => p.agent_type === "qa")?.prompt_text ?? "",
      document:
        promptsQuery.data.find((p) => p.agent_type === "document")?.prompt_text ?? "",
    });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [promptsQuery.data]);

  useEffect(() => {
    useUnsavedChangesStore
      .getState()
      .setDirty(isDirty, isDirty ? t.unsavedGuardMessage : "");
  }, [isDirty, t.unsavedGuardMessage]);
  useUnsavedChangesGuard(isDirty);

  useEffect(() => {
    return () => {
      useUnsavedChangesStore.getState().setDirty(false);
    };
  }, []);

  async function onSubmit(values: PromptsFormValues) {
    const changedKeys = (Object.keys(dirtyFields) as AgentPromptKey[]).filter(
      (key) => dirtyFields[key],
    );
    if (changedKeys.length === 0) return;

    logger.info({
      scope: "settings.prompts",
      event: "save_requested",
      data: { keys: changedKeys },
    });
    try {
      await Promise.all(
        changedKeys.map((key) =>
          api.put(`/admin/prompts/${key}`, { prompt_text: values[key] }, { admin: true }),
        ),
      );
      logger.info({ scope: "settings.prompts", event: "save_succeeded" });
      toast(t.savedNote, "success");
      await queryClient.invalidateQueries({ queryKey: ["admin", "prompts"] });
      reset(values);
    } catch (error) {
      logger.error({ scope: "settings.prompts", event: "save_failed", error });
      toast(describeApiError(error, lang), "error");
    }
  }

  return {
    prompts,
    isLoading: promptsQuery.isLoading,
    isError: promptsQuery.isError,
    retry: () => void promptsQuery.refetch(),
    register,
    errors,
    isDirty,
    isSubmitting,
    submitForm: handleSubmit(onSubmit),
  };
}
