import { describe, it, expect } from "vitest";
import { emptyBucket, addEvent, summarize, type Bucket } from "@/lib/telemetry";
import type { AgentEvent } from "@/lib/events";

function ev(kind: string, payload: any = {}): AgentEvent {
  return { kind, payload };
}

describe("addEvent", () => {
  it("counts events, llm and tool calls", () => {
    let b = emptyBucket();
    b = addEvent(b, ev("llm.request"));
    b = addEvent(b, ev("llm.token"));
    b = addEvent(b, ev("tool.invoked"));
    expect(b.events).toBe(3);
    expect(b.llm).toBe(2);
    expect(b.tools).toBe(1);
  });

  it("counts sub-agent spawns (delegation activity)", () => {
    let b = emptyBucket();
    b = addEvent(b, ev("subagent.spawned"));
    b = addEvent(b, ev("subagent.spawned"));
    b = addEvent(b, ev("tool.invoked"));
    expect(b.subagents).toBe(2);
    expect(b.events).toBe(3);
  });

  it("accumulates tokens and cost from budget.consumed", () => {
    let b = emptyBucket();
    b = addEvent(b, ev("budget.consumed", { input_tokens: 100, output_tokens: 20, cost_microcents: 5_000_000 }));
    b = addEvent(b, ev("budget.consumed", { input_tokens: 50, output_tokens: 10, cost_microcents: 1_000_000 }));
    expect(b.tokensIn).toBe(150);
    expect(b.tokensOut).toBe(30);
    expect(b.costMc).toBe(6_000_000);
    expect(b.events).toBe(2);
  });

  it("does not mutate the input bucket", () => {
    const b0 = emptyBucket();
    const b1 = addEvent(b0, ev("llm.request"));
    expect(b0.events).toBe(0);
    expect(b1.events).toBe(1);
  });
});

describe("summarize", () => {
  it("averages rates over the bucket window and finds the peak second", () => {
    const buckets: Bucket[] = [
      { events: 10, llm: 6, tools: 2, tokensIn: 100, tokensOut: 20, costMc: 1_000_000, subagents: 2 },
      { events: 2, llm: 1, tools: 0, tokensIn: 0, tokensOut: 0, costMc: 0, subagents: 1 },
    ];
    const t = summarize(buckets);
    expect(t.windowSec).toBe(2);
    expect(t.totalEvents).toBe(12);
    expect(t.eventsPerSec).toBe(6); // 12 / 2s
    expect(t.tokensPerSec).toBe(60); // (120) / 2
    expect(t.costPerSecMc).toBe(500_000);
    expect(t.peakEvents).toBe(10);
    expect(t.subagentsTotal).toBe(3); // 2 + 1 summed over the window
  });

  it("is safe on an empty window (no divide-by-zero)", () => {
    const t = summarize([]);
    expect(t.eventsPerSec).toBe(0);
    expect(t.totalEvents).toBe(0);
    expect(t.peakEvents).toBe(0);
  });
});
