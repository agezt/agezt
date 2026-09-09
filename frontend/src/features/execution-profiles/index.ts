// features/execution-profiles — Day 13 seventh feature carve-out (paired
// with features/schedules; the plan calls these out as one day because
// they're both small view-only features with no lib/ helpers).
//
// Owns:
//   - ExecutionProfiles page (ExecutionProfiles.tsx, 1343 satır —
//     carries the profile inventory view + ExecutionProfile /
//     ExecutionProfileInventory / ExecutionProfileCheck /
//     ExecutionProfileHealthReport / ConfigValueEntry /
//     ExecutionProfilePolicyValues / ExecutionProfileBackendValues
//     types + profileStatusTone + checkStatusTone + checksByProfileID
//     + executionProfileRollup + executionProfilePolicyFromConfigValues
//     + executionProfileBackendFromConfigValues helpers)
//
// Public surface (re-exported here so the rest of the codebase reads
// @/features/execution-profiles and never needs to know about the
// sub-paths):
//   - components/ExecutionProfiles.tsx — the page view
//   - The 6 helpers (profileStatusTone, checkStatusTone,
//     checksByProfileID, executionProfileRollup,
//     executionProfilePolicyFromConfigValues,
//     executionProfileBackendFromConfigValues) — directly tested in
//     components/ExecutionProfiles.test.tsx, so part of the contract
//   - types.ts — the 7 shared shapes
//
// No lib/ helper in this feature — the business logic is colocated
// with the page in components/ExecutionProfiles.tsx. A future
// Day-26+ split should pull the helpers + types out into
// lib/profiles.ts to make them unit-testable without React, but
// that's deferred so today's commit stays focused on the carve-out.
//
// Cross-feature deps:
//   - @/app/api (getJSON, postJSON) — Day 3
//   - @/components/ui/* (shared UI primitives)
//
// See docs/FRONTEND-REFACTOR-PLAN.md for the carve-out rationale.
export { ExecutionProfiles } from "./components/ExecutionProfiles";
export {
  profileStatusTone,
  checkStatusTone,
  checksByProfileID,
  executionProfileRollup,
  executionProfilePolicyFromConfigValues,
  executionProfileBackendFromConfigValues,
} from "./components/ExecutionProfiles";
export * from "./types";
