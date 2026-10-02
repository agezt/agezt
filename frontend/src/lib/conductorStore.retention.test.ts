import { describe, expect, it } from "vitest";
import { conductorRunsSnapshot, ingestConductorEvent } from "@/lib/conductorStore";
import type { AgentEvent } from "@/app/events";

// The Conductor store is wired to the whole firehose from App.tsx and was never
// bounded: every conductor.* event added a correlation-id entry and nothing
// ever removed one, so an open tab accumulated runs indefinitely — while also
// copying the entire map on every fold. It had no reader at all (the view was
// retired), which made that pure waste.
//
// These assert the real cap through conductorRunsSnapshot(), the store's own
// read primitive, rather than reaching into module internals.

/** The cap the store documents, mirrored here so a change to it is a visible diff. */
const CAP = 20;

function evt(corr: string, kind: string, at: number): AgentEvent {
  return { kind, correlation_id: corr, occurred_at: at } as unknown as AgentEvent;
}

describe("conductor store retention", () => {
  it("starts with no runs", () => {
    // A fresh module in a fresh test file. The other tests in this file feed it,
    // so run order matters — vitest executes in declaration order within a file.
    expect(conductorRunsSnapshot()).toEqual([]);
  });

  it("ignores events that are not conductor traffic", () => {
    ingestConductorEvent(evt("noise-1", "run.completed", Date.now()));
    ingestConductorEvent({ kind: "conductor.started" } as unknown as AgentEvent);
    ingestConductorEvent({ kind: "conductor.step", correlation_id: "" } as unknown as AgentEvent);
    expect(conductorRunsSnapshot()).toEqual([]);
  });

  it("folds a run it has seen before instead of duplicating it", () => {
    ingestConductorEvent(evt("fold-1", "conductor.started", 1_000));
    ingestConductorEvent(evt("fold-1", "conductor.step", 2_000));
    ingestConductorEvent(evt("fold-1", "conductor.done", 3_000));
    const runs = conductorRunsSnapshot();
    expect(runs).toHaveLength(1);
    expect(runs[0].corr).toBe("fold-1");
  });

  it("never retains more than the cap, however many runs arrive", () => {
    const base = 10_000;
    for (let i = 0; i < 200; i++) {
      ingestConductorEvent(evt(`bulk-${i}`, "conductor.started", base + i));
      ingestConductorEvent(evt(`bulk-${i}`, "conductor.done", base + i));
    }
    const runs = conductorRunsSnapshot();
    expect(runs.length).toBeLessThanOrEqual(CAP);
    expect(runs.length).toBeGreaterThan(0);
  });

  it("keeps the most recent runs and drops the oldest", () => {
    // Feed a batch beyond the cap and check eviction is by recency, not by
    // insertion accident: the newest correlation ids must be the survivors.
    const base = 50_000;
    for (let i = 0; i < CAP + 10; i++) {
      ingestConductorEvent(evt(`recent-${i}`, "conductor.started", base + i));
    }
    const corr = conductorRunsSnapshot().map((r) => r.corr);
    expect(corr).toHaveLength(CAP);
    // newest first
    expect(corr[0]).toBe(`recent-${CAP + 9}`);
    expect(corr).toContain(`recent-${CAP}`);
    expect(corr).not.toContain("recent-0");
  });
});
