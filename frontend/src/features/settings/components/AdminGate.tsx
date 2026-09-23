import { useEffect, useState } from "react";
import type { FormEvent, ReactNode } from "react";
import { Button, Card, Input, Skeleton } from "@/shared/ui";
import { api } from "@/shared/lib/api";
import { useAdminAuthStore } from "@/shared/stores/useAdminAuthStore";
import { useLangStore } from "@/shared/stores/useLangStore";
import { describeApiError } from "@/shared/lib/apiErrorMessages";
import { logger } from "@/shared/lib/logger";
import { settingsDictionary } from "../locales";

type GateStatus = "checking" | "verified" | "unverified";

/**
 * `X-Admin-Token` для `/admin/*` (Stage 6, instructions.md «Общий словарь
 * FE↔BE» — не привязан к сессии/роли пользователя). Простой prompt-гейт:
 * форма токена, проверка через `GET /admin/ping` (единственный смысл этой
 * ручки по openapi.yaml), токен сохраняется в `useAdminAuthStore` только
 * после успешной проверки — невалидный/отозванный токен никогда не лежит
 * в localStorage как будто он рабочий. Уже сохранённый с прошлой сессии
 * токен тоже перепроверяется при каждом монтировании (мог протухнуть/быть
 * отозван), не используется вслепую.
 */
export function AdminGate({ children }: { children: ReactNode }) {
  const lang = useLangStore((state) => state.lang);
  const t = settingsDictionary[lang].adminGate;
  const adminToken = useAdminAuthStore((state) => state.adminToken);
  const setAdminToken = useAdminAuthStore((state) => state.setAdminToken);
  const clearAdminToken = useAdminAuthStore((state) => state.clearAdminToken);

  const [status, setStatus] = useState<GateStatus>(
    adminToken ? "checking" : "unverified",
  );
  const [tokenInput, setTokenInput] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [isSubmitting, setIsSubmitting] = useState(false);

  useEffect(() => {
    if (!adminToken) {
      setStatus("unverified");
      return;
    }
    let cancelled = false;
    setStatus("checking");
    api
      .get("/admin/ping", { admin: true })
      .then(() => {
        if (!cancelled) setStatus("verified");
      })
      .catch((err: unknown) => {
        if (cancelled) return;
        logger.info({ scope: "settings.admin-gate", event: "stored_token_rejected" });
        clearAdminToken();
        setError(describeApiError(err, lang));
        setStatus("unverified");
      });
    return () => {
      cancelled = true;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [adminToken]);

  async function handleSubmit(event: FormEvent) {
    event.preventDefault();
    setIsSubmitting(true);
    setError(null);
    setAdminToken(tokenInput.trim());
    try {
      await api.get("/admin/ping", { admin: true });
      logger.info({ scope: "settings.admin-gate", event: "token_verified" });
      setStatus("verified");
    } catch (err) {
      clearAdminToken();
      setError(describeApiError(err, lang));
    } finally {
      setIsSubmitting(false);
    }
  }

  if (status === "verified") return <>{children}</>;

  if (status === "checking") {
    return <Skeleton className="h-40 max-w-sm" />;
  }

  return (
    <Card className="mx-auto max-w-sm">
      <h2 className="mb-1.5 text-h3 text-ink">{t.title}</h2>
      <p className="mb-4 text-body-sm text-muted">{t.subtitle}</p>
      <form onSubmit={handleSubmit} className="flex flex-col gap-3">
        <Input
          type="password"
          label={t.tokenLabel}
          value={tokenInput}
          onChange={(event) => setTokenInput(event.target.value)}
          autoFocus
        />
        {error && <p className="text-caption text-danger">{error}</p>}
        <Button type="submit" disabled={isSubmitting || tokenInput.trim().length === 0}>
          {isSubmitting ? "…" : t.submit}
        </Button>
      </form>
    </Card>
  );
}
