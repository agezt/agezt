import { describe, expect, it } from "vitest";
import { fleetAgentLiveOpsSummary, fleetAgentRepairOpsSummary, wakeStateDescriptor } from "@/components/Fleet";

describe("wakeStateDescriptor", () => {
  it("spells out live, sleeping, manual, paused, and retired states", () => {
    expect(
      wakeStateDescriptor(
        { kind: "roster", state: "running", running: true },
        { mode: "cadence", label: "daily 03:00" },
      ),
    ).toMatchObject({ label: "awake now", detail: "cadence: daily 03:00", mode: "running" });

    expect(
      wakeStateDescriptor(
        { kind: "roster", state: "armed", running: false },
        { mode: "cron", label: "0 9 * * *" },
      ),
    ).toMatchObject({ label: "sleeping until trigger", mode: "armed" });

    expect(wakeStateDescriptor({ kind: "roster", state: "manual", running: false })).toMatchObject({
      label: "sleeping",
      detail: "manual / delegated",
      mode: "manual",
    });

    expect(wakeStateDescriptor({ kind: "workflow", state: "paused", running: false })).toMatchObject({
      label: "paused",
      detail: "wake disabled",
      mode: "paused",
    });

    expect(wakeStateDescriptor({ kind: "roster", state: "retired", running: false, retired: true })).toMatchObject({
      label: "graveyard",
      detail: "retired identity",
      mode: "retired",
    });
  });
});

describe("fleetAgentRepairOpsSummary", () => {
  it("prioritizes live repair state and falls back to guarded/manual policy", () => {
    expect(fleetAgentRepairOpsSummary({ retired: true }, null)).toEqual({
      value: "graveyard",
      detail: "repair blocked until revived",
      tone: "muted",
    });
    expect(fleetAgentRepairOpsSummary({}, null)).toEqual({
      value: "manual repair",
      detail: "no retry, doctor, or self-repair policy configured",
      tone: "warn",
    });
    expect(fleetAgentRepairOpsSummary({
      retry_policy: { max_attempts: 3 },
      health_policy: { doctor_agent: "doctor" },
      self_repair: { enabled: true, max_attempts: 2, escalate_to: "lead" },
    }, null)).toEqual({
      value: "repair guarded",
      detail: "retry 3x · doctor doctor · self-repair 2x · escalate lead",
      tone: "good",
    });
    expect(fleetAgentRepairOpsSummary({}, {
      repairInflight: 1,
      repairText: "doctor 1",
      repairKindText: "doctor",
      repairDetail: "attempt 1/2",
      repairTone: "accent",
    } as any)).toEqual({
      value: "doctor 1",
      detail: "doctor · attempt 1/2",
      tone: "accent",
    });
    expect(fleetAgentRepairOpsSummary({}, {
      repairText: "repair exhausted",
      repairKindText: "repair",
      repairDetail: "attempt 2/2",
      repairIncidentDetail: "root lead",
      repairTone: "bad",
    } as any)).toEqual({
      value: "repair exhausted",
      detail: "repair · attempt 2/2 · root lead",
      tone: "bad",
    });
  });
});

describe("fleetAgentLiveOpsSummary", () => {
  it("combines awake/sleep state, health, repair, and next wake into one live ops passport", () => {
    expect(
      fleetAgentLiveOpsSummary(
        { kind: "roster", state: "running", running: true, nextRunMs: 0 },
        {
          activePhase: "using tool",
          liveText: "running 1",
          liveDetail: "shell",
          activeModel: "gpt-5",
          activeTool: "shell",
        } as any,
        { mode: "cadence", label: "hourly" },
        { value: "repair guarded", detail: "retry 3x", tone: "good" },
      ),
    ).toMatchObject({
      value: "awake · using tool",
      tone: "accent",
    });

    expect(
      fleetAgentLiveOpsSummary(
        { kind: "roster", state: "armed", running: false, nextRunMs: Date.UTC(2026, 0, 1, 12, 0, 0) },
        null,
        { mode: "cron", label: "0 * * * *" },
        { value: "manual repair", detail: "no policy", tone: "warn" },
      ),
    ).toMatchObject({
      value: "sleeping · armed",
      tone: "warn",
    });

    expect(
      fleetAgentLiveOpsSummary(
        { kind: "roster", state: "retired", running: false, retired: true },
        null,
        undefined,
        null,
      ),
    ).toEqual({
      value: "graveyard · asleep",
      detail: "retired identity · retired identity is inspectable but asleep",
      tone: "muted",
    });
  });
});
