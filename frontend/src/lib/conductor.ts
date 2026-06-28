import type { AgentEvent } from "@/lib/events";

// Conductor live model (M997). The backend emits a stream of conductor.* events
// as the panel works — started → one step per role turn (thinker, worker,
// verifier, repeating on a failed verify) → done. This module folds that stream
// into a ConductorRun so the Web UI can watch the loop unfold live and rebuild it
// after a navigation away (events keep flowing into a module-level store). All
// folds are pure (time passed in) so they're unit-testable without a daemon.
//
// Mirrors lib/council.ts. The live step events carry clipped text + a verifier
// exec FLAG (not the full execution output); the blocking POST result later
// upgrades each step to its complete form via applyConductorResult.

export interface ConductorRoles {
  thinker: string;
  worker: string;
  verifier: string;
}

interface ConductorExec {
  ran: boolean;
  ok: boolean;
  language?: string;
  output?: string;
}

export interface ConductorStep {
  round: number;
  role: string; // thinker | worker | verifier
  model: string;
  text?: string;
  verdict?: string; // verifier: pass | fail
  reason?: string;
  exec?: ConductorExec;
  error?: string;
}

type ConductorPhase = "running" | "done";

export interface ConductorRun {
  corr: string;
  task: string;
  roles: ConductorRoles;
  maxRounds: number;
  steps: ConductorStep[];
  answer?: string;
  passed?: boolean;
  plan?: string;
  phase: ConductorPhase;
  startedMs: number;
  updatedMs: number;
  done: boolean;
}

const emptyRoles = (): ConductorRoles => ({ thinker: "", worker: "", verifier: "" });

export function newConductorRun(corr: string, nowMs: number, seed?: Partial<ConductorRun>): ConductorRun {
  return {
    corr,
    task: "",
    roles: emptyRoles(),
    maxRounds: 0,
    steps: [],
    phase: "running",
    startedMs: nowMs,
    updatedMs: nowMs,
    done: false,
    ...seed,
  };
}

const str = (v: unknown): string => (typeof v === "string" ? v : "");
const num = (v: unknown): number => (typeof v === "number" ? v : Number(v) || 0);
const stepKey = (role: string, round: number) => `${role}#${round}`;

// foldConductorEvent folds one conductor.* event into the run, returning a new
// run (or the same reference when the event isn't relevant). Unknown conductor
// kinds are ignored so adding kinds later never throws here.
export function foldConductorEvent(run: ConductorRun, e: AgentEvent, nowMs: number): ConductorRun {
  if (!e.kind || !e.kind.startsWith("conductor.")) return run;
  const p = (e.payload || {}) as Record<string, unknown>;
  switch (e.kind) {
    case "conductor.started": {
      return {
        ...run,
        task: str(p.task) || run.task,
        roles: {
          thinker: str(p.thinker) || run.roles.thinker,
          worker: str(p.worker) || run.roles.worker,
          verifier: str(p.verifier) || run.roles.verifier,
        },
        maxRounds: num(p.rounds) || run.maxRounds,
        phase: "running",
        updatedMs: nowMs,
      };
    }
    case "conductor.step": {
      const role = str(p.role);
      const round = num(p.round);
      if (!role) return run;
      const step: ConductorStep = { round, role, model: str(p.model) };
      if (str(p.error)) step.error = str(p.error);
      if (role === "verifier") {
        if (str(p.verdict)) step.verdict = str(p.verdict);
        if (str(p.reason)) step.reason = str(p.reason);
        // The live event carries exec as a bool; output arrives with the result.
        if (p.exec === true) step.exec = { ran: true, ok: str(p.verdict) === "pass" };
      } else if (str(p.text)) {
        step.text = str(p.text);
      }
      return { ...run, steps: upsertStep(run.steps, step), updatedMs: nowMs };
    }
    case "conductor.done": {
      return {
        ...run,
        passed: p.passed === true,
        answer: str(p.answer) || run.answer,
        phase: "done",
        done: true,
        updatedMs: nowMs,
      };
    }
    default:
      return run;
  }
}

// upsertStep replaces an existing step for the same role+round (re-delivery) or
// appends a new one, preserving arrival order.
function upsertStep(steps: ConductorStep[], step: ConductorStep): ConductorStep[] {
  const key = stepKey(step.role, step.round);
  const idx = steps.findIndex((s) => stepKey(s.role, s.round) === key);
  if (idx < 0) return [...steps, step];
  const next = steps.slice();
  next[idx] = step;
  return next;
}

// --- derived selectors (pure) ---

// progressLabel summarises the live phase for the header line.
export function progressLabel(run: ConductorRun): string {
  if (run.done) return run.passed ? "Verified" : "Not verified";
  const last = run.steps[run.steps.length - 1];
  if (!last) return "Convening the panel…";
  switch (last.role) {
    case "thinker":
      return "Thinker is planning…";
    case "worker":
      return `Worker is solving (round ${last.round})…`;
    case "verifier":
      return last.verdict ? "Verified — finishing…" : `Verifier is checking (round ${last.round})…`;
    default:
      return "Working…";
  }
}
