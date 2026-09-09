// features/overseer — Day 16 sixteenth feature carve-out.
//
// Owns:
//   - Overseer page (Overseer.tsx, 554 satır; carries the system
//     overseer dashboard — global agent/fleet live view, refresh
//     control, overseerShouldRefresh helper that filters the live
//     event stream to the overseer-relevant events)
//
// Public surface (re-exported here so the rest of the codebase reads
// @/features/overseer and never needs to know about the sub-paths):
//   - components/Overseer.tsx — the page view
//   - overseerShouldRefresh (re-exported) — directly tested in
//     components/Overseer.test.tsx, so part of the contract
//
// No lib/ helper in this feature — all business logic is colocated
// with the page in components/Overseer.tsx.
//
// Cross-feature deps:
//   - @/app/api, @/app/events, @/app/utils, @/components/ui/*,
//     @/features/agents/components/AgentAvatar (shared) — same
//     pattern as the rest of the features
//
// The 554-line Overseer.tsx is the smallest god file in the
// post-Day-1 refactor set; no immediate split needed.
//
// See docs/FRONTEND-REFACTOR-PLAN.md for the carve-out rationale.
export { Overseer } from "./components/Overseer";
export { overseerShouldRefresh } from "./components/Overseer";
export * from "./types";
