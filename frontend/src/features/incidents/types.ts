// features/incidents/types.ts — shared types for the incident feature.
// Day 7 carve-out: the incident feature owns the IncidentProfileLite
// + IncidentMeta + IncidentHealthSummary shapes. See
// docs/FRONTEND-REFACTOR-PLAN.md.

export type {
  IncidentMeta,
  IncidentProfileLite,
} from "./lib/incidents";

// IncidentHealthSummary is exported from lib/incidents but is not
// always present (it's a derived shape); re-export when available.
// (Comment kept as a hint — the runtime export below may be a
// no-op if the source doesn't define the type.)
