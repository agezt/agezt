import { useEffect, useMemo, useState } from "react";
import {
  Network,
  RefreshCw,
  X,
  Bot,
  Layers,
  Coins,
  Repeat,
  GitBranch,
  ArrowLeft,
  Clock,
  Users,
  Anchor,
  CalendarClock,
  GitFork,
  Cpu,
  Search,
  Radar,
} from "lucide-react";
import { getJSON } from "@/lib/api";
import { useEvents } from "@/lib/events";
import { money } from "@/lib/format";
import { cn, clip, fmtTime } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { ErrorText } from "@/components/JsonView";
import { SkeletonList } from "@/components/ui/skeleton";
import { EmptyState } from "@/components/ui/empty";
import { DelegationGraph } from "@/components/DelegationGraph";
import { RunDetailLoader } from "@/components/RunDetail";
import { AgentAvatar } from "@/components/AgentAvatar";
import { FleetCard, FleetDetail } from "@/components/Fleet";
import { buildDelegationTree, type RunNode } from "@/lib/delegation";
import {
  buildFleet,
  fleetCensus,
  statusKind,
  type StatusKind,
  type FleetKind,
  type ApiProfile,
  type ApiOrder,
  type ApiSchedule,
  type ApiWorkflow,
  type ApiPulse,
} from "@/lib/fleet";

// Re-export the run-status helpers from their canonical home (lib/fleet) so the
// existing test + Dashboard imports from "@/views/Agents" keep working.
export { statusKind, scheduleAgentSlug } from "@/lib/fleet";
export type { StatusKind } from "@/lib/fleet";

interface ApiRun {
  correlation_id?: string;
  parent_correlation?: string;
  status?: string;
  model?: string;
  spent_mc?: number;
  iters?: number;
  intent?: string;
  agent?: string;
  answer_preview?: string;
  started_unix_ms?: number;
}

function toRunNodes(runs: ApiRun[]): RunNode[] {
  return runs.map((r) => ({
    id: r.correlation_id || "",
    parent: r.parent_correlation || undefined,
    status: r.status,
    model: r.model,
    spentMc: r.spent_mc,
    iters: r.iters,
    intent: r.intent,
  }));
}

// RootSummary is one lead run's at-a-glance card data: its own fields plus the
// aggregates of its whole sub-agent delegation tree.
export interface RootSummary {
  id: string;
  intent?: string;
  status?: string;
  kind: StatusKind;
  model?: string;
  agentName?: string;
  answerPreview?: string;
  iters: number;
  spentMc: number; // the lead's own spend
  treeSpentMc: number; // the whole tree's spend
  agents: number; // tree node count (lead + sub-agents)
  subAgents: number;
  depth: number;
  startedMs?: number;
}

// summarizeRoots turns the runs list into one card summary per LEAD run (a run
// with no parent), folding each lead's delegation subtree into aggregates and
// sorting running-first then most-recent. Only includes runs that have an
// agent field set — plain chat conversations (no --agent) are excluded since
// the Agents gallery is for roster agent runs, not ad-hoc chat sessions.
export function summarizeRoots(runs: ApiRun[]): RootSummary[] {
  const nodes = toRunNodes(runs);
  const startedById = new Map<string, number | undefined>();
  const agentById = new Map<string, string | undefined>();
  const previewById = new Map<string, string | undefined>();
  for (const r of runs) {
    if (r.correlation_id) {
      startedById.set(r.correlation_id, r.started_unix_ms);
      agentById.set(r.correlation_id, r.agent);
      previewById.set(r.correlation_id, r.answer_preview);
    }
  }
  const out: RootSummary[] = [];
  for (const n of nodes) {
    if (n.parent) continue; // sub-agents fold into their lead's card
    // Skip runs without an agent — those are ad-hoc chat conversations, not
    // roster agent runs. The Agents gallery should only show runs started via
    // --agent (cron, continuous, event, or manual trigger).
    const agentName = agentById.get(n.id);
    if (!agentName) continue;
    const tree = buildDelegationTree(nodes, n.id);
    out.push({
      id: n.id,
      intent: n.intent,
      status: n.status,
      kind: statusKind(n.status),
      model: n.model,
      agentName,
      answerPreview: previewById.get(n.id) || undefined,
      iters: n.iters || 0,
      spentMc: n.spentMc || 0,
      treeSpentMc: tree.totalSpentMc,
      agents: tree.count,
      subAgents: Math.max(0, tree.count - 1),
      depth: tree.maxDepth,
      startedMs: startedById.get(n.id),
    });
  }
  out.sort((a, b) => {
    if (a.kind === "running" && b.kind !== "running") return -1;
    if (b.kind === "running" && a.kind !== "running") return 1;
    return (b.startedMs || 0) - (a.startedMs || 0);
  });
  return out;
}

export type Filter = "all" | "running" | "done" | "failed";

// filterRoots applies the active filter chip. Pure + unit-tested.
export function filterRoots(roots: RootSummary[], filter: Filter): RootSummary[] {
  if (filter === "all") return roots;
  return roots.filter((r) => r.kind === filter);
}

const KIND_DOT: Record<StatusKind, string> = {
  running: "bg-accent",
  done: "bg-good",
  failed: "bg-bad",
  other: "bg-muted",
};

type Tab = "fleet" | "live";
type FleetFilter = "all" | FleetKind | "running";

const FLEET_FILTERS: { id: FleetFilter; label: string; icon: typeof Bot }[] = [
  { id: "all", label: "All", icon: Network },
  { id: "roster", label: "Roster", icon: Users },
  { id: "standing", label: "Standing", icon: Anchor },
  { id: "schedule", label: "Schedules", icon: CalendarClock },
  { id: "workflow", label: "Workflows", icon: GitFork },
  { id: "system", label: "System", icon: Cpu },
  { id: "running", label: "Running", icon: RefreshCw },
];

// Agents is the multi-agent console. The default "Fleet" tab is a complete
// census of every agent + automation you own — roster identities, standing
// orders, schedules, workflows and the always-on system engines — each card
// spelling out HOW it gets triggered, so the page is full and informative even
// when nothing is running. The "Live" tab is the run monitor: a gallery of
// in-flight lead runs and their sub-agent delegation trees.
export function Agents() {
  const { events } = useEvents();
  const [tab, setTab] = useState<Tab>("fleet");
  const [runs, setRuns] = useState<ApiRun[] | null>(null);
  const [profiles, setProfiles] = useState<ApiProfile[]>([]);
  const [orders, setOrders] = useState<ApiOrder[]>([]);
  const [schedules, setSchedules] = useState<ApiSchedule[]>([]);
  const [workflows, setWorkflows] = useState<ApiWorkflow[]>([]);
  const [pulse, setPulse] = useState<ApiPulse | undefined>(undefined);
  const [catalogLoaded, setCatalogLoaded] = useState(false);
  const [filter, setFilter] = useState<Filter>("all"); // live tab
  const [fleetFilter, setFleetFilter] = useState<FleetFilter>("all");
  const [query, setQuery] = useState("");
  const [sel, setSel] = useState<string>(""); // drilled-into lead run (live tab)
  const [picked, setPicked] = useState<string | null>(null); // node in the graph
  const [selKey, setSelKey] = useState<string>(""); // selected fleet entity
  const [err, setErr] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);

  async function reload() {
    setLoading(true);
    try {
      const d = await getJSON<{ runs?: ApiRun[] }>("/api/runs");
      setRuns(d.runs || []);
      setErr(null);
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setLoading(false);
    }
  }
  // The catalogue inputs change rarely (operator edits), so they load once + on
  // the slower poll, while runs stay on the fast 6s cycle. Each fetch is
  // best-effort: one endpoint failing must not blank the rest of the census.
  async function reloadCatalog() {
    const [a, o, s, w, p] = await Promise.allSettled([
      getJSON<{ profiles?: ApiProfile[] }>("/api/agents"),
      getJSON<{ orders?: ApiOrder[] }>("/api/standing"),
      getJSON<{ schedules?: ApiSchedule[] }>("/api/schedules"),
      getJSON<{ workflows?: ApiWorkflow[] }>("/api/workflows"),
      getJSON<ApiPulse>("/api/pulse"),
    ]);
    if (a.status === "fulfilled") setProfiles(a.value.profiles || []);
    if (o.status === "fulfilled") setOrders(o.value.orders || []);
    if (s.status === "fulfilled") setSchedules(s.value.schedules || []);
    if (w.status === "fulfilled") setWorkflows(w.value.workflows || []);
    if (p.status === "fulfilled") setPulse(p.value);
    setCatalogLoaded(true);
  }

  function reloadAll() {
    reload();
    reloadCatalog();
  }

  useEffect(() => {
    reload();
    reloadCatalog();
    const id = setInterval(reload, 6000);
    const catId = setInterval(reloadCatalog, 30000);
    return () => {
      clearInterval(id);
      clearInterval(catId);
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // Nudge on run lifecycle + delegation events.
  const head = events[0]?.kind;
  useEffect(() => {
    if (head === "task.completed" || head === "task.failed" || head === "subagent.spawned" || head === "task.received")
      reload();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [head, events[0]?.id]);

  const nodes = useMemo(() => (runs ? toRunNodes(runs) : []), [runs]);
  const roots = useMemo(() => (runs ? summarizeRoots(runs) : []), [runs]);
  const shown = useMemo(() => filterRoots(roots, filter), [roots, filter]);
  const tree = useMemo(() => (sel ? buildDelegationTree(nodes, sel) : null), [nodes, sel]);
  const pickedNode = useMemo(() => tree?.nodes.find((n) => n.id === picked) || null, [tree, picked]);

  const running = roots.filter((r) => r.kind === "running").length;
  const totalSubs = roots.reduce((s, r) => s + r.subAgents, 0);
  const totalSpend = roots.reduce((s, r) => s + r.treeSpentMc, 0);

  // The unified census.
  const fleet = useMemo(
    () => buildFleet(profiles, orders, schedules, workflows, runs || [], pulse),
    [profiles, orders, schedules, workflows, runs, pulse],
  );
  const census = useMemo(() => fleetCensus(fleet), [fleet]);
  const shownFleet = useMemo(() => {
    const q = query.trim().toLowerCase();
    return fleet.filter((e) => {
      if (fleetFilter === "running" && !e.running) return false;
      if (fleetFilter !== "all" && fleetFilter !== "running" && e.kind !== fleetFilter) return false;
      if (!q) return true;
      return (
        e.name.toLowerCase().includes(q) ||
        (e.description || "").toLowerCase().includes(q) ||
        (e.model || "").toLowerCase().includes(q) ||
        e.triggers.some((t) => `${t.mode} ${t.label}`.toLowerCase().includes(q))
      );
    });
  }, [fleet, fleetFilter, query]);
  const selEntity = useMemo(() => fleet.find((e) => e.key === selKey) || null, [fleet, selKey]);

  // Jump from a fleet card to the matching live run + its delegation graph.
  function openLiveFor(slug: string) {
    const run = (runs || []).find((r) => (r.agent || "") === slug && statusKind(r.status) === "running");
    if (!run?.correlation_id) return;
    setTab("live");
    setSel(run.correlation_id);
    setPicked(null);
  }
  function manage(view: string) {
    if (view) location.hash = view;
  }

  const header = (
    <div className="flex flex-wrap items-center gap-2">
      <h2 className="flex items-center gap-2 text-sm font-semibold">
        <Network className="size-4 text-accent" /> Agents
      </h2>
      <div className="inline-flex rounded-lg border border-border p-0.5 text-xs">
        {(["fleet", "live"] as Tab[]).map((t) => (
          <button
            key={t}
            onClick={() => setTab(t)}
            className={cn(
              "rounded-md px-2.5 py-1 capitalize transition-colors",
              tab === t ? "bg-accent/15 text-accent" : "text-muted hover:text-foreground",
            )}
          >
            {t === "fleet" ? "Fleet" : "Live"}
            {t === "live" && running > 0 ? <span className="ml-1 text-accent">· {running}</span> : null}
          </button>
        ))}
      </div>
      <Button variant="ghost" size="sm" onClick={reloadAll} title="Reload" className="ml-auto">
        <RefreshCw className={cn("size-3.5", loading && "animate-spin")} />
      </Button>
    </div>
  );

  // ───────────────────────── Live tab: delegation drill-in ─────────────────────────
  if (tab === "live" && sel) {
    return (
      <div className="flex h-full min-h-0 flex-col gap-3">
        <div className="flex flex-wrap items-center gap-2">
          <Button
            variant="ghost"
            size="sm"
            onClick={() => {
              setSel("");
              setPicked(null);
            }}
          >
            <ArrowLeft className="size-3.5" /> All agents
          </Button>
          {tree && tree.count > 0 && (
            <div className="flex flex-wrap gap-1.5 text-xs">
              <Stat icon={Bot} label="agents" value={tree.count} />
              <Stat icon={GitBranch} label="sub-agents" value={Math.max(0, tree.count - 1)} />
              <Stat icon={Layers} label="depth" value={tree.maxDepth} />
              <Stat icon={Coins} label="tree spend" value={money(tree.totalSpentMc)} />
            </div>
          )}
          <Button variant="ghost" size="sm" onClick={reload} title="Reload" className="ml-auto">
            <RefreshCw className={cn("size-3.5", loading && "animate-spin")} />
          </Button>
        </div>
        <div className="flex min-h-0 flex-1 flex-col gap-3 lg:flex-row">
          <div className="min-h-0 flex-1 overflow-hidden rounded-lg border border-border bg-card">
            <DelegationGraph runs={nodes} rootId={sel} onSelect={setPicked} selectedId={picked ?? undefined} />
          </div>
          {pickedNode && (
            <aside className="min-h-0 overflow-auto rounded-lg border border-border bg-card p-3 lg:w-96 lg:shrink-0">
              <div className="mb-2 flex items-start gap-2">
                <div className="min-w-0 flex-1">
                  <div className="text-[10px] font-semibold uppercase tracking-wider text-muted">
                    {pickedNode.root ? "lead" : `sub-agent · L${pickedNode.depth}`}
                  </div>
                  <div className="truncate text-xs font-medium" title={pickedNode.intent || pickedNode.id}>
                    {clip(pickedNode.intent || pickedNode.id, 80)}
                  </div>
                </div>
                <button
                  onClick={() => setPicked(null)}
                  className="shrink-0 rounded-md border border-border p-1 text-muted hover:border-accent hover:text-foreground"
                  title="Close"
                >
                  <X className="size-3.5" />
                </button>
              </div>
              <RunDetailLoader correlationId={pickedNode.id} status={pickedNode.status} />
            </aside>
          )}
        </div>
      </div>
    );
  }

  // ───────────────────────── Live tab: run gallery ─────────────────────────
  if (tab === "live") {
    return (
      <div className="space-y-3">
        {header}

        {runs && (
          <div className="grid grid-cols-2 gap-2 sm:grid-cols-3 lg:grid-cols-5">
            <BigStat icon={Network} label="leads" value={roots.length} />
            <BigStat icon={RefreshCw} label="running" value={running} accent={running > 0} />
            <BigStat icon={GitBranch} label="sub-agents" value={totalSubs} />
            <BigStat icon={Bot} label="roster" value={profiles.length || "—"} />
            <BigStat icon={Coins} label="spend" value={money(totalSpend)} />
          </div>
        )}

        {runs && roots.length > 0 && (
          <div className="flex flex-wrap items-center gap-1.5">
            {(["all", "running", "done", "failed"] as Filter[]).map((f) => {
              const n = f === "all" ? roots.length : roots.filter((r) => r.kind === f).length;
              return (
                <button
                  key={f}
                  onClick={() => setFilter(f)}
                  className={cn(
                    "inline-flex items-center gap-1.5 rounded-full border px-2.5 py-1 text-xs capitalize transition-colors",
                    filter === f ? "border-accent bg-accent/10 text-accent" : "border-border text-muted hover:border-accent",
                  )}
                >
                  {f}
                  <span className="rounded-full bg-card px-1.5 text-[10px] tabular-nums">{n}</span>
                </button>
              );
            })}
          </div>
        )}

        {err ? (
          <ErrorText>{err}</ErrorText>
        ) : !runs ? (
          <SkeletonList count={3} lines={2} />
        ) : roots.length === 0 ? (
          <EmptyState
            icon={Network}
            title="No agent runs right now"
            hint="Nothing is executing — switch to the Fleet tab to see every agent you have and how each one gets triggered."
            action={
              <Button variant="ghost" size="sm" onClick={() => setTab("fleet")}>
                <Radar className="size-3.5" /> View the fleet
              </Button>
            }
          />
        ) : shown.length === 0 ? (
          <EmptyState icon={Network} title={`No ${filter} runs`} hint="Try a different filter." />
        ) : (
          <div className="grid gap-2.5 sm:grid-cols-2 xl:grid-cols-3">
            {shown.map((r) => (
              <RunCard
                key={r.id}
                r={r}
                onOpen={() => {
                  setSel(r.id);
                  setPicked(null);
                }}
              />
            ))}
          </div>
        )}
      </div>
    );
  }

  // ───────────────────────── Fleet tab: the census ─────────────────────────
  return (
    <div className="space-y-3">
      {header}

      {/* Census band — what you own, at a glance, even at rest. */}
      <div className="grid grid-cols-3 gap-2 sm:grid-cols-4 lg:grid-cols-6">
        <BigStat icon={Users} label="roster" value={census.roster} />
        <BigStat icon={Anchor} label="standing" value={census.standing} />
        <BigStat icon={CalendarClock} label="schedules" value={census.schedule} />
        <BigStat icon={GitFork} label="workflows" value={census.workflow} />
        <BigStat icon={Cpu} label="system" value={census.system} />
        <BigStat icon={RefreshCw} label="running" value={census.running} accent={census.running > 0} />
      </div>

      {/* Kind filters + search. */}
      <div className="flex flex-wrap items-center gap-1.5">
        {FLEET_FILTERS.map((f) => {
          const n =
            f.id === "all"
              ? fleet.length
              : f.id === "running"
                ? census.running
                : fleet.filter((e) => e.kind === f.id).length;
          return (
            <button
              key={f.id}
              onClick={() => setFleetFilter(f.id)}
              className={cn(
                "inline-flex items-center gap-1.5 rounded-full border px-2.5 py-1 text-xs transition-colors",
                fleetFilter === f.id ? "border-accent bg-accent/10 text-accent" : "border-border text-muted hover:border-accent",
              )}
            >
              <f.icon className="size-3" />
              {f.label}
              <span className="rounded-full bg-card px-1.5 text-[10px] tabular-nums">{n}</span>
            </button>
          );
        })}
        <div className="relative ml-auto">
          <Search className="pointer-events-none absolute left-2 top-1/2 size-3 -translate-y-1/2 text-muted" />
          <input
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="Search agents…"
            className="w-44 rounded-full border border-border bg-card py-1 pl-7 pr-2 text-xs outline-none focus:border-accent"
          />
        </div>
      </div>

      {err && !catalogLoaded ? (
        <ErrorText>{err}</ErrorText>
      ) : !catalogLoaded ? (
        <SkeletonList count={4} lines={2} />
      ) : fleet.length === 0 ? (
        <EmptyState
          icon={Network}
          title="No agents yet"
          hint="Create a roster agent, a standing order, a schedule, or a workflow — each appears here with how it gets triggered."
        />
      ) : (
        <div className="flex min-h-0 flex-col gap-3 lg:flex-row">
          <div
            className={cn(
              "grid min-w-0 flex-1 gap-2.5",
              selEntity ? "sm:grid-cols-1 xl:grid-cols-2" : "sm:grid-cols-2 xl:grid-cols-3",
            )}
          >
            {shownFleet.length === 0 ? (
              <EmptyState icon={Search} title="No matches" hint="Try a different filter or search term." />
            ) : (
              shownFleet.map((e) => <FleetCard key={e.key} e={e} onOpen={() => setSelKey(e.key)} />)
            )}
          </div>
          {selEntity && (
            <FleetDetail
              e={selEntity}
              onClose={() => setSelKey("")}
              onManage={manage}
              onLive={selEntity.kind === "roster" ? () => openLiveFor(selEntity.slug) : undefined}
            />
          )}
        </div>
      )}
    </div>
  );
}

// RunCard is one lead run in the gallery — everything about it at a glance.
function RunCard({ r, onOpen }: { r: RootSummary; onOpen: () => void }) {
  return (
    <button
      onClick={onOpen}
      className="group flex flex-col gap-2 rounded-lg border border-border bg-card p-3 text-left transition-colors hover:border-accent"
    >
      <div className="flex items-center gap-2">
        <span
          className={cn("size-2 shrink-0 rounded-full", KIND_DOT[r.kind], r.kind === "running" && "animate-pulse")}
        />
        <span className="text-[10px] font-semibold uppercase tracking-wider text-muted">{r.status || "run"}</span>
        {r.agentName && (
          <span className="inline-flex items-center gap-1 rounded-full bg-panel py-0.5 pl-0.5 pr-1.5 text-[10px] text-accent">
            <AgentAvatar slug={r.agentName} size={14} status={r.kind === "running" ? "running" : undefined} /> {r.agentName}
          </span>
        )}
        {r.startedMs ? (
          <span className="ml-auto inline-flex items-center gap-1 text-[10px] text-muted">
            <Clock className="size-2.5" /> {fmtTime(r.startedMs)}
          </span>
        ) : null}
      </div>

      <div className="line-clamp-2 min-h-[2.4em] text-xs font-medium text-foreground/90" title={r.intent || r.id}>
        {r.intent ? clip(r.intent, 160) : <span className="font-mono text-muted">{clip(r.id, 40)}</span>}
      </div>

      {r.answerPreview && (
        <div className="line-clamp-2 rounded-md bg-panel/50 px-2 py-1 text-[11px] text-muted" title={r.answerPreview}>
          {clip(r.answerPreview, 160)}
        </div>
      )}

      <div className="flex flex-wrap items-center gap-1.5">
        <Chip icon={Bot} title="agents in this run's tree">
          {r.agents}
        </Chip>
        {r.subAgents > 0 && (
          <Chip icon={GitBranch} title="sub-agents">
            {r.subAgents}
          </Chip>
        )}
        {r.depth > 0 && (
          <Chip icon={Layers} title="delegation depth">
            {r.depth}
          </Chip>
        )}
        <Chip icon={Repeat} title="iterations">
          {r.iters}
        </Chip>
        <Chip icon={Coins} title="tree spend">
          {money(r.treeSpentMc)}
        </Chip>
      </div>

      {r.model && (
        <div className="truncate font-mono text-[10px] text-muted" title={r.model}>
          {r.model}
        </div>
      )}
    </button>
  );
}

function Chip({ icon: Icon, children, title }: { icon: typeof Bot; children: React.ReactNode; title?: string }) {
  return (
    <span
      title={title}
      className="inline-flex items-center gap-1 rounded-md border border-border bg-panel/50 px-1.5 py-0.5 text-[11px] tabular-nums text-foreground/80"
    >
      <Icon className="size-3 text-muted" /> {children}
    </span>
  );
}

function BigStat({
  icon: Icon,
  label,
  value,
  accent,
}: {
  icon: typeof Bot;
  label: string;
  value: number | string;
  accent?: boolean;
}) {
  return (
    <div className={cn("rounded-lg border bg-card p-2.5", accent ? "border-accent/50" : "border-border")}>
      <div className="flex items-center gap-1.5 text-[10px] font-semibold uppercase tracking-wider text-muted">
        <Icon className={cn("size-3", accent && "text-accent")} /> {label}
      </div>
      <div className={cn("mt-0.5 text-lg font-semibold tabular-nums", accent && "text-accent")}>{value}</div>
    </div>
  );
}

function Stat({ icon: Icon, label, value }: { icon: typeof Bot; label: string; value: number | string }) {
  return (
    <div className="inline-flex items-center gap-1.5 rounded-md border border-border bg-card px-2.5 py-1">
      <Icon className="size-3 text-muted" />
      <span className="text-muted">{label}</span>
      <span className="font-semibold tabular-nums">{value}</span>
    </div>
  );
}
