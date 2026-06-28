import { useEffect, useState, type ReactNode } from "react";
import { Cpu, RefreshCw, Route, GitFork, RotateCw } from "lucide-react";
import { getJSON, postAction } from "@/lib/api";
import { useEvents } from "@/lib/events";
import { useUI } from "@/components/ui/feedback";
import { cn, fmtTime } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { Muted, ErrorText } from "@/components/JsonView";
import { SkeletonList } from "@/components/ui/skeleton";
import { PageHeader } from "@/components/ui/page-header";
import { BarList } from "@/components/Charts";
import { MetricWidget, MetricGrid } from "@/components/ui/metric-widget";

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
  const ui = useUI();
  const [stats, setStats] = useState<Stats | null>(null);
  const [log, setLog] = useState<LogEvent[]>([]);
  const [err, setErr] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [reloading, setReloading] = useState(false);

  // Re-read credentials + catalog on the daemon, in place (M745) — apply a key or
  // provider change without restarting. Distinct from Refresh, which only re-fetches
  // these stats. Surfaces the daemon's note when it could only refresh the catalog.
  async function reloadProviders() {
    setReloading(true);
    try {
      const r = await postAction<{ providers_reloaded?: boolean; provider_count?: number; note?: string }>(
        "/api/provider/reload",
        {},
      );
      ui.toast(
        r.note
          ? r.note
          : `Providers reloaded${r.provider_count != null ? ` — ${r.provider_count} provider${r.provider_count === 1 ? "" : "s"}` : ""}`,
        r.providers_reloaded ? "success" : "info",
      );
      await reload();
    } catch (e) {
      ui.toast((e as Error).message, "error");
    } finally {
      setReloading(false);
    }
  }

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
      <PageHeader
        icon={Cpu}
        title="Providers"

        actions={
          <>
            <Button variant="ghost" size="sm" onClick={reloadProviders} disabled={reloading} title="Re-read credentials & catalog without restarting the daemon">
              <RotateCw className={cn("size-3.5", reloading && "animate-spin")} /> Reload
            </Button>
            <Button variant="ghost" size="sm" onClick={reload} disabled={loading} title="Re-fetch these stats">
              <RefreshCw className={cn("size-3.5", loading && "animate-spin")} /> Refresh
            </Button>
          </>
        }
      />

      {err ? (
        <ErrorText>{err}</ErrorText>
      ) : !stats ? (
        <SkeletonList count={3} lines={1} />
      ) : (
        <>
          <MetricGrid cols="repeat(auto-fill, minmax(140px, 1fr))">
            <MetricWidget
              icon={GitFork}
              label="Fallback rate"
              value={`${fbRatePct}%`}
              tone={stats.routed ? (fbRatePct < 5 ? "good" : fbRatePct < 20 ? "warn" : "bad") : "muted"}
            />
            <MetricWidget icon={Route} label="Routed" value={(stats.routed ?? 0).toLocaleString()} tone="accent" />
            <MetricWidget icon={GitFork} label="Fallbacks" value={stats.fallbacks ?? 0} tone={(stats.fallbacks ?? 0) > 0 ? "bad" : "muted"} />
            <MetricWidget icon={Cpu} label="Providers" value={rows.length} tone="muted" />
          </MetricGrid>

          <ProviderPanel
            title="Routes by provider"
            icon={Cpu}
            status={rows.length ? `${rows.length} active` : "waiting for routes"}
            tone={rows.length ? "accent" : "muted"}
          >
            {rows.length ? <BarList rows={rows} /> : <Muted>no routing decisions yet</Muted>}
          </ProviderPanel>

          <ProviderPanel
            title="Routing activity"
            icon={Route}
            status={log.length ? `${log.length} recent` : "quiet"}
            tone={log.some((ev) => ev.kind === "fallback") ? "warn" : log.length ? "accent" : "muted"}
          >
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
          </ProviderPanel>
        </>
      )}
    </div>
  );
}

function ProviderPanel({
  title,
  icon: Icon,
  status,
  tone,
  children,
}: {
  title: string;
  icon: typeof Cpu;
  status: string;
  tone: "accent" | "warn" | "bad" | "good" | "muted";
  children: ReactNode;
}) {
  const toneCls: Record<typeof tone, string> = {
    accent: "border-accent/35 bg-accent/5 text-accent",
    warn: "border-warn/35 bg-warn/5 text-warn",
    bad: "border-bad/35 bg-bad/5 text-bad",
    good: "border-good/35 bg-good/5 text-good",
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
