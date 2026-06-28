import { useEffect, useRef, useState } from "react";
import { cn } from "@/lib/utils";

// AnimatedNumber count-ups (or down) to its target whenever the value changes
// (M975), so the cockpit's counters feel alive instead of snapping. Eases out
// over ~600ms, formats with thousands separators, and respects
// prefers-reduced-motion (renders the exact target with no animation). The
// initial render shows the target immediately, so it's test- and SSR-safe.
// A subtle "pulse" glow is added on change so the number feels alive.
export function AnimatedNumber({
  value,
  durationMs = 600,
  className,
}: {
  value: number;
  durationMs?: number;
  className?: string;
}) {
  const [display, setDisplay] = useState(value);
  const [pulse, setPulse] = useState(false);
  const fromRef = useRef(value);
  const rafRef = useRef<number | null>(null);
  const prevRef = useRef(value);

  useEffect(() => {
    if (!Number.isFinite(value)) {
      setDisplay(value);
      fromRef.current = value;
      return;
    }
    const reduce =
      typeof window !== "undefined" && window.matchMedia?.("(prefers-reduced-motion: reduce)").matches;
    const from = fromRef.current;
    if (reduce || from === value || typeof requestAnimationFrame === "undefined") {
      setDisplay(value);
      fromRef.current = value;
      return;
    }
    // Pulse on change
    if (value !== prevRef.current) {
      setPulse(true);
      setTimeout(() => setPulse(false), 400);
      prevRef.current = value;
    }
    const start = performance.now();
    const tick = (t: number) => {
      const p = Math.min(1, (t - start) / durationMs);
      const eased = 1 - Math.pow(1 - p, 3); // easeOutCubic
      setDisplay(from + (value - from) * eased);
      if (p < 1) {
        rafRef.current = requestAnimationFrame(tick);
      } else {
        fromRef.current = value;
      }
    };
    rafRef.current = requestAnimationFrame(tick);
    return () => {
      if (rafRef.current) cancelAnimationFrame(rafRef.current);
    };
  }, [value, durationMs]);

  if (!Number.isFinite(value)) return <span className={className}>{String(value)}</span>;
  return (
    <span
      className={cn(
        "transition-shadow duration-300",
        pulse && "shadow-[0_0_8px_-2px_var(--accent)] rounded-sm",
        className,
      )}
    >
      {Math.round(display).toLocaleString()}
    </span>
  );
}
