import { num } from "@/lib/rundetail";
import type { AgentEvent } from "@/lib/events";

// ActiveRun is one agent run as the Activity monitor sees it live — folded from
// the event firehose so the operator can answer "is anything running right now,
// and what is it doing?". Sub-agent runs carry a parentCorr linking them to the
// lead run that delegated them (the "background agents").
export interface ActiveRun {
  corr: string;
  intent: string;
  status: "running" | "completed" | "failed";
  startedMs: number;
  endedMs?: number;
  iters: number;
  spentMc: number;
  activity: string; // human current-activity line ("calling shell", "thinking · iter 2")
  parentCorr: string; // "" for top-level runs
  depth: number;
  lastSeq: number;
}

export type ActivityState = Record<string, ActiveRun>;

// How many finished runs to retain for the "recently finished" tail. The fold
// drops older ones so a long-lived session doesn't grow without bound; running
// runs are never pruned.
const KEEP_FINISHED = 20;

function blankRun(corr: string): ActiveRun {
  return {
    corr,
    intent: "",
    status: "running",
    startedMs: 0,
    iters: 0,
    spentMc: 0,
    activity: "working…",
    parentCorr: "",
    depth: 0,
    lastSeq: 0,
  };
}

// seedFromRuns turns a /api/runs payload into the initial state — the runs that
// are already in flight when the monitor mounts (events before mount aren't on
// the firehose). Only `running` entries seed; terminal ones are history.
export function seedFromRuns(runs: any[]): ActivityState {
  const state: ActivityState = {};
  for (const r of runs || []) {
    const status = String(r.status || "");
    if (status !== "running") continue;
    const corr = String(r.correlation_id || "");
    if (!corr) continue;
    const parentCorr = String(r.parent_correlation || "");
    state[corr] = {
      corr,
      intent: String(r.intent || ""),
      status: "running",
      startedMs: num(r.started_unix_ms),
      iters: num(r.iters),
      spentMc: num(r.spent_mc),
      activity: "working…",
      parentCorr,
      depth: parentCorr ? 1 : 0,
      lastSeq: 0,
    };
  }
  return state;
}

// foldActivityEvent folds one firehose event into the run map. Pure (no React),
// so the live monitor's whole state machine is unit-tested. Field names mirror
// deriveDetail/chat so all three agree on how an event arc reads.
export function foldActivityEvent(state: ActivityState, e: AgentEvent): ActivityState {
  const corr = String(e.correlation_id || "");
  if (!corr) return state;
  const p = e.payload || {};
  const kind = e.kind || "";

  // subagent.spawned is published under the PARENT correlation; its payload
  // names the CHILD. Attach the link to the child (creating a stub if the spawn
  // is seen before the child's own first event), and note the delegation on the
  // parent so its activity line reads "delegating: …".
  if (kind === "subagent.spawned") {
    const child = String(p.child_correlation || "");
    if (!child) return state;
    const parent = String(p.parent || corr);
    const next = { ...state };
    const c = { ...(next[child] || blankRun(child)) };
    c.parentCorr = parent;
    c.depth = 1;
    if (p.task && (!c.intent || c.intent === "working…")) c.intent = String(p.task);
    next[child] = c;
    if (next[parent]) {
      next[parent] = { ...next[parent], activity: `delegating: ${String(p.task || "subtask")}` };
    }
    return next;
  }

  const existing = state[corr];
  // Ignore events for runs we never saw begin — otherwise the capped firehose
  // buffer could resurrect long-finished correlations on mount.
  if (!existing && kind !== "task.received") return state;

  const next = { ...state };
  const r = { ...(existing || blankRun(corr)) };
  if (!r.startedMs) r.startedMs = num(e.ts_unix_ms);

  switch (kind) {
    case "task.received":
      r.status = "running";
      if (p.intent) r.intent = String(p.intent);
      r.activity = "starting…";
      break;
    case "llm.request":
    case "llm.response":
      r.iters = Math.max(r.iters, num(p.iter) + 1);
      r.activity = `thinking · iter ${r.iters}`;
      break;
    case "tool.invoked":
      r.activity = `calling ${String(p.tool || p.capability || "tool")}`;
      break;
    case "policy.decision":
      if (p.allow === false) r.activity = `blocked ${String(p.capability || p.tool || "")}`.trim();
      break;
    case "tool.result":
      r.activity = `ran ${String(p.tool || "tool")}${p.error ? " (error)" : ""}`;
      break;
    case "budget.consumed":
      r.spentMc += num(p.cost_microcents);
      break;
    case "task.completed":
      r.status = "completed";
      r.endedMs = num(e.ts_unix_ms) || r.endedMs;
      r.activity = "done";
      if (p.iters != null) r.iters = Math.max(r.iters, num(p.iters));
      break;
    case "task.failed":
      r.status = "failed";
      r.endedMs = num(e.ts_unix_ms) || r.endedMs;
      r.activity = p.error ? `failed: ${String(p.error)}` : "failed";
      break;
  }
  r.lastSeq = Math.max(r.lastSeq, num(e.seq));
  next[corr] = r;
  return pruneFinished(next);
}

// pruneFinished keeps every running run and only the most recently ended
// finished runs, so a long session's state stays bounded.
function pruneFinished(state: ActivityState): ActivityState {
  const finished = Object.values(state).filter((r) => r.status !== "running");
  if (finished.length <= KEEP_FINISHED) return state;
  finished.sort((a, b) => (b.endedMs || 0) - (a.endedMs || 0));
  const drop = new Set(finished.slice(KEEP_FINISHED).map((r) => r.corr));
  const next: ActivityState = {};
  for (const [k, v] of Object.entries(state)) if (!drop.has(k)) next[k] = v;
  return next;
}

export interface ActivitySummary {
  running: number;
  completed: number;
  failed: number;
  spentMc: number;
}

export function summarize(state: ActivityState): ActivitySummary {
  const s: ActivitySummary = { running: 0, completed: 0, failed: 0, spentMc: 0 };
  for (const r of Object.values(state)) {
    if (r.status === "running") s.running++;
    else if (r.status === "completed") s.completed++;
    else if (r.status === "failed") s.failed++;
    s.spentMc += r.spentMc;
  }
  return s;
}

// A run grouped with the sub-agents it delegated, for tree rendering.
export interface RunTreeNode {
  run: ActiveRun;
  children: ActiveRun[];
}

// buildTree groups runs into top-level nodes each with their delegated children.
// Running runs sort before finished; within a group, newest first. A child whose
// parent isn't in state is promoted to top-level so nothing is hidden.
export function buildTree(state: ActivityState): RunTreeNode[] {
  const all = Object.values(state);
  const byCorr = new Set(all.map((r) => r.corr));
  const childrenOf = new Map<string, ActiveRun[]>();
  for (const r of all) {
    if (r.parentCorr && byCorr.has(r.parentCorr)) {
      const list = childrenOf.get(r.parentCorr) || [];
      list.push(r);
      childrenOf.set(r.parentCorr, list);
    }
  }
  const rank = (r: ActiveRun) => (r.status === "running" ? 0 : 1);
  const recency = (r: ActiveRun) => r.endedMs || r.startedMs || 0;
  const tops = all
    .filter((r) => !(r.parentCorr && byCorr.has(r.parentCorr)))
    .sort((a, b) => rank(a) - rank(b) || recency(b) - recency(a));
  return tops.map((run) => ({
    run,
    children: (childrenOf.get(run.corr) || []).sort((a, b) => rank(a) - rank(b) || recency(b) - recency(a)),
  }));
}
