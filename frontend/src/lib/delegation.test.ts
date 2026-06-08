import { describe, it, expect } from "vitest";
import { buildDelegationTree, pickDefaultRoot, type RunNode } from "@/lib/delegation";

// lead → (a, b); a → a1. Plus an unrelated run.
const runs: RunNode[] = [
  { id: "lead", status: "completed", spentMc: 1_000_000, iters: 3 },
  { id: "a", parent: "lead", status: "completed", spentMc: 500_000 },
  { id: "b", parent: "lead", status: "running", spentMc: 0 },
  { id: "a1", parent: "a", status: "completed", spentMc: 250_000 },
  { id: "other", status: "failed", spentMc: 2_000_000 },
];

describe("buildDelegationTree", () => {
  it("includes the root and all descendants, not unrelated runs", () => {
    const t = buildDelegationTree(runs, "lead");
    expect(t.count).toBe(4);
    expect(t.nodes.map((n) => n.id).sort()).toEqual(["a", "a1", "b", "lead"]);
    expect(t.nodes.some((n) => n.id === "other")).toBe(false);
  });

  it("builds parent→child edges", () => {
    const t = buildDelegationTree(runs, "lead");
    const keys = t.edges.map((e) => `${e.from}->${e.to}`).sort();
    expect(keys).toEqual(["a->a1", "lead->a", "lead->b"]);
  });

  it("sums spend across the subtree and tracks depth", () => {
    const t = buildDelegationTree(runs, "lead");
    expect(t.totalSpentMc).toBe(1_750_000); // 1.0 + 0.5 + 0 + 0.25 (M)
    expect(t.maxDepth).toBe(2); // lead(0) → a(1) → a1(2)
  });

  it("marks the root and lays depth on the y axis", () => {
    const t = buildDelegationTree(runs, "lead");
    const root = t.nodes.find((n) => n.id === "lead")!;
    expect(root.root).toBe(true);
    expect(root.y).toBe(0);
    const a1 = t.nodes.find((n) => n.id === "a1")!;
    expect(a1.depth).toBe(2);
    expect(a1.y).toBeGreaterThan(root.y);
  });

  it("returns empty for an unknown root", () => {
    expect(buildDelegationTree(runs, "nope").count).toBe(0);
  });

  it("is cycle-safe", () => {
    const cyclic: RunNode[] = [
      { id: "x", parent: "y" },
      { id: "y", parent: "x" },
    ];
    // Should terminate and include both at most once.
    const t = buildDelegationTree(cyclic, "x");
    expect(t.count).toBeLessThanOrEqual(2);
  });
});

describe("pickDefaultRoot", () => {
  it("prefers the newest run that has sub-agents", () => {
    expect(pickDefaultRoot(runs)).toBe("lead");
  });
  it("falls back to the newest run when none delegate", () => {
    expect(pickDefaultRoot([{ id: "solo" }, { id: "older" }])).toBe("solo");
  });
  it("is undefined for no runs", () => {
    expect(pickDefaultRoot([])).toBeUndefined();
  });
});
