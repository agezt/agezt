import { useEffect, useState } from "react";
import { Activity, RefreshCw, Cpu, Wallet, ListTree, Network, Radio } from "lucide-react";
import { cn } from "@/lib/utils";
import { money } from "@/lib/format";
import { getJSON } from "@/lib/api";
import { useEvents } from "@/lib/events";
import { Button } from "@/components/ui/button";
import { fmtTime } from "@/lib/utils";

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

// Dashboard is the cockpit: every key gauge of the running system at a glance —
// what's running now, today's throughput and success rate, spend against the
// ceiling, the active model, per-model cost, and a live event ticker — refreshed
// on a timer and nudged by the live event stream. One screen to see, understand,
// and monitor the whole daemon.
export function Dashboard() {
  const { events, connected } = useEvents();
  const [stats, setStats] = useState<Stats | null>(null);
  const [budget, setBudget] = useState<Budget | null>(null);
  const [status, setStatus] = useState<Record<string, any> | null>(null);
  const [loading, setLoading] = useState(false);

  async function refresh() {
    setLoading(true);
    const [s, b, st] = await Promise.allSettled([
      getJSON<Stats>("/api/stats"),
      getJSON<Budget>("/api/budget"),
      getJSON<Record<string, any>>("/api/status"),
    ]);
    if (s.status === "fulfilled") setStats(s.value);
    if (b.status === "fulfilled") setBudget(b.value);
    if (st.status === "fulfilled") setStatus(st.value);
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

  return (
    <div className="space-y-4">
      <div className="flex items-center gap-3">
        <h2 className="flex items-center gap-2 text-sm font-semibold">
          <Activity className="size-4 text-accent" /> Cockpit
        </h2>
        <span className={cn("inline-flex items-center gap-1 text-xs", connected ? "text-good" : "text-bad")}>
          ● {connected ? "live" : "disconnected"}
        </span>
        <Button variant="ghost" size="sm" onClick={refresh} disabled={loading} className="ml-auto">
          <RefreshCw className={cn("size-3.5", loading && "animate-spin")} /> Refresh
        </Button>
      </div>

      {/* Run gauges */}
      <div className="grid grid-cols-2 gap-2 sm:grid-cols-4">
        <Tile icon={ListTree} label="running now" value={stats?.running ?? 0} tone="accent" pulse={(stats?.running ?? 0) > 0} />
        <Tile icon={ListTree} label="completed" value={stats?.completed ?? 0} tone="good" />
        <Tile icon={ListTree} label="failed" value={stats?.failed ?? 0} tone={(stats?.failed ?? 0) > 0 ? "bad" : "muted"} />
        <Tile
          icon={Activity}
          label="success rate"
          value={stats?.total ? `${Math.round((stats.success_rate ?? 0) * 100)}%` : "—"}
          tone="muted"
        />
      </div>

      {/* Budget + model + throughput */}
      <div className="grid grid-cols-1 gap-3 lg:grid-cols-3">
        <Card title="Budget (today)" icon={Wallet}>
          <div className="flex items-baseline gap-2">
            <span className="text-2xl font-semibold tabular-nums">{money(spent)}</span>
            <span className="text-xs text-muted">{ceiling > 0 ? `of ${money(ceiling)} ceiling` : "no ceiling"}</span>
          </div>
          {ceiling > 0 && (
            <div className="mt-2 h-2 overflow-hidden rounded-full bg-panel">
              <div
                className={cn("h-full rounded-full", pctUsed > 85 ? "bg-bad" : pctUsed > 60 ? "bg-accent" : "bg-good")}
                style={{ width: `${pctUsed}%` }}
              />
            </div>
          )}
          <div className="mt-1.5 text-xs text-muted">
            {budget?.strict_pricing ? "strict pricing on" : "strict pricing off"}
          </div>
        </Card>

        <Card title="Active model" icon={Cpu}>
          <div className="truncate text-lg font-semibold">{model}</div>
          <div className="mt-1 text-xs text-muted">avg {stats?.avg_iters ? stats.avg_iters.toFixed(1) : "—"} iters/run</div>
          <div className="mt-1 text-xs text-muted">
            {stats?.delegations ? `${stats.delegations} sub-agent delegation(s)` : "no delegations"}
          </div>
        </Card>

        <Card title="Spend by model" icon={Network}>
          {byModel.length === 0 ? (
            <span className="text-xs text-muted">no spend yet</span>
          ) : (
            <ul className="space-y-1 text-xs">
              {byModel
                .sort((a, b) => (b[1].spent_microcents ?? 0) - (a[1].spent_microcents ?? 0))
                .slice(0, 5)
                .map(([m, v]) => (
                  <li key={m} className="flex items-center justify-between gap-2">
                    <span className="truncate text-muted">{m}</span>
                    <span className="shrink-0 tabular-nums">
                      {money(v.spent_microcents ?? 0)} · {v.runs ?? 0} run{(v.runs ?? 0) === 1 ? "" : "s"}
                    </span>
                  </li>
                ))}
            </ul>
          )}
        </Card>
      </div>

      {/* Live event ticker */}
      <div className="rounded-lg border border-border bg-card">
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
    <div className="rounded-lg border border-border bg-card px-3 py-2.5">
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
    <div className="rounded-lg border border-border bg-card p-3">
      <div className="mb-2 flex items-center gap-1.5 text-xs font-semibold uppercase tracking-wider text-muted">
        <Icon className="size-3.5" /> {title}
      </div>
      {children}
    </div>
  );
}
