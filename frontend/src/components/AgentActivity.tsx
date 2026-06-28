import { useEffect, useMemo, useState } from "react";
import { Activity, ListTree, ScrollText } from "lucide-react";
import { getJSON } from "@/lib/api";
import { cn, fmtTime } from "@/lib/utils";
import { money } from "@/lib/format";
import { SkeletonList } from "@/components/ui/skeleton";
import { Badge, statusVariant } from "@/components/ui/badge";
import { ErrorText } from "@/components/JsonView";
import { useEvents } from "@/lib/events";
import { agentActivityEventMatches, agentActivityOperationalState, agentActivityPulse, agentRunCorrelations, filterAgentLogEvents } from "@/lib/agentactivity";
import { RunDetailLoader } from "@/components/RunDetail";
import { IncidentBadges } from "@/components/IncidentBadges";
import {
  incidentBadgeItem,
  incidentEventSummary,
  isIncidentFamilyEvent,
} from "@/lib/incidentevents";

interface ActivityItem {
  seq: number;
  kind: string;
  ts_unix_ms?: number;
  correlation_id?: string;
  summary: string;
}

// RunLite is the subset of /api/runs we need to list an agent's own runs and
// derive its spend/last-active summary. Mirrors ApiRun in Agents.tsx.
interface RunLite {
  correlation_id?: string;
  status?: string;
  model?: string;
  spent_mc?: number;
  iters?: number;
  intent?: string;
  agent?: string;
  started_unix_ms?: number;
}

type DrillTab = "activity" | "runs" | "logs";

// AgentActivity is the per-agent drill-in (M941): a tabbed, live view of one
// agent — its journal-derived activity timeline (M854), its own runs (with
// inline run detail + steer), and a live raw-log tail filtered from the SSE
// stream. Re-fetches on any event attributable to the agent so it tracks a
// running agent in real time. Reuses RunDetailLoader, the Skeleton kit, and
// the live events context. Extracted from views/Roster (M953) so the Roster
// drill-in and the Command Center deep panel share one implementation.
export function AgentActivity({
  slug,
  initialOpenRun,
  initialTab = "activity",
}: {
  slug: string;
  initialOpenRun?: string;
  initialTab?: DrillTab;
}) {
  const { events, subscribe } = useEvents();
  const [tab, setTab] = useState<DrillTab>(initialTab);
  const [items, setItems] = useState<ActivityItem[] | null>(null);
  const [runs, setRuns] = useState<RunLite[] | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [focusedRun, setFocusedRun] = useState<string | null>(initialOpenRun || null);
  const [inlineRun, setInlineRun] = useState<string | null>(null);
  const [bump, setBump] = useState(0);

  useEffect(() => {
    if (!initialOpenRun) return;
    setFocusedRun(initialOpenRun);
    setInlineRun(null);
    setTab(initialTab);
  }, [initialOpenRun, initialTab]);

  // Re-load the digested timeline + run list whenever the agent changes or a
  // fresh event for this agent lands (debounced via the bump counter below).
  useEffect(() => {
    let alive = true;
    Promise.all([
      getJSON<{ activity?: ActivityItem[] }>("/api/agents/activity", { ref: slug, limit: "60" }),
      getJSON<{ runs?: RunLite[] }>("/api/runs"),
    ])
      .then(([a, r]) => {
        if (!alive) return;
        setItems(a.activity || []);
        setRuns((r.runs || []).filter((x) => x.agent === slug));
      })
      .catch((e) => alive && setErr((e as Error).message));
    return () => {
      alive = false;
    };
  }, [slug, bump]);
  const runCorrs = useMemo(() => agentRunCorrelations(runs), [runs]);

  // Live: any event attributable to this agent (direct actor, one of its run
  // correlations, or a targeted doctor/roster event) triggers a debounced
  // refetch so the timeline/runs track it without a manual reload.
  useEffect(() => {
    let t: ReturnType<typeof setTimeout> | undefined;
    const off = subscribe((e) => {
      if (!agentActivityEventMatches(e, slug, runCorrs)) return;
      if (t) clearTimeout(t);
      t = setTimeout(() => setBump((b) => b + 1), 700);
    });
    return () => {
      if (t) clearTimeout(t);
      off();
    };
  }, [slug, subscribe, runCorrs]);

  // Live raw-log tail: the rolling SSE buffer filtered to this agent. No fetch —
  // it updates as events arrive.
  const logs = filterAgentLogEvents(events, slug, runCorrs);
  const pulse = agentActivityPulse(runs, logs);
  const operationalState = agentActivityOperationalState(pulse);

  const runCount = runs?.length ?? 0;
  const totalSpent = (runs || []).reduce((s, r) => s + (r.spent_mc || 0), 0);
  const lastActive = items && items.length > 0 ? items[0].ts_unix_ms : undefined;

  const tabBtn = (id: DrillTab, label: string, count: number | undefined, Icon: typeof Activity) => (
    <button
      onClick={() => setTab(id)}
      className={cn(
        "flex items-center gap-1 rounded px-2 py-1 text-[11px] font-medium transition-colors",
        tab === id ? "bg-card text-foreground" : "text-muted hover:text-foreground",
      )}
    >
      <Icon className="h-3 w-3" />
      {label}
      {count !== undefined && count > 0 && (
        <span className="ml-0.5 rounded bg-panel px-1 font-mono text-[9px] text-muted">{count}</span>
      )}
    </button>
  );

  return (
    <div className="mt-2 rounded-md border border-border bg-panel/60 p-2">
      {/* summary band */}
      <div className="mb-2 flex flex-wrap items-center gap-x-4 gap-y-1 text-[11px] text-muted">
        <span
          title={operationalState.detail}
          className={cn(
            "rounded-md border border-border bg-card px-1.5 py-0.5 font-medium",
            operationalState.tone === "accent" && "border-accent/30 bg-accent/10 text-accent",
            operationalState.tone === "good" && "border-good/30 bg-good/10 text-good",
            operationalState.tone === "warn" && "border-warn/35 bg-warn/10 text-warn",
          )}
        >
          {operationalState.value}
        </span>
        <span
          title={pulse.detail}
          className={cn(
            "rounded-md border border-border bg-card px-1.5 py-0.5 font-medium",
            pulse.tone === "good" && "border-good/30 bg-good/10 text-good",
            pulse.tone === "warn" && "border-warn/35 bg-warn/10 text-warn",
          )}
        >
          {pulse.value}
        </span>
        <span><span className="font-mono text-foreground">{runCount}</span> runs</span>
        <span><span className="font-mono text-foreground">{money(totalSpent)}</span> spent</span>
        <span>last active <span className="font-mono text-foreground/85">{lastActive ? fmtTime(lastActive) : "—"}</span></span>
        {(pulse.doctorEvents > 0 || pulse.delegations > 0 || pulse.mailboxEvents > 0) && (
          <span className="truncate" title={pulse.detail}>{pulse.detail}</span>
        )}
      </div>

      {/* tabs */}
      <div className="mb-2 flex items-center gap-1 border-b border-border pb-1">
        {tabBtn("activity", "Activity", items?.length, Activity)}
        {tabBtn("runs", "Runs", runCount, ListTree)}
        {tabBtn("logs", "Logs", logs.length, ScrollText)}
      </div>

      {err && <ErrorText>{err}</ErrorText>}

      {focusedRun && (
        <div className="mb-2 rounded-md border border-border bg-card/40 p-2">
          <div className="mb-1 flex items-center gap-2 text-[10px] uppercase tracking-normal text-muted">
            <span>focused run</span>
            <span className="font-mono normal-case text-foreground/80">{focusedRun}</span>
            <button
              className="ml-auto rounded px-1 text-muted hover:bg-panel hover:text-foreground"
              onClick={() => setFocusedRun(null)}
              title="Close focused run"
            >
              close
            </button>
          </div>
          <RunDetailLoader correlationId={focusedRun} status="running" />
        </div>
      )}

      {tab === "activity" && (
        !items ? (
          <SkeletonList count={4} lines={1} />
        ) : items.length === 0 ? (
          <div className="text-xs text-muted">no recorded activity yet</div>
        ) : (
          <ul className="space-y-1">
            {items.map((a) => {
              const focused = !!a.correlation_id && focusedRun === a.correlation_id;
              const open = !!a.correlation_id && inlineRun === a.correlation_id && !focused;
              return (
                <li key={a.seq} className="text-xs">
                  <div
                    className={cn("flex items-start gap-2", a.correlation_id && "cursor-pointer")}
                    onClick={() => {
                      if (!a.correlation_id) return;
                      if (focused) {
                        setFocusedRun(null);
                        return;
                      }
                      setInlineRun(open ? null : a.correlation_id);
                    }}
                  >
                    <span className="shrink-0 rounded bg-card px-1.5 py-0.5 font-mono text-[10px] text-accent">{a.kind}</span>
                    <span className="text-foreground/85">{a.summary}</span>
                    {focused && <span className="rounded bg-accent/10 px-1.5 py-0.5 text-[10px] text-accent">focused</span>}
                    <span className="ml-auto shrink-0 font-mono text-[10px] text-muted opacity-70">{fmtTime(a.ts_unix_ms)}</span>
                  </div>
                  {open && (
                    <div className="mt-1 pl-2">
                      <RunDetailLoader correlationId={a.correlation_id} />
                    </div>
                  )}
                </li>
              );
            })}
          </ul>
        )
      )}

      {tab === "runs" && (
        !runs ? (
          <SkeletonList count={3} lines={1} />
        ) : runs.length === 0 ? (
          <div className="text-xs text-muted">this agent has no runs yet</div>
        ) : (
          <ul className="space-y-1">
            {runs.map((r) => {
              const focused = !!r.correlation_id && focusedRun === r.correlation_id;
              const open = inlineRun === r.correlation_id && !focused;
              return (
                <li key={r.correlation_id} className="text-xs">
                  <div
                    className="flex cursor-pointer items-center gap-2"
                    onClick={() => {
                      if (!r.correlation_id) return;
                      if (focused) {
                        setFocusedRun(null);
                        return;
                      }
                      setInlineRun(open ? null : r.correlation_id);
                    }}
                  >
                    <Badge variant={statusVariant(r.status)}>{r.status || "?"}</Badge>
                    <span className="truncate text-foreground/85">{r.intent || r.correlation_id || "run"}</span>
                    {focused && <span className="rounded bg-accent/10 px-1.5 py-0.5 text-[10px] text-accent">focused</span>}
                    <span className="ml-auto shrink-0 font-mono text-[10px] text-muted opacity-70">
                      {r.spent_mc ? money(r.spent_mc) + " · " : ""}{fmtTime(r.started_unix_ms)}
                    </span>
                  </div>
                  {open && (
                    <div className="mt-1 pl-2">
                      <RunDetailLoader correlationId={r.correlation_id} status={r.status} />
                    </div>
                  )}
                </li>
              );
            })}
          </ul>
        )
      )}

      {tab === "logs" && (
        logs.length === 0 ? (
          <div className="text-xs text-muted">no live events from this agent yet</div>
        ) : (
          <ul className="space-y-1">
            {logs.map((e, i) => (
              <li key={e.id || e.seq || i} className="flex items-start gap-2 text-xs">
                <span className="shrink-0 rounded bg-card px-1.5 py-0.5 font-mono text-[10px] text-accent">{e.kind}</span>
                {isIncidentFamilyEvent(e) && <IncidentBadges item={incidentBadgeItem(e)} mono />}
                <span className="truncate text-foreground/85">
                  {incidentEventSummary(e) || e.subject || ""}
                </span>
                <span className="ml-auto shrink-0 font-mono text-[10px] text-muted opacity-70">{fmtTime(e.ts_unix_ms)}</span>
              </li>
            ))}
          </ul>
        )
      )}
    </div>
  );
}
