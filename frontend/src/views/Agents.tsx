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
  ShieldCheck,
  Skull,
  Search,
  Radar,
  UserCheck,
  Wrench,
  Radio,
  CheckCircle2,
  XCircle,
  AlertTriangle,
  Pause,
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
import { Badge, statusVariant } from "@/components/ui/badge";
import { RunDetailLoader } from "@/components/RunDetail";
import { AgentAvatar } from "@/components/AgentAvatar";
import { FleetCard, FleetDetail } from "@/components/Fleet";
import { AgentDetail } from "@/components/AgentDetail";
import { openAgent } from "@/lib/agentnav";
import { AnimatedNumber } from "@/components/AnimatedNumber";
import { PageHeader } from "@/components/ui/page-header";
import { TabNav } from "@/components/ui/tab-nav";
import { MetricWidget, MetricGrid } from "@/components/ui/metric-widget";
import type { AgentProfile } from "@/views/Roster";
import { buildDelegationTree, type RunNode } from "@/lib/delegation";
import {
  buildFleet,
  filterFleetEntities,
  fleetCensus,
  statusKind,
  type StatusKind,
  type FleetEntityFilter,
  type ApiProfile,
  type ApiOrder,
  type ApiSchedule,
  type ApiWorkflow,
  type ApiPulse,
} from "@/lib/fleet";
import { applyAgentLivePatches, reduceAgentLivePatchMap, shouldReloadAgentCatalog, type AgentLivePatchMap } from "@/lib/agentlive";

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
// agent field set — plain chat conversations (no roster agent) are excluded since
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
    // a structured roster agent binding (cron, continuous, event, or manual trigger).
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
type FleetFilter = FleetEntityFilter;

const FLEET_FILTERS: { id: FleetFilter; label: string; icon: typeof Bot }[] = [
  { id: "all", label: "All", icon: Network },
  { id: "guardians", label: "Guardians", icon: ShieldCheck },
  { id: "direct", label: "Direct", icon: UserCheck },
  { id: "subagents", label: "Sub-agents", icon: GitBranch },
  { id: "repair", label: "Repair", icon: Wrench },
  { id: "graveyard", label: "Inactive", icon: Skull },
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
  const { events, subscribe } = useEvents();
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
  const [livePatches, setLivePatches] = useState<AgentLivePatchMap>({});

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

  useEffect(() => {
    let t: ReturnType<typeof setTimeout> | undefined;
    const off = subscribe((ev) => {
      setLivePatches((prev) => reduceAgentLivePatchMap(prev, ev));
      if (!shouldReloadAgentCatalog(ev)) return;
      if (t) clearTimeout(t);
      t = setTimeout(() => void reloadCatalog(), 1200);
    });
    return () => {
      if (t) clearTimeout(t);
      off();
    };
  }, [subscribe]);

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
  const liveProfiles = useMemo(() => applyAgentLivePatches(profiles, livePatches), [profiles, livePatches]);
  const fleet = useMemo(
    () => buildFleet(liveProfiles, orders, schedules, workflows, runs || [], pulse),
    [liveProfiles, orders, schedules, workflows, runs, pulse],
  );
  const census = useMemo(() => fleetCensus(fleet), [fleet]);
  const shownFleet = useMemo(() => {
    return filterFleetEntities(fleet, fleetFilter, query);
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
    <PageHeader
      icon={Network}
      title="Agents"
      actions={
        <>
          <Button variant="ghost" size="sm" onClick={reloadAll} title="Reload">
            <RefreshCw className={cn("size-3.5", loading && "animate-spin")} />
          </Button>
        </>
      }
    />
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
          <div className="glass min-h-0 flex-1 overflow-hidden rounded-xl">
            <DelegationGraph runs={nodes} rootId={sel} onSelect={setPicked} selectedId={picked ?? undefined} />
          </div>
          {pickedNode && (
            <aside className="glass min-h-0 overflow-auto rounded-xl p-3 lg:w-96 lg:shrink-0">
              <div className="mb-2 flex items-start gap-2">
                <div className="min-w-0 flex-1">
                  <div className="text-xs font-semibold uppercase tracking-normal text-muted">
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
          <MetricGrid cols="repeat(auto-fill, minmax(140px, 1fr))">
            <MetricWidget icon={Network} label="Leads" value={roots.length} tone="muted" />
            <MetricWidget icon={Radio} label="Running" value={running} tone={running > 0 ? "accent" : "muted"} pulse={running > 0} />
            <MetricWidget icon={GitBranch} label="Sub-agents" value={totalSubs} tone="muted" />
            <MetricWidget icon={Bot} label="Roster" value={profiles.length || "—"} tone="muted" />
            <MetricWidget icon={Coins} label="Spend" value={money(totalSpend)} tone="muted" />
          </MetricGrid>
        )}

        {runs && roots.length > 0 && (
          <TabNav
            tabs={[
              {
                id: "all",
                label: "All",
                icon: Network,
                count: roots.length,
                content: roots.length === 0 ? (
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
                ) : (
                  <div className="grid gap-2.5 sm:grid-cols-2 xl:grid-cols-3">
                    {roots.map((r) => (
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
                ),
              },
              {
                id: "running",
                label: "Running",
                icon: Radio,
                count: roots.filter((r) => r.kind === "running").length,
                content: (() => {
                  const runningRoots = roots.filter((r) => r.kind === "running");
                  return runningRoots.length === 0 ? (
                    <EmptyState icon={Radio} title="No running agents" hint="No agents are currently executing." />
                  ) : (
                    <div className="grid gap-2.5 sm:grid-cols-2 xl:grid-cols-3">
                      {runningRoots.map((r) => (
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
                  );
                })(),
              },
              {
                id: "done",
                label: "Done",
                icon: CheckCircle2,
                count: roots.filter((r) => r.kind === "done").length,
                content: (() => {
                  const doneRoots = roots.filter((r) => r.kind === "done");
                  return doneRoots.length === 0 ? (
                    <EmptyState icon={CheckCircle2} title="No completed runs" hint="No runs have completed successfully." />
                  ) : (
                    <div className="grid gap-2.5 sm:grid-cols-2 xl:grid-cols-3">
                      {doneRoots.map((r) => (
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
                  );
                })(),
              },
              {
                id: "failed",
                label: "Failed",
                icon: XCircle,
                count: roots.filter((r) => r.kind === "failed").length,
                content: (() => {
                  const failedRoots = roots.filter((r) => r.kind === "failed");
                  return failedRoots.length === 0 ? (
                    <EmptyState icon={XCircle} title="No failed runs" hint="No runs have failed." />
                  ) : (
                    <div className="grid gap-2.5 sm:grid-cols-2 xl:grid-cols-3">
                      {failedRoots.map((r) => (
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
                  );
                })(),
              },
            ]}
            value={filter}
            onValueChange={(v) => setFilter(v as Filter)}
          />
        )}

        {err && <ErrorText>{err}</ErrorText>}
        {!runs && !err && <SkeletonList count={3} lines={2} />}
      </div>
    );
  }

  // ───────────────────────── Fleet tab: the census ─────────────────────────
  return (
    <div className="space-y-3">
      {header}

      {/* Census band — what you own, at a glance, even at rest. */}
      <MetricGrid cols="repeat(auto-fill, minmax(140px, 1fr))">
        <MetricWidget icon={Users} label="Roster" value={census.roster} tone="muted" />
        <MetricWidget icon={Anchor} label="Standing" value={census.standing} tone="muted" />
        <MetricWidget icon={CalendarClock} label="Schedules" value={census.schedule} tone="muted" />
        <MetricWidget icon={GitFork} label="Workflows" value={census.workflow} tone="muted" />
        <MetricWidget icon={Cpu} label="System" value={census.system} tone="muted" />
        <MetricWidget icon={Radio} label="Running" value={census.running} tone={census.running > 0 ? "accent" : "muted"} pulse={census.running > 0} />
        <MetricWidget icon={Skull} label="Inactive" value={census.graveyard} tone={census.graveyard > 0 ? "bad" : "muted"} />
      </MetricGrid>

      {/* Status filters + search. */}
      <div className="flex flex-wrap items-center gap-2">
        <TabNav
          tabs={[
            {
              id: "all",
              label: "All",
              icon: Network,
              count: fleet.length,
              content: null,
            },
            {
              id: "running",
              label: "Awake",
              icon: Radio,
              count: census.running,
              content: null,
            },
            {
              id: "repair",
              label: "Repair",
              icon: Wrench,
              count: census.repair,
              content: null,
            },
            {
              id: "graveyard",
              label: "Inactive",
              icon: Skull,
              count: census.graveyard,
              content: null,
            },
          ]}
          value={fleetFilter === "all" || fleetFilter === "running" || fleetFilter === "repair" || fleetFilter === "graveyard" ? fleetFilter : "all"}
          onValueChange={(v) => setFleetFilter(v as FleetFilter)}
        />
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
      ) : selEntity && selEntity.kind === "roster" ? (
        // Roster agents get the full-width Command Center deep panel (M953):
        // soul, triggers, activity, memory, skills, permissions/diagnostics,
        // files — everything about one agent in one place. Other kinds keep the
        // compact "how does this run?" aside below.
        <AgentDetail
          slug={selEntity.slug}
          profile={selEntity.raw as AgentProfile}
          runs={runs || []}
          orders={orders}
          triggers={selEntity.triggers}
          state={selEntity.state}
          schedules={schedules}
          onClose={() => setSelKey("")}
          onManage={manage}
          onLive={() => openLiveFor(selEntity.slug)}
          onChanged={reloadAll}
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
              // A roster agent (created or guardian) opens its full, deep-linkable
              // identity PAGE (#agent/<slug>) — what the owner expects when clicking
              // an agent. Automations (standing/schedule/workflow) have no page, so
              // they open the inline "how does this run?" panel.
              shownFleet.map((e) => (
                <FleetCard key={e.key} e={e} onOpen={() => (e.kind === "roster" ? openAgent(e.slug) : setSelKey(e.key))} onAction={reloadAll} />
              ))
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
      className="group flex flex-col gap-2 glass rounded-xl p-3 text-left transition-colors hover:border-accent"
    >
      <div className="flex items-center gap-2">
        <span
          className={cn("size-2 shrink-0 rounded-full", KIND_DOT[r.kind], r.kind === "running" && "animate-pulse")}
        />
        <Badge variant={statusVariant(r.status)} className="text-xs">
          {r.status || "run"}
        </Badge>
        {r.agentName && (
          <span className="inline-flex items-center gap-1 rounded-full bg-panel py-0.5 pl-0.5 pr-1.5 text-xs text-accent">
            <AgentAvatar slug={r.agentName} size={14} status={r.kind === "running" ? "running" : undefined} /> {r.agentName}
          </span>
        )}
        {r.startedMs ? (
          <span className="ml-auto inline-flex items-center gap-1 text-xs text-muted">
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
        <div className="truncate font-mono text-xs text-muted" title={r.model}>
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
    <div className={cn("glass rounded-xl p-2.5 transition-shadow hover:shadow-e3", accent && "glow-accent")}>
      <div className="flex items-center gap-1.5 text-xs font-semibold uppercase tracking-normal text-muted">
        <Icon className={cn("size-3", accent && "text-accent")} /> {label}
      </div>
      <div className={cn("mt-0.5 text-2xl font-bold tabular-nums", accent ? "text-gradient" : "text-foreground")}>
        {typeof value === "number" ? <AnimatedNumber value={value} /> : value}
      </div>
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
