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

import { Roster, NewAgentForm, profileFields, slugOk, usdToMc, agentHue, initials, sortAgentRoster, agentIdentityKind, agentNeedsRepair, agentNeedsAttention, filterAgentRoster, agentEnableToast, agentRemoveToast, agentRetireToast, agentReviveToast, agentNoisePolicySummary, agentNoiseBudgetPassport, agentSchedulePressurePassport, systemGuardianSafetySummary, systemGuardianNoiseContract, systemGuardianRiskSummary, noisySystemGuardians, guardianQuietPolicyPayload, guardianQuietingSummary, agentLifecycleSummary, agentLifecycleDispositionPassport, agentGraveyardSummary, agentGraveyardStats, agentGraveyardCleanupPassport, agentTaskContractSummary, agentTaskProgressSummary, agentHierarchySummary, agentHierarchyTreePassport, agentDelegationPassport, agentIdentityDossier, agentIdentityCardSummary, agentRosterIdentityManifest, agentIdentityLedger, agentCommandStrip, agentCapabilityPassportSummary, agentResourcePassportSummary, agentConfigPassportSummary, agentRepairGovernancePassport, agentRepairOperationsPassport, agentModelPassportSummary, agentSkillPassportSummary, agentLifecycleRail, agentLifeSummary, agentLivePresencePassport, rosterWaitingMailboxCounts, rosterMailboxPassport, agentRemovalCascadePreset, agentRemovalCustodySummary, agentRemovalDeathCertificate, agentRemovalDecisionSummary, agentRemovalImpactSummary, agentRemovalLifecycleSummary, agentRemovalLedger, agentRemovalPlan, rosterWakeIssue, rosterRepairIssue, agentHealthIssueSummary, formatWakeDue } from "@/views/Roster";
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

describe("agentLifecycleDispositionPassport", () => {
  it("states whether an identity is alive, cyclic, one-shot, or in the graveyard", () => {
    expect(agentLifecycleDispositionPassport({})).toEqual({
      value: "persistent · alive",
      detail: "this identity stays available across runs until an operator, doctor, or lifecycle rule retires it",
      tone: "good",
    });
    expect(agentLifecycleDispositionPassport({ lifecycle: { mode: "cycle", completed_cycles: 2, max_cycles: 5 } })).toEqual({
      value: "cycle · 2/5",
      detail: "this identity wakes repeatedly and retires when the configured cycle count is reached",
      tone: "good",
    });
    expect(agentLifecycleDispositionPassport({ lifecycle: { mode: "retire_on_complete", retire_on_complete: true } })).toEqual({
      value: "one-shot · retires on completion",
      detail: "this identity is expected to finish its assigned total task contract, then move to the graveyard",
      tone: "warn",
    });
    const retired = agentLifecycleDispositionPassport({
      retired: true,
      retired_ms: Date.UTC(2026, 0, 2, 3, 4, 5),
      retired_reason: "mission complete",
    });
    expect(retired.value).toBe("graveyard · revive/remove");
    expect(retired.detail).toContain("soul, logs, memory, skills, config, and workspace remain inspectable");
    expect(retired.detail).toContain("reason: mission complete");
    expect(retired.tone).toBe("muted");
  });
});

describe("agentGraveyardSummary", () => {
  it("shows graveyard state only for retired identities", () => {
    expect(agentGraveyardSummary({ retired: false })).toBe("");
    expect(agentGraveyardSummary({ retired: true })).toBe("graveyard");
    expect(agentGraveyardSummary({ retired: true, retired_ms: Date.UTC(2026, 0, 2, 3, 4, 5) })).toContain("graveyard since");
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
      detail: "2 retired · 1 can be removed · 1 system protected · 1 without reason",
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

describe("agentLifecycleRail", () => {
  it("maps roster state into a stable sleep/wake/work/repair strip", () => {
    const running = agentLifecycleRail(
      {
        enabled: true,
        self_repair: { enabled: true },
        tasklist: [{ title: "sync", scope: "cycle", status: "todo" }],
      },
      {
        activeRunCount: 1,
        activePhase: "using tool",
        liveDetail: "models.dev/api.json",
        nextWakeMs: Date.UTC(2026, 0, 1, 13, 0, 0),
        repairTone: "muted",
        repairInflight: 0,
      },
      "",
      Date.UTC(2026, 0, 1, 12, 0, 0),
    );
    expect(running.map((s) => s.label)).toEqual(["Sleep", "Wake", "Work", "Repair"]);
    expect(running.map((s) => s.value)).toEqual(["awake", "in 1h 0m", "using tool", "self-ready"]);
    expect(running.map((s) => s.tone)).toEqual(["accent", "accent", "accent", "good"]);

    const blocked = agentLifecycleRail(
      { enabled: true, retired: true, health_policy: { doctor_agent: "guardian-doctor" } },
      { activeRunCount: 0, repairTone: "muted", repairInflight: 0 },
      "revive this agent before waking it",
    );
    expect(blocked.map((s) => s.value)).toEqual(["graveyard", "blocked", "idle", "doctor"]);
    expect(blocked[1].tone).toBe("warn");

    const retrying = agentLifecycleRail(
      { enabled: true, self_repair: { enabled: true } },
      {
        activeRunCount: 0,
        repairTone: "muted",
        repairInflight: 0,
        retryText: "retry 2",
        retryDetail: "attempt 3/4 · provider timeout",
      },
    );
    expect(retrying[3]).toMatchObject({
      id: "repair",
      value: "retry 2",
      detail: "attempt 3/4 · provider timeout",
      tone: "bad",
    });
  });
});

describe("agentLifeSummary", () => {
  it("summarizes sleep, wake route, and current work pressure", () => {
    expect(agentLifeSummary(
      { enabled: true, tasklist: [{ title: "sync", scope: "cycle", status: "todo" }] },
      { activeRunCount: 0, operationalText: "sleeping", nextWakeMs: Date.UTC(2026, 0, 1, 13, 0, 0) },
      "",
      0,
      Date.UTC(2026, 0, 1, 12, 0, 0),
    )).toBe("sleeping · next in 1h 0m · 1 task queued");
    expect(agentLifeSummary(
      { enabled: true, kind: "subagent", parent_agent: "lead", tasklist: [] },
      { activeRunCount: 0, operationalText: "sleeping" },
      "managed sub-agent; wake lead instead",
      2,
    )).toBe("sleeping · manager wake: lead · 2 mailbox waiting");
    expect(agentLifeSummary(
      { enabled: true, tasklist: [] },
      { activeRunCount: 1, activePhase: "using tool", activeContextDetail: "source: schedule" },
    )).toBe("awake · using tool · manual wake · source: schedule");
  });
});

describe("agentLivePresencePassport", () => {
  it("turns runtime status into an always-visible live presence", () => {
    const now = Date.UTC(2026, 0, 1, 12, 0, 0);
    expect(agentLivePresencePassport(
      { enabled: true },
      {
        activeRunCount: 1,
        activePhase: "using tool",
        liveDetail: "using tool · source: schedule · model: gpt-5",
        activeStartedMs: now - 10 * 60_000,
        activeLastEventMs: now - 2 * 60_000,
      },
      0,
      now,
    )).toEqual({
      value: "awake · using tool",
      detail: "using tool · source: schedule · model: gpt-5 · running for 10m · last event 2m ago",
      tone: "good",
    });
    expect(agentLivePresencePassport(
      { enabled: true },
      { activeRunCount: 0, operationalText: "sleeping", nextWakeMs: now + 60 * 60_000, wakeDetail: "1 schedule" },
      0,
      now,
    )).toEqual({
      value: "sleeping · wakes in 1h 0m",
      detail: "1 schedule",
      tone: "good",
    });
    expect(agentLivePresencePassport(
      { enabled: true },
      { activeRunCount: 0, operationalText: "sleeping", wakeText: "wake 1" },
      3,
      now,
    )).toEqual({
      value: "sleeping · inbox 3",
      detail: "3 mailbox messages waiting · wake 1",
      tone: "warn",
    });
    expect(agentLivePresencePassport(
      { enabled: false },
      { activeRunCount: 0, operationalState: "paused", operationalText: "paused" },
      1,
      now,
    )).toEqual({
      value: "paused",
      detail: "paused · 1 inbox waiting",
      tone: "warn",
    });
    expect(agentLivePresencePassport(
      { enabled: true, retired: true },
      { activeRunCount: 0, lastActivitySummary: "completed final task" },
      0,
      now,
    )).toEqual({
      value: "graveyard",
      detail: "retired · last completed final task",
      tone: "muted",
    });
  });
});

describe("agentIdentityCardSummary", () => {
  it("collapses identity, delegation, lifecycle, mailbox, and runtime into a card header", () => {
    expect(agentIdentityCardSummary(
      {
        kind: "subagent",
        parent_agent: "lead",
        direct_callable: false,
        enabled: true,
        lifecycle: { mode: "cycle", completed_cycles: 1, max_cycles: 3 },
        tasklist: [{ title: "scan", scope: "cycle", status: "doing" }],
      },
      {
        activeRunCount: 1,
        activePhase: "using tool",
        liveDetail: "models.dev/api.json",
      },
      "",
      2,
      Date.now(),
      "limited · shell · L3",
    )).toEqual({
      label: "subagent · running",
      detail: "models.dev/api.json · manager-only wake · lead · limited · shell · L3 · 2 inbox waiting · 1 cycle / 0 total / 1 doing",
      tone: "accent",
    });
    expect(agentIdentityCardSummary({ retired: true, system: true }, {}, "", 0)).toEqual({
      label: "system · graveyard",
      detail: "graveyard · will not wake until revived · revive/remove · no tasks",
      tone: "muted",
    });
    expect(agentIdentityCardSummary(
      { enabled: true, tasklist: [] },
      { operationalText: "sleeping", nextWakeMs: Date.UTC(2026, 0, 1, 13, 0, 0) },
      "",
      1,
      Date.UTC(2026, 0, 1, 12, 0, 0),
      "open high-impact · L4",
    )).toEqual({
      label: "custom · sleeping",
      detail: "persistent · alive · next in 1h 0m · open high-impact · L4 · 1 inbox waiting · no tasks",
      tone: "warn",
    });
    expect(agentIdentityCardSummary(
      { enabled: false, tool_allow: ["shell"], tasklist: [] } as any,
      { operationalText: "paused", operationalState: "paused" },
      "resume this agent before waking it",
      0,
      Date.now(),
      "high-impact allow · shell · L4",
    )).toEqual({
      label: "custom · paused",
      detail: "paused · paused · wake blocked · high-impact allow · shell · L4 · no tasks",
      tone: "warn",
    });
  });
});

describe("agentRosterIdentityManifest", () => {
  it("keeps identity-card manifest fields stable across class, owner, wake, soul, contract, and authority", () => {
    const manifest = agentRosterIdentityManifest(
      {
        slug: "researcher",
        name: "Researcher",
        soul: "Investigate and report.",
        enabled: true,
        kind: "subagent",
        parent_agent: "lead",
        direct_callable: false,
        lifecycle: { mode: "cycle", completed_cycles: 2, max_cycles: 5 },
        tasklist: [
          { title: "scan", scope: "cycle", status: "doing" },
          { title: "report", scope: "total", status: "todo" },
        ],
        tool_allow: ["memory", "shell"],
        trust_ceiling: "L2",
      },
      { nextWakeMs: Date.UTC(2026, 0, 1, 13, 0, 0), wakeDetail: "schedule wake" },
      "",
      1,
      2,
      "high-impact allow · shell · L2",
      Date.UTC(2026, 0, 1, 12, 0, 0),
    );

    expect(manifest.map((entry) => entry.label)).toEqual(["class", "owner", "wake", "soul", "contract", "authority"]);
    expect(manifest.map((entry) => entry.value)).toEqual([
      "subagent · sleeping",
      "managed by lead",
      "in 1h 0m",
      "soul set",
      "cycle · 2/5",
      "high-impact allow · shell · L2",
    ]);
    expect(manifest.find((entry) => entry.label === "wake")?.detail).toBe("schedule wake · 1 mailbox waiting");
    expect(manifest.find((entry) => entry.label === "authority")?.detail).toBe("high-impact allow · shell · L2 · 2 private skills");
  });
});

describe("agentCommandStrip", () => {
  it("keeps wake, mailbox, schedule, authority, resources, and repair visible in one stable strip", () => {
    const items = agentCommandStrip(
      {
        slug: "researcher",
        enabled: true,
        kind: "subagent",
        parent_agent: "lead",
        direct_callable: false,
        memory_scope: "researcher",
        workdir: "agents/researcher",
        tool_allow: ["memory", "shell"],
        tool_deny: ["notify"],
        trust_ceiling: "L2",
        model: "@research-chain",
        task_type: "research",
        retry_policy: { max_attempts: 3 },
        health_policy: { doctor_agent: "guardian-doctor" },
        self_repair: { enabled: true, max_attempts: 2 },
      },
      {
        activeRunCount: 0,
        wakeText: "scheduled sync",
        wakeDetail: "source: schedule",
        nextWakeMs: Date.UTC(2026, 0, 1, 13, 0, 0),
      },
      { value: "inbox 2 waiting", detail: "2 waiting messages · wake subjects armed: DM", tone: "warn" },
      { detail: "schedule pressure: 1/2 frequent · fastest 5m", tone: "warn" },
      "high-impact allow · shell · L2",
      { detail: "memory researcher · workdir agents/researcher · allow 2 · deny 1", tone: "good" },
      "managed sub-agent; wake lead instead",
      Date.UTC(2026, 0, 1, 12, 0, 0),
    );
    expect(items.map((item) => item.label)).toEqual(["wake", "mailbox", "schedule", "route", "authority", "resources", "repair"]);
    expect(items.map((item) => item.value)).toEqual([
      "in 1h 0m",
      "inbox 2 waiting",
      "schedule pressure: 1/2 frequent · fastest 5m",
      "chain @research-chain",
      "high-impact allow · shell · L2",
      "memory researcher · workdir agents/researcher · allow 2 · deny 1",
      "retry 3x · doctor guardian-doctor · self-repair 2x",
    ]);
    expect(items.map((item) => item.tone)).toEqual(["warn", "warn", "warn", "good", "good", "good", "good"]);
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

describe("rosterMailboxPassport", () => {
  it("combines inbox backlog with armed mailbox wake subjects", () => {
    expect(rosterMailboxPassport("researcher", 2, [
      { enabled: true, triggers: [{ type: "event", subject: "board.dm.researcher" }] },
      { enabled: false, triggers: [{ type: "event", subject: "board.help.researcher" }] },
      { enabled: true, triggers: [{ type: "event", subject: "board.broadcast" }] },
    ])).toEqual({
      value: "inbox 2 waiting",
      detail: "2 waiting messages · wake subjects armed: DM, Broadcast",
      tone: "warn",
    });
    expect(rosterMailboxPassport("ops", 0, [
      { enabled: true, triggers: [{ type: "event", subject: "board.help.ops" }] },
    ])).toEqual({
      value: "mailbox armed · Help",
      detail: "0 waiting messages · wake subjects armed: Help",
      tone: "good",
    });
    expect(rosterMailboxPassport("idle", 0, [])).toEqual({
      value: "mailbox manual",
      detail: "0 waiting messages · no mailbox wake subjects armed",
      tone: "muted",
    });
    expect(rosterMailboxPassport("idle", 2, [])).toEqual({
      value: "inbox 2 waiting",
      detail: "2 waiting messages · no mailbox wake subjects armed; manual wake required",
      tone: "warn",
    });
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

describe("agentHierarchyTreePassport", () => {
  it("summarizes parent ownership and active dependent counts", () => {
    const profiles = [
      { slug: "lead" },
      { slug: "worker", parent_agent: "lead" },
      { slug: "scout", owner_agent: "lead" },
      { slug: "old", parent_agent: "lead", retired: true },
    ];
    expect(agentHierarchyTreePassport({ slug: "lead" }, profiles)).toEqual({
      value: "leader · 2 dependents",
      detail: "root callable · 2 dependents · children: worker, scout · 1 retired",
      tone: "good",
    });
    expect(agentHierarchyTreePassport({ slug: "worker", parent_agent: "lead", direct_callable: false }, profiles)).toEqual({
      value: "managed by lead",
      detail: "parent/owner lead · no dependents · no active children",
      tone: "warn",
    });
    expect(agentHierarchyTreePassport({ slug: "orphan", kind: "subagent" }, profiles)).toEqual({
      value: "managed · no manager",
      detail: "manager missing · no dependents · no active children",
      tone: "bad",
    });
    expect(agentHierarchyTreePassport({ slug: "solo" }, profiles)).toEqual({
      value: "direct · no dependents",
      detail: "root callable · no dependents · no active children",
      tone: "muted",
    });
  });
});

describe("agentDelegationPassport", () => {
  it("summarizes who may wake an agent and blocks managed sub-agent direct paths", () => {
    expect(agentDelegationPassport({}).detail).toBe("operator/schedule/channel wake");
    expect(agentDelegationPassport({ parent_agent: "lead" })).toEqual({
      detail: "operator/schedule/channel wake · parent lead",
      tone: "good",
    });
    expect(agentDelegationPassport({ parent_agent: "lead", direct_callable: false })).toEqual({
      detail: "manager-only wake · lead",
      tone: "warn",
    });
    expect(agentDelegationPassport({ kind: "subagent" })).toEqual({
      detail: "manager-only wake · no manager",
      tone: "bad",
    });
    expect(agentDelegationPassport({ enabled: false })).toEqual({
      detail: "paused · wake blocked",
      tone: "bad",
    });
    expect(agentDelegationPassport({ retired: true, parent_agent: "lead", direct_callable: false })).toEqual({
      detail: "graveyard · wake blocked",
      tone: "muted",
    });
  });
});

describe("agentIdentityDossier", () => {
  it("combines identity class, command chain, lifecycle, and durable task contract", () => {
    expect(agentIdentityDossier({
      kind: "subagent",
      parent_agent: "lead",
      lifecycle: { mode: "cycle", completed_cycles: 1, max_cycles: 3 },
      tasklist: [{ title: "scan", scope: "cycle", status: "todo" }],
    })).toBe("subagent identity · managed by lead · cycle agent · repeats on each wake · 1/3 cycles · retires at max cycles · 1 cycle / 0 total tasks");
    expect(agentIdentityDossier({ retired: true, system: true })).toBe(
      "system identity · direct · graveyard · will not wake until revived",
    );
  });
});

describe("agentCapabilityPassportSummary", () => {
  it("summarizes open, capped, allowlisted, and denied tool posture", () => {
    expect(agentCapabilityPassportSummary({})).toBe("open high-impact · L4");
    expect(agentCapabilityPassportSummary({ trust_ceiling: "L2" })).toBe("trust-gated high-impact · L2");
    expect(agentCapabilityPassportSummary({ system: true, tool_deny: ["memory"], trust_ceiling: "L4" })).toBe("system-governed · notify gated · L4");
    expect(agentCapabilityPassportSummary({ tool_allow: ["memory", "shell"], trust_ceiling: "L3" })).toBe("high-impact allow · shell · L3");
    expect(agentCapabilityPassportSummary({ tool_deny: ["notify"] })).toBe("broad minus · notify · L4");
    expect(agentCapabilityPassportSummary({ tool_allow: ["memory"], tool_deny: ["shell", "notify"], trust_ceiling: "L2" })).toBe("limited · 1 allow · 2 denies · L2");
  });
});

describe("agentResourcePassportSummary", () => {
  it("summarizes workspace, memory, data lake, and config ownership for roster cards", () => {
    expect(agentResourcePassportSummary({
      slug: "ops",
      workdir: "agents/ops",
      memory_scope: "private",
      tool_allow: ["db", "memory"],
      config_overrides: { AGEZT_MAX_ITER: "4" },
    })).toEqual({
      detail: "workspace agents/ops · memory private · data lake via db · 1 cfg",
      tone: "good",
    });
    expect(agentResourcePassportSummary({ slug: "ops", tool_allow: ["memory"], tool_deny: ["db"] })).toEqual({
      detail: "shared workspace · memory ops · data lake blocked · default config",
      tone: "warn",
    });
    expect(agentResourcePassportSummary({ slug: "ops", tool_allow: ["memory"] })).toEqual({
      detail: "shared workspace · memory ops · data lake not allowlisted · default config",
      tone: "warn",
    });
  });
});

describe("agentConfigPassportSummary", () => {
  it("summarizes agent-local config and quiet memory policy", () => {
    expect(agentConfigPassportSummary({})).toBe("default config");
    expect(agentConfigPassportSummary({ config_overrides: { AGEZT_PROVIDER: "openai" } })).toBe("1 cfg");
    expect(agentConfigPassportSummary({ config_overrides: { AGEZT_PROVIDER: "openai" } }, 1)).toBe("1 cfg · !1");
    expect(agentConfigPassportSummary({
      noise_policy: { silent_on_success: true, disable_memory_writes: true, min_notify_severity: "warning" },
    })).toBe("default config · quiet policy · memory off");
  });
});

describe("agentModelPassportSummary", () => {
  it("summarizes primary model, task type, fallback chain, and @chain routing", () => {
    expect(agentModelPassportSummary({})).toBe("daemon default model · no fallback");
    expect(agentModelPassportSummary({ model: "gpt-5", task_type: "research", fallbacks: ["gpt-4.1"] })).toBe(
      "model gpt-5 · task research · 1 fallback",
    );
    expect(agentModelPassportSummary({ model: "@code-chain", fallbacks: ["ignored"] })).toBe("chain @code-chain · chain-owned fallback");
  });
});

describe("agentSkillPassportSummary", () => {
  it("summarizes private skill ownership for agent identity cards", () => {
    expect(agentSkillPassportSummary(0)).toBe("no private skills");
    expect(agentSkillPassportSummary(1)).toBe("1 private skill");
    expect(agentSkillPassportSummary(3)).toBe("3 private skills");
  });
});

describe("agentIdentityLedger", () => {
  it("keeps the roster card identity passport in a stable order", () => {
    const ledger = agentIdentityLedger({
      slug: "researcher",
      name: "Research Agent",
      soul: "Investigate changes and report only actionable findings.",
      kind: "subagent",
      parent_agent: "lead",
      direct_callable: false,
      enabled: true,
      model: "@research-chain",
      task_type: "research",
      memory_scope: "researcher",
      workdir: "agents/researcher",
      tool_allow: ["memory", "shell"],
      tool_deny: ["notify"],
      trust_ceiling: "L2",
      config_overrides: { AGEZT_MODE: "quiet" },
      lifecycle: { mode: "cycle", completed_cycles: 1, max_cycles: 3 },
      tasklist: [{ title: "scan", scope: "cycle", status: "doing" }],
    }, 2);
    expect(ledger.map((entry) => entry.label)).toEqual(["identity", "soul", "model", "memory", "authority", "lifecycle"]);
    expect(ledger.map((entry) => entry.value)).toEqual([
      "subagent · alive",
      "soul set",
      "chain @research-chain · task research · chain-owned fallback",
      "memory researcher",
      "high-impact allow · shell · L2",
      "cycle · 1/3",
    ]);
    expect(ledger.find((entry) => entry.label === "authority")?.detail).toContain("2 private skills");
  });

  it("flags missing souls as a roster-card identity gap", () => {
    const ledger = agentIdentityLedger({ slug: "ops", enabled: true });
    expect(ledger.find((entry) => entry.label === "soul")).toMatchObject({
      value: "soul missing",
      tone: "warn",
    });
  });
});

describe("agentRepairGovernancePassport", () => {
  it("adds retry, doctor, and self-repair detail for roster governance", () => {
    expect(agentRepairGovernancePassport({})).toEqual({
      value: "manual recovery",
      detail: "single attempt · no doctor · self-repair off",
      tone: "warn",
    });
    expect(agentRepairGovernancePassport({
      retry_policy: { max_attempts: 3, backoff: "exponential", retry_on: ["error", "timeout"] },
      health_policy: { doctor_agent: "guardian-doctor", failure_threshold: 5 },
      self_repair: { enabled: true, max_attempts: 2, escalate_to: "lead" },
    })).toEqual({
      value: "retry 3x · doctor guardian-doctor · self-repair 2x · escalate lead",
      detail: "run retry 3x exponential on error, timeout · doctor guardian-doctor after 5 failures · self-repair 2x then lead",
      tone: "good",
    });
  });
});

describe("agentRepairOperationsPassport", () => {
  it("prioritizes live repair state over the static repair policy", () => {
    expect(agentRepairOperationsPassport({ retired: true }, {})).toEqual({
      value: "graveyard",
      detail: "repair blocked until the agent is revived",
      tone: "muted",
    });
    expect(agentRepairOperationsPassport({}, {})).toEqual({
      value: "manual repair",
      detail: "no retry, doctor, or self-repair policy configured",
      tone: "warn",
    });
    expect(agentRepairOperationsPassport({
      retry_policy: { max_attempts: 3 },
      health_policy: { doctor_agent: "guardian-doctor" },
      self_repair: { enabled: true, max_attempts: 2 },
    }, {})).toEqual({
      value: "repair guarded",
      detail: "run retry 3x · doctor guardian-doctor · self-repair 2x",
      tone: "good",
    });
    expect(agentRepairOperationsPassport({
      self_repair: { enabled: true },
    }, {
      repairInflight: 1,
      repairText: "doctor 1",
      repairKindText: "doctor",
      repairDetail: "attempt 1/2",
    })).toEqual({
      value: "doctor 1",
      detail: "doctor · attempt 1/2",
      tone: "accent",
    });
    expect(agentRepairOperationsPassport({}, {
      repairText: "repair exhausted",
      repairTone: "bad",
      repairKindText: "repair",
      repairDetail: "attempt 2/2",
      repairIncidentDetail: "root lead · incident abc123",
    })).toEqual({
      value: "repair exhausted",
      detail: "repair · attempt 2/2 · root lead · incident abc123",
      tone: "bad",
    });
    expect(agentRepairOperationsPassport({}, {
      retryText: "retry 2",
      retryDetail: "attempt 2/3 · timeout",
    })).toEqual({
      value: "retry 2",
      detail: "attempt 2/3 · timeout",
      tone: "bad",
    });
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
    expect(agentRemovalLifecycleSummary({
      standing: [],
      schedules: [],
      memories: [],
      authoredMemories: [],
      skills: [],
      configs: [],
      mailboxMessages: ["dm received"],
      subagents: ["ops-worker"],
      subagentStanding: [],
      subagentSchedules: [],
      subagentMemories: [],
      subagentAuthoredMemories: [],
      subagentSkills: [],
      subagentConfigs: [],
      subagentMailboxMessages: ["ops-worker: dm received"],
      cascade: { standing: false, schedules: false, memory: false, authored_memory: false, skills: false, config: false, workspace: false, subagents: true },
    })).toEqual({
      label: "identity death plan",
      detail: "profile deleted · 1 dependent sub-agent retired, not deleted · 2 mailbox/audit records retained",
      tone: "warn",
    });
    expect(agentRemovalLifecycleSummary({
      standing: [],
      schedules: [],
      memories: [],
      authoredMemories: [],
      skills: [],
      configs: [],
      subagents: ["ops-worker", "ops-helper"],
      subagentStanding: [],
      subagentSchedules: [],
      subagentMemories: [],
      subagentAuthoredMemories: [],
      subagentSkills: [],
      subagentConfigs: [],
      cascade: { standing: false, schedules: false, memory: false, authored_memory: false, skills: false, config: false, workspace: false, subagents: false },
    })).toEqual({
      label: "orphan guard",
      detail: "2 dependent sub-agents would lose their owner; removal is blocked until they are retired with the parent",
      tone: "bad",
    });
    expect(agentRemovalCustodySummary(planInput)).toEqual({
      deletedGroups: 10,
      retainedGroups: 4,
      hardRetainedGroups: 2,
      subagentsRetired: 1,
      label: "custody split",
      detail: "10 cleanup groups · 4 operator-retained groups · 2 audit/workflow groups retained by design · 1 sub-agent retired",
      tone: "warn",
    });
    expect(agentRemovalCustodySummary({ ...planInput, cascade: { ...planInput.cascade, subagents: false } })).toMatchObject({
      label: "custody blocked",
      tone: "bad",
    });
    expect(agentRemovalDeathCertificate(planInput)).toEqual({
      label: "retire dependent tree",
      detail: "profile deleted; soul, settings, lifecycle removed · 1 retired to graveyard · 10 cleanup groups · 4 owned retained · 2 mailbox/audit records retained · ready",
      tone: "warn",
      fields: {
        identity: "profile deleted; soul, settings, lifecycle removed",
        dependents: "1 retired to graveyard",
        cleanup: "10 cleanup groups",
        retained: "4 owned retained",
        audit: "2 mailbox/audit records retained",
        guard: "ready",
      },
    });
    expect(agentRemovalDeathCertificate({ ...planInput, cascade: { ...planInput.cascade, subagents: false } })).toMatchObject({
      label: "death blocked",
      tone: "bad",
      fields: {
        dependents: "1 would be orphaned",
        guard: "blocked by dependent sub-agents",
      },
    });
    expect(agentRemovalLedger({
      standing: ["check logs"],
      schedules: ["refresh"],
      memories: ["note"],
      authoredMemories: ["shared note"],
      skills: ["skill"],
      configs: ["agent/ops/runtime"],
      workspaces: ["agents/ops"],
      mailboxMessages: ["dm received"],
      subagents: ["ops-worker"],
      subagentStanding: ["ops-worker: watch"],
      subagentSchedules: [],
      subagentMemories: ["ops-worker: note"],
      subagentAuthoredMemories: [],
      subagentSkills: [],
      subagentConfigs: ["ops-worker: config"],
      subagentWorkspaces: ["agents/ops-worker"],
      subagentMailboxMessages: ["ops-worker: dm received"],
      cascade: { standing: true, schedules: false, memory: true, authored_memory: false, skills: true, config: true, workspace: true, subagents: true },
    })).toEqual([
      {
        label: "identity",
        value: "delete profile",
        detail: "soul, settings, lifecycle, and direct identity record are removed",
        tone: "bad",
      },
      {
        label: "sub-agents",
        value: "1 retire",
        detail: "dependent identities are retired to the graveyard, not deleted",
        tone: "warn",
      },
      {
        label: "owned cleanup",
        value: "11 groups",
        detail: "1 standing, 1 private memory, 1 skill, 1 config, shared config access refs, 1 workspace, 1 sub-agent, 1 sub-agent standing, 1 sub-agent private memory, 1 sub-agent config, 1 sub-agent workspace",
        tone: "good",
      },
      {
        label: "retained owned",
        value: "2 groups",
        detail: "1 schedule, 1 authored shared memory",
        tone: "warn",
      },
      {
        label: "audit trail",
        value: "2 retained",
        detail: "mailbox/audit records are retained for inspection",
        tone: "muted",
      },
      {
        label: "guard",
        value: "ready",
        detail: "hard removal can proceed with the selected cascade",
        tone: "good",
      },
    ]);
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
  it("renders profiles from /api/agents with state, model, and budget", async () => {
    getJSON.mockImplementation((path: string) => {
      if (path === "/api/skills") return Promise.resolve({ skills: [{ id: "sk-1", name: "Web Scan", agent: "researcher" }] });
      if (path === "/api/standing") {
        return Promise.resolve({
          orders: [
            { id: "stand-dm", enabled: true, agent: "researcher", triggers: [{ type: "event", subject: "board.dm.researcher" }] },
            { id: "stand-broadcast", enabled: true, agent: "researcher", triggers: [{ type: "event", subject: "board.broadcast" }] },
          ],
        });
      }
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
            kind: "subagent", managed: true,
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
    await waitFor(() => expect(screen.getByText("researcher")).toBeTruthy());
    expect(screen.getByText("ops")).toBeTruthy();
    // "paused" appears both as the ops badge and the summary-band stat label.
    expect(screen.getAllByText("paused").length).toBeGreaterThan(0);
    expect(screen.getByText("m-1")).toBeTruthy();
    expect(screen.getByText("$0.5000")).toBeTruthy();
    expect(screen.getByText("lead")).toBeTruthy();
    expect(screen.getAllByText("Identity").length).toBeGreaterThan(0);
    expect(screen.getAllByText("Authority").length).toBeGreaterThan(0);
    expect(screen.getAllByText("Operations").length).toBeGreaterThan(0);
    expect(screen.getByLabelText("researcher identity card passports")).toBeTruthy();
    expect(screen.getByLabelText("ops identity card passports")).toBeTruthy();
    expect(screen.getByLabelText("researcher identity manifest")).toBeTruthy();
    expect(screen.getByLabelText("ops identity manifest")).toBeTruthy();
    expect(screen.getByLabelText("researcher identity ledger")).toBeTruthy();
    expect(screen.getByLabelText("ops identity ledger")).toBeTruthy();
    expect(screen.getAllByText("soul set").length).toBeGreaterThan(0);
    expect(screen.getAllByText("memory researcher").length).toBeGreaterThan(0);
    expect(screen.getByLabelText("researcher command strip")).toBeTruthy();
    expect(screen.getByLabelText("ops command strip")).toBeTruthy();
    expect(screen.getAllByLabelText("Agent lifecycle rail").length).toBeGreaterThan(0);
    expect(screen.getAllByText("high-impact allow · shell · L2").length).toBeGreaterThan(0);
    expect(screen.getAllByText(/memory off/).length).toBeGreaterThan(0);
    expect(screen.getAllByText(/1 cfg/).length).toBeGreaterThan(0);
    expect(screen.getByText("You dig deep.")).toBeTruthy();
    expect(screen.getByLabelText("researcher command strip")).toBeTruthy();
    expect(screen.getByLabelText("ops command strip")).toBeTruthy();
    expect(screen.getAllByText("Identity").length).toBeGreaterThan(0);
    expect(screen.getAllByText("subagent · sleeping").length).toBeGreaterThanOrEqual(1);
    expect(screen.getAllByText("class").length).toBeGreaterThan(0);
    expect(screen.getAllByText("owner").length).toBeGreaterThan(0);
    expect(screen.getAllByText("contract").length).toBeGreaterThan(0);
    expect(screen.getAllByText("managed by lead").length).toBeGreaterThan(0);
    expect(screen.getByText("subagent identity · managed by lead · cycle agent · repeats on each wake · 2/5 cycles · retires at max cycles · 1 cycle / 1 total tasks · 1 blocked")).toBeTruthy();
    expect(screen.getByText("sleeping · manager wake: lead · 1 mailbox waiting")).toBeTruthy();
    expect(screen.getByText("paused · wake blocked · 1 mailbox waiting")).toBeTruthy();
    // The roster summary band + filter segments expose the inbox/attention indicators
    // (capitalized labels in the redesign's MetricWidget + segment row).
    expect(screen.getAllByText("Inbox").length).toBeGreaterThan(0);
    expect(screen.getAllByText("Attention").length).toBeGreaterThan(0);
    // The roster filter segments are now Radix tabs (role="tab"); their counts depend
    // on async board + schedule data.
    expect(await screen.findByRole("tab", { name: /Attention2/ })).toBeTruthy();
    expect(await screen.findByRole("tab", { name: /Inbox2/ })).toBeTruthy();
    expect(screen.getAllByText("inbox 1").length).toBeGreaterThanOrEqual(2);
    expect(screen.getAllByText("custom · paused").length).toBeGreaterThanOrEqual(1);
    fireEvent.click(screen.getByRole("tab", { name: /Attention2/ }));
    expect(screen.getByText("researcher")).toBeTruthy();
    expect(screen.getByText("ops")).toBeTruthy();
    fireEvent.click(screen.getByRole("tab", { name: /Inbox2/ }));
    expect(screen.getByText("researcher")).toBeTruthy();
    expect(screen.getByText("ops")).toBeTruthy();
    expect(screen.getAllByText("wake").length).toBeGreaterThan(0);
    expect(screen.getAllByText("high-impact allow · shell · L2").length).toBeGreaterThan(0);
    expect(screen.getAllByText("authority").length).toBeGreaterThan(0);
    expect(screen.getAllByText("model").length).toBeGreaterThan(0);
    expect(screen.getAllByText("model m-1 · task research · 1 fallback").length).toBeGreaterThan(0);
    expect(screen.getAllByText("skills").length).toBeGreaterThan(0);
    expect(screen.getAllByText("1 private skill").length).toBeGreaterThan(0);
    expect(screen.getAllByText("noise").length).toBeGreaterThan(0);
    expect(screen.getAllByText("silent on success · no memory writes · notify >= warning · cooldown 28800s · 1 schedule").length).toBeGreaterThan(0);
    expect(screen.getAllByText("schedule").length).toBeGreaterThan(0);
    expect(screen.getAllByText("schedule pressure: 1/1 frequent · fastest 5m").length).toBeGreaterThan(0);
    fireEvent.click(screen.getByLabelText("Pause frequent schedules for researcher"));
    fireEvent.click(await screen.findByRole("button", { name: "Pause schedules" }));
    await waitFor(() =>
      expect(postAction).toHaveBeenCalledWith("/api/schedule/enable", { id: "sched-fast", enabled: "false" }),
    );
    expect(screen.getAllByText("config").length).toBeGreaterThan(0);
    expect(screen.getAllByText("1 cfg · quiet policy · memory off").length).toBeGreaterThan(0);
    expect(screen.getAllByText("resilience").length).toBeGreaterThan(0);
    expect(screen.getAllByText("retry 3x · doctor guardian-doctor · self-repair").length).toBeGreaterThan(0);
    expect(screen.getAllByText("wake / health").length).toBeGreaterThan(0);
    expect(screen.getAllByText("cfg/links !2").length).toBeGreaterThan(0);
    expect(screen.getAllByText("in 10m").length).toBeGreaterThan(0);
    expect(screen.getAllByText("managed by lead").length).toBeGreaterThan(0);
    expect(screen.getByText("custom")).toBeTruthy();
    expect(screen.getByText("managed sub-agent")).toBeTruthy();
    expect(screen.getByText("retry: 3x")).toBeTruthy();
    expect(screen.getByText("guardian-doctor")).toBeTruthy();
    expect(screen.getByText("self-repair")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "incident" }));
    expect(location.hash).toBe("#incident/inc-root-1");
    expect(screen.getByText("quiet policy")).toBeTruthy();
    expect(screen.getAllByText(/silent on success · no memory writes · notify >= warning · cooldown 28800s/).length).toBeGreaterThan(0);
    expect(screen.getAllByText("cycle 2/5").length).toBeGreaterThan(0);
    expect(screen.getByText("ceiling L2")).toBeTruthy();
    expect(screen.getByText(/allow: memory, shell/)).toBeTruthy();
    expect(screen.getByText(/deny: notify/)).toBeTruthy();
    expect(screen.getByText(/cfg: AGEZT_X_MODE/)).toBeTruthy();
    expect(screen.getByText("You dig deep.")).toBeTruthy();
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
    expect(screen.getByText("custom · running")).toBeTruthy();
    expect(screen.getByText("Now")).toBeTruthy();
    expect(screen.getAllByText("using tool").length).toBeGreaterThan(0);
    expect(screen.getAllByText(/sync catalog/).length).toBeGreaterThan(0);
    expect(screen.getAllByText(/models.dev\/api.json/).length).toBeGreaterThan(0);
    expect(screen.getAllByText(/source: schedule/).length).toBeGreaterThan(0);
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
    expect(screen.getByText("quiet, memory off, notify >= warning, cooldown >=8h, capped, trust <= L2")).toBeTruthy();
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
    expect(screen.getByText("identity death plan")).toBeTruthy();
    expect(screen.getByText("profile deleted · 1 dependent sub-agent retired, not deleted · 2 mailbox/audit records retained")).toBeTruthy();
    expect(screen.getByText("custody split")).toBeTruthy();
    expect(screen.getByText("12 cleanup groups · 2 operator-retained groups · 4 audit/workflow groups retained by design · 1 sub-agent retired")).toBeTruthy();
    expect(screen.getByText("12 delete groups")).toBeTruthy();
    expect(screen.getByText("2 operator retained")).toBeTruthy();
    expect(screen.getByText("4 audit/workflow retained")).toBeTruthy();
    expect(screen.getByText("1 sub-agent retire")).toBeTruthy();
    expect(screen.getByLabelText("ops death certificate")).toBeTruthy();
    expect(screen.getByText("Death certificate")).toBeTruthy();
    expect(screen.getByText("retire dependent tree")).toBeTruthy();
    expect(screen.getAllByText("profile deleted; soul, settings, lifecycle removed").length).toBeGreaterThan(0);
    expect(screen.getAllByText("1 retired to graveyard").length).toBeGreaterThan(0);
    expect(screen.getAllByText(/2 workflow refs retained/).length).toBeGreaterThan(0);
    expect(screen.getAllByText("ready").length).toBeGreaterThan(0);
    expect(screen.getByText("Removal ledger")).toBeTruthy();
    expect(screen.getByText("delete profile")).toBeTruthy();
    expect(screen.getByText("1 retire")).toBeTruthy();
    expect(screen.getByText("mailbox/audit records are retained for inspection")).toBeTruthy();
    expect(screen.getByText("hard removal can proceed with the selected cascade")).toBeTruthy();
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
    expect(screen.getByText("orphan guard")).toBeTruthy();
    expect(screen.getByText("1 dependent sub-agent would lose their owner; removal is blocked until they are retired with the parent")).toBeTruthy();
    expect(screen.getByText("dependent sub-agent tree would be orphaned; include it so every descendant retires before this identity is deleted")).toBeTruthy();
    expect(screen.getByText("death blocked")).toBeTruthy();
    expect(screen.getAllByText("1 would be orphaned").length).toBeGreaterThan(0);
    expect(screen.getByText("blocked by dependent sub-agents")).toBeTruthy();
    expect(screen.getByText("select dependent sub-agent tree before hard removal")).toBeTruthy();
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
    await waitFor(() => expect(screen.getByText(/graveyard since/)).toBeTruthy());
    await waitFor(() => expect(screen.getByText("reason: replaced by guardian")).toBeTruthy());
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
    expect(screen.getByText("replaced by guardian")).toBeTruthy();
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
