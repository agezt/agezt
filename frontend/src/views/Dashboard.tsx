import { useEffect, useRef, useState } from "react";
import {
  Activity,
  RefreshCw,
  Cpu,
  Wallet,
  Network,
  Radio,
  CalendarClock,
  Gauge,
  ShieldAlert,
  AlertTriangle,
  Bot,
  GitBranch,
  Coins,
  Repeat,
  Mail,
  Wrench,
  Skull,
  Pause,
  XCircle,
  Zap,
  CheckCircle2,
  XOctagon,
  Clock,
} from "lucide-react";
import { cn } from "@/lib/utils";
import { money } from "@/lib/format";
import { getJSON } from "@/lib/api";
import { ConnectivityStrip } from "@/views/Connections";
import { Advanced, Calm } from "@/components/ui/advanced";
import { useEvents, type AgentEvent } from "@/lib/events";
import { recentAttentionAlerts, type RankedAlert } from "@/lib/alerts";
import { incidentRootId } from "@/lib/incidents";
import { focusRun } from "@/lib/runfocus";
import { openIncident } from "@/lib/incidentnav";
import { doctorIncidentPhase, doctorIncidentSourceLabel } from "@/lib/autonomy";
import {
  IncidentBadges,
} from "@/components/IncidentBadges";
import { Button } from "@/components/ui/button";
import { fmtTime, clip } from "@/lib/utils";
import { Ring, Sparkline, BarRow } from "@/components/Widgets";
import { PageHeader } from "@/components/ui/page-header";
import { summarizeRoots, type RootSummary } from "@/views/Agents";
import { TabNav } from "@/components/ui/tab-nav";
import { MetricWidget, MetricGrid } from "@/components/ui/metric-widget";
import { JarvisPresenceCard } from "@/components/JarvisPresenceCard";
import { CollapsibleSection } from "@/components/ui/collapsible-section";

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
    if (r.status === "fulfilled")
      setActive(summarizeRoots(r.value.runs || []).filter((x) => x.kind === "running"));
    if (j.status === "fulfilled")
      setAlerts(recentAttentionAlerts(j.value.events || [], { limit: 4, nowMs: Date.now() }));
    if (a.status === "fulfilled") {
      const messages = bd.status === "fulfilled" ? bd.value.messages || [] : [];
      setFleetOps(dashboardFleetOpsSummary(a.value.profiles || [], messages));
    }
    if (st.status === "fulfilled") {
      setStatus(st.value);
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

  // Build the three tabs
  const tabs = [
    {
      id: "overview",
      label: "Overview",
      icon: Gauge,
      content: (
        <OverviewTab
          stats={stats}
          fleetOps={fleetOps}
          active={active}
          alerts={alerts}
          successPct={successPct}
          pctUsed={pctUsed}
          ceiling={ceiling}
          spent={spent}
          schedTotal={schedTotal}
          schedEnabled={schedEnabled}
          schedRunning={schedRunning}
          schedResidentOffline={schedResidentOffline}
          series={series}
          status={status}
        />
      ),
    },
    {
      id: "activity",
      label: "Activity",
      icon: Zap,
      count: events.length > 0 ? events.length : undefined,
      content: (
        <ActivityTab
          events={events}
          series={series}
          loading={loading}
          refresh={refresh}
        />
      ),
    },
    {
      id: "budget",
      label: "Budget",
      icon: Wallet,
      content: (
        <BudgetTab
          stats={stats}
          budget={budget}
          byModel={byModel}
          maxModelSpend={maxModelSpend}
          model={model}
        />
      ),
    },
  ];

  return (
    <div className="flex flex-col gap-4">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-3">
          <span className="grid size-9 shrink-0 place-items-center rounded-xl bg-gradient-to-br from-accent/25 to-accent2/20 text-accent ring-1 ring-inset ring-accent/30">
            <Activity className="size-5" />
          </span>
          <div>
            <h2 className="text-gradient text-base font-bold leading-tight tracking-tight">Cockpit</h2>
            <span className={cn(
              "inline-flex items-center gap-1 text-xs font-medium",
              connected ? "text-good" : "text-bad",
            )}>
              ● {connected ? "live" : "disconnected"}
            </span>
          </div>
        </div>
        <Button variant="ghost" size="sm" onClick={refresh} disabled={loading}>
          <RefreshCw className={cn("size-3.5", loading && "animate-spin")} /> Refresh
        </Button>
      </div>

      <TabNav tabs={tabs} />
    </div>
  );
}

// ───────────────────────── Overview tab ─────────────────────────

function OverviewTab({
  stats,
  fleetOps,
  active,
  alerts,
  successPct,
  pctUsed,
  ceiling,
  spent,
  schedTotal,
  schedEnabled,
  schedRunning,
  schedResidentOffline,
  series,
  status,
}: {
  stats: Stats | null;
  fleetOps: DashboardFleetOps | null;
  active: RootSummary[];
  alerts: RankedAlert[];
  successPct: number;
  pctUsed: number;
  ceiling: number;
  spent: number;
  schedTotal: number;
  schedEnabled: number;
  schedRunning: number;
  schedResidentOffline: boolean;
  series: number[];
  status: Record<string, any> | null;
}) {
  return (
    <div className="flex flex-col gap-3">
      {/* Alerts — always visible when present */}
      {alerts.length > 0 && (
        <CollapsibleSection
          icon={ShieldAlert}
          title="Needs attention"
          count={alerts.length}
          tone="bad"
          defaultOpen={true}
        >
          <ul className="space-y-1">
            {alerts.map((a) => {
              const Icon = a.level === "critical" ? ShieldAlert : AlertTriangle;
              const iconCls = a.level === "critical" ? "text-bad" : "text-warn";
              return (
                <li key={a.id} className="flex items-center gap-2 text-sm">
                  <Icon className={cn("size-3.5 shrink-0", iconCls)} />
                  <span className="shrink-0 font-medium">{a.title}</span>
                  {a.source === "doctor" && (
                    <IncidentBadges
                      item={{
                        subject: a.subject,
                        phase: (a.payload as any)?.phase,
                        mode: (a.payload as any)?.mode,
                      }}
                    />
                  )}
                  {a.detail && (
                    <span className="min-w-0 flex-1 truncate text-muted">{a.detail}</span>
                  )}
                  {incidentRootId(a) && (
                    <button
                      onClick={() => openIncident(incidentRootId(a))}
                      className="shrink-0 text-xs font-medium text-accent transition-colors hover:text-accent/80"
                    >
                      incident →
                    </button>
                  )}
                </li>
              );
            })}
          </ul>
        </CollapsibleSection>
      )}

      {/* Jarvis presence — discoverable entry point to the triad (M1002). */}
      <JarvisPresenceCard />

      {/* Key metrics grid */}
      <MetricGrid>
        <MetricWidget
          icon={CheckCircle2}
          label="Success rate"
          value={`${successPct}%`}
          tone={successPct >= 90 ? "good" : successPct >= 70 ? "warn" : "bad"}
          trend={[]}
        />
        <MetricWidget
          icon={Wallet}
          label="Budget used"
          value={ceiling > 0 ? `${Math.round(pctUsed)}%` : money(spent)}
          subvalue={ceiling > 0 ? `of ${money(ceiling)}` : undefined}
          tone={pctUsed > 85 ? "bad" : pctUsed > 60 ? "warn" : "good"}
        />
        <MetricWidget
          icon={CalendarClock}
          label="Schedules"
          value={schedResidentOffline ? "offline" : `${schedRunning} live`}
          subvalue={`${schedEnabled} of ${schedTotal} enabled`}
          tone={schedResidentOffline ? "bad" : schedRunning > 0 ? "accent" : "muted"}
          pulse={schedRunning > 0}
        />
        <MetricWidget
          icon={Zap}
          label="Activity"
          value={series.at(-1) ?? 0}
          subvalue="events/5s"
          tone="accent"
          trend={series}
          pulse={true}
        />
      </MetricGrid>

      {/* Fleet ops — compact strip */}
      {fleetOps && fleetOps.total > 0 && (
        <CollapsibleSection
          icon={Bot}
          title="Agent operations"
          count={`${fleetOps.running} awake`}
          tone="accent"
          defaultOpen={true}
          actions={
            <button
              onClick={() => (location.hash = "roster")}
              className="text-xs text-accent hover:underline"
            >
              roster →
            </button>
          }
        >
          <div className="grid grid-cols-4 gap-2 sm:grid-cols-8">
            <MiniMetric icon={Bot} label="agents" value={fleetOps.total} />
            <MiniMetric icon={Radio} label="awake" value={fleetOps.running} tone={fleetOps.running > 0 ? "accent" : "muted"} pulse={fleetOps.running > 0} />
            <MiniMetric icon={Wrench} label="repair" value={fleetOps.repair} tone={fleetOps.repair > 0 ? "bad" : "muted"} />
            <MiniMetric icon={Mail} label="mailbox" value={fleetOps.mailboxBacklog} tone={fleetOps.mailboxBacklog > 0 ? "warn" : "muted"} />
            <MiniMetric icon={Pause} label="paused" value={fleetOps.paused} tone={fleetOps.paused > 0 ? "warn" : "muted"} />
            <MiniMetric icon={Skull} label="graveyard" value={fleetOps.graveyard} />
            <MiniMetric icon={ShieldAlert} label="system" value={fleetOps.system} />
            <MiniMetric icon={GitBranch} label="subagents" value={fleetOps.subagents} tone={fleetOps.subagents > 0 ? "accent" : "muted"} />
          </div>
          {(fleetOps.repair > 0 || fleetOps.mailboxBacklog > 0) && (
            <div className="mt-2 flex flex-wrap gap-2 text-xs">
              {fleetOps.repair > 0 && (
                <button onClick={() => (location.hash = "roster")} className="text-bad hover:text-bad/80">
                  {fleetOps.repair} need repair
                </button>
              )}
              {fleetOps.mailboxBacklog > 0 && (
                <button onClick={() => (location.hash = "board")} className="text-warn hover:text-warn/80">
                  {fleetOps.mailboxBacklog} messages waiting
                </button>
              )}
            </div>
          )}
        </CollapsibleSection>
      )}

      {/* Active runs */}
      {active.length > 0 && (
        <CollapsibleSection
          icon={Repeat}
          title="Active runs"
          count={active.length}
          tone="accent"
          defaultOpen={active.length <= 3}
        >
          <div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-3">
            {active.slice(0, 6).map((r) => (
              <button
                key={r.id}
                onClick={() => (location.hash = "agents")}
                className="flex flex-col gap-2 rounded-lg border border-border bg-panel/60 p-3 text-left transition-colors hover:border-accent hover:bg-panel"
              >
                <div className="flex items-center gap-2">
                  <span className="size-2 rounded-full bg-accent animate-pulse" />
                  <span className="truncate text-sm font-medium" title={r.intent || r.id}>
                    {r.intent ? clip(r.intent, 60) : r.id}
                  </span>
                </div>
                <div className="flex flex-wrap items-center gap-2 text-xs text-muted">
                  <span className="inline-flex items-center gap-1"><Bot className="size-3" /> {r.agents}</span>
                  {r.subAgents > 0 && <span className="inline-flex items-center gap-1"><GitBranch className="size-3" /> {r.subAgents}</span>}
                  <span className="inline-flex items-center gap-1"><Coins className="size-3" /> {money(r.treeSpentMc)}</span>
                  {r.model && <span className="ml-auto truncate font-mono opacity-70" title={r.model}>{r.model}</span>}
                </div>
              </button>
            ))}
          </div>
        </CollapsibleSection>
      )}

      {/* Run counters */}
      <MetricGrid cols="repeat(auto-fill, minmax(140px, 1fr))">
        <MetricWidget icon={Repeat} label="Running" value={stats?.running ?? 0} tone="accent" pulse={(stats?.running ?? 0) > 0} />
        <MetricWidget icon={CheckCircle2} label="Completed" value={stats?.completed ?? 0} tone="good" />
        <MetricWidget icon={XOctagon} label="Failed" value={stats?.failed ?? 0} tone={(stats?.failed ?? 0) > 0 ? "bad" : "muted"} />
        <MetricWidget icon={GitBranch} label="Delegations" value={stats?.delegations ?? 0} tone={(stats?.delegations ?? 0) > 0 ? "accent" : "muted"} />
        <MetricWidget icon={CalendarClock} label="Active skills" value={status?.active_skills ?? 0} />
      </MetricGrid>
    </div>
  );
}

// ───────────────────────── Activity tab ─────────────────────────

function ActivityTab({
  events,
  series,
  loading,
  refresh,
}: {
  events: AgentEvent[];
  series: number[];
  loading: boolean;
  refresh: () => void;
}) {
  return (
    <div className="flex flex-col gap-3">
      <div className="flex items-center gap-3">
        <CollapsibleSection
          icon={Gauge}
          title="Activity pulse"
          tone="accent"
          defaultOpen={true}
        >
          <Sparkline data={series} tone="accent" height={48} />
          <p className="mt-1 text-xs text-muted">
            {series.length >= 2 ? `${series[series.length - 1]} events/5s` : "collecting…"}
          </p>
        </CollapsibleSection>
      </div>

      <Advanced>
        <div className="rounded-xl border border-border bg-card shadow-e1">
          <div className="flex items-center justify-between border-b border-border px-3 py-2">
            <div className="inline-flex items-center gap-1.5 text-xs font-semibold text-muted">
              <Radio className="size-3.5" /> Live stream
            </div>
            <Button variant="ghost" size="sm" onClick={refresh} disabled={loading}>
              <RefreshCw className={cn("size-3.5", loading && "animate-spin")} />
            </Button>
          </div>
          <div className="max-h-80 overflow-auto">
            {events.length === 0 ? (
              <div className="px-3 py-4 text-xs text-muted">waiting for activity…</div>
            ) : (
              <ul className="divide-y divide-border/60 text-xs">
                {events.slice(0, 40).map((e, i) => (
                  <li key={e.id || i} className="flex items-center gap-2 px-3 py-1.5">
                    <span className="w-16 shrink-0 tabular-nums text-muted">{fmtTime(e.ts_unix_ms)}</span>
                    <span className="w-36 shrink-0 truncate font-medium text-accent">{e.kind}</span>
                    <span className="min-w-0 flex-1 truncate text-muted">{eventSummary(e)}</span>
                  </li>
                ))}
              </ul>
            )}
          </div>
        </div>
      </Advanced>
    </div>
  );
}

// ───────────────────────── Budget tab ─────────────────────────

function BudgetTab({
  stats,
  budget,
  byModel,
  maxModelSpend,
  model,
}: {
  stats: Stats | null;
  budget: Budget | null;
  byModel: [string, { runs?: number; spent_microcents?: number }][];
  maxModelSpend: number;
  model: string;
}) {
  return (
    <div className="flex flex-col gap-3">
      <MetricGrid>
        <MetricWidget
          icon={Cpu}
          label="Active model"
          value={model}
          subvalue={stats?.avg_iters ? `avg ${stats.avg_iters.toFixed(1)} iters/run` : undefined}
          tone="accent"
        />
        <MetricWidget
          icon={Coins}
          label="Total spend"
          value={money(stats?.spent_microcents ?? 0)}
          tone="warn"
        />
        <MetricWidget
          icon={Repeat}
          label="Total runs"
          value={stats?.total ?? 0}
          tone="muted"
        />
      </MetricGrid>

      {byModel.length > 0 && (
        <CollapsibleSection
          icon={Network}
          title="Spend by model"
          tone="accent"
          defaultOpen={true}
        >
          <div className="space-y-2">
            {byModel
              .sort((a, b) => (b[1].spent_microcents ?? 0) - (a[1].spent_microcents ?? 0))
              .slice(0, 8)
              .map(([m, v]) => (
                <BarRow
                  key={m}
                  label={m}
                  value={v.spent_microcents ?? 0}
                  max={maxModelSpend}
                  display={`${money(v.spent_microcents ?? 0)} · ${v.runs ?? 0} runs`}
                />
              ))}
          </div>
        </CollapsibleSection>
      )}

      <CollapsibleSection
        icon={Wallet}
        title="Budget settings"
        tone="muted"
        defaultOpen={false}
      >
        <div className="space-y-1 text-sm text-muted">
          <p>Ceiling: {money(budget?.ceiling_mc ?? 0)}</p>
          <p>Strict pricing: {budget?.strict_pricing ? "on" : "off"}</p>
        </div>
      </CollapsibleSection>
    </div>
  );
}

// ───────────────────────── Shared helpers ─────────────────────────

function MiniMetric({
  icon: Icon,
  label,
  value,
  tone = "muted",
  pulse,
}: {
  icon: typeof Activity;
  label: string;
  value: number;
  tone?: "accent" | "warn" | "bad" | "muted";
  pulse?: boolean;
}) {
  const colorCls = { accent: "text-accent", warn: "text-warn", bad: "text-bad", muted: "text-foreground" }[tone];
  return (
    <div className="flex flex-col gap-0.5 rounded-lg border border-border bg-panel/60 px-2 py-1.5">
      <div className="inline-flex items-center gap-1 text-[10px] text-muted">
        <Icon className={cn("size-2.5", pulse && "animate-pulse")} />
        {label}
      </div>
      <div className={cn("text-base font-semibold tabular-nums", colorCls)}>{value}</div>
    </div>
  );
}

function eventSummary(e: AgentEvent): string {
  const subject = String(e.subject || "").trim();
  const p: any = e.payload || {};
  if (subject === "doctor.auto_repair" || subject === "agent.repair" || subject === "agent.wake") {
    const bits = [
      String(p.agent || p.root_agent || "").trim(),
      String(p.reason || p.error || "").trim(),
    ].filter(Boolean);
    return clip(bits.join(" · "), 100);
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
