// features/runs — Day 9 focused run-core carve-out.
// Owns:
//   - The Runs list page (Runs.tsx)
//   - Run detail shaping (rundetail.ts: ToolCall + RunDetail types
//     and the helpers that fold a journal into a run summary)
//   - Run focus routing (runfocus.ts: the deep-link helper that
//     highlights a run in the inspector)
//
// RunDetail.tsx (841 LOC) is the per-run detail component — kept
// in components/ since it's reused across multiple surfaces
// (RunDetailLoader, RunDetail.steer, Runs page itself).
//
// Public surface (the only thing external callers see):
//   - components/Runs.tsx — the runs list page
//   - types.ts — ToolCall + RunDetail
//
// The 2 internal lib files (rundetail, runfocus) stay
// package-internal.
//
// Cross-feature deps:
//   - @/app/events (AgentEvent) — Day 4 global hook
//   - (no incidents import — the only AgentEvent consumer is rundetail.ts
//     which is satisfied by @/app/events alone)
//
// See docs/FRONTEND-REFACTOR-PLAN.md for the carve-out rationale.
export { Runs } from "./components/Runs";
export * from "./types";
