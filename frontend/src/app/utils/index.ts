// app/utils — cross-cutting helpers shared across the entire WebUI.
// Day 3 carve-out: pulled from lib/utils.ts (was the highest-imported
// module in the codebase, 139 import sites) so that subsequent
// feature carve-outs can lean on a stable core instead of the flat
// lib/ dump. See docs/FRONTEND-REFACTOR-PLAN.md for context.
export * from "./utils";
