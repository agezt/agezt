import { useEffect, useState, type ReactNode } from "react";
import {
  RefreshCw,
  Activity,
  ListTree,
  CheckSquare,
  Database,
  Network,
  Brain,
  Sparkles,
  CalendarClock,
  Server,
  KeyRound,
  Cpu,
  Clock,
  ShieldAlert,
} from "lucide-react";
import { getJSON } from "@/lib/api";
import { useEvents } from "@/lib/events";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { ErrorText } from "@/components/JsonView";
import { SkeletonList } from "@/components/ui/skeleton";
import { Advanced, Calm } from "@/components/ui/advanced";
import { TabNav } from "@/components/ui/tab-nav";
import { MetricWidget, MetricGrid } from "@/components/ui/metric-widget";
import { Badge } from "@/components/ui/badge";

interface StatusData {
  daemon?: string;
  protocol?: string;
  model?: string;
  halted?: boolean;
  uptime_seconds?: number;
  active_runs?: number;
  pending_approvals?: number;
  journal_head?: number;
  memory_records?: number;
  world_entities?: number;
  active_skills?: number;
  tools?: number;
  schedules?: { enabled?: number; total?: number; running?: number; resident?: boolean };
  delegation?: { enabled?: boolean; max_depth?: number; max_fanout?: number; max_spend_microcents?: number };
  http_servers?: { name?: string; addr?: string; loopback?: boolean }[];
  cred_chain?: string;
  provider_fallbacks?: { count?: number; last_reason?: string };
}

function uptime(s: number): string {
  if (!s || s < 0) return "—";
  const d = Math.floor(s / 86400);
  const h = Math.floor((s % 86400) / 3600);
  const m = Math.floor((s % 3600) / 60);
  const sec = Math.floor(s % 60);
  if (d) return `${d}d ${h}h ${m}m`;
  if (h) return `${h}h ${m}m`;
  if (m) return `${m}m ${sec}s`;
  return `${sec}s`;
}

// Status is the system health dashboard: the daemon's vitals at a glance —
// operational state, uptime, model, and the live counters (runs, approvals,
// journal, memory, world, skills, schedules) — plus delegation limits, the HTTP
// surface, credentials, and the most recent provider fallback.
export function Status() {
  const { connected, events } = useEvents();
  const [data, setData] = useState<StatusData | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);

  async function reload() {
    setLoading(true);
    try {
      setData(await getJSON<StatusData>("/api/status"));
      setErr(null);
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setLoading(false);
    }
  }
  useEffect(() => {
    reload();
    const id = setInterval(reload, 5000);
    return () => clearInterval(id);
  }, []);

  const head = events[0]?.kind;
  useEffect(() => {
    if (head === "task.received" || head === "task.completed" || head === "schedule.fired" || head === "kernel.halt" || head === "kernel.resume")
      reload();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [head, events[0]?.id]);

  const halted = data?.halted;
  const scheduleRunning = data?.schedules?.running ?? 0;
  const scheduleEnabled = data?.schedules?.enabled ?? 0;
  const scheduleTotal = data?.schedules?.total ?? 0;
  const scheduleResidentOffline = scheduleEnabled > 0 && data?.schedules?.resident === false;
  const scheduleValue = scheduleResidentOffline
    ? "offline"
    : scheduleRunning > 0
      ? `${scheduleRunning} live`
      : `${scheduleEnabled}/${scheduleTotal}`;

  const tabs = [
    {
      id: "overview",
      label: "Overview",
      icon: Activity,
      content: (
        <div className="flex flex-col gap-3">
          {err ? (
            <ErrorText>{err}</ErrorText>
          ) : !data ? (
            <SkeletonList count={3} lines={2} />
          ) : (
            <>
              {/* Status strip */}
              <MetricGrid>
                <MetricWidget
                  icon={Activity}
                  label={halted ? "halted" : "operational"}
                  value={halted ? "Halted" : "Up"}
                  tone={halted ? "bad" : "good"}
                />
                <MetricWidget
                  icon={Cpu}
                  label="model"
                  value={data.model || "—"}
                  tone="accent"
                />
                <MetricWidget
                  icon={Clock}
                  label="uptime"
                  value={uptime(data.uptime_seconds || 0)}
                  tone="muted"
                />
                <MetricWidget
                  icon={Server}
                  label="daemon"
                  value={`v${data.daemon || "?"}`}
                  tone="muted"
                />
              </MetricGrid>

              {/* Live counters */}
              <MetricGrid>
                <MetricWidget
                  icon={ListTree}
                  label="active runs"
                  value={data.active_runs ?? 0}
                  tone={(data.active_runs ?? 0) > 0 ? "accent" : "muted"}
                  pulse={(data.active_runs ?? 0) > 0}
                />
                <MetricWidget
                  icon={CheckSquare}
                  label="pending approvals"
                  value={data.pending_approvals ?? 0}
                  tone={(data.pending_approvals ?? 0) > 0 ? "bad" : "muted"}
                />
                <MetricWidget
                  icon={Database}
                  label="journal head"
                  value={(data.journal_head ?? 0).toLocaleString()}
                  tone="muted"
                />
                <MetricWidget
                  icon={Brain}
                  label="memory records"
                  value={data.memory_records ?? 0}
                  tone="muted"
                />
                <MetricWidget
                  icon={Network}
                  label="world entities"
                  value={data.world_entities ?? 0}
                  tone="muted"
                />
                <MetricWidget
                  icon={Sparkles}
                  label="active skills"
                  value={data.active_skills ?? 0}
                  tone="muted"
                />
                <MetricWidget
                  icon={CalendarClock}
                  label="schedules"
                  value={scheduleValue}
                  tone={scheduleResidentOffline ? "bad" : scheduleRunning > 0 ? "accent" : "muted"}
                  pulse={scheduleRunning > 0}
                />
                <MetricWidget
                  icon={Cpu}
                  label="tools"
                  value={data.tools ?? 0}
                  tone="muted"
                />
              </MetricGrid>

              <Calm>
                <p className="text-xs text-muted">Advanced mode for delegation, HTTP, credentials, and routing detail.</p>
              </Calm>
            </>
          )}
        </div>
      ),
    },
    {
      id: "advanced",
      label: "Advanced",
      icon: ShieldAlert,
      content: (
        <Advanced>
          {err ? (
            <ErrorText>{err}</ErrorText>
          ) : !data ? (
            <SkeletonList count={3} lines={2} />
          ) : (
            <div className="grid grid-cols-1 gap-3 lg:grid-cols-2">
              <StatusPanel
                icon={Network}
                title="Delegation"
                status={data.delegation?.enabled ? "enabled" : "off"}
                tone={data.delegation?.enabled ? "accent" : "muted"}
              >
                {data.delegation ? (
                  <ul className="space-y-1 text-xs">
                    <Row k="enabled" v={data.delegation.enabled ? "yes" : "no"} />
                    <Row k="max depth" v={data.delegation.max_depth ?? "—"} />
                    <Row k="max fan-out" v={data.delegation.max_fanout ? data.delegation.max_fanout : "unbounded"} />
                    <Row
                      k="max spend"
                      v={data.delegation.max_spend_microcents ? `${(data.delegation.max_spend_microcents / 1e9).toFixed(4)}` : "uncapped"}
                    />
                  </ul>
                ) : (
                  <EmptyDash />
                )}
              </StatusPanel>

              <StatusPanel
                icon={Server}
                title="HTTP surface"
                status={`${data.http_servers?.length ?? 0} server${(data.http_servers?.length ?? 0) === 1 ? "" : "s"}`}
                tone={(data.http_servers?.length ?? 0) > 0 ? "accent" : "muted"}
              >
                {data.http_servers?.length ? (
                  <ul className="space-y-1 text-xs">
                    {data.http_servers.map((s, i) => (
                      <li key={i} className="flex items-center gap-2">
                        <span className="font-mono">{s.addr}</span>
                        <span className="text-muted">{s.name}</span>
                        {s.loopback && (
                          <Badge variant="good" className="ml-auto">loopback</Badge>
                        )}
                      </li>
                    ))}
                  </ul>
                ) : (
                  <EmptyDash label="none" />
                )}
              </StatusPanel>

              <StatusPanel
                icon={KeyRound}
                title="Credentials"
                status={data.cred_chain ? "chain loaded" : "not reported"}
                tone={data.cred_chain ? "good" : "muted"}
              >
                <div className="break-words font-mono text-xs text-muted">{data.cred_chain || "—"}</div>
              </StatusPanel>

              <StatusPanel
                icon={ShieldAlert}
                title="Provider routing"
                status={`${data.provider_fallbacks?.count ?? 0} fallback${(data.provider_fallbacks?.count ?? 0) === 1 ? "" : "s"}`}
                tone={(data.provider_fallbacks?.count ?? 0) > 0 ? "warn" : "good"}
              >
                <div className="text-xs">
                  <Row k="fallbacks" v={data.provider_fallbacks?.count ?? 0} />
                  {data.provider_fallbacks?.last_reason && (
                    <div className="mt-1.5 rounded-md border border-warn/40 bg-warn/5 p-2 text-[11px] text-muted">
                      <span className="text-warn">last fallback: </span>
                      {data.provider_fallbacks.last_reason.slice(0, 200)}
                    </div>
                  )}
                </div>
              </StatusPanel>
            </div>
          )}
        </Advanced>
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
            <h2 className="text-gradient text-base font-bold leading-tight tracking-normal">System health</h2>
            <span className={cn(
              "inline-flex items-center gap-1 text-xs font-medium",
              connected ? "text-good" : "text-bad",
            )}>
              ● {connected ? "live" : "disconnected"}
            </span>
          </div>
        </div>
        <Button variant="ghost" size="sm" onClick={reload} disabled={loading}>
          <RefreshCw className={cn("size-3.5", loading && "animate-spin")} /> Refresh
        </Button>
      </div>

      <TabNav tabs={tabs} />
    </div>
  );
}

function StatusPanel({
  icon: Icon,
  title,
  status,
  tone,
  children,
}: {
  icon: typeof Activity;
  title: string;
  status: string;
  tone: "accent" | "good" | "warn" | "bad" | "muted";
  children: ReactNode;
}) {
  const toneCls: Record<typeof tone, string> = {
    accent: "border-accent/35 bg-accent/5 text-accent",
    good: "border-good/35 bg-good/5 text-good",
    warn: "border-warn/35 bg-warn/5 text-warn",
    bad: "border-bad/35 bg-bad/5 text-bad",
    muted: "border-border bg-panel text-muted",
  };
  return (
    <section className="rounded-xl border border-border bg-card/70 p-3 shadow-e1">
      <div className="mb-2 flex items-center gap-2">
        <span className={cn("grid size-8 place-items-center rounded-lg border", toneCls[tone])}>
          <Icon className="size-4" />
        </span>
        <div className="min-w-0 flex-1">
          <h3 className="text-sm font-semibold">{title}</h3>
          <div className="truncate text-xs text-muted">{status}</div>
        </div>
      </div>
      {children}
    </section>
  );
}

function EmptyDash({ label = "—" }: { label?: string }) {
  return <div className="text-xs text-muted">{label}</div>;
}

function Row({ k, v }: { k: string; v: ReactNode }) {
  return (
    <div className="flex items-center justify-between gap-2 border-b border-border/40 py-0.5 last:border-0">
      <span className="text-muted">{k}</span>
      <span className="tabular-nums">{v}</span>
    </div>
  );
}
