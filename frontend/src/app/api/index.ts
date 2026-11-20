// app/api — the console's HTTP client + SSE token plumbing.
// Day 3 carve-out: pulled from lib/api.ts (111 import sites, the
// second-most-imported module in the codebase) so that all feature
// carve-outs depend on a stable cross-cutting core instead of the
// flat lib/ dump. See docs/FRONTEND-REFACTOR-PLAN.md for context.
//
// ./client is GENERATED (go run ./tools/openapi): its path unions and verbs
// come from the daemon's operation registry and the console's route
// catalogue. ./transport is hand-written: the bearer token, the error shape
// and the event feed's URL are how this browser talks at all, not what it
// asks for.
export * from "./client";
export * from "./transport";
