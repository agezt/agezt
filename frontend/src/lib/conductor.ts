import { useSyncExternalStore } from "react";
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

// ───────────────────────── live store ─────────────────────────
// Conductor store (M997): a module-level singleton — deliberately ABOVE the view
// router, like the council store — so a run keeps assembling from the live event
// stream even when you navigate away from the Conductor page and back. The view
// subscribes and renders; it holds no authoritative state itself. Mirrors
// lib/council.ts. (Previously lib/conductorStore.ts; merged here so the store and
// the pure folds it drives live as one module — C2.)

interface ConductorState {
  runs: Record<string, ConductorRun>;
  activeCorr: string | null;
}

let state: ConductorState = { runs: {}, activeCorr: null };
const listeners = new Set<() => void>();

function setState(next: ConductorState) {
  state = next;
  listeners.forEach((l) => l());
}

function now(): number {
  return Date.now();
}

// startConductorRun seeds a run the instant the operator hits Conduct — before
// any event arrives — so the live view appears immediately with the known roles,
// and marks it active so a return-visit resumes it.
export function startConductorRun(corr: string, task: string, roles: ConductorRoles, maxRounds: number): void {
  const run = newConductorRun(corr, now(), { task, roles, maxRounds });
  setState({ runs: { ...state.runs, [corr]: run }, activeCorr: corr });
}

// ingestConductorEvent folds one firehose event into its run. Wired once at the
// app level so it captures the whole stream regardless of which view is mounted.
export function ingestConductorEvent(e: AgentEvent): void {
  if (!e.kind || !e.kind.startsWith("conductor.") || !e.correlation_id) return;
  const corr = e.correlation_id;
  const prev = state.runs[corr] ?? newConductorRun(corr, now());
  const next = foldConductorEvent(prev, e, now());
  if (next === prev) return;
  const activeCorr = e.kind === "conductor.started" ? corr : state.activeCorr ?? corr;
  setState({ runs: { ...state.runs, [corr]: next }, activeCorr });
}

// applyConductorResult merges the blocking POST's full (un-clipped) transcript —
// including verifier exec output and the final answer — into the run once it
// returns, upgrading the event-clipped steps to their complete form.
export function applyConductorResult(
  corr: string,
  result: {
    answer?: string;
    passed?: boolean;
    plan?: string;
    rounds?: number;
    roles?: Record<string, string>;
    steps?: ConductorStep[];
  },
): void {
  const prev = state.runs[corr];
  if (!prev) return;
  const next: ConductorRun = {
    ...prev,
    steps: result.steps?.length ? result.steps : prev.steps,
    answer: result.answer ?? prev.answer,
    passed: result.passed ?? prev.passed,
    plan: result.plan || prev.plan,
    maxRounds: result.rounds ?? prev.maxRounds,
    roles: result.roles
      ? {
          thinker: result.roles.thinker ?? prev.roles.thinker,
          worker: result.roles.worker ?? prev.roles.worker,
          verifier: result.roles.verifier ?? prev.roles.verifier,
        }
      : prev.roles,
    phase: "done",
    done: true,
    updatedMs: now(),
  };
  setState({ runs: { ...state.runs, [corr]: next }, activeCorr: state.activeCorr });
}

function getSnapshot(): ConductorState {
  return state;
}

function subscribe(l: () => void): () => void {
  listeners.add(l);
  return () => {
    listeners.delete(l);
  };
}

// useConductorStore exposes the live store to React. Components select the run
// they started (or the active one on a return visit).
export function useConductorStore(): ConductorState {
  return useSyncExternalStore(subscribe, getSnapshot, getSnapshot);
}

// genConductorCorr mints a client-side correlation id that satisfies the daemon's
// sanitizeCorr (plain [A-Za-z0-9_-]) so the UI can subscribe to a run's events
// before the blocking ask returns.
export function genConductorCorr(): string {
  const rand =
    typeof crypto !== "undefined" && crypto.randomUUID ? crypto.randomUUID() : `${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 10)}`;
  return "wcd-" + rand;
}
