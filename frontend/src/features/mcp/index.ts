// features/mcp — Day 18 twenty-first feature carve-out.
//
// Owns:
//   - Mcp page (Mcp.tsx, 829 satır; carries the Model Context
//     Protocol server browser UI, NewServerForm sub-component)
//
// Public surface (re-exported here so the rest of the codebase reads
// @/features/mcp and never needs to know about the sub-paths):
//   - components/Mcp.tsx — the page view
//   - NewServerForm (re-exported) — used by views/Wizards.tsx
//
// No lib/ helper in this feature — all business logic is colocated
// with the page in components/Mcp.tsx. The 829-line page is the
// biggest single risk; a future Day-26+ split should pull the
// NewServerForm sub-component out into components/Mcp/form.tsx.
//
// Cross-feature deps:
//   - @/app/api, @/app/events, @/app/utils, @/components/ui/*
//     (shared UI primitives — same pattern as the rest of the
//     features)
//
// See docs/FRONTEND-REFACTOR-PLAN.md for the carve-out rationale.
export { Mcp } from "./components/Mcp";
export { NewServerForm } from "./components/Mcp";
export * from "./types";
