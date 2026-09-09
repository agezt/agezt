// features/data — Day 17 eighteenth feature carve-out.
//
// Owns:
//   - Data page (Data.tsx, 882 satır; carries the data-lake
//     browser UI: dataRecordAttribution + dataRecordWriter +
//     dataLakeActorAgent + dataLakeAgents + filterDataRecordsByAgent
//     helpers that fold the data-lake records into the row + actor
//     shapes the page renders)
//   - lib/datalakedate.ts (95 satır; the date canonicalization:
//     canonicalDate + dayKey + localDayKey + localMonthKey +
//     monthKey functions that normalize timestamps to the
//     datalake's date taxonomy)
//
// Public surface (re-exported here so the rest of the codebase reads
// @/features/data and never needs to know about the sub-paths):
//   - components/Data.tsx — the page view
//   - The 5 helpers (dataRecordAttribution, dataRecordWriter,
//     dataLakeActorAgent, dataLakeAgents,
//     filterDataRecordsByAgent) — directly tested in
//     components/Data.test.tsx, so part of the contract
//
// The 1 lib file (datalakedate) is the feature's domain business
// logic; external code reaches it via
// @/features/data/lib/datalakedate (allowed but not advertised).
// No cross-feature consumer of datalakedate today (the previous
// data-lake date tests were co-located with the page and moved
// with it).
//
// Cross-feature deps:
//   - @/app/api, @/app/events, @/app/utils, @/components/ui/*,
//     @/components/DataView (shared, stays in components/)
//
// The 882-line Data.tsx is the biggest single risk in this
// feature; a future Day-26+ split should pull the helpers into
// lib/data.ts. Day 17's commit only MOVES the file; the split is
// deferred so typecheck + test verification stays focused on the
// carve-out.
//
// See docs/FRONTEND-REFACTOR-PLAN.md for the carve-out rationale.
export { Data } from "./components/Data";
export {
  dataRecordAttribution,
  dataRecordWriter,
  dataLakeActorAgent,
  dataLakeAgents,
  filterDataRecordsByAgent,
} from "./components/Data";
export * from "./types";
