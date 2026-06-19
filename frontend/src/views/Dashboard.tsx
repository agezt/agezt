import { useEffect, useRef, useState } from "react";
import { Activity, RefreshCw, Cpu, Wallet, ListTree, Network, Radio, CalendarClock, Gauge, ShieldAlert, AlertTriangle } from "lucide-react";
import { cn } from "@/lib/utils";
import { money } from "@/lib/format";
import { getJSON } from "@/lib/api";
import { ConnectivityStrip } from "@/views/Connections";
import { useEvents, type AgentEvent } from "@/lib/events";
import { recentAttentionAlerts, type RankedAlert } from "@/lib/alerts";
import { incidentRootId } from "@/lib/incidents";
import { focusRun } from "@/lib/runfocus";
import { openIncident } from "@/lib/incidentnav";
import { doctorIncidentPhase, doctorIncidentSourceLabel } from "@/lib/autonomy";
import {
  IncidentBadges,
  incidentPhaseBadgeClass,
  incidentSourceBadgeClass,
} from "@/components/IncidentBadges";
import { Button } from "@/components/ui/button";
import { fmtTime, clip } from "@/lib/utils";
import { Ring, Sparkline, BarRow } from "@/components/Widgets";
import { PageHeader } from "@/components/ui/page-header";
import { summarizeRoots, type RootSummary } from "@/views/Agents";
import { Bot, GitBranch, Coins, Repeat, Mail, Wrench, Skull, Pause } from "lucide-react";

// RunRow is the subset of /api/runs the cockpit folds into the active-agents
// panel — structurally compatible with the Agents view's run shape.
interface RunRow {
  correlation_id?: string;
  parent_correlation?: string;
  status?: string;
  model?: string;
  spent_mc?: number;
  iters?: number;
  intent?: string;
  started_unix_ms?: number;
}

interface Stats {
  total?: number;
  completed?: number;
  failed?: number;
  running?: number;
  success_rate?: number;
  avg_iters?: number;
  spent_microcents?: number;
  delegations?: number;
  by_model?: Record<string, { runs?: number; spent_microcents?: number }>;
}
interface Budget {
  spent_mc?: number;
  ceiling_mc?: number;
  strict_pricing?: boolean;
}
interface DashboardAgentProfile {
  slug: string;
  enabled?: boolean;
  retired?: boolean;
  system?: boolean;
  kind?: string;
  managed?: boolean;
  direct_callable?: boolean;
  status?: {
    active_run_count?: number;
    operational_state?: string;
    repair_inflight?: number;
    repair_state?: string;
    health_state?: string;
  };
}
interface DashboardBoardMessage {
  id?: string;
  from?: string;
  to?: string;
  reply_to?: string;
  acked_by?: string[];
}
export interface DashboardFleetOps {
  total: number;
  active: number;
  paused: number;
  running: number;
  repair: number;
  graveyard: number;
  mailboxAgents: number;
  mailboxBacklog: number;
  system: number;
  subagents: number;
}

const MAX_SERIES = 32;

// Dashboard is the cockpit: every key gauge of the running system at a glance —
// throughput and success rate as rings, today's spend against the ceiling, a live
// activity sparkline driven by the journal head, per-model cost as bars, and a
// live event ticker. One screen to see, understand, and monitor the whole daemon.
export function Dashboard() {
  const { events, connected } = useEvents();
  const [stats, setStats] = useState<Stats | null>(null);
  const [budget, setBudget] = useState<Budget | null>(null);
  const [status, setStatus] = useState<Record<string, any> | null>(null);
  const [series, setSeries] = useState<number[]>([]);
  const [alerts, setAlerts] = useState<RankedAlert[]>([]);
  const [active, setActive] = useState<RootSummary[]>([]);
  const [fleetOps, setFleetOps] = useState<DashboardFleetOps | null>(null);
  const [loading, setLoading] = useState(false);
  const lastHead = useRef<number | null>(null);

  async function refresh() {
    setLoading(true);
    const [s, b, st, j, r, a, bd] = await Promise.allSettled([
      getJSON<Stats>("/api/stats"),
      getJSON<Budget>("/api/budget"),
      getJSON<Record<string, any>>("/api/status"),
      getJSON<{ events?: AgentEvent[] }>("/api/journal", { limit: "300" }),
      getJSON<{ runs?: RunRow[] }>("/api/runs"),
      getJSON<{ profiles?: DashboardAgentProfile[] }>("/api/agents"),
      getJSON<{ messages?: DashboardBoardMessage[] }>("/api/board", { limit: "200" }),
    ]);
    if (s.status === "fulfilled") setStats(s.value);
    if (b.status === "fulfilled") setBudget(b.value);
    // Live fleet (M914): the lead runs currently in flight, folded from /api/runs
    // with each one's sub-agent subtree (reuses the Agents gallery's summarizer).
    if (r.status === "fulfilled")
      setActive(summarizeRoots(r.value.runs || []).filter((x) => x.kind === "running"));
    // Recency-bounded + halt-resolved (M913): the journal backfills weeks of
    // history, so without a window an old halt/failure would sit in "Needs
    // attention" forever.
    if (j.status === "fulfilled")
      setAlerts(recentAttentionAlerts(j.value.events || [], { limit: 4, nowMs: Date.now() }));
    if (a.status === "fulfilled") {
      const messages = bd.status === "fulfilled" ? bd.value.messages || [] : [];
      setFleetOps(dashboardFleetOpsSummary(a.value.profiles || [], messages));
    }
    if (st.status === "fulfilled") {
      setStatus(st.value);
      // Activity rate = growth of the journal head between samples (events/tick).
      const head = Number(st.value.journal_head ?? 0);
      if (lastHead.current !== null) {
        const delta = Math.max(0, head - lastHead.current);
        setSeries((prev) => [...prev, delta].slice(-MAX_SERIES));
      }
      lastHead.current = head;
    }
    setLoading(false);
  }

  useEffect(() => {
    refresh();
    const id = setInterval(refresh, 5000);
    return () => clearInterval(id);
  }, []);

  // Snappy refresh right after a run starts or ends.
  const head = events[0]?.kind;
  useEffect(() => {
    if (head === "task.received" || head === "task.completed" || head === "task.failed" || head === "schedule.fired") refresh();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [head, events[0]?.id]);

  const spent = budget?.spent_mc ?? stats?.spent_microcents ?? 0;
  const ceiling = budget?.ceiling_mc ?? 0;
  const pctUsed = ceiling > 0 ? Math.min(100, (spent / ceiling) * 100) : 0;
  const model = (status?.model as string) || "—";
  const byModel = stats?.by_model ? Object.entries(stats.by_model) : [];
  const maxModelSpend = Math.max(1, ...byModel.map(([, v]) => v.spent_microcents ?? 0));
  const successPct = stats?.total ? Math.round((stats.success_rate ?? 0) * 100) : 0;
  const schedTotal = Number(status?.schedules?.total ?? 0);
  const schedEnabled = Number(status?.schedules?.enabled ?? 0);
  const schedRunning = Number(status?.schedules?.running ?? 0);
  const schedResidentOffline = schedEnabled > 0 && status?.schedules?.resident === false;
  const schedCenter = schedResidentOffline ? "offline" : schedRunning > 0 ? `${schedRunning} live` : `${schedEnabled}`;

  return (
    <div className="space-y-4">
      <PageHeader
        icon={Activity}
        title="Cockpit"
        description="Every key gauge of the running system at a glance."
        actions={
          <>
            <span className={cn("inline-flex items-center gap-1 text-xs font-medium", connected ? "text-good" : "text-bad")}>
              ● {connected ? "live" : "disconnected"}
            </span>
            <Button variant="ghost" size="sm" onClick={refresh} disabled={loading}>
              <RefreshCw className={cn("size-3.5", loading && "animate-spin")} /> Refresh
            </Button>
          </>
        }
      />

      {/* Needs attention (M780): the most recent warning/critical alerts surfaced on the
          landing cockpit, so the first screen tells you WHAT the agent flagged — not just
          that something happened (the nav badge). Hidden when all is well. */}
      {alerts.length > 0 && (
        <div className="rounded-lg border border-bad/40 bg-bad/5 p-3">
          <div className="mb-2 flex items-center gap-1.5 text-xs font-semibold uppercase tracking-wider text-bad">
            <ShieldAlert className="size-3.5" /> Needs attention ({alerts.length})
          </div>
          <ul className="space-y-1">
            {alerts.map((a) => {
              const Icon = a.level === "critical" ? ShieldAlert : AlertTriangle;
              const iconCls = a.level === "critical" ? "text-bad" : "text-warn";
              const inner = (
                <>
                  <Icon className={cn("size-3.5 shrink-0", iconCls)} />
                  <span className="shrink-0 font-medium text-foreground">{a.title}</span>
                  {a.source === "doctor" && (
                    <IncidentBadges
                      item={{
                        subject: a.subject,
                        phase: a.payload?.phase,
                        mode: a.payload?.mode,
                      }}
                    />
                  )}
                  {a.detail && <span className="min-w-0 flex-1 truncate text-muted">{a.detail}</span>}
                  <span className="ml-auto shrink-0 font-mono text-[10px] text-muted">{a.source}</span>
                  {incidentRootId(a) && (
                    <button
                      onClick={() => openIncident(incidentRootId(a))}
                      className="shrink-0 font-medium text-accent/80 transition-colors hover:text-accent"
                      title="Open the incident tree this alert belongs to"
                    >
                      incident →
                    </button>
                  )}
                  {a.tsMs ? <span className="w-12 shrink-0 text-right tabular-nums text-muted">{fmtTime(a.tsMs)}</span> : null}
                </>
              );
              // A run-associated alert links to its run (M781): open it in the Runs view.
              return a.correlationId ? (
                <li key={a.id}>
                  <button
                    onClick={() => { focusRun(a.correlationId!); location.hash = "runs"; }}
                    className="flex w-full items-center gap-2 rounded text-left text-xs transition-colors hover:bg-bad/10"
                    title="Open the run this alert came from"
                  >
                    {inner}
                  </button>
                </li>
              ) : (
                <li key={a.id} className="flex items-center gap-2 text-xs">{inner}</li>
              );
            })}
          </ul>
        </div>
      )}

      <ConnectivityStrip />

      {fleetOps && fleetOps.total > 0 && (
        <div className="rounded-lg border border-border bg-panel/45 p-3">
          <div className="mb-2 flex flex-wrap items-center gap-2">
            <button
              onClick={() => (location.hash = "roster")}
              className="flex items-center gap-1.5 text-xs font-semibold uppercase tracking-wider text-accent hover:underline"
              title="Open the roster identity cards"
            >
              <Bot className="size-3.5" /> Agent operations
            </button>
            <span className="text-xs text-muted">
              {fleetOps.active} active · {fleetOps.paused} paused · {fleetOps.graveyard} graveyard
            </span>
          </div>
          <div className="grid grid-cols-2 gap-2 sm:grid-cols-4 lg:grid-cols-8">
            <MiniOp icon={Bot} label="agents" value={fleetOps.total} tone="accent" />
            <MiniOp icon={Radio} label="awake" value={fleetOps.running} tone={fleetOps.running > 0 ? "accent" : "muted"} pulse={fleetOps.running > 0} />
            <MiniOp icon={Wrench} label="repair" value={fleetOps.repair} tone={fleetOps.repair > 0 ? "bad" : "muted"} />
            <MiniOp icon={Mail} label="mailbox" value={fleetOps.mailboxBacklog} tone={fleetOps.mailboxBacklog > 0 ? "warn" : "muted"} />
            <MiniOp icon={Pause} label="paused" value={fleetOps.paused} tone={fleetOps.paused > 0 ? "warn" : "muted"} />
            <MiniOp icon={Skull} label="graveyard" value={fleetOps.graveyard} tone={fleetOps.graveyard > 0 ? "muted" : "muted"} />
            <MiniOp icon={ShieldAlert} label="system" value={fleetOps.system} tone="muted" />
            <MiniOp icon={GitBranch} label="subagents" value={fleetOps.subagents} tone={fleetOps.subagents > 0 ? "accent" : "muted"} />
          </div>
          {(fleetOps.repair > 0 || fleetOps.mailboxBacklog > 0) && (
            <div className="mt-2 flex flex-wrap gap-2 text-[11px] text-muted">
              {fleetOps.repair > 0 && (
                <button onClick={() => (location.hash = "roster")} className="text-bad transition-colors hover:text-bad/80">
                  {fleetOps.repair} agent{fleetOps.repair === 1 ? "" : "s"} need repair attention
                </button>
              )}
              {fleetOps.mailboxBacklog > 0 && (
                <button onClick={() => (location.hash = "board")} className="text-warn transition-colors hover:text-warn/80">
                  {fleetOps.mailboxBacklog} mailbox message{fleetOps.mailboxBacklog === 1 ? "" : "s"} waiting across {fleetOps.mailboxAgents} agent{fleetOps.mailboxAgents === 1 ? "" : "s"}
                </button>
              )}
            </div>
          )}
        </div>
      )}

      {/* Active agents (M914): a live window into the fleet right on the cockpit —
          which lead runs are in flight now, with their sub-agent counts and spend.
          Click one to drill into the Agents monitor. Hidden when nothing is running. */}
      {active.length > 0 && (
        <div className="rounded-lg border border-accent/40 bg-accent/5 p-3">
          <button
            onClick={() => (location.hash = "agents")}
            className="mb-2 flex w-full items-center gap-1.5 text-xs font-semibold uppercase tracking-wider text-accent hover:underline"
            title="Open the Agents monitor"
          >
            <Radio className="size-3.5 animate-pulse" /> Active agents ({active.length})
          </button>
          <div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-3">
            {active.slice(0, 6).map((r) => (
              <button
                key={r.id}
                onClick={() => (location.hash = "agents")}
                className="flex flex-col gap-1.5 rounded-md border border-border bg-card p-2.5 text-left shadow-e1 transition-[box-shadow,border-color] hover:border-accent hover:shadow-e2"
              >
                <div className="flex items-center gap-1.5">
                  <span className="size-2 shrink-0 animate-pulse rounded-full bg-accent" />
                  <span className="truncate text-xs font-medium text-foreground/90" title={r.intent || r.id}>
                    {r.intent ? clip(r.intent, 80) : r.id}
                  </span>
                </div>
                <div className="flex flex-wrap items-center gap-1.5 text-[11px] text-muted">
                  <span className="inline-flex items-center gap-1" title="agents in this run's tree">
                    <Bot className="size-3" /> {r.agents}
                  </span>
                  {r.subAgents > 0 && (
                    <span className="inline-flex items-center gap-1" title="sub-agents">
                      <GitBranch className="size-3" /> {r.subAgents}
                    </span>
                  )}
                  <span className="inline-flex items-center gap-1" title="iterations">
                    <Repeat className="size-3" /> {r.iters}
                  </span>
                  <span className="inline-flex items-center gap-1" title="tree spend">
                    <Coins className="size-3" /> {money(r.treeSpentMc)}
                  </span>
                  {r.model && <span className="ml-auto truncate font-mono opacity-70" title={r.model}>{r.model}</span>}
                </div>
              </button>
            ))}
          </div>
        </div>
      )}

      {/* Gauges + live activity */}
      <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
        <GaugeCard>
          <Ring
            pct={successPct}
            center={stats?.total ? `${successPct}%` : "—"}
            label="success rate"
            tone={successPct >= 90 ? "good" : successPct >= 70 ? "warn" : "bad"}
          />
        </GaugeCard>
        <GaugeCard>
          <Ring
            pct={pctUsed}
            center={ceiling > 0 ? `${Math.round(pctUsed)}%` : money(spent)}
            label={ceiling > 0 ? "budget used" : "spent today"}
            tone={pctUsed > 85 ? "bad" : pctUsed > 60 ? "warn" : "good"}
          />
        </GaugeCard>
        <GaugeCard>
          <Ring
            pct={schedTotal > 0 ? (schedEnabled / schedTotal) * 100 : 0}
            center={schedCenter}
            label={`of ${schedTotal} schedules`}
            tone={schedResidentOffline ? "bad" : "accent"}
          />
        </GaugeCard>
        <div className="glass rounded-xl p-3">
          <div className="mb-1 flex items-center gap-1.5 text-xs font-semibold uppercase tracking-wider text-muted">
            <Gauge className="size-3.5" /> Activity
          </div>
          <Sparkline data={series} tone="accent" height={56} />
          <div className="mt-1 text-[11px] text-muted">
            {series.length >= 2 ? `${series[series.length - 1]} events/5s` : "collecting…"}
          </div>
        </div>
      </div>

      {/* Run counters */}
      <div className="grid grid-cols-2 gap-2 sm:grid-cols-4">
        <Tile icon={ListTree} label="running now" value={stats?.running ?? 0} tone="accent" pulse={(stats?.running ?? 0) > 0} />
        <Tile icon={ListTree} label="completed" value={stats?.completed ?? 0} tone="good" />
        <Tile icon={ListTree} label="failed" value={stats?.failed ?? 0} tone={(stats?.failed ?? 0) > 0 ? "bad" : "muted"} />
        <Tile icon={CalendarClock} label="active skills" value={status?.active_skills ?? 0} tone="muted" />
      </div>

      {/* Model + spend breakdown */}
      <div className="grid grid-cols-1 gap-3 lg:grid-cols-2">
        <Card title="Active model" icon={Cpu}>
          <div className="truncate text-lg font-semibold">{model}</div>
          <div className="mt-1 text-xs text-muted">avg {stats?.avg_iters ? stats.avg_iters.toFixed(1) : "—"} iters/run</div>
          <div className="mt-1 text-xs text-muted">
            {stats?.delegations ? `${stats.delegations} sub-agent delegation(s)` : "no delegations"}
          </div>
          <div className="mt-1 text-xs text-muted">{budget?.strict_pricing ? "strict pricing on" : "strict pricing off"}</div>
        </Card>

        <Card title="Spend by model" icon={Network}>
          {byModel.length === 0 ? (
            <span className="text-xs text-muted">no spend yet</span>
          ) : (
            <div className="space-y-1.5">
              {byModel
                .sort((a, b) => (b[1].spent_microcents ?? 0) - (a[1].spent_microcents ?? 0))
                .slice(0, 5)
                .map(([m, v]) => (
                  <BarRow
                    key={m}
                    label={m}
                    value={v.spent_microcents ?? 0}
                    max={maxModelSpend}
                    display={`${money(v.spent_microcents ?? 0)} · ${v.runs ?? 0}`}
                  />
                ))}
            </div>
          )}
        </Card>
      </div>

      {/* Live event ticker */}
      <div className="glass rounded-xl">
        <div className="flex items-center gap-2 border-b border-border px-3 py-2 text-xs font-semibold uppercase tracking-wider text-muted">
          <Radio className="size-3.5" /> Live events
        </div>
        <div className="max-h-64 overflow-auto">
          {events.length === 0 ? (
            <div className="px-3 py-4 text-xs text-muted">waiting for activity…</div>
          ) : (
            <ul className="divide-y divide-border/60 text-xs">
              {events.slice(0, 40).map((e, i) => (
                <li key={e.id || i} className="flex items-center gap-2 px-3 py-1.5">
                  <span className="w-16 shrink-0 tabular-nums text-muted">{fmtTime(e.ts_unix_ms)}</span>
                  <span className="w-40 shrink-0 truncate font-medium text-accent">{e.kind}</span>
                  {eventSourceLabel(e) && (
                    <span className={incidentSourceBadgeClass(eventSourceLabel(e)!)}>
                      {eventSourceLabel(e)}
                    </span>
                  )}
                  {eventPhaseLabel(e) && (
                    <span className={incidentPhaseBadgeClass(eventPhaseLabel(e)!.tone)}>
                      {eventPhaseLabel(e)!.label}
                    </span>
                  )}
                  <span className="truncate text-muted">
                    {eventSummary(e)}
                  </span>
                </li>
              ))}
            </ul>
          )}
        </div>
      </div>
    </div>
  );
}

function eventSourceLabel(e: AgentEvent): string {
  const subject = String(e.subject || "").trim();
  if (!subject) return "";
  if (subject === "doctor.auto_repair" || subject === "agent.repair" || subject === "agent.wake") {
    return doctorIncidentSourceLabel({
      subject,
      phase: (e.payload as any)?.phase,
      mode: (e.payload as any)?.mode,
    });
  }
  return "";
}

function eventPhaseLabel(
  e: AgentEvent,
): { label: string; tone: "accent" | "warn" | "good" | "bad" | "muted" } | null {
  const subject = String(e.subject || "").trim();
  if (subject === "doctor.auto_repair" || subject === "agent.repair" || subject === "agent.wake") {
    return doctorIncidentPhase({
      subject,
      phase: (e.payload as any)?.phase,
      mode: (e.payload as any)?.mode,
    });
  }
  return null;
}

function eventSummary(e: AgentEvent): string {
  const subject = String(e.subject || "").trim();
  const p: any = e.payload || {};
  if (subject === "doctor.auto_repair" || subject === "agent.repair" || subject === "agent.wake") {
    const bits = [
      String(p.agent || p.root_agent || "").trim(),
      String(p.reason || p.error || "").trim(),
      String(subject || "").trim(),
    ].filter(Boolean);
    return clip(bits.join(" · "), 140);
  }
  return String(e.subject || "").trim();
}

export function dashboardFleetOpsSummary(
  profiles: DashboardAgentProfile[],
  messages: DashboardBoardMessage[] = [],
): DashboardFleetOps {
  const mailbox = dashboardMailboxCounts(messages, profiles.map((p) => p.slug));
  let active = 0;
  let paused = 0;
  let running = 0;
  let repair = 0;
  let graveyard = 0;
  let system = 0;
  let subagents = 0;
  let mailboxAgents = 0;
  let mailboxBacklog = 0;
  for (const p of profiles) {
    if (p.retired) graveyard++;
    else if (p.enabled === false) paused++;
    else active++;
    if (p.system || p.kind === "system") system++;
    if (p.kind === "subagent" || p.managed || p.direct_callable === false) subagents++;
    if ((p.status?.active_run_count || 0) > 0 || p.status?.operational_state === "running") running++;
    if (dashboardAgentNeedsRepair(p)) repair++;
    const waiting = mailbox[p.slug.toLowerCase()] || mailbox[p.slug] || 0;
    if (!p.retired && waiting > 0) {
      mailboxAgents++;
      mailboxBacklog += waiting;
    }
  }
  return { total: profiles.length, active, paused, running, repair, graveyard, mailboxAgents, mailboxBacklog, system, subagents };
}

function dashboardAgentNeedsRepair(p: DashboardAgentProfile): boolean {
  if (p.retired) return false;
  const repairState = (p.status?.repair_state || "").toLowerCase();
  const healthState = (p.status?.health_state || "").toLowerCase();
  return (
    (p.status?.repair_inflight || 0) > 0 ||
    repairState === "queued" ||
    repairState === "failed" ||
    repairState === "attempts_exhausted" ||
    healthState === "degraded" ||
    healthState === "misconfigured" ||
    healthState === "unstable" ||
    healthState === "force_failed" ||
    healthState === "force_exhausted"
  );
}

function dashboardMailboxCounts(messages: DashboardBoardMessage[], agents: string[] = []): Record<string, number> {
  const answered = new Set(messages.filter((m) => m.reply_to).map((m) => String(m.reply_to || "").trim()).filter(Boolean));
  const broadcastReplies = new Map<string, Set<string>>();
  for (const m of messages) {
    const replyTo = String(m.reply_to || "").trim();
    const from = String(m.from || "").trim().toLowerCase();
    if (!replyTo || !from) continue;
    if (!broadcastReplies.has(replyTo)) broadcastReplies.set(replyTo, new Set());
    broadcastReplies.get(replyTo)?.add(from);
  }
  const roster = agents.map((a) => a.trim().toLowerCase()).filter(Boolean);
  const counts: Record<string, number> = {};
  for (const m of messages) {
    const id = String(m.id || "").trim();
    if (!id || m.reply_to) continue;
    const to = String(m.to || "").trim().toLowerCase();
    const from = String(m.from || "").trim().toLowerCase();
    const acked = new Set((m.acked_by || []).map((a) => a.trim().toLowerCase()).filter(Boolean));
    if (to && to !== "*" && !answered.has(id) && !acked.has(to)) counts[to] = (counts[to] || 0) + 1;
    if (to === "*") {
      const replied = broadcastReplies.get(id) || new Set<string>();
      for (const agent of roster) {
        if (!agent || agent === from || acked.has(agent) || replied.has(agent)) continue;
        counts[agent] = (counts[agent] || 0) + 1;
      }
    }
  }
  return counts;
}

function GaugeCard({ children }: { children: React.ReactNode }) {
  return <div className="flex items-center justify-center glass rounded-xl p-3 shadow-e1">{children}</div>;
}

function MiniOp({
  icon: Icon,
  label,
  value,
  tone,
  pulse,
}: {
  icon: typeof Activity;
  label: string;
  value: number | string;
  tone: "accent" | "warn" | "bad" | "muted";
  pulse?: boolean;
}) {
  const color = { accent: "text-accent", warn: "text-warn", bad: "text-bad", muted: "text-foreground" }[tone];
  return (
    <div className="rounded-md border border-border bg-card/55 px-2 py-1.5">
      <div className="flex items-center gap-1 text-[10px] uppercase tracking-wider text-muted">
        <Icon className={cn("size-3", pulse && "animate-pulse")} /> {label}
      </div>
      <div className={cn("mt-0.5 text-lg font-semibold tabular-nums", color)}>{value}</div>
    </div>
  );
}

function Tile({
  icon: Icon,
  label,
  value,
  tone,
  pulse,
}: {
  icon: typeof Activity;
  label: string;
  value: number | string;
  tone: "accent" | "good" | "bad" | "muted";
  pulse?: boolean;
}) {
  const color = { accent: "text-accent", good: "text-good", bad: "text-bad", muted: "text-foreground" }[tone];
  return (
    <div className="glass rounded-xl px-3 py-2.5 shadow-e1">
      <div className="flex items-center gap-1.5 text-xs text-muted">
        <Icon className="size-3.5" /> {label}
        {pulse && <span className="ml-auto size-2 animate-pulse rounded-full bg-accent" />}
      </div>
      <div className={cn("mt-1 text-2xl font-semibold tabular-nums", color)}>{value}</div>
    </div>
  );
}

function Card({ title, icon: Icon, children }: { title: string; icon: typeof Activity; children: React.ReactNode }) {
  return (
    <div className="glass rounded-xl p-3 shadow-e1">
      <div className="mb-2 flex items-center gap-1.5 text-xs font-semibold uppercase tracking-wider text-muted">
        <Icon className="size-3.5" /> {title}
      </div>
      {children}
    </div>
  );
}
