// Chat.tsx — full-page conversation surface.
//
// Day 26 brought the original full-feature chat back after Day 23's cleanup
// deleted the @/views/* tree. The original implementation (commit 1d1136f6
// survived in features/chat/legacy/) had every chat affordance — conversation
// sidebar with pin/rename/delete, the message timeline with retry/regenerate/
// continue/edit, attached-skill/memory/run chips, the composer with mic and
// voice-mode button, model/agent/profile/persona pickers, trust toggles, queue
// panel, suggestions bar, export-to-markdown — and shared the same ChatEngine
// (lib/chat.ts + lib/chatStore.tsx) that powers the MiniChat overlay, so a
// single thread persists across both surfaces.
//
// Re-export the legacy Chat as the default so nav.tsx's lazy() loader picks it
// up without any rewrite on the nav side.
export { Chat as default, Chat } from "../legacy/Chat";
export {
  AssistantBubble,
  CompactionNote,
  ContextChip,
  ContextModal,
  ConversationPersona,
  ExecutionProfilePicker,
  FallbackNote,
  PromptLauncher,
  SummaryDivider,
  UserBubble,
  barTone,
} from "../legacy/message";
export { ConversationItem } from "../legacy/conversation";
