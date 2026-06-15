import { cn } from "@/lib/utils";

// Sparkline (M976) — a zero-dependency inline trend graphic: a smoothed area +
// line for a small number series, sized to fit beside a counter. No chart lib,
// no axes, no labels — just the shape of recent movement. Pure SVG so it tints
// with the theme via stroke/fill utility classes.

// sparkPaths maps a value series into SVG line + area path `d` strings inside a
// (w × h) box with a small vertical padding. Exported pure for unit tests. A
// flat/empty series yields a centered baseline so the element still renders.
export function sparkPaths(points: number[], w: number, h: number, pad = 1): { line: string; area: string } {
  const n = points.length;
  if (n === 0) {
    const mid = h / 2;
    return { line: `M0 ${mid} L${w} ${mid}`, area: "" };
  }
  const min = Math.min(...points);
  const max = Math.max(...points);
  const span = max - min || 1;
  const innerH = h - pad * 2;
  const x = (i: number) => (n === 1 ? w / 2 : (i / (n - 1)) * w);
  const y = (v: number) => pad + innerH - ((v - min) / span) * innerH;
  let line = `M${x(0).toFixed(2)} ${y(points[0]).toFixed(2)}`;
  for (let i = 1; i < n; i++) line += ` L${x(i).toFixed(2)} ${y(points[i]).toFixed(2)}`;
  const area = `${line} L${w.toFixed(2)} ${h} L0 ${h} Z`;
  return { line, area };
}

export function Sparkline({
  points,
  width = 64,
  height = 18,
  className,
  strokeClass = "stroke-accent",
  fillClass = "fill-accent/15",
}: {
  points: number[];
  width?: number;
  height?: number;
  className?: string;
  strokeClass?: string;
  fillClass?: string;
}) {
  const { line, area } = sparkPaths(points, width, height);
  return (
    <svg
      width={width}
      height={height}
      viewBox={`0 0 ${width} ${height}`}
      preserveAspectRatio="none"
      className={cn("overflow-visible", className)}
      aria-hidden="true"
    >
      {area && <path d={area} className={cn(fillClass, "stroke-none")} />}
      <path d={line} className={cn(strokeClass, "fill-none")} strokeWidth={1.5} strokeLinejoin="round" strokeLinecap="round" />
    </svg>
  );
}
