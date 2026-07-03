import { useEffect, useState } from "react";
import { Activity, AlertTriangle, Loader2 } from "lucide-react";
import { cn } from "@/lib/utils";
import { connectionState, useEvents } from "@/lib/events";

// ConnectionChip — the three-state "live / stale / disconnected" indicator
// that lives in the app header. Owns its own ticking clock so the App
// itself doesn't have to re-render every second; we re-read only the
// derived connectionState when (a) the underlying provider state changes
// (event arrives, socket opens/closes) or (b) our 1 s tick crosses the
// STALE_MS threshold.

const TICK_MS = 1_000;

export function ConnectionChip() {
  const { connected, lastEventAt } = useEvents();
  const [now, setNow] = useState(() => Date.now());

  useEffect(() => {
    // Re-tick every second so the age-label stays fresh. Cheap — setState of
    // a number once per second, and only this leaf component subscribes.
    const id = window.setInterval(() => setNow(Date.now()), TICK_MS);
    return () => window.clearInterval(id);
  }, []);

  const { state, label } = connectionState({ connected, lastEventAt }, now);

  const dot =
    state === "live"
      ? "bg-good shadow-[0_0_4px_currentColor] text-good"
      : state === "stale"
        ? "bg-warn/80 text-warn"
        : "bg-bad/80 text-bad";
  const Icon = state === "live" ? Activity : state === "stale" ? Loader2 : AlertTriangle;
  const tone =
    state === "live"
      ? "border-good/30 bg-good/10 text-good"
      : state === "stale"
        ? "border-warn/30 bg-warn/10 text-warn"
        : "border-bad/30 bg-bad/10 text-bad";

  return (
    <span
      className={cn(
        "inline-flex items-center gap-1.5 rounded-full border px-2 py-0.5 text-xs font-medium uppercase tracking-wide",
        tone,
      )}
      role="status"
      aria-live="polite"
      title={label}
      data-connection-state={state}
    >
      <span className={cn("inline-block size-1.5 rounded-full", dot)} aria-hidden="true" />
      <Icon className={cn("size-3", state === "stale" && "animate-spin")} aria-hidden="true" />
      <span className="sr-only">{state}</span>
    </span>
  );
}
