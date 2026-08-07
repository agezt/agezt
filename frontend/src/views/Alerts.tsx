import { useEffect, useRef, useState } from "react";
import { ShieldAlert, AlertTriangle, Info, Bell, BellOff, X, RotateCcw } from "lucide-react";
import { useEvents, type AgentEvent } from "@/lib/events";
import { getJSON } from "@/lib/api";
import { focusRun } from "@/lib/runfocus";
import { classifyAlert, type Alert, type AlertLevel } from "@/lib/alerts";
import { incidentMetaFromEvent, incidentRootId } from "@/lib/incidents";
import { openIncident } from "@/lib/incidentnav";
import { cn, fmtWhen } from "@/lib/utils";
import { IncidentBadges } from "@/components/IncidentBadges";
import { Badge } from "@/components/ui/badge";
import { TabNav } from "@/components/ui/tab-nav";
import { MetricWidget, MetricGrid } from "@/components/ui/metric-widget";
import { Page } from "@/components/ui/page";

const MAX_ALERTS = 100;

// Dismissed-alert ids persist per browser (there is no server-side alert store —
// alerts are classified client-side from journal events), so an acknowledged
// signal stays acknowledged across reloads instead of resurfacing every visit.
const DISMISSED_KEY = "agezt.alerts.dismissed.v1";
const DISMISSED_CAP = 400;

export function loadDismissed(): Set<string> {
  try {
    const raw = JSON.parse(localStorage.getItem(DISMISSED_KEY) || "[]");
    return new Set(Array.isArray(raw) ? raw.filter((v) => typeof v === "string") : []);
  } catch {
    return new Set();
  }
}

function saveDismissed(ids: Set<string>) {
  try {
    localStorage.setItem(DISMISSED_KEY, JSON.stringify([...ids].slice(-DISMISSED_CAP)));
  } catch {
    /* storage full/blocked: dismissal stays session-only */
  }
}

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
  const [dismissed, setDismissed] = useState<Set<string>>(loadDismissed);
  const [showDismissed, setShowDismissed] = useState(false);
  const seeded = useRef(false);

  function dismiss(id: string) {
    setDismissed((prev) => {
      const next = new Set(prev);
      next.add(id);
      saveDismissed(next);
      return next;
    });
  }

  function restore(id: string) {
    setDismissed((prev) => {
      const next = new Set(prev);
      next.delete(id);
      saveDismissed(next);
      return next;
    });
  }

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

  // Counts and tabs reflect the LIVE (non-dismissed) alerts so the numbers here
  // agree with the nav badge; dismissed history stays reachable via the toggle.
  const live = rows.filter((r) => !dismissed.has(r.id));
  const dismissedRows = rows.filter((r) => dismissed.has(r.id));
  const counts = live.reduce(
    (acc, r) => ((acc[r.level] = (acc[r.level] || 0) + 1), acc),
    {} as Record<AlertLevel, number>,
  );
  const base = showDismissed ? rows : live;
  const shown = filter === "all" ? base : base.filter((r) => r.level === filter);

  return (
    <Page
      icon={Bell}
      title="Alerts"
      width="readable"
      actions={
        <Badge variant={connected ? "good" : "bad"}>{connected ? "live" : "offline"}</Badge>
      }
    >
      <TabNav
        tabs={[
          {
            id: "all",
            label: "All",
            icon: Bell,
            count: live.length,
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
        <MetricWidget icon={Bell} label="Total" value={live.length} tone={live.length > 0 ? "accent" : "muted"} />
        <MetricWidget icon={ShieldAlert} label="Critical" value={counts.critical || 0} tone={(counts.critical || 0) > 0 ? "bad" : "muted"} />
        <MetricWidget icon={AlertTriangle} label="Warning" value={counts.warning || 0} tone={(counts.warning || 0) > 0 ? "warn" : "muted"} />
        <MetricWidget icon={Info} label="Info" value={counts.info || 0} tone={(counts.info || 0) > 0 ? "accent" : "muted"} />
      </MetricGrid>

      {dismissedRows.length > 0 && (
        <button
          onClick={() => setShowDismissed((v) => !v)}
          className="self-start text-xs font-medium text-muted transition-colors hover:text-foreground"
        >
          {showDismissed
            ? "hide dismissed"
            : `${dismissedRows.length} dismissed · show`}
        </button>
      )}

      {shown.length === 0 ? (
        <div className="flex flex-1 flex-col items-center justify-center gap-2 text-muted">
          <BellOff className="size-8 opacity-40" />
          <span className="text-sm">{live.length === 0 ? "no alerts — all quiet" : "none at this level"}</span>
        </div>
      ) : (
        <ul className="space-y-1.5">
          {shown.map((r) => {
              const s = LEVEL_STYLE[r.level];
              const Icon = s.icon;
              const isDismissed = dismissed.has(r.id);
              return (
                <li
                  key={r.id}
                  className={cn(
                    "group flex items-start gap-2 rounded-lg border px-3 py-2",
                    s.ring,
                    isDismissed && "opacity-50",
                  )}
                >
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
                        {r.tsMs ? fmtWhen(r.tsMs) : ""}
                      </span>
                      {isDismissed ? (
                        <button
                          onClick={() => restore(r.id)}
                          className="shrink-0 text-muted transition-colors hover:text-foreground"
                          title="Restore this alert"
                        >
                          <RotateCcw className="size-3.5" />
                        </button>
                      ) : (
                        <button
                          onClick={() => dismiss(r.id)}
                          className="shrink-0 text-muted opacity-0 transition-all group-hover:opacity-100"
                          title="Dismiss — acknowledged, stop showing it"
                        >
                          <X className="size-3.5" />
                        </button>
                      )}
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
      )}
    </Page>
  );
}
