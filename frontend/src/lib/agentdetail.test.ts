import { describe, it, expect } from "vitest";
import {
  agentScope,
  agentCorrelations,
  filterByCorrelation,
  filterAgentMemory,
  filterAgentSkills,
  summarizeAgent,
  lastFailure,
  healthSnapshot,
  summarizeConfigOverrides,
  summarizeAutoRepair,
  summarizeEscalations,
  escalationChainLabel,
  incidentLineageLabel,
  escalationOperationalTasks,
  summarizeAgentRuntimeStatus,
  fleetCardIssueSummary,
  summarizeProviderRoutingRow,
  summarizeAgentPolicyDenials,
  lastAutonomyRunbookSourceLabel,
  wakeLineage,
  escalationCausalityLineage,
  mailboxWakeFor,
  type AgentRepairStatus,
  type ReaperReport,
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
  {
    correlation_id: "c1",
    agent: "researcher",
    status: "completed",
    spent_mc: 1e9,
    started_unix_ms: 100,
  },
  {
    correlation_id: "c2",
    agent: "researcher",
    status: "failed",
    spent_mc: 5e8,
    started_unix_ms: 300,
  },
  {
    correlation_id: "c3",
    agent: "writer",
    status: "completed",
    spent_mc: 2e9,
    started_unix_ms: 200,
  },
  {
    correlation_id: "c4",
    agent: "researcher",
    status: "running",
    spent_mc: 0,
    started_unix_ms: 400,
  },
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
    expect(filterAgentSkills(skills, "researcher").map((s) => s.id)).toEqual([
      "s1",
    ]);
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

describe("healthSnapshot", () => {
  const report: ReaperReport = {
    degraded_agents: [
      {
        slug: "researcher",
        failures: 3,
        window: 5,
        threshold: 3,
        doctor_agent: "guardian-doctor",
        self_repair_enabled: true,
        escalate_to: "lead",
        last_failure_ms: 999,
      },
    ],
    misconfigured_agents: [
      {
        slug: "builder",
        issues: ["AGEZT_MAX_ITER: must be an integer"],
        doctor_agent: "guardian-doctor",
        self_repair_enabled: true,
        escalate_to: "lead",
      },
      { slug: "researcher", issues: ["AGEZT_MODEL: value is blank"] },
    ],
    routing_pressure_agents: [
      {
        slug: "router",
        count: 3,
        threshold: 3,
        window_sec: 3600,
        doctor_agent: "guardian-doctor",
        self_repair_enabled: true,
        escalate_to: "lead",
        last_failed_model: "gpt-5",
        last_next_model: "gpt-4.1",
        last_reason: "provider timeout",
      },
    ],
    routing_forced_probation_agents: [
      {
        slug: "router-forced",
        count: 2,
        threshold: 3,
        window_sec: 3600,
        doctor_agent: "guardian-doctor",
        self_repair_enabled: true,
        escalate_to: "lead",
        task_type: "code",
        forced_chain: ["gpt-5", "gpt-4.1"],
        last_reason: "provider timeout",
      },
    ],
    routing_forced_failed_agents: [
      {
        slug: "router-force-failed",
        count: 4,
        threshold: 3,
        window_sec: 3600,
        doctor_agent: "guardian-doctor",
        self_repair_enabled: true,
        escalate_to: "lead",
        task_type: "code",
        forced_chain: ["gpt-5", "gpt-4.1"],
        routing_force_generation: 2,
        last_reason: "provider timeout",
      },
    ],
    routing_forced_exhausted_agents: [
      {
        slug: "router-force-exhausted",
        count: 5,
        threshold: 3,
        window_sec: 3600,
        doctor_agent: "guardian-doctor",
        self_repair_enabled: true,
        escalate_to: "lead",
        task_type: "code",
        forced_chain: ["gpt-5", "gpt-4.1"],
        routing_force_generation: 3,
        last_reason: "provider timeout",
      },
    ],
    routing_unstable_agents: [
      {
        slug: "router-loop",
        count: 1,
        threshold: 1,
        window_sec: 3600,
        doctor_agent: "guardian-doctor",
        self_repair_enabled: true,
        escalate_to: "lead",
        task_type: "code",
        current_chain: ["gpt-5", "gpt-4.1"],
        previous_chain: ["gpt-4.1", "deepseek-chat"],
        last_reason: "provider timeout",
      },
    ],
    dead_agents: [{ slug: "writer", last_active_ms: 123 }],
  };

  it("reports graveyard first", () => {
    const got = healthSnapshot("researcher", true, report);
    expect(got.state).toBe("retired");
  });

  it("reports degraded agents with doctor/self-repair detail", () => {
    const got = healthSnapshot("researcher", false, report);
    expect(got.state).toBe("degraded");
    expect(got.doctorAgent).toBe("guardian-doctor");
    expect(got.selfRepairEnabled).toBe(true);
    expect(got.escalateTo).toBe("lead");
    expect(got.detail).toContain("config/hierarchy");
  });

  it("reports misconfigured agents when config health is the only signal", () => {
    const got = healthSnapshot("builder", false, report);
    expect(got.state).toBe("misconfigured");
    expect(got.detail).toContain("config/hierarchy");
    expect(got.configIssues).toEqual(["AGEZT_MAX_ITER: must be an integer"]);
  });

  it("reports stale agents from the reaper list", () => {
    const got = healthSnapshot("writer", false, report);
    expect(got.state).toBe("stale");
    expect(got.lastActiveMs).toBe(123);
  });

  it("reports routing pressure as degraded health when it is the only signal", () => {
    const got = healthSnapshot("router", false, report);
    expect(got.state).toBe("degraded");
    expect(got.label).toBe("fallback pressure");
    expect(got.detail).toContain("3 model-chain fallback hop");
    expect(got.detail).toContain("gpt-5 → gpt-4.1");
  });

  it("reports unstable routing before plain routing pressure", () => {
    const got = healthSnapshot("router-loop", false, report);
    expect(got.state).toBe("unstable");
    expect(got.label).toBe("unstable routing");
    expect(got.detail).toContain("current gpt-5 → gpt-4.1");
    expect(got.detail).toContain("previous gpt-4.1 → deepseek-chat");
  });

  it("reports forced-chain probation before plain routing pressure", () => {
    const got = healthSnapshot("router-forced", false, report);
    expect(got.state).toBe("stabilizing");
    expect(got.label).toBe("forced-chain probation");
    expect(got.detail).toContain("forced gpt-5 → gpt-4.1");
  });

  it("reports forced-chain failures distinctly after probation expires", () => {
    const got = healthSnapshot("router-force-failed", false, report);
    expect(got.state).toBe("force_failed");
    expect(got.label).toBe("forced chain failed");
    expect(got.detail).toContain("probation expired");
    expect(got.detail).toContain("forced gpt-5 → gpt-4.1");
    expect(got.detail).toContain("generation 2");
  });

  it("reports forced-chain exhaustion distinctly after repeated forced generations", () => {
    const got = healthSnapshot("router-force-exhausted", false, report);
    expect(got.state).toBe("force_exhausted");
    expect(got.label).toBe("forced chain exhausted");
    expect(got.detail).toContain("repeated forced-chain retries");
    expect(got.detail).toContain("generation 3");
  });

  it("falls back to healthy when no signal exists", () => {
    const got = healthSnapshot("ghost", false, report);
    expect(got.state).toBe("healthy");
  });
});

describe("summarizeConfigOverrides", () => {
  it("separates runtime overrides from generic config and explains their effects", () => {
    const got = summarizeConfigOverrides({
      AGEZT_MODEL: "gpt-5",
      AGEZT_PARALLEL_TOOLS: "4",
      AGEZT_X_MODE: "agent-only",
    });
    expect(got.runtime.map((r) => r.key)).toEqual([
      "AGEZT_MODEL",
      "AGEZT_PARALLEL_TOOLS",
    ]);
    expect(got.runtime[0]?.effect).toContain("gpt-5");
    expect(got.runtime[1]?.effect).toContain("4");
    expect(got.generic).toEqual([{ key: "AGEZT_X_MODE", value: "agent-only" }]);
  });

  it("marks malformed runtime values invalid", () => {
    const got = summarizeConfigOverrides({
      AGEZT_MAX_ITER: "abc",
      AGEZT_DISABLE_HEURISTIC_BYPASS: "maybe",
      AGEZT_AUTO_CONTINUE_WAIT: "later",
    });
    expect(got.runtime.every((r) => r.valid === false)).toBe(true);
    expect(got.runtime.map((r) => r.issue)).toEqual([
      "must be a Go duration like 250ms, 2s, 1m30s",
      "must be a boolean like true/false or 1/0",
      "must be an integer",
    ]);
  });
});

describe("summarizeAutoRepair", () => {
  it("falls back to idle when the agent has no repair history", () => {
    expect(summarizeAutoRepair(null).state).toBe("idle");
  });

  it("reports queued autonomous repairs", () => {
    const status: AgentRepairStatus = {
      latest: {
        mode: "misconfigured",
        phase: "queued",
        issues: ["AGEZT_MAX_ITER: must be an integer"],
        next_eligible_ms: 123,
      },
    };
    const got = summarizeAutoRepair(status);
    expect(got.state).toBe("queued");
    expect(got.detail).toContain("queued or in flight");
    expect(got.nextEligibleMs).toBe(123);
  });

  it("reports completed autonomous repairs and applied changes", () => {
    const got = summarizeAutoRepair({
      latest: {
        mode: "misconfigured",
        phase: "completed",
        applied: ["config_overrides", "model"],
      },
    });
    expect(got.state).toBe("completed");
    expect(got.detail).toContain("2 profile change");
  });

  it("reports failed autonomous repairs", () => {
    const got = summarizeAutoRepair({
      latest: {
        mode: "misconfigured",
        phase: "failed",
        error: "provider unavailable",
      },
    });
    expect(got.state).toBe("failed");
    expect(got.detail).toContain("provider unavailable");
  });

  it("reports exhausted self-repair attempt budgets", () => {
    const got = summarizeAutoRepair({
      latest: {
        mode: "misconfigured",
        phase: "attempts_exhausted",
        self_repair_attempt: 2,
        self_repair_max_attempts: 2,
      },
    });
    expect(got.state).toBe("failed");
    expect(got.label).toBe("repair exhausted");
    expect(got.detail).toContain("2/2");
    expect(got.detail).toContain("owner/parent");
  });

  it("reports deterministic resolution follow-up failures", () => {
    const got = summarizeAutoRepair({
      latest: {
        mode: "misconfigured",
        phase: "resolution_failed",
        resolution: "retired",
        reason: "permission denied",
      },
    });
    expect(got.state).toBe("failed");
    expect(got.label).toBe("retired failed");
    expect(got.detail).toContain("permission denied");
  });

  it("reports degraded doctor runs distinctly", () => {
    const got = summarizeAutoRepair({
      latest: { mode: "degraded", phase: "queued" },
    });
    expect(got.label).toBe("doctor queued");
    expect(got.detail).toContain("degraded agent");
  });

  it("reports routing repair runs distinctly", () => {
    const got = summarizeAutoRepair({
      latest: { mode: "routing", phase: "queued" },
    });
    expect(got.label).toBe("routing queued");
    expect(got.detail).toContain("routing repair");
  });

  it("reports routing rewrites with task chain detail", () => {
    const got = summarizeAutoRepair({
      latest: {
        mode: "routing",
        phase: "completed",
        routing_task_type: "code",
        routing_task_model_chain: ["gpt-4.1", "deepseek-chat"],
      },
    });
    expect(got.state).toBe("completed");
    expect(got.label).toBe("routing stabilized");
    expect(got.detail).toContain("code");
    expect(got.detail).toContain("gpt-4.1 → deepseek-chat");
  });

  it("reports routing rollback runs distinctly", () => {
    const queued = summarizeAutoRepair({
      latest: {
        mode: "routing",
        phase: "routing_rollback_queued",
        routing_task_type: "code",
        routing_task_model_chain: ["gpt-5", "gpt-4.1"],
      },
    });
    expect(queued.state).toBe("queued");
    expect(queued.label).toBe("rollback queued");
    expect(queued.detail).toContain("gpt-5 → gpt-4.1");

    const completed = summarizeAutoRepair({
      latest: {
        mode: "routing",
        phase: "routing_rollback_completed",
        routing_task_type: "code",
        routing_task_model_chain: ["gpt-5", "gpt-4.1"],
        previous_routing_task_model_chain: ["gpt-4.1", "deepseek-chat"],
      },
    });
    expect(completed.state).toBe("completed");
    expect(completed.label).toBe("rolled back");
    expect(completed.detail).toContain("from gpt-4.1 → deepseek-chat");

    const failed = summarizeAutoRepair({
      latest: {
        mode: "routing",
        phase: "routing_rollback_failed",
        error: "persist failed",
      },
    });
    expect(failed.state).toBe("failed");
    expect(failed.label).toBe("rollback failed");
    expect(failed.detail).toContain("persist failed");
  });

  it("reports unstable routing detection as an escalation state", () => {
    const got = summarizeAutoRepair({
      latest: {
        mode: "routing_unstable",
        phase: "routing_unstable_detected",
        routing_task_type: "code",
      },
    });
    expect(got.state).toBe("failed");
    expect(got.label).toBe("unstable routing");
    expect(got.detail).toContain("escalated");
  });

  it("reports forced-chain failure detection as an escalation state", () => {
    const got = summarizeAutoRepair({
      latest: {
        mode: "routing_forced_failed",
        phase: "routing_forced_failed_detected",
        routing_task_type: "code",
        routing_force_generation: 2,
      },
    });
    expect(got.state).toBe("failed");
    expect(got.label).toBe("forced chain failed");
    expect(got.detail).toContain("probation");
    expect(got.detail).toContain("escalated");
    expect(got.detail).toContain("gen 2");
  });

  it("reports forced-chain exhaustion as a harder escalation state", () => {
    const got = summarizeAutoRepair({
      latest: {
        mode: "routing_forced_exhausted",
        phase: "routing_force_exhausted_detected",
        routing_task_type: "code",
        routing_force_generation: 3,
      },
    });
    expect(got.state).toBe("failed");
    expect(got.label).toBe("forced chain exhausted");
    expect(got.detail).toContain("human-grade ownership");
    expect(got.detail).toContain("gen 3");
  });

  it("surfaces owner wake after a failed repair", () => {
    const got = summarizeAutoRepair({
      latest: {
        mode: "misconfigured",
        phase: "escalation_woke",
        target_agent: "lead",
      },
    });
    expect(got.state).toBe("failed");
    expect(got.label).toBe("manager woke");
    expect(got.detail).toContain("lead");
  });

  it("surfaces escalation thread closure by the owner agent", () => {
    const got = summarizeAutoRepair({
      latest: {
        mode: "misconfigured",
        phase: "escalation_answered",
        target_agent: "lead",
      },
    });
    expect(got.state).toBe("completed");
    expect(got.label).toBe("manager answered");
    expect(got.detail).toContain("lead");
  });

  it("surfaces delegated escalation closures distinctly", () => {
    const got = summarizeAutoRepair({
      latest: {
        mode: "misconfigured",
        phase: "escalation_answered",
        target_agent: "lead",
        resolution: "delegated",
        resolution_summary: "needs infra owner review",
        delegate_to: "infra-lead",
      },
    });
    expect(got.state).toBe("completed");
    expect(got.label).toBe("manager delegated");
    expect(got.detail).toContain("infra-lead");
    expect(got.detail).toContain("needs infra owner review");
  });

  it("surfaces applied force-chain resolutions distinctly", () => {
    const got = summarizeAutoRepair({
      latest: {
        mode: "routing",
        phase: "resolution_applied",
        target_agent: "lead",
        resolution: "force_chain",
        routing_task_type: "code",
        routing_task_model_chain: ["gpt-5", "gpt-4.1"],
        routing_force_generation: 2,
      },
    });
    expect(got.state).toBe("completed");
    expect(got.label).toBe("manager forced chain");
    expect(got.detail).toContain("code");
    expect(got.detail).toContain("gpt-5");
    expect(got.detail).toContain("gen 2");
  });

  it("surfaces delegated follow-up queue and wake states", () => {
    const queued = summarizeAutoRepair({
      latest: {
        mode: "misconfigured",
        phase: "delegation_queued",
        delegate_to: "infra-lead",
        resolution_summary: "needs infra owner review",
      },
    });
    expect(queued.state).toBe("queued");
    expect(queued.label).toBe("delegation queued");
    expect(queued.detail).toContain("infra-lead");

    const woke = summarizeAutoRepair({
      latest: {
        mode: "misconfigured",
        phase: "delegation_woke",
        delegate_to: "infra-lead",
      },
    });
    expect(woke.state).toBe("completed");
    expect(woke.label).toBe("delegation woke");
  });

  it("surfaces delegated follow-up failures", () => {
    const got = summarizeAutoRepair({
      latest: {
        mode: "misconfigured",
        phase: "delegation_failed",
        delegate_to: "infra-lead",
        reason: "target agent infra-lead is paused",
      },
    });
    expect(got.state).toBe("failed");
    expect(got.label).toBe("delegation failed");
    expect(got.detail).toContain("infra-lead");
    expect(got.detail).toContain("paused");
  });
});

describe("summarizeAgentRuntimeStatus", () => {
  it("surfaces misconfigured health and queued repair state for cards", () => {
    const got = summarizeAgentRuntimeStatus({
      health_state: "misconfigured",
      misconfiguration_count: 2,
      repair_state: "queued",
      repair_inflight: 1,
      repair_incident_id: "inc-child-123456789",
      repair_root_incident_id: "inc-root-123456789",
      repair_root_agent: "builder",
      repair_chain_depth: 1,
    });
    expect(got.healthText).toBe("cfg/links !2");
    expect(got.repairKindText).toBe("repair");
    expect(got.repairText).toBe("repair 1");
    expect(got.repairTone).toBe("accent");
    expect(got.repairIncidentText).toBe("incident");
    expect(got.repairIncidentId).toBe("inc-root-123456789");
    expect(got.repairIncidentDetail).toContain("root builder");
  });

  it("surfaces failed repair state", () => {
    const got = summarizeAgentRuntimeStatus({
      health_state: "healthy",
      repair_mode: "misconfigured",
      repair_state: "failed",
      repair_label: "repair failed",
      repair_last_error: "tool denied",
      repair_next_eligible_ms: Date.UTC(2026, 0, 1, 12, 0, 0),
    });
    expect(got.healthText).toBeUndefined();
    expect(got.repairKindText).toBe("repair");
    expect(got.repairText).toBe("repair failed");
    expect(got.repairDetail).toContain("last error: tool denied");
    expect(got.repairDetail).toContain("next eligible");
    expect(got.repairTone).toBe("bad");
  });

  it("surfaces exhausted self-repair budgets on runtime cards", () => {
    const got = summarizeAgentRuntimeStatus({
      repair_mode: "misconfigured",
      repair_state: "attempts_exhausted",
      repair_label: "repair exhausted",
      repair_self_attempt: 2,
      repair_self_max_attempts: 2,
    });
    expect(got.repairKindText).toBe("repair");
    expect(got.repairText).toBe("repair exhausted");
    expect(got.repairDetail).toContain("attempt 2/2");
    expect(got.repairTone).toBe("bad");
  });

  it("surfaces degraded doctor queue distinctly", () => {
    const got = summarizeAgentRuntimeStatus({
      repair_mode: "degraded",
      repair_state: "queued",
      repair_inflight: 2,
    });
    expect(got.repairKindText).toBe("doctor");
    expect(got.repairText).toBe("doctor 2");
  });

  it("surfaces routing fallback pressure distinctly", () => {
    const got = summarizeAgentRuntimeStatus({
      routing_fallback_count: 3,
      routing_last_failed: "gpt-5",
      routing_last_next: "gpt-4.1",
    });
    expect(got.routingText).toBe("fallback 3");
    expect(got.routingTone).toBe("bad");
    expect(got.routingDetail).toBe("gpt-5 → gpt-4.1");
    expect(got.routingFallbackCount).toBe(3);
  });

  it("surfaces whole-run retry pressure distinctly", () => {
    const got = summarizeAgentRuntimeStatus({
      retry_count: 2,
      retry_next_attempt: 3,
      retry_max_attempts: 4,
      retry_last_reason: "provider timeout",
      retry_last_ts_ms: Date.UTC(2026, 0, 1, 12, 0, 0),
    });
    expect(got.retryText).toBe("retry 2");
    expect(got.retryTone).toBe("bad");
    expect(got.retryDetail).toContain("attempt 3/4");
    expect(got.retryDetail).toContain("provider timeout");
    expect(got.retryCount).toBe(2);
  });

  it("surfaces wake bindings for roster cards", () => {
    const got = summarizeAgentRuntimeStatus({
      wake_schedule_count: 1,
      wake_standing_count: 2,
      next_wake_ms: 123456,
      next_wake_label: "check disks",
      wake_event_subjects: ["ops.alert"],
    });
    expect(got.wakeText).toBe("wake 3");
    expect(got.wakeTone).toBe("accent");
    expect(got.wakeScheduleCount).toBe(1);
    expect(got.wakeStandingCount).toBe(2);
    expect(got.nextWakeMs).toBe(123456);
    expect(got.wakeDetail).toContain("next: check disks");
    expect(got.wakeDetail).toContain("events: ops.alert");
  });

  it("surfaces live running state for roster cards", () => {
    const got = summarizeAgentRuntimeStatus({
      active_run_count: 2,
      active_intent: "check disks",
      active_model: "gpt-5",
      active_correlation_id: "corr-1",
      active_started_ms: 123456,
      active_last_event_ms: 123999,
      active_phase: "using tool",
      active_detail: "shell",
      active_tool: "shell.exec",
      active_wake_source: "schedule",
      active_wake_reason: "intent",
      active_schedule_id: "sched-ops",
    });
    expect(got.liveText).toBe("running 2");
    expect(got.liveTone).toBe("accent");
    expect(got.activeRunCount).toBe(2);
    expect(got.activeCorrelationId).toBe("corr-1");
    expect(got.activeStartedMs).toBe(123456);
    expect(got.activeLastEventMs).toBe(123999);
    expect(got.activePhase).toBe("using tool");
    expect(got.activeTool).toBe("shell.exec");
    expect(got.activeModel).toBe("gpt-5");
    expect(got.activeContextDetail).toContain("source: schedule");
    expect(got.activeContextDetail).toContain("schedule: sched-ops");
    expect(got.operationalState).toBe("running");
    expect(got.liveDetail).toContain("using tool");
    expect(got.liveDetail).toContain("check disks");
    expect(got.liveDetail).toContain("shell");
    expect(got.liveDetail).toContain("tool: shell.exec");
    expect(got.liveDetail).toContain("source: schedule");
    expect(got.liveDetail).toContain("model: gpt-5");
    expect(got.liveDetail).toContain("corr: corr-1");
  });

  it("uses standing wake names in live context when available", () => {
    const got = summarizeAgentRuntimeStatus({
      active_run_count: 1,
      active_intent: "triage ops event",
      active_phase: "starting",
      active_wake_source: "standing",
      active_standing_id: "standing-ops-events",
      active_standing_name: "ops events",
      active_trigger_subject: "ops.alert",
    });

    expect(got.activeContextDetail).toContain("source: standing");
    expect(got.activeContextDetail).toContain("standing: ops events");
    expect(got.activeContextDetail).toContain("trigger: ops.alert");
    expect(got.liveDetail).toContain("standing: ops events");
    expect(got.liveDetail).not.toContain("standing-ops-events");
  });

  it("surfaces sleeping operational state and last activity", () => {
    const got = summarizeAgentRuntimeStatus({
      operational_state: "sleeping",
      operational_label: "sleeping",
      last_activity_ms: 456,
      last_activity_summary: "completed a run",
      last_autonomy_runbook: {
        trigger_contract: "operator_schedule_channel",
        route_contract: "self_owned",
        recovery_contract: "self_repair",
        sleep_contract: "persistent",
        phase: "completed",
        correlation_id: "wake-1",
      },
    });
    expect(got.liveText).toBeUndefined();
    expect(got.operationalState).toBe("sleeping");
    expect(got.operationalText).toBe("sleeping");
    expect(got.lastActivityMs).toBe(456);
    expect(got.lastActivitySummary).toBe("completed a run");
    expect(got.lastAutonomyRunbook).toMatchObject({
      recovery_contract: "self_repair",
      sleep_contract: "persistent",
      phase: "completed",
      correlation_id: "wake-1",
    });
  });

  it("surfaces unstable routing health distinctly", () => {
    const got = summarizeAgentRuntimeStatus({
      health_state: "unstable",
      health_label: "unstable routing",
      repair_state: "routing_unstable_detected",
      repair_mode: "routing_unstable",
      repair_label: "unstable routing",
    });
    expect(got.healthText).toBe("unstable routing");
    expect(got.repairKindText).toBe("routing");
    expect(got.repairText).toBe("unstable routing");
    expect(got.repairTone).toBe("bad");
  });

  it("surfaces forced-chain probation health distinctly", () => {
    const got = summarizeAgentRuntimeStatus({
      health_state: "stabilizing",
      health_label: "forced-chain probation",
    });
    expect(got.healthText).toBe("forced-chain probation");
    expect(got.healthTone).toBe("muted");
  });

  it("surfaces forced-chain failure health distinctly", () => {
    const got = summarizeAgentRuntimeStatus({
      health_state: "force_failed",
      health_label: "forced chain failed",
      repair_state: "routing_forced_failed_detected",
      repair_mode: "routing_forced_failed",
      repair_label: "forced chain failed",
    });
    expect(got.healthText).toBe("forced chain failed");
    expect(got.healthTone).toBe("bad");
    expect(got.repairKindText).toBe("routing");
    expect(got.repairText).toBe("forced chain failed");
    expect(got.repairTone).toBe("bad");
  });

  it("surfaces forced-chain exhaustion health distinctly", () => {
    const got = summarizeAgentRuntimeStatus({
      health_state: "force_exhausted",
      health_label: "forced chain exhausted",
      repair_state: "routing_force_exhausted_detected",
      repair_mode: "routing_forced_exhausted",
      repair_label: "forced chain exhausted",
    });
    expect(got.healthText).toBe("forced chain exhausted");
    expect(got.healthTone).toBe("bad");
    expect(got.repairKindText).toBe("routing");
    expect(got.repairText).toBe("forced chain exhausted");
    expect(got.repairTone).toBe("bad");
  });

  it("surfaces routing repair mode distinctly", () => {
    const got = summarizeAgentRuntimeStatus({
      repair_mode: "routing",
      repair_state: "queued",
      repair_inflight: 1,
    });
    expect(got.repairKindText).toBe("routing");
    expect(got.repairText).toBe("routing 1");
  });
});

describe("summarizeProviderRoutingRow", () => {
  it("formats fallback hops with failed→next primary text and reason detail", () => {
    const got = summarizeProviderRoutingRow({
      kind: "fallback",
      failed: "gpt-5",
      next: "gpt-4.1",
      task_type: "code",
      reason: "provider timeout",
      scope: "agent/researcher",
    });
    expect(got.kindLabel).toBe("fallback");
    expect(got.kindTone).toBe("bad");
    expect(got.stateLabel).toBe("degraded");
    expect(got.stateTone).toBe("bad");
    expect(got.primaryText).toBe("gpt-5 → gpt-4.1");
    expect(got.failedModel).toBe("gpt-5");
    expect(got.nextModel).toBe("gpt-4.1");
    expect(got.secondaryText).toBe("code · agent/researcher");
    expect(got.detail).toBe("provider timeout");
  });

  it("formats route decisions with primary model and chain detail", () => {
    const got = summarizeProviderRoutingRow({
      kind: "route",
      primary: "@research-chain",
      task_type: "research",
      chain: "gpt-5,gpt-4.1",
    });
    expect(got.kindLabel).toBe("route");
    expect(got.kindTone).toBe("muted");
    expect(got.stateLabel).toBe("normal route");
    expect(got.stateTone).toBe("good");
    expect(got.primaryText).toBe("@research-chain");
    expect(got.primaryModel).toBe("@research-chain");
    expect(got.secondaryText).toBe("research");
    expect(got.detail).toBe("gpt-5,gpt-4.1");
  });
});

describe("summarizeEscalations", () => {
  it("counts open and acked escalation responsibilities", () => {
    const got = summarizeEscalations([
      {
        message_id: "m1",
        status: "open",
        ts_unix_ms: 10,
        origin_kind: "doctor",
        origin_agent: "guardian-doctor",
        root_agent: "builder",
        chain_depth: 0,
      },
      {
        message_id: "m3",
        status: "open",
        ts_unix_ms: 15,
        origin_kind: "delegated",
        origin_agent: "lead",
        root_agent: "builder",
        chain_depth: 1,
      },
      {
        message_id: "m2",
        status: "acked",
        ts_unix_ms: 20,
        source_agent: "builder",
      },
    ]);
    expect(got.openCount).toBe(2);
    expect(got.ackedCount).toBe(1);
    expect(got.doctorOpenCount).toBe(1);
    expect(got.delegatedOpenCount).toBe(1);
    expect(got.latest?.message_id).toBe("m2");
  });
});

describe("escalationChainLabel", () => {
  it("formats doctor and delegated chain provenance", () => {
    expect(
      escalationChainLabel({
        origin_kind: "doctor",
        origin_agent: "guardian-doctor",
        root_agent: "builder",
        chain_depth: 0,
      }),
    ).toBe("doctor guardian-doctor · root builder");
    expect(
      escalationChainLabel({
        origin_kind: "delegated",
        origin_agent: "lead",
        root_agent: "builder",
        chain_depth: 2,
      }),
    ).toBe("delegated by lead · root builder · hop 2");
  });
});

describe("incidentLineageLabel", () => {
  it("formats root, hop, incident, and next owner", () => {
    expect(
      incidentLineageLabel({
        root_agent: "builder",
        chain_depth: 2,
        incident_id: "builder-1710000000000-provider-timeout",
        delegate_to: "infra-lead",
      }),
    ).toBe(
      "root builder · hop 2 · incident builder-1710000000 · next owner infra-lead",
    );
  });
});

describe("escalationOperationalTasks", () => {
  it("derives live operational tasks from escalation rows", () => {
    const got = escalationOperationalTasks([
      {
        message_id: "m1",
        status: "open",
        source_agent: "builder",
        mode: "misconfigured",
        text: "repair failed",
        ts_unix_ms: 10,
      },
      {
        message_id: "m2",
        status: "acked",
        source_agent: "writer",
        mode: "degraded",
        text: "doctor failed",
        ts_unix_ms: 20,
      },
    ]);
    expect(got.map((row) => row.title)).toEqual([
      "Handle escalation for writer",
      "Handle escalation for builder",
    ]);
    expect(got[0]?.status).toBe("doing");
    expect(got[0]?.description).toContain("degraded doctor");
    expect(got[1]?.status).toBe("todo");
  });

  it("uses structured escalation closure details when present", () => {
    const got = escalationOperationalTasks([
      {
        message_id: "m1",
        status: "answered",
        source_agent: "builder",
        mode: "misconfigured",
        resolution: "delegated",
        resolution_summary: "needs infra owner review",
        delegate_to: "infra-lead",
        ts_unix_ms: 10,
      },
    ]);
    expect(got[0]?.status).toBe("done");
    expect(got[0]?.description).toContain("delegated to infra-lead");
    expect(got[0]?.description).toContain("needs infra owner review");
  });
});

describe("lastAutonomyRunbookSourceLabel", () => {
  it("labels schedule and standing wake provenance", () => {
    expect(
      lastAutonomyRunbookSourceLabel({ source: "schedule", schedule_id: "sched-ops" }),
    ).toBe("via schedule sched-ops");
    expect(
      lastAutonomyRunbookSourceLabel({ source: "standing", standing_name: "ops events" }),
    ).toBe("via standing ops events");
    expect(
      lastAutonomyRunbookSourceLabel({ source: "standing", standing_id: "standing-ops" }),
    ).toBe("via standing standing-ops");
  });

  it("labels mailbox wake provenance from the sender", () => {
    expect(
      lastAutonomyRunbookSourceLabel({
        source: "standing",
        wake_via: "mailbox",
        mailbox_from: "planner",
      }),
    ).toBe("via mailbox from planner");
    expect(
      lastAutonomyRunbookSourceLabel({ source: "standing", wake_via: "mailbox" }),
    ).toBe("via mailbox");
  });

  it("labels delegated wake provenance from the leader", () => {
    expect(
      lastAutonomyRunbookSourceLabel({ source: "delegated", delegated_by: "lead" }),
    ).toBe("via delegation by lead");
    expect(lastAutonomyRunbookSourceLabel({ source: "delegated" })).toBe(
      "via delegation",
    );
  });

  it("labels doctor wake provenance from the repaired agent", () => {
    expect(
      lastAutonomyRunbookSourceLabel({ source: "doctor", doctor_for: "builder" }),
    ).toBe("via doctor for builder");
    expect(lastAutonomyRunbookSourceLabel({ source: "doctor" })).toBe("via doctor");
  });
});

describe("mailboxWakeFor", () => {
  const wakes = { "msg-77": { correlation_id: "corr-1", trigger_subject: "board.dm.ops" } };

  it("returns the wake a message triggered", () => {
    expect(mailboxWakeFor(wakes, "msg-77")?.correlation_id).toBe("corr-1");
  });

  it("returns undefined for non-waking or missing ids", () => {
    expect(mailboxWakeFor(wakes, "msg-other")).toBeUndefined();
    expect(mailboxWakeFor(wakes, undefined)).toBeUndefined();
    expect(mailboxWakeFor(undefined, "msg-77")).toBeUndefined();
  });
});

describe("summarizeAgentPolicyDenials", () => {
  it("is the healthy muted case when nothing was refused", () => {
    expect(summarizeAgentPolicyDenials(undefined).tone).toBe("muted");
    expect(summarizeAgentPolicyDenials({ policy_denied_count: 0 }).count).toBe(0);
  });

  it("summarizes the count and most recent refusal", () => {
    const got = summarizeAgentPolicyDenials({
      policy_denied_count: 3,
      policy_denied_last_tool: "shell",
      policy_denied_last_capability: "process.exec",
      policy_denied_last_reason: "tool denied for this agent",
      policy_denied_last_hard: true,
    });
    expect(got.count).toBe(3);
    expect(got.text).toBe("3 tool calls refused");
    expect(got.tone).toBe("bad");
    expect(got.detail).toContain("shell (process.exec)");
    expect(got.detail).toContain("hard-denied");
  });

  it("uses warn tone for a soft (non-hard) denial", () => {
    expect(
      summarizeAgentPolicyDenials({ policy_denied_count: 1, policy_denied_last_tool: "http" }).tone,
    ).toBe("warn");
  });
});

describe("wakeLineage", () => {
  it("exposes the doctor incident as a navigable target", () => {
    const got = wakeLineage({ source: "doctor", doctor_for: "builder", incident_id: "inc-9" });
    expect(got.label).toBe("via doctor for builder");
    expect(got.incidentId).toBe("inc-9");
    expect(got.parentCorrelationId).toBeUndefined();
  });

  it("exposes the parent run for a delegated wake", () => {
    const got = wakeLineage({ source: "delegated", delegated_by: "lead", parent_correlation_id: "lead-run-1" });
    expect(got.label).toBe("via delegation by lead");
    expect(got.parentCorrelationId).toBe("lead-run-1");
  });

  it("is empty with no runbook", () => {
    expect(wakeLineage(undefined)).toEqual({ label: "" });
  });

  it("is empty for manual wakes with no source", () => {
    expect(lastAutonomyRunbookSourceLabel(undefined)).toBe("");
    expect(lastAutonomyRunbookSourceLabel({ phase: "completed" })).toBe("");
  });
});

describe("escalationCausalityLineage", () => {
  it("folds a doctor escalation into the wake run and incident chain", () => {
    const got = escalationCausalityLineage({
      origin_kind: "doctor",
      origin_agent: "guardian-doctor",
      wake_correlation_id: "wake-run-1",
      incident_id: "inc-child-abc",
      root_incident_id: "inc-root-abc",
      root_agent: "builder",
      chain_depth: 0,
    });
    expect(got.origin).toBe("doctor");
    expect(got.label).toBe("woke via doctor for guardian-doctor");
    expect(got.wakeCorrelationId).toBe("wake-run-1");
    expect(got.incidentId).toBe("inc-child-abc");
    expect(got.rootIncidentId).toBe("inc-root-abc");
    expect(got.rootAgent).toBe("builder");
    expect(got.chainDepth).toBe(0);
    expect(got.nextOwner).toBeUndefined();
    expect(got.parentIncidentId).toBeUndefined();
  });

  it("folds a delegated escalation with the parent incident and next owner", () => {
    const got = escalationCausalityLineage({
      origin_kind: "delegated",
      origin_agent: "lead",
      wake_correlation_id: "deleg-run-2",
      incident_id: "inc-child-2",
      root_incident_id: "inc-root-2",
      parent_incident_id: "inc-parent-2",
      delegate_to: "infra-lead",
      root_agent: "builder",
      chain_depth: 2,
    });
    expect(got.origin).toBe("delegated");
    expect(got.label).toBe("woke via delegation by lead · hop 2");
    expect(got.wakeCorrelationId).toBe("deleg-run-2");
    expect(got.parentIncidentId).toBe("inc-parent-2");
    expect(got.nextOwner).toBe("infra-lead");
    expect(got.chainDepth).toBe(2);
  });

  it("labels a delegated wake without an origin agent", () => {
    const got = escalationCausalityLineage({
      origin_kind: "delegated",
      wake_correlation_id: "r",
    });
    expect(got.label).toBe("woke via delegation");
    expect(got.origin).toBe("delegated");
  });

  it("omits the hop suffix when chain depth is zero or missing", () => {
    expect(
      escalationCausalityLineage({
        origin_kind: "doctor",
        origin_agent: "doc",
      }).label,
    ).toBe("woke via doctor for doc");
    expect(
      escalationCausalityLineage({
        origin_kind: "doctor",
        origin_agent: "doc",
        chain_depth: 0,
      }).label,
    ).toBe("woke via doctor for doc");
  });

  it("trims whitespace and drops blank fields", () => {
    const got = escalationCausalityLineage({
      origin_kind: "doctor",
      origin_agent: "  doc  ",
      wake_correlation_id: "  ",
      incident_id: "  inc  ",
    });
    expect(got.wakeCorrelationId).toBeUndefined();
    expect(got.incidentId).toBe("inc");
    expect(got.label).toBe("woke via doctor for doc");
  });

  it("returns an empty origin for null/undefined/non-escalation rows", () => {
    expect(escalationCausalityLineage(null)).toEqual({ origin: "", label: "" });
    expect(escalationCausalityLineage(undefined)).toEqual({
      origin: "",
      label: "",
    });
    expect(
      escalationCausalityLineage({ origin_kind: "" }).origin,
    ).toBe("");
    expect(
      escalationCausalityLineage({ origin_kind: "other" }).origin,
    ).toBe("");
  });
});

describe("fleetCardIssueSummary", () => {
  it("reports no issues for a null or healthy runtime status", () => {
    expect(fleetCardIssueSummary(null)).toEqual({ count: 0, tone: "none", detail: "" });
    const healthy = summarizeAgentRuntimeStatus({ health_state: "healthy" });
    expect(fleetCardIssueSummary(healthy)).toEqual({ count: 0, tone: "none", detail: "" });
  });

  it("collapses health/routing/retry/escalation into a single bad-tone count", () => {
    const rs = summarizeAgentRuntimeStatus({
      health_state: "misconfigured",
      misconfiguration_count: 2,
      routing_fallback_count: 3,
      retry_count: 1,
      escalation_open_count: 2,
    });
    const got = fleetCardIssueSummary(rs);
    expect(got.tone).toBe("bad");
    expect(got.count).toBe(4);
    expect(got.detail).toContain("routing fallback ×3");
    expect(got.detail).toContain("retry ×1");
    expect(got.detail).toContain("2 open escalations");
  });

  it("surfaces in-flight repair as an accent signal when otherwise healthy", () => {
    const rs = summarizeAgentRuntimeStatus({
      health_state: "healthy",
      repair_state: "queued",
      repair_inflight: 1,
    });
    const got = fleetCardIssueSummary(rs);
    expect(got.count).toBe(1);
    expect(got.tone).toBe("accent");
  });
});
