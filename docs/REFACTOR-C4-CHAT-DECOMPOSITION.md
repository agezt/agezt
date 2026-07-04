# Refactor C4 — `views/Chat.tsx` decomposition (named sub-files)

> Companion to `docs/REFACTORING-SCAN.md` finding **C4**.
> **Generated:** 2026-07-03. Grounded in a live read of the **pre-extraction monolith** `Chat.tsx` + its 10 sibling Chat test files.
>
> **Current-state note (2026-07-04):** this doc's measurements below are the **historical baseline** used to plan the decomposition. After P0-P5 extraction, the implementation now lives under `frontend/src/views/Chat/` with:
> - `Chat.tsx` (barrel shim) = 1 line
> - `Chat/Chat.tsx` = **416 lines / ~17.4 KiB**
> - `useChatSession.ts` = **122 lines**
> - extracted modules: `message.tsx`, `context.tsx`, `pickers.tsx`, `conversation.tsx`, `useComposer.ts`, `useVoice.ts`, `useContextWindow.ts`, `useConversationRouting.ts`, `useSteering.ts`, `useConversationControls.ts`
> See `docs/MISSING-PARTS-PLAN.md` P1-A for the latest slice status.

## Evidence (measured)

**Historical baseline (before decomposition):**

`frontend/src/views/Chat.tsx` — **1,973 lines / 76.6 KB**, **29 components in one file**,
**30 `useState`, 17 `useEffect`, 9 `useRef`, 2 `useMemo`, 0 `useReducer`**. Ten sibling tests:
`Chat.{bubble,context,conversation,edit,executionProfile,fallback,history,launcher,persona,summary}`.

**Critical constraint:** tests import **named exports** from `@/views/Chat`
(e.g. `Chat.context.test.tsx`: `import { ContextChip, ContextModal, CompactionNote, barTone } from "@/views/Chat"`).
`Chat.tsx` already acts as a **barrel**. The plan preserves that import surface via a barrel so
**no test needs editing** through Phase 6.

Two separable problems: (1) file size / co-location; (2) 30-useState state blob in root `Chat()`.

## Component inventory (by concern)

| Group | Components | Exported (test-coupled) |
|---|---|---|
| Bubbles / render | MessageRow, UserBubble*, AssistantBubble*, ReasoningBlock, WorkingIndicator, SteerNote, TurnMeta, ToolChip, LearnedChips | UserBubble, AssistantBubble |
| Answer actions | CopyAnswer, SpeakAnswer | — |
| Context window | ContextChip*, ContextModal*, CompactionNote*, barTone, ROLE_ORDER, ROLE_FILL | all + barTone |
| Summary | SummaryDivider*, CompactionNote* | yes |
| Execution profile | ExecutionProfilePicker*, EXEC_PROFILE_OPTIONS, EXEC_PROFILE_HINTS | picker |
| Persona | ConversationPersona* | yes |
| Launcher / empty | PromptLauncher*, EmptyState, DEFAULT_EXAMPLES | launcher |
| Fallback | FallbackNote* | yes |
| Conversation list | ConversationItem*, QueuePanel | ConversationItem |
| Root | Chat() (30 useState / 17 useEffect) | yes (default) |

`*` = imported by a test → export name MUST survive.

## Target layout

```
frontend/src/views/Chat/
├── index.tsx                 ← re-export barrel (keeps "@/views/Chat" stable)
├── Chat.tsx                  ← root orchestrator ONLY (layout + wiring, target <200 lines)
├── useChatSession.ts         ← extracted state machine (collapses the 30 useState)
├── message/
│   ├── MessageRow.tsx  UserBubble.tsx  AssistantBubble.tsx  ReasoningBlock.tsx
│   ├── WorkingIndicator.tsx  SteerNote.tsx  TurnMeta.tsx  ToolChip.tsx  LearnedChips.tsx
│   └── AnswerActions.tsx     (CopyAnswer + SpeakAnswer)
├── context/
│   ├── ContextChip.tsx       (+ barTone, ROLE_ORDER, ROLE_FILL)
│   ├── ContextModal.tsx      CompactionNote.tsx
├── ExecutionProfilePicker.tsx (+ EXEC_PROFILE_OPTIONS/HINTS)
├── ConversationPersona.tsx    FallbackNote.tsx    SummaryDivider.tsx
├── PromptLauncher.tsx         (+ EmptyState, DEFAULT_EXAMPLES)
└── conversation/
    ├── ConversationItem.tsx   QueuePanel.tsx
```

`index.tsx` re-exports every test-used name → `import { ContextChip } from "@/views/Chat"` still
resolves (Vite/TS resolve the dir to `index.tsx`). Zero test edits through Phase 6.

## Phases (each ends green: `tsc --noEmit && vitest run --run src/views/Chat && npm run build`)

- **P0 barrel shim (no logic moves):** move `Chat.tsx` → `Chat/Chat.tsx`; add `Chat/index.tsx`
  = `export * from "./Chat"; export { default } from "./Chat";`. Confirm 10 tests still resolve.
- **P1 message group:** move the 9 render components + `AnswerActions` into `message/`; barrel
  re-exports UserBubble/AssistantBubble. Gate: `Chat.bubble.test`.
- **P2 context group:** move ContextChip(+barTone/ROLE_*)/ContextModal/CompactionNote into
  `context/`; barrel re-exports the four. Gate: `Chat.context.test`.
- **P3 self-contained pickers/notes (5 tiny commits):** ExecutionProfilePicker, ConversationPersona,
  FallbackNote, SummaryDivider, PromptLauncher — one file move + one barrel line + one gate each
  (`executionProfile`/`persona`/`fallback`/`summary`/`launcher` tests).
- **P4 conversation list:** ConversationItem, QueuePanel → `conversation/`; re-export
  ConversationItem. Gate: `Chat.conversation.test` + `Chat.history.test`.
- **P5 extract state machine (the real win):** pull `Chat()`'s 30 useState / 17 useEffect into
  `useChatSession.ts`, composed of sub-hooks: `useComposer`, `useStreaming` (reuse `lib/events.ts`,
  see scan C9), `useConversationRouting`, `useVoice` (AUTOSPEAK_KEY + speak/stop), `useContextWindow`
  (catalog fetch + fold), `useExecutionProfile`, `usePersona`. Root becomes `const s = useChatSession()`.
  Tests render components with props, not the hook — add a focused `useChatSession.test.ts` only if
  a test reaches into root state. **Only risky phase — isolate in its own PR.**
- **P6 thin root:** `Chat.tsx` = layout only (left rail / thread / composer / launcher), <200 lines,
  no `useState` beyond trivial UI toggles.
- **P7 (optional, later):** repoint tests to leaf paths and slim the barrel. Cosmetic.

## Sequencing

```
P0 barrel     ← de-risks everything; 1 commit
P1 message    P2 context    ← pure moves behind stable barrel
P3 pickers    ← 5 tiny commits, one gate each
P4 convo      ← pure moves
P5 useChatSession ← behavior change; own PR, full Chat suite as gate
P6 thin root  P7 repoint (optional)
```

P0–P4 are mechanical moves behind a stable barrel (near-zero risk, each revertable). P5 is the only
behavior-touching phase.

## Per-phase gate

`cd frontend && npx tsc --noEmit && npx vitest run --run src/views/Chat && npm run build`

## Coordination

P1 moves AssistantBubble/MessageRow which render `<Markdown>` + `<ToolOutput>`; the S0–S4 sessions
recently rewrote `components/Markdown.tsx` (MonacoView code blocks). Rebase on their landed work
before P1 so the Markdown import surface is stable.
