// features/council — Day 8 third feature carve-out.
// Owns:
//   - Council decision routing (council.ts: member selection,
//     voting, verdict aggregation)
//   - Live-event ingest for council events (councilStore.ts:
//     routes agent decision events into the council view)
//   - The Council page view (Council.tsx)
//
// Public surface (the only thing the rest of the codebase sees):
//   - components/Council.tsx — the page view
//   - types.ts — CouncilMember / CouncilDecision / CouncilVote /
//     CouncilSession / CouncilVerdict
//
// Cross-feature deps:
//   - @/app/events (AgentEvent) — the global event hook from Day 4
//
// See docs/FRONTEND-REFACTOR-PLAN.md for the carve-out rationale.
export { Council } from "./components/Council";
export * from "./types";
