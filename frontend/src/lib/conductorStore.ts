import type { AgentEvent } from "@/app/events";
import { newConductorRun, foldConductorEvent, type ConductorRun } from "@/lib/conductor";

// Conductor store (M997): a module-level singleton — deliberately ABOVE the view
// router, like the council store — so a run keeps assembling from the live event
// stream even when you navigate away from the Conductor page and back. The view
// subscribes and renders; it holds no authoritative state itself. Mirrors
// lib/councilStore.ts.
//
// The read side is currently absent: the Conductor view was retired in the
// Day-23/28 IA cleanup, so startConductorRun / applyConductorResult /
// useConductorStore / genConductorCorr lost their only callers and were
// removed. ingestConductorEvent stays — App.tsx wires it so the store keeps
// folding the firehose — which leaves this module write-only. Restoring the
// view means restoring the read side, not rewriting the store.

interface ConductorState {
  runs: Record<string, ConductorRun>;
  activeCorr: string | null;
}

// The store is fed by the whole firehose and was never bounded: every
// conductor.* event added an entry keyed by correlation id, and nothing ever
// removed one. In a browser tab left open, a long enough series of conductor
// runs grows the map without limit — and because each fold rebuilds the object
// (`{ ...state.runs }`), every event also copies the whole map. That is an
// O(events x runs) allocation churn in a tab that, at the time of writing, has
// no reader at all.
//
// Capping by recency fixes it whether or not the view comes back, so it does
// not depend on the surface decision: the Conductor panel is not a place where
// an operator scrolls back through last month's deliberations, and the runs
// are reconstructible from the journal if they ever are.
const MAX_RETAINED_RUNS = 20;

let state: ConductorState = { runs: {}, activeCorr: null };

// Monotonic insert counter. `updatedMs` has millisecond resolution and a busy
// Conductor folds several steps inside the same millisecond, so recency ties are
// constant. Ordering by it alone would silently fall back to insertion order,
// which is what made the snapshot's "newest first" contract untrue.
const insertSeq = new Map<string, number>();
let seqCounter = 0;

function now(): number {
  return Date.now();
}

/** Drop the oldest runs until the map is within MAX_RETAINED_RUNS. */
function prune(runs: Record<string, ConductorRun>): Record<string, ConductorRun> {
  const keys = Object.keys(runs);
  if (keys.length <= MAX_RETAINED_RUNS) return runs;

  // Oldest first by recency, then by insert order for ties, then drop the
  // surplus. The second key is what makes eviction deterministic when a batch
  // of runs lands in the same millisecond.
  const ordered = keys.sort(
    (a, b) => runs[a].updatedMs - runs[b].updatedMs || (insertSeq.get(a) ?? 0) - (insertSeq.get(b) ?? 0),
  );
  const drop = new Set(ordered.slice(0, ordered.length - MAX_RETAINED_RUNS));
  if (drop.size === 0) return runs;

  const kept: Record<string, ConductorRun> = {};
  for (const k of keys) if (!drop.has(k)) kept[k] = runs[k];
  for (const k of drop) insertSeq.delete(k);
  return kept;
}

// conductorRunsSnapshot returns the retained runs, newest first. It is the
// store's read primitive: a store whose contents cannot be inspected cannot be
// debugged, and it is what a restored Conductor view would render from. It also
// makes the retention cap observable, which is how the regression test asserts
// it without reaching into module internals.
export function conductorRunsSnapshot(): ConductorRun[] {
  return Object.values(state.runs)
    .map((r) => ({ run: r, seq: insertSeq.get(r.corr) ?? 0 }))
    .sort((a, b) => b.run.updatedMs - a.run.updatedMs || b.seq - a.seq)
    .map((x) => x.run);
}

// ingestConductorEvent folds one firehose event into its run. Wired once at the
// app level so it captures the whole stream regardless of which view is mounted.
export function ingestConductorEvent(e: AgentEvent): void {
  if (!e.kind || !e.kind.startsWith("conductor.") || !e.correlation_id) return;
  const corr = e.correlation_id;
  const prev = state.runs[corr] ?? newConductorRun(corr, now());
  const next = foldConductorEvent(prev, e, now());
  if (next === prev) return;
  if (!insertSeq.has(corr)) insertSeq.set(corr, ++seqCounter);
  const activeCorr = e.kind === "conductor.started" ? corr : state.activeCorr ?? corr;
  state = { runs: prune({ ...state.runs, [corr]: next }), activeCorr };
}

