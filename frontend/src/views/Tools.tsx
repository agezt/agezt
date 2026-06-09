import { useEffect, useState } from "react";
import { Wrench, RefreshCw, Activity, AlertTriangle, Boxes } from "lucide-react";
import { getJSON } from "@/lib/api";
import { useEvents } from "@/lib/events";
import { cn, clip, fmtTime } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { Muted, ErrorText } from "@/components/JsonView";
import { SkeletonList } from "@/components/ui/skeleton";
import { Ring } from "@/components/Widgets";

interface ToolStat {
  calls?: number;
  errors?: number;
  avg_ms?: number;
}
interface Stats {
  total?: number;
  errored?: number;
  error_rate?: number;
  tools?: number;
  by_tool?: Record<string, ToolStat>;
}
interface Invocation {
  ts_unix_ms?: number;
  tool?: string;
  error?: boolean;
  duration_ms?: number;
  input?: string;
  output?: string;
}
interface ToolDef {
  name?: string;
  description?: string;
}

function ms(v?: number): string {
  if (v == null) return "—";
  if (v < 1000) return `${v}ms`;
  return `${(v / 1000).toFixed(1)}s`;
}

// Tools is the tool-usage monitor: call volume, error rate, per-tool calls /
// errors / latency, and a live colour-coded invocation log.
export function Tools() {
  const { events } = useEvents();
  const [stats, setStats] = useState<Stats | null>(null);
  const [log, setLog] = useState<Invocation[]>([]);
  const [catalog, setCatalog] = useState<ToolDef[]>([]);
  const [err, setErr] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);

  async function reload() {
    setLoading(true);
    const [s, l, c] = await Promise.allSettled([
      getJSON<Stats>("/api/tools"),
      getJSON<{ invocations?: Invocation[] }>("/api/tool_log", { limit: "50" }),
      getJSON<{ tools?: ToolDef[] }>("/api/tools_catalog"),
    ]);
    if (s.status === "fulfilled") {
      setStats(s.value);
      setErr(null);
    } else setErr((s.reason as Error).message);
    if (l.status === "fulfilled") setLog(l.value.invocations || []);
    if (c.status === "fulfilled") setCatalog(c.value.tools || []);
    setLoading(false);
  }
  useEffect(() => {
    reload();
    const id = setInterval(reload, 6000);
    return () => clearInterval(id);
  }, []);

  const head = events[0]?.kind;
  useEffect(() => {
    if (head === "tool.result" || head === "tool.invoked" || head === "task.completed") reload();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [head, events[0]?.id]);

  const byTool = stats?.by_tool || {};
  const tools = Object.entries(byTool).sort((a, b) => (b[1].calls || 0) - (a[1].calls || 0));
  const maxCalls = Math.max(1, ...tools.map(([, t]) => t.calls || 0));
  const errPct = Math.round((stats?.error_rate ?? 0) * 100);

  return (
    <div className="space-y-4">
      <div className="flex items-center gap-2">
        <h2 className="flex items-center gap-2 text-sm font-semibold">
          <Wrench className="size-4 text-accent" /> Tools
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
                pct={errPct}
                center={stats.total ? `${errPct}%` : "—"}
                label="error rate"
                tone={!stats.total ? "muted" : errPct === 0 ? "good" : errPct < 10 ? "warn" : "bad"}
              />
            </div>
            <Tile icon={Activity} label="calls" value={(stats.total ?? 0).toLocaleString()} tone="accent" />
            <Tile icon={AlertTriangle} label="errored" value={stats.errored ?? 0} tone={(stats.errored ?? 0) > 0 ? "bad" : "muted"} />
            <Tile icon={Wrench} label="tools used" value={stats.tools ?? tools.length} tone="muted" />
          </div>

          <Card title={`Available tools — what the agent can do (${catalog.length})`} icon={Boxes}>
            {catalog.length === 0 ? (
              <Muted>no tools registered</Muted>
            ) : (
              <ul className="grid gap-1.5 sm:grid-cols-2">
                {catalog.map((t) => {
                  const used = (byTool[t.name || ""]?.calls ?? 0) > 0;
                  return (
                    <li key={t.name} className="rounded-md border border-border/60 bg-panel/40 px-2.5 py-1.5">
                      <div className="flex items-center gap-1.5">
                        <span className="truncate font-mono text-xs font-medium">{t.name}</span>
                        <span
                          className={cn(
                            "ml-auto shrink-0 rounded-full px-1.5 py-0.5 text-[10px] font-semibold uppercase tracking-wider",
                            used ? "bg-accent/15 text-accent" : "bg-panel text-muted",
                          )}
                          title={used ? "called this session" : "available but not yet called"}
                        >
                          {used ? "used" : "idle"}
                        </span>
                      </div>
                      {t.description && <p className="mt-0.5 line-clamp-2 text-[11px] text-muted">{clip(t.description, 140)}</p>}
                    </li>
                  );
                })}
              </ul>
            )}
          </Card>

          <Card title="Usage by tool" icon={Wrench}>
            {tools.length === 0 ? (
              <Muted>no tool calls yet</Muted>
            ) : (
              <ul className="space-y-2">
                {tools.map(([name, t]) => {
                  const calls = t.calls || 0;
                  const errors = t.errors || 0;
                  const errPct = calls ? errors / calls : 0;
                  return (
                    <li key={name}>
                      <div className="mb-0.5 flex items-baseline justify-between gap-2 text-xs">
                        <span className="truncate font-mono">{name}</span>
                        <span className="shrink-0 tabular-nums text-muted">
                          {calls} call{calls === 1 ? "" : "s"}
                          {errors ? <span className="text-bad"> · {errors} err</span> : null} · {ms(t.avg_ms)}
                        </span>
                      </div>
                      {/* Bar with an error-share segment. */}
                      <div className="flex h-1.5 overflow-hidden rounded-full bg-panel" style={{ width: `${(calls / maxCalls) * 100}%`, minWidth: "8px" }}>
                        <div className="h-full bg-accent/70" style={{ width: `${(1 - errPct) * 100}%` }} />
                        {errors > 0 && <div className="h-full bg-bad/70" style={{ width: `${errPct * 100}%` }} />}
                      </div>
                    </li>
                  );
                })}
              </ul>
            )}
          </Card>

          <Card title="Invocation log" icon={Activity}>
            {log.length === 0 ? (
              <Muted>no invocations</Muted>
            ) : (
              <ul className="max-h-80 overflow-auto font-mono text-xs">
                {log.map((ev, i) => (
                  <li
                    key={i}
                    className={cn("flex items-center gap-2 border-b border-border/40 py-1 last:border-0", ev.error && "bg-bad/5")}
                  >
                    <span className="w-14 shrink-0 tabular-nums text-muted">{fmtTime(ev.ts_unix_ms)}</span>
                    <span className={cn("w-28 shrink-0 truncate font-medium", ev.error ? "text-bad" : "text-accent")}>
                      {ev.error ? "✗" : "✓"} {ev.tool || "?"}
                    </span>
                    {ev.duration_ms != null && <span className="w-12 shrink-0 tabular-nums text-muted">{ms(ev.duration_ms)}</span>}
                    <span className="min-w-0 flex-1 truncate text-muted">
                      {[ev.input, ev.output].filter(Boolean).map((s) => clip(String(s), 60)).join(" → ")}
                    </span>
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
  icon: typeof Wrench;
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

function Card({ title, icon: Icon, children }: { title: string; icon: typeof Wrench; children: React.ReactNode }) {
  return (
    <div className="rounded-lg border border-border bg-card p-3">
      <div className="mb-2 flex items-center gap-1.5 text-xs font-semibold uppercase tracking-wider text-muted">
        <Icon className="size-3.5" /> {title}
      </div>
      {children}
    </div>
  );
}
