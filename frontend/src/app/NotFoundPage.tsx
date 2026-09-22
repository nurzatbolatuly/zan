import { Link } from "react-router-dom";
import { EmptyState } from "@/shared/ui/EmptyState";
import { useLangStore } from "@/shared/stores/useLangStore";
import { navDictionary } from "@/shared/locales/nav";

export function NotFoundPage() {
  const lang = useLangStore((state) => state.lang);
  const t = navDictionary[lang];

  return (
    <div className="pt-14">
      <EmptyState
        title={t.notFoundTitle}
        description={t.notFoundBody}
        action={
          <Link
            to="/"
            className="inline-flex h-11 items-center justify-center rounded-md bg-accent px-4 text-body-sm font-semibold text-accent-ink hover:opacity-90 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent"
          >
            {t.notFoundCta}
          </Link>
        }
      />
    </div>
  );
}
