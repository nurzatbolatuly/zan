import { Trash2 } from "lucide-react";
import { Link } from "react-router-dom";
import { Badge, Card, IconButton } from "@/shared/ui";
import { formatDate, formatDuration } from "@/shared/lib/format";
import { THREAD_STATUS_TONE } from "@/shared/lib/threadStatusTone";
import type { Thread } from "./types";
import { historyDictionary } from "./locales";
import type { Lang } from "@/shared/types/common";

interface ThreadCardProps {
  thread: Thread;
  lang: Lang;
  onDelete: (thread: Thread) => void;
}

export function ThreadCard({ thread, lang, onDelete }: ThreadCardProps) {
  const dictionary = historyDictionary[lang];

  return (
    <Card className="relative transition-colors hover:border-accent">
      <Link
        to={`/?thread=${thread.id}`}
        aria-label={`${dictionary.open}: ${thread.title}`}
        className="block rounded-md pr-10 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent"
      >
        <div className="truncate text-body font-semibold text-ink">{thread.title}</div>
        <div className="mt-1 truncate text-body-sm text-muted">{thread.preview}</div>
        <div className="mt-2 flex flex-wrap items-center gap-3 font-mono text-micro text-muted">
          <Badge tone={THREAD_STATUS_TONE[thread.status]}>
            {dictionary.statusLabel[thread.status]}
          </Badge>
          <span>{formatDate(thread.updatedAt)}</span>
          <span>{formatDuration(thread.processingTimeSeconds, lang)}</span>
        </div>
      </Link>
      {/* Внутри карточки, правый верхний угол — без рамки (variant="ghost-danger"),
          не отдельный блок рядом с карточкой, как было раньше. */}
      <IconButton
        aria-label={`${dictionary.deleteAction}: ${thread.title}`}
        variant="ghost-danger"
        onClick={() => onDelete(thread)}
        className="absolute right-2 top-2"
      >
        <Trash2 className="h-5 w-5" aria-hidden="true" />
      </IconButton>
    </Card>
  );
}
