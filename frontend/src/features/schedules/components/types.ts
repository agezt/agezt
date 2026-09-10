// types.ts — schedules type definitions (extracted from Schedules.tsx). Day 23 god file split.
//
// Only exported type/interface declarations live here. Internal types
// stay in page.tsx because they aren't consumed outside the
// package's barrel. Public surface (page.tsx re-exports) unchanged.

export type ScheduleTarget = "agent" | "workflow" | "system_task" | "tool";
