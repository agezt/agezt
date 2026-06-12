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
import { buildDelegationTree, type RunNode } from "@/lib/delegation";

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

// statusKind normalizes the daemon's run statuses into the four buckets the
// gallery colors + filters by. Pure + unit-tested.
export type StatusKind = "running" | "done" | "failed" | "other";
export function statusKind(status?: string): StatusKind {
  const s = (status || "").toLowerCase();
  if (s === "running" || s === "active" || s === "in_progress") return "running";
  if (s === "completed" || s === "done" || s === "succeeded" || s === "ok") return "done";
  if (s === "failed" || s === "error" || s === "cancelled" || s === "canceled" || s === "halted") return "failed";
  return "other";
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
// sorting running-first then most-recent. Pure, so the whole gallery model is
// unit-testable without the network.
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
    const tree = buildDelegationTree(nodes, n.id);
    out.push({
      id: n.id,
      intent: n.intent,
      status: n.status,
      kind: statusKind(n.status),
      model: n.model,
      agentName: agentById.get(n.id) || undefined,
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

// Agents is the multi-agent monitor — a visual gallery of every lead run and its
// sub-agent fleet: status, model, identity, sub-agent count, depth, iterations
// and spend all visible at a glance, no dropdown. Click a card to drill into its
// live delegation graph + per-agent detail.
export function Agents() {
  const { events } = useEvents();
  const [runs, setRuns] = useState<ApiRun[] | null>(null);
  const [rosterCount, setRosterCount] = useState<number | null>(null);
  const [filter, setFilter] = useState<Filter>("all");
  const [sel, setSel] = useState<string>(""); // the drilled-into lead run
  const [picked, setPicked] = useState<string | null>(null); // node in the graph
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
  useEffect(() => {
    reload();
    getJSON<{ profiles?: unknown[] }>("/api/agents")
      .then((d) => setRosterCount((d.profiles || []).length))
      .catch(() => setRosterCount(null));
    const id = setInterval(reload, 6000);
    return () => clearInterval(id);
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

  // Drill-down view: the selected lead's live delegation graph + detail.
  if (sel) {
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

  // Gallery view.
  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-center gap-2">
        <h2 className="flex items-center gap-2 text-sm font-semibold">
          <Network className="size-4 text-accent" /> Agents
        </h2>
        <Button variant="ghost" size="sm" onClick={reload} title="Reload" className="ml-auto">
          <RefreshCw className={cn("size-3.5", loading && "animate-spin")} />
        </Button>
      </div>

      {/* Summary band — the fleet at a glance. */}
      {runs && (
        <div className="grid grid-cols-2 gap-2 sm:grid-cols-3 lg:grid-cols-5">
          <BigStat icon={Network} label="leads" value={roots.length} />
          <BigStat icon={RefreshCw} label="running" value={running} accent={running > 0} />
          <BigStat icon={GitBranch} label="sub-agents" value={totalSubs} />
          <BigStat icon={Bot} label="roster" value={rosterCount ?? "—"} />
          <BigStat icon={Coins} label="spend" value={money(totalSpend)} />
        </div>
      )}

      {/* Filter chips replace the dropdown. */}
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
          title="No agent runs yet"
          hint="Start a run from Chat or the CLI — each lead run and its sub-agent fleet appears here as a live card."
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
          <span className="inline-flex items-center gap-1 rounded-full bg-panel px-1.5 py-0.5 text-[10px] text-accent">
            <Bot className="size-2.5" /> {r.agentName}
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
