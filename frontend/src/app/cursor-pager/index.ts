// app/cursor-pager — cursor-based pagination hook (useCursorPager).
// Day 4 carve-out: pulled from lib/cursorPager.ts (13 import sites).
// Pulled into a dash-separated directory because the file name uses
// camelCase (cursorPager) but the import path is kebab-case
// (cursor-pager) for consistency with feature/ folders. See
// docs/FRONTEND-REFACTOR-PLAN.md.
export * from "./cursorPager";
