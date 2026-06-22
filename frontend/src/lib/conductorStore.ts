import { useSyncExternalStore } from "react";
import type { AgentEvent } from "@/lib/events";
import { newConductorRun, foldConductorEvent, type ConductorRun, type ConductorRoles, type ConductorStep } from "@/lib/conductor";

// Conductor store (M997): a module-level singleton — deliberately ABOVE the view
// router, like the council store — so a run keeps assembling from the live event
// stream even when you navigate away from the Conductor page and back. The view
// subscribes and renders; it holds no authoritative state itself. Mirrors
// lib/councilStore.ts.

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
