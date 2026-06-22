import { describe, it, expect } from "vitest";
import { newConductorRun, foldConductorEvent, progressLabel, type ConductorRun } from "@/lib/conductor";
import type { AgentEvent } from "@/lib/events";

const ev = (kind: string, payload: Record<string, unknown>, corr = "c1"): AgentEvent => ({
  kind,
  correlation_id: corr,
  payload,
});

function fold(run: ConductorRun, e: AgentEvent): ConductorRun {
  return foldConductorEvent(run, e, 1000);
}

describe("foldConductorEvent", () => {
  it("ignores non-conductor events (same reference)", () => {
    const run = newConductorRun("c1", 0);
    expect(fold(run, ev("council.opinion", {}))).toBe(run);
  });

  it("folds a full thinker→worker→verifier→done run", () => {
    let run = newConductorRun("c1", 0);
    run = fold(run, ev("conductor.started", { task: "add()", thinker: "m-a", worker: "m-b", verifier: "m-c", rounds: 2 }));
    expect(run.roles).toEqual({ thinker: "m-a", worker: "m-b", verifier: "m-c" });
    expect(run.maxRounds).toBe(2);
    expect(run.phase).toBe("running");

    run = fold(run, ev("conductor.step", { role: "thinker", model: "m-a", round: 0, text: "plan it" }));
    run = fold(run, ev("conductor.step", { role: "worker", model: "m-b", round: 1, text: "```python\n...\n```" }));
    run = fold(run, ev("conductor.step", { role: "verifier", model: "m-c", round: 1, verdict: "fail", reason: "bug", exec: true }));
    expect(run.steps).toHaveLength(3);
    const v = run.steps[2];
    expect(v.verdict).toBe("fail");
    expect(v.exec).toEqual({ ran: true, ok: false });

    // retry round
    run = fold(run, ev("conductor.step", { role: "worker", model: "m-b", round: 2, text: "fixed" }));
    run = fold(run, ev("conductor.step", { role: "verifier", model: "m-c", round: 2, verdict: "pass", reason: "ok", exec: true }));
    expect(run.steps).toHaveLength(5);
    expect(run.steps[4].exec).toEqual({ ran: true, ok: true });

    run = fold(run, ev("conductor.done", { passed: true, rounds: 2, answer: "final" }));
    expect(run.done).toBe(true);
    expect(run.passed).toBe(true);
    expect(run.answer).toBe("final");
  });

  it("upserts a re-delivered step in place (no duplicate)", () => {
    let run = newConductorRun("c1", 0);
    run = fold(run, ev("conductor.step", { role: "worker", model: "m", round: 1, text: "v1" }));
    run = fold(run, ev("conductor.step", { role: "worker", model: "m", round: 1, text: "v2" }));
    expect(run.steps).toHaveLength(1);
    expect(run.steps[0].text).toBe("v2");
  });

  it("carries a step error", () => {
    let run = newConductorRun("c1", 0);
    run = fold(run, ev("conductor.step", { role: "worker", model: "m", round: 1, error: "provider down" }));
    expect(run.steps[0].error).toBe("provider down");
  });
});

describe("progressLabel", () => {
  it("reflects the live phase then the verdict", () => {
    let run = newConductorRun("c1", 0);
    expect(progressLabel(run)).toMatch(/Convening/);
    run = fold(run, ev("conductor.step", { role: "thinker", model: "m", round: 0, text: "x" }));
    expect(progressLabel(run)).toMatch(/Thinker/);
    run = fold(run, ev("conductor.step", { role: "worker", model: "m", round: 1, text: "x" }));
    expect(progressLabel(run)).toMatch(/Worker/);
    run = fold(run, ev("conductor.done", { passed: false }));
    expect(progressLabel(run)).toBe("Not verified");
  });
});
