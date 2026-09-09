// features/memory — Day 14 ninth feature carve-out.
//
// Owns:
//   - Memory page (Memory.tsx, 1073 satır — god file; carries the
//     memory browser UI, parseMemoryJSON loader, TeachFactForm +
//     ReviseFactForm sub-components for the fact CRUD)
//
// Public surface (re-exported here so the rest of the codebase reads
// @/features/memory and never needs to know about the sub-paths):
//   - components/Memory.tsx — the page view
//   - parseMemoryJSON + TeachFactForm + ReviseFactForm
//     (re-exported) — the JSON parser is used by lib/snapshot.ts
//     for snapshot generation; the two form sub-components are
//     used in the fact CRUD flows and may be reused in wizards
//     later
//
// No lib/ helper in this feature — the JSON parser lives inline in
// components/Memory.tsx. A future Day-26+ split should pull
// parseMemoryJSON into lib/parseMemoryJSON.ts to make it
// unit-testable without React, but that's deferred so today's
// commit stays focused on the carve-out.
//
// Cross-feature deps:
//   - @/app/api, @/app/events, @/app/utils, @/components/ui/*
//     (shared UI primitives — same pattern as the rest of the
//     features)
//   - features/agents/components/agentdetail/MemoryTab.tsx —
//     renders Memory entries inside the agent detail panel; it
//     imports the agent-detail shapes (RunLite, MemoryRecord,
//     SkillLite) and does NOT import from this feature
//
// The 1073-line Memory.tsx god file is the biggest single risk in
// this feature. A future Day-26+ split should separate:
//   - components/Memory/page.tsx (the Memory() default)
//   - components/Memory/forms.tsx (TeachFactForm + ReviseFactForm)
//   - components/Memory/parse.ts (parseMemoryJSON)
//   - components/Memory.tsx (re-export the page)
// Day 14's commit only MOVES the file; the split is deferred so
// typecheck + test verification stays focused on the carve-out.
//
// See docs/FRONTEND-REFACTOR-PLAN.md for the carve-out rationale.
export { Memory } from "./components/Memory";
export {
  parseMemoryJSON,
  TeachFactForm,
  ReviseFactForm,
} from "./components/Memory";
export * from "./types";
