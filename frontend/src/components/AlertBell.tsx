import { useEffect, useState } from "react";
import { Bell } from "lucide-react";
import { useEvents } from "@/lib/events";
import { classifyAlert } from "@/lib/alerts";
import { cn } from "@/lib/utils";

// AlertBell is the global proactive-signal indicator: it lives in the header on
// EVERY view and counts the daemon's warning/critical alerts (self-health
// degradations, failures, halts, …) as they stream in — so a problem is visible
// no matter which panel you're on, not just on the Alerts tab. Clicking it jumps
// to Alerts and clears the count (an acknowledge).
export function AlertBell() {
  const { subscribe } = useEvents();
  const [warn, setWarn] = useState(0);
  const [crit, setCrit] = useState(0);

  useEffect(
    () =>
      subscribe((e) => {
        const a = classifyAlert(e);
        if (!a) return;
        if (a.level === "critical") setCrit((n) => n + 1);
        else if (a.level === "warning") setWarn((n) => n + 1);
      }),
    [subscribe],
  );

  const total = warn + crit;
  const has = total > 0;

  function open() {
    setWarn(0);
    setCrit(0);
    if (location.hash.replace(/^#\/?/, "") !== "alerts") location.hash = "alerts";
  }

  return (
    <button
      onClick={open}
      title={has ? `${crit} critical · ${warn} warning — view Alerts` : "Alerts"}
      className={cn(
        "relative inline-flex size-8 items-center justify-center rounded-md border transition-colors",
        has ? "border-border text-foreground hover:border-accent" : "border-border text-muted hover:text-foreground",
      )}
    >
      <Bell className={cn("size-4", has && crit > 0 && "animate-pulse text-bad", has && crit === 0 && "text-warn")} />
      {has && (
        <span
          className={cn(
            "absolute -right-1.5 -top-1.5 inline-flex min-w-4 items-center justify-center rounded-full px-1 text-xs font-semibold text-white tabular-nums",
            crit > 0 ? "bg-bad" : "bg-warn",
          )}
        >
          {total > 99 ? "99+" : total}
        </span>
      )}
    </button>
  );
}
