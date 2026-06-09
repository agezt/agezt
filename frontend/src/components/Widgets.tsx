import { cn } from "@/lib/utils";

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

// Ring is a circular progress gauge: a big centred value over a partial arc.
// pct is 0..100; center/sub are the labels shown inside and below.
export function Ring({
  pct,
  center,
  label,
  tone = "accent",
  size = 116,
}: {
  pct: number;
  center: string;
  label: string;
  tone?: Tone;
  size?: number;
}) {
  const stroke = 9;
  const r = (size - stroke) / 2;
  const c = 2 * Math.PI * r;
  const clamped = Math.max(0, Math.min(100, pct));
  const dash = (clamped / 100) * c;
  return (
    <div className="flex flex-col items-center gap-1">
      <div className="relative" style={{ width: size, height: size }}>
        <svg width={size} height={size} className="-rotate-90">
          <circle cx={size / 2} cy={size / 2} r={r} fill="none" strokeWidth={stroke} className="stroke-panel" />
          <circle
            cx={size / 2}
            cy={size / 2}
            r={r}
            fill="none"
            strokeWidth={stroke}
            strokeLinecap="round"
            className={cn("transition-[stroke-dasharray] duration-500", STROKE[tone])}
            strokeDasharray={`${dash} ${c}`}
          />
        </svg>
        <div className="absolute inset-0 flex flex-col items-center justify-center">
          <span className={cn("text-xl font-semibold tabular-nums", TEXT[tone])}>{center}</span>
        </div>
      </div>
      <span className="text-[11px] text-muted">{label}</span>
    </div>
  );
}

// Sparkline plots a series as a smooth line over a faint area fill. Good for a
// live rate (activity, spend) where the SHAPE matters more than exact values.
export function Sparkline({
  data,
  tone = "accent",
  height = 40,
  width = 160,
}: {
  data: number[];
  tone?: Tone;
  height?: number;
  width?: number;
}) {
  if (data.length < 2) {
    return <div className="flex items-center justify-center text-[10px] text-muted" style={{ height }}>collecting…</div>;
  }
  const max = Math.max(...data, 1);
  const min = Math.min(...data, 0);
  const span = max - min || 1;
  const stepX = width / (data.length - 1);
  const pts = data.map((v, i) => {
    const x = i * stepX;
    const y = height - ((v - min) / span) * (height - 4) - 2;
    return [x, y] as const;
  });
  const line = pts.map(([x, y], i) => `${i === 0 ? "M" : "L"}${x.toFixed(1)},${y.toFixed(1)}`).join(" ");
  const area = `${line} L${width},${height} L0,${height} Z`;
  return (
    <svg width="100%" height={height} viewBox={`0 0 ${width} ${height}`} preserveAspectRatio="none">
      <path d={area} className={cn(FILL[tone], "opacity-10")} />
      <path d={line} fill="none" strokeWidth={2} strokeLinejoin="round" strokeLinecap="round" className={STROKE[tone]} />
    </svg>
  );
}

// SEG_SHADE gives stacked-breakdown segments a cohesive look: the same accent
// hue at decreasing opacity, ranked by size — so a category mix reads as one
// themed bar instead of a clash of arbitrary colors.
const SEG_SHADE = ["bg-accent", "bg-accent/70", "bg-accent/55", "bg-accent/40", "bg-accent/30", "bg-accent/20"];

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

// BarRow is a labelled horizontal bar — a value against a max — for ranked
// breakdowns (spend by model, counts by kind) that read better than a list.
export function BarRow({
  label,
  value,
  max,
  display,
  tone = "accent",
}: {
  label: string;
  value: number;
  max: number;
  display?: string;
  tone?: Tone;
}) {
  const pct = max > 0 ? Math.max(2, Math.min(100, (value / max) * 100)) : 0;
  const barBg = { accent: "bg-accent", good: "bg-good", bad: "bg-bad", warn: "bg-amber-500", muted: "bg-muted" }[tone];
  return (
    <div className="flex items-center gap-2 text-xs">
      <span className="w-28 shrink-0 truncate text-muted" title={label}>
        {label}
      </span>
      <div className="h-2 flex-1 overflow-hidden rounded-full bg-panel">
        <div className={cn("h-full rounded-full transition-[width] duration-500", barBg)} style={{ width: `${pct}%` }} />
      </div>
      {display && <span className="shrink-0 tabular-nums text-foreground/80">{display}</span>}
    </div>
  );
}
