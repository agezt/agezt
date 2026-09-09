// features/incidents — Day 7 second feature carve-out.
// Owns:
//   - The incident dashboard view (IncidentPage.tsx)
//   - The incident badge UI components (IncidentBadges.tsx)
//   - Incident data shaping (incidents.ts: IncidentProfileLite,
//     IncidentHealthSummary, buildIncidentRollups, etc.)
//   - The live-event ingest for incident events (incidentevents.ts:
//     agent incident events routed through the global useEvents hook)
//   - The deep-link helper (incidentnav.ts: incidentIdFromHash)
//
// Public surface (re-exported here so the rest of the codebase reads
// @/features/incidents and never needs to know about the sub-paths):
//   - components/IncidentPage.tsx — the dashboard page
//   - components/IncidentBadges.tsx — the badge UI components
//   - types.ts — IncidentMeta / IncidentProfileLite /
//     IncidentHealthSummary
//
// The 3 internal lib files (incidents, incidentevents, incidentnav)
// stay package-internal — they're the feature's business logic;
// only the entry points + views + types are surfaced.
//
// Cross-feature deps:
//   - @/app/events (AgentEvent type) — global event hook (Day 4 carve-out)
//   - @/lib/autonomy (AutonomyItem type) — owned by the autonomy feature
//     (carved out later); kept as a direct lib/ import for now and will
//     migrate to @/features/autonomy/lib once that feature exists.
//
// See docs/FRONTEND-REFACTOR-PLAN.md for the carve-out rationale.
export { IncidentPage } from "./components/IncidentPage";
export { IncidentBadges } from "./components/IncidentBadges";
export * from "./types";
