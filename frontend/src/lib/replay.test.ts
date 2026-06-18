import { describe, it, expect } from "vitest";
import { buildReplay } from "@/lib/replay";
import type { AgentEvent } from "@/lib/events";

function ev(seq: number, kind: string, payload: any = {}): AgentEvent {
  return { seq, kind, ts_unix_ms: 1000 + seq, payload, correlation_id: "r1" };
}

describe("buildReplay", () => {
  it("orders by seq and folds cumulative tokens/cost/tools", () => {
    const arc: AgentEvent[] = [
      ev(5, "budget.consumed", { input_tokens: 100, output_tokens: 20, cost_microcents: 5_000_000, model: "m" }),
      ev(1, "task.received", { intent: "do it" }),
      ev(2, "llm.request", { iter: 0, messages: 1, context_chars: 50, tools: 3 }),
      ev(3, "tool.invoked", { tool: "shell", call_id: "c1", input: { command: "dir" } }),
      ev(4, "tool.result", { tool: "shell", call_id: "c1", output: "ok" }),
      ev(6, "task.completed", { answer: "done" }),
    ];
    const steps = buildReplay(arc);
    expect(steps.map((s) => s.kind)).toEqual([
      "task.received",
      "llm.request",
      "tool.invoked",
      "tool.result",
      "budget.consumed",
      "task.completed",
    ]);
    // Cumulative tool count increments at tool.invoked and holds after.
    expect(steps[2].cumTools).toBe(1);
    expect(steps.at(-1)!.cumTools).toBe(1);
    // Cumulative tokens/cost only land at/after budget.consumed.
    expect(steps[3].cumIn).toBe(0);
    const last = steps.at(-1)!;
    expect(last.cumIn).toBe(100);
    expect(last.cumOut).toBe(20);
    expect(last.cumCostMc).toBe(5_000_000);
  });

  it("drops streaming token/reasoning deltas", () => {
    const arc: AgentEvent[] = [
      ev(1, "task.received", { intent: "x" }),
      ev(2, "llm.token", { text: "a" }),
      ev(3, "llm.reasoning", { text: "b" }),
      ev(4, "task.completed", { answer: "y" }),
    ];
    const steps = buildReplay(arc);
    expect(steps.map((s) => s.kind)).toEqual(["task.received", "task.completed"]);
  });

  it("labels steering events with the steer tone", () => {
    const steps = buildReplay([
      ev(1, "run.paused", {}),
      ev(2, "run.steered", { directive: "focus on X" }),
      ev(3, "run.resumed", {}),
    ]);
    expect(steps.map((s) => s.tone)).toEqual(["steer", "steer", "steer"]);
    expect(steps[1].detail).toContain("focus on X");
  });

  it("carries the iteration forward (sticky) onto later non-iter steps", () => {
    const steps = buildReplay([
      ev(1, "llm.request", { iter: 2, messages: 1 }),
      ev(2, "tool.invoked", { tool: "shell", call_id: "c" }),
      ev(3, "budget.consumed", { input_tokens: 1, output_tokens: 1, cost_microcents: 1 }),
    ]);
    expect(steps.map((s) => s.iter)).toEqual([2, 2, 2]);
  });

  it("is empty for an empty arc", () => {
    expect(buildReplay([])).toEqual([]);
  });

  it("turns incident-family events into compact replay steps with badge metadata", () => {
    const steps = buildReplay([
      {
        seq: 1,
        kind: "info",
        subject: "agent.wake",
        ts_unix_ms: 1001,
        correlation_id: "r1",
        payload: {
          agent: "infra-lead",
          phase: "completed",
          reason: "incident owner woke",
        },
      },
    ]);
    expect(steps[0].title).toBe("infra-lead · incident owner woke · agent.wake");
    expect(steps[0].detail).toBe("");
    expect(steps[0].incident).toEqual({
      subject: "agent.wake",
      phase: "completed",
      mode: undefined,
    });
  });
});
