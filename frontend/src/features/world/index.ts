// features/world — Day 18 twenty-fourth feature carve-out.
//
// Owns:
//   - World page (World.tsx, 789 satır; carries the world
//     knowledge-graph browser UI + entityMatches helper +
//     parseWorldJSON loader — the entity graph + the snapshot
//     persistence that lib/snapshot.ts reads)
//
// Public surface (re-exported here so the rest of the codebase reads
// @/features/world and never needs to know about the sub-paths):
//   - components/World.tsx — the page view
//   - parseWorldJSON (re-exported) — used by lib/snapshot.ts for
//     snapshot generation
//   - entityMatches + any other tested helpers from World.test.tsx
//     (re-exported)
//
// No lib/ helper in this feature — all business logic is colocated
// with the page in components/World.tsx.
//
// Cross-feature deps:
//   - @/app/api, @/app/events, @/app/utils, @/components/ui/*,
//     @/components/WorldGraph (shared, stays in components/)
//
// The 789-line World.tsx is the biggest single risk; a future
// Day-26+ split should separate the page from the entity-match
// helpers + parseWorldJSON loader.
//
// See docs/FRONTEND-REFACTOR-PLAN.md for the carve-out rationale.
export { World } from "./components/World";
export { parseWorldJSON, entityMatches } from "./components/World";
export * from "./types";
