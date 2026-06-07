import { describe, it, expect } from "vitest";
import { deriveDetail, num, mergeEvents } from "@/lib/rundetail";
import type { AgentEvent } from "@/lib/events";

// A realistic HA-tool run arc (mirrors what /api/journal returns), deliberately
// out of seq order to prove deriveDetail sorts before folding.
const arc: AgentEvent[] = [
  { seq: 0, kind: "task.received", payload: {} },
  { seq: 1, kind: "llm.request", payload: { iter: 0, model: "mock" } },
  { seq: 2, kind: "llm.response", payload: { iter: 0, tool_calls: 1 } },
  { seq: 3, kind: "policy.decision", payload: { call_id: "c1", tool: "homeassistant", capability: "homeassistant.call", allow: true } },
  { seq: 4, kind: "tool.invoked", payload: { call_id: "c1", tool: "homeassistant" } },
  { seq: 5, kind: "tool.result", payload: { call_id: "c1", tool: "homeassistant", output: "called light.turn_off ok", error: false } },
  { seq: 8, kind: "budget.consumed", payload: { model: "mock", input_tokens: 100, output_tokens: 20, cached_input_tokens: 10, cost_microcents: 5000 } },
  { seq: 6, kind: "llm.request", payload: { iter: 1, model: "mock" } },
  { seq: 7, kind: "policy.decision", payload: { call_id: "c2", tool: "homeassistant", capability: "homeassistant.read", allow: true } },
  { seq: 9, kind: "tool.result", payload: { call_id: "c2", tool: "homeassistant", output: '{"state":"off"}', error: false } },
  { seq: 10, kind: "task.completed", payload: { answer: "done" } },
];

describe("deriveDetail", () => {
  it("folds the arc into a summary regardless of input order", () => {
    const d = deriveDetail(arc);
    expect(d.status).toBe("completed");
    expect(d.model).toBe("mock");
    expect(d.iterations).toBe(2); // iters 0 and 1 → max+1
    expect(d.answer).toBe("done");
  });

  it("sums budget tokens + cost and sets hasBudget", () => {
    const d = deriveDetail(arc);
    expect(d.hasBudget).toBe(true);
    expect(d.inputTokens).toBe(100);
    expect(d.outputTokens).toBe(20);
    expect(d.cachedTokens).toBe(10);
    expect(d.costMicrocents).toBe(5000);
  });

  it("groups tool calls by call_id with capability + verdict, result wins", () => {
    const d = deriveDetail(arc);
    expect(d.toolCalls).toHaveLength(2);
    const [a, b] = d.toolCalls;
    expect(a.callId).toBe("c1");
    expect(a.capability).toBe("homeassistant.call");
    expect(a.allow).toBe(true);
    expect(a.error).toBe(false);
    expect(a.output).toBe("called light.turn_off ok");
    expect(b.capability).toBe("homeassistant.read");
  });

  it("marks hasBudget false when no budget event was journaled", () => {
    const d = deriveDetail([
      { seq: 1, kind: "llm.request", payload: { iter: 0, model: "mock" } },
      { seq: 2, kind: "task.completed", payload: { answer: "hi" } },
    ]);
    expect(d.hasBudget).toBe(false);
    expect(d.inputTokens).toBe(0);
    expect(d.toolCalls).toHaveLength(0);
  });

  it("captures a denied / hard-denied tool call", () => {
    const d = deriveDetail([
      { seq: 1, kind: "policy.decision", payload: { call_id: "x", tool: "shell", capability: "shell", allow: false, hard_denied: true } },
    ]);
    expect(d.toolCalls).toHaveLength(1);
    expect(d.toolCalls[0].allow).toBe(false);
    expect(d.toolCalls[0].hardDenied).toBe(true);
  });

  it("records a failed run's error as the answer", () => {
    const d = deriveDetail([{ seq: 1, kind: "task.failed", payload: { error: "boom" } }]);
    expect(d.status).toBe("failed");
    expect(d.answer).toBe("boom");
  });

  it("handles an empty arc", () => {
    const d = deriveDetail([]);
    expect(d.iterations).toBe(0);
    expect(d.toolCalls).toHaveLength(0);
    expect(d.status).toBeUndefined();
  });
});

describe("num", () => {
  it("coerces and guards non-finite values", () => {
    expect(num(5)).toBe(5);
    expect(num("7")).toBe(7);
    expect(num(undefined)).toBe(0);
    expect(num("nope")).toBe(0);
    expect(num(null)).toBe(0);
  });
});

describe("mergeEvents", () => {
  it("dedups by seq when a live event repeats the fetched snapshot", () => {
    const snap: AgentEvent[] = [
      { seq: 1, kind: "llm.request" },
      { seq: 2, kind: "tool.invoked" },
    ];
    const live: AgentEvent[] = [
      { seq: 2, kind: "tool.invoked" }, // duplicate of snapshot
      { seq: 3, kind: "tool.result" }, // new
    ];
    const merged = mergeEvents(snap, live);
    expect(merged.map((e) => e.seq)).toEqual([1, 2, 3]);
  });

  it("preserves seq 0 (a real first event) and dedups by id when seq is absent", () => {
    const merged = mergeEvents(
      [{ seq: 0, kind: "task.received" }, { id: "x", kind: "a" }],
      [{ id: "x", kind: "a" }, { id: "y", kind: "b" }],
    );
    expect(merged).toHaveLength(3);
    expect(merged.some((e) => e.seq === 0)).toBe(true);
  });

  it("returns the snapshot unchanged when there are no live events", () => {
    const snap: AgentEvent[] = [{ seq: 1, kind: "a" }];
    expect(mergeEvents(snap, [])).toEqual(snap);
  });
});
