import type { AgentEvent } from "@/lib/events";

// Council live model (M987). The backend emits a stream of council.* events as a
// deliberation unfolds — convened → each member starts its turn → each opinion
// lands → consensus. This module folds that stream into a CouncilRun so the Web
// UI can watch the whole path to the verdict live, and rebuild it after a
// navigation away (the events keep flowing into a module-level store). All folds
// are pure (time passed in) so they're unit-testable without a live daemon.

export interface CouncilSeat {
  seat: string;
  model: string;
}

export interface CouncilOpinion {
  seat: string;
  model: string;
  round: number;
  text: string;
  error?: string;
}

type CouncilPhase = "convening" | "deliberating" | "done";

export interface CouncilRun {
  corr: string;
  question: string;
  seats: CouncilSeat[];
  rounds: number; // deliberation rounds on top of the opening round (round 0)
  opinions: CouncilOpinion[];
  // Seats currently mid-turn, keyed as `${seat}#${round}` → {seat, round}. Drives
  // the "thinking now" pulse on a seat node.
  thinking: Record<string, { seat: string; round: number }>;
  consensus?: string;
  dissent?: string;
  // Grounding (current date + shared web research brief) the panel convened with.
  // asOf is YYYY-MM-DD; brief is the rendered research-brief text (empty when web
  // search is off or returned nothing).
  asOf?: string;
  brief?: string;
  phase: CouncilPhase;
  startedMs: number;
  updatedMs: number;
  done: boolean;
}

export function newCouncilRun(corr: string, nowMs: number, seed?: Partial<CouncilRun>): CouncilRun {
  return {
    corr,
    question: "",
    seats: [],
    rounds: 0,
    opinions: [],
    thinking: {},
    phase: "convening",
    startedMs: nowMs,
    updatedMs: nowMs,
    done: false,
    ...seed,
  };
}

const thinkingKey = (seat: string, round: number) => `${seat}#${round}`;

// foldCouncilEvent folds one council.* event into the run, returning a new run
// (or the same reference if the event isn't relevant). Unknown council kinds are
// ignored so adding event kinds later never throws here.
export function foldCouncilEvent(run: CouncilRun, e: AgentEvent, nowMs: number): CouncilRun {
  if (!e.kind || !e.kind.startsWith("council.")) return run;
  const p = (e.payload || {}) as Record<string, unknown>;
  switch (e.kind) {
    case "council.convened": {
      const seats = parseSeats(p);
      return {
        ...run,
        question: typeof p.question === "string" ? p.question : run.question,
        seats: seats.length ? seats : run.seats,
        rounds: typeof p.rounds === "number" ? p.rounds : run.rounds,
        phase: "deliberating",
        updatedMs: nowMs,
      };
    }
    case "council.brief": {
      // The dated web research brief that grounds the panel, landing just after
      // convened and before the first turn. Show it so the operator sees what
      // evidence the council was given.
      return {
        ...run,
        asOf: typeof p.as_of === "string" ? p.as_of : run.asOf,
        brief: typeof p.text === "string" ? p.text : run.brief,
        updatedMs: nowMs,
      };
    }
    case "council.member.started": {
      const seat = String(p.seat ?? "");
      const round = Number(p.round ?? 0);
      if (!seat) return run;
      return {
        ...run,
        phase: run.phase === "convening" ? "deliberating" : run.phase,
        thinking: { ...run.thinking, [thinkingKey(seat, round)]: { seat, round } },
        updatedMs: nowMs,
      };
    }
    case "council.opinion": {
      const seat = String(p.seat ?? "");
      const round = Number(p.round ?? 0);
      if (!seat) return run;
      const thinking = { ...run.thinking };
      delete thinking[thinkingKey(seat, round)];
      const op: CouncilOpinion = {
        seat,
        model: String(p.model ?? ""),
        round,
        text: typeof p.text === "string" ? p.text : "",
        error: typeof p.error_text === "string" && p.error_text ? p.error_text : undefined,
      };
      // De-dupe: replace any existing opinion for the same seat+round (an event
      // could be re-delivered) rather than appending a duplicate.
      const opinions = run.opinions.filter((o) => !(o.seat === seat && o.round === round));
      opinions.push(op);
      return { ...run, thinking, opinions, updatedMs: nowMs };
    }
    case "council.consensus": {
      return {
        ...run,
        consensus: typeof p.consensus === "string" ? p.consensus : run.consensus,
        dissent: typeof p.dissent === "string" && p.dissent ? p.dissent : run.dissent,
        thinking: {},
        phase: "done",
        done: true,
        updatedMs: nowMs,
      };
    }
    default:
      return run;
  }
}

function parseSeats(p: Record<string, unknown>): CouncilSeat[] {
  const raw = p.seats;
  if (!Array.isArray(raw)) return [];
  return raw
    .map((s) => {
      const o = (s || {}) as Record<string, unknown>;
      return { seat: String(o.seat ?? ""), model: String(o.model ?? "") };
    })
    .filter((s) => s.seat || s.model);
}

// --- derived selectors (pure) ---

export type SeatStatus = "waiting" | "thinking" | "done" | "error";

// seatStatus reports a seat's state in the LATEST active round, for its node in
// the live view: thinking (pulsing), done/error (has an opinion), or waiting.
export function seatStatus(run: CouncilRun, seat: string): SeatStatus {
  const round = currentRound(run);
  if (Object.values(run.thinking).some((t) => t.seat === seat)) return "thinking";
  const op = lastOpinionFor(run, seat);
  if (op && op.round === round) return op.error ? "error" : "done";
  if (run.done) return op?.error ? "error" : op ? "done" : "waiting";
  return op ? "done" : "waiting";
}

export function lastOpinionFor(run: CouncilRun, seat: string): CouncilOpinion | undefined {
  let found: CouncilOpinion | undefined;
  for (const o of run.opinions) if (o.seat === seat) found = o;
  return found;
}

// currentRound is the highest round seen among in-flight turns and landed
// opinions — i.e. how far the deliberation has progressed.
export function currentRound(run: CouncilRun): number {
  let r = 0;
  for (const o of run.opinions) r = Math.max(r, o.round);
  for (const t of Object.values(run.thinking)) r = Math.max(r, t.round);
  return r;
}

export interface RoundGroup {
  round: number;
  opinions: CouncilOpinion[];
}

// opinionsByRound groups landed opinions by round, ascending — the deliberation
// transcript / path to the verdict.
export function opinionsByRound(run: CouncilRun): RoundGroup[] {
  const byRound = new Map<number, CouncilOpinion[]>();
  for (const o of run.opinions) {
    const list = byRound.get(o.round);
    if (list) list.push(o);
    else byRound.set(o.round, [o]);
  }
  return [...byRound.keys()].sort((a, b) => a - b).map((round) => ({ round, opinions: byRound.get(round)! }));
}

// roundLabel names a round for the transcript header.
export function roundLabel(round: number): string {
  return round === 0 ? "Opening positions" : `Deliberation round ${round}`;
}

// progressLabel summarises the live phase for the header line.
export function progressLabel(run: CouncilRun): string {
  if (run.done) return "Verdict reached";
  const thinking = Object.keys(run.thinking).length;
  if (run.phase === "convening") return "Convening the council…";
  if (thinking > 0) {
    const round = currentRound(run);
    return `${roundLabel(round)} — ${thinking} member${thinking === 1 ? "" : "s"} deliberating…`;
  }
  if (run.opinions.length > 0) return "Chair is synthesizing the consensus…";
  return "Deliberating…";
}
