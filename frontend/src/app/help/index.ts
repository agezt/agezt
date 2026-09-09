// app/help — per-view help topics (HelpTopic interface + HELP
// record + helpTopicFor() dispatcher + FALLBACK_TOPIC).
// Day 5a carve-out: pulled from lib/help.ts (2795 LOC — the
// largest god file in the codebase, a single Record<string,
// HelpTopic> constant). The full HELP record still lives in one
// file at this commit; the topic-by-topic split into
// app/help/{converse,monitor,agents,automation,knowledge,system}
// lands in Day 5b. See docs/FRONTEND-REFACTOR-PLAN.md.
export * from "./help";
