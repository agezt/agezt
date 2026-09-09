// features/models — Day 17 nineteenth feature carve-out.
//
// Owns:
//   - Models page (Models.tsx, 616 satır; carries the model
//     catalog browser UI)
//   - lib/models.ts (165 satır; the model catalog domain:
//     flattenModels + filterModels + groupByProvider + pinnedOptions
//     + fmtContext + findModelContext + modelHealth helpers +
//     ModelCatalog + ModelHealth types)
//
// Public surface (re-exported here so the rest of the codebase reads
// @/features/models and never needs to know about the sub-paths):
//   - components/Models.tsx — the page view
//   - The 7 helpers (flattenModels, filterModels, groupByProvider,
//     pinnedOptions, fmtContext, findModelContext, modelHealth)
//     from lib/models — directly tested in
//     components/Models.test.tsx + lib/models.test.ts, so part of
//     the contract
//   - types.ts — ModelCatalog, ModelHealth (the most-used
//     cross-feature types)
//
// The 1 lib file (models) is the feature's domain business
// logic; external code reaches it via
// @/features/models/lib/models (allowed but not advertised).
// lib/models is the most heavily cross-feature consumed of any
// lib in the post-Day-1 set: 9 importers across chat (context +
// message), routing, agent detail, workflow chains, the shared
// ModelChip + ModelPicker widgets, and the catalog/search/filter
// surfaces — all rewritten to the new path.
//
// Cross-feature deps:
//   - @/app/api, @/app/events, @/app/utils, @/components/ui/*,
//     @/components/ModelChip + @/components/ModelPicker (shared,
//     stay in components/ — they import from this feature's lib)
//
// The 616-line Models.tsx is the smallest god file in the
// post-Day-1 set; no immediate split needed beyond the existing
// lib/components split.
//
// See docs/FRONTEND-REFACTOR-PLAN.md for the carve-out rationale.
export { Models } from "./components/Models";
export * from "./types";
