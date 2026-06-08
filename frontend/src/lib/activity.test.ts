// @vitest-environment jsdom
// activity.ts imports rundetail (pure) and the AgentEvent type; jsdom isn't
// strictly required but keeps it uniform with the other lib tests that touch api.
import { describe, it, expect } from "vitest";
import {
  seedFromRuns,
  foldActivityEvent,
  summarize,
  buildTree,
  type ActivityState,
} from "@/lib/activity";
import type { AgentEvent } from "@/lib/events";

function fold(events: AgentEvent[], init: ActivityState = {}): ActivityState {
  return events.reduce(foldActivityEvent, init);
}

describe("seedFromRuns", () => {
  it("seeds only in-flight runs, dropping terminal history", () => {
    const state = seedFromRuns([
      { correlation_id: "r1", intent: "live one", status: "running", started_unix_ms: 100, iters: 2, spent_mc: 500 },
      { correlation_id: "r2", intent: "old", status: "completed", started_unix_ms: 1 },
    ]);
    expect(Object.keys(state)).toEqual(["r1"]);
    expect(state.r1.intent).toBe("live one");
    expect(state.r1.iters).toBe(2);
    expect(state.r1.spentMc).toBe(500);
  });
});

describe("foldActivityEvent", () => {
  it("tracks a run from received → tool → completed with a live activity line", () => {
    const s1 = fold([{ kind: "task.received", correlation_id: "r1", ts_unix_ms: 10, payload: { intent: "do it" } }]);
    expect(s1.r1.status).toBe("running");
    expect(s1.r1.intent).toBe("do it");
    expect(s1.r1.activity).toBe("starting…");

    const s2 = fold(
      [
        { kind: "llm.request", correlation_id: "r1", payload: { iter: 0 } },
        { kind: "tool.invoked", correlation_id: "r1", payload: { tool: "shell" } },
        { kind: "budget.consumed", correlation_id: "r1", payload: { cost_microcents: 1200 } },
      ],
      s1,
    );
    expect(s2.r1.activity).toBe("calling shell");
    expect(s2.r1.iters).toBe(1);
    expect(s2.r1.spentMc).toBe(1200);

    const s3 = fold([{ kind: "task.completed", correlation_id: "r1", ts_unix_ms: 99, payload: { iters: 1 } }], s2);
    expect(s3.r1.status).toBe("completed");
    expect(s3.r1.endedMs).toBe(99);
    expect(s3.r1.activity).toBe("done");
  });

  it("ignores stray events for runs it never saw begin", () => {
    const s = fold([{ kind: "tool.invoked", correlation_id: "ghost", payload: { tool: "x" } }]);
    expect(Object.keys(s)).toHaveLength(0);
  });

  it("links a delegated sub-agent to its parent and notes the delegation", () => {
    const s = fold([
      { kind: "task.received", correlation_id: "lead", payload: { intent: "big job" } },
      { kind: "subagent.spawned", correlation_id: "lead", payload: { child_correlation: "sub", parent: "lead", task: "fetch docs", depth: 1 } },
      { kind: "task.received", correlation_id: "sub", payload: { intent: "fetch docs" } },
      { kind: "tool.invoked", correlation_id: "sub", payload: { tool: "http" } },
    ]);
    expect(s.sub.parentCorr).toBe("lead");
    expect(s.sub.depth).toBe(1);
    expect(s.sub.activity).toBe("calling http");
    expect(s.lead.activity).toBe("delegating: fetch docs");

    const tree = buildTree(s);
    expect(tree).toHaveLength(1); // only the lead is top-level
    expect(tree[0].run.corr).toBe("lead");
    expect(tree[0].children.map((c) => c.corr)).toEqual(["sub"]);
  });

  it("marks a failed run with its reason", () => {
    const s = fold([
      { kind: "task.received", correlation_id: "r1", payload: {} },
      { kind: "task.failed", correlation_id: "r1", ts_unix_ms: 5, payload: { error: "max iters" } },
    ]);
    expect(s.r1.status).toBe("failed");
    expect(s.r1.activity).toBe("failed: max iters");
  });
});

describe("summarize + buildTree ordering", () => {
  it("counts by status and sums spend", () => {
    const s = fold([
      { kind: "task.received", correlation_id: "a", payload: {} },
      { kind: "budget.consumed", correlation_id: "a", payload: { cost_microcents: 100 } },
      { kind: "task.received", correlation_id: "b", payload: {} },
      { kind: "task.completed", correlation_id: "b", ts_unix_ms: 2, payload: {} },
      { kind: "task.received", correlation_id: "c", payload: {} },
      { kind: "task.failed", correlation_id: "c", ts_unix_ms: 3, payload: {} },
    ]);
    expect(summarize(s)).toEqual({ running: 1, completed: 1, failed: 1, spentMc: 100 });
  });

  it("orders running runs before finished ones", () => {
    const s = fold([
      { kind: "task.received", correlation_id: "done", ts_unix_ms: 1, payload: {} },
      { kind: "task.completed", correlation_id: "done", ts_unix_ms: 2, payload: {} },
      { kind: "task.received", correlation_id: "live", ts_unix_ms: 3, payload: {} },
    ]);
    const tree = buildTree(s);
    expect(tree[0].run.corr).toBe("live"); // running first
    expect(tree[1].run.corr).toBe("done");
  });
});
