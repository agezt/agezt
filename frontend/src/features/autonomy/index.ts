// features/autonomy — Day 16 fifteenth feature carve-out.
//
// Owns:
//   - Autonomy page (Autonomy.tsx, 863 satır; carries the
//     autonomy dashboard UI, PulseControl sub-component for the
//     cadence pulse controls, cadenceLabel formatter)
//   - lib/autonomy.ts (445 satır; the autonomy domain: AutonomyItem
//     type + status/severity/timing formatters, runRefFromAutonomy,
//     attention/attention-count helpers, filterAutonomyItems,
//     and the multi-line export group at the top of the file)
//
// Public surface (re-exported here so the rest of the codebase reads
// @/features/autonomy and never needs to know about the sub-paths):
//   - components/Autonomy.tsx — the page view
//   - PulseControl + cadenceLabel (re-exported from components)
//   - types.ts — AutonomyItem (the most-used cross-feature type)
//
// The 1 lib file (autonomy) is the feature's domain business
// logic; external code reaches it via
// @/features/autonomy/lib/autonomy (allowed but not advertised).
//
// Cross-feature deps:
//   - @/app/api, @/app/events, @/app/utils, @/components/ui/*
//     (shared UI primitives — same pattern as the rest of the
//     features)
//   - features/incidents/* — 3 consumer (lib/incidents.ts for
//     AutonomyItem type, components/IncidentPage.tsx + components/
//     IncidentBadges.tsx for live-event matching + badge formatters)
//   - views/Activity.tsx — multi-import for the activity dashboard
//   - components/DoctorIncidentTrees.tsx — multi-import for the
//     doctor flow's incident tree
//
// The 863-line Autonomy.tsx page is the biggest single risk in this
// feature; a future Day-26+ split should separate the page from
// the PulseControl sub-component. Day 16's commit only MOVES the
// file; the split is deferred so typecheck + test verification
// stays focused on the carve-out.
//
// See docs/FRONTEND-REFACTOR-PLAN.md for the carve-out rationale.
export { Autonomy } from "./components/Autonomy";
export { PulseControl, cadenceLabel } from "./components/Autonomy";
export * from "./types";
