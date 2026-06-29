import { describe, it, expect } from "vitest";
import {
  newCouncilRun,
  foldCouncilEvent,
  seatStatus,
  currentRound,
  opinionsByRound,
  roundLabel,
  progressLabel,
  lastOpinionFor,
  type CouncilRun,
} from "@/lib/council";
import type { AgentEvent } from "@/lib/events";

const ev = (kind: string, payload: Record<string, unknown>, corr = "wc-1"): AgentEvent => ({
  kind,
  correlation_id: corr,
  payload,
});

function seed(): CouncilRun {
  return newCouncilRun("wc-1", 1000, {
    question: "Ship it?",
    seats: [
      { seat: "Elder 1", model: "deepseek-chat" },
      { seat: "Elder 2", model: "gpt-4o" },
    ],
    rounds: 1,
  });
}

describe("foldCouncilEvent", () => {
  it("ignores non-council and corr-less events", () => {
    const r = seed();
    expect(foldCouncilEvent(r, ev("tool.result", {}), 2)).toBe(r);
    expect(foldCouncilEvent(r, { kind: "council.opinion", payload: {} }, 2)).toBe(r);
  });

  it("convened sets seats/rounds and moves to deliberating", () => {
    let r = newCouncilRun("wc-1", 1000);
    r = foldCouncilEvent(r, ev("council.convened", {
      question: "Ship it?",
      seats: [{ seat: "Elder 1", model: "m1" }, { seat: "Elder 2", model: "m2" }],
      rounds: 2,
    }), 1100);
    expect(r.phase).toBe("deliberating");
    expect(r.seats).toHaveLength(2);
    expect(r.rounds).toBe(2);
    expect(r.question).toBe("Ship it?");
  });

  it("brief records the dated research brief", () => {
    let r = seed();
    r = foldCouncilEvent(
      r,
      ev("council.brief", { as_of: "2026-06-29", count: 2, text: "RESEARCH BRIEF — live web results retrieved 2026-06-29..." }),
      1150,
    );
    expect(r.asOf).toBe("2026-06-29");
    expect(r.brief).toContain("RESEARCH BRIEF");
    // An unknown/odd brief payload doesn't clobber an existing brief.
    const r2 = foldCouncilEvent(r, ev("council.brief", {}), 1160);
    expect(r2.asOf).toBe("2026-06-29");
    expect(r2.brief).toContain("RESEARCH BRIEF");
  });

  it("member.started marks a seat thinking; opinion clears it and records text", () => {
    let r = seed();
    r = foldCouncilEvent(r, ev("council.member.started", { seat: "Elder 1", model: "deepseek-chat", round: 0 }), 1200);
    expect(seatStatus(r, "Elder 1")).toBe("thinking");
    expect(seatStatus(r, "Elder 2")).toBe("waiting");

    r = foldCouncilEvent(r, ev("council.opinion", { seat: "Elder 1", model: "deepseek-chat", round: 0, text: "Yes, ship.", error: false }), 1300);
    expect(seatStatus(r, "Elder 1")).toBe("done");
    expect(lastOpinionFor(r, "Elder 1")?.text).toBe("Yes, ship.");
    expect(Object.keys(r.thinking)).toHaveLength(0);
  });

  it("opinion with error_text surfaces the error", () => {
    let r = seed();
    r = foldCouncilEvent(r, ev("council.opinion", { seat: "Elder 2", model: "gpt-4o", round: 0, text: "", error: true, error_text: "rate limited" }), 1300);
    expect(seatStatus(r, "Elder 2")).toBe("error");
    expect(lastOpinionFor(r, "Elder 2")?.error).toBe("rate limited");
  });

  it("re-delivered opinion replaces, not duplicates", () => {
    let r = seed();
    const e = ev("council.opinion", { seat: "Elder 1", model: "m1", round: 0, text: "v1" });
    r = foldCouncilEvent(r, e, 1300);
    r = foldCouncilEvent(r, { ...e, payload: { ...e.payload, text: "v2" } }, 1400);
    expect(r.opinions.filter((o) => o.seat === "Elder 1" && o.round === 0)).toHaveLength(1);
    expect(lastOpinionFor(r, "Elder 1")?.text).toBe("v2");
  });

  it("consensus sets the verdict, clears thinking, completes the run", () => {
    let r = seed();
    r = foldCouncilEvent(r, ev("council.member.started", { seat: "Elder 1", round: 0 }), 1200);
    r = foldCouncilEvent(r, ev("council.consensus", { consensus: "Ship on Friday.", dissent: "Elder 2 prefers Monday." }), 1500);
    expect(r.done).toBe(true);
    expect(r.phase).toBe("done");
    expect(r.consensus).toBe("Ship on Friday.");
    expect(r.dissent).toBe("Elder 2 prefers Monday.");
    expect(Object.keys(r.thinking)).toHaveLength(0);
  });
});

describe("selectors", () => {
  it("currentRound tracks the furthest round and groups opinions ascending", () => {
    let r = seed();
    r = foldCouncilEvent(r, ev("council.opinion", { seat: "Elder 1", round: 0, text: "a" }), 1);
    r = foldCouncilEvent(r, ev("council.opinion", { seat: "Elder 1", round: 1, text: "b" }), 2);
    expect(currentRound(r)).toBe(1);
    const groups = opinionsByRound(r);
    expect(groups.map((g) => g.round)).toEqual([0, 1]);
  });

  it("roundLabel names opening vs deliberation", () => {
    expect(roundLabel(0)).toBe("Opening positions");
    expect(roundLabel(2)).toBe("Deliberation round 2");
  });

  it("progressLabel reflects phase", () => {
    let r = newCouncilRun("wc-1", 0);
    expect(progressLabel(r)).toBe("Convening the council…");
    r = foldCouncilEvent(r, ev("council.member.started", { seat: "Elder 1", round: 0 }), 1);
    expect(progressLabel(r)).toContain("deliberating");
    r = foldCouncilEvent(r, ev("council.consensus", { consensus: "done" }), 2);
    expect(progressLabel(r)).toBe("Verdict reached");
  });
});
