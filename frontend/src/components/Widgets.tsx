import { cn } from "@/app/utils";

// A small kit of pure-SVG/CSS data widgets — gauges, sparklines, bars — so views
// can show state visually instead of as flat tables. No chart library: each is a
// few dozen lines of SVG, themeable via the app's color tokens.

type Tone = "accent" | "good" | "bad" | "warn" | "muted";

const STROKE: Record<Tone, string> = {
  accent: "stroke-accent",
  good: "stroke-good",
  bad: "stroke-bad",
  warn: "stroke-amber-500",
  muted: "stroke-muted",
};
const TEXT: Record<Tone, string> = {
  accent: "text-accent",
  good: "text-good",
  bad: "text-bad",
  warn: "text-amber-500",
  muted: "text-foreground",
};
const FILL: Record<Tone, string> = {
  accent: "fill-accent",
  good: "fill-good",
  bad: "fill-bad",
  warn: "fill-amber-500",
  muted: "fill-muted",
};



// SEG_SHADE gives stacked-breakdown segments a cohesive look: the same accent
// hue at decreasing opacity, ranked by size — so a category mix reads as one
// themed bar instead of a clash of arbitrary colors.
const SEG_SHADE = ["bg-accent", "bg-accent/70", "bg-accent/55", "bg-accent/40", "bg-accent/20", "bg-accent/20"];

// BreakdownBar shows how a total splits across categories: a single stacked
// proportion bar plus a count chip per category, ranked largest-first. Good for
// "memories by type", "entities by kind" — a glanceable mix instead of a header
// number. Categories beyond the palette share the faintest shade.
export function BreakdownBar({ segments }: { segments: { label: string; count: number }[] }) {
  const present = segments.filter((s) => s.count > 0).sort((a, b) => b.count - a.count);
  const total = present.reduce((s, x) => s + x.count, 0) || 1;
  if (present.length === 0) return null;
  return (
    <div className="rounded-lg border border-border bg-card p-3">
      <div className="flex h-2.5 overflow-hidden rounded-full bg-panel">
        {present.map((s, i) => (
          <div
            key={s.label}
            className={cn("h-full transition-[width] duration-500", SEG_SHADE[i] || "bg-accent/20")}
            style={{ width: `${(s.count / total) * 100}%` }}
            title={`${s.count} ${s.label}`}
          />
        ))}
      </div>
      <div className="mt-2 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs">
        {present.map((s, i) => (
          <span key={s.label} className="inline-flex items-center gap-1.5">
            <span className={cn("size-2 rounded-full", SEG_SHADE[i] || "bg-accent/20")} />
            <span className="font-semibold tabular-nums text-foreground">{s.count}</span>
            <span className="text-muted">{s.label}</span>
          </span>
        ))}
      </div>
    </div>
  );
}

