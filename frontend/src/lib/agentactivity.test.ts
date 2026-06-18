import { describe, expect, it } from "vitest";

import { agentActivityEventMatches, agentActivityOperationalState, agentActivityPulse, agentRunCorrelations, filterAgentLogEvents } from "@/lib/agentactivity";

describe("agentactivity helpers", () => {
  it("matches direct actor, run correlation, and targeted doctor events", () => {
    const runCorrs = agentRunCorrelations([{ correlation_id: "corr-1" }]);
    expect(agentActivityEventMatches({ actor: "builder" }, "builder", runCorrs)).toBe(true);
    expect(agentActivityEventMatches({ correlation_id: "corr-1", kind: "task.completed" }, "builder", runCorrs)).toBe(true);
    expect(agentActivityEventMatches({ subject: "doctor.auto_repair", payload: { agent: "builder" } }, "builder", runCorrs)).toBe(true);
    expect(agentActivityEventMatches({ subject: "doctor.auto_repair", payload: { target_agent: "builder" } }, "builder", runCorrs)).toBe(true);
    expect(agentActivityEventMatches({ subject: "agent.repair", payload: { agent: "builder" } }, "builder", new Set())).toBe(true);
    expect(agentActivityEventMatches({ kind: "agent.retry", payload: { agent: "builder" } }, "builder", new Set())).toBe(true);
    expect(agentActivityEventMatches({ kind: "board.posted", payload: { from: "Builder", to: "planner" } }, "builder", new Set())).toBe(true);
    expect(agentActivityEventMatches({ kind: "board.posted", payload: { from: "planner", to: "builder" } }, "builder", new Set())).toBe(true);
    expect(agentActivityEventMatches({ kind: "board.posted", payload: { from: "planner", to: "*", acked_by: ["Builder"] } }, "builder", new Set())).toBe(true);
    expect(agentActivityEventMatches({ kind: "roster.updated", payload: { slug: "builder" } }, "builder", runCorrs)).toBe(true);
    expect(agentActivityEventMatches({ actor: "writer" }, "builder", runCorrs)).toBe(false);
  });

  it("filters the live tail with the same attribution rules", () => {
    const runCorrs = agentRunCorrelations([{ correlation_id: "corr-1" }]);
    const got = filterAgentLogEvents(
      [
        { id: "a", actor: "builder" },
        { id: "b", correlation_id: "corr-1", kind: "task.failed" },
        { id: "c", subject: "doctor.auto_repair", payload: { agent: "builder" } },
        { id: "d", subject: "doctor.auto_repair", payload: { target_agent: "builder" } },
        { id: "e", subject: "agent.repair", payload: { agent: "builder" } },
        { id: "f", kind: "agent.retry", payload: { agent: "builder" } },
        { id: "g", kind: "roster.updated", payload: { slug: "builder" } },
        { id: "h", kind: "board.posted", payload: { from: "writer", to: "builder" } },
        { id: "i", actor: "writer" },
      ],
      "builder",
      runCorrs,
      10,
    );
    expect(got.map((row) => row.id)).toEqual(["a", "b", "c", "d", "e", "f", "g", "h"]);
  });

  it("summarizes live run, doctor, delegation, and mailbox pressure", () => {
    expect(agentActivityPulse(
      [{ correlation_id: "r1", status: "running" }, { correlation_id: "r2", status: "completed" }],
      [
        { subject: "doctor.auto_repair", kind: "info" },
        { kind: "agent.retry" },
        { kind: "subagent.spawned" },
        { kind: "board.posted" },
      ],
    )).toEqual({
      liveRuns: 1,
      doctorEvents: 1,
      incidentEvents: 2,
      delegations: 1,
      mailboxEvents: 1,
      value: "1 live run",
      detail: "1 active run · 1 doctor/repair · 2 incident-linked · 1 delegation · 1 mailbox",
      tone: "good",
    });
    expect(agentActivityPulse([], [{ subject: "doctor.auto_repair" }]).value).toBe("1 doctor signal");
    expect(agentActivityPulse([], [{ kind: "subagent.completed" }]).value).toBe("1 delegation");
    expect(agentActivityPulse([], [{ kind: "board.posted" }]).value).toBe("1 mailbox event");
    expect(agentActivityPulse([], [])).toEqual({
      liveRuns: 0,
      doctorEvents: 0,
      incidentEvents: 0,
      delegations: 0,
      mailboxEvents: 0,
      value: "quiet",
      detail: "no live run, doctor, delegation, or mailbox activity in the current event tail",
      tone: "muted",
    });
  });

  it("turns the pulse into a visible operational state", () => {
    expect(agentActivityOperationalState(agentActivityPulse([{ status: "running" }], [{ kind: "board.posted" }]))).toMatchObject({
      value: "awake",
      tone: "accent",
    });
    expect(agentActivityOperationalState(agentActivityPulse([], [{ subject: "doctor.auto_repair" }]))).toMatchObject({
      value: "self-repair",
      tone: "warn",
    });
    expect(agentActivityOperationalState(agentActivityPulse([], [{ kind: "subagent.spawned" }]))).toMatchObject({
      value: "delegating",
      tone: "good",
    });
    expect(agentActivityOperationalState(agentActivityPulse([], [{ kind: "board.posted" }]))).toMatchObject({
      value: "mailbox active",
      tone: "good",
    });
    expect(agentActivityOperationalState(agentActivityPulse([], []))).toEqual({
      value: "sleeping",
      detail: "no active run, repair, delegation, or mailbox event in the current live tail",
      tone: "muted",
    });
  });
});
