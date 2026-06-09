import { useEffect, useState } from "react";
import { Cpu, RefreshCw, Route, GitFork } from "lucide-react";
import { getJSON } from "@/lib/api";
import { useEvents } from "@/lib/events";
import { cn, fmtTime } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { Muted, ErrorText } from "@/components/JsonView";
import { SkeletonList } from "@/components/ui/skeleton";
import { BarList } from "@/components/Charts";
import { Ring } from "@/components/Widgets";

interface Stats {
  routed?: number;
  fallbacks?: number;
  fallback_rate?: number;
  by_primary?: Record<string, number>;
  fallbacks_by_primary?: Record<string, number>;
}
interface LogEvent {
  ts_unix_ms?: number;
  kind?: string;
  primary?: string;
  failed?: string;
  next?: string;
  reason?: string;
  task_type?: string;
}

// Providers is the routing monitor: how many calls were routed, the fallback
// rate, which providers served them (with their fallback share), and a live
// colour-coded routing log.
export function Providers() {
  const { events } = useEvents();
  const [stats, setStats] = useState<Stats | null>(null);
  const [log, setLog] = useState<LogEvent[]>([]);
  const [err, setErr] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);

  async function reload() {
    setLoading(true);
    const [s, l] = await Promise.allSettled([
      getJSON<Stats>("/api/providers"),
      getJSON<{ events?: LogEvent[] }>("/api/provider_log", { limit: "50" }),
    ]);
    if (s.status === "fulfilled") {
      setStats(s.value);
      setErr(null);
    } else setErr((s.reason as Error).message);
    if (l.status === "fulfilled") setLog(l.value.events || []);
    setLoading(false);
  }
  useEffect(() => {
    reload();
    const id = setInterval(reload, 6000);
    return () => clearInterval(id);
  }, []);

  // Nudge on routing/fallback events.
  const head = events[0]?.kind;
  useEffect(() => {
    if (head === "routing.decision" || head === "provider.fallback" || head === "task.completed") reload();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [head, events[0]?.id]);

  const byPrimary = stats?.by_primary || {};
  const fbBy = stats?.fallbacks_by_primary || {};
  const fbRatePct = Math.round((stats?.fallback_rate ?? 0) * 100);
  const rows = Object.entries(byPrimary)
    .sort((a, b) => b[1] - a[1])
    .map(([name, n]) => ({
      label: name,
      value: n,
      sub: `${n} routed${fbBy[name] ? ` · ${fbBy[name]} fb` : ""}`,
    }));

  return (
    <div className="space-y-4">
      <div className="flex items-center gap-2">
        <h2 className="flex items-center gap-2 text-sm font-semibold">
          <Cpu className="size-4 text-accent" /> Providers
        </h2>
        <Button variant="ghost" size="sm" className="ml-auto" onClick={reload} disabled={loading}>
          <RefreshCw className={cn("size-3.5", loading && "animate-spin")} /> Refresh
        </Button>
      </div>

      {err ? (
        <ErrorText>{err}</ErrorText>
      ) : !stats ? (
        <SkeletonList count={3} lines={1} />
      ) : (
        <>
          <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
            <div className="flex items-center justify-center rounded-lg border border-border bg-card p-3">
              <Ring
                pct={fbRatePct}
                center={`${fbRatePct}%`}
                label="fallback rate"
                tone={stats.routed ? (fbRatePct < 5 ? "good" : fbRatePct < 20 ? "warn" : "bad") : "muted"}
              />
            </div>
            <Tile icon={Route} label="routed" value={(stats.routed ?? 0).toLocaleString()} tone="accent" />
            <Tile icon={GitFork} label="fallbacks" value={stats.fallbacks ?? 0} tone={(stats.fallbacks ?? 0) > 0 ? "bad" : "muted"} />
            <Tile icon={Cpu} label="providers" value={rows.length} tone="muted" />
          </div>

          <Card title="Routes by provider" icon={Cpu}>
            {rows.length ? <BarList rows={rows} /> : <Muted>no routing decisions yet</Muted>}
          </Card>

          <Card title="Routing activity" icon={Route}>
            {log.length === 0 ? (
              <Muted>no routing events</Muted>
            ) : (
              <ul className="max-h-80 overflow-auto font-mono text-xs">
                {log.map((ev, i) => (
                  <li key={i} className="flex items-center gap-2 border-b border-border/40 py-1 last:border-0">
                    <span className="w-14 shrink-0 tabular-nums text-muted">{fmtTime(ev.ts_unix_ms)}</span>
                    {ev.kind === "fallback" ? (
                      <span className="text-bad">
                        ↻ fallback {ev.failed || "?"} → {ev.next || "?"}
                        {ev.reason ? <span className="text-muted"> · {ev.reason.slice(0, 80)}</span> : null}
                      </span>
                    ) : (
                      <span className="text-accent">
                        → route {ev.primary || "?"}
                        {ev.task_type ? <span className="text-muted"> · {ev.task_type}</span> : null}
                      </span>
                    )}
                  </li>
                ))}
              </ul>
            )}
          </Card>
        </>
      )}
    </div>
  );
}

function Tile({
  icon: Icon,
  label,
  value,
  tone,
}: {
  icon: typeof Cpu;
  label: string;
  value: number | string;
  tone: "accent" | "bad" | "muted";
}) {
  const color = { accent: "text-accent", bad: "text-bad", muted: "text-foreground" }[tone];
  return (
    <div className="rounded-lg border border-border bg-card px-3 py-2.5">
      <div className="flex items-center gap-1.5 text-xs text-muted">
        <Icon className="size-3.5" /> {label}
      </div>
      <div className={cn("mt-1 text-xl font-semibold tabular-nums", color)}>{value}</div>
    </div>
  );
}

function Card({ title, icon: Icon, children }: { title: string; icon: typeof Cpu; children: React.ReactNode }) {
  return (
    <div className="rounded-lg border border-border bg-card p-3">
      <div className="mb-2 flex items-center gap-1.5 text-xs font-semibold uppercase tracking-wider text-muted">
        <Icon className="size-3.5" /> {title}
      </div>
      {children}
    </div>
  );
}
