import { cn } from "@/lib/utils";

// Lightweight, dependency-free, theme-aware charts (inline SVG + flex bars).
// Colours come from `currentColor` driven by Tailwind text classes, so they
// follow the active theme and the strict CSP (no canvas, no chart lib).

// SpendArea draws a filled area + line for a monotonic-ish series (e.g.
// cumulative spend). Values are plotted left→right, scaled to their own max.
export function SpendArea({ values, className }: { values: number[]; className?: string }) {
  const w = 100;
  const h = 32;
  if (values.length === 0) return <div className="h-24 w-full" />;
  const max = Math.max(...values, 1);
  const nn = values.length;
  const pts = values.map((v, i) => {
    const x = nn === 1 ? w : (i / (nn - 1)) * w;
    const y = h - (v / max) * h;
    return [x, y] as const;
  });
  const line = pts.map(([x, y], i) => `${i === 0 ? "M" : "L"}${x.toFixed(2)},${y.toFixed(2)}`).join(" ");
  const area = `${line} L${w.toFixed(2)},${h} L0,${h} Z`;
  return (
    <svg viewBox={`0 0 ${w} ${h}`} preserveAspectRatio="none" className={cn("h-24 w-full text-accent", className)}>
      <path d={area} fill="currentColor" opacity={0.15} />
      <path d={line} fill="none" stroke="currentColor" strokeWidth={0.9} vectorEffect="non-scaling-stroke" />
    </svg>
  );
}

export interface BarRow {
  label: string;
  value: number;
  sub?: string;
}

// BarList renders horizontal bars normalized to the largest value — used for
// per-model breakdowns. Bars use the accent colour; values are right-aligned.
export function BarList({ rows, className }: { rows: BarRow[]; className?: string }) {
  const max = Math.max(1, ...rows.map((r) => r.value));
  if (rows.length === 0) return <div className="text-xs text-muted">no data</div>;
  return (
    <ul className={cn("space-y-1.5", className)}>
      {rows.map((r) => (
        <li key={r.label}>
          <div className="mb-0.5 flex items-baseline justify-between gap-2 text-xs">
            <span className="truncate font-mono">{r.label}</span>
            <span className="shrink-0 tabular-nums text-muted">{r.sub}</span>
          </div>
          <div className="h-1.5 overflow-hidden rounded-full bg-panel">
            <div className="h-full rounded-full bg-accent/70" style={{ width: `${(r.value / max) * 100}%` }} />
          </div>
        </li>
      ))}
    </ul>
  );
}

// OutcomeBar is a single stacked bar splitting completed / failed / running.
export function OutcomeBar({ completed, failed, running }: { completed: number; failed: number; running: number }) {
  const total = Math.max(1, completed + failed + running);
  const seg = (v: number, cls: string, title: string) =>
    v > 0 ? <div className={cls} style={{ width: `${(v / total) * 100}%` }} title={`${title}: ${v}`} /> : null;
  return (
    <div className="space-y-1.5">
      <div className="flex h-2.5 overflow-hidden rounded-full bg-panel">
        {seg(completed, "bg-good", "completed")}
        {seg(failed, "bg-bad", "failed")}
        {seg(running, "bg-accent", "running")}
      </div>
      <div className="flex flex-wrap gap-x-3 gap-y-0.5 text-xs text-muted">
        <Legend dot="bg-good" label="completed" value={completed} />
        <Legend dot="bg-bad" label="failed" value={failed} />
        <Legend dot="bg-accent" label="running" value={running} />
      </div>
    </div>
  );
}

function Legend({ dot, label, value }: { dot: string; label: string; value: number }) {
  return (
    <span className="inline-flex items-center gap-1">
      <span className={cn("size-2 rounded-full", dot)} /> {label} <span className="tabular-nums text-foreground">{value}</span>
    </span>
  );
}
