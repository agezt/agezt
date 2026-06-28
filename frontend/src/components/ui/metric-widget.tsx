import { cn } from "@/lib/utils";
import type { LucideIcon } from "lucide-react";

// MetricWidget — a single KPI at a glance. Large number, small label, optional
// icon + pulse + trend sparkline. Designed to replace the cramped BigStat grid
// cells with something that reads as a proper dashboard widget.
export function MetricWidget({
  icon: Icon,
  label,
  value,
  subvalue,
  tone = "accent",
  pulse = false,
  trend,
  className,
}: {
  icon?: LucideIcon;
  /** Human-readable label shown below the value */
  label: string;
  /** Primary metric — large, bold */
  value: React.ReactNode;
  /** Optional smaller sub-metric shown under the value */
  subvalue?: React.ReactNode;
  /** Visual tone: accent (live/primary), good (success), warn (caution), bad (error), muted */
  tone?: "accent" | "good" | "warn" | "bad" | "muted";
  /** Show a pulsing dot next to the icon — for live/running data */
  pulse?: boolean;
  /** Optional sparkline trend data (array of numbers) */
  trend?: number[];
  className?: string;
}) {
  const toneCls = {
    accent: "text-accent",
    good: "text-good",
    warn: "text-warn",
    bad: "text-bad",
    muted: "text-foreground",
  }[tone];

  const bgCls = {
    accent: "bg-accent/10",
    good: "bg-good/10",
    warn: "bg-warn/10",
    bad: "bg-bad/10",
    muted: "bg-panel",
  }[tone];

  return (
    <div
      className={cn(
        "flex flex-col gap-2 rounded-xl border border-border bg-card p-4 shadow-e1 transition-shadow hover:shadow-e2",
        className,
      )}
    >
      <div className="flex items-start justify-between gap-2">
        {/* Icon + label column */}
        <div className="flex flex-col gap-1">
          <div className={cn("inline-flex items-center gap-1.5 rounded-md px-1.5 py-0.5 text-xs font-medium", bgCls, toneCls)}>
            {Icon && <Icon className="size-3" aria-hidden />}
            <span>{label}</span>
            {pulse && (
              <span className="size-1.5 rounded-full bg-current animate-pulse" aria-hidden />
            )}
          </div>
          {subvalue && (
            <div className="text-xs text-muted">{subvalue}</div>
          )}
        </div>

        {/* Trend sparkline */}
        {trend && trend.length >= 2 && (
          <TrendSparkline data={trend} tone={tone} />
        )}
      </div>

      {/* Big number */}
      <div className={cn("text-3xl font-bold tabular-nums tracking-normal", toneCls)}>
        {value}
      </div>
    </div>
  );
}

// TrendSparkline — tiny inline SVG sparkline for a metric widget corner.
function TrendSparkline({
  data,
  tone,
}: {
  data: number[];
  tone: "accent" | "good" | "warn" | "bad" | "muted";
}) {
  const width = 64;
  const height = 28;
  const stroke = {
    accent: "var(--accent)",
    good: "var(--good)",
    warn: "var(--warn)",
    bad: "var(--bad)",
    muted: "var(--muted)",
  }[tone];

  if (data.length < 2) return null;

  const max = Math.max(...data, 1);
  const min = Math.min(...data, 0);
  const span = max - min || 1;
  const stepX = width / (data.length - 1);

  const pts = data.map((v, i) => {
    const x = i * stepX;
    const y = height - ((v - min) / span) * (height - 4) - 2;
    return [x, y] as const;
  });

  const path = pts.map(([x, y], i) => `${i === 0 ? "M" : "L"}${x.toFixed(1)},${y.toFixed(1)}`).join(" ");
  const area = `${path} L${width},${height} L0,${height} Z`;

  return (
    <svg width={width} height={height} viewBox={`0 0 ${width} ${height}`} className="opacity-70">
      <path d={area} fill={stroke} fillOpacity="0.1" />
      <path
        d={path}
        fill="none"
        stroke={stroke}
        strokeWidth={1.5}
        strokeLinejoin="round"
        strokeLinecap="round"
      />
    </svg>
  );
}

// MetricGrid — lays out MetricWidgets in a responsive grid.
export function MetricGrid({
  children,
  cols = "auto-fill",
  className,
}: {
  children: React.ReactNode;
  cols?: string;
  className?: string;
}) {
  return (
    <div
      className={cn("grid gap-3", className)}
      style={{
        gridTemplateColumns: cols === "auto-fill"
          ? "repeat(auto-fill, minmax(160px, 1fr))"
          : cols,
      }}
    >
      {children}
    </div>
  );
}
