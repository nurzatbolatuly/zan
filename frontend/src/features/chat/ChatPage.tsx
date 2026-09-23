import { Link } from "react-router-dom";
import { Button, EmptyState, Skeleton } from "@/shared/ui";
import { cn } from "@/shared/lib/cn";
import { useLangStore } from "@/shared/stores/useLangStore";
import { ChatEmptyState } from "./components/ChatEmptyState";
import { ThreadMessages } from "./components/ThreadMessages";
import { Composer } from "./components/Composer";
import { OnboardingModal } from "./components/OnboardingModal";
import { chatDictionary } from "./locales";
import { CHAT_COLUMN_CLASS } from "./layout";
import { useChatThread } from "./useChatThread";

/**
 * Точка входа (PLAN.md §5 Stage 1, Stage 6 — реальный тред через
 * `?thread=:id`). Вся логика в useChatThread (FRONT_CODING_STANDARDS.md §1).
 */
export function ChatPage() {
  const lang = useLangStore((state) => state.lang);
  const t = chatDictionary[lang];
  const thread = useChatThread();

  // Первое сообщение нового треда "в полёте" (id ещё нет) — уже диалог,
  // не пустой экран: показываем его сразу, не дожидаясь POST /threads.
  const isNewThread = thread.threadId === null && !thread.isSending;

  return (
    <div className="pb-40 md:pb-32">
      {isNewThread && (
        <ChatEmptyState
          t={t}
          onSelectTopic={(topicLabel, draft) => thread.applyQuickTopic(topicLabel, draft)}
        />
      )}

      {!isNewThread && thread.isThreadError && (
        <div className="pt-10">
          <EmptyState
            title={t.loadError}
            action={<Button onClick={thread.retryThread}>{t.retry}</Button>}
          />
        </div>
      )}

      {!isNewThread && !thread.isThreadError && thread.isLoadingThread && (
        <div className={cn(CHAT_COLUMN_CLASS, "flex flex-col gap-4 pt-4")}>
          <Skeleton className="h-16" />
          <Skeleton className="ml-auto h-12 w-2/3" />
        </div>
      )}

      {!isNewThread && !thread.isThreadError && !thread.isLoadingThread && (
        <ThreadMessages
          messages={thread.messages}
          replyProgress={thread.replyProgress}
          streamingText={thread.streamingText}
          streamingMessageId={thread.streamingMessageId}
          expandedSourceMessageIds={thread.expandedSourceMessageIds}
          onToggleSources={thread.toggleSources}
          onVote={thread.voteMessage}
          t={t}
        />
      )}

      {!isNewThread &&
        !thread.isThreadError &&
        !thread.isLoadingThread &&
        thread.statusNotice && (
          <p className={cn(CHAT_COLUMN_CLASS, "pb-4 text-body-sm text-muted")}>
            {thread.statusNotice}
          </p>
        )}

      {!isNewThread && !thread.isThreadError && !thread.isLoadingThread && (
        <div
          className={cn(CHAT_COLUMN_CLASS, "flex flex-wrap items-center gap-2.5 pb-4")}
        >
          {thread.isAwaitingPayment && (
            <>
              <Button
                onClick={() => void thread.resumeQuestion()}
                disabled={thread.isResuming}
              >
                {t.resumeQuestionAction}
              </Button>
              <Link
                to="/tariffs"
                className="inline-flex h-11 items-center rounded-md px-3 text-body-sm font-semibold text-accent hover:underline focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent"
              >
                {t.topUpBalanceAction}
              </Link>
            </>
          )}
        </div>
      )}

      <Composer
        t={t}
        draft={thread.draft}
        onDraftChange={thread.setDraft}
        attachment={thread.attachment}
        onAttach={(file) => void thread.attachFile(file)}
        onRemoveAttachment={thread.removeAttachment}
        isRecording={thread.isRecording}
        onStartRecording={() => void thread.startRecording()}
        onStopRecording={() => void thread.stopRecording()}
        onSend={() => void thread.sendMessage()}
        canSend={thread.canSend && !thread.isSending}
      />

      <OnboardingModal t={t} />
    </div>
  );
}
