// features/sandbox — Day 16 seventeenth feature carve-out.
//
// Owns:
//   - Sandbox page (Sandbox.tsx, 485 satır; carries the sandbox
//     explorer UI — view-only build artifact log + isBuildNoise
//     helper that filters build-tool noise from the artifact list)
//
// Public surface (re-exported here so the rest of the codebase reads
// @/features/sandbox and never needs to know about the sub-paths):
//   - components/Sandbox.tsx — the page view
//   - isBuildNoise (re-exported) — directly tested in
//     components/Sandbox.test.tsx, so part of the contract
//
// No lib/ helper in this feature — all business logic is colocated
// with the page in components/Sandbox.tsx.
//
// Cross-feature deps:
//   - @/app/api, @/app/events, @/app/utils, @/components/ui/*
//     (shared UI primitives — same pattern as the rest of the
//     features)
//
// The 485-line Sandbox.tsx is small; no immediate split needed.
//
// See docs/FRONTEND-REFACTOR-PLAN.md for the carve-out rationale.
export { Sandbox } from "./components/Sandbox";
export { isBuildNoise } from "./components/Sandbox";
export * from "./types";
