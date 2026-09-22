import { useLangStore } from "@/shared/stores/useLangStore";
import { ChatEmptyState } from "./components/ChatEmptyState";
import { ThreadMessages } from "./components/ThreadMessages";
import { Composer } from "./components/Composer";
import { OnboardingModal } from "./components/OnboardingModal";
import { chatDictionary } from "./locales";
import { useChatThread } from "./useChatThread";

/**
 * Точка входа Stage 1 (PLAN.md §5) — только композиция, вся логика в
 * useChatThread (FRONT_CODING_STANDARDS.md §1).
 */
export function ChatPage() {
  const lang = useLangStore((state) => state.lang);
  const t = chatDictionary[lang];
  const thread = useChatThread();

  const isEmpty = thread.messages.length === 0;

  return (
    <div className="pb-40 md:pb-32">
      {isEmpty ? (
        <ChatEmptyState
          lang={lang}
          t={t}
          onSelectTopic={(topicLabel, draft) => thread.applyQuickTopic(topicLabel, draft)}
        />
      ) : (
        <ThreadMessages
          messages={thread.messages}
          isAssistantTyping={thread.isAssistantTyping}
          expandedSourceMessageIds={thread.expandedSourceMessageIds}
          onToggleSources={thread.toggleSources}
          onVote={thread.voteMessage}
          t={t}
        />
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
        onStopRecording={thread.stopRecording}
        onSend={thread.sendMessage}
        canSend={thread.canSend}
      />

      <OnboardingModal t={t} />
    </div>
  );
}
