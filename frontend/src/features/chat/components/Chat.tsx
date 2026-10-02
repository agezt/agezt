// Chat.tsx — the module nav.tsx lazily imports for the Chat view.
//
// It is a re-export shim, not an implementation. The Chat surface itself lives
// in ../impl/ — which used to be called ../legacy/, a name that described
// neither the code nor its status. It was the only copy of the implementation,
// so "legacy" read as an invitation to delete the Chat feature; a dead-code
// report during the 2026-09 audit called it an unused duplicate of the live
// Chat, which is the reverse of the truth. Renamed, because a misleading path
// is a maintenance hazard, not a tidiness issue.
//
// Day 26 brought the original full-feature chat back after Day 23's cleanup
// deleted the @/views/* tree. The original implementation (commit 1d1136f6)
// had every chat affordance — conversation sidebar with pin/rename/delete, the
// message timeline with retry/regenerate/continue/edit, attached-skill/memory/
// run chips, the composer with mic and voice-mode button, model/agent/profile/
// persona pickers, trust toggles, queue panel, suggestions bar,
// export-to-markdown — and shared the same ChatEngine (lib/chat.ts +
// lib/chatStore.tsx) that powers the MiniChat overlay, so a single thread
// persists across both surfaces.
//
// Re-exporting from here means nav.tsx's lazy() loader resolves the view
// without a nav-side rewrite.
export { Chat as default, Chat } from "../impl/Chat";
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
} from "../impl/message";
export { ConversationItem } from "../impl/conversation";
