import { Card, Button, EmptyState, Skeleton, Textarea } from "@/shared/ui";
import { useLangStore } from "@/shared/stores/useLangStore";
import { settingsDictionary } from "../locales";
import { usePromptsForm } from "../usePromptsForm";

/** Settings → Промпты (PLAN.md §5 Stage 4a, Stage 6 — реальный `AgentPrompt` CRUD). */
export function PromptsTab() {
  const lang = useLangStore((state) => state.lang);
  const t = settingsDictionary[lang];
  const {
    prompts,
    isLoading,
    isError,
    retry,
    register,
    errors,
    isDirty,
    isSubmitting,
    submitForm,
  } = usePromptsForm();

  if (isError) {
    return (
      <Card>
        <EmptyState
          title={t.common.loadError}
          action={<Button onClick={retry}>{t.common.retry}</Button>}
        />
      </Card>
    );
  }

  if (isLoading) {
    return (
      <Card>
        <Skeleton className="mb-4 h-36" />
        <Skeleton className="h-36" />
      </Card>
    );
  }

  return (
    <Card>
      <h2 className="mb-3.5 text-h3 text-ink">{t.prompts.title}</h2>
      <form onSubmit={submitForm} className="flex flex-col gap-4">
        {prompts.map((prompt) => (
          <div key={prompt.key}>
            <Textarea
              label={prompt.label}
              hint={prompt.hint}
              rows={6}
              className="font-mono text-caption leading-relaxed"
              {...register(prompt.key)}
            />
            {errors[prompt.key] && (
              <p className="mt-1 text-caption text-danger">
                {errors[prompt.key]?.message}
              </p>
            )}
          </div>
        ))}
        <div className="flex items-center gap-2.5">
          <Button type="submit" disabled={!isDirty || isSubmitting}>
            {t.prompts.saveButton}
          </Button>
        </div>
      </form>
    </Card>
  );
}
