// @vitest-environment jsdom
//
// Tests for Mission's pure `notableEvents` fold — the timeline the mission view
// derives from the rolling event feed. Only the exported pure helper is covered
// here; the component rendering is exercised by the view-level smoke tests.

import { describe, it, expect } from "vitest";
import type { AgentEvent } from "@/lib/events";
import { notableEvents } from "@/views/Mission";

describe("notableEvents", () => {
  it("keeps two distinct notable events that carry neither an id nor a seq", () => {
    // Regression: the dedup key used to fall back to `${kind}-${seq ?? ""}`, so
    // every id-less, seq-less event of one kind collapsed onto the constant
    // "task.failed-" and all but the first were silently dropped from the
    // timeline. Both rows below are genuinely different runs.
    const runA: AgentEvent = {
      kind: "task.failed",
      subject: "agent:alpha",
      correlation_id: "corr-AAA",
      ts_unix_ms: 100,
      payload: { reason: "provider 500" },
    };
    const runB: AgentEvent = {
      kind: "task.failed",
      subject: "agent:bravo",
      correlation_id: "corr-BBB",
      ts_unix_ms: 200,
      payload: { reason: "tool timeout" },
    };
    const kept = notableEvents([runA, runB], 8).map((e) => e.correlation_id);
    expect(kept.sort()).toEqual(["corr-AAA", "corr-BBB"]);
  });

  it("still dedups one event delivered by both the backfill and the live feed", () => {
    const live: AgentEvent = {
      kind: "task.failed",
      seq: 7,
      correlation_id: "c1",
      ts_unix_ms: 10,
      payload: { reason: "boom" },
    };
    expect(notableEvents([live, { ...live }], 8)).toHaveLength(1);
  });

  it("keeps non-notable kinds out of the timeline", () => {
    const noise: AgentEvent = {
      kind: "tool.result",
      correlation_id: "c1",
      ts_unix_ms: 10,
      payload: {},
    };
    expect(notableEvents([noise], 8)).toHaveLength(0);
  });
});
