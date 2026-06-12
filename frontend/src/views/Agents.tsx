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
  Shield,
  CalendarClock,
  Zap,
  Timer,
  Moon,
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

// ───────────────────────── Standing army (M930) ─────────────────────────
// The runs gallery shows agents that ARE running; the army map shows the force
// that is waiting — every roster agent with what will wake it (standing-order
// cron/event triggers, schedules that run as it) and when it last marched.

interface ApiProfile {
  slug: string;
  name?: string;
  model?: string;
  enabled: boolean;
  retired?: boolean;
}

interface ApiTrigger {
  type?: string;
  schedule?: string;
  subject?: string;
}

interface ApiOrder {
  id: string;
  name?: string;
  enabled: boolean;
  agent?: string;
  triggers?: ApiTrigger[];
}

interface ApiSchedule {
  id: string;
  intent?: string;
  cadence?: string;
  enabled: boolean;
}

export interface WakeSource {
  type: "cron" | "event" | "schedule";
  detail: string;
  via: string;
}

export interface ArmyAgent {
  slug: string;
  name: string;
  model?: string;
  enabled: boolean;
  running: boolean;
  lastRunMs?: number;
  wake: WakeSource[];
}

// scheduleAgentSlug extracts the roster slug a cadence intent runs as: cadence
// entries have no agent field, so "--agent <slug>" inside the intent is the
// binding (mirrors how the daemon resolves it at fire time).
export function scheduleAgentSlug(intent?: string): string {
  const m = (intent || "").match(/--agent[= ]+([\w.-]+)/);
  return m ? m[1] : "";
}

// buildArmy folds roster + standing orders + schedules + recent runs into the
// army map. Retired agents stay in the graveyard, not the ranks. Sorted:
// running first, then trigger-armed, then alphabetical. Pure + unit-tested.
export function buildArmy(
  profiles: ApiProfile[],
  orders: ApiOrder[],
  schedules: ApiSchedule[],
  runs: ApiRun[],
): ArmyAgent[] {
  const out: ArmyAgent[] = [];
  for (const p of profiles) {
    if (p.retired) continue;
    const wake: WakeSource[] = [];
    for (const o of orders) {
      if (!o.enabled || o.agent !== p.slug) continue;
      for (const t of o.triggers || []) {
        if (t.type === "cron" && t.schedule) wake.push({ type: "cron", detail: t.schedule, via: o.name || o.id });
        else if (t.type === "event" && t.subject) wake.push({ type: "event", detail: t.subject, via: o.name || o.id });
      }
    }
    for (const sch of schedules) {
      if (sch.enabled && scheduleAgentSlug(sch.intent) === p.slug)
        wake.push({ type: "schedule", detail: sch.cadence || sch.id, via: sch.id });
    }
    let running = false;
    let lastRunMs: number | undefined;
    for (const r of runs) {
      if ((r.agent || "") !== p.slug) continue;
      if (statusKind(r.status) === "running") running = true;
      if ((r.started_unix_ms || 0) > (lastRunMs || 0)) lastRunMs = r.started_unix_ms;
    }
    out.push({ slug: p.slug, name: p.name || p.slug, model: p.model, enabled: p.enabled, running, lastRunMs, wake });
  }
  out.sort((a, b) => {
    if (a.running !== b.running) return a.running ? -1 : 1;
    if (a.wake.length > 0 !== (b.wake.length > 0)) return a.wake.length > 0 ? -1 : 1;
    return a.slug.localeCompare(b.slug);
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
  const [profiles, setProfiles] = useState<ApiProfile[]>([]);
  const [orders, setOrders] = useState<ApiOrder[]>([]);
  const [schedules, setSchedules] = useState<ApiSchedule[]>([]);
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
  // The army inputs change rarely (operator edits), so they load once + on the
  // slower poll, while runs stay on the fast 6s cycle.
  async function reloadArmy() {
    try {
      const [a, o, s] = await Promise.all([
        getJSON<{ profiles?: ApiProfile[] }>("/api/agents"),
        getJSON<{ orders?: ApiOrder[] }>("/api/standing"),
        getJSON<{ schedules?: ApiSchedule[] }>("/api/schedules"),
      ]);
      setProfiles(a.profiles || []);
      setOrders(o.orders || []);
      setSchedules(s.schedules || []);
    } catch {
      /* army panel is best-effort; the runs gallery still works */
    }
  }

  useEffect(() => {
    reload();
    reloadArmy();
    const id = setInterval(reload, 6000);
    const armyId = setInterval(reloadArmy, 30000);
    return () => {
      clearInterval(id);
      clearInterval(armyId);
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
  const army = useMemo(() => buildArmy(profiles, orders, schedules, runs || []), [profiles, orders, schedules, runs]);
  const armed = army.filter((a) => a.wake.length > 0).length;

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
          <BigStat icon={Bot} label="roster" value={profiles.length || "—"} />
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

      {/* Standing army (M930): the force that is waiting, and what wakes it. */}
      {army.length > 0 && (
        <div>
          <div className="mb-1.5 mt-1 flex items-center gap-1.5 text-[11px] font-semibold uppercase tracking-wide text-muted">
            <Shield className="size-3" /> Standing army ({army.length})
            <span className="font-normal normal-case tracking-normal">
              — {armed} trigger-armed, {army.length - armed} on call
            </span>
          </div>
          <div className="grid gap-2 sm:grid-cols-2 xl:grid-cols-3">
            {army.map((a) => (
              <ArmyCard key={a.slug} a={a} />
            ))}
          </div>
        </div>
      )}
    </div>
  );
}

const WAKE_ICON: Record<WakeSource["type"], typeof Bot> = {
  cron: CalendarClock,
  event: Zap,
  schedule: Timer,
};

// ArmyCard — one waiting soldier: identity, state (marching / armed / on call /
// paused) and the exact tripwires that will wake it.
function ArmyCard({ a }: { a: ArmyAgent }) {
  return (
    <div
      className={cn(
        "flex flex-col gap-1.5 rounded-lg border bg-card p-2.5",
        a.running ? "border-accent/60" : "border-border",
        !a.enabled && "opacity-60",
      )}
    >
      <div className="flex items-center gap-2">
        <span
          className={cn(
            "size-2 shrink-0 rounded-full",
            a.running ? "animate-pulse bg-accent" : !a.enabled ? "bg-muted" : a.wake.length > 0 ? "bg-good" : "bg-muted",
          )}
        />
        <span className="truncate text-xs font-semibold">{a.name}</span>
        <span className="truncate font-mono text-[10px] text-muted">{a.slug}</span>
        <span className="ml-auto shrink-0 text-[10px] uppercase tracking-wider text-muted">
          {a.running ? (
            <span className="text-accent">running</span>
          ) : !a.enabled ? (
            "paused"
          ) : a.wake.length > 0 ? (
            "armed"
          ) : (
            "on call"
          )}
        </span>
      </div>
      {a.wake.length > 0 ? (
        <div className="flex flex-wrap gap-1">
          {a.wake.map((w, i) => {
            const Icon = WAKE_ICON[w.type];
            return (
              <span
                key={`${w.type}-${w.detail}-${i}`}
                title={`${w.type} · via ${w.via}`}
                className="inline-flex max-w-full items-center gap-1 rounded-md border border-border bg-panel/50 px-1.5 py-0.5 text-[10px] text-foreground/80"
              >
                <Icon className="size-2.5 shrink-0 text-muted" />
                <span className="truncate font-mono">{w.detail}</span>
              </span>
            );
          })}
        </div>
      ) : (
        <div className="flex items-center gap-1 text-[10px] text-muted">
          <Moon className="size-2.5" /> wakes only by delegation or a direct run
        </div>
      )}
      <div className="flex items-center gap-2 text-[10px] text-muted">
        {a.model && (
          <span className="truncate font-mono" title={a.model}>
            {a.model}
          </span>
        )}
        <span className="ml-auto inline-flex shrink-0 items-center gap-1">
          <Clock className="size-2.5" /> {a.lastRunMs ? fmtTime(a.lastRunMs) : "never marched"}
        </span>
      </div>
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
