import { useEffect, useState } from "react";
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
import { Muted, ErrorText } from "@/components/JsonView";
import { SkeletonList } from "@/components/ui/skeleton";

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
  schedules?: { enabled?: number; total?: number };
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
    if (head === "task.received" || head === "task.completed" || head === "kernel.halt" || head === "kernel.resume")
      reload();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [head, events[0]?.id]);

  const halted = data?.halted;

  return (
    <div className="space-y-4">
      <div className="flex items-center gap-3">
        <h2 className="flex items-center gap-2 text-sm font-semibold">
          <Activity className="size-4 text-accent" /> System health
        </h2>
        <span className={cn("inline-flex items-center gap-1 text-xs", connected ? "text-good" : "text-bad")}>
          ● {connected ? "live" : "disconnected"}
        </span>
        <Button variant="ghost" size="sm" className="ml-auto" onClick={reload} disabled={loading}>
          <RefreshCw className={cn("size-3.5", loading && "animate-spin")} /> Refresh
        </Button>
      </div>

      {err ? (
        <ErrorText>{err}</ErrorText>
      ) : !data ? (
        <SkeletonList count={3} lines={2} />
      ) : (
        <>
          {/* Operational banner */}
          <div
            className={cn(
              "flex flex-wrap items-center gap-x-6 gap-y-2 rounded-lg border p-3",
              halted ? "border-bad bg-bad/10" : "border-good/40 bg-good/5",
            )}
          >
            <div className="flex items-center gap-2">
              <span className={cn("size-2.5 rounded-full", halted ? "bg-bad animate-pulse" : "bg-good")} />
              <span className={cn("text-lg font-semibold", halted ? "text-bad" : "text-good")}>
                {halted ? "HALTED" : "Operational"}
              </span>
            </div>
            <Vital icon={Cpu} label="model" value={data.model || "—"} />
            <Vital icon={Clock} label="uptime" value={uptime(data.uptime_seconds || 0)} />
            <Vital icon={Server} label="daemon" value={`v${data.daemon || "?"} · ${data.protocol || ""}`} />
          </div>

          {/* Live counters */}
          <div className="grid grid-cols-2 gap-2 sm:grid-cols-4">
            <Tile icon={ListTree} label="active runs" value={data.active_runs ?? 0} tone={(data.active_runs ?? 0) > 0 ? "accent" : "muted"} pulse={(data.active_runs ?? 0) > 0} />
            <Tile icon={CheckSquare} label="pending approvals" value={data.pending_approvals ?? 0} tone={(data.pending_approvals ?? 0) > 0 ? "bad" : "muted"} />
            <Tile icon={Database} label="journal head" value={(data.journal_head ?? 0).toLocaleString()} tone="muted" />
            <Tile icon={Cpu} label="tools" value={data.tools ?? 0} tone="muted" />
            <Tile icon={Brain} label="memory records" value={data.memory_records ?? 0} tone="muted" />
            <Tile icon={Network} label="world entities" value={data.world_entities ?? 0} tone="muted" />
            <Tile icon={Sparkles} label="active skills" value={data.active_skills ?? 0} tone="muted" />
            <Tile
              icon={CalendarClock}
              label="schedules"
              value={`${data.schedules?.enabled ?? 0}/${data.schedules?.total ?? 0}`}
              tone="muted"
            />
          </div>

          {/* Detail cards */}
          <div className="grid grid-cols-1 gap-3 lg:grid-cols-2">
            <Card title="Delegation" icon={Network}>
              {data.delegation ? (
                <ul className="space-y-1 text-xs">
                  <Row k="enabled" v={data.delegation.enabled ? "yes" : "no"} />
                  <Row k="max depth" v={data.delegation.max_depth ?? "—"} />
                  <Row k="max fan-out" v={data.delegation.max_fanout ? data.delegation.max_fanout : "unbounded"} />
                  <Row
                    k="max spend"
                    v={data.delegation.max_spend_microcents ? `$${(data.delegation.max_spend_microcents / 1e9).toFixed(4)}` : "uncapped"}
                  />
                </ul>
              ) : (
                <Muted>—</Muted>
              )}
            </Card>

            <Card title="HTTP surface" icon={Server}>
              {data.http_servers?.length ? (
                <ul className="space-y-1 text-xs">
                  {data.http_servers.map((s, i) => (
                    <li key={i} className="flex items-center gap-2">
                      <span className="font-mono">{s.addr}</span>
                      <span className="text-muted">{s.name}</span>
                      {s.loopback && (
                        <span className="ml-auto rounded-full bg-good/15 px-1.5 py-0.5 text-[10px] text-good">loopback</span>
                      )}
                    </li>
                  ))}
                </ul>
              ) : (
                <Muted>none</Muted>
              )}
            </Card>

            <Card title="Credentials" icon={KeyRound}>
              <div className="break-words font-mono text-xs text-muted">{data.cred_chain || "—"}</div>
            </Card>

            <Card title="Provider routing" icon={ShieldAlert}>
              <div className="text-xs">
                <Row k="fallbacks" v={data.provider_fallbacks?.count ?? 0} />
                {data.provider_fallbacks?.last_reason && (
                  <div className="mt-1.5 rounded-md border border-warn/40 bg-warn/5 p-2 text-[11px] text-muted">
                    <span className="text-warn">last fallback: </span>
                    {data.provider_fallbacks.last_reason.slice(0, 200)}
                  </div>
                )}
              </div>
            </Card>
          </div>
        </>
      )}
    </div>
  );
}

function Vital({ icon: Icon, label, value }: { icon: typeof Activity; label: string; value: string }) {
  return (
    <div className="flex items-center gap-2">
      <Icon className="size-4 text-muted" />
      <div>
        <div className="text-[10px] uppercase tracking-wider text-muted">{label}</div>
        <div className="text-sm font-medium tabular-nums">{value}</div>
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
      <div className={cn("mt-1 text-xl font-semibold tabular-nums", color)}>{value}</div>
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

function Row({ k, v }: { k: string; v: React.ReactNode }) {
  return (
    <div className="flex items-center justify-between gap-2 border-b border-border/40 py-0.5 last:border-0">
      <span className="text-muted">{k}</span>
      <span className="tabular-nums">{v}</span>
    </div>
  );
}
