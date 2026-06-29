import { useEffect, useRef, useState } from "react";
import { ShieldAlert, AlertTriangle, Info, Bell, BellOff } from "lucide-react";
import { useEvents, type AgentEvent } from "@/lib/events";
import { getJSON } from "@/lib/api";
import { focusRun } from "@/lib/runfocus";
import { classifyAlert, type Alert, type AlertLevel } from "@/lib/alerts";
import { incidentMetaFromEvent, incidentRootId } from "@/lib/incidents";
import { openIncident } from "@/lib/incidentnav";
import { cn, fmtTime } from "@/lib/utils";
import { IncidentBadges } from "@/components/IncidentBadges";
import { Badge } from "@/components/ui/badge";
import { TabNav } from "@/components/ui/tab-nav";
import { MetricWidget, MetricGrid } from "@/components/ui/metric-widget";

const MAX_ALERTS = 100;

interface AlertRow extends Alert {
  id: string;
  tsMs?: number;
  kind: string;
  subject?: string;
  payload?: any;
  correlationId?: string;
  incidentId?: string;
  rootIncidentId?: string;
}

// mergeAlerts combines two alert lists into one newest-first, deduped by id, capped list
// (M777). Used to fold the journal-history backfill together with the live SSE feed
// without double-counting an alert that appears in both.
export function mergeAlerts(a: AlertRow[], b: AlertRow[]): AlertRow[] {
  const seen = new Set<string>();
  const out: AlertRow[] = [];
  for (const r of [...a, ...b]) {
    if (seen.has(r.id)) continue;
    seen.add(r.id);
    out.push(r);
  }
  out.sort((x, y) => (y.tsMs ?? 0) - (x.tsMs ?? 0));
  return out.slice(0, MAX_ALERTS);
}

const LEVEL_STYLE: Record<AlertLevel, { ring: string; text: string; icon: typeof Info }> = {
  critical: { ring: "border-bad/60 bg-bad/5", text: "text-bad", icon: ShieldAlert },
  warning: { ring: "border-warn/60 bg-warn/5", text: "text-warn", icon: AlertTriangle },
  info: { ring: "border-border bg-card", text: "text-muted", icon: Info },
};

function rowOf(e: AgentEvent): AlertRow | null {
  const a = classifyAlert(e);
  if (!a) return null;
  const meta = incidentMetaFromEvent(e);
  return {
    ...a,
    id: e.id || `${e.kind}-${e.seq ?? Math.random()}`,
    tsMs: e.ts_unix_ms,
    kind: e.kind || "",
    subject: e.subject,
    payload: e.payload,
    correlationId: e.correlation_id,
    incidentId: meta.incidentId,
    rootIncidentId: meta.rootIncidentId,
  };
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
      setRows((prev) => mergeAlerts(prev, seed));
    }
    return subscribe((e) => {
      const r = rowOf(e);
      if (!r) return;
      setRows((prev) => mergeAlerts([r], prev)); // dedupe a live event already in history
    });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [subscribe]);

  // Backfill from the journal on mount (M777): the SSE stream only carries events from
  // connect-time forward, so without this the view would miss everything the agent
  // flagged while you weren't watching (and lose its history on every reload). Classify
  // a recent journal slice through the same rules as the live feed, and merge.
  useEffect(() => {
    let alive = true;
    getJSON<{ events?: AgentEvent[] }>("/api/journal", { limit: "500" })
      .then((d) => {
        if (!alive) return;
        const hist = (d.events || []).map(rowOf).filter(Boolean) as AlertRow[];
        if (hist.length) setRows((prev) => mergeAlerts(prev, hist));
      })
      .catch(() => {});
    return () => {
      alive = false;
    };
  }, []);

  const counts = rows.reduce(
    (acc, r) => ((acc[r.level] = (acc[r.level] || 0) + 1), acc),
    {} as Record<AlertLevel, number>,
  );
  const shown = filter === "all" ? rows : rows.filter((r) => r.level === filter);

  return (
    <div className="flex h-full min-h-0 flex-col gap-3">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-3">
          <span className="grid size-9 shrink-0 place-items-center rounded-xl bg-gradient-to-br from-accent/25 to-accent2/20 text-accent ring-1 ring-inset ring-accent/30">
            <Bell className="size-5" />
          </span>
          <div>
            <h2 className="text-gradient text-base font-bold leading-tight tracking-normal">Alerts</h2>
            <Badge variant={connected ? "good" : "bad"} className="mt-0.5">
              {connected ? "live" : "offline"}
            </Badge>
          </div>
        </div>
      </div>

      <TabNav
        tabs={[
          {
            id: "all",
            label: "All",
            icon: Bell,
            count: rows.length,
            content: null,
          },
          {
            id: "critical",
            label: "Critical",
            icon: ShieldAlert,
            count: counts.critical || 0,
            content: null,
          },
          {
            id: "warning",
            label: "Warning",
            icon: AlertTriangle,
            count: counts.warning || 0,
            content: null,
          },
          {
            id: "info",
            label: "Info",
            icon: Info,
            count: counts.info || 0,
            content: null,
          },
        ]}
        value={filter}
        onValueChange={(v) => setFilter(v as AlertLevel | "all")}
      />

      <MetricGrid cols="repeat(auto-fill, minmax(140px, 1fr))">
        <MetricWidget icon={Bell} label="Total" value={rows.length} tone={rows.length > 0 ? "accent" : "muted"} />
        <MetricWidget icon={ShieldAlert} label="Critical" value={counts.critical || 0} tone={(counts.critical || 0) > 0 ? "bad" : "muted"} />
        <MetricWidget icon={AlertTriangle} label="Warning" value={counts.warning || 0} tone={(counts.warning || 0) > 0 ? "warn" : "muted"} />
        <MetricWidget icon={Info} label="Info" value={counts.info || 0} tone={(counts.info || 0) > 0 ? "accent" : "muted"} />
      </MetricGrid>

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
                      {r.source === "doctor" && (
                        <IncidentBadges
                          item={{
                            subject: r.subject,
                            phase: r.payload?.phase,
                            mode: r.payload?.mode,
                          }}
                        />
                      )}
                      <span className="ml-auto shrink-0 text-xs tabular-nums text-muted">
                        {r.tsMs ? fmtTime(r.tsMs) : ""}
                      </span>
                    </div>
                    {r.detail && <div className="mt-0.5 break-words text-xs text-foreground/80">{r.detail}</div>}
                    <div className="mt-0.5 flex items-center gap-2 text-xs text-muted">
                      <span className="rounded bg-panel px-1.5 py-0.5 font-medium">{r.source}</span>
                      <span className="font-mono opacity-70">{r.kind}</span>
                      {incidentRootId(r) && (
                        <button
                          onClick={() => openIncident(incidentRootId(r))}
                          className="font-medium text-accent/80 transition-colors hover:text-accent"
                          title="Open the doctor incident this alert belongs to"
                        >
                          incident →
                        </button>
                      )}
                      {r.correlationId && (
                        <button
                          onClick={() => { focusRun(r.correlationId!); location.hash = "runs"; }}
                          className="ml-auto font-medium text-accent/80 transition-colors hover:text-accent"
                          title="Open the run this alert came from"
                        >
                          open run →
                        </button>
                      )}
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
