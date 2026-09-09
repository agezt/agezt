// app/events — global agent event stream hook (useEvents +
// AgentEvent interface + the MAX_FEED ring buffer).
// Day 4 carve-out: pulled from lib/events.tsx (69 import sites,
// the global event hook that every view + many components reach
// for live updates). The events URL itself is built by the
// app/api module (which sets the SSE_TOKEN); this module owns the
// React-side context, ring buffer, and connection lifecycle.
// See docs/FRONTEND-REFACTOR-PLAN.md for context.
export * from "./events";
