// features/standing — Day 14 tenth feature carve-out.
//
// Owns:
//   - Standing page (Standing.tsx, 1071 satır — god file; carries
//     the standing-orders browser UI, initiativeEnforcement +
//     parseStandingJSON helpers, NewOrderForm + EditOrderForm
//     sub-components, standingResumeIssue + standingAttentionReasons
//     + standingNeedsAttention + standingAttentionCount +
//     standingFrequencyIssue formatters)
//
// Public surface (re-exported here so the rest of the codebase reads
// @/features/standing and never needs to know about the sub-paths):
//   - components/Standing.tsx — the page view
//   - NewOrderForm (re-exported) — used by views/Wizards.tsx as a
//     sub-component for the create-order wizard
//   - parseStandingJSON (re-exported) — used by lib/snapshot.ts
//     for snapshot generation
//   - initiativeEnforcement + standingResumeIssue +
//     standingAttentionReasons + standingNeedsAttention +
//     standingAttentionCount + standingFrequencyIssue
//     (re-exported) — the standing formatters, all directly tested
//     in components/Standing.test.tsx so part of the contract
//
// No lib/ helper in this feature — all business logic is colocated
// with the page in components/Standing.tsx. A future Day-26+ split
// should pull the formatters + JSON parser into lib/standing.ts to
// make them unit-testable without React.
//
// Cross-feature deps:
//   - @/app/api, @/app/events, @/app/utils, @/components/ui/*
//     (shared UI primitives — same pattern as the rest of the
//     features)
//
// The 1071-line Standing.tsx god file is the biggest single risk in
// this feature. A future Day-26+ split should separate:
//   - components/Standing/page.tsx (the Standing() default)
//   - components/Standing/forms.tsx (NewOrderForm + EditOrderForm)
//   - components/Standing/format.ts (the 6 formatters:
//     initiativeEnforcement, standingResumeIssue,
//     standingAttentionReasons, standingNeedsAttention,
//     standingAttentionCount, standingFrequencyIssue)
//   - components/Standing/parse.ts (parseStandingJSON)
//   - components/Standing.tsx (re-export the page)
// Day 14's commit only MOVES the file; the split is deferred so
// typecheck + test verification stays focused on the carve-out.
//
// See docs/FRONTEND-REFACTOR-PLAN.md for the carve-out rationale.
export { Standing } from "./components/Standing";
export { NewOrderForm } from "./components/Standing";
export {
  parseStandingJSON,
  initiativeEnforcement,
  standingResumeIssue,
  standingAttentionReasons,
  standingNeedsAttention,
  standingAttentionCount,
  standingFrequencyIssue,
} from "./components/Standing";
export * from "./types";
