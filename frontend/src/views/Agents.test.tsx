// @vitest-environment jsdom
import { describe, it, expect } from "vitest";
import { statusKind, summarizeRoots, filterRoots, scheduleAgentSlug, type RootSummary } from "@/views/Agents";

describe("statusKind", () => {
  it("buckets the daemon's run statuses", () => {
    expect(statusKind("running")).toBe("running");
    expect(statusKind("in_progress")).toBe("running");
    expect(statusKind("completed")).toBe("done");
    expect(statusKind("ok")).toBe("done");
    expect(statusKind("failed")).toBe("failed");
    expect(statusKind("cancelled")).toBe("failed");
    expect(statusKind("abandoned")).toBe("other");
    expect(statusKind(undefined)).toBe("other");
  });
});

describe("summarizeRoots", () => {
  const runs = [
    // A lead with two sub-agents (one nested) — a 3-node tree of depth 2.
    { correlation_id: "lead1", status: "completed", model: "m", intent: "research the topic", spent_mc: 1_000_000, iters: 3, started_unix_ms: 100, answer_preview: "the answer", agent: "ops" },
    { correlation_id: "sub1", parent_correlation: "lead1", status: "completed", spent_mc: 500_000 },
    { correlation_id: "sub2", parent_correlation: "sub1", status: "completed", spent_mc: 250_000 },
    // A second, running lead (no sub-agents), more recent.
    { correlation_id: "lead2", status: "running", intent: "do the thing", iters: 1, started_unix_ms: 200, agent: "researcher" },
    // A chat run without an agent — should be filtered out (not an agent run).
    { correlation_id: "chat1", status: "completed", intent: "just chatting", iters: 1, started_unix_ms: 50 },
  ];

  it("makes one summary per lead, folding the subtree, running-first then newest", () => {
    const roots = summarizeRoots(runs);
    // chat1 is filtered out because it has no agent field
    expect(roots.map((r) => r.id)).toEqual(["lead2", "lead1"]); // running sorts first

    const l1 = roots.find((r) => r.id === "lead1")!;
    expect(l1.agents).toBe(3);
    expect(l1.subAgents).toBe(2);
    expect(l1.depth).toBe(2);
    expect(l1.treeSpentMc).toBe(1_750_000); // 1.0 + 0.5 + 0.25
    expect(l1.kind).toBe("done");
    expect(l1.answerPreview).toBe("the answer");
    expect(l1.agentName).toBe("ops");

    const l2 = roots.find((r) => r.id === "lead2")!;
    expect(l2.kind).toBe("running");
    expect(l2.subAgents).toBe(0);
    expect(l2.agentName).toBe("researcher");
  });

  it("does not surface sub-agents as their own cards", () => {
    const ids = summarizeRoots(runs).map((r) => r.id);
    expect(ids).not.toContain("sub1");
    expect(ids).not.toContain("sub2");
  });

  it("filters out runs without an agent (chat conversations)", () => {
    const roots = summarizeRoots(runs);
    expect(roots.map((r) => r.id)).not.toContain("chat1");
  });
});

describe("filterRoots", () => {
  const roots = [
    { id: "a", kind: "running" },
    { id: "b", kind: "done" },
    { id: "c", kind: "failed" },
    { id: "d", kind: "done" },
  ] as RootSummary[];

  it("passes everything through for 'all'", () => {
    expect(filterRoots(roots, "all")).toHaveLength(4);
  });
  it("filters to a single kind", () => {
    expect(filterRoots(roots, "done").map((r) => r.id)).toEqual(["b", "d"]);
    expect(filterRoots(roots, "running").map((r) => r.id)).toEqual(["a"]);
    expect(filterRoots(roots, "failed").map((r) => r.id)).toEqual(["c"]);
  });
});

describe("scheduleAgentSlug", () => {
  it("extracts the --agent binding from a cadence intent", () => {
    expect(scheduleAgentSlug("run the digest --agent researcher")).toBe("researcher");
    expect(scheduleAgentSlug("--agent=ops-watcher check disks")).toBe("ops-watcher");
    expect(scheduleAgentSlug("plain intent, default persona")).toBe("");
    expect(scheduleAgentSlug(undefined)).toBe("");
  });
});
