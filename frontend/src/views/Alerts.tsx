import { useEffect, useRef, useState } from "react";
import { ShieldAlert, AlertTriangle, Info, Bell, BellOff } from "lucide-react";
import { useEvents, type AgentEvent } from "@/lib/events";
import { classifyAlert, type Alert, type AlertLevel } from "@/lib/alerts";
import { cn, fmtTime } from "@/lib/utils";

const MAX_ALERTS = 100;

interface AlertRow extends Alert {
  id: string;
  tsMs?: number;
  kind: string;
}

const LEVEL_STYLE: Record<AlertLevel, { ring: string; text: string; icon: typeof Info }> = {
  critical: { ring: "border-bad/60 bg-bad/5", text: "text-bad", icon: ShieldAlert },
  warning: { ring: "border-warn/60 bg-warn/5", text: "text-warn", icon: AlertTriangle },
  info: { ring: "border-border bg-card", text: "text-muted", icon: Info },
};

function rowOf(e: AgentEvent): AlertRow | null {
  const a = classifyAlert(e);
  if (!a) return null;
  return { ...a, id: e.id || `${e.kind}-${e.seq ?? Math.random()}`, tsMs: e.ts_unix_ms, kind: e.kind || "" };
}

// Alerts is the proactive-signal feed: what the daemon flagged ON ITS OWN —
// self-health degradations, pulse briefings, run failures, blocked egress,
// budget/rate trips, halts — distinct from the raw event firehose. It answers
// "is there anything I should look at?" at a glance.
export function Alerts() {
  const { events, subscribe, connected } = useEvents();
  const [rows, setRows] = useState<AlertRow[]>([]);
  const [filter, setFilter] = useState<AlertLevel | "all">("all");
  const seeded = useRef(false);

  // Seed once from the current firehose buffer, then live-append.
  useEffect(() => {
    if (!seeded.current) {
      seeded.current = true;
      const seed = events.map(rowOf).filter(Boolean) as AlertRow[];
      setRows(seed.slice(-MAX_ALERTS).reverse()); // newest first
    }
    return subscribe((e) => {
      const r = rowOf(e);
      if (!r) return;
      setRows((prev) => [r, ...prev].slice(0, MAX_ALERTS));
    });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [subscribe]);

  const counts = rows.reduce(
    (acc, r) => ((acc[r.level] = (acc[r.level] || 0) + 1), acc),
    {} as Record<AlertLevel, number>,
  );
  const shown = filter === "all" ? rows : rows.filter((r) => r.level === filter);

  return (
    <div className="flex h-full min-h-0 flex-col gap-3">
      <div className="flex flex-wrap items-center gap-2">
        <h2 className="flex items-center gap-2 text-sm font-semibold">
          <Bell className="size-4 text-accent" /> Alerts
        </h2>
        <span className={cn("inline-flex items-center gap-1 text-xs", connected ? "text-good" : "text-bad")}>
          ● {connected ? "live" : "offline"}
        </span>
        <div className="ml-auto flex items-center gap-1.5 text-xs">
          <Chip active={filter === "all"} onClick={() => setFilter("all")} label={`all ${rows.length}`} />
          <Chip
            active={filter === "critical"}
            onClick={() => setFilter("critical")}
            label={`critical ${counts.critical || 0}`}
            tone="text-bad"
          />
          <Chip
            active={filter === "warning"}
            onClick={() => setFilter("warning")}
            label={`warning ${counts.warning || 0}`}
            tone="text-warn"
          />
          <Chip active={filter === "info"} onClick={() => setFilter("info")} label={`info ${counts.info || 0}`} />
        </div>
      </div>

      {shown.length === 0 ? (
        <div className="flex flex-1 flex-col items-center justify-center gap-2 text-muted">
          <BellOff className="size-8 opacity-40" />
          <span className="text-sm">{rows.length === 0 ? "no alerts — all quiet" : "none at this level"}</span>
        </div>
      ) : (
        <div className="min-h-0 flex-1 overflow-auto">
          <ul className="space-y-1.5">
            {shown.map((r) => {
              const s = LEVEL_STYLE[r.level];
              const Icon = s.icon;
              return (
                <li key={r.id} className={cn("flex items-start gap-2 rounded-lg border px-3 py-2", s.ring)}>
                  <Icon className={cn("mt-0.5 size-4 shrink-0", s.text)} />
                  <div className="min-w-0 flex-1">
                    <div className="flex items-center gap-2">
                      <span className="truncate text-sm font-medium">{r.title}</span>
                      <span className="ml-auto shrink-0 text-[10px] tabular-nums text-muted">
                        {r.tsMs ? fmtTime(r.tsMs) : ""}
                      </span>
                    </div>
                    {r.detail && <div className="mt-0.5 break-words text-xs text-foreground/80">{r.detail}</div>}
                    <div className="mt-0.5 flex items-center gap-2 text-[10px] text-muted">
                      <span className="rounded bg-panel px-1.5 py-0.5 font-medium">{r.source}</span>
                      <span className="font-mono opacity-70">{r.kind}</span>
                    </div>
                  </div>
                </li>
              );
            })}
          </ul>
        </div>
      )}
    </div>
  );
}

function Chip({
  active,
  onClick,
  label,
  tone,
}: {
  active: boolean;
  onClick: () => void;
  label: string;
  tone?: string;
}) {
  return (
    <button
      onClick={onClick}
      className={cn(
        "rounded-full border px-2 py-0.5 transition-colors",
        active ? "border-accent text-accent" : "border-border text-muted hover:text-foreground",
        !active && tone,
      )}
    >
      {label}
    </button>
  );
}
