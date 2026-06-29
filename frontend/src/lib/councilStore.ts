import { useSyncExternalStore } from "react";
import type { AgentEvent } from "@/lib/events";
import { newCouncilRun, foldCouncilEvent, type CouncilRun, type CouncilSeat } from "@/lib/council";

// Council store (M987): a module-level singleton — deliberately ABOVE the view
// router, like the chat store — so a council keeps assembling from the live event
// stream even when you navigate away from the Council page and come back. The
// Council view subscribes and renders; it holds no authoritative state itself.

interface CouncilState {
  runs: Record<string, CouncilRun>;
  activeCorr: string | null;
}

let state: CouncilState = { runs: {}, activeCorr: null };
const listeners = new Set<() => void>();

function setState(next: CouncilState) {
  state = next;
  listeners.forEach((l) => l());
}

function now(): number {
  return Date.now();
}

// startCouncilRun seeds a run the instant the operator hits Convene — before any
// event arrives — so the live view appears immediately with the known seats, and
// marks it active so a return-visit resumes it.
export function startCouncilRun(corr: string, question: string, seats: CouncilSeat[], rounds: number): void {
  const run = newCouncilRun(corr, now(), { question, seats, rounds });
  setState({ runs: { ...state.runs, [corr]: run }, activeCorr: corr });
}

// ingestCouncilEvent folds one firehose event into its run. Wired once at the app
// level so it captures the whole stream regardless of which view is mounted.
export function ingestCouncilEvent(e: AgentEvent): void {
  if (!e.kind || !e.kind.startsWith("council.") || !e.correlation_id) return;
  const corr = e.correlation_id;
  const prev = state.runs[corr] ?? newCouncilRun(corr, now());
  const next = foldCouncilEvent(prev, e, now());
  if (next === prev) return;
  const activeCorr = e.kind === "council.convened" ? corr : state.activeCorr ?? corr;
  setState({ runs: { ...state.runs, [corr]: next }, activeCorr });
}

// applyCouncilResult merges the blocking POST's full (un-clipped) opinions and
// consensus into the run once it returns, upgrading the event-clipped text to the
// complete answer. Best-effort: a run that already completed from events is fine.
export function applyCouncilResult(
  corr: string,
  result: {
    consensus?: string;
    dissent?: string;
    asOf?: string;
    brief?: string;
    opinions?: { seat: string; model: string; round: number; text: string; error?: string }[];
  },
): void {
  const prev = state.runs[corr];
  if (!prev) return;
  const opinions = result.opinions?.length
    ? result.opinions.map((o) => ({ seat: o.seat, model: o.model, round: o.round, text: o.text, error: o.error }))
    : prev.opinions;
  const next: CouncilRun = {
    ...prev,
    opinions,
    consensus: result.consensus ?? prev.consensus,
    dissent: result.dissent || prev.dissent,
    asOf: result.asOf || prev.asOf,
    brief: result.brief || prev.brief,
    thinking: {},
    phase: "done",
    done: true,
    updatedMs: now(),
  };
  setState({ runs: { ...state.runs, [corr]: next }, activeCorr: state.activeCorr });
}

function getSnapshot(): CouncilState {
  return state;
}

function subscribe(l: () => void): () => void {
  listeners.add(l);
  return () => {
    listeners.delete(l);
  };
}

// useCouncilStore exposes the live store to React. Components select the run they
// care about (the one they started, or the active one on a return visit).
export function useCouncilStore(): CouncilState {
  return useSyncExternalStore(subscribe, getSnapshot, getSnapshot);
}

// genCouncilCorr mints a client-side correlation id that satisfies the daemon's
// sanitizeCorr (plain [A-Za-z0-9_-]) so the UI can subscribe to a run's events
// before the blocking ask returns.
export function genCouncilCorr(): string {
  const rand =
    typeof crypto !== "undefined" && crypto.randomUUID ? crypto.randomUUID() : `${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 10)}`;
  return "wc-" + rand;
}
