import { useEffect, useMemo, useState } from "react";
import { Wrench, RefreshCw, Activity, AlertTriangle, Boxes, Search, ShieldCheck } from "lucide-react";
import { getJSON } from "@/app/api";
import { useEvents } from "@/app/events";
import { cn, clip, fmtTime } from "@/app/utils";
import { Button } from "@/components/ui/button";
import { Muted, ErrorText } from "@/components/JsonView";
import { SkeletonList } from "@/components/ui/skeleton";
import { Page } from "@/components/ui/page";
import { Ring } from "@/components/Widgets";
import { useToolLogPager } from "@/app/cursor-pager";
import { LoadMoreFooter } from "@/components/ui/load-more-footer";
import { StatTile } from "@/components/ui/metric-widget";
import { Segmented } from "@/components/ui/segmented";

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
  observation_trust?: string;
  observation_source?: string;
  directive_like?: boolean;
  directive_matches?: string[];
}
interface ToolDef {
  name?: string;
  description?: string;
  capability?: string; // the Edict capability this tool exercises (M916)
  effect_class?: string;
  rollback_mode?: string;
  rollback_notes?: string;
}

// ToolView is one row of the capability gallery: the catalog definition joined
// with its live usage (calls/errors/latency) and a derived source.
export interface ToolView {
  name: string;
  description: string;
  capability: string;
  effectClass: string;
  rollbackMode: string;
  rollbackNotes: string;
  source: ToolSource;
  calls: number;
  errors: number;
  avgMs?: number;
}

export type ToolSource = "mcp" | "forged" | "skill" | "builtin";

// toolSource infers where a tool comes from by its name — the same prefixes the
// kernel uses (mcp_<server>_<tool> for attached MCP servers; forged/skill tools
// carry their own prefixes). Pure + unit-tested.
export function toolSource(name: string): ToolSource {
  if (name.startsWith("mcp_")) return "mcp";
  if (name.startsWith("forge_") || name.startsWith("forged_")) return "forged";
  if (name.startsWith("skill_")) return "skill";
  return "builtin";
}

// mergeToolViews joins the tool catalog with the per-tool usage stats into a
// single sorted gallery model (used tools first by call volume, then idle
// alphabetically). Pure + unit-tested.
export function mergeToolViews(catalog: ToolDef[], byTool: Record<string, ToolStat>): ToolView[] {
  const views = catalog
    .filter((t) => t.name)
    .map((t): ToolView => {
      const s = byTool[t.name!] || {};
      return {
        name: t.name!,
        description: t.description || "",
        capability: t.capability || "",
        effectClass: t.effect_class || "",
        rollbackMode: t.rollback_mode || "",
        rollbackNotes: t.rollback_notes || "",
        source: toolSource(t.name!),
        calls: s.calls || 0,
        errors: s.errors || 0,
        avgMs: s.avg_ms,
      };
    });
  views.sort((a, b) => {
    if ((b.calls > 0 ? 1 : 0) !== (a.calls > 0 ? 1 : 0)) return b.calls - a.calls; // used first
    if (a.calls !== b.calls) return b.calls - a.calls;
    return a.name.localeCompare(b.name);
  });
  return views;
}



const SOURCE_LABEL: Record<ToolSource, string> = {
  mcp: "mcp",
  forged: "forged",
  skill: "skill",
  builtin: "built-in",
};

function ms(v?: number): string {
  if (v == null) return "—";
  if (v < 1000) return `${v}ms`;
  return `${(v / 1000).toFixed(1)}s`;
}

function rollbackBadge(v: ToolView): { label: string; tone: "good" | "warn" | "bad" | "muted" } | null {
  switch (v.rollbackMode) {
    case "audit_only":
      return { label: "audit only", tone: "bad" };
    case "compensate":
      return { label: "compensate", tone: "warn" };
    case "rollbackable":
      return { label: "rollbackable", tone: "good" };
    case "none_needed":
      return { label: "no rollback", tone: "muted" };
    default:
      return v.effectClass === "irreversible" ? { label: "audit only", tone: "bad" } : null;
  }
}

// Tools is the tool-usage monitor: call volume, error rate, per-tool calls /
// errors / latency, and a live colour-coded invocation log.
export function Tools() {
  const { events } = useEvents();
  const [stats, setStats] = useState<Stats | null>(null);
  const [catalog, setCatalog] = useState<ToolDef[]>([]);
  const [err, setErr] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);

  // The invocation log is cursor-paginated via useToolLogPager (which owns its
  // own polling + live-event reload); reload() below fetches only the stats and
  // catalog. Rows arrive as the generic LogRow shape; read back as Invocation.
  const {
    paged: logRows,
    loadMore,
    loadingMore,
    moreError,
    hasMore,
  } = useToolLogPager(50);
  const log = logRows as unknown as Invocation[];

  async function reload() {
    setLoading(true);
    const [s, c] = await Promise.allSettled([
      getJSON<Stats>("/api/tools"),
      getJSON<{ tools?: ToolDef[] }>("/api/tools_catalog"),
    ]);
    if (s.status === "fulfilled") {
      setStats(s.value);
      setErr(null);
    } else setErr((s.reason as Error).message);
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

  // Capability gallery model (M916): the catalog joined with usage, the capability
  // filter chips, and the current filtered view.
  const views = useMemo(() => mergeToolViews(catalog, byTool), [catalog, byTool]);

  return (
    <Page
      icon={Wrench}
      title="Tool usage"
      description="How much each tool is actually called, how often it errors, and a live invocation log. The Tool registry lists what exists."
      width="wide"
      mode="scroll"
      className="gap-4"
      actions={
        <Button variant="ghost" size="sm" onClick={reload} disabled={loading}>
          <RefreshCw className={cn("size-3.5", loading && "animate-spin")} /> Refresh
        </Button>
      }
    >

      {err ? (
        <ErrorText>{err}</ErrorText>
      ) : !stats ? (
        <SkeletonList count={3} lines={1} />
      ) : (
        <>
          {/* The dial earns its 130px only once there is a rate to draw. With
              no calls yet it read "—" next to three zeros — a quarter of the
              page spent saying nothing has happened. */}
          <div className={cn("grid gap-3", stats.total ? "grid-cols-2 lg:grid-cols-4" : "grid-cols-1 sm:grid-cols-3")}>
            {!!stats.total && (
              <div className="flex items-center justify-center glass rounded-xl p-3">
                <Ring
                  pct={errPct}
                  center={`${errPct}%`}
                  label="error rate"
                  tone={errPct === 0 ? "good" : errPct < 10 ? "warn" : "bad"}
                />
              </div>
            )}
            <StatTile icon={Activity} label="calls" value={(stats.total ?? 0).toLocaleString()} tone="accent" />
            <StatTile icon={AlertTriangle} label="errored" value={stats.errored ?? 0} tone={(stats.errored ?? 0) > 0 ? "bad" : "muted"} />
            <StatTile icon={Wrench} label="tools used" value={stats.tools ?? tools.length} />
          </div>

          {/* No tool catalogue here. This card used to redraw every registered
              tool — name, capability, description — which is exactly what the
              Tool registry (Agents › Capabilities) is, and it did it directly
              above "Usage by tool", which already carries the call counts it
              was also showing. This page answers "what is being called and how
              often"; the registry answers "what exists and under what policy".
              The search and capability filter went with the list, to the page
              that has thirty tools to search. */}
          <Card title="Usage by tool" icon={Wrench}>
            {tools.length === 0 ? (
              <Muted>
                No tool calls yet. Every tool the agent <em>can</em> call is listed in the{" "}
                <a href="#catalog" className="text-accent hover:underline">
                  Tool registry
                </a>
                .
              </Muted>
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
              <>
                <ul className="max-h-80 overflow-auto font-mono text-xs">
                  {log.map((ev, i) => (
                    <li
                      key={(ev as { seq?: number }).seq ?? i}
                      className={cn("flex items-center gap-2 border-b border-border/40 py-1 last:border-0", ev.error && "bg-bad/5")}
                    >
                      <span className="w-14 shrink-0 tabular-nums text-muted">{fmtTime(ev.ts_unix_ms)}</span>
                      <span className={cn("w-28 shrink-0 truncate font-medium", ev.error ? "text-bad" : "text-accent")}>
                        {ev.error ? "✗" : "✓"} {ev.tool || "?"}
                      </span>
                      {ev.duration_ms != null && <span className="w-12 shrink-0 tabular-nums text-muted">{ms(ev.duration_ms)}</span>}
                      <ObservationBadge ev={ev} />
                      <span className="min-w-0 flex-1 truncate text-muted">
                        {[ev.input, ev.output].filter(Boolean).map((s) => clip(String(s), 60)).join(" → ")}
                      </span>
                    </li>
                  ))}
                </ul>
                <LoadMoreFooter
                  hasMore={hasMore}
                  loadingMore={loadingMore}
                  moreError={moreError}
                  onLoadMore={loadMore}
                  pageSize={50}
                  label="invocation log"
                />
              </>
            )}
          </Card>
        </>
      )}
    </Page>
  );
}

function ObservationBadge({ ev }: { ev: Invocation }) {
  const source = ev.observation_source ? ` from ${ev.observation_source}` : "";
  if (ev.directive_like) {
    const matches = ev.directive_matches?.length ? `; matches: ${ev.directive_matches.join(", ")}` : "";
    return (
      <span
        className="inline-flex shrink-0 items-center gap-1 rounded-full border border-bad/40 bg-bad/10 px-1.5 py-0.5 text-xs font-semibold text-bad"
        title={`Directive-like untrusted observation${source}${matches}`}
      >
        <AlertTriangle className="size-3" /> injection
      </span>
    );
  }
  if (ev.observation_trust === "untrusted") {
    return (
      <span
        className="inline-flex shrink-0 items-center gap-1 rounded-full border border-border bg-panel px-1.5 py-0.5 text-xs font-semibold text-muted"
        title={`Untrusted observation${source}`}
      >
        <ShieldCheck className="size-3" /> untrusted
      </span>
    );
  }
  return null;
}



function Card({ title, icon: Icon, children }: { title: string; icon: typeof Wrench; children: React.ReactNode }) {
  return (
    <div className="glass rounded-xl p-3">
      <div className="mb-2 flex items-center gap-1.5 text-xs font-semibold uppercase tracking-normal text-muted">
        <Icon className="size-3.5" /> {title}
      </div>
      {children}
    </div>
  );
}
