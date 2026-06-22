import { describe, it, expect } from "vitest";
import { computeInsights, type RunRow } from "@/lib/insights";

const runs: RunRow[] = [
  { status: "completed", model: "a", spent_mc: 1_000_000, started_unix_ms: 300, duration_ms: 2000, iters: 2 },
  { status: "failed", model: "a", spent_mc: 500_000, started_unix_ms: 100, duration_ms: 1000, iters: 1 },
  { status: "completed", model: "b", spent_mc: 4_000_000, started_unix_ms: 200, duration_ms: 3000, iters: 4 },
  { status: "running", model: "b", spent_mc: 0, started_unix_ms: 400 },
];

describe("computeInsights", () => {
  it("counts outcomes and success rate over finished runs", () => {
    const i = computeInsights(runs);
    expect(i.total).toBe(4);
    expect(i.completed).toBe(2);
    expect(i.failed).toBe(1);
    expect(i.running).toBe(1);
    // 2 completed / 3 finished
    expect(i.successRate).toBeCloseTo(2 / 3);
  });

  it("sums spend and averages duration/iters over valid rows", () => {
    const i = computeInsights(runs);
    expect(i.totalSpentMc).toBe(5_500_000);
    // durations: 2000,1000,3000 → mean 2000 (running has none)
    expect(i.avgDurationMs).toBe(2000);
    // iters: 2,1,4 → mean 2.33…
    expect(i.avgIters).toBeCloseTo(7 / 3);
  });

  it("groups by model sorted by spend desc", () => {
    const i = computeInsights(runs);
    expect(i.byModel.map((m) => m.model)).toEqual(["b", "a"]);
    expect(i.byModel[0]).toMatchObject({ model: "b", runs: 2, spentMc: 4_000_000 });
    expect(i.byModel[1]).toMatchObject({ model: "a", runs: 2, spentMc: 1_500_000 });
  });

  it("computes per-model efficiency (avg cost/run and avg iters/run)", () => {
    const i = computeInsights(runs);
    const b = i.byModel[0];
    // b: 4_000_000 across 2 runs → 2_000_000/run; iters 4 + 0(running) → 2/run
    expect(b.avgSpentMc).toBe(2_000_000);
    expect(b.avgIters).toBe(2);
    // a: 1_500_000 across 2 runs → 750_000/run; iters 2 + 1 → 1.5/run
    const a = i.byModel[1];
    expect(a.avgSpentMc).toBe(750_000);
    expect(a.avgIters).toBe(1.5);
  });

  it("builds a chronological cumulative spend series", () => {
    const i = computeInsights(runs);
    expect(i.spend.map((p) => p.t)).toEqual([100, 200, 300, 400]);
    expect(i.spend.map((p) => p.cum)).toEqual([500_000, 4_500_000, 5_500_000, 5_500_000]);
  });

  it("is safe on an empty list", () => {
    const i = computeInsights([]);
    expect(i.total).toBe(0);
    expect(i.successRate).toBe(0);
    expect(i.avgDurationMs).toBe(0);
    expect(i.spend).toEqual([]);
  });
});
