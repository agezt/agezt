// @vitest-environment jsdom
import { describe, it, expect, vi, afterEach, beforeEach } from "vitest";
import { render, screen, cleanup, fireEvent, waitFor, within } from "@testing-library/react";
import type { ReactNode } from "react";

const getJSON = vi.fn();
const postJSON = vi.fn();
const postAction = vi.fn();
vi.mock("@/lib/api", () => ({
  getJSON: (...a: unknown[]) => getJSON(...a),
  postJSON: (...a: unknown[]) => postJSON(...a),
  postAction: (...a: unknown[]) => postAction(...a),
}));
vi.mock("@/lib/events", () => ({
  useEvents: () => ({ events: [], connected: true, subscribe: () => () => {} }),
}));

import { Roster, NewAgentForm, profileFields, slugOk, usdToMc, agentHue, initials, sortAgentRoster, agentIdentityKind, agentNeedsRepair, agentNeedsAttention, filterAgentRoster, agentEnableToast, agentRemoveToast, agentRetireToast, agentReviveToast, agentNoisePolicySummary, agentNoiseBudgetPassport, agentSchedulePressurePassport, systemGuardianSafetySummary, systemGuardianNoiseContract, systemGuardianRiskSummary, noisySystemGuardians, guardianQuietPolicyPayload, guardianQuietingSummary, agentLifecycleSummary, agentGraveyardStats, agentGraveyardCleanupPassport, agentTaskContractSummary, agentTaskProgressSummary, agentHierarchySummary, rosterWaitingMailboxCounts, agentRemovalCascadePreset, agentRemovalDecisionSummary, agentRemovalImpactSummary, agentRemovalPlan, rosterWakeIssue, rosterRepairIssue, agentHealthIssueSummary, formatWakeDue } from "@/views/Roster";
import { UIProvider } from "@/components/ui/feedback";

const withUI = (node: ReactNode) => <UIProvider>{node}</UIProvider>;

function chooseAgentOption(group: string, name: RegExp | string) {
  fireEvent.click(within(screen.getByRole("group", { name: group })).getByRole("button", { name }));
}

afterEach(cleanup);
beforeEach(() => {
  getJSON.mockReset();
  postJSON.mockReset();
  postAction.mockReset();
  postJSON.mockResolvedValue({});
  postAction.mockResolvedValue({});
});

describe("slugOk", () => {
  it("mirrors the kernel slug rule", () => {
    for (const s of ["researcher", "ops-watcher", "r2.d2", "x_1", "a"]) expect(slugOk(s)).toBe(true);
    for (const s of ["", "Researcher", "has space", "-lead", ".lead", "_lead", "a".repeat(65)])
      expect(slugOk(s)).toBe(false);
  });
});

describe("agentRetireToast", () => {
  it("includes trigger pause counts when retiring muted bound automation", () => {
    expect(agentRetireToast("ops")).toBe("ops retired to the graveyard");
    expect(agentRetireToast("ops", { standing_paused: 2, schedules_paused: 1 })).toBe(
      "ops retired to the graveyard (2 standing, 1 schedule paused)",
    );
  });

  it("keeps revive explicit that automation remains paused", () => {
    expect(agentReviveToast("ops")).toBe("ops revived (paused)");
    expect(agentReviveToast("ops", { standing_paused: 2, schedules_paused: 1 })).toBe(
      "ops revived (paused; 2 standing, 1 schedule still paused)",
    );
  });

  it("keeps resume explicit that bound automation remains paused", () => {
    expect(agentEnableToast("ops", false)).toBe("ops paused");
    expect(agentEnableToast("ops", true)).toBe("ops resumed");
    expect(agentEnableToast("ops", true, { standing_paused: 2, schedules_paused: 1 })).toBe(
      "ops resumed (2 standing, 1 schedule still paused)",
    );
  });

  it("names sub-agents retired during hard removal", () => {
    expect(agentRemoveToast("lead", { removed: false })).toBe("lead was not removed");
    expect(agentRemoveToast("lead", {
      removed: true,
      standing_removed: 1,
      schedules_removed: 1,
      memories_forgotten: 1,
      authored_memories_forgotten: 1,
      skills_archived: 1,
      configs_deleted: 1,
      configs_access_pruned: 2,
      workspaces_deleted: 1,
      subagents_retired: 2,
      subagents_retired_slugs: ["scout", "worker"],
      mailbox_messages_retained: 3,
      workflow_refs_retained: 1,
      subagent_workflow_refs_retained: 1,
    })).toBe("lead removed (identity deleted; audit retained; 1 standing, 1 schedule, 1 private memory, 1 authored shared memory, 1 skill, 1 config, 2 shared config access pruned, 1 workspace, 2 subagent, 3 mailbox/audit retained, 1 workflow ref retained, 1 subagent workflow ref retained; retired: scout, worker)");
    expect(agentRemoveToast("lead", {
      removed: true,
      subagents_retired: 4,
      subagents_retired_slugs: ["a", "b", "c", "d"],
    })).toContain("retired: a, b, c +1");
  });
});

describe("formatWakeDue", () => {
  it("renders upcoming and overdue wake times", () => {
    const now = 1_000_000;
    expect(formatWakeDue(now + 10 * 60_000, now)).toBe("in 10m");
    expect(formatWakeDue(now + 2 * 60 * 60_000 + 5 * 60_000, now)).toBe("in 2h 5m");
    expect(formatWakeDue(now - 3 * 60_000, now)).toBe("overdue 3m");
  });
});

describe("agentHue", () => {
  it("is deterministic and in 0–359", () => {
    expect(agentHue("researcher")).toBe(agentHue("researcher"));
    for (const s of ["a", "ops", "the-long-agent-name"]) {
      const h = agentHue(s);
      expect(h).toBeGreaterThanOrEqual(0);
      expect(h).toBeLessThan(360);
    }
    // Different slugs generally differ (not a hard guarantee, but these do).
    expect(agentHue("researcher")).not.toBe(agentHue("ops"));
  });
});

describe("initials", () => {
  it("uses two name words, else two chars, else the slug", () => {
    expect(initials("The Researcher", "researcher")).toBe("TR");
    expect(initials("Ops", "ops")).toBe("OP");
    expect(initials(undefined, "watcher")).toBe("WA");
    expect(initials("", "qa")).toBe("QA");
  });
});

describe("usdToMc", () => {
  it("converts dollars to USD-microcents ($1 = 1e9)", () => {
    expect(usdToMc("0.50")).toBe(500_000_000);
    expect(usdToMc("$1")).toBe(1_000_000_000);
    expect(usdToMc("")).toBe(0); // blank = no cap
    expect(usdToMc("abc")).toBeNull();
    expect(usdToMc("-1")).toBeNull();
  });
});

describe("sortAgentRoster", () => {
  it("keeps active identities first and graveyard last", () => {
    const got = sortAgentRoster([
      { id: "4", slug: "zombie", enabled: false, retired: true },
      { id: "3", slug: "worker", enabled: true, kind: "subagent", direct_callable: false },
      { id: "1", slug: "guardian", enabled: true, system: true, kind: "system" },
      { id: "5", slug: "paused", enabled: false },
      { id: "2", slug: "coder", enabled: true, kind: "custom" },
    ]).map((p) => p.slug);
    expect(got).toEqual(["guardian", "coder", "worker", "paused", "zombie"]);
  });
});

describe("filterAgentRoster", () => {
  const profiles = [
    { id: "1", slug: "direct", enabled: true },
    { id: "2", slug: "worker", enabled: true, kind: "subagent" as const },
    { id: "3", slug: "guardian", enabled: true, system: true, kind: "system" as const },
    { id: "4", slug: "broken", enabled: true, status: { health_state: "degraded" as const } },
    { id: "5", slug: "paused", enabled: false },
    { id: "6", slug: "old", enabled: false, retired: true },
  ];

  it("filters roster identities by operational class", () => {
    expect(filterAgentRoster(profiles, "direct").map((p) => p.slug)).toEqual(["direct", "broken", "paused"]);
    expect(filterAgentRoster(profiles, "subagents").map((p) => p.slug)).toEqual(["worker"]);
    expect(filterAgentRoster(profiles, "system").map((p) => p.slug)).toEqual(["guardian"]);
    expect(filterAgentRoster(profiles, "repair").map((p) => p.slug)).toEqual(["broken"]);
    expect(filterAgentRoster(profiles, "mailbox", { worker: 2, paused: 1 }).map((p) => p.slug)).toEqual(["worker", "paused"]);
    expect(filterAgentRoster(profiles, "attention", { worker: 2 }, { direct: { detail: "schedule pressure", tone: "warn", active: 1, frequent: 1, frequentIds: ["s1"] } }).map((p) => p.slug)).toEqual(["direct", "worker", "guardian", "broken"]);
    expect(filterAgentRoster(profiles, "paused").map((p) => p.slug)).toEqual(["paused"]);
    expect(filterAgentRoster(profiles, "graveyard").map((p) => p.slug)).toEqual(["old"]);
    expect(agentNeedsRepair({ status: { repair_state: "queued", repair_inflight: 1 } })).toBe(true);
    expect(agentNeedsAttention({ id: "3", slug: "guardian", enabled: true, system: true, noise_policy: { min_notify_severity: "info" } })).toBe(true);
  });
});

describe("agentIdentityKind", () => {
  it("derives identity class from durable behavior fields", () => {
    expect(agentIdentityKind({ system: true })).toBe("system");
    expect(agentIdentityKind({ kind: "system" })).toBe("system");
    expect(agentIdentityKind({ kind: "subagent" })).toBe("subagent");
    expect(agentIdentityKind({ managed: true })).toBe("subagent");
    expect(agentIdentityKind({ direct_callable: false })).toBe("subagent");
    expect(agentIdentityKind({ kind: "custom" })).toBe("custom");
  });
});

describe("agentNoisePolicySummary", () => {
  it("summarizes the controls that keep system agents quiet", () => {
    expect(agentNoisePolicySummary({})).toBe("");
    expect(agentNoisePolicySummary({
      noise_policy: {
        silent_on_success: true,
        disable_memory_writes: true,
        min_notify_severity: "warning",
        min_notify_interval_sec: 14400,
      },
    })).toBe("silent on success · no memory writes · notify >= warning · cooldown 14400s");
  });
});

describe("systemGuardianSafetySummary", () => {
  it("shows whether a system guardian is capped and quiet", () => {
    expect(systemGuardianSafetySummary({ system: false })).toBe("");
    expect(systemGuardianSafetySummary({
      system: true,
      slug: "guardian-health",
      max_cost_mc: 50_000_000,
      max_daily_mc: 50_000_000,
    })).toContain("memory writes enabled");
    expect(systemGuardianSafetySummary({
      system: true,
      slug: "guardian-health",
      max_cost_mc: 50_000_000,
      max_daily_mc: 50_000_000,
      tool_deny: ["memory"],
    })).toContain("no noise policy");
    expect(systemGuardianSafetySummary({
      system: true,
      slug: "guardian-health",
      memory_scope: "system/guardian-health",
      max_cost_mc: 50_000_000,
      max_daily_mc: 50_000_000,
      trust_ceiling: "L2" as const,
      noise_policy: {
        silent_on_success: true,
        disable_memory_writes: true,
        min_notify_severity: "warning",
        min_notify_interval_sec: 28800,
      },
    })).toBe("quiet, memory off, notify >= warning, cooldown >=8h, capped, trust <= L2");
    expect(systemGuardianSafetySummary({
      system: true,
      slug: "guardian-health",
      noise_policy: { min_notify_severity: "info" },
    })).toContain("notify below warning");
    expect(systemGuardianSafetySummary({
      system: true,
      slug: "guardian-health",
      max_cost_mc: 50_000_000,
      max_daily_mc: 50_000_000,
      noise_policy: {
        disable_memory_writes: true,
        min_notify_severity: "warning",
        min_notify_interval_sec: 28800,
      },
    })).toContain("success notifications enabled");
    expect(systemGuardianSafetySummary({
      system: true,
      slug: "guardian-health",
      max_cost_mc: 50_000_000,
      max_daily_mc: 50_000_000,
      noise_policy: {
        silent_on_success: true,
        disable_memory_writes: true,
        min_notify_severity: "warning",
        min_notify_interval_sec: 14400,
      },
    })).toContain("notify cooldown <8h");
    expect(systemGuardianSafetySummary({
      system: true,
      slug: "guardian-health",
      memory_scope: "guardian-health",
      max_cost_mc: 50_000_000,
      max_daily_mc: 50_000_000,
      tool_deny: ["memory"],
      noise_policy: {
        silent_on_success: true,
        disable_memory_writes: true,
        min_notify_severity: "warning",
        min_notify_interval_sec: 28800,
      },
    })).toContain("memory scope not isolated");
    expect(systemGuardianSafetySummary({
      system: true,
      slug: "guardian-health",
      memory_scope: "system/guardian-health",
      max_cost_mc: 100_000_000,
      max_daily_mc: 100_000_000,
      trust_ceiling: "L3",
      tool_deny: ["memory"],
      noise_policy: {
        silent_on_success: true,
        disable_memory_writes: true,
        min_notify_severity: "warning",
        min_notify_interval_sec: 28800,
      },
    })).toBe("review: daily cap too high, run cap too high, trust above L2");
  });

  it("summarizes active guardian risk for the roster band", () => {
    const quiet = {
      id: "g1",
      slug: "guardian-health",
      system: true as const,
      memory_scope: "system/guardian-health",
      max_cost_mc: 50_000_000,
      max_daily_mc: 50_000_000,
      trust_ceiling: "L2" as const,
      tool_deny: ["memory"],
      noise_policy: {
        silent_on_success: true,
        disable_memory_writes: true,
        min_notify_severity: "warning" as const,
        min_notify_interval_sec: 28800,
      },
    };
    expect(systemGuardianRiskSummary([])).toBe("");
    expect(systemGuardianRiskSummary([quiet])).toBe("1 guardian quiet");
    expect(systemGuardianRiskSummary([quiet, { slug: "guardian-routing", system: true, noise_policy: { min_notify_severity: "info" } }])).toBe("1/2 guardian review");
    expect(systemGuardianRiskSummary([quiet, { ...quiet, retired: true }])).toBe("1 guardian quiet");
    expect(guardianQuietingSummary([quiet])).toEqual({
      label: "guardians quiet",
      detail: "1 active guardian · memory off · notify >= warning · cooldown >=8h",
      tone: "good",
      quietTargets: 0,
      frequentSchedules: 0,
    });
    expect(guardianQuietingSummary([quiet, { id: "g2", slug: "guardian-routing", system: true, noise_policy: { min_notify_severity: "info" } }])).toEqual({
      label: "1/2 guardians need quieting",
      detail: "quiet action enforces memory off, isolated system memory scope, notify >= warning, cooldown >=8h, run/day caps, trust <= L2",
      tone: "warn",
      quietTargets: 1,
      frequentSchedules: 0,
    });
    expect(guardianQuietingSummary(
      [quiet, { id: "g2", slug: "guardian-routing", system: true, noise_policy: { min_notify_severity: "info" } }],
      { "guardian-routing": { frequent: 1, frequentIds: ["sched-fast"] } },
    )).toEqual({
      label: "1/2 guardians need quieting",
      detail: "quiet action enforces memory off, isolated system memory scope, notify >= warning, cooldown >=8h, run/day caps, trust <= L2 · 1 frequent guardian schedule will be paused",
      tone: "warn",
      quietTargets: 1,
      frequentSchedules: 1,
    });
    expect(guardianQuietingSummary(
      [quiet],
      { "guardian-health": { frequent: 1, frequentIds: ["sched-fast"] } },
    )).toEqual({
      label: "guardian schedule pressure",
      detail: "1 active guardian quiet · 1 frequent guardian schedule will be paused",
      tone: "warn",
      quietTargets: 0,
      frequentSchedules: 1,
    });
    expect(systemGuardianNoiseContract(
      { slug: "guardian-health", system: true, noise_policy: { min_notify_severity: "info", min_notify_interval_sec: 60 } },
      { active: 2, frequent: 1, detail: "schedule pressure: 1/2 frequent · fastest 1h" },
    )).toEqual({
      label: "guardian noise review",
      detail: "success notifications enabled, memory writes enabled, notify below warning, notify cooldown <8h, no daily cap, no run cap, trust above L2, memory scope not isolated, 1/2 frequent schedule · quiet action disables memory writes, raises notify threshold/cooldown, caps spend, lowers trust, and pauses frequent schedules",
      tone: "warn",
      issues: [
        "success notifications enabled",
        "memory writes enabled",
        "notify below warning",
        "notify cooldown <8h",
        "no daily cap",
        "no run cap",
        "trust above L2",
        "memory scope not isolated",
        "1/2 frequent schedule",
      ],
    });
    expect(systemGuardianNoiseContract(quiet, { active: 1, frequent: 0, detail: "1 schedule · fastest 1d" })).toEqual({
      label: "guardian quiet contract",
      detail: "memory off · notify >= warning · cooldown >=8h · capped · trust <= L2 · 1 schedule · fastest 1d",
      tone: "good",
      issues: [],
    });
  });

  it("turns noise controls into a passport summary", () => {
    expect(agentNoiseBudgetPassport({
      slug: "guardian-health",
      system: true,
      status: { wake_schedule_count: 3 },
      noise_policy: { min_notify_severity: "info", min_notify_interval_sec: 60 },
    }).detail).toBe("noise review: success notifications enabled, memory writes enabled, notify below warning, notify cooldown <8h, no daily cap, no run cap, trust above L2, memory scope not isolated · 3 scheduled wakes");
    expect(agentNoiseBudgetPassport({
      slug: "guardian-routing",
      system: true,
      memory_scope: "system/guardian-routing",
      max_cost_mc: 50_000_000,
      max_daily_mc: 50_000_000,
      trust_ceiling: "L2" as const,
      noise_policy: {
        silent_on_success: true,
        disable_memory_writes: true,
        min_notify_severity: "warning",
        min_notify_interval_sec: 28800,
      },
      status: { wake_schedule_count: 1 },
    })).toEqual({ detail: "quiet budget · memory off · notify >= warning · 1 schedule", tone: "good" });
    expect(agentNoiseBudgetPassport({ slug: "ops", status: { wake_schedule_count: 2 } })).toEqual({
      detail: "default noise · 2 schedules",
      tone: "muted",
    });
  });

  it("summarizes schedule pressure without turning schedules into prompts", () => {
    expect(agentSchedulePressurePassport({ slug: "guardian-health", system: true }, [
      { id: "fast", agent: "guardian-health", mode: "interval", interval_sec: 3600, enabled: true },
      { id: "daily", agent: "guardian-health", mode: "interval", interval_sec: 86400, enabled: true },
      { id: "off", agent: "guardian-health", mode: "interval", interval_sec: 60, enabled: false },
    ])).toEqual({
      detail: "schedule pressure: 1/2 frequent · fastest 1h",
      tone: "warn",
      active: 2,
      frequent: 1,
      frequentIds: ["fast"],
    });
    expect(agentSchedulePressurePassport({ slug: "ops" }, [
      { id: "legacy", intent: "run reports --agent ops", mode: "interval", interval_sec: 3600, enabled: true },
    ])).toEqual({
      detail: "1 schedule · fastest 1h",
      tone: "good",
      active: 1,
      frequent: 0,
      frequentIds: [],
    });
    expect(agentSchedulePressurePassport({ slug: "idle" }, [])).toEqual({
      detail: "no schedules",
      tone: "muted",
      active: 0,
      frequent: 0,
      frequentIds: [],
    });
  });

  it("builds a bulk quiet payload for noisy system guardians", () => {
    const noisy = {
      id: "g1",
      slug: "guardian-health",
      system: true,
      enabled: true,
      trust_ceiling: "L4" as const,
      noise_policy: { min_notify_severity: "info" as const, min_notify_interval_sec: 60 },
      config_overrides: { AGEZT_MODE: "watch" },
    };
    const quiet = {
      id: "g2",
      slug: "guardian-routing",
      system: true,
      enabled: true,
      memory_scope: "system/guardian-routing",
      tool_deny: ["memory"],
      max_cost_mc: 50_000_000,
      max_daily_mc: 50_000_000,
      trust_ceiling: "L2" as const,
      noise_policy: {
        silent_on_success: true,
        disable_memory_writes: true,
        min_notify_severity: "warning" as const,
        min_notify_interval_sec: 28800,
      },
    };
    expect(noisySystemGuardians([noisy, quiet]).map((p) => p.slug)).toEqual(["guardian-health"]);
    expect(guardianQuietPolicyPayload(noisy)).toEqual({
      ref: "guardian-health",
      memory_scope: "system/guardian-health",
      max_cost_mc: 50_000_000,
      max_daily_mc: 50_000_000,
      trust_ceiling: "L2",
      tool_allow: [],
      tool_deny: ["memory"],
      noise_policy: {
        silent_on_success: true,
        disable_memory_writes: true,
        min_notify_severity: "warning",
        min_notify_interval_sec: 28800,
      },
      config_overrides: { AGEZT_MODE: "watch" },
    });
    expect(guardianQuietPolicyPayload({ ...noisy, tool_deny: ["notify", "MEMORY"] }).tool_deny).toEqual(["notify", "memory"]);
    expect(guardianQuietPolicyPayload({ ...noisy, tool_allow: ["memory", "notify"], tool_deny: ["MEMORY"] })).toMatchObject({
      tool_allow: ["notify"],
      tool_deny: ["memory"],
    });
    expect(guardianQuietPolicyPayload({
      ...noisy,
      max_cost_mc: 250_000_000,
      max_daily_mc: 250_000_000,
      trust_ceiling: "L3",
    })).toMatchObject({
      max_cost_mc: 50_000_000,
      max_daily_mc: 50_000_000,
      trust_ceiling: "L2",
    });
  });
});

describe("agentLifecycleSummary", () => {
  it("shows cycle progress and one-shot lifecycle plainly", () => {
    expect(agentLifecycleSummary({})).toBe("persistent");
    expect(agentLifecycleSummary({ lifecycle: { mode: "retire_on_complete", retire_on_complete: true } })).toBe("one-shot");
    expect(agentLifecycleSummary({ lifecycle: { mode: "cycle", completed_cycles: 2, max_cycles: 5 } })).toBe("cycle 2/5");
    expect(agentLifecycleSummary({ lifecycle: { mode: "cycle", completed_cycles: 3 } })).toBe("cycle 3 done");
  });
});

describe("agentGraveyardStats", () => {
  it("summarizes graveyard composition and cleanup posture", () => {
    const stats = agentGraveyardStats([
      { id: "1", slug: "old-custom", retired: true, retired_ms: 200, retired_reason: "superseded" },
      { id: "2", slug: "system-old", retired: true, retired_ms: 100, system: true },
      { id: "3", slug: "worker-old", retired: true, kind: "subagent" },
      { id: "4", slug: "active", retired: false },
    ]);
    expect(stats.count).toBe(3);
    expect(stats.custom).toBe(1);
    expect(stats.system).toBe(1);
    expect(stats.subagents).toBe(1);
    expect(stats.withReason).toBe(1);
    expect(stats.oldest?.slug).toBe("system-old");
    expect(agentGraveyardCleanupPassport([
      { retired: true, retired_reason: "superseded" },
      { retired: true, system: true },
      { retired: false },
    ])).toEqual({
      label: "1 removable",
      detail: "2 retired \u00b7 1 can be removed \u00b7 1 system protected \u00b7 1 without reason",
      tone: "warn",
    });
    expect(agentGraveyardCleanupPassport([])).toEqual({
      label: "graveyard empty",
      detail: "no retired identities are waiting for revive or removal",
      tone: "muted",
    });
  });
});

describe("agentTaskProgressSummary", () => {
  it("summarizes active cycle/total task progress", () => {
    expect(agentTaskProgressSummary()).toBe("");
    expect(agentTaskProgressSummary([{ title: "old", status: "retired" }])).toBe("");
    expect(
      agentTaskProgressSummary([
        { title: "check inbox", scope: "cycle", status: "doing" },
        { title: "finish migration", scope: "total", status: "blocked" },
        { title: "write report", scope: "total", status: "done" },
      ]),
    ).toBe("1 cycle / 2 total / 1 doing / 1 blocked / 1 done");
  });
});

describe("agentTaskContractSummary", () => {
  it("states whether an agent persists, cycles, retires, or sleeps in the graveyard", () => {
    expect(agentTaskContractSummary({})).toBe("persistent agent · stays alive after runs · no durable tasks");
    expect(agentTaskContractSummary({ retired: true })).toBe("graveyard · will not wake until revived");
    expect(agentTaskContractSummary({
      lifecycle: { mode: "cycle", completed_cycles: 2, max_cycles: 4 },
      tasklist: [{ title: "scan", scope: "cycle", status: "todo" }],
    })).toBe("cycle agent · repeats on each wake · 2/4 cycles · retires at max cycles · 1 cycle / 0 total tasks");
    expect(agentTaskContractSummary({
      lifecycle: { mode: "persistent", max_cycles: 3 },
    })).toBe("cycle agent · repeats on each wake · 0/3 cycles · retires at max cycles · no durable tasks");
    expect(agentTaskContractSummary({
      lifecycle: { mode: "retire_on_complete", retire_on_complete: true },
      tasklist: [
        { title: "draft", scope: "total", status: "done" },
        { title: "ship", scope: "total", status: "blocked" },
      ],
    })).toBe("one-shot agent · retires on completion · 0 cycle / 2 total tasks · 1/2 total done · 1 blocked");
  });
});

describe("rosterWaitingMailboxCounts", () => {
  it("counts direct and broadcast unanswered mailbox messages per agent", () => {
    expect(rosterWaitingMailboxCounts([
      { id: "m1", from: "operator", to: "researcher" },
      { id: "m2", from: "operator", to: "researcher", acked_by: ["researcher"] },
      { id: "m3", from: "operator", to: "ops" },
      { id: "m4", from: "researcher", to: "operator", reply_to: "m1" },
      { id: "m5", from: "operator", to: "ops" },
      { id: "m6", from: "operator", to: "*", acked_by: ["researcher"] },
      { id: "m7", from: "ops", to: "*" },
      { id: "m8", from: "researcher", to: "ops", reply_to: "m7" },
    ], ["researcher", "ops", "guardian"])).toEqual({ ops: 3, guardian: 2 });
  });
});

describe("agentHierarchySummary", () => {
  it("separates directly callable agents from managed sub-agents", () => {
    expect(agentHierarchySummary({})).toBe("direct");
    expect(agentHierarchySummary({ owner_agent: "lead" })).toBe("direct · owner lead");
    expect(agentHierarchySummary({ parent_agent: "lead", direct_callable: false })).toBe("managed by lead");
    expect(agentHierarchySummary({ kind: "subagent", parent_agent: "lead" })).toBe("managed by lead");
    expect(agentHierarchySummary({ managed: true, owner_agent: "owner" })).toBe("managed by owner");
  });
});

describe("agentRemovalPlan", () => {
  it("provides explicit cleanup presets for identity deletion", () => {
    expect(agentRemovalCascadePreset("clean_all")).toEqual({
      standing: true,
      schedules: true,
      memory: true,
      authored_memory: true,
      skills: true,
      config: true,
      workspace: true,
      subagents: true,
    });
    expect(agentRemovalCascadePreset("keep_all")).toEqual({
      standing: false,
      schedules: false,
      memory: false,
      authored_memory: false,
      skills: false,
      config: false,
      workspace: false,
      subagents: false,
    });
  });

  it("states cleanup and retained resources including dependent sub-agent bindings", () => {
    const planInput = {
      standing: ["check logs"],
      schedules: ["refresh"],
      memories: ["note"],
      authoredMemories: ["shared note"],
      skills: ["skill"],
      configs: ["agent/ops/runtime"],
      mailboxMessages: ["dm received"],
      subagents: ["ops-worker"],
      subagentStanding: ["ops-worker: watch"],
      subagentSchedules: ["ops-worker: refresh"],
      subagentMemories: ["ops-worker: note"],
      subagentAuthoredMemories: ["ops-worker: shared note"],
      subagentSkills: ["ops-worker: skill"],
      subagentConfigs: ["ops-worker: config"],
      subagentMailboxMessages: ["ops-worker: dm received"],
      cascade: { standing: true, schedules: true, memory: false, authored_memory: false, skills: true, config: true, workspace: false, subagents: true },
    };
    const plan = agentRemovalPlan(planInput);

    expect(plan.cleanupPlan).toEqual([
      "1 standing",
      "1 schedule",
      "1 skill",
      "1 config",
      "shared config access refs",
      "1 sub-agent",
      "1 sub-agent standing",
      "1 sub-agent schedule",
      "1 sub-agent skill",
      "1 sub-agent config",
    ]);
    expect(plan.keepPlan).toEqual([
      "1 private memory",
      "1 authored shared memory",
      "1 mailbox/audit messages",
      "1 sub-agent private memory",
      "1 sub-agent authored shared memory",
      "1 sub-agent mailbox/audit messages",
    ]);
    expect(plan.blockedBySubagents).toBe(false);
    expect(agentRemovalImpactSummary(plan)).toBe("10 cleanup groups · 6 retained groups");
    expect(agentRemovalDecisionSummary(plan)).toEqual({
      label: "remove with retained resources",
      detail: "delete identity · clean 10 groups · keep 1 private memory, 1 authored shared memory, 1 mailbox/audit messages, 1 sub-agent private memory, 1 sub-agent authored shared memory, 1 sub-agent mailbox/audit messages",
      tone: "warn",
    });
    expect(agentRemovalImpactSummary({ cleanupPlan: [], keepPlan: ["1 schedule"], blockedBySubagents: true })).toBe(
      "no cleanup selected · 1 retained group · blocked by dependent sub-agent tree",
    );
    expect(agentRemovalDecisionSummary({ cleanupPlan: [], keepPlan: ["1 schedule"], blockedBySubagents: true })).toEqual({
      label: "removal blocked",
      detail: "dependent sub-agent tree would be orphaned; include it so every descendant retires before this identity is deleted",
      tone: "bad",
    });
  });
});

describe("rosterWakeIssue", () => {
  it("keeps roster quick wake aligned with agent callability", () => {
    expect(rosterWakeIssue({})).toBe("");
    expect(rosterWakeIssue({ enabled: false })).toBe("resume this agent before waking it");
    expect(rosterWakeIssue({ retired: true })).toBe("revive this agent before waking it");
    expect(rosterWakeIssue({ direct_callable: false, parent_agent: "lead" })).toBe(
      "managed sub-agent; wake lead instead",
    );
    expect(rosterWakeIssue({ kind: "subagent", parent_agent: "lead" })).toBe(
      "managed sub-agent; wake lead instead",
    );
  });

  it("keeps roster quick repair aligned with agent ownership", () => {
    expect(rosterRepairIssue({})).toBe("");
    expect(rosterRepairIssue({ retired: true })).toBe("revive this agent before requesting repair");
    expect(rosterRepairIssue({ direct_callable: false, parent_agent: "lead" })).toBe(
      "managed sub-agent; request repair through lead",
    );
    expect(rosterRepairIssue({ kind: "subagent" })).toBe(
      "managed sub-agent; request repair through its parent/owner",
    );
  });
});

describe("agentHealthIssueSummary", () => {
  it("summarizes runtime configuration issues without hiding the first repair target", () => {
    expect(agentHealthIssueSummary({ configIssues: [] })).toBe("");
    expect(agentHealthIssueSummary({ configIssues: ["schedule:sync: bound schedule cannot call managed sub-agent"] })).toBe(
      "schedule:sync: bound schedule cannot call managed sub-agent",
    );
    expect(agentHealthIssueSummary({
      configIssues: [
        "schedule:sync: bound schedule cannot call managed sub-agent",
        "standing:daily: bound standing order targets paused agent",
      ],
    })).toBe("2 issues · schedule:sync: bound schedule cannot call managed sub-agent");
  });
});

describe("NewAgentForm", () => {
  it("disables Create until the slug is valid, then posts the profile shape", async () => {
    const onCreated = vi.fn();
    render(<NewAgentForm onCreated={onCreated} onError={() => {}} />);
    const create = screen.getByRole("button", { name: /Create agent/ }) as HTMLButtonElement;
    expect(create.disabled).toBe(true);

    fireEvent.change(screen.getByLabelText("Agent slug"), { target: { value: "BAD SLUG" } });
    expect((screen.getByRole("button", { name: /Create agent/ }) as HTMLButtonElement).disabled).toBe(true);

    fireEvent.change(screen.getByLabelText("Agent slug"), { target: { value: "researcher" } });
    fireEvent.change(screen.getByLabelText("Agent soul"), { target: { value: "You dig deep." } });
    // Model is chosen via the ModelPicker (a button + modal, M963), not a text
    // input — the model/fallbacks → wire-shape mapping is covered directly by the
    // profileFields unit test below. Here we verify slug gating and the post.
    fireEvent.change(screen.getByLabelText("Max cost per run"), { target: { value: "0.50" } });
    const btn = screen.getByRole("button", { name: /Create agent/ }) as HTMLButtonElement;
    expect(btn.disabled).toBe(false);
    fireEvent.click(btn);

    await waitFor(() =>
      expect(postJSON).toHaveBeenCalledWith(
        "/api/agents/add",
        expect.objectContaining({
          profile: expect.objectContaining({
            slug: "researcher",
            soul: "You dig deep.",
            max_cost_mc: 500_000_000,
          }),
        }),
      ),
    );
    expect(onCreated).toHaveBeenCalledWith("researcher");
  });

  it("previews the durable task contract while creating an agent", () => {
    render(<NewAgentForm onCreated={() => {}} onError={() => {}} />);

    expect(screen.getByLabelText("Task contract preview").textContent).toContain(
      "persistent agent · stays alive after runs · no durable tasks",
    );

    chooseAgentOption("Agent lifecycle", /One-shot/);
    fireEvent.change(screen.getByLabelText("Every-cycle tasks"), { target: { value: "check inbox" } });
    fireEvent.change(screen.getByLabelText("Total tasklist"), { target: { value: "ship migration\nwrite report" } });

    expect(screen.getByLabelText("Task contract preview").textContent).toContain(
      "one-shot agent · retires on completion · 1 cycle / 2 total tasks · 0/2 total done",
    );
  });

  it("treats max cycles as a cycle contract in the create form payload", () => {
    render(<NewAgentForm onCreated={() => {}} onError={() => {}} />);

    chooseAgentOption("Agent lifecycle", /Persistent/);
    fireEvent.change(screen.getByLabelText("Max cycles"), { target: { value: "3" } });

    expect(screen.getByLabelText("Task contract preview").textContent).toContain(
      "cycle agent · repeats on each wake · 0/3 cycles · retires at max cycles",
    );

    const out = profileFields({
      name: "",
      soul: "",
      instructions: "",
      model: "",
      fallbacks: "",
      taskType: "",
      maxCost: "",
      maxDaily: "",
      memoryScope: "",
      workdir: "",
      ownerAgent: "",
      parentAgent: "",
      directCallable: "true",
      retryAttempts: "",
      retryBackoff: "",
      retryBaseDelay: "",
      retryMaxDelay: "",
      healthDoctor: "",
      healthFailureThreshold: "",
      selfRepairEnabled: "",
      selfRepairAttempts: "",
      selfRepairEscalate: "",
      trustCeiling: "L4",
      toolAllow: "",
      toolDeny: "",
      configOverrides: "",
      lifecycleMode: "persistent",
      lifecycleMaxCycles: "3",
      cycleTasks: "",
      totalTasks: "",
      description: "",
    }) as Record<string, unknown>;
    expect(out).toMatchObject({ lifecycle: { mode: "cycle", retire_on_complete: false, max_cycles: 3 } });
  });

  it("profileFields maps model + fallbacks, and treats a @chain model as self-contained", () => {
    const base = {
      name: "",
      soul: "",
      instructions: "",
      taskType: "",
      maxCost: "",
      maxDaily: "",
      memoryScope: "",
      workdir: "",
      ownerAgent: "",
      parentAgent: "",
      directCallable: "true",
      retryAttempts: "",
      retryBackoff: "",
      retryBaseDelay: "",
      retryMaxDelay: "",
      healthDoctor: "",
      healthFailureThreshold: "",
      selfRepairEnabled: "",
      selfRepairAttempts: "",
      selfRepairEscalate: "",
      trustCeiling: "L4",
      toolAllow: "",
      toolDeny: "",
      configOverrides: "",
      lifecycleMode: "",
      lifecycleMaxCycles: "",
      cycleTasks: "",
      totalTasks: "",
      description: "",
    };
    // Plain model: fallbacks flow through as a trimmed list.
    const plain = profileFields({ ...base, model: "m-1", fallbacks: "m2, m3" });
    expect(plain).toMatchObject({ model: "m-1", fallbacks: ["m2", "m3"] });
    // Chain reference: model is the @ref and fallbacks are dropped (the chain
    // carries its own ladder).
    const chain = profileFields({ ...base, model: "@fast", fallbacks: "m2, m3" }) as Record<string, unknown>;
    expect(chain.model).toBe("@fast");
    expect(chain.fallbacks).toBeUndefined();
  });

  it("profileFields carries agent hierarchy and direct-call policy", () => {
    const out = profileFields({
      name: "",
      soul: "",
      instructions: "",
      model: "",
      fallbacks: "",
      taskType: "",
      maxCost: "",
      maxDaily: "",
      memoryScope: "",
      workdir: "",
      ownerAgent: "lead",
      parentAgent: "lead",
      directCallable: "false",
      retryAttempts: "",
      retryBackoff: "",
      retryBaseDelay: "",
      retryMaxDelay: "",
      healthDoctor: "",
      healthFailureThreshold: "",
      selfRepairEnabled: "",
      selfRepairAttempts: "",
      selfRepairEscalate: "",
      trustCeiling: "L4",
      toolAllow: "",
      toolDeny: "",
      configOverrides: "",
      lifecycleMode: "",
      lifecycleMaxCycles: "",
      cycleTasks: "",
      totalTasks: "",
      description: "",
    }) as Record<string, unknown>;
    expect(out).toMatchObject({ owner_agent: "lead", parent_agent: "lead", direct_callable: false });
  });

  it("profileFields rejects unmanaged managed subagents", () => {
    const out = profileFields({
      name: "",
      soul: "",
      instructions: "",
      model: "",
      fallbacks: "",
      taskType: "",
      maxCost: "",
      maxDaily: "",
      memoryScope: "",
      workdir: "",
      ownerAgent: "",
      parentAgent: "",
      directCallable: "false",
      retryAttempts: "",
      retryBackoff: "",
      retryBaseDelay: "",
      retryMaxDelay: "",
      healthDoctor: "",
      healthFailureThreshold: "",
      selfRepairEnabled: "",
      selfRepairAttempts: "",
      selfRepairEscalate: "",
      trustCeiling: "L4",
      toolAllow: "",
      toolDeny: "",
      configOverrides: "",
      lifecycleMode: "",
      lifecycleMaxCycles: "",
      cycleTasks: "",
      totalTasks: "",
      description: "",
    });
    expect(out).toBe("managed sub-agent needs an owner or parent agent");
  });

  it("profileFields carries retry, health, and self-repair policies", () => {
    const out = profileFields({
      name: "",
      soul: "",
      instructions: "",
      model: "",
      fallbacks: "",
      taskType: "",
      maxCost: "",
      maxDaily: "",
      memoryScope: "",
      workdir: "",
      ownerAgent: "",
      parentAgent: "",
      directCallable: "true",
      retryAttempts: "3",
      retryBackoff: "exponential",
      retryBaseDelay: "30",
      retryMaxDelay: "300",
      retryOn: "error, timeout",
      healthDoctor: "guardian-doctor",
      healthFailureThreshold: "5",
      selfRepairEnabled: "true",
      selfRepairAttempts: "2",
      selfRepairEscalate: "lead",
      trustCeiling: "L4",
      toolAllow: "",
      toolDeny: "",
      configOverrides: "",
      lifecycleMode: "",
      lifecycleMaxCycles: "",
      cycleTasks: "",
      totalTasks: "",
      description: "",
    }) as Record<string, unknown>;
    expect(out).toMatchObject({
      retry_policy: { max_attempts: 3, backoff: "exponential", base_delay_sec: 30, max_delay_sec: 300, retry_on: ["error", "timeout"] },
      health_policy: { doctor_agent: "guardian-doctor", failure_threshold: 5 },
      self_repair: { enabled: true, max_attempts: 2, escalate_to: "lead" },
    });
  });

  it("profileFields carries instructions, lifecycle, and task lists", () => {
    const out = profileFields({
      name: "",
      soul: "",
      instructions: "Stay quiet\nReport changes only",
      model: "",
      fallbacks: "",
      taskType: "",
      maxCost: "",
      maxDaily: "",
      memoryScope: "",
      workdir: "",
      ownerAgent: "",
      parentAgent: "",
      directCallable: "true",
      retryAttempts: "",
      retryBackoff: "",
      retryBaseDelay: "",
      retryMaxDelay: "",
      healthDoctor: "",
      healthFailureThreshold: "",
      selfRepairEnabled: "",
      selfRepairAttempts: "",
      selfRepairEscalate: "",
      trustCeiling: "L4",
      toolAllow: "",
      toolDeny: "",
      configOverrides: "",
      lifecycleMode: "cycle",
      lifecycleMaxCycles: "12",
      cycleTasks: "check inbox",
      totalTasks: "finish migration",
      description: "",
    }) as Record<string, unknown>;
    expect(out).toMatchObject({
      instructions: ["Stay quiet", "Report changes only"],
      lifecycle: { mode: "cycle", retire_on_complete: false, max_cycles: 12 },
      tasklist: [
        { title: "check inbox", scope: "cycle", status: "todo" },
        { title: "finish migration", scope: "total", status: "todo" },
      ],
    });
  });

  it("profileFields carries tool permissions and trust ceiling", () => {
    const out = profileFields({
      name: "",
      soul: "",
      instructions: "",
      model: "",
      fallbacks: "",
      taskType: "",
      maxCost: "",
      maxDaily: "",
      memoryScope: "",
      workdir: "",
      ownerAgent: "",
      parentAgent: "",
      directCallable: "true",
      retryAttempts: "",
      retryBackoff: "",
      retryBaseDelay: "",
      retryMaxDelay: "",
      healthDoctor: "",
      healthFailureThreshold: "",
      selfRepairEnabled: "",
      selfRepairAttempts: "",
      selfRepairEscalate: "",
      trustCeiling: "L2",
      toolAllow: "shell, memory, SHELL",
      toolDeny: "notify, NOTIFY",
      configOverrides: "AGEZT_X_MODE=agent-only",
      lifecycleMode: "",
      lifecycleMaxCycles: "",
      cycleTasks: "",
      totalTasks: "",
      description: "",
    }) as Record<string, unknown>;
    expect(out).toMatchObject({
      trust_ceiling: "L2",
      tool_allow: ["shell", "memory"],
      tool_deny: ["notify"],
      config_overrides: { AGEZT_X_MODE: "agent-only" },
    });
  });

  it("rejects a bad max-cost without posting", async () => {
    const onError = vi.fn();
    render(<NewAgentForm onCreated={() => {}} onError={onError} />);
    fireEvent.change(screen.getByLabelText("Agent slug"), { target: { value: "ops" } });
    fireEvent.change(screen.getByLabelText("Max cost per run"), { target: { value: "lots" } });
    fireEvent.click(screen.getByRole("button", { name: /Create agent/ }));
    await waitFor(() => expect(onError).toHaveBeenCalled());
    expect(postJSON).not.toHaveBeenCalled();
  });
});

describe("Roster", () => {
  it("renders profiles from /api/agents with pills, a details disclosure, and no prose surfaces", async () => {
    getJSON.mockImplementation((path: string) => {
      if (path === "/api/skills") return Promise.resolve({ skills: [{ id: "sk-1", name: "Web Scan", agent: "researcher" }] });
      if (path === "/api/board") {
        return Promise.resolve({
          messages: [
            { id: "mail-1", from: "operator", to: "researcher", text: "check queue", ts_unix_ms: 3 },
            { id: "mail-2", from: "operator", to: "researcher", text: "already handled", ts_unix_ms: 2 },
            { id: "mail-3", from: "researcher", to: "operator", reply_to: "mail-2", text: "done", ts_unix_ms: 4 },
            { id: "mail-4", from: "operator", to: "*", acked_by: ["researcher"], text: "fleet heads up", ts_unix_ms: 5 },
          ],
        });
      }
      if (path === "/api/schedules") {
        return Promise.resolve({
          schedules: [
            { id: "sched-fast", agent: "researcher", mode: "interval", interval_sec: 300, enabled: true },
            { id: "sched-paused", agent: "ops", mode: "interval", interval_sec: 60, enabled: false },
          ],
        });
      }
      return Promise.resolve({
        profiles: [
          {
            id: "01A", slug: "researcher", name: "The Researcher", enabled: true,
            model: "m-1", task_type: "research", max_cost_mc: 500_000_000,
            soul: "You dig deep.", fallbacks: ["m2"], parent_agent: "lead", direct_callable: false,
            kind: "subagent", managed: true, memory_scope: "researcher",
            retry_policy: { max_attempts: 3 },
            health_policy: { doctor_agent: "guardian-doctor" },
            self_repair: { enabled: true },
            status: {
              health_state: "misconfigured",
              health_label: "misconfigured",
              misconfiguration_count: 2,
              config_issues: [
                "schedule:sync: bound schedule cannot call managed sub-agent",
                "standing:daily: bound standing order targets paused agent",
              ],
              repair_state: "queued",
              repair_inflight: 1,
              repair_incident_id: "inc-child-1",
              repair_root_incident_id: "inc-root-1",
              repair_root_agent: "researcher",
              repair_chain_depth: 0,
              wake_schedule_count: 1,
              next_wake_ms: Date.now() + 10 * 60_000,
              next_wake_label: "scheduled sync",
            },
            noise_policy: { silent_on_success: true, disable_memory_writes: true, min_notify_severity: "warning", min_notify_interval_sec: 28800 },
            lifecycle: { mode: "cycle", completed_cycles: 2, max_cycles: 5 },
            trust_ceiling: "L2",
            tool_allow: ["memory", "shell"],
            tool_deny: ["notify"],
            config_overrides: { AGEZT_X_MODE: "agent-only" },
            tasklist: [
              { title: "check inbox", scope: "cycle", status: "doing" },
              { title: "finish migration", scope: "total", status: "blocked" },
            ],
          },
          { id: "01B", slug: "ops", enabled: false, kind: "custom" },
        ],
        count: 2,
        enabled_count: 1,
      });
    });
    render(withUI(<Roster />));
    // "researcher" appears as the card slug and as the raw memory-scope value
    // inside the details disclosure.
    await waitFor(() => expect(screen.getAllByText("researcher").length).toBeGreaterThan(0));
    expect(screen.getByText("ops")).toBeTruthy();
    expect(screen.getByText("The Researcher")).toBeTruthy();
    expect(screen.getByText("enabled")).toBeTruthy();
    // "paused" appears both as the ops badge and the summary-band stat label.
    expect(screen.getAllByText("paused").length).toBeGreaterThan(0);

    // Glance pills on the card header.
    expect(screen.getByText("managed sub-agent")).toBeTruthy();
    expect(screen.getByText("custom")).toBeTruthy();
    expect(screen.getByText("1 skill")).toBeTruthy();
    expect(screen.getByText("quiet policy")).toBeTruthy();
    expect(screen.getByText("quiet policy").getAttribute("title")).toBe(
      "silent on success \u00b7 no memory writes \u00b7 notify >= warning \u00b7 cooldown 28800s",
    );
    expect(screen.getByText("self-repair")).toBeTruthy();
    expect(screen.getByText("ceiling L2")).toBeTruthy();
    expect(screen.getByText("cfg/links !2")).toBeTruthy();
    expect(screen.getByText("repair 1")).toBeTruthy();
    expect(screen.getByText("wake 1")).toBeTruthy();
    expect(screen.getByText("cfg 1")).toBeTruthy();
    expect(screen.getByText("cycle 2/5")).toBeTruthy();
    expect(screen.getAllByText("inbox 1").length).toBeGreaterThanOrEqual(2);
    expect(screen.getAllByText("sleeping").length).toBeGreaterThanOrEqual(2);
    expect(screen.getByText("You dig deep.")).toBeTruthy();

    // The prose identity surfaces are gone.
    expect(screen.queryByLabelText(/identity manifest/)).toBeNull();
    expect(screen.queryByLabelText(/identity ledger/)).toBeNull();
    expect(screen.queryByLabelText(/identity card passports/)).toBeNull();
    expect(screen.queryByLabelText(/lifecycle rail/i)).toBeNull();
    expect(screen.queryByLabelText(/command strip/)).toBeNull();
    expect(screen.queryByText("Now")).toBeNull();
    expect(screen.queryByText(/subagent identity \u00b7 managed by lead/)).toBeNull();

    // One "details" disclosure per card holding the raw facts. Disclosure
    // children stay mounted while collapsed, so plain getByText finds them.
    expect(screen.getAllByText("details").length).toBe(2);
    expect(screen.getByText("model")).toBeTruthy();
    expect(screen.getByText("m-1")).toBeTruthy();
    expect(screen.getByText("fallbacks")).toBeTruthy();
    expect(screen.getByText("m2")).toBeTruthy();
    expect(screen.getByText("task type")).toBeTruthy();
    expect(screen.getByText("research")).toBeTruthy();
    expect(screen.getByText("per-run cap")).toBeTruthy();
    expect(screen.getByText("$0.5000")).toBeTruthy();
    expect(screen.getByText("parent")).toBeTruthy();
    expect(screen.getByText("lead")).toBeTruthy();
    expect(screen.getByText("memory scope")).toBeTruthy();
    expect(screen.getAllByText("researcher").length).toBeGreaterThanOrEqual(2);
    expect(screen.getByText("tools")).toBeTruthy();
    expect(screen.getByText("2 allow \u00b7 1 deny")).toBeTruthy();
    expect(screen.getByText("config overrides")).toBeTruthy();
    expect(screen.getByText("trust ceiling")).toBeTruthy();
    expect(screen.getByText("L2")).toBeTruthy();

    // Summary band + filter tabs still expose inbox/attention rollups; counts
    // depend on async board + schedule data.
    expect(screen.getAllByText("Inbox").length).toBeGreaterThan(0);
    expect(screen.getAllByText("Attention").length).toBeGreaterThan(0);
    expect(await screen.findByRole("tab", { name: /Attention2/ })).toBeTruthy();
    expect(await screen.findByRole("tab", { name: /Inbox2/ })).toBeTruthy();

    // The repair-incident pill still deep-links to the root incident.
    fireEvent.click(screen.getByRole("button", { name: "incident" }));
    expect(location.hash).toBe("#incident/inc-root-1");

    // Pause-frequent-schedules stays wired from the card footer.
    fireEvent.click(screen.getByLabelText("Pause frequent schedules for researcher"));
    fireEvent.click(await screen.findByRole("button", { name: "Pause schedules" }));
    await waitFor(() =>
      expect(postAction).toHaveBeenCalledWith("/api/schedule/enable", { id: "sched-fast", enabled: "false" }),
    );
    expect(getJSON).toHaveBeenCalledWith("/api/agents", undefined, expect.objectContaining({ timeoutMs: expect.any(Number) }));
  });

  it("pause posts /api/agents/enable with ref + enabled=false", async () => {
    getJSON.mockResolvedValue({
      profiles: [{ id: "01A", slug: "researcher", enabled: true }],
      count: 1, enabled_count: 1,
    });
    render(withUI(<Roster />));
    await waitFor(() => expect(screen.getByText("researcher")).toBeTruthy());
    fireEvent.click(screen.getByRole("button", { name: "Pause researcher" }));
    await waitFor(() =>
      expect(postAction).toHaveBeenCalledWith("/api/agents/enable", { ref: "researcher", enabled: "false" }),
    );
  });

  it("quick wake posts /api/agents/wake and opens inline activity", async () => {
    postAction.mockResolvedValueOnce({ correlation_id: "run-roster-wake" });
    getJSON.mockImplementation((path: string) => {
      if (path === "/api/agents") {
        return Promise.resolve({
          profiles: [{ id: "01A", slug: "researcher", enabled: true }],
          count: 1,
          enabled_count: 1,
        });
      }
      if (path === "/api/agents/activity") return Promise.resolve({ activity: [] });
      if (path === "/api/runs") return Promise.resolve({ runs: [] });
      return Promise.resolve({});
    });
    render(withUI(<Roster />));
    await waitFor(() => expect(screen.getByText("researcher")).toBeTruthy());
    fireEvent.click(screen.getByRole("button", { name: "Wake researcher" }));
    await waitFor(() =>
      expect(postAction).toHaveBeenCalledWith("/api/agents/wake", {
        ref: "researcher",
        reason: "manual operator wake",
      }),
    );
    await waitFor(() => expect(getJSON).toHaveBeenCalledWith("/api/agents/activity", { ref: "researcher", limit: "60" }));
    expect(await screen.findByText("focused run")).toBeTruthy();
    expect(screen.getByText("run-roster-wake")).toBeTruthy();
  });

  it("quick repair posts /api/agents/repair and opens inline activity", async () => {
    postJSON.mockResolvedValueOnce({ correlation_id: "run-roster-repair" });
    getJSON.mockImplementation((path: string) => {
      if (path === "/api/agents") {
        return Promise.resolve({
          profiles: [{ id: "01A", slug: "researcher", enabled: true, status: { health_state: "degraded" } }],
          count: 1,
          enabled_count: 1,
        });
      }
      if (path === "/api/agents/activity") return Promise.resolve({ activity: [] });
      if (path === "/api/runs") return Promise.resolve({ runs: [] });
      return Promise.resolve({});
    });
    render(withUI(<Roster />));
    await waitFor(() => expect(screen.getByText("researcher")).toBeTruthy());
    fireEvent.click(screen.getByRole("button", { name: "Repair researcher" }));
    await waitFor(() =>
      expect(postJSON).toHaveBeenCalledWith("/api/agents/repair", {
        ref: "researcher",
        reason: "operator requested repair from researcher roster card",
      }),
    );
    await waitFor(() => expect(getJSON).toHaveBeenCalledWith("/api/agents/activity", { ref: "researcher", limit: "60" }));
    expect(await screen.findByText("focused run")).toBeTruthy();
    expect(screen.getByText("run-roster-repair")).toBeTruthy();
  });

  it("disables quick wake for managed sub-agents", async () => {
    getJSON.mockResolvedValue({
      profiles: [{ id: "01W", slug: "worker", enabled: true, direct_callable: false, parent_agent: "lead" }],
      count: 1,
      enabled_count: 1,
    });
    render(withUI(<Roster />));
    await waitFor(() => expect(screen.getByText("worker")).toBeTruthy());
    expect(screen.getByText("managed sub-agent")).toBeTruthy();
    expect(screen.queryByText("custom")).toBeNull();
    const wake = screen.getByRole("button", { name: "Wake worker" }) as HTMLButtonElement;
    expect(wake.disabled).toBe(true);
    expect(wake.title).toBe("managed sub-agent; wake lead instead");
    fireEvent.click(wake);
    expect(postAction).not.toHaveBeenCalledWith("/api/agents/wake", expect.anything());
  });

  it("disables quick repair for managed sub-agents", async () => {
    getJSON.mockResolvedValue({
      profiles: [{ id: "01W", slug: "worker", enabled: true, direct_callable: false, parent_agent: "lead" }],
      count: 1,
      enabled_count: 1,
    });
    render(withUI(<Roster />));
    await waitFor(() => expect(screen.getByText("worker")).toBeTruthy());
    const repair = screen.getByRole("button", { name: "Repair worker" }) as HTMLButtonElement;
    expect(repair.disabled).toBe(true);
    expect(repair.title).toBe("managed sub-agent; request repair through lead");
    fireEvent.click(repair);
    expect(postJSON).not.toHaveBeenCalledWith("/api/agents/repair", expect.anything());
  });

  it("surfaces a running agent's current work context on the roster card", async () => {
    getJSON.mockResolvedValue({
      profiles: [{
        id: "01R",
        slug: "runner",
        enabled: true,
        status: {
          active_run_count: 1,
          active_phase: "using tool",
          active_intent: "sync catalog",
          active_detail: "models.dev/api.json",
          active_wake_source: "schedule",
          active_wake_reason: "interval",
          active_schedule_id: "sched-catalog",
          active_model: "gpt-5",
          active_correlation_id: "corr-runner",
          active_started_ms: Date.UTC(2026, 0, 1, 12, 0, 0),
          active_last_event_ms: Date.UTC(2026, 0, 1, 12, 1, 0),
        },
      }],
      count: 1,
      enabled_count: 1,
    });
    render(withUI(<Roster />));

    await waitFor(() => expect(screen.getByText("runner")).toBeTruthy());
    // The live pill carries the run context (phase, intent, wake source) in
    // its title now that the prose Now band is gone.
    const live = screen.getByText("running 1");
    expect(live.getAttribute("title")).toContain("using tool");
    expect(live.getAttribute("title")).toContain("sync catalog");
    expect(live.getAttribute("title")).toContain("models.dev/api.json");
    expect(live.getAttribute("title")).toContain("source: schedule");
    expect(screen.queryByText("Now")).toBeNull();
  });

  it("keeps system agents retireable but hides hard remove", async () => {
    getJSON.mockResolvedValue({
      profiles: [{
        id: "01G",
        slug: "guardian-health",
        enabled: true,
        system: true,
        kind: "system",
        memory_scope: "system/guardian-health",
        tool_deny: ["memory"],
        max_cost_mc: 50_000_000,
        max_daily_mc: 50_000_000,
        trust_ceiling: "L2",
        noise_policy: {
          silent_on_success: true,
          disable_memory_writes: true,
          min_notify_severity: "warning",
          min_notify_interval_sec: 28800,
        },
      }],
      count: 1,
      enabled_count: 1,
    });
    render(withUI(<Roster />));
    await waitFor(() => expect(screen.getByText("guardian-health")).toBeTruthy());

    expect(screen.getByRole("button", { name: "Retire guardian-health" })).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Remove guardian-health" })).toBeNull();
    // The guardian quieting posture now lives under the "Guardian noise" section
    // (its quiet headline + detail), instead of standalone label badges.
    expect(screen.getByText("Guardian noise")).toBeTruthy();
    expect(screen.getByText("1 active guardian · memory off · notify >= warning · cooldown >=8h")).toBeTruthy();
    expect(screen.getByText("system safety")).toBeTruthy();
    expect(screen.getByText("system safety").getAttribute("title")).toBe("quiet, memory off, notify >= warning, cooldown >=8h, capped, trust <= L2");
  });

  it("can quiet noisy system guardians in bulk from the roster", async () => {
    getJSON.mockImplementation((path: string) => {
      if (path === "/api/agents") {
        return Promise.resolve({
          profiles: [
            {
              id: "01G",
              slug: "guardian-health",
              enabled: true,
              system: true,
              kind: "system",
              trust_ceiling: "L4",
              memory_scope: "system/guardian-health",
              noise_policy: { min_notify_severity: "info", min_notify_interval_sec: 0 },
            },
            {
              id: "02G",
              slug: "guardian-routing",
              enabled: true,
              system: true,
              kind: "system",
              memory_scope: "system/guardian-routing",
              tool_deny: ["memory"],
              max_cost_mc: 50_000_000,
              max_daily_mc: 50_000_000,
      trust_ceiling: "L2" as const,
              noise_policy: {
                silent_on_success: true,
                disable_memory_writes: true,
                min_notify_severity: "warning",
                min_notify_interval_sec: 28800,
              },
            },
          ],
          count: 2,
          enabled_count: 2,
        });
      }
      if (path === "/api/skills") return Promise.resolve({ skills: [] });
      if (path === "/api/schedules")
        return Promise.resolve({
          schedules: [
            { id: "guardian-fast", agent: "guardian-health", mode: "interval", interval_sec: 3600, enabled: true },
            { id: "guardian-quiet", agent: "guardian-routing", mode: "interval", interval_sec: 86400, enabled: true },
          ],
        });
      return Promise.resolve({});
    });
    render(withUI(<Roster />));
    // The noisy-guardian quieting state now surfaces under the "Guardian noise"
    // section (headline + detail + quiet-target / frequent-schedule badges).
    await waitFor(() => expect(screen.getByText("Guardian noise")).toBeTruthy());
    expect(screen.getByText("quiet action enforces memory off, isolated system memory scope, notify >= warning, cooldown >=8h, run/day caps, trust <= L2 · 1 frequent guardian schedule will be paused")).toBeTruthy();
    expect(screen.getByText("1 quiet target")).toBeTruthy();
    expect(screen.getByText("1 frequent schedule")).toBeTruthy();
    expect(screen.getByText("guardian noise review")).toBeTruthy();
    expect(screen.getAllByText(/memory writes enabled, notify below warning, notify cooldown <8h/).length).toBeGreaterThan(0);
    expect(screen.getAllByText("guardian-health").length).toBeGreaterThan(1);

    fireEvent.click(screen.getByRole("button", { name: "Quiet noisy guardians" }));

    await waitFor(() =>
      expect(postJSON).toHaveBeenCalledWith("/api/agents/capabilities", {
        ref: "guardian-health",
        memory_scope: "system/guardian-health",
        max_cost_mc: 50_000_000,
        max_daily_mc: 50_000_000,
        trust_ceiling: "L2",
        tool_allow: [],
        tool_deny: ["memory"],
        noise_policy: {
          silent_on_success: true,
          disable_memory_writes: true,
          min_notify_severity: "warning",
          min_notify_interval_sec: 28800,
        },
        config_overrides: {},
      }),
    );
    expect(postJSON).toHaveBeenCalledTimes(1);
    await waitFor(() =>
      expect(postAction).toHaveBeenCalledWith("/api/schedule/enable", { id: "guardian-fast", enabled: "false" }),
    );
  });

  it("pauses frequent schedules even when system guardians are otherwise quiet", async () => {
    getJSON.mockImplementation((path: string) => {
      if (path === "/api/agents") {
        return Promise.resolve({
          profiles: [{
            id: "01G",
            slug: "guardian-health",
            enabled: true,
            system: true,
            kind: "system",
            memory_scope: "system/guardian-health",
            tool_deny: ["memory"],
            max_cost_mc: 50_000_000,
            max_daily_mc: 50_000_000,
            trust_ceiling: "L2" as const,
            noise_policy: {
              silent_on_success: true,
              disable_memory_writes: true,
              min_notify_severity: "warning",
              min_notify_interval_sec: 28800,
            },
          }],
          count: 1,
          enabled_count: 1,
        });
      }
      if (path === "/api/skills") return Promise.resolve({ skills: [] });
      if (path === "/api/schedules")
        return Promise.resolve({
          schedules: [
            { id: "guardian-fast", agent: "guardian-health", mode: "interval", interval_sec: 3600, enabled: true },
          ],
        });
      return Promise.resolve({});
    });
    render(withUI(<Roster />));
    // The schedule-pressure quieting state surfaces under the "Guardian noise" section.
    await waitFor(() => expect(screen.getByText("Guardian noise")).toBeTruthy());
    expect(screen.getByText("1 active guardian quiet · 1 frequent guardian schedule will be paused")).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: "Quiet noisy guardians" }));

    await waitFor(() =>
      expect(postAction).toHaveBeenCalledWith("/api/schedule/enable", { id: "guardian-fast", enabled: "false" }),
    );
    expect(postJSON).not.toHaveBeenCalledWith("/api/agents/capabilities", expect.anything());
  });

  it("retires an agent with an operator reason", async () => {
    getJSON.mockImplementation((path: string) => {
      if (path === "/api/agents") {
        return Promise.resolve({
          profiles: [{ id: "01A", slug: "ops", enabled: true }],
          count: 1,
          enabled_count: 1,
        });
      }
      if (path === "/api/skills") return Promise.resolve({ skills: [] });
      if (path === "/api/agents/impact") {
        return Promise.resolve({
          standing_orders: ["check logs"],
          standing_count: 1,
          schedules: ["refresh (sch-1)"],
          schedule_count: 1,
          memories: ["ops note (mem-1)"],
          memory_count: 1,
          authored_shared_memories: ["ops shared note (mem-shared-1)"],
          authored_shared_memory_count: 1,
          skills: ["ops skill (skill-1)"],
          skill_count: 1,
          configs: ["agent/ops/runtime [internal]"],
          config_count: 1,
          workflow_refs: ["ops-flow/handoff handoff ops [tool]"],
          workflow_ref_count: 1,
          mailbox_messages: ["dm received (msg-1)"],
          mailbox_message_count: 1,
          subagents: ["ops-worker [parent]"],
          subagent_count: 1,
          subagent_standing_orders: ["ops-worker: worker watch"],
          subagent_standing_count: 1,
          subagent_schedules: ["ops-worker: worker refresh (sch-2)"],
          subagent_schedule_count: 1,
          subagent_memories: ["ops-worker: worker note (mem-2)"],
          subagent_memory_count: 1,
          subagent_authored_shared_memories: ["ops-worker: worker shared note (mem-shared-2)"],
          subagent_authored_shared_memory_count: 1,
          subagent_skills: ["ops-worker: worker skill (skill-2)"],
          subagent_skill_count: 1,
          subagent_configs: ["ops-worker: agent/ops-worker/runtime [internal]"],
          subagent_config_count: 1,
          subagent_workflow_refs: ["ops-worker: worker-flow/delegate [tool]"],
          subagent_workflow_ref_count: 1,
          subagent_mailbox_messages: ["ops-worker: dm received (msg-2)"],
          subagent_mailbox_message_count: 1,
        });
      }
      return Promise.resolve({});
    });

    render(withUI(<Roster />));
    await waitFor(() => expect(screen.getByText("ops")).toBeTruthy());
    fireEvent.click(screen.getByRole("button", { name: "Retire ops" }));

    await waitFor(() => expect(screen.getByRole("dialog", { name: "Retire ops to the graveyard" })).toBeTruthy());
    expect(screen.getByText("check logs")).toBeTruthy();
    expect(screen.getByText("refresh (sch-1)")).toBeTruthy();
    expect(screen.getByText("ops note (mem-1)")).toBeTruthy();
    expect(screen.getByText("ops shared note (mem-shared-1)")).toBeTruthy();
    expect(screen.getByText("ops skill (skill-1)")).toBeTruthy();
    expect(screen.getByText("agent/ops/runtime [internal]")).toBeTruthy();
    expect(screen.getByText("ops-flow/handoff handoff ops [tool]")).toBeTruthy();
    expect(screen.getByText("dm received (msg-1)")).toBeTruthy();
    expect(screen.getByText("ops-worker [parent]")).toBeTruthy();
    expect(screen.getAllByText("ops-worker: worker watch").length).toBeGreaterThan(0);
    expect(screen.getAllByText("ops-worker: worker refresh (sch-2)").length).toBeGreaterThan(0);
    expect(screen.getAllByText("ops-worker: worker note (mem-2)").length).toBeGreaterThan(0);
    expect(screen.getAllByText("ops-worker: worker shared note (mem-shared-2)").length).toBeGreaterThan(0);
    expect(screen.getAllByText("ops-worker: worker skill (skill-2)").length).toBeGreaterThan(0);
    expect(screen.getAllByText("ops-worker: agent/ops-worker/runtime [internal]").length).toBeGreaterThan(0);
    expect(screen.getAllByText("ops-worker: worker-flow/delegate [tool]").length).toBeGreaterThan(0);
    expect(screen.getAllByText("ops-worker: dm received (msg-2)").length).toBeGreaterThan(0);
    expect(screen.getByText("Kept inspectable with the retired identity.")).toBeTruthy();
    expect(screen.getByText("Kept inspectable with the dependent retired identities.")).toBeTruthy();
    fireEvent.change(screen.getByLabelText("Retirement reason"), { target: { value: "idle identity" } });
    fireEvent.click(screen.getAllByRole("button", { name: "Retire" }).at(-1)!);

    await waitFor(() =>
      expect(postAction).toHaveBeenCalledWith("/api/agents/retire", { ref: "ops", reason: "idle identity" }),
    );
  });

  it("removes an agent with selected cascade cleanup", async () => {
    getJSON.mockImplementation((path: string) => {
      if (path === "/api/agents") {
        return Promise.resolve({
          profiles: [{ id: "01A", slug: "ops", enabled: true }],
          count: 1,
          enabled_count: 1,
        });
      }
      if (path === "/api/skills") return Promise.resolve({ skills: [] });
      if (path === "/api/agents/impact") {
        return Promise.resolve({
          standing_orders: ["check logs"],
          schedules: ["refresh (sch-1)"],
          memories: ["ops note (mem-1)"],
          authored_shared_memories: ["ops shared note (mem-shared-1)"],
          skills: ["ops skill (skill-1)"],
          configs: ["agent/ops/runtime [internal]"],
          workflow_refs: ["ops-flow/handoff handoff ops [tool]"],
          mailbox_messages: ["dm received (msg-1)"],
          subagents: ["ops-worker [parent]"],
          subagent_standing_orders: ["ops-worker: worker watch"],
          subagent_schedules: ["ops-worker: worker refresh (sch-2)"],
          subagent_memories: ["ops-worker: worker note (mem-2)"],
          subagent_authored_shared_memories: ["ops-worker: worker shared note (mem-shared-2)"],
          subagent_skills: ["ops-worker: worker skill (skill-2)"],
          subagent_configs: ["ops-worker: agent/ops-worker/runtime [internal]"],
          subagent_workflow_refs: ["ops-worker: worker-flow/delegate [tool]"],
          subagent_mailbox_messages: ["ops-worker: dm received (msg-2)"],
        });
      }
      return Promise.resolve({});
    });
    postJSON.mockResolvedValue({ removed: true, standing_removed: 1, schedules_removed: 1, memories_forgotten: 0, authored_memories_forgotten: 0, skills_archived: 1, configs_deleted: 1, subagents_retired: 1 });

    render(withUI(<Roster />));
    await waitFor(() => expect(screen.getByText("ops")).toBeTruthy());
    fireEvent.click(screen.getByRole("button", { name: "Remove ops" }));

    await waitFor(() => expect(screen.getByRole("dialog", { name: "Remove ops" })).toBeTruthy());
    expect(screen.getByText("check logs")).toBeTruthy();
    expect(screen.getByText("refresh (sch-1)")).toBeTruthy();
    expect(screen.getByText("ops note (mem-1)")).toBeTruthy();
    expect(screen.getByText("ops shared note (mem-shared-1)")).toBeTruthy();
    expect(screen.getByText("ops skill (skill-1)")).toBeTruthy();
    expect(screen.getByText("agent/ops/runtime [internal]")).toBeTruthy();
    expect(screen.getByText("ops-flow/handoff handoff ops [tool]")).toBeTruthy();
    expect(screen.getByText("dm received (msg-1)")).toBeTruthy();
    expect(screen.getByText("ops-worker [parent]")).toBeTruthy();
    expect(screen.getAllByText("ops-worker: worker watch").length).toBeGreaterThan(0);
    expect(screen.getAllByText("ops-worker: worker refresh (sch-2)").length).toBeGreaterThan(0);
    expect(screen.getAllByText("ops-worker: worker note (mem-2)").length).toBeGreaterThan(0);
    expect(screen.getAllByText("ops-worker: worker shared note (mem-shared-2)").length).toBeGreaterThan(0);
    expect(screen.getAllByText("ops-worker: worker skill (skill-2)").length).toBeGreaterThan(0);
    expect(screen.getAllByText("ops-worker: agent/ops-worker/runtime [internal]").length).toBeGreaterThan(0);
    expect(screen.getAllByText("ops-worker: worker-flow/delegate [tool]").length).toBeGreaterThan(0);
    expect(screen.getAllByText("ops-worker: dm received (msg-2)").length).toBeGreaterThan(0);
    expect(screen.getByText("remove with retained resources")).toBeTruthy();
    expect(screen.getByText("delete identity · clean 12 groups · keep 1 authored shared memory, 1 workflow reference, 1 mailbox/audit messages, 1 sub-agent authored shared memory, 1 sub-agent workflow reference, 1 sub-agent mailbox/audit messages")).toBeTruthy();
    expect(screen.getByText("12 cleanup groups · 6 retained groups")).toBeTruthy();
    expect(screen.getByText(/Remove plan: delete identity; clean 1 standing, 1 schedule, 1 private memory, 1 skill, 1 config, shared config access refs, 1 sub-agent, 1 sub-agent standing, 1 sub-agent schedule, 1 sub-agent private memory, 1 sub-agent skill, 1 sub-agent config; keep 1 authored shared memory, 1 workflow reference, 1 mailbox\/audit messages, 1 sub-agent authored shared memory, 1 sub-agent workflow reference, 1 sub-agent mailbox\/audit messages/)).toBeTruthy();
    expect(screen.getByLabelText(/Standing orders/).closest("label")?.textContent).toContain("2");
    expect(screen.getByLabelText(/Schedules/).closest("label")?.textContent).toContain("2");
    expect(screen.getByLabelText(/Private memory/).closest("label")?.textContent).toContain("2");
    expect(screen.getByLabelText(/Private skills/).closest("label")?.textContent).toContain("2");
    expect(screen.getByLabelText(/Agent config/).closest("label")?.textContent).toContain("2");

    fireEvent.click(screen.getByLabelText(/Dependent sub-agent tree/));
    expect(screen.getByLabelText(/Private memory/).closest("label")?.textContent).toContain("1");
    expect(screen.getByText("removal blocked")).toBeTruthy();
    expect(screen.getByText("dependent sub-agent tree would be orphaned; include it so every descendant retires before this identity is deleted")).toBeTruthy();
    expect(screen.getByText("6 cleanup groups · 11 retained groups · blocked by dependent sub-agent tree")).toBeTruthy();
    expect(screen.getByText(/Dependent sub-agent tree must be retired with this removal/)).toBeTruthy();
    expect((screen.getAllByRole("button", { name: "Remove" }).at(-1)! as HTMLButtonElement).disabled).toBe(true);
    fireEvent.click(screen.getByLabelText(/Dependent sub-agent tree/));

    fireEvent.click(screen.getByLabelText(/Private memory/));
    expect(screen.getAllByText(/keep 1 private memory, 1 authored shared memory, 1 workflow reference, 1 mailbox\/audit messages, 1 sub-agent private memory, 1 sub-agent authored shared memory, 1 sub-agent workflow reference, 1 sub-agent mailbox\/audit messages/).length).toBeGreaterThan(0);
    fireEvent.click(screen.getAllByRole("button", { name: "Remove" }).at(-1)!);

    await waitFor(() =>
      expect(postJSON).toHaveBeenCalledWith("/api/agents/remove", {
        ref: "ops",
        cascade: { standing: true, schedules: true, memory: false, authored_memory: false, skills: true, config: true, workspace: false, subagents: true },
      }),
    );
  });

  it("shows a retired agent reason in the roster card", async () => {
    getJSON.mockResolvedValue({
      profiles: [{ id: "01A", slug: "ops", enabled: false, retired: true, retired_ms: Date.UTC(2026, 0, 2, 3, 4, 5), retired_reason: "replaced by guardian" }],
      count: 1,
      enabled_count: 0,
    });
    render(withUI(<Roster />));
    await waitFor(() => expect(screen.getAllByText("ops").length).toBeGreaterThan(0));
    expect(screen.getAllByText("graveyard").length).toBeGreaterThan(0);
    expect(screen.getByText("retired reason")).toBeTruthy();
    expect(screen.getAllByText("replaced by guardian").length).toBeGreaterThan(0);
  });

  it("surfaces a graveyard operations band for retired identities", async () => {
    getJSON.mockImplementation((path: string) => {
      if (path === "/api/agents") {
        return Promise.resolve({
          profiles: [
            { id: "01A", slug: "old-ops", enabled: false, retired: true, retired_ms: Date.UTC(2026, 0, 2, 3, 4, 5), retired_reason: "replaced by guardian" },
            { id: "02A", slug: "guardian-old", enabled: false, retired: true, system: true, kind: "system", retired_ms: Date.UTC(2026, 0, 1, 1, 2, 3) },
            { id: "03A", slug: "active", enabled: true },
          ],
          count: 3,
          enabled_count: 1,
        });
      }
      if (path === "/api/skills") return Promise.resolve({ skills: [] });
      if (path === "/api/agents/impact") {
        return Promise.resolve({
          standing_orders: [],
          schedules: [],
          memories: [],
          authored_shared_memories: [],
          skills: [],
          configs: [],
          subagents: [],
        });
      }
      return Promise.resolve({});
    });

    render(withUI(<Roster />));
    await waitFor(() => expect(screen.getByLabelText("Agent graveyard")).toBeTruthy());
    expect(screen.getByText("2 retired identities")).toBeTruthy();
    expect(screen.getByText("1 custom")).toBeTruthy();
    expect(screen.getByText("1 system")).toBeTruthy();
    expect(screen.getByText("1 with reason")).toBeTruthy();
    expect(screen.getByText("Cleanup")).toBeTruthy();
    expect(screen.getByText("1 removable")).toBeTruthy();
    expect(screen.getByText("2 retired · 1 can be removed · 1 system protected · 1 without reason")).toBeTruthy();
    expect(screen.getAllByText("replaced by guardian").length).toBeGreaterThan(0);
    expect(screen.queryByRole("button", { name: "Remove from graveyard guardian-old" })).toBeNull();

    fireEvent.click(screen.getByRole("button", { name: "Revive from graveyard old-ops" }));
    await waitFor(() => expect(postAction).toHaveBeenCalledWith("/api/agents/revive", { ref: "old-ops" }));

    fireEvent.click(screen.getByRole("button", { name: "Remove from graveyard old-ops" }));
    await waitFor(() => expect(screen.getByRole("dialog", { name: "Remove old-ops" })).toBeTruthy());
  });

  it("filters the roster by identity and repair state", async () => {
    getJSON.mockImplementation((path: string) => {
      if (path === "/api/agents") {
        return Promise.resolve({
          profiles: [
            { id: "1", slug: "direct", enabled: true },
            { id: "2", slug: "worker", enabled: true, kind: "subagent" },
            { id: "3", slug: "guardian", enabled: true, system: true, kind: "system" },
            { id: "4", slug: "broken", enabled: true, status: { health_state: "degraded" } },
          ],
        });
      }
      if (path === "/api/skills") return Promise.resolve({ skills: [] });
      return Promise.resolve({});
    });
    render(withUI(<Roster />));
    await waitFor(() => expect(screen.getAllByText("direct").length).toBeGreaterThan(0));

    // The roster filter segments are Radix tabs; selection activates on
    // mousedown/focus (automatic activation mode), not a plain click.
    const selectTab = (name: RegExp) => {
      const tab = screen.getByRole("tab", { name });
      fireEvent.mouseDown(tab);
      fireEvent.focus(tab);
    };
    await screen.findByRole("tab", { name: /Sub-agents1/ });
    selectTab(/Sub-agents1/);
    expect(screen.getByText("worker")).toBeTruthy();
    expect(screen.queryByRole("button", { name: "direct" })).toBeNull();
    expect(screen.queryByRole("button", { name: "guardian" })).toBeNull();

    selectTab(/Repair1/);
    expect(screen.getByText("broken")).toBeTruthy();
    expect(screen.queryByRole("button", { name: "worker" })).toBeNull();
  });

  it("shows the empty state when the roster is empty", async () => {
    getJSON.mockResolvedValue({ profiles: [], count: 0, enabled_count: 0 });
    render(withUI(<Roster />));
    await waitFor(() => expect(screen.getByText("No agents yet")).toBeTruthy());
  });
});
