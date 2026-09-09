// features/connections — Day 18 twenty-fifth feature carve-out.
//
// Owns:
//   - Connections page (Connections.tsx, 693 satır; carries the
//     connectivity status view + ConnectivityStrip sub-component
//     that other pages may embed as a status banner)
//
// Public surface (re-exported here so the rest of the codebase reads
// @/features/connections and never needs to know about the
// sub-paths):
//   - components/Connections.tsx — the page view
//   - ConnectivityStrip (re-exported) — directly tested in
//     components/Connections.test.tsx
//
// No lib/ helper in this feature — all business logic is colocated
// with the page in components/Connections.tsx.
//
// Cross-feature deps:
//   - @/app/api, @/app/events, @/app/utils, @/components/ui/*,
//     @/components/ConnectionChip (shared, stays in components/ —
//     the per-connection status pill used by Dashboard + others)
//
// See docs/FRONTEND-REFACTOR-PLAN.md for the carve-out rationale.
export { Connections } from "./components/Connections";
export { ConnectivityStrip } from "./components/Connections";
export * from "./types";
