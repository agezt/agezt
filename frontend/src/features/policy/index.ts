// features/policy — Day 18 twenty-third feature carve-out.
//
// Owns:
//   - Policy page (Policy.tsx, 800 satır; carries the policy
//     editor UI: DenyAddForm + PolicyTestForm + RedactionCheckForm
//     sub-components for the 3 policy CRUD flows)
//
// Public surface (re-exported here so the rest of the codebase reads
// @/features/policy and never needs to know about the sub-paths):
//   - components/Policy.tsx — the page view
//   - DenyAddForm + PolicyTestForm + RedactionCheckForm
//     (re-exported) — directly tested in components/Policy.test.tsx
//
// No lib/ helper in this feature — all business logic is colocated
// with the page in components/Policy.tsx.
//
// Cross-feature deps:
//   - @/app/api, @/app/events, @/app/utils, @/components/ui/*
//     (shared UI primitives — same pattern as the rest of the
//     features)
//
// The 800-line Policy.tsx is the biggest single risk; a future
// Day-26+ split should pull the 3 form sub-components into
// components/Policy/forms.tsx.
//
// See docs/FRONTEND-REFACTOR-PLAN.md for the carve-out rationale.
export { Policy } from "./components/Policy";
export {
  DenyAddForm,
  PolicyTestForm,
  RedactionCheckForm,
} from "./components/Policy";
export * from "./types";
