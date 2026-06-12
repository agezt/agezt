// @vitest-environment jsdom
import { describe, it, expect } from "vitest";
import { statusKind, summarizeRoots, filterRoots, scheduleAgentSlug, buildArmy, type RootSummary } from "@/views/Agents";

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
    { correlation_id: "lead1", status: "completed", model: "m", intent: "research the topic", spent_mc: 1_000_000, iters: 3, started_unix_ms: 100, answer_preview: "the answer" },
    { correlation_id: "sub1", parent_correlation: "lead1", status: "completed", spent_mc: 500_000 },
    { correlation_id: "sub2", parent_correlation: "sub1", status: "completed", spent_mc: 250_000 },
    // A second, running lead (no sub-agents), more recent.
    { correlation_id: "lead2", status: "running", intent: "do the thing", iters: 1, started_unix_ms: 200, agent: "researcher" },
  ];

  it("makes one summary per lead, folding the subtree, running-first then newest", () => {
    const roots = summarizeRoots(runs);
    expect(roots.map((r) => r.id)).toEqual(["lead2", "lead1"]); // running sorts first

    const l1 = roots.find((r) => r.id === "lead1")!;
    expect(l1.agents).toBe(3);
    expect(l1.subAgents).toBe(2);
    expect(l1.depth).toBe(2);
    expect(l1.treeSpentMc).toBe(1_750_000); // 1.0 + 0.5 + 0.25
    expect(l1.kind).toBe("done");
    expect(l1.answerPreview).toBe("the answer");

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

describe("buildArmy", () => {
  const profiles = [
    { slug: "researcher", name: "Researcher", model: "m1", enabled: true },
    { slug: "ops", enabled: true },
    { slug: "idle-bot", enabled: false },
    { slug: "ghost", enabled: true, retired: true },
  ];
  const orders = [
    {
      id: "ord1",
      name: "Nightly digest",
      enabled: true,
      agent: "researcher",
      triggers: [
        { type: "cron", schedule: "0 9 * * *" },
        { type: "event", subject: "task.failed" },
      ],
    },
    // Disabled order must not arm anyone.
    { id: "ord2", enabled: false, agent: "ops", triggers: [{ type: "cron", schedule: "* * * * *" }] },
  ];
  const schedules = [
    { id: "sch1", intent: "summarize inbox --agent ops", cadence: "daily 09:00", enabled: true },
    { id: "sch2", intent: "--agent researcher noop", cadence: "hourly", enabled: false },
  ];
  const runs = [
    { correlation_id: "r1", status: "running", agent: "ops", started_unix_ms: 500 },
    { correlation_id: "r2", status: "completed", agent: "researcher", started_unix_ms: 300 },
  ];

  it("folds wake sources per agent, skips retired, keeps disabled sources out", () => {
    const army = buildArmy(profiles, orders, schedules, runs);
    expect(army.map((a) => a.slug)).toEqual(["ops", "researcher", "idle-bot"]); // running > armed > rest
    const ops = army[0];
    expect(ops.running).toBe(true);
    expect(ops.wake).toEqual([{ type: "schedule", detail: "daily 09:00", via: "sch1" }]);
    const res = army[1];
    expect(res.running).toBe(false);
    expect(res.lastRunMs).toBe(300);
    expect(res.wake.map((w) => w.type)).toEqual(["cron", "event"]);
    expect(res.wake[0].via).toBe("Nightly digest");
    // Retired agent never enlists; disabled agent still listed (paused).
    expect(army.find((a) => a.slug === "ghost")).toBeUndefined();
    expect(army.find((a) => a.slug === "idle-bot")?.enabled).toBe(false);
  });
});
