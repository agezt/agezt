// features/skills — Day 15 twelfth feature carve-out.
//
// Owns:
//   - Skills page (Skills.tsx, 1047 satır — god file; carries the
//     skill browser UI, AuthorSkillForm sub-component for the
//     skill-create wizard, skillMatches + isWorkshopProposal +
//     scanSkill + lineDiff + diffSkillAgainstParent formatters)
//
// Public surface (re-exported here so the rest of the codebase reads
// @/features/skills and never needs to know about the sub-paths):
//   - components/Skills.tsx — the page view
//   - AuthorSkillForm (re-exported) — may be reused in wizards
//   - The 5 helpers (skillMatches, isWorkshopProposal, scanSkill,
//     lineDiff, diffSkillAgainstParent) — directly tested in
//     components/Skills.test.tsx, so part of the contract
//
// No lib/ helper in this feature — all business logic is colocated
// with the page in components/Skills.tsx. A future Day-26+ split
// should pull the helpers into lib/skills.ts to make them
// unit-testable without React.
//
// Cross-feature deps:
//   - @/app/api, @/app/events, @/app/utils, @/components/ui/*
//     (shared UI primitives — same pattern as the rest of the
//     features)
//
// The 1047-line Skills.tsx god file is the biggest single risk in
// this feature. A future Day-26+ split should separate:
//   - components/Skills/page.tsx (the Skills() default)
//   - components/Skills/form.tsx (AuthorSkillForm)
//   - components/Skills/format.ts (skillMatches, isWorkshopProposal,
//     scanSkill, lineDiff, diffSkillAgainstParent)
//   - components/Skills.tsx (re-export the page)
// Day 15's commit only MOVES the file; the split is deferred so
// typecheck + test verification stays focused on the carve-out.
//
// See docs/FRONTEND-REFACTOR-PLAN.md for the carve-out rationale.
export { Skills } from "./components/Skills";
export { AuthorSkillForm } from "./components/Skills";
export {
  skillMatches,
  isWorkshopProposal,
  scanSkill,
  lineDiff,
  diffSkillAgainstParent,
} from "./components/Skills";
export * from "./types";
