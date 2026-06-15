import { useEffect, useRef, useState } from "react";
import { Activity, RefreshCw, Cpu, Wallet, ListTree, Network, Radio, CalendarClock, Gauge, ShieldAlert, AlertTriangle } from "lucide-react";
import { cn } from "@/lib/utils";
import { money } from "@/lib/format";
import { getJSON } from "@/lib/api";
import { useEvents, type AgentEvent } from "@/lib/events";
import { recentAttentionAlerts, type RankedAlert } from "@/lib/alerts";
import { focusRun } from "@/lib/runfocus";
import { Button } from "@/components/ui/button";
import { fmtTime, clip } from "@/lib/utils";
import { Ring, Sparkline, BarRow } from "@/components/Widgets";
import { PageHeader } from "@/components/ui/page-header";
import { summarizeRoots, type RootSummary } from "@/views/Agents";
import { Bot, GitBranch, Coins, Repeat } from "lucide-react";

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
  const [loading, setLoading] = useState(false);
  const lastHead = useRef<number | null>(null);

  async function refresh() {
    setLoading(true);
    const [s, b, st, j, r] = await Promise.allSettled([
      getJSON<Stats>("/api/stats"),
      getJSON<Budget>("/api/budget"),
      getJSON<Record<string, any>>("/api/status"),
      getJSON<{ events?: AgentEvent[] }>("/api/journal", { limit: "300" }),
      getJSON<{ runs?: RunRow[] }>("/api/runs"),
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
    if (head === "task.received" || head === "task.completed" || head === "task.failed") refresh();
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
                  {a.detail && <span className="min-w-0 flex-1 truncate text-muted">{a.detail}</span>}
                  <span className="ml-auto shrink-0 font-mono text-[10px] text-muted">{a.source}</span>
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
            center={`${schedEnabled}`}
            label={`of ${schedTotal} schedules`}
            tone="accent"
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
                  <span className="truncate text-muted">{e.subject}</span>
                </li>
              ))}
            </ul>
          )}
        </div>
      </div>
    </div>
  );
}

function GaugeCard({ children }: { children: React.ReactNode }) {
  return <div className="flex items-center justify-center glass rounded-xl p-3 shadow-e1">{children}</div>;
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
