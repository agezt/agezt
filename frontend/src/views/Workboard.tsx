import { useEffect, useMemo, useState, type ReactNode } from "react";
import {
  Columns3,
  RefreshCw,
  CheckCircle2,
  CircleDot,
  CircleStop,
  CornerDownRight,
  GitBranch,
  Link2,
  MessageSquare,
  Play,
  Plus,
  RotateCcw,
  ShieldAlert,
  ShieldCheck,
  BadgeCheck,
  XCircle,
  Armchair,
  User,
  type LucideIcon,
} from "lucide-react";
import { getJSON, postJSON } from "@/lib/api";
import { cn, fmtTime } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Page } from "@/components/ui/page";
import { ErrorText, Muted } from "@/components/JsonView";
import { SkeletonList } from "@/components/ui/skeleton";
import { EmptyState } from "@/components/ui/empty";

interface WorkboardClaim {
  agent?: string;
  run_id?: string;
  heartbeat_ms?: number;
}

interface WorkboardRetryPolicy {
  max_attempts?: number;
  escalate_to?: string;
}

interface WorkboardAttempt {
  id?: string;
  agent?: string;
  run_id?: string;
  status?: string;
  started_ms?: number;
  finished_ms?: number;
  summary?: string;
}

interface WorkboardComment {
  id?: string;
  author?: string;
  body?: string;
  created_ms?: number;
}

interface WorkboardLink {
  id?: string;
  type?: string;
  target?: string;
  created_ms?: number;
}

interface WorkboardDependency {
  id?: string;
  created_ms?: number;
}

interface WorkboardDependencyState {
  id?: string;
  title?: string;
  status?: string;
  missing?: boolean;
  created_ms?: number;
}

interface WorkboardTask {
  id: string;
  title?: string;
  description?: string;
  status?: string;
  priority?: number;
  tenant?: string;
  assignee?: string;
  owner?: string;
  tags?: string[];
  artifacts?: string[];
  retry_policy?: WorkboardRetryPolicy;
  claim?: WorkboardClaim;
  dependencies?: WorkboardDependency[];
  attempts?: WorkboardAttempt[];
  comments?: WorkboardComment[];
  links?: WorkboardLink[];
  block_reason?: string;
  criteria?: WorkboardCriterion[];
  proof?: WorkboardProof;
  criteria_count?: number;
  criteria_met?: number;
  gated?: boolean;
  proven?: boolean;
  seat?: string;
  attempt_count?: number;
  failed_attempt_count?: number;
  max_attempts?: number;
  next_attempt?: number;
  created_ms?: number;
  updated_ms?: number;
  completed_ms?: number;
  archived_ms?: number;
}

interface WorkboardCriterion {
  text?: string;
  met?: boolean;
  note?: string;
}

interface WorkboardProof {
  verdict?: { complete?: boolean; gap?: string };
  criteria?: WorkboardCriterion[];
  evidence?: { corr?: string; artifacts?: string[]; journal_from?: number; journal_to?: number };
  attempts?: number;
  judge?: string;
  proved_ms?: number;
}

interface WorkboardSeat {
  id: string;
  name?: string;
  description?: string;
  execution_profile?: string;
}

interface WorkboardLane {
  assignee?: string;
  label?: string;
  counts?: Record<string, number>;
  tasks?: WorkboardTask[];
  count?: number;
}

interface WorkboardLanesData {
  lanes?: WorkboardLane[];
  count?: number;
  task_count?: number;
}

interface WorkboardEvent {
  seq?: number;
  ts_unix_ms?: number;
  kind?: string;
  subject?: string;
  correlation_id?: string;
  payload?: Record<string, unknown>;
}

interface WorkboardWatchData {
  task?: WorkboardTask;
  events?: WorkboardEvent[];
  count?: number;
  run_id?: string;
  blocked_dependencies?: WorkboardDependencyState[];
}

const STATUS_ORDER = ["triage", "todo", "ready", "running", "blocked", "review", "done", "archived"];

// LANE_CARD_WINDOW caps how many task cards each lane renders at once. The
// lanes fetch is bounded at 500, but rendering every card balloons the DOM —
// each lane windows its own cards and grows via a compact per-lane Load-more
// button. Lane headers and status chips keep the FULL counts.
const LANE_CARD_WINDOW = 30;

export function workboardTaskFromHash(hash: string): string {
  const raw = hash.replace(/^#\/?/, "");
  const [, query = ""] = raw.split("?");
  return query ? new URLSearchParams(query).get("task")?.trim() || "" : "";
}

export function workboardHashForTask(id: string): string {
  const ref = id.trim();
  return ref ? `#workboard?task=${encodeURIComponent(ref)}` : "#workboard";
}

export function workboardStatusCounts(lanes: WorkboardLane[] = []): Record<string, number> {
  const out: Record<string, number> = {};
  for (const lane of lanes) {
    for (const [status, count] of Object.entries(lane.counts || {})) {
      out[status] = (out[status] || 0) + count;
    }
  }
  return out;
}

export function workboardOpenCount(lanes: WorkboardLane[] = []): number {
  const counts = workboardStatusCounts(lanes);
  return Object.entries(counts).reduce((sum, [status, count]) => (
    status === "done" || status === "archived" ? sum : sum + count
  ), 0);
}

export function workboardRetryText(task?: WorkboardTask | null): string {
  if (!task) return "";
  const max = task.max_attempts || task.retry_policy?.max_attempts || 0;
  const failed = task.failed_attempt_count || 0;
  if (max <= 0 && failed <= 0) return "";
  const parts = [`${failed}/${max || "?"} failed`];
  if (task.next_attempt) parts.push(`next ${task.next_attempt}`);
  if (task.retry_policy?.escalate_to) parts.push(`escalate ${task.retry_policy.escalate_to}`);
  return parts.join(" · ");
}

export function Workboard() {
  const [lanes, setLanes] = useState<WorkboardLane[]>([]);
  const [selectedID, setSelectedID] = useState(() => workboardTaskFromHash(location.hash));
  const [watch, setWatch] = useState<WorkboardWatchData | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [busy, setBusy] = useState(false);
  const [actor, setActor] = useState("operator");
  const [comment, setComment] = useState("");
  const [blockReason, setBlockReason] = useState("");
  const [failReason, setFailReason] = useState("");
  const [policyMax, setPolicyMax] = useState("2");
  const [policyEscalate, setPolicyEscalate] = useState("");
  const [dispatchIntent, setDispatchIntent] = useState("");
  const [seats, setSeats] = useState<WorkboardSeat[]>([]);
  const [showCreate, setShowCreate] = useState(false);
  const [newTitle, setNewTitle] = useState("");
  const [newDesc, setNewDesc] = useState("");
  const [newCriteria, setNewCriteria] = useState("");
  const [newSeat, setNewSeat] = useState("");

  async function createTask() {
    const title = newTitle.trim();
    if (!title) return;
    const criteria = newCriteria.split("\n").map((s) => s.trim()).filter(Boolean);
    setBusy(true);
    try {
      await postJSON("/api/workboard/create", {
        title,
        description: newDesc.trim(),
        criteria,
        seat: newSeat,
      });
      setNewTitle("");
      setNewDesc("");
      setNewCriteria("");
      setNewSeat("");
      setShowCreate(false);
      await reload();
      setErr(null);
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setBusy(false);
    }
  }

  useEffect(() => {
    getJSON<{ seats?: WorkboardSeat[] }>("/api/seats")
      .then((d) => setSeats(Array.isArray(d.seats) ? d.seats : []))
      .catch(() => {});
  }, []);

  const tasks = useMemo(() => lanes.flatMap((lane) => lane.tasks || []), [lanes]);
  const selectedFromLanes = useMemo(() => tasks.find((task) => task.id === selectedID) || null, [selectedID, tasks]);
  const task = watch?.task || selectedFromLanes;
  const counts = useMemo(() => workboardStatusCounts(lanes), [lanes]);
  const openCount = useMemo(() => workboardOpenCount(lanes), [lanes]);

  async function reload(nextSelected = selectedID) {
    setLoading(true);
    try {
      const d = await getJSON<WorkboardLanesData>("/api/workboard/lanes", { limit: "500" });
      const nextLanes = Array.isArray(d.lanes) ? d.lanes : [];
      setLanes(nextLanes);
      const flat = nextLanes.flatMap((lane) => lane.tasks || []);
      const target = nextSelected && flat.some((row) => row.id === nextSelected) ? nextSelected : flat[0]?.id || "";
      if (target !== selectedID) {
        setSelectedID(target);
      }
      setErr(null);
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setLoading(false);
    }
  }

  async function loadWatch(id: string) {
    if (!id) {
      setWatch(null);
      return;
    }
    try {
      const d = await getJSON<WorkboardWatchData>("/api/workboard/watch", { id, limit: "80" });
      setWatch(d);
      setErr(null);
    } catch (e) {
      setErr((e as Error).message);
    }
  }

  function selectTask(id: string) {
    setSelectedID(id);
    setWatch(null);
    const nextHash = workboardHashForTask(id);
    if (location.hash !== nextHash) history.replaceState(null, "", nextHash);
  }

  async function mutate(path: string, body: Record<string, unknown>, after?: () => void) {
    if (!task?.id || busy) return;
    setBusy(true);
    try {
      await postJSON(path, { id: task.id, actor: actor.trim() || "operator", author: actor.trim() || "operator", ...body });
      after?.();
      await reload(task.id);
      await loadWatch(task.id);
      setErr(null);
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setBusy(false);
    }
  }

  useEffect(() => {
    reload();
    const id = setInterval(() => reload(selectedID), 8000);
    return () => clearInterval(id);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    void loadWatch(selectedID);
    if (selectedID && location.hash !== workboardHashForTask(selectedID)) history.replaceState(null, "", workboardHashForTask(selectedID));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [selectedID]);

  return (
    <Page
      icon={Columns3}
      title="Workboard"
      description="Durable task lanes."
      mode="fill"
      width="full"
      actions={
        <div className="flex items-center gap-1.5">
          <Button size="sm" onClick={() => setShowCreate((v) => !v)} title="Create a task">
            <Plus className="size-3.5" /> New task
          </Button>
          <Button variant="ghost" size="sm" onClick={() => reload(selectedID)} disabled={loading} title="Reload">
            <RefreshCw className={cn("size-3.5", loading && "animate-spin")} />
          </Button>
        </div>
      }
    >
      {showCreate && (
        <form
          className="space-y-2 rounded-lg border border-accent/40 bg-card/70 p-3"
          onSubmit={(e) => {
            e.preventDefault();
            void createTask();
          }}
        >
          <div className="flex flex-wrap gap-2">
            <input value={newTitle} onChange={(e) => setNewTitle(e.target.value)} aria-label="Task title" placeholder="Task title" className="h-8 min-w-0 flex-1 rounded-md border border-border bg-panel px-2 text-sm outline-none focus-visible:border-accent" />
            <select value={newSeat} onChange={(e) => setNewSeat(e.target.value)} aria-label="Task seat" className="h-8 rounded-md border border-border bg-panel px-2 text-xs outline-none focus-visible:border-accent">
              {(seats.length ? seats : [{ id: "default", name: "Default" } as WorkboardSeat]).map((s) => (
                <option key={s.id} value={s.id === "default" ? "" : s.id}>seat: {s.name || s.id}</option>
              ))}
            </select>
          </div>
          <input value={newDesc} onChange={(e) => setNewDesc(e.target.value)} aria-label="Task description" placeholder="Description (optional)" className="h-8 w-full rounded-md border border-border bg-panel px-2 text-sm outline-none focus-visible:border-accent" />
          <textarea value={newCriteria} onChange={(e) => setNewCriteria(e.target.value)} aria-label="Acceptance criteria" rows={2} placeholder="Acceptance criteria — one per line (declaring any gates the task on proof)" className="w-full resize-y rounded-md border border-border bg-panel px-2 py-1.5 text-xs outline-none focus-visible:border-accent" />
          <div className="flex justify-end gap-1.5">
            <Button type="button" size="sm" variant="ghost" onClick={() => setShowCreate(false)}>Cancel</Button>
            <Button type="submit" size="sm" disabled={busy || !newTitle.trim()}><Plus className="size-3.5" /> Create</Button>
          </div>
        </form>
      )}

      <section className="grid gap-2 md:grid-cols-4">
        <Metric label="Open" value={openCount} tone="accent" />
        <Metric label="Running" value={counts.running || 0} tone="accent" />
        <Metric label="Blocked" value={counts.blocked || 0} tone="warn" />
        <Metric label="Review" value={counts.review || 0} tone="good" />
      </section>

      {err && <ErrorText>{err}</ErrorText>}

      {loading && lanes.length === 0 ? (
        <SkeletonList count={4} lines={2} />
      ) : tasks.length === 0 ? (
        <EmptyState icon={Columns3} title="No workboard tasks" hint="Durable tasks created by agents, workflows, or the CLI will appear here." />
      ) : (
        <div className="grid min-h-0 flex-1 gap-3 lg:grid-cols-[minmax(280px,360px)_1fr]">
          <section className="min-h-0 overflow-auto rounded-lg border border-border bg-card/75 p-2">
            <div className="mb-2 flex items-center justify-between gap-2 px-1">
              <div className="text-xs font-semibold uppercase tracking-normal text-muted">Lanes</div>
              <Badge>{tasks.length} tasks</Badge>
            </div>
            <div className="space-y-2">
              {lanes.map((lane) => (
                <LaneColumn key={lane.assignee || lane.label || "unassigned"} lane={lane} selectedID={selectedID} onSelect={selectTask} />
              ))}
            </div>
          </section>

          <section className="min-h-0 overflow-auto rounded-lg border border-border bg-card/75">
            {task ? (
              <TaskDetail
                task={task}
                watch={watch}
                busy={busy}
                actor={actor}
                setActor={setActor}
                comment={comment}
                setComment={setComment}
                blockReason={blockReason}
                setBlockReason={setBlockReason}
                failReason={failReason}
                setFailReason={setFailReason}
                policyMax={policyMax}
                setPolicyMax={setPolicyMax}
                policyEscalate={policyEscalate}
                setPolicyEscalate={setPolicyEscalate}
                dispatchIntent={dispatchIntent}
                setDispatchIntent={setDispatchIntent}
                seats={seats}
                mutate={mutate}
              />
            ) : (
              <EmptyState icon={Columns3} title="Select a task" hint="Pick a lane item to inspect attempts, dependencies, links, and events." />
            )}
          </section>
        </div>
      )}
    </Page>
  );
}

function Metric({ label, value, tone }: { label: string; value: number; tone: "accent" | "warn" | "good" }) {
  return (
    <div className={cn(
      "rounded-lg border bg-card/80 px-3 py-2",
      tone === "warn" ? "border-warn/35" : tone === "good" ? "border-good/35" : "border-accent/30",
    )}>
      <div className="text-[11px] font-semibold uppercase tracking-normal text-muted">{label}</div>
      <div className={cn("mt-0.5 text-2xl font-semibold tabular-nums", tone === "warn" ? "text-warn" : tone === "good" ? "text-good" : "text-accent")}>{value}</div>
    </div>
  );
}

function LaneColumn({ lane, selectedID, onSelect }: { lane: WorkboardLane; selectedID: string; onSelect: (id: string) => void }) {
  const laneTasks = lane.tasks || [];
  // Per-lane render window (consistent with LoadMoreFooter, but compact so it
  // fits inside a lane). The lane header Badge keeps the FULL count.
  const [win, setWin] = useState(LANE_CARD_WINDOW);
  const shownTasks = laneTasks.slice(0, win);
  const hidden = laneTasks.length - shownTasks.length;
  return (
    <div className="rounded-md border border-border bg-panel/45 p-2">
      <div className="mb-1.5 flex min-w-0 items-center gap-2">
        <User className="size-3.5 shrink-0 text-muted" />
        <div className="min-w-0 flex-1 truncate text-xs font-semibold">{lane.label || lane.assignee || "unassigned"}</div>
        <Badge>{lane.count || laneTasks.length}</Badge>
      </div>
      <div className="mb-1.5 flex flex-wrap gap-1">
        {STATUS_ORDER.filter((status) => (lane.counts?.[status] || 0) > 0).map((status) => (
          <span key={status} className="inline-flex items-center gap-1 rounded-full border border-border bg-background/55 px-1.5 py-0.5 text-[10px] text-muted">
            <span className={cn("size-1.5 rounded-full", statusDot(status))} />
            {status} {lane.counts?.[status]}
          </span>
        ))}
      </div>
      <ul className="space-y-1">
        {shownTasks.map((task) => (
          <li key={task.id}>
            <button
              onClick={() => onSelect(task.id)}
              className={cn(
                "w-full rounded-md border px-2 py-1.5 text-left transition-colors",
                selectedID === task.id ? "border-accent bg-accent/10" : "border-border/70 bg-background/50 hover:border-accent/60",
              )}
            >
              <div className="flex min-w-0 items-center gap-1.5">
                <span className={cn("size-2 shrink-0 rounded-full", statusDot(task.status))} />
                <span className="min-w-0 flex-1 truncate text-xs font-medium">{task.title || task.id}</span>
                {task.gated ? (
                  task.proven ? (
                    <BadgeCheck className="size-3.5 shrink-0 text-emerald-500" aria-label="proven" />
                  ) : (
                    <ShieldCheck className="size-3.5 shrink-0 text-amber-500" aria-label={`${task.criteria_met ?? 0}/${task.criteria_count ?? 0} criteria met`} />
                  )
                ) : null}
                {task.priority ? <span className="font-mono text-[10px] text-muted">P{task.priority}</span> : null}
              </div>
              <div className="mt-0.5 flex min-w-0 items-center gap-1 text-[10px] text-muted">
                <span className="font-mono">{short(task.id)}</span>
                {task.seat && task.seat !== "default" && (
                  <span className="inline-flex items-center gap-0.5 rounded-full border border-border bg-background/55 px-1 py-0.5">
                    <Armchair className="size-2.5" />{task.seat}
                  </span>
                )}
                {workboardRetryText(task) && <span className="truncate">{workboardRetryText(task)}</span>}
              </div>
            </button>
          </li>
        ))}
      </ul>
      {hidden > 0 && (
        <button
          type="button"
          onClick={() => setWin((w) => w + LANE_CARD_WINDOW)}
          className="mt-1.5 w-full rounded-md border border-border bg-card px-2 py-1 text-[11px] text-muted transition-colors hover:border-accent hover:text-foreground"
        >
          Load {Math.min(LANE_CARD_WINDOW, hidden)} more ({hidden} hidden)
        </button>
      )}
    </div>
  );
}

function TaskDetail({
  task,
  watch,
  busy,
  actor,
  setActor,
  comment,
  setComment,
  blockReason,
  setBlockReason,
  failReason,
  setFailReason,
  policyMax,
  setPolicyMax,
  policyEscalate,
  setPolicyEscalate,
  dispatchIntent,
  setDispatchIntent,
  seats,
  mutate,
}: {
  task: WorkboardTask;
  watch: WorkboardWatchData | null;
  busy: boolean;
  actor: string;
  setActor: (v: string) => void;
  comment: string;
  setComment: (v: string) => void;
  blockReason: string;
  setBlockReason: (v: string) => void;
  failReason: string;
  setFailReason: (v: string) => void;
  policyMax: string;
  setPolicyMax: (v: string) => void;
  policyEscalate: string;
  setPolicyEscalate: (v: string) => void;
  dispatchIntent: string;
  setDispatchIntent: (v: string) => void;
  seats: WorkboardSeat[];
  mutate: (path: string, body: Record<string, unknown>, after?: () => void) => Promise<void>;
}) {
  const blockedDeps = watch?.blocked_dependencies || [];
  const events = watch?.events || [];
  const retry = workboardRetryText(task);
  return (
    <div className="space-y-3 p-3">
      <div className="flex flex-wrap items-start justify-between gap-2">
        <div className="min-w-0">
          <div className="mb-1 flex flex-wrap items-center gap-1.5">
            <Badge variant={statusVariant(task.status)}>{task.status || "?"}</Badge>
            {task.assignee && <Badge><User className="mr-1 size-3" />{task.assignee}</Badge>}
            {retry && <Badge variant="warn"><RotateCcw className="mr-1 size-3" />{retry}</Badge>}
          </div>
          <h2 className="truncate text-lg font-semibold">{task.title || task.id}</h2>
          <div className="mt-0.5 flex flex-wrap gap-x-3 gap-y-1 text-xs text-muted">
            <span className="font-mono">{task.id}</span>
            {task.tenant && <span>{task.tenant}</span>}
            {task.owner && <span>owner {task.owner}</span>}
            {task.updated_ms ? <span>updated {fmtTime(task.updated_ms)}</span> : null}
          </div>
        </div>
        <div className="flex flex-wrap gap-1.5">
          <Button size="sm" disabled={busy || task.status === "running"} onClick={() => mutate("/api/workboard/dispatch", { agent: task.assignee || "", intent: dispatchIntent, reason: "operator dispatch" })}>
            <Play className="size-3.5" /> Dispatch
          </Button>
          {task.gated && (
            <Button size="sm" variant="ghost" disabled={busy} title="Judge acceptance criteria against produced evidence" onClick={() => mutate("/api/workboard/prove", {})}>
              <ShieldCheck className="size-3.5" /> Prove
            </Button>
          )}
          <Button size="sm" variant="ghost" disabled={busy || task.status === "done" || (task.gated && !task.proven)} title={task.gated && !task.proven ? "Prove acceptance criteria before completing" : undefined} onClick={() => mutate("/api/workboard/complete", {})}>
            <CheckCircle2 className="size-3.5" /> Complete
          </Button>
          {task.status === "blocked" && (
            <Button size="sm" variant="ghost" disabled={busy} onClick={() => mutate("/api/workboard/unblock", {})}>
              <RotateCcw className="size-3.5" /> Unblock
            </Button>
          )}
        </div>
      </div>

      {task.description && <p className="rounded-md border border-border bg-background/45 p-2 text-sm text-foreground/90">{task.description}</p>}

      {task.criteria && task.criteria.length > 0 && <ProofPanel task={task} />}

      {seats.length > 0 && (
        <div className="flex flex-wrap items-center gap-2 rounded-md border border-border bg-background/45 p-2 text-xs">
          <span className="inline-flex items-center gap-1 font-semibold text-muted"><Armchair className="size-3.5" /> Execution seat</span>
          <select
            aria-label="Execution seat"
            value={task.seat || "default"}
            disabled={busy}
            onChange={(e) => mutate("/api/workboard/seat", { seat: e.target.value })}
            className="h-7 rounded-md border border-border bg-panel px-2 text-xs outline-none focus-visible:border-accent"
          >
            {seats.map((s) => (
              <option key={s.id} value={s.id}>
                {s.name || s.id}
                {s.execution_profile ? ` · ${s.execution_profile}` : ""}
              </option>
            ))}
          </select>
          <span className="min-w-0 flex-1 truncate text-muted">{seats.find((s) => s.id === (task.seat || "default"))?.description}</span>
        </div>
      )}

      <div className="grid gap-3 xl:grid-cols-[minmax(280px,360px)_1fr]">
        <section className="space-y-2">
          <PanelTitle icon={ShieldAlert} title="Actions" />
          <input value={actor} onChange={(e) => setActor(e.target.value)} aria-label="Actor" className="h-8 w-full rounded-md border border-border bg-panel px-2 text-xs outline-none focus-visible:border-accent" />
          <textarea value={dispatchIntent} onChange={(e) => setDispatchIntent(e.target.value)} aria-label="Dispatch intent" rows={2} placeholder="dispatch intent override" className="w-full resize-none rounded-md border border-border bg-panel px-2 py-1.5 text-xs outline-none focus-visible:border-accent" />
          <div className="grid grid-cols-[1fr_auto] gap-1.5">
            <input value={comment} onChange={(e) => setComment(e.target.value)} aria-label="Comment" placeholder="comment" className="h-8 rounded-md border border-border bg-panel px-2 text-xs outline-none focus-visible:border-accent" />
            <Button size="sm" title="Comment" disabled={busy || !comment.trim()} onClick={() => mutate("/api/workboard/comment", { body: comment }, () => setComment(""))}>
              <MessageSquare className="size-3.5" />
            </Button>
          </div>
          <div className="grid grid-cols-[1fr_auto] gap-1.5">
            <input value={blockReason} onChange={(e) => setBlockReason(e.target.value)} aria-label="Block reason" placeholder="block reason" className="h-8 rounded-md border border-border bg-panel px-2 text-xs outline-none focus-visible:border-accent" />
            <Button size="sm" variant="ghost" title="Block" disabled={busy || !blockReason.trim()} onClick={() => mutate("/api/workboard/block", { reason: blockReason }, () => setBlockReason(""))}>
              <CircleStop className="size-3.5" />
            </Button>
          </div>
          <div className="grid grid-cols-[1fr_auto] gap-1.5">
            <input value={failReason} onChange={(e) => setFailReason(e.target.value)} aria-label="Fail reason" placeholder="fail reason" className="h-8 rounded-md border border-border bg-panel px-2 text-xs outline-none focus-visible:border-accent" />
            <Button size="sm" variant="ghost" title="Fail" disabled={busy || !failReason.trim()} onClick={() => mutate("/api/workboard/fail", { reason: failReason }, () => setFailReason(""))}>
              <ShieldAlert className="size-3.5" />
            </Button>
          </div>
          <div className="grid gap-1.5 sm:grid-cols-[80px_1fr_auto_auto]">
            <input value={policyMax} onChange={(e) => setPolicyMax(e.target.value)} aria-label="Max attempts" className="h-8 rounded-md border border-border bg-panel px-2 text-xs outline-none focus-visible:border-accent" />
            <input value={policyEscalate} onChange={(e) => setPolicyEscalate(e.target.value)} aria-label="Escalate to" placeholder="escalate to" className="h-8 rounded-md border border-border bg-panel px-2 text-xs outline-none focus-visible:border-accent" />
            <Button size="sm" variant="ghost" disabled={busy || Number(policyMax) < 1} onClick={() => mutate("/api/workboard/policy", { max_attempts: Number(policyMax), escalate_to: policyEscalate })}>Policy</Button>
            <Button size="sm" variant="ghost" disabled={busy} onClick={() => mutate("/api/workboard/policy", { clear: true })}>Clear</Button>
          </div>
        </section>

        <section className="grid gap-3 lg:grid-cols-2">
          <TaskListSection title="Dependencies" icon={GitBranch}>
            {(task.dependencies || []).length === 0 && blockedDeps.length === 0 ? (
              <Muted>no dependencies</Muted>
            ) : (
              <ul className="space-y-1">
                {(task.dependencies || []).map((dep) => (
                  <li key={dep.id} className="flex items-center gap-2 rounded-md border border-border bg-background/45 px-2 py-1 text-xs">
                    <CornerDownRight className="size-3 text-muted" />
                    <span className="font-mono">{dep.id}</span>
                  </li>
                ))}
                {blockedDeps.map((dep) => (
                  <li key={`blocked-${dep.id}`} className="rounded-md border border-warn/35 bg-warn/10 px-2 py-1 text-xs text-warn">
                    {dep.id} {dep.title ? `- ${dep.title}` : ""} ({dep.missing ? "missing" : dep.status})
                  </li>
                ))}
              </ul>
            )}
          </TaskListSection>

          <TaskListSection title="Attempts" icon={RotateCcw}>
            {(task.attempts || []).length === 0 ? (
              <Muted>no attempts</Muted>
            ) : (
              <ul className="space-y-1">
                {(task.attempts || []).slice().reverse().map((attempt) => (
                  <li key={attempt.id || `${attempt.run_id}-${attempt.started_ms}`} className="rounded-md border border-border bg-background/45 px-2 py-1.5 text-xs">
                    <div className="flex min-w-0 items-center gap-1.5">
                      <Badge variant={attemptVariant(attempt.status)}>{attempt.status || "?"}</Badge>
                      {attempt.agent && <span>{attempt.agent}</span>}
                      <span className="ml-auto font-mono text-[10px] text-muted">{short(attempt.run_id || "")}</span>
                    </div>
                    {attempt.summary && <div className="mt-1 line-clamp-2 text-muted">{attempt.summary}</div>}
                  </li>
                ))}
              </ul>
            )}
          </TaskListSection>

          <TaskListSection title="Links and Artifacts" icon={Link2}>
            {(task.links || []).length === 0 && (task.artifacts || []).length === 0 ? (
              <Muted>no links</Muted>
            ) : (
              <ul className="space-y-1">
                {(task.links || []).map((link) => (
                  <li key={link.id || `${link.type}-${link.target}`} className="flex min-w-0 items-center gap-2 rounded-md border border-border bg-background/45 px-2 py-1 text-xs">
                    <Badge>{link.type || "link"}</Badge>
                    <span className="min-w-0 truncate font-mono">{link.target}</span>
                  </li>
                ))}
                {(task.artifacts || []).map((artifact) => (
                  <li key={artifact} className="min-w-0 truncate rounded-md border border-border bg-background/45 px-2 py-1 font-mono text-xs">{artifact}</li>
                ))}
              </ul>
            )}
          </TaskListSection>

          <TaskListSection title="Events" icon={CircleDot}>
            {events.length === 0 ? (
              <Muted>no events</Muted>
            ) : (
              <ul className="space-y-1">
                {events.slice().reverse().map((event) => (
                  <li key={event.seq || `${event.kind}-${event.ts_unix_ms}`} className="rounded-md border border-border bg-background/45 px-2 py-1 text-xs">
                    <div className="flex items-center gap-2">
                      <span className="font-mono text-muted">#{event.seq}</span>
                      <span className="truncate">{event.kind}</span>
                      <span className="ml-auto font-mono text-[10px] text-muted">{fmtTime(event.ts_unix_ms)}</span>
                    </div>
                    {event.payload?.action ? <div className="mt-0.5 text-[10px] text-muted">{String(event.payload.action)}</div> : null}
                  </li>
                ))}
              </ul>
            )}
          </TaskListSection>
        </section>
      </div>

      {(task.comments || []).length > 0 && (
        <section>
          <PanelTitle icon={MessageSquare} title="Comments" />
          <ul className="mt-2 space-y-1">
            {(task.comments || []).slice().reverse().map((comment) => (
              <li key={comment.id || `${comment.author}-${comment.created_ms}`} className="rounded-md border border-border bg-background/45 px-2 py-1.5 text-xs">
                <div className="mb-0.5 flex items-center gap-2 text-muted">
                  <span className="font-semibold text-foreground/80">{comment.author || "unknown"}</span>
                  <span className="ml-auto font-mono text-[10px]">{fmtTime(comment.created_ms)}</span>
                </div>
                <div className="text-foreground/90">{comment.body}</div>
              </li>
            ))}
          </ul>
        </section>
      )}
    </div>
  );
}

function ProofPanel({ task }: { task: WorkboardTask }) {
  const criteria = task.criteria || [];
  const met = criteria.filter((c) => c.met).length;
  const proven = !!task.proven;
  const gap = task.proof?.verdict?.gap?.trim();
  const ev = task.proof?.evidence;
  const artifacts = ev?.artifacts?.length || 0;
  return (
    <section
      className={cn(
        "rounded-md border p-2.5",
        proven ? "border-emerald-500/40 bg-emerald-500/5" : "border-amber-500/40 bg-amber-500/5",
      )}
    >
      <div className="mb-2 flex items-center gap-1.5">
        {proven ? <BadgeCheck className="size-4 text-emerald-500" /> : <ShieldCheck className="size-4 text-amber-500" />}
        <span className="text-xs font-semibold uppercase tracking-normal">Proof</span>
        <Badge variant={proven ? "good" : "warn"}>{proven ? "proven" : "unproven"}</Badge>
        <span className="text-xs text-muted">{met}/{criteria.length} criteria met</span>
      </div>
      <ul className="space-y-1">
        {criteria.map((c, i) => (
          <li key={i} className="flex items-start gap-1.5 text-sm">
            {c.met ? (
              <CheckCircle2 className="mt-0.5 size-3.5 shrink-0 text-emerald-500" />
            ) : (
              <XCircle className="mt-0.5 size-3.5 shrink-0 text-muted" />
            )}
            <span className="min-w-0">
              <span className={cn(c.met ? "text-foreground/90" : "text-foreground/70")}>{c.text}</span>
              {c.note && <span className="text-muted"> — {c.note}</span>}
            </span>
          </li>
        ))}
      </ul>
      {!proven && gap && <p className="mt-2 text-xs text-amber-600 dark:text-amber-400">Gap: {gap}</p>}
      {task.proof && (
        <p className="mt-2 text-xs text-muted">
          Evidence: {artifacts} artifact{artifacts === 1 ? "" : "s"}
          {ev && (ev.journal_from || ev.journal_to) ? `, journal #${ev.journal_from ?? 0}–#${ev.journal_to ?? 0}` : ""}
        </p>
      )}
    </section>
  );
}

function PanelTitle({ icon: Icon, title }: { icon: LucideIcon; title: string }) {
  return (
    <div className="mb-1.5 flex items-center gap-1.5 text-xs font-semibold uppercase tracking-normal text-muted">
      <Icon className="size-3.5" />
      {title}
    </div>
  );
}

function TaskListSection({ title, icon, children }: { title: string; icon: LucideIcon; children: ReactNode }) {
  return (
    <section className="min-w-0 rounded-md border border-border bg-panel/35 p-2">
      <PanelTitle icon={icon} title={title} />
      {children}
    </section>
  );
}

function statusVariant(status?: string): "default" | "good" | "bad" | "warn" | "accent" {
  switch ((status || "").toLowerCase()) {
    case "done":
    case "ready":
      return "good";
    case "blocked":
      return "warn";
    case "running":
    case "review":
      return "accent";
    case "failed":
    case "stale":
      return "bad";
    default:
      return "default";
  }
}

function attemptVariant(status?: string): "default" | "good" | "bad" | "warn" | "accent" {
  if (status === "done" || status === "review") return "good";
  if (status === "failed" || status === "stale") return "bad";
  if (status === "running") return "accent";
  return "default";
}

function statusDot(status?: string): string {
  switch ((status || "").toLowerCase()) {
    case "done":
    case "ready":
      return "bg-good";
    case "blocked":
      return "bg-warn";
    case "running":
    case "review":
      return "bg-accent";
    default:
      return "bg-border";
  }
}

function short(v: string): string {
  return v.length > 12 ? v.slice(0, 12) : v;
}
