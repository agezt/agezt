import { describe, it, expect } from "vitest";
import {
  agentScope,
  agentCorrelations,
  filterByCorrelation,
  filterAgentMemory,
  filterAgentSkills,
  summarizeAgent,
  lastFailure,
  type MemoryRecord,
  type SkillLite,
  type RunLite,
} from "@/lib/agentdetail";

describe("agentScope", () => {
  it("uses explicit memory_scope when set", () => {
    expect(agentScope("researcher", "shared-brain")).toBe("shared-brain");
  });
  it("falls back to the slug when scope is blank", () => {
    expect(agentScope("researcher", "")).toBe("researcher");
    expect(agentScope("researcher", "  ")).toBe("researcher");
    expect(agentScope("researcher")).toBe("researcher");
  });
});

const RUNS: RunLite[] = [
  { correlation_id: "c1", agent: "researcher", status: "completed", spent_mc: 1e9, started_unix_ms: 100 },
  { correlation_id: "c2", agent: "researcher", status: "failed", spent_mc: 5e8, started_unix_ms: 300 },
  { correlation_id: "c3", agent: "writer", status: "completed", spent_mc: 2e9, started_unix_ms: 200 },
  { correlation_id: "c4", agent: "researcher", status: "running", spent_mc: 0, started_unix_ms: 400 },
];

describe("agentCorrelations", () => {
  it("collects correlation ids of runs started as the agent", () => {
    const c = agentCorrelations(RUNS, "researcher");
    expect([...c].sort()).toEqual(["c1", "c2", "c4"]);
    expect(c.has("c3")).toBe(false);
  });
});

describe("filterByCorrelation", () => {
  const rows = [
    { correlation_id: "c1", actor: "run-1", capability: "shell" },
    { correlation_id: "c3", actor: "run-2", capability: "fs" }, // writer's run
    { correlation_id: "zz", actor: "researcher", capability: "net" }, // actor match
    { correlation_id: "yy", actor: "other", capability: "x" }, // neither
  ];
  it("keeps rows whose correlation belongs to the agent or whose actor is the slug", () => {
    const corrs = agentCorrelations(RUNS, "researcher");
    const got = filterByCorrelation(rows, corrs, "researcher");
    expect(got.map((r) => r.capability)).toEqual(["shell", "net"]);
  });
});

describe("filterAgentMemory", () => {
  const recs: MemoryRecord[] = [
    { id: "m1", subject: "private", tags: { scope: "researcher" } },
    { id: "m2", subject: "shared", tags: {} },
    { id: "m3", subject: "authored", added_by: "researcher" },
    { id: "m4", subject: "other-scope", tags: { scope: "writer" } },
    { id: "m5", subject: "no-tags" },
  ];
  it("keeps records scoped to the agent or authored by it, excludes shared/other", () => {
    const got = filterAgentMemory(recs, "researcher");
    expect(got.map((r) => r.id).sort()).toEqual(["m1", "m3"]);
  });
  it("honours an explicit memory scope", () => {
    const got = filterAgentMemory(recs, "researcher", "writer");
    expect(got.map((r) => r.id).sort()).toEqual(["m3", "m4"]);
  });
});

describe("filterAgentSkills", () => {
  const skills: SkillLite[] = [
    { id: "s1", name: "a", agent: "researcher" },
    { id: "s2", name: "b" },
    { id: "s3", name: "c", agent: "writer" },
  ];
  it("keeps only skills private to the agent", () => {
    expect(filterAgentSkills(skills, "researcher").map((s) => s.id)).toEqual(["s1"]);
  });
});

describe("summarizeAgent", () => {
  it("folds run count, total spend, and most-recent start", () => {
    const s = summarizeAgent(RUNS, "researcher");
    expect(s.runs).toBe(3);
    expect(s.totalSpentMc).toBe(1e9 + 5e8 + 0);
    expect(s.lastStartedMs).toBe(400);
  });
  it("is zeroed for an agent with no runs", () => {
    const s = summarizeAgent(RUNS, "ghost");
    expect(s).toEqual({ runs: 0, totalSpentMc: 0, lastStartedMs: undefined });
  });
});

describe("lastFailure", () => {
  it("returns the most recent failed run for the agent", () => {
    expect(lastFailure(RUNS, "researcher")?.correlation_id).toBe("c2");
  });
  it("returns undefined when the agent has no failures", () => {
    expect(lastFailure(RUNS, "writer")).toBeUndefined();
  });
});
