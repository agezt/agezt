// Event categorisation for the live stream console: maps a journal event kind to
// a stable category with a fixed hue, so the firehose is scannable and
// filterable. Hues are chosen to read on both light and dark panels (they don't
// depend on theme tokens — this is a console). Pure, unit-tested.

export interface EventCat {
  key: string;
  label: string;
  color: string; // CSS color (hsl), theme-independent
}

export const CATEGORIES: EventCat[] = [
  { key: "task", label: "task", color: "hsl(145 55% 45%)" },
  { key: "llm", label: "llm", color: "hsl(210 80% 58%)" },
  { key: "tool", label: "tool", color: "hsl(265 70% 62%)" },
  { key: "policy", label: "policy", color: "hsl(35 90% 52%)" },
  { key: "budget", label: "budget", color: "hsl(165 60% 42%)" },
  { key: "steer", label: "steer", color: "hsl(190 85% 48%)" },
  { key: "provider", label: "provider", color: "hsl(330 70% 58%)" },
  { key: "context", label: "context", color: "hsl(50 80% 45%)" },
  { key: "knowledge", label: "knowledge", color: "hsl(280 50% 60%)" },
  { key: "system", label: "system", color: "hsl(0 0% 55%)" },
  { key: "other", label: "other", color: "hsl(0 0% 50%)" },
];

const CAT_BY_KEY = new Map(CATEGORIES.map((c) => [c.key, c]));

// categoryOf maps an event kind to its category by prefix. Unknown kinds land in
// "other" so the stream never drops an event.
export function categoryOf(kind: string | undefined): EventCat {
  const k = (kind || "").toLowerCase();
  const key = prefixKey(k);
  return CAT_BY_KEY.get(key) || CAT_BY_KEY.get("other")!;
}

function prefixKey(k: string): string {
  if (k.startsWith("task.")) return "task";
  if (k.startsWith("llm.")) return "llm";
  if (k.startsWith("tool.")) return "tool";
  if (k.startsWith("policy.")) return "policy";
  if (k.startsWith("budget.") || k.startsWith("rate.")) return "budget";
  if (k.startsWith("run.")) return "steer";
  if (k.startsWith("provider.") || k.startsWith("routing.") || k.startsWith("capability.")) return "provider";
  if (k.startsWith("context.")) return "context";
  if (k.startsWith("memory.") || k.startsWith("world.") || k.startsWith("skill.") || k.startsWith("subagent.")) return "knowledge";
  if (
    k.startsWith("warden.") ||
    k.startsWith("netguard.") ||
    k.startsWith("kernel.") ||
    k === "halt" ||
    k === "resume" ||
    k.startsWith("schedule.") ||
    k.startsWith("webhook.")
  )
    return "system";
  return "other";
}

// isErrorKind flags kinds that represent a failure/denial, so the stream can
// highlight them regardless of category.
export function isErrorKind(kind: string | undefined): boolean {
  const k = (kind || "").toLowerCase();
  return (
    k === "task.failed" ||
    k === "budget.exceeded" ||
    k === "rate.limited" ||
    k === "netguard.blocked" ||
    k === "capability.rejected" ||
    k.endsWith(".denied") ||
    k.includes("error")
  );
}
