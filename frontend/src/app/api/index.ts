// app/api — HTTP client + SDK bindings + SSE token plumbing.
// Day 3 carve-out: pulled from lib/api.ts (111 import sites, the
// second-most-imported module in the codebase) so that all feature
// carve-outs depend on a stable cross-cutting core instead of the
// flat lib/ dump. See docs/FRONTEND-REFACTOR-PLAN.md for context.
export * from "./api";
