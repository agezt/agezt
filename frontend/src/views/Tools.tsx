import { useEffect, useMemo, useState } from "react";
import { Wrench, RefreshCw, Activity, AlertTriangle, Boxes, Search, ShieldCheck } from "lucide-react";
import { getJSON } from "@/lib/api";
import { useEvents } from "@/lib/events";
import { cn, clip, fmtTime } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { Muted, ErrorText } from "@/components/JsonView";
import { SkeletonList } from "@/components/ui/skeleton";
import { PageHeader } from "@/components/ui/page-header";
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
  observation_trust?: string;
  observation_source?: string;
  directive_like?: boolean;
  directive_matches?: string[];
}
interface ToolDef {
  name?: string;
  description?: string;
  capability?: string; // the Edict capability this tool exercises (M916)
}

// ToolView is one row of the capability gallery: the catalog definition joined
// with its live usage (calls/errors/latency) and a derived source.
export interface ToolView {
  name: string;
  description: string;
  capability: string;
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

// filterTools narrows the gallery by a free-text query (name/description/
// capability, case-insensitive) and an optional exact capability. Pure + tested.
export function filterTools(views: ToolView[], query: string, capability: string): ToolView[] {
  const q = query.trim().toLowerCase();
  return views.filter((v) => {
    if (capability && v.capability !== capability) return false;
    if (!q) return true;
    return (
      v.name.toLowerCase().includes(q) ||
      v.description.toLowerCase().includes(q) ||
      v.capability.toLowerCase().includes(q)
    );
  });
}

// capabilityCounts tallies tools per capability for the filter chips, sorted by
// count then name. Pure + unit-tested.
export function capabilityCounts(views: ToolView[]): { capability: string; n: number }[] {
  const m = new Map<string, number>();
  for (const v of views) {
    if (!v.capability) continue;
    m.set(v.capability, (m.get(v.capability) || 0) + 1);
  }
  return [...m.entries()]
    .map(([capability, n]) => ({ capability, n }))
    .sort((a, b) => (b.n !== a.n ? b.n - a.n : a.capability.localeCompare(b.capability)));
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

// Tools is the tool-usage monitor: call volume, error rate, per-tool calls /
// errors / latency, and a live colour-coded invocation log.
export function Tools() {
  const { events } = useEvents();
  const [stats, setStats] = useState<Stats | null>(null);
  const [log, setLog] = useState<Invocation[]>([]);
  const [catalog, setCatalog] = useState<ToolDef[]>([]);
  const [err, setErr] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [query, setQuery] = useState("");
  const [capFilter, setCapFilter] = useState("");

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

  // Capability gallery model (M916): the catalog joined with usage, the capability
  // filter chips, and the current filtered view.
  const views = useMemo(() => mergeToolViews(catalog, byTool), [catalog, byTool]);
  const capChips = useMemo(() => capabilityCounts(views), [views]);
  const shownTools = useMemo(() => filterTools(views, query, capFilter), [views, query, capFilter]);

  return (
    <div className="space-y-4">
      <PageHeader
        icon={Wrench}
        title="Tools"
        description="Tool usage monitor — capabilities, call volume, and a live invocation log."
        actions={
          <Button variant="ghost" size="sm" onClick={reload} disabled={loading}>
            <RefreshCw className={cn("size-3.5", loading && "animate-spin")} /> Refresh
          </Button>
        }
      />

      {err ? (
        <ErrorText>{err}</ErrorText>
      ) : !stats ? (
        <SkeletonList count={3} lines={1} />
      ) : (
        <>
          <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
            <div className="flex items-center justify-center glass rounded-xl p-3">
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
              <>
                {/* Search + capability filter chips (M916) — find any of the agent's
                    tools fast, or narrow to one Edict capability. */}
                <div className="mb-2 flex items-center gap-2">
                  <div className="relative flex-1">
                    <Search className="pointer-events-none absolute left-2 top-1/2 size-3.5 -translate-y-1/2 text-muted" />
                    <input
                      value={query}
                      onChange={(e) => setQuery(e.target.value)}
                      placeholder="Search tools by name, description, or capability…"
                      aria-label="Search tools"
                      className="h-8 w-full rounded-md border border-border bg-panel pl-7 pr-2 text-xs outline-none focus:border-accent"
                    />
                  </div>
                  <span className="shrink-0 text-[11px] tabular-nums text-muted">{shownTools.length} shown</span>
                </div>
                {capChips.length > 0 && (
                  <div className="mb-2 flex flex-wrap gap-1.5">
                    <FilterChip label="all" n={views.length} active={capFilter === ""} onClick={() => setCapFilter("")} />
                    {capChips.map((c) => (
                      <FilterChip
                        key={c.capability}
                        label={c.capability}
                        n={c.n}
                        active={capFilter === c.capability}
                        onClick={() => setCapFilter(capFilter === c.capability ? "" : c.capability)}
                      />
                    ))}
                  </div>
                )}
                {shownTools.length === 0 ? (
                  <Muted>no tools match</Muted>
                ) : (
                  <ul className="grid gap-1.5 sm:grid-cols-2">
                    {shownTools.map((t) => (
                      <li key={t.name} className="rounded-md border border-border/60 bg-panel/40 px-2.5 py-1.5">
                        <div className="flex items-center gap-1.5">
                          <span className="truncate font-mono text-xs font-medium">{t.name}</span>
                          {t.source !== "builtin" && (
                            <span className="shrink-0 rounded bg-accent/10 px-1 text-[9px] font-semibold uppercase tracking-wider text-accent">
                              {SOURCE_LABEL[t.source]}
                            </span>
                          )}
                          <span
                            className={cn(
                              "ml-auto shrink-0 rounded-full px-1.5 py-0.5 text-[10px] font-semibold tabular-nums",
                              t.calls > 0 ? "bg-accent/15 text-accent" : "bg-panel text-muted",
                            )}
                            title={t.calls > 0 ? "called this session" : "available but not yet called"}
                          >
                            {t.calls > 0 ? `${t.calls} call${t.calls === 1 ? "" : "s"}` : "idle"}
                          </span>
                        </div>
                        <div className="mt-1 flex flex-wrap items-center gap-x-2 gap-y-0.5 text-[10px] text-muted">
                          {t.capability && (
                            <span className="inline-flex items-center gap-0.5" title="Edict capability">
                              <ShieldCheck className="size-2.5" /> {t.capability}
                            </span>
                          )}
                          {t.errors > 0 && <span className="text-bad">{t.errors} err</span>}
                          {t.avgMs != null && <span>{ms(t.avgMs)} avg</span>}
                        </div>
                        {t.description && <p className="mt-0.5 line-clamp-2 text-[11px] text-muted">{clip(t.description, 140)}</p>}
                      </li>
                    ))}
                  </ul>
                )}
              </>
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
                    <ObservationBadge ev={ev} />
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

function ObservationBadge({ ev }: { ev: Invocation }) {
  const source = ev.observation_source ? ` from ${ev.observation_source}` : "";
  if (ev.directive_like) {
    const matches = ev.directive_matches?.length ? `; matches: ${ev.directive_matches.join(", ")}` : "";
    return (
      <span
        className="inline-flex shrink-0 items-center gap-1 rounded-full border border-bad/40 bg-bad/10 px-1.5 py-0.5 text-[10px] font-semibold text-bad"
        title={`Directive-like untrusted observation${source}${matches}`}
      >
        <AlertTriangle className="size-3" /> injection
      </span>
    );
  }
  if (ev.observation_trust === "untrusted") {
    return (
      <span
        className="inline-flex shrink-0 items-center gap-1 rounded-full border border-border bg-panel px-1.5 py-0.5 text-[10px] font-semibold text-muted"
        title={`Untrusted observation${source}`}
      >
        <ShieldCheck className="size-3" /> untrusted
      </span>
    );
  }
  return null;
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

function FilterChip({ label, n, active, onClick }: { label: string; n: number; active: boolean; onClick: () => void }) {
  return (
    <button
      onClick={onClick}
      className={cn(
        "inline-flex items-center gap-1 rounded-full border px-2 py-0.5 text-[11px] transition-colors",
        active ? "border-accent bg-accent/10 text-accent" : "border-border text-muted hover:border-accent",
      )}
    >
      <span className="font-mono">{label}</span>
      <span className="rounded-full bg-card px-1 text-[10px] tabular-nums">{n}</span>
    </button>
  );
}

function Card({ title, icon: Icon, children }: { title: string; icon: typeof Wrench; children: React.ReactNode }) {
  return (
    <div className="glass rounded-xl p-3">
      <div className="mb-2 flex items-center gap-1.5 text-xs font-semibold uppercase tracking-wider text-muted">
        <Icon className="size-3.5" /> {title}
      </div>
      {children}
    </div>
  );
}
