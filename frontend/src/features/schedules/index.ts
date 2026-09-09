// features/schedules — Day 13 seventh feature carve-out.
//
// Owns:
//   - Schedules page (Schedules.tsx, 1518 satır — god file; carries the
//     schedule CRUD UI, ScheduleTarget union, scheduleSelectedAgentIssue +
//     scheduleIntentFieldHint + schedulePayloadContract +
//     scheduleFormCadenceLabel formatters, NewScheduleForm sub-component)
//   - lib/shared.ts (616 satır — the schedule domain: parseSchedulesJSON,
//     scheduleActionTitle, scheduleCounts, scheduleTargetCounts,
//     scheduleTargetMixLabel, scheduleTargetLabel, filterScheduleItems,
//     scheduleAttentionReasons/Count/NeedsAttention, scheduleFireMeta,
//     scheduleAgentManaged, scheduleToolAgentIssue, systemTask* helpers,
//     DUE_SOON_MS constant, etc.)
//
// Public surface (re-exported here so the rest of the codebase reads
// @/features/schedules and never needs to know about the sub-paths):
//   - components/Schedules.tsx — the page view
//   - NewScheduleForm (re-exported) — used by views/Wizards.tsx as a
//     sub-component for the create-agent wizard
//   - scheduleSelectedAgentIssue + scheduleIntentFieldHint +
//     schedulePayloadContract + scheduleFormCadenceLabel (re-exported
//     from components/Schedules) — the schedule form helpers used by
//     Schedules.test.tsx
//   - types.ts — ScheduleTarget (the union) + ScheduleAgent (the row
//     shape used by the form's agent picker)
//
// The 1 lib file (shared) stays package-internal — it's the
// feature's business logic; only the views + sub-components + types
// are surfaced. External code reaches it via
// @/features/schedules/lib/shared (allowed but not advertised).
// `lib/snapshot.ts` is a known cross-feature consumer — it imports
// parseSchedulesJSON from this lib.
//
// Cross-feature deps:
//   - @/app/api, @/app/events, @/app/utils, @/components/ui/* (shared
//     UI primitives — same pattern as the rest of the features)
//   - lib/snapshot.ts (parseSchedulesJSON consumer) — rewrites
//     to @/features/schedules/lib/shared after the move
//   - views/Wizards.tsx (NewScheduleForm consumer) — rewrites to
//     @/features/schedules after the move
//
// The 1518-line Schedules.tsx god file is the biggest single risk in
// this feature; it carries the page + 4 helpers + NewScheduleForm
// sub-component in one file. A future Day-26+ split should separate:
//   - components/Schedules/page.tsx (the Schedules() default)
//   - components/Schedules/form.tsx (NewScheduleForm + form helpers)
//   - components/Schedules/forms.ts (the 4 formatters: scheduleSelectedAgentIssue,
//     scheduleIntentFieldHint, schedulePayloadContract,
//     scheduleFormCadenceLabel)
//   - components/Schedules.tsx (re-export the page)
// Day 13's commit only MOVES the file; the split is deferred so
// typecheck + test verification stays focused on the carve-out.
//
// See docs/FRONTEND-REFACTOR-PLAN.md for the carve-out rationale.
export { Schedules } from "./components/Schedules";
export { NewScheduleForm } from "./components/Schedules";
export {
  scheduleSelectedAgentIssue,
  scheduleIntentFieldHint,
  schedulePayloadContract,
  scheduleFormCadenceLabel,
} from "./components/Schedules";
export * from "./types";
