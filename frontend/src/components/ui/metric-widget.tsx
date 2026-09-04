import { cn } from "@/lib/utils";
import type { LucideIcon } from "lucide-react";
import { toneBg, toneBorder, toneChip, toneText, type Tone } from "@/lib/tone";

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
  // "muted" reads as plain foreground here (the value is the point, not its
  // state); every other tone comes from the shared colour language.
  const toneCls = tone === "muted" ? "text-foreground" : toneText[tone];
  // Background only: toneChip also carries a text colour, and two competing
  // `text-*` classes on one element resolve by stylesheet order, not by which
  // one you wrote last.
  const bgCls = toneBg[tone];

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

      {/* Big number — display face, so hero metrics share the headings' voice. */}
      <div className={cn("font-display text-3xl font-bold tabular-nums tracking-normal", toneCls)}>
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
//
// `cols` accepts either a CSS grid-template-columns VALUE
// ("repeat(auto-fill, minmax(140px, 1fr))") or Tailwind grid classes
// ("grid-cols-2 lg:grid-cols-5"). The class form used to be applied as an
// inline style — invalid CSS the browser dropped, silently collapsing the grid
// to one full-width column per widget (the Activity/Insights stacked-cards
// bug), so the component now detects it and routes it to className.
export function MetricGrid({
  children,
  cols = "auto-fill",
  className,
}: {
  children: React.ReactNode;
  cols?: string;
  className?: string;
}) {
  const isClassList = /(?:^|[\s:])grid-cols-/.test(cols);
  return (
    <div
      className={cn("stagger-in grid gap-3", isClassList && cols, className)}
      style={
        isClassList
          ? undefined
          : {
              gridTemplateColumns:
                cols === "auto-fill" ? "repeat(auto-fill, minmax(160px, 1fr))" : cols,
            }
      }
    >
      {children}
    </div>
  );
}

/**
 * StatTile — the COMPACT stat: one number, its label, and a tone. Sits in dense
 * rows where a full MetricWidget (with its trend, subvalue and hover lift)
 * would be too heavy.
 *
 * Five views had each written their own version of exactly this — Execution
 * Profiles, OKR, Taste, Workboard, the flight recorder — and no two agreed on
 * the padding, the label case, or which colour "good" was. A number means the
 * same thing on every page, so it should look the same on every page.
 */
export function StatTile({
  label,
  value,
  suffix,
  icon: Icon,
  tone = "muted",
  size = "md",
  className,
}: {
  label: string;
  value: React.ReactNode;
  suffix?: string;
  icon?: LucideIcon;
  tone?: Tone;
  /** "sm" for inline strips (flight recorder), "md" for page-level rows. */
  size?: "sm" | "md";
  className?: string;
}) {
  const sm = size === "sm";
  return (
    <div
      className={cn(
        "flex items-center gap-2.5 rounded-lg border bg-card/80",
        sm ? "px-2.5 py-1.5" : "px-3 py-2",
        toneBorder[tone],
        className,
      )}
    >
      {Icon && (
        <span className={cn("grid size-9 shrink-0 place-items-center rounded-lg", toneChip[tone])}>
          <Icon className="size-5" />
        </span>
      )}
      <div className="min-w-0">
        <div
          className={cn(
            "font-semibold leading-none tabular-nums",
            sm ? "text-sm" : "text-2xl",
            tone === "muted" ? "text-foreground" : toneText[tone],
          )}
        >
          {value}
          {suffix}
        </div>
        <div className="mt-1 truncate text-[11px] font-semibold uppercase tracking-normal text-muted">{label}</div>
      </div>
    </div>
  );
}
