// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

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

import {
  AgentDetail,
  agentBoardMessages,
  agentControlInterventionSummary,
  agentLifecycleDecisionLedger,
  agentLifecycleActionResultSummary,
  agentRepairCommandSummary,
  agentRepairDecisionSummary,
  agentAutonomyRunbook,
  agentEntityContractLedger,
  agentHealthContractLedger,
  agentOperationsPassport,
  agentRuntimeDoctorLedger,
  agentSystemGuardianContract,
  agentMailboxSubjects,
  agentMailboxWakeContract,
  agentMailboxPassport,
  agentInboxPrioritySummary,
  agentLifecycleInterventionSummary,
  agentAuthorityContractSummary,
  agentAuthorityManifest,
  agentAuthorityLedger,
  agentCapabilityRiskPassport,
  agentConfigAuthorityContract,
  agentRemovalImpactPlan,
  agentRemovalRiskLabel,
  agentRetryPolicyDetail,
  agentRepairOperationsSummary,
  agentResourcePassportDetail,
  agentScheduleBindingTitle,
  agentDelegationPassportDetail,
  workflowToolAccessSummary,
  mailboxWakeArmIssue,
  mailboxSubjectBinding,
  messageAckedBy,
  messageAckedByLabel,
  normalizeNoiseToolPolicy,
  operatorWakeIssue,
  waitingForAgent,
} from "@/components/AgentDetail";
import { UIProvider } from "@/components/ui/feedback";

const withUI = (node: ReactNode) => <UIProvider>{node}</UIProvider>;

afterEach(cleanup);
beforeEach(() => {
  location.hash = "";
  getJSON.mockReset();
  postJSON.mockReset();
  postAction.mockReset();
  postJSON.mockResolvedValue({ removed: true, standing_removed: 1, schedules_removed: 1, memories_forgotten: 1, authored_memories_forgotten: 0, skills_archived: 1, configs_deleted: 1, subagents_retired: 1 });
  postAction.mockResolvedValue({});
  getJSON.mockImplementation((path: string, params?: { ref?: string }) => {
    if (path === "/api/agents/impact") {
      return Promise.resolve({
        standing_orders: ["night watch"],
        schedules: ["refresh (sch-1)"],
        memories: ["ops note (mem-1)"],
        authored_shared_memories: ["ops shared note (mem-shared-1)"],
        skills: ["ops skill (skill-1)"],
        configs: ["agent/ops/runtime [internal]"],
        workspaces: ["agents/ops"],
        workflow_refs: ["ops-flow/handoff handoff ops [tool]"],
        subagents: ["ops-worker [parent]"],
        subagent_standing_orders: ["ops-worker: worker watch"],
        subagent_schedules: ["ops-worker: worker refresh (sch-2)"],
        subagent_memories: ["ops-worker: worker note (mem-2)"],
        subagent_authored_shared_memories: ["ops-worker: worker shared note (mem-shared-2)"],
        subagent_skills: ["ops-worker: worker skill (skill-2)"],
        subagent_configs: ["ops-worker: agent/ops-worker/runtime [internal]"],
        subagent_workspaces: ["ops-worker: agents/ops-worker"],
        subagent_workflow_refs: ["ops-worker: worker-flow/delegate [tool]"],
        subagent_mailbox_messages: ["ops-worker: dm received (msg-2)"],
      });
    }
    if (path === "/api/memory") return Promise.resolve({ records: [] });
    if (path === "/api/skills") return Promise.resolve({ skills: [] });
    if (path === "/api/policy_log") return Promise.resolve({ decisions: [] });
    if (path === "/api/tool_log") return Promise.resolve({ invocations: [] });
    if (path === "/api/policy") return Promise.resolve({});
    if (path === "/api/edict_show") return Promise.resolve({ levels: { "code.exec": "L0", "memory": "L4", "net.fetch": "L2" } });
    if (path === "/api/tools_catalog")
      return Promise.resolve({
        tools: [
          { name: "memory", description: "remember", capability: "memory" },
          { name: "shell", description: "run commands", capability: "code.exec" },
          { name: "fetch", description: "fetch urls", capability: "net.fetch" },
        ],
      });
    if (path === "/api/agents/permissions" && params?.ref === "worker")
      return Promise.resolve({
        wake_access: {
          status: "managed",
          reason: "managed by lead",
          direct_callable: false,
          direct_allowed: false,
          schedule_allowed: false,
          channel_allowed: false,
          operator_allowed: false,
          delegation_allowed: true,
          delegation_scope: "manager",
          delegation_sources: ["lead"],
          manager: "lead",
        },
        permissions: [],
        config_entries: [],
        governance: {
          summary: "tools 0/0 allowed, 0 ask, 0 blocked, config 0/0 visible, trust L4",
          risk: "restricted",
        },
      });
    if (path === "/api/agents/permissions")
      return Promise.resolve({
        wake_access: {
          status: "direct",
          reason: "directly callable",
          direct_callable: true,
          direct_allowed: true,
          schedule_allowed: true,
          channel_allowed: true,
          operator_allowed: true,
          delegation_allowed: true,
          delegation_scope: "any",
        },
        permissions: [
          { name: "memory", capability: "memory", allowed: true, ask: false, status: "allowed", source: "edict", reason: "capability set to L4 (allow)" },
          { name: "shell", capability: "code.exec", allowed: false, ask: false, status: "hidden", source: "agent_allow", reason: "not in agent tool allowlist" },
          { name: "fetch", capability: "net.fetch", allowed: true, ask: true, status: "L2", source: "edict", reason: "capability set to L2 (ask)" },
        ],
        config_entries: [
          { key: "public:value", rating: "internal", visible: true, source: "config_global", reason: "visible to all eligible agents" },
          { key: "agent/ops/runtime", rating: "internal", visible: true, owned: true, source: "config_allowed", reason: "agent is in config allowed_agents", allowed_agents: ["ops"] },
          { key: "secret:value", rating: "secret", visible: false, source: "config_excluded", reason: "agent is in config excluded_agents", excluded_agents: ["ops"] },
        ],
        governance: {
          summary: "tools 1/3 allowed, 1 ask, 1 blocked, config 2/3 visible, trust L2",
          risk: "restricted",
          authority_boundary: "agent identity boundary",
          execution_boundary: "wakes and workflows invoke through ops policy",
          tool_policy: "1 direct · 1 ask · 1 blocked · trust L2",
          memory_policy: "agent/ops · writes enabled",
          memory_writes: "writes enabled",
          trust_ceiling: "L2",
          tool_count: 3,
          allowed_count: 1,
          ask_count: 1,
          blocked_count: 1,
          config_count: 3,
          config_visible_count: 2,
          config_hidden_count: 1,
        },
      });
    if (path === "/api/board") return Promise.resolve({ messages: [] });
    if (path === "/api/catalog")
      return Promise.resolve({
        providers: [
          {
            id: "openai",
            name: "OpenAI",
            credentialed: true,
            models: [
              { id: "gpt-5", name: "GPT-5", tool_call: true },
              { id: "gpt-4.1", name: "GPT-4.1", tool_call: true },
            ],
          },
        ],
      });
    if (path === "/api/chains") return Promise.resolve({ chains: {} });
    if (path === "/api/routing") return Promise.resolve({ chains: {} });
    if (path === "/api/provider_log") return Promise.resolve({ events: [] });
    if (path === "/api/reaper/scan") return Promise.resolve({});
    if (path === "/api/agents/repair_status") return Promise.resolve({});
    if (path === "/api/agents/escalations") return Promise.resolve({ escalations: [] });
    return Promise.resolve({});
  });
});

describe("normalizeNoiseToolPolicy", () => {
  it("turns memory write suppression into an explicit memory tool deny", () => {
    expect(normalizeNoiseToolPolicy(["memory", "shell"], ["notify"], true)).toEqual({
      allow: ["shell"],
      deny: ["notify", "memory"],
    });
    expect(normalizeNoiseToolPolicy(["shell"], ["MEMORY"], true)).toEqual({
      allow: ["shell"],
      deny: ["memory"],
    });
    expect(normalizeNoiseToolPolicy(["memory"], [], false)).toEqual({
      allow: ["memory"],
      deny: [],
    });
  });
});

describe("agentRetryPolicyDetail", () => {
  it("spells out attempts, backoff, delay, and retry reasons", () => {
    expect(agentRetryPolicyDetail({ retry_policy: { max_attempts: 3, backoff: "exponential", base_delay_sec: 30, max_delay_sec: 300, retry_on: ["error", "timeout"] } })).toBe(
      "up to 3 attempts · backoff exponential · delay 30s..300s · retry on error, timeout",
    );
    expect(agentRetryPolicyDetail({ retry_policy: { max_attempts: 1 } })).toBe("single attempt; no run-level retry");
    expect(agentRetryPolicyDetail({})).toBe("single attempt; no run-level retry");
  });

  it("summarizes repair command policy, latest event, and cooldown", () => {
    expect(agentRepairCommandSummary({
      retry_policy: { max_attempts: 3, backoff: "linear" },
      health_policy: { doctor_agent: "guardian-doctor" },
      self_repair: { enabled: true, max_attempts: 2, escalate_to: "lead" },
    }, {
      cooldown_sec: 60,
      latest: { phase: "failed", mode: "degraded", error: "tool denied" },
    })).toEqual({
      contract: "up to 3 attempts · backoff linear · no delay · retry on error, timeout · doctor guardian-doctor · self-repair on 2x · escalate lead",
      latest: "failed · doctor · tool denied",
      cooldown: "cooldown 60s",
    });
    expect(agentRepairCommandSummary({}, null).latest).toBe("no repair events yet");
  });
});

describe("agentResourcePassportDetail", () => {
  it("summarizes workspace, memory, data lake access, and config overlays", () => {
    expect(agentResourcePassportDetail({ workdir: "agents/ops", memory_scope: "private", tool_allow: ["db", "memory"], config_overrides: { AGEZT_MAX_ITER: "4" } }, "ops")).toBe(
      "workspace agents/ops · memory private · data lake via db · 1 config override",
    );
    expect(agentResourcePassportDetail({ tool_allow: ["memory"], tool_deny: ["db"] }, "ops")).toBe(
      "shared workspace · memory ops · data lake blocked · default config",
    );
    expect(agentResourcePassportDetail({ tool_allow: ["memory"] }, "ops")).toContain("data lake not allowlisted");
  });
});

describe("agentControlInterventionSummary", () => {
  it("turns control center cell tones into an operator intervention verdict", () => {
    expect(agentControlInterventionSummary([
      { label: "tools", value: "broad tools", detail: "allow all", tone: "warn" },
      { label: "trust", value: "L4", detail: "open", tone: "warn" },
      { label: "config", value: "default", detail: "defaults", tone: "muted" },
    ])).toEqual({
      label: "control review",
      detail: "tools, trust need review; open capability control to adjust tool allow/deny, trust, memory, config, or noise policy",
      tone: "warn",
    });
    expect(agentControlInterventionSummary([
      { label: "tools", value: "blocked", detail: "bad", tone: "bad" },
    ])).toEqual({
      label: "control blocked",
      detail: "tools require intervention; open capability control to repair access, trust, memory, config, or noise policy",
      tone: "bad",
    });
    expect(agentControlInterventionSummary([
      { label: "tools", value: "allow 1", detail: "ok", tone: "good" },
      { label: "trust", value: "L2", detail: "ok", tone: "good" },
    ])).toMatchObject({ label: "control ready", tone: "good" });
  });
});

describe("agentScheduleBindingTitle", () => {
  it("distinguishes agent wake schedules from workflow/tool schedules running as an agent", () => {
    expect(agentScheduleBindingTitle({ intent: "check disks" }, "ops")).toBe("wakes ops: check disks");
    expect(agentScheduleBindingTitle({ target: "workflow", workflow: "nightly-sync" }, "ops")).toBe("runs workflow nightly-sync as ops");
    expect(agentScheduleBindingTitle({ target: "tool", tool: "shell" }, "ops")).toBe("invokes tool shell as ops");
  });
});

describe("agentDelegationPassportDetail", () => {
  it("summarizes direct and manager-only wake authority", () => {
    expect(agentDelegationPassportDetail({}).value).toBe("operator/schedule/channel");
    expect(agentDelegationPassportDetail({ direct_callable: false, parent_agent: "lead" })).toEqual({
      value: "manager-only · lead",
      detail: "direct operator, schedule, and channel wake are blocked; delegation is accepted from lead",
      tone: "warn",
    });
    expect(agentDelegationPassportDetail({ kind: "subagent" })).toEqual({
      value: "manager-only · no manager",
      detail: "direct wake is blocked and no parent/owner delegation source is configured",
      tone: "bad",
    });
    expect(agentDelegationPassportDetail({ direct_callable: true }, { schedule_allowed: false, channel_allowed: false, operator_allowed: true, delegation_allowed: true })).toEqual({
      value: "allowed operator, delegation",
      detail: "blocked schedule, channel; allowed operator, delegation",
      tone: "warn",
    });
    expect(agentDelegationPassportDetail({ retired: true, parent_agent: "lead", direct_callable: false })).toEqual({
      value: "graveyard · blocked",
      detail: "graveyard agent cannot be woken until revived",
      tone: "muted",
    });
  });
});

describe("workflowToolAccessSummary", () => {
  it("summarizes whether an agent can author and run workflow chains", () => {
    expect(workflowToolAccessSummary([])).toEqual({
      label: "workflow tool not registered",
      detail: "agent cannot author or run workflow chains through the workflow tool in this daemon snapshot",
      tone: "muted",
    });
    expect(workflowToolAccessSummary([{ name: "workflow", description: "", capability: "workflow", allowed: true, ask: false, label: "allowed", reason: "capability L4" }])).toEqual({
      label: "workflow chains available",
      detail: "capability L4",
      tone: "good",
    });
    expect(workflowToolAccessSummary([{ name: "workflow", description: "", capability: "workflow", allowed: true, ask: true, label: "L2", reason: "requires approval" }])).toEqual({
      label: "workflow chains ask-gated",
      detail: "requires approval",
      tone: "warn",
    });
    expect(workflowToolAccessSummary([{ name: "workflow", description: "", capability: "workflow", allowed: false, ask: false, label: "hidden", reason: "not in agent allowlist" }])).toEqual({
      label: "workflow chains blocked",
      detail: "not in agent allowlist",
      tone: "bad",
    });
  });
});

describe("agentAuthorityContractSummary", () => {
  it("summarizes an agent's tool, workflow, config, and data authority contract", () => {
    expect(
      agentAuthorityContractSummary(
        { trust_ceiling: "L2", tool_allow: ["memory"], tool_deny: ["shell"] },
        [
          { name: "memory", description: "", capability: "memory", allowed: true, ask: false, label: "allowed", reason: "" },
          { name: "workflow", description: "", capability: "workflow", allowed: true, ask: true, label: "L2", reason: "" },
          { name: "db", description: "", capability: "data", allowed: false, ask: false, label: "hidden", reason: "" },
        ],
        [{ visible: true }, { visible: false }],
      ),
    ).toEqual({
      label: "managed authority",
      detail: "1 direct · 1 ask · 1 blocked · workflow ask · data lake blocked · config 1/2 · trust L2",
      tone: "good",
    });
    expect(agentAuthorityContractSummary({}, [], [])).toEqual({
      label: "authority unknown",
      detail: "0 explicit rules · config unknown · trust L4",
      tone: "muted",
    });
  });
});

describe("agentAuthorityManifest", () => {
  it("builds a durable authority manifest from tool, config, memory, and backend governance", () => {
    expect(
      agentAuthorityManifest(
        {
          trust_ceiling: "L2",
          tool_allow: ["memory", "workflow"],
          tool_deny: ["shell"],
          memory_scope: "agent/ops",
          noise_policy: { disable_memory_writes: false },
        },
        [
          { name: "memory", description: "", capability: "memory", allowed: true, ask: false, label: "allowed", reason: "" },
          { name: "workflow", description: "", capability: "workflow", allowed: true, ask: true, label: "L2", reason: "" },
          { name: "db", description: "", capability: "data", allowed: false, ask: false, label: "hidden", reason: "" },
        ],
        {
          config_entries: [
            { key: "agent/ops/runtime", visible: true, owned: true },
            { key: "secret:value", visible: false },
          ],
          governance: {
            risk: "restricted",
            authority_boundary: "agent identity boundary",
            execution_boundary: "wakes and workflows invoke through ops policy",
            tool_policy: "1 direct · 1 ask · 1 blocked · trust L2",
            memory_policy: "agent/ops · writes enabled",
            memory_writes: "writes enabled",
          },
        },
        "ops",
      ),
    ).toEqual({
      label: "managed authority manifest",
      tone: "good",
      fields: {
        boundary: "agent identity boundary",
        tools: "1 direct · 1 ask · 1 blocked · trust L2",
        workflow: "workflow ask",
        data: "data lake blocked",
        config: "1/2 visible · 1 owned · 1 blocked",
        memory: "agent/ops · writes enabled",
        execution: "wakes and workflows invoke through ops policy",
      },
      detail: "agent identity boundary · 1 direct · 1 ask · 1 blocked · trust L2 · workflow ask · data lake blocked · 1/2 visible · 1 owned · 1 blocked · agent/ops · writes enabled · wakes and workflows invoke through ops policy",
    });
  });
});

describe("agentRepairOperationsSummary", () => {
  it("summarizes retry, doctor, self-repair, latest repair, and cooldown", () => {
    expect(
      agentRepairOperationsSummary(
        {
          retry_policy: { max_attempts: 3 },
          health_policy: { doctor_agent: "guardian-doctor" },
          self_repair: { enabled: true, max_attempts: 2, escalate_to: "lead" },
        },
        {
          cooldown_sec: 90,
          inflight_count: 0,
          latest: {
            phase: "failed",
            mode: "degraded",
            error: "tool denied",
            self_repair_attempt: 2,
            self_repair_max_attempts: 3,
            incident_id: "inc-child-123456789",
            root_incident_id: "inc-root-123456789",
            root_agent: "builder",
            chain_depth: 1,
          },
        },
      ),
    ).toEqual({
      label: "repair failing",
      detail: "up to 3 attempts · backoff none · no delay · retry on error, timeout · doctor guardian-doctor · self-repair on 2x · escalate lead · latest failed · doctor · tool denied · attempt 2/3 · root builder · hop 1 · incident inc-child-12345678 · cooldown 90s",
      tone: "bad",
    });
    expect(agentRepairOperationsSummary({}, null)).toEqual({
      label: "manual repair",
      detail: "single attempt; no run-level retry · no doctor · self-repair off",
      tone: "muted",
    });
  });
});

describe("agentRepairDecisionSummary", () => {
  it("summarizes backend repair contract and next action", () => {
    const decision = agentRepairDecisionSummary({
      contract: {
        retry_attempts: 3,
        retry_backoff: "exponential",
        retry_on: ["error", "timeout"],
        doctor_agent: "guardian-doctor",
        failure_threshold: 2,
        self_repair_enabled: true,
        self_repair_attempts: 2,
        escalate_to: "lead",
        cooldown_sec: 1800,
        authority_boundary: "agent identity owns repair",
      },
      next_action: {
        action: "wait_inflight",
        label: "repair in flight",
        detail: "doctor/self-repair run is already queued",
        tone: "accent",
        fingerprint: "fp-2",
        phase: "queued",
      },
    });
    expect(decision.label).toBe("repair in flight");
    expect(decision.tone).toBe("accent");
    expect(decision.detail).toContain("doctor/self-repair run is already queued");
    expect(decision.detail).toContain("retry 3x exponential");
    expect(decision.detail).toContain("signals error, timeout");
    expect(decision.detail).toContain("doctor guardian-doctor");
    expect(decision.detail).toContain("self-repair 2x");
    expect(decision.detail).toContain("agent identity owns repair");
  });

  it("falls back when repair status is still loading", () => {
    expect(agentRepairDecisionSummary(null)).toEqual({
      label: "decision loading",
      detail: "repair status has not arrived yet",
      tone: "muted",
    });
  });
});

describe("agentHealthContractLedger", () => {
  it("turns retry, doctor, self-repair, and wake guard into an agent health contract", () => {
    const ledger = agentHealthContractLedger(
      {
        enabled: true,
        retry_policy: { max_attempts: 3, retry_on: ["error", "timeout"] },
        health_policy: { doctor_agent: "guardian-doctor", failure_threshold: 2, failure_window: 5 },
        self_repair: { enabled: true, max_attempts: 2, escalate_to: "lead" },
      },
      {
        state: "degraded",
        label: "degraded",
        detail: "2 failures in window",
        doctorAgent: "guardian-doctor",
        selfRepairEnabled: true,
      },
      {
        inflight_count: 1,
        latest: { phase: "queued", reason: "provider timeout" },
      },
      { retryCount: 2, retryText: "2 retries", retryDetail: "last timeout", repairInflight: 1 },
    );
    expect(ledger.map((entry) => `${entry.label}:${entry.value}`)).toEqual([
      "retry:3 attempts",
      "doctor:guardian-doctor",
      "self-repair:armed",
      "wake guard:active",
    ]);
    expect(ledger.find((entry) => entry.label === "doctor")?.detail).toContain("threshold 2");
    expect(ledger.find((entry) => entry.label === "self-repair")).toMatchObject({ tone: "warn" });
    expect(ledger.find((entry) => entry.label === "wake guard")?.detail).toContain("repair loop owns");
  });
});

describe("agentOperationsPassport", () => {
  it("collapses wake, mailbox, repair, authority, and config into one operating verdict", () => {
    expect(agentOperationsPassport(
      { enabled: true },
      { activeRunCount: 0, operationalText: "sleeping", wakeText: "scheduled", wakeDetail: "next hourly" },
      { value: "mailbox armed · DM", detail: "armed DM", tone: "good" },
      { detail: "1 direct · 1 ask · trust L2", level: "scoped" },
      { value: "1 local override · 2/2 visible", detail: "config ok", tone: "good" },
      { label: "repair guarded", detail: "self-repair on 2x", tone: "good" },
    )).toEqual({
      value: "autonomous ready",
      detail: "sleeping · direct callable · scheduled · next hourly · mailbox armed · DM · repair guarded · 1 direct · 1 ask · trust L2 · 1 local override · 2/2 visible",
      tone: "good",
    });
    expect(agentOperationsPassport(
      { enabled: true, direct_callable: false, parent_agent: "lead" },
      { activeRunCount: 0, operationalText: "sleeping" },
      { value: "mailbox blocked", detail: "managed sub-agent", tone: "warn" },
      { detail: "high authority", level: "open" },
      { value: "0 local overrides", detail: "config ok", tone: "muted" },
      { label: "repair exhausted", detail: "attempt 2/2", tone: "bad" },
    )).toMatchObject({
      value: "operator attention",
      tone: "bad",
    });
  });
});

describe("agentEntityContractLedger", () => {
  it("turns an agent into durable identity, wake, authority, resource, and recovery cells", () => {
    const ledger = agentEntityContractLedger(
      "ops",
      {
        enabled: true,
        kind: "subagent",
        direct_callable: false,
        parent_agent: "lead",
        memory_scope: "agent",
        tool_allow: ["db"],
        config_overrides: { AGEZT_MODE: "watch" },
        tasklist: [{ id: "cycle-1", title: "check queue", scope: "cycle" }],
      },
      { activeRunCount: 0, operationalText: "sleeping", wakeText: "mailbox", wakeDetail: "lead only" },
      { value: "mailbox gated", detail: "managed sub-agent", tone: "warn" },
      { detail: "1 direct · trust L2", level: "scoped" },
      { value: "1 local override", detail: "AGEZT_MODE visible", tone: "good" },
      { label: "repair guarded", detail: "self-repair on 2x", tone: "good" },
    );

    expect(ledger.map((entry) => entry.label)).toEqual([
      "identity",
      "wake",
      "authority",
      "ownership",
      "resources",
      "recovery",
    ]);
    expect(ledger.find((entry) => entry.label === "identity")).toMatchObject({
      value: "subagent · managed",
      tone: "accent",
    });
    expect(ledger.find((entry) => entry.label === "ownership")?.detail).toBe("sub-agent wake is routed through lead");
    expect(ledger.find((entry) => entry.label === "resources")?.detail).toContain("memory agent");
    expect(ledger.find((entry) => entry.label === "recovery")?.detail).toContain("1 local override");
  });
});

describe("agentAutonomyRunbook", () => {
  it("spells out trigger, route, mailbox, execution, recovery, and sleep behavior", () => {
    const runbook = agentAutonomyRunbook(
      {
        enabled: true,
        kind: "subagent",
        direct_callable: false,
        parent_agent: "lead",
        retry_policy: { max_attempts: 3 },
        health_policy: { doctor_agent: "guardian-doctor" },
        self_repair: { enabled: true, max_attempts: 2 },
        lifecycle: { mode: "cycle", completed_cycles: 1 },
      },
      { activeRunCount: 1, activePhase: "tool", operationalText: "running", wakeText: "mailbox", wakeDetail: "lead only" },
      { value: "mailbox armed · DM", detail: "1 waiting · wake subjects armed: DM", tone: "good" },
      {
        status: "managed",
        passport: "managed by lead",
        detail: "Direct wake is blocked; delegation is accepted from manager: lead.",
        operatorAllowed: false,
        scheduleAllowed: false,
        channelAllowed: false,
        delegationAllowed: true,
        delegationDetail: "manager: lead",
        tone: "warn",
      },
      { label: "repair guarded", detail: "self-repair on 2x", tone: "good" },
    );

    expect(runbook.map((entry) => entry.label)).toEqual(["trigger", "route", "mailbox", "execution", "recovery", "sleep"]);
    expect(runbook.find((entry) => entry.label === "trigger")).toMatchObject({
      value: "delegation only",
      tone: "warn",
    });
    expect(runbook.find((entry) => entry.label === "route")?.detail).toBe(
      "sub-agent receives wake through lead; direct schedule/operator/channel wake stays blocked",
    );
    expect(runbook.find((entry) => entry.label === "execution")).toMatchObject({
      value: "awake · tool",
      tone: "accent",
    });
    expect(runbook.find((entry) => entry.label === "recovery")?.detail).toContain("doctor guardian-doctor");
    expect(runbook.find((entry) => entry.label === "sleep")).toMatchObject({
      value: "cycle",
      tone: "good",
    });
  });
});

describe("agentRuntimeDoctorLedger", () => {
  it("splits live, retry, repair, and escalation pressure into operational cells", () => {
    const ledger = agentRuntimeDoctorLedger(
      {
        activeRunCount: 1,
        activePhase: "using tool",
        liveDetail: "using tool · model gpt-5",
        retryText: "retry 2",
        retryDetail: "attempt 2/3 · timeout",
        retryTone: "bad",
        escalationText: "esc 1",
        repairIncidentDetail: "incident inc-1",
      },
      { label: "repair failing", detail: "doctor failed", tone: "bad" },
      { value: "self-repair ready", detail: "doctor configured", tone: "good" },
      { openCount: 1, ackedCount: 0, doctorOpenCount: 1, delegatedOpenCount: 0 },
    );
    expect(ledger.map((entry) => entry.label)).toEqual(["live", "retry", "repair", "escalation"]);
    expect(ledger.map((entry) => entry.value)).toEqual(["1 awake · using tool", "retry 2", "repair failing", "1 open"]);
    expect(ledger.find((entry) => entry.label === "repair")?.detail).toContain("incident inc-1");
    expect(ledger.find((entry) => entry.label === "escalation")?.tone).toBe("bad");
  });
});

describe("agentSystemGuardianContract", () => {
  it("only reports durable system guardian noise and authority posture", () => {
    expect(agentSystemGuardianContract({ system: false, slug: "ops" })).toBeNull();
    expect(agentSystemGuardianContract({
      system: true,
      slug: "guardian-health",
      memory_scope: "system/guardian-health",
      max_cost_mc: 50_000_000,
      max_daily_mc: 50_000_000,
      trust_ceiling: "L2",
      tool_deny: ["memory"],
      noise_policy: {
        silent_on_success: true,
        disable_memory_writes: true,
        min_notify_severity: "warning",
        min_notify_interval_sec: 28800,
      },
      status: { wake_schedule_count: 1 },
    })).toEqual({
      value: "quiet guardian",
      detail: "quiet, memory off, notify >= warning, cooldown >=8h, capped, trust <= L2 · 1 wake schedule",
      tone: "good",
    });
    expect(agentSystemGuardianContract({
      kind: "system",
      slug: "guardian-routing",
      max_cost_mc: 250_000_000,
      max_daily_mc: 250_000_000,
      trust_ceiling: "L3",
      noise_policy: { min_notify_severity: "info", min_notify_interval_sec: 60 },
      status: { wake_schedule_count: 3 },
    })).toEqual({
      value: "guardian intervention",
      detail: "success notifications enabled, memory writes enabled, notify below warning, notify cooldown <8h, daily cap too high, run cap too high, trust above L2, memory scope not isolated · 3 wake schedules",
      tone: "bad",
    });
  });
});

describe("agentCapabilityRiskPassport", () => {
  it("summarizes authority risk from tools, config, and governance", () => {
    expect(
      agentCapabilityRiskPassport(
        { trust_ceiling: "L2", tool_allow: ["memory"], tool_deny: ["shell"] },
        [
          { name: "memory", description: "", capability: "memory", allowed: true, ask: false, label: "allowed", reason: "" },
          { name: "workflow", description: "", capability: "workflow", allowed: true, ask: true, label: "L2", reason: "" },
          { name: "shell", description: "", capability: "code.exec", allowed: false, ask: false, label: "hidden", reason: "" },
        ],
        {
          config_entries: [{ key: "agent/ops/runtime", visible: true }, { key: "secret:value", visible: false }],
          governance: { risk: "restricted" },
        },
      ),
    ).toEqual({
      label: "guarded high authority",
      detail: "ask-gated high-impact: workflow · direct memory · ask workflow · config 1/2 · trust L2",
      tone: "warn",
    });
    expect(
      agentCapabilityRiskPassport(
        { trust_ceiling: "L4" },
        [{ name: "shell", description: "", capability: "code.exec", allowed: true, ask: false, label: "allowed", reason: "" }],
        { config_entries: [] },
      ),
    ).toEqual({
      label: "high authority",
      detail: "direct high-impact: shell · direct shell · ask none · config unknown · trust L4",
      tone: "warn",
    });
  });
});

describe("agentAuthorityLedger", () => {
  it("collapses tools, config, memory, workspace, budget, noise, and trust into an agent ledger", () => {
    const ledger = agentAuthorityLedger(
      {
        trust_ceiling: "L2",
        tool_allow: ["memory", "workflow"],
        tool_deny: ["shell"],
        memory_scope: "private",
        workdir: "agents/ops",
        max_cost_mc: 50_000_000,
        max_daily_mc: 100_000_000,
        noise_policy: {
          silent_on_success: true,
          disable_memory_writes: true,
          min_notify_severity: "warning",
          min_notify_interval_sec: 7200,
        },
      },
      [
        { name: "memory", description: "", capability: "memory", allowed: true, ask: false, label: "allowed", reason: "" },
        { name: "workflow", description: "", capability: "workflow", allowed: true, ask: true, label: "L2", reason: "" },
        { name: "shell", description: "", capability: "code.exec", allowed: false, ask: false, label: "hidden", reason: "" },
      ],
      {
        config_entries: [
          { key: "agent/ops/runtime", visible: true, owned: true },
          { key: "secret:value", visible: false },
        ],
        governance: { summary: "tools 1/3 allowed, 1 ask, 1 blocked, config 1/2 visible, trust L2" },
      },
      "ops",
    );

    expect(ledger.map((entry) => entry.label)).toEqual(["tools", "config", "memory", "workspace", "budget", "noise", "trust"]);
    expect(ledger.find((entry) => entry.label === "tools")).toMatchObject({
      value: "ask high-impact 1",
      tone: "good",
    });
    expect(ledger.find((entry) => entry.label === "memory")).toMatchObject({
      value: "writes blocked",
      detail: "scope private · memory writes disabled or tool denied",
      tone: "warn",
    });
    expect(ledger.find((entry) => entry.label === "budget")).toMatchObject({
      value: "$0.0500 / $0.1000",
      tone: "good",
    });
    expect(ledger.find((entry) => entry.label === "trust")).toMatchObject({
      value: "L2",
      detail: "tools 1/3 allowed, 1 ask, 1 blocked, config 1/2 visible, trust L2",
      tone: "good",
    });
  });

  it("flags open uncapped agents as broad authority", () => {
    const ledger = agentAuthorityLedger(
      { trust_ceiling: "L4", tool_allow: [], tool_deny: [] },
      [
        { name: "memory", description: "", capability: "memory", allowed: true, ask: false, label: "allowed", reason: "" },
        { name: "shell", description: "", capability: "code.exec", allowed: true, ask: false, label: "allowed", reason: "" },
      ],
      { config_entries: [] },
      "ops",
    );

    expect(ledger.find((entry) => entry.label === "tools")).toMatchObject({
      value: "direct high-impact 1",
      tone: "warn",
    });
    expect(ledger.find((entry) => entry.label === "workspace")).toMatchObject({
      value: "shared workspace",
      tone: "warn",
    });
    expect(ledger.find((entry) => entry.label === "budget")).toMatchObject({
      value: "uncapped",
      tone: "warn",
    });
  });
});

describe("agentConfigAuthorityContract", () => {
  it("summarizes local overrides, visible/owned config, and hidden config risk", () => {
    expect(agentConfigAuthorityContract({ config_overrides: { AGEZT_MODE: "watch" } }, null)).toEqual({
      value: "1 local override · config loading",
      detail: "config center access has not loaded yet",
      tone: "muted",
    });
    expect(agentConfigAuthorityContract(
      { config_overrides: { AGEZT_MODE: "watch", AGEZT_PROVIDER: "openai" } },
      {
        config_entries: [
          { key: "agent/ops/runtime", rating: "internal", visible: true, owned: true, source: "config_allowed" },
          { key: "public:value", rating: "internal", visible: true, source: "config_global" },
          { key: "secret:value", rating: "secret", visible: false, source: "config_excluded" },
        ],
      },
    )).toEqual({
      value: "2 local overrides · 2/3 visible · 1 owned · 1 blocked",
      detail: "agent-local runtime config is set on the identity · 2 visible config center entries · 1 owned by this agent · 1 allowlisted · 1 excluded · 1 hidden secret",
      tone: "warn",
    });
    expect(agentConfigAuthorityContract({}, { config_entries: [] })).toEqual({
      value: "0 local overrides · 0 center entries",
      detail: "no agent-local runtime overrides · no config center entries reported · no owned entries",
      tone: "muted",
    });
  });
});

describe("agent mailbox helpers", () => {
  it("derives stable mailbox wake subjects from the agent slug", () => {
    expect(agentMailboxSubjects("ops")).toEqual([
      { kind: "dm", label: "DM", subject: "board.dm.ops" },
      { kind: "help", label: "Help", subject: "board.help.ops" },
      { kind: "broadcast", label: "Broadcast", subject: "board.broadcast" },
    ]);
    expect(
      mailboxSubjectBinding(
        [
          {
            id: "so-1",
            name: "ops mailbox",
            enabled: true,
            triggers: [{ type: "event", subject: "board.dm.ops" }],
          },
        ],
        "board.dm.ops",
      )?.name,
    ).toBe("ops mailbox");
    expect(agentMailboxWakeContract("ops", [
      { id: "dm", name: "ops dm", enabled: true, triggers: [{ type: "event", subject: "board.dm.ops" }] },
      { id: "help", name: "ops help", enabled: false, triggers: [{ type: "event", subject: "board.help.ops" }] },
    ], { enabled: true })).toEqual({
      value: "mailbox armed · DM",
      detail: "armed DM · paused Help · idle Broadcast · channel wake allowed",
      tone: "good",
    });
    expect(agentMailboxWakeContract("worker", [], { enabled: true, direct_callable: false, parent_agent: "lead" })).toEqual({
      value: "mailbox blocked",
      detail: "idle DM, Help, Broadcast · blocked: managed sub-agent; arm mailbox wake on lead",
      tone: "warn",
    });
    expect(agentMailboxWakeContract("ops", [], { enabled: false })).toEqual({
      value: "mailbox blocked",
      detail: "idle DM, Help, Broadcast · blocked: resume this agent before arming mailbox wake",
      tone: "bad",
    });
  });

  it("explains why unmanaged mailbox wake cannot be armed directly", () => {
    expect(mailboxWakeArmIssue({})).toBe("");
    expect(mailboxWakeArmIssue({ enabled: false })).toBe("resume this agent before arming mailbox wake");
    expect(mailboxWakeArmIssue({ retired: true })).toBe("revive this agent before arming mailbox wake");
    expect(mailboxWakeArmIssue({ direct_callable: false, parent_agent: "lead" })).toBe(
      "managed sub-agent; arm mailbox wake on lead",
    );
    expect(mailboxWakeArmIssue({ kind: "subagent", parent_agent: "lead" })).toBe(
      "managed sub-agent; arm mailbox wake on lead",
    );
    expect(mailboxWakeArmIssue({ direct_callable: true }, { channel_allowed: false, manager: "lead" })).toBe(
      "channel wake blocked; arm mailbox wake on lead",
    );
    expect(mailboxWakeArmIssue({ direct_callable: true }, { channel_allowed: false, reason: "channel policy denied" })).toBe(
      "channel policy denied",
    );
  });

  it("explains when an operator wake is not allowed directly", () => {
    expect(operatorWakeIssue({})).toBe("");
    expect(operatorWakeIssue({ enabled: false })).toBe("resume this agent before waking it");
    expect(operatorWakeIssue({ retired: true })).toBe("revive this agent before waking it");
    expect(operatorWakeIssue({ direct_callable: false, parent_agent: "lead" })).toBe(
      "managed sub-agent; wake lead instead",
    );
    expect(operatorWakeIssue({ kind: "subagent", parent_agent: "lead" })).toBe(
      "managed sub-agent; wake lead instead",
    );
  });

  it("keeps sent, directed, broadcast, and acknowledged messages sorted newest first", () => {
    const got = agentBoardMessages(
      [
        { id: "old", from: "ops", to: "researcher", text: "a", ts_unix_ms: 1 },
        { id: "mine", from: "researcher", to: "ops", text: "b", ts_unix_ms: 3 },
        { id: "all", from: "ops", to: "*", text: "c", ts_unix_ms: 2 },
        { id: "seen", from: "ops", to: "writer", text: "d", acked_by: ["researcher"], ts_unix_ms: 4 },
        { id: "other", from: "ops", to: "writer", text: "e", ts_unix_ms: 5 },
      ],
      "researcher",
    );
    expect(got.map((m) => m.id)).toEqual(["seen", "mine", "all", "old"]);
  });

  it("treats replied or acked messages as no longer waiting", () => {
    const messages = [
      { id: "q1", from: "ops", to: "researcher", text: "still waiting" },
      { id: "q2", from: "ops", to: "researcher", text: "answered" },
      { id: "r2", from: "researcher", to: "ops", reply_to: "q2", text: "done" },
      { id: "b1", from: "ops", to: "*", text: "acked", acked_by: ["Researcher"] },
      { id: "b2", from: "ops", to: "*", text: "broadcast waiting" },
      { id: "self", from: "researcher", to: "*", text: "not waiting for self" },
    ];

    expect(messageAckedBy(messages[3], "researcher")).toBe(true);
    expect(messageAckedByLabel(messages[3])).toBe("Researcher");
    expect(waitingForAgent(messages, "researcher").map((m) => m.id)).toEqual(["q1", "b2"]);
  });

  it("summarizes direct, broadcast, help, replied, and stale inbox priority buckets", () => {
    const now = Date.UTC(2026, 0, 2, 12);
    expect(agentInboxPrioritySummary([
      { id: "direct", from: "lead", to: "researcher", text: "direct", ts_unix_ms: now - 2 * 60 * 60 * 1000 },
      { id: "broadcast", from: "ops", to: "*", text: "broadcast", ts_unix_ms: now - 25 * 60 * 60 * 1000 },
      { id: "help", from: "ops", to: "researcher", text: "help", help: true, ts_unix_ms: now - 10 * 60 * 1000 },
      { id: "answered", from: "ops", to: "researcher", text: "answered", ts_unix_ms: now - 5 * 60 * 1000 },
      { id: "reply", from: "researcher", to: "ops", reply_to: "answered", text: "done", ts_unix_ms: now - 4 * 60 * 1000 },
      { id: "seen", from: "ops", to: "researcher", text: "seen", acked_by: ["researcher"], ts_unix_ms: now - 3 * 60 * 1000 },
    ], "researcher", now)).toEqual({
      direct: 2,
      broadcast: 1,
      help: 1,
      replied: 1,
      stale: 1,
      waiting: 3,
      label: "3 waiting",
      detail: "2 direct · 1 broadcast · 1 help · 1 replied · 1 stale",
      tone: "warn",
    });
  });

  it("summarizes mailbox backlog and armed wake subjects for the identity passport", () => {
    expect(agentMailboxPassport("researcher", [
      { id: "q1", from: "ops", to: "researcher", text: "still waiting" },
      { id: "b1", from: "ops", to: "*", text: "broadcast waiting" },
      { id: "sent", from: "researcher", to: "ops", text: "sent" },
    ], [
      { id: "so-1", name: "researcher dm", enabled: true, triggers: [{ type: "event", subject: "board.dm.researcher" }] },
      { id: "so-2", name: "researcher broadcast", enabled: true, triggers: [{ type: "event", subject: "board.broadcast" }] },
    ])).toEqual({
      value: "inbox 2 waiting",
      detail: "2 waiting · 2 received · 1 sent · wake subjects armed: DM, Broadcast",
      tone: "warn",
    });
    expect(agentMailboxPassport("idle", [], [])).toEqual({
      value: "no mailbox traffic",
      detail: "0 waiting · 0 received · 0 sent · no mailbox wake subjects armed",
      tone: "muted",
    });
  });
});

describe("AgentDetail lifecycle intervention", () => {
  it("computes explicit removal cleanup and retention impact", () => {
    const plan = agentRemovalImpactPlan(
      {
        standing_orders: ["watch"],
        schedules: ["digest"],
        memories: ["private"],
        authored_shared_memories: ["shared"],
        skills: ["skill"],
        configs: ["config"],
        workflow_refs: ["flow/handoff"],
        mailbox_messages: ["dm m1", "broadcast m2"],
        subagents: ["worker"],
        subagent_schedules: ["worker schedule"],
        subagent_skills: ["worker skill"],
        subagent_workflow_refs: ["worker: worker-flow/delegate"],
        subagent_mailbox_messages: ["worker dm"],
      },
      {
        standing: true,
        schedules: false,
        memory: true,
        authored_memory: false,
        skills: true,
        config: false,
        subagents: true,
      },
    );
    expect(plan.clean).toEqual(["1 standing", "1 private memory", "1 skill", "1 sub-agent", "1 sub-agent skill"]);
    expect(plan.keep).toEqual(["1 schedule", "1 authored shared memory", "1 config", "shared config access refs", "1 workflow reference", "2 mailbox/audit messages", "1 sub-agent schedule", "1 sub-agent workflow reference", "1 sub-agent mailbox/audit messages"]);
    expect(plan.blockedBySubagents).toBe(false);
    expect(agentRemovalRiskLabel(plan)).toBe("retains dependent resources after identity deletion");
    expect(agentLifecycleInterventionSummary({ retired: false, system: false }, plan)).toEqual({
      disposition: "active identity",
      retire: "retire moves the identity to the graveyard and pauses its direct standing/schedule wakes while preserving audit, soul, memory, skills, config, mailbox, and workspace",
      remove: "remove deletes the identity, cleans 5 groups, and leaves 1 schedule, 1 authored shared memory, 1 config, shared config access refs, 1 workflow reference, 2 mailbox/audit messages, 1 sub-agent schedule, 1 sub-agent workflow reference, 1 sub-agent mailbox/audit messages",
      tone: "warn",
    });
    expect(
      agentRemovalRiskLabel(agentRemovalImpactPlan(
        { subagents: ["worker"] },
        { standing: false, schedules: false, memory: false, authored_memory: false, skills: false, config: false, subagents: false },
      )),
    ).toBe("blocked: dependent sub-agents would be orphaned");
    expect(
      agentRemovalRiskLabel(agentRemovalImpactPlan(
        { schedules: ["digest"] },
        { standing: false, schedules: true, memory: false, authored_memory: false, skills: false, config: false, subagents: false },
      )),
    ).toBe("cleans selected owned resources with identity deletion");
    expect(
      agentRemovalRiskLabel(agentRemovalImpactPlan(
        {},
        { standing: false, schedules: false, memory: false, authored_memory: false, skills: false, config: false, subagents: false },
      )),
    ).toBe("identity-only removal");
    expect(
      agentLifecycleInterventionSummary(
        { retired: false, system: true },
        agentRemovalImpactPlan({}, { standing: false, schedules: false, memory: false, authored_memory: false, skills: false, config: false, subagents: false }),
      ),
    ).toMatchObject({
      disposition: "protected system identity",
      remove: "hard remove is blocked for system identities; pause or retire instead",
      tone: "muted",
    });
  });

  it("builds a lifecycle decision ledger for state, tasks, retire and remove choices", () => {
    const ledger = agentLifecycleDecisionLedger(
      {
        lifecycle: { mode: "cycle", completed_cycles: 1, max_cycles: 3 },
        tasklist: [{ title: "scan", scope: "cycle", status: "doing" }],
      },
      { clean: ["1 schedule", "1 skill"], keep: ["1 mailbox/audit messages"], blockedBySubagents: false },
    );
    expect(ledger.map((entry) => entry.label)).toEqual(["state", "tasks", "retire", "remove"]);
    expect(ledger.map((entry) => entry.value)).toEqual([
      "alive · cycle 1/3",
      "1 cycle / 0 total / 1 doing",
      "graveyard available",
      "2 clean · 1 keep",
    ]);
    expect(agentLifecycleDecisionLedger(
      { system: true },
      { clean: [], keep: [], blockedBySubagents: false },
    ).find((entry) => entry.label === "remove")).toMatchObject({
      value: "blocked",
      detail: "hard remove is blocked for system identities",
      tone: "muted",
    });
    expect(agentLifecycleDecisionLedger(
      {},
      { clean: [], keep: [], blockedBySubagents: true },
    ).find((entry) => entry.label === "remove")).toMatchObject({
      value: "blocked by sub-agents",
      tone: "bad",
    });
  });

  it("summarizes lifecycle action results for retire, revive, and hard removal", () => {
    expect(agentLifecycleActionResultSummary("retire", "ops", { standing_paused: 1, schedules_paused: 2 })).toEqual({
      label: "ops retired",
      detail: "identity moved to graveyard · 1 standing wake paused · 2 schedule wakes paused · soul, memory, skills, config, mailbox, workspace and audit remain inspectable",
      tone: "muted",
    });
    expect(agentLifecycleActionResultSummary("revive", "ops", { standing_paused: 1, schedules_paused: 0 })).toEqual({
      label: "ops revived",
      detail: "identity returned from graveyard in paused service · 1 standing and 0 schedule wake routes remain paused · operator must explicitly resume or re-arm wakes",
      tone: "good",
    });
    expect(agentLifecycleActionResultSummary("remove", "ops", {
      removed: true,
      standing_removed: 1,
      schedules_removed: 1,
      memories_forgotten: 1,
      skills_archived: 1,
      configs_deleted: 1,
      configs_access_pruned: 2,
      workspaces_deleted: 1,
      subagents_retired: 2,
      subagents_retired_slugs: ["worker", "scout"],
      mailbox_messages_retained: 3,
      workflow_refs_retained: 1,
      subagent_workflow_refs_retained: 1,
    })).toEqual({
      label: "ops removed",
      detail: "identity profile deleted · cleaned 1 standing, 1 schedule, 1 private memory, 1 skill, 1 config, 2 shared config access refs, 1 workspace · retired 2 dependent sub-agents (worker, scout) · retained 3 mailbox/audit messages, 1 workflow refs, 1 sub-agent workflow refs",
      tone: "warn",
    });
  });

  it("can wake a directly callable agent from the identity header", async () => {
    postAction.mockResolvedValueOnce({ accepted: true, agent: "ops", correlation_id: "wake-1" });
    render(
      withUI(
        <AgentDetail
          slug="ops"
          profile={{
            id: "01",
            slug: "ops",
            enabled: true,
            soul: "Operate.",
            model: "@ops-chain",
            task_type: "operations",
            trust_ceiling: "L2",
            tool_allow: ["memory", "shell", "db"],
            tool_deny: ["notify"],
            noise_policy: {
              silent_on_success: true,
              disable_memory_writes: true,
              min_notify_severity: "warning",
              min_notify_interval_sec: 28800,
            },
            config_overrides: { AGEZT_MODE: "watch" },
            status: {
              wake_schedule_count: 1,
              next_wake_ms: Date.now() + 10 * 60_000,
              next_wake_label: "night watch",
              last_autonomy_runbook: {
                trigger_contract: "operator_schedule_channel",
                route_contract: "self_owned",
                recovery_contract: "self_repair",
                sleep_contract: "persistent",
                phase: "completed",
                correlation_id: "wake-prev",
              },
            },
            tasklist: [
              { id: "task-1", title: "check queue", scope: "cycle", status: "doing" },
              { id: "task-2", title: "ship migration", scope: "total", status: "blocked" },
            ],
          }}
          runs={[]}
          orders={[]}
          triggers={[]}
          state="manual"
          schedules={[{ id: "sch-1", agent: "ops", enabled: true, mode: "interval", interval_sec: 3600 }]}
          onClose={() => {}}
          onManage={() => {}}
        />,
      ),
    );

    expect(await screen.findByText("Agent identity card")).toBeTruthy();
    expect(screen.getAllByText("Presence").length).toBeGreaterThan(0);
    expect(screen.getByText("Daily")).toBeTruthy();
    expect(screen.getByText("Identity & Maintenance")).toBeTruthy();
    expect(screen.getByRole("tablist", { name: "ops detail sections" })).toBeTruthy();
    expect(screen.getByRole("button", { name: /Overview/ }).getAttribute("aria-pressed")).toBe("true");
    expect(screen.getByRole("button", { name: /Soul/ }).getAttribute("aria-pressed")).toBe("false");
    expect(screen.getByText("More actions")).toBeTruthy();
    expect(screen.getByText("Pause wakes")).toBeTruthy();
    expect(screen.getByLabelText("ops command strip")).toBeTruthy();
    expect(screen.getByLabelText("ops control center")).toBeTruthy();
    expect(screen.getByText("Control center")).toBeTruthy();
    expect(screen.getByText("allow 3 · shell, db")).toBeTruthy();
    expect(screen.getByText("writes off")).toBeTruthy();
    expect(screen.getByText("silent success · memory off · notify >= warning · cooldown 28800s")).toBeTruthy();
    expect(screen.getByText("control review")).toBeTruthy();
    expect(screen.getByText(/memory need review/)).toBeTruthy();
    expect(screen.getByText("custom · sleeping")).toBeTruthy();
    expect(screen.getAllByText("1 schedule · fastest 1h").length).toBeGreaterThan(0);
    expect(screen.getAllByText("route").length).toBeGreaterThan(0);
    expect(screen.getAllByText("chain @ops-chain").length).toBeGreaterThan(0);
    expect(screen.getByText("Mailbox wake contract")).toBeTruthy();
    expect(screen.getAllByText("mailbox manual").length).toBeGreaterThan(0);
    expect(screen.getAllByText("idle DM, Help, Broadcast · channel wake allowed").length).toBeGreaterThan(0);
    expect(screen.getByText("Health contract")).toBeTruthy();
    // The Health contract card now shows cell labels with the value/posture in the
    // hover title; the single-attempt retry posture renders in the retry-policy row.
    expect(screen.getAllByText("single attempt; no run-level retry").length).toBeGreaterThan(0);
    expect(screen.getAllByText("manual").length).toBeGreaterThan(0);
    expect(screen.getByText("wake guard")).toBeTruthy();
    expect(screen.getByTitle(/wake guard: eligible for schedule/)).toBeTruthy();
    expect(screen.getByText("Agent entity contract")).toBeTruthy();
    const entityContract = screen.getByLabelText("ops entity contract");
    expect(entityContract.textContent).toContain("identity");
    expect(entityContract.textContent).toContain("agent · direct");
    expect(entityContract.textContent).toContain("operator callable");
    expect(entityContract.textContent).toContain("shared workspace · memory ops");
    expect(screen.getByText("Autonomy runbook")).toBeTruthy();
    const autonomyRunbook = screen.getByLabelText("ops autonomy runbook");
    expect(autonomyRunbook.textContent).toContain("trigger");
    expect(autonomyRunbook.textContent).toContain("3 direct routes");
    expect(autonomyRunbook.textContent).toContain("self-owned");
    expect(autonomyRunbook.textContent).toContain("sleeps between wake events");
    expect(autonomyRunbook.textContent).toContain("last contract completed");
    expect(autonomyRunbook.textContent).toContain("corr wake-prev");
    expect(autonomyRunbook.textContent).toContain("journal self_repair");
    expect(autonomyRunbook.textContent).toContain("persistent");
    expect(screen.getByText("Operations passport")).toBeTruthy();
    expect(screen.getByText("guarded standby")).toBeTruthy();
    const runtimeDoctorLedger = screen.getByLabelText("ops runtime doctor ledger");
    expect(runtimeDoctorLedger).toBeTruthy();
    expect(screen.getByText("Runtime doctor ledger")).toBeTruthy();
    expect(runtimeDoctorLedger.textContent).toContain("sleeping");
    expect(runtimeDoctorLedger.textContent).toContain("manual repair");
    expect(screen.getAllByText("tools 1/3 allowed, 1 ask, 1 blocked, config 2/3 visible, trust L2").length).toBeGreaterThan(0);
    expect(screen.getByText("Config authority")).toBeTruthy();
    expect(screen.getByText("1 local override · 2/3 visible · 1 owned · 1 blocked")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Edit controls" }));
    expect(screen.getByText("Capability control")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: /Overview/ }));
    expect(screen.getAllByText("shared workspace · memory ops · data lake via db · 1 config override").length).toBeGreaterThan(0);
    expect(screen.getAllByText("manual recovery").length).toBeGreaterThan(0);
    await waitFor(() => expect(screen.getAllByText(/next wake/).length).toBeGreaterThan(0));
    fireEvent.click(screen.getByRole("button", { name: /Soul/ }));
    expect(screen.getByText("soul — identity core")).toBeTruthy();
    await waitFor(() => expect(screen.getAllByText(/in 10m/).length).toBeGreaterThan(0));
    expect(screen.getAllByText(/1 cycle \/ 1 total \/ 1 doing \/ 1 blocked/).length).toBeGreaterThan(0);
    expect(screen.getAllByText(/persistent agent · stays alive after runs · 1 cycle \/ 1 total tasks · 1 blocked/).length).toBeGreaterThan(0);
    fireEvent.click(screen.getByRole("button", { name: "Wake ops" }));
    await waitFor(() =>
      expect(postAction).toHaveBeenCalledWith("/api/agents/wake", {
        ref: "ops",
        reason: "manual operator wake",
      }),
    );
    await waitFor(() => expect(screen.getByText(/focused run/)).toBeTruthy());
    expect(getJSON).toHaveBeenCalledWith("/api/journal", {
      correlation_id: "wake-1",
      limit: "500",
    });
  });

  it("shows the system guardian contract on system agent details", async () => {
    render(
      withUI(
        <AgentDetail
          slug="guardian-health"
          profile={{
            id: "01G",
            slug: "guardian-health",
            enabled: true,
            system: true,
            kind: "system",
            memory_scope: "system/guardian-health",
            max_cost_mc: 50_000_000,
            max_daily_mc: 50_000_000,
            trust_ceiling: "L2",
            tool_deny: ["memory"],
            noise_policy: {
              silent_on_success: true,
              disable_memory_writes: true,
              min_notify_severity: "warning",
              min_notify_interval_sec: 28800,
            },
            status: { wake_schedule_count: 1 },
          }}
          runs={[]}
          orders={[]}
          triggers={[]}
          state="manual"
          schedules={[]}
          onClose={() => {}}
          onManage={() => {}}
        />,
      ),
    );

    expect(await screen.findByText("System guardian contract")).toBeTruthy();
    expect(screen.getByText("quiet guardian")).toBeTruthy();
    expect(screen.getByText("quiet, memory off, notify >= warning, cooldown >=8h, capped, trust <= L2 · 1 wake schedule")).toBeTruthy();
  });

  it("can quiet a noisy system guardian from the detail contract", async () => {
    const onChanged = vi.fn();
    render(
      withUI(
        <AgentDetail
          slug="guardian-routing"
          profile={{
            id: "02G",
            slug: "guardian-routing",
            enabled: true,
            system: true,
            kind: "system",
            memory_scope: "shared",
            max_cost_mc: 250_000_000,
            max_daily_mc: 250_000_000,
            trust_ceiling: "L3",
            tool_allow: ["memory"],
            tool_deny: ["shell"],
            noise_policy: { min_notify_severity: "info", min_notify_interval_sec: 60 },
            config_overrides: { AGEZT_MODE: "watch" },
            status: { wake_schedule_count: 2 },
          }}
          runs={[]}
          orders={[]}
          triggers={[]}
          state="manual"
          schedules={[]}
          onClose={() => {}}
          onManage={() => {}}
          onChanged={onChanged}
        />,
      ),
    );

    expect(await screen.findByText("guardian intervention")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Quiet guardian" }));

    await waitFor(() =>
      expect(postJSON).toHaveBeenCalledWith("/api/agents/capabilities", {
        ref: "guardian-routing",
        memory_scope: "system/guardian-routing",
        max_cost_mc: 50_000_000,
        max_daily_mc: 50_000_000,
        trust_ceiling: "L2",
        tool_allow: [],
        tool_deny: ["shell", "memory"],
        noise_policy: {
          silent_on_success: true,
          disable_memory_writes: true,
          min_notify_severity: "warning",
          min_notify_interval_sec: 28800,
        },
        config_overrides: { AGEZT_MODE: "watch" },
      }),
    );
    expect(onChanged).toHaveBeenCalled();
  });

  it("retires and revives an identity from the agent header", async () => {
    const onChanged = vi.fn();
    const { rerender } = render(
      withUI(
        <AgentDetail
          slug="ops"
          profile={{ id: "01", slug: "ops", enabled: true, soul: "Operate." }}
          runs={[]}
          orders={[]}
          triggers={[]}
          state="manual"
          schedules={[]}
          onClose={() => {}}
          onManage={() => {}}
          onChanged={onChanged}
        />,
      ),
    );

    fireEvent.click(await screen.findByRole("button", { name: "Retire ops" }));
    await waitFor(() =>
      expect(postAction).toHaveBeenCalledWith("/api/agents/retire", {
        ref: "ops",
        reason: "operator retired from agent identity header",
      }),
    );
    expect(onChanged).toHaveBeenCalled();

    postAction.mockClear();
    rerender(
      withUI(
        <AgentDetail
          slug="ops"
          profile={{ id: "01", slug: "ops", enabled: false, retired: true, soul: "Operate." }}
          runs={[]}
          orders={[]}
          triggers={[]}
          state="manual"
          schedules={[]}
          onClose={() => {}}
          onManage={() => {}}
          onChanged={onChanged}
        />,
      ),
    );

    fireEvent.click(await screen.findByRole("button", { name: "Revive ops" }));
    await waitFor(() =>
      expect(postAction).toHaveBeenCalledWith("/api/agents/revive", {
        ref: "ops",
      }),
    );
  });

  it("keeps lifecycle removal impact reachable from the agent header", async () => {
    render(
      withUI(
        <AgentDetail
          slug="ops"
          profile={{ id: "01", slug: "ops", enabled: true, soul: "Operate." }}
          runs={[]}
          orders={[]}
          triggers={[]}
          state="manual"
          schedules={[]}
          onClose={() => {}}
          onManage={() => {}}
        />,
      ),
    );

    fireEvent.click(await screen.findByRole("button", { name: "Lifecycle ops" }));
    await waitFor(() => expect(screen.getByText("Lifecycle intervention")).toBeTruthy());
    expect(screen.getByLabelText("ops lifecycle ledger")).toBeTruthy();
    expect(screen.getByText("Lifecycle ledger")).toBeTruthy();
    expect(screen.getByText("alive · persistent")).toBeTruthy();
    expect(screen.getByText("graveyard available")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Manage" }));
    await waitFor(() => expect(screen.getByText("Remove identity and cleanup")).toBeTruthy());
    expect(screen.getByText("active identity")).toBeTruthy();
    expect(screen.getByText(/retire moves the identity to the graveyard/)).toBeTruthy();
    expect(screen.getByText(/remove deletes the identity, cleans/)).toBeTruthy();
    expect(screen.getByRole("button", { name: "Clean all" })).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Keep all" }));
    await waitFor(() => expect(screen.getByText("blocked: dependent sub-agents would be orphaned")).toBeTruthy());
    expect(screen.getByText(/remove blocked until dependent sub-agents are included/)).toBeTruthy();
    expect(screen.getByText("night watch")).toBeTruthy();
    expect(screen.getByText("ops-worker [parent]")).toBeTruthy();
  });

  it("explains that system identities can be retired but not hard removed", async () => {
    render(
      withUI(
        <AgentDetail
          slug="guardian-health"
          profile={{
            id: "sys-1",
            slug: "guardian-health",
            enabled: true,
            system: true,
            kind: "system",
            soul: "Guard health.",
          }}
          runs={[]}
          orders={[]}
          triggers={[]}
          state="manual"
          schedules={[]}
          onClose={() => {}}
          onManage={() => {}}
        />,
      ),
    );

    fireEvent.click(await screen.findByRole("button", { name: "Lifecycle guardian-health" }));
    fireEvent.click(screen.getByRole("button", { name: "Manage" }));
    expect(await screen.findByText("protected system identity")).toBeTruthy();
    expect(screen.getByText("hard remove is blocked for system identities; pause or retire instead")).toBeTruthy();
    expect(await screen.findByText("System identity protection")).toBeTruthy();
    expect(screen.getByText(/System agents cannot be permanently removed/)).toBeTruthy();
    expect(screen.queryByText("Remove identity and cleanup")).toBeNull();
    expect(screen.getByRole("button", { name: "Retire guardian-health" })).toBeTruthy();
  });

  it("does not wake a managed sub-agent directly from the identity header", async () => {
    render(
      withUI(
        <AgentDetail
          slug="worker"
          profile={{
            id: "01",
            slug: "worker",
            enabled: true,
            soul: "Work.",
            parent_agent: "lead",
            direct_callable: false,
          }}
          runs={[]}
          orders={[]}
          triggers={[]}
          state="manual"
          schedules={[]}
          onClose={() => {}}
          onManage={() => {}}
        />,
      ),
    );

    const wake = screen.getByRole("button", { name: "Wake worker" }) as HTMLButtonElement;
    expect(wake.disabled).toBe(true);
    expect(wake.title).toBe("managed sub-agent; wake lead instead");
    expect(screen.getByText("managed sub-agent; wake lead instead")).toBeTruthy();
    expect((await screen.findAllByText("managed by lead")).length).toBeGreaterThanOrEqual(1);
    fireEvent.click(screen.getByRole("button", { name: "Open wake owner lead" }));
    expect(location.hash).toBe("#agent/lead");
    fireEvent.click(wake);
    expect(postAction).not.toHaveBeenCalledWith("/api/agents/wake", expect.anything());
  });

  it("surfaces operational state and last activity in the identity page", async () => {
    render(
      withUI(
        <AgentDetail
          slug="ops"
          profile={{
            id: "01",
            slug: "ops",
            enabled: true,
            soul: "Operate.",
            parent_agent: "lead",
            direct_callable: false,
            workdir: "agents/ops",
            memory_scope: "private",
            tool_allow: ["db", "memory"],
            config_overrides: { AGEZT_MAX_ITER: "4" },
            lifecycle: { mode: "cycle", completed_cycles: 1, max_cycles: 3 },
            retry_policy: { max_attempts: 3 },
            health_policy: { doctor_agent: "guardian-doctor" },
            self_repair: { enabled: true, max_attempts: 2, escalate_to: "lead" },
            status: {
              active_run_count: 1,
              active_phase: "using tool",
              active_intent: "check disks",
              active_detail: "shell",
              active_tool: "shell.exec",
              active_model: "gpt-5",
              active_correlation_id: "corr-1",
              active_wake_source: "schedule",
              active_schedule_id: "sched-ops",
              operational_state: "running",
              operational_label: "using tool",
              last_activity_ms: Date.now() - 60_000,
              last_activity_summary: "started a run: check disks",
            },
          }}
          runs={[]}
          orders={[]}
          triggers={[]}
          state="manual"
          schedules={[]}
          onClose={() => {}}
          onManage={() => {}}
        />,
      ),
    );

    expect(screen.getAllByText("using tool").length).toBeGreaterThan(0);
    expect(screen.getByText("Now")).toBeTruthy();
    expect(screen.getAllByText(/check disks/).length).toBeGreaterThan(0);
    expect(screen.getAllByText(/source: schedule/).length).toBeGreaterThan(0);
    expect(screen.getAllByText(/schedule: sched-ops/).length).toBeGreaterThan(0);
    expect(screen.getAllByText("shell.exec").length).toBeGreaterThan(0);
    expect(screen.getAllByText("gpt-5").length).toBeGreaterThan(0);
    expect(screen.getAllByText(/corr-1/).length).toBeGreaterThan(0);
    // The redesign surfaces the live run via the "Now" panel on the overview; the
    // last-activity summary moved to the Soul tab's last-activity row (asserted below).
    expect(screen.getAllByText(/using tool · using tool · check disks · shell/).length).toBeGreaterThan(0);
    // The wake source moved into the Status Dashboard: a "Wake source" cell showing "none".
    expect(screen.getByText("Wake source")).toBeTruthy();
    expect(screen.getAllByText("none").length).toBeGreaterThan(0);
    expect(screen.getAllByText(/cycle 1\/3/).length).toBeGreaterThan(0);
    expect(screen.getAllByText(/1\/3 cycles complete; retires at max cycles/).length).toBeGreaterThan(0);
    expect(screen.getAllByText("managed by lead").length).toBeGreaterThan(0);
    expect(screen.getByText("Agent identity card")).toBeTruthy();
    expect(screen.getByText("subagent · running")).toBeTruthy();
    expect(screen.getByText("Identity passport")).toBeTruthy();
    expect(screen.getByText("task contract")).toBeTruthy();
    expect(screen.getByText("config access")).toBeTruthy();
    expect(screen.getByText("resilience")).toBeTruthy();
    expect(screen.getAllByText("manager repair").length).toBeGreaterThan(0);
    expect(screen.getAllByText("request repair through lead").length).toBeGreaterThan(0);
    expect(screen.getByText("Run retry policy")).toBeTruthy();
    expect(screen.getAllByText("up to 3 attempts · backoff none · no delay · retry on error, timeout").length).toBeGreaterThan(0);
    expect(screen.getByText("Resource passport")).toBeTruthy();
    expect(screen.getAllByText("workspace agents/ops · memory private · data lake via db · 1 config override").length).toBeGreaterThan(0);
    await waitFor(() => expect(screen.getByText("2/3 visible · 1 owned · 1 blocked")).toBeTruthy());
    // The "Now" panel's Inspect (scoped by its title) opens the focused run on the Activity tab.
    fireEvent.click(screen.getByTitle("Inspect the active run in this agent"));
    expect(screen.getByText(/focused run/)).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: /Overview/ }));
    // The Status Dashboard's active-run card lets the operator inspect the run inline,
    // which loads the run journal for the active correlation id.
    expect(screen.getAllByText(/Active run/).length).toBeGreaterThan(0);
    expect(screen.getAllByText(/corr-1/).length).toBeGreaterThan(0);
    const inspectToggles = screen.getAllByRole("button", { name: "Inspect" });
    fireEvent.click(inspectToggles[inspectToggles.length - 1]);
    await waitFor(() =>
      expect(getJSON).toHaveBeenCalledWith("/api/journal", {
        correlation_id: "corr-1",
        limit: "500",
      }),
    );
    expect(screen.getAllByText(/corr-1/).length).toBeGreaterThan(0);

    fireEvent.click(screen.getByRole("button", { name: /Soul/ }));
    expect(screen.getAllByText(/using tool · using tool · check disks · shell/).length).toBeGreaterThan(0);
    expect(screen.getAllByText(/started a run: check disks/).length).toBeGreaterThan(0);
  });

  it("shows one-shot lifecycle retirement behavior in the identity page", () => {
    render(
      withUI(
        <AgentDetail
          slug="janitor"
          profile={{
            id: "02",
            slug: "janitor",
            enabled: true,
            soul: "Clean up one backlog.",
            lifecycle: { mode: "retire_on_complete", retire_on_complete: true },
          }}
          runs={[]}
          orders={[]}
          triggers={[]}
          state="manual"
          schedules={[]}
          onClose={() => {}}
          onManage={() => {}}
        />,
      ),
    );

    expect(screen.getAllByText(/one-shot/).length).toBeGreaterThan(0);
    expect(screen.getAllByText(/retires after the next successful completion/).length).toBeGreaterThan(0);
  });

  it("edits lifecycle contract from the identity page without dropping agent fields", async () => {
    const onChanged = vi.fn();
    render(
      withUI(
        <AgentDetail
          slug="ops"
          profile={{
            id: "01",
            slug: "ops",
            name: "Ops",
            enabled: true,
            soul: "Operate.",
            instructions: ["stay quiet"],
            model: "gpt-5",
            fallbacks: ["gpt-4.1"],
            task_type: "ops",
            direct_callable: false,
            parent_agent: "lead",
            lifecycle: { mode: "persistent", completed_cycles: 1 },
            tasklist: [{ id: "task-1", title: "check queue", scope: "cycle", status: "todo" }],
            retry_policy: { max_attempts: 3 },
            health_policy: { doctor_agent: "guardian-doctor" },
            self_repair: { enabled: true, max_attempts: 2, escalate_to: "lead" },
            noise_policy: { silent_on_success: true, disable_memory_writes: true, min_notify_severity: "warning", min_notify_interval_sec: 3600 },
            tool_allow: ["memory"],
            tool_deny: ["notify"],
            trust_ceiling: "L2",
            config_overrides: { AGEZT_MODE: "quiet" },
          }}
          runs={[]}
          orders={[]}
          triggers={[]}
          state="manual"
          schedules={[]}
          onClose={() => {}}
          onManage={() => {}}
          onChanged={onChanged}
        />,
      ),
    );

    fireEvent.click(screen.getByRole("button", { name: /Soul/ }));
    fireEvent.change(screen.getByLabelText("Agent lifecycle mode"), { target: { value: "cycle" } });
    fireEvent.change(screen.getByLabelText("Agent max cycles"), { target: { value: "4" } });
    fireEvent.click(screen.getByRole("button", { name: /Save lifecycle/ }));

    await waitFor(() =>
      expect(postJSON).toHaveBeenCalledWith("/api/agents/edit", {
        ref: "ops",
        profile: expect.objectContaining({
          name: "Ops",
          soul: "Operate.",
          instructions: ["stay quiet"],
          model: "gpt-5",
          fallbacks: ["gpt-4.1"],
          task_type: "ops",
          direct_callable: false,
          parent_agent: "lead",
          lifecycle: { mode: "cycle", retire_on_complete: false, max_cycles: 4, completed_cycles: 1 },
          tasklist: [{ id: "task-1", title: "check queue", scope: "cycle", status: "todo" }],
          retry_policy: { max_attempts: 3 },
          health_policy: { doctor_agent: "guardian-doctor" },
          self_repair: { enabled: true, max_attempts: 2, escalate_to: "lead" },
          noise_policy: { silent_on_success: true, disable_memory_writes: true, min_notify_severity: "warning", min_notify_interval_sec: 3600 },
          tool_allow: ["memory"],
          tool_deny: ["notify"],
          trust_ceiling: "L2",
          config_overrides: { AGEZT_MODE: "quiet" },
        }),
      }),
    );
    expect(onChanged).toHaveBeenCalled();
  });

  it("summarizes the agent capability passport on the overview", async () => {
    getJSON.mockImplementation((path: string, params?: { ref?: string }) => {
      if (path === "/api/agents/impact") return Promise.resolve({ standing_orders: [], schedules: [], memories: [], authored_shared_memories: [], skills: [], configs: [], subagents: [] });
      if (path === "/api/memory") return Promise.resolve({ records: [] });
      if (path === "/api/skills") return Promise.resolve({ skills: [{ id: "sk-ops", name: "Ops Probe", agent: "ops", triggers: ["manual", "schedule"] }] });
      if (path === "/api/policy_log") return Promise.resolve({ decisions: [] });
      if (path === "/api/tool_log") return Promise.resolve({ invocations: [] });
      if (path === "/api/policy") return Promise.resolve({});
      if (path === "/api/edict_show") return Promise.resolve({ levels: { "code.exec": "L0", "memory": "L4", "net.fetch": "L2" } });
      if (path === "/api/tools_catalog")
        return Promise.resolve({
          tools: [
            { name: "memory", description: "remember", capability: "memory" },
            { name: "shell", description: "run commands", capability: "code.exec" },
            { name: "fetch", description: "fetch urls", capability: "net.fetch" },
          ],
        });
      if (path === "/api/agents/permissions" && params?.ref === "ops")
        return Promise.resolve({
          wake_access: {
            status: "direct",
            reason: "directly callable",
            direct_callable: true,
            direct_allowed: true,
            schedule_allowed: true,
            channel_allowed: true,
            operator_allowed: true,
            delegation_allowed: true,
            delegation_scope: "any",
          },
          permissions: [
            { name: "memory", capability: "memory", allowed: true, ask: false, status: "allowed", source: "edict", reason: "capability set to L4 (allow)" },
            { name: "shell", capability: "code.exec", allowed: false, ask: false, status: "hidden", source: "agent_allow", reason: "not in agent tool allowlist" },
            { name: "fetch", capability: "net.fetch", allowed: true, ask: true, status: "L2", source: "edict", reason: "capability set to L2 (ask)" },
          ],
          config_entries: [
            { key: "public:value", rating: "internal", visible: true, source: "config_global", reason: "visible to all eligible agents" },
            { key: "agent/ops/runtime", rating: "internal", visible: true, owned: true, source: "config_allowed", reason: "agent is in config allowed_agents", allowed_agents: ["ops"] },
            { key: "secret:value", rating: "secret", visible: false, source: "config_excluded", reason: "agent is in config excluded_agents", excluded_agents: ["ops"] },
          ],
          governance: {
            summary: "tools 1/3 allowed, 1 ask, 1 blocked, config 2/3 visible, trust L2",
            risk: "restricted",
            trust_ceiling: "L2",
          },
        });
      return Promise.resolve({});
    });

    render(
      withUI(
        <AgentDetail
          slug="ops"
          profile={{
            id: "01",
            slug: "ops",
            enabled: true,
            soul: "Operate.",
            model: "gpt-5",
            task_type: "ops",
            fallbacks: ["gpt-4.1"],
            trust_ceiling: "L2",
            tool_allow: ["memory", "fetch"],
            tool_deny: ["shell"],
            system: true,
            status: { wake_schedule_count: 2 },
            noise_policy: { disable_memory_writes: true, min_notify_severity: "warning" },
            config_overrides: { AGEZT_MAX_ITER: "4" },
          }}
          runs={[]}
          orders={[]}
          triggers={[]}
          state="manual"
          schedules={[{ id: "sch-fast", agent: "ops", enabled: true, mode: "interval", interval_sec: 3600, cadence: "every 1h" }]}
          onClose={() => {}}
          onManage={() => {}}
        />,
      ),
    );

    await waitFor(() =>
      expect(screen.getByText("Capability passport")).toBeTruthy(),
    );
    expect(screen.getAllByText("high-impact ask-gated: fetch · 1 allowed, 1 ask-gated, 1 blocked or hidden out of 3 tools").length).toBeGreaterThan(0);
    expect(screen.getAllByText("tools 1/3 allowed, 1 ask, 1 blocked, config 2/3 visible, trust L2").length).toBeGreaterThan(0);
    expect(screen.getByText("trust L2")).toBeTruthy();
    expect(screen.getByText("allow 2")).toBeTruthy();
    expect(screen.getByText("deny 1")).toBeTruthy();
    expect(screen.getByText("memory writes off")).toBeTruthy();
    expect(screen.getByText("notify >= warning")).toBeTruthy();
    expect(screen.getByText("1 config override")).toBeTruthy();
    expect(screen.getByText("model gpt-5 · task ops · 1 fallback")).toBeTruthy();
    expect(screen.getByText("1 private skill · 2 triggers")).toBeTruthy();
    expect(screen.getByText("noise review: success notifications enabled, notify cooldown <8h, no daily cap, no run cap, memory scope not isolated · 2 scheduled wakes")).toBeTruthy();
    expect(screen.getAllByText("schedule pressure: 1/1 frequent · fastest 1h").length).toBeGreaterThan(0);
    fireEvent.click(screen.getByLabelText("Pause frequent schedules for ops"));
    fireEvent.click(await screen.findByRole("button", { name: "Pause schedules" }));
    await waitFor(() =>
      expect(postAction).toHaveBeenCalledWith("/api/schedule/enable", { id: "sch-fast", enabled: "false" }),
    );
    fireEvent.click(screen.getByRole("button", { name: /Soul/ }));
    expect(screen.getByText("model route")).toBeTruthy();
    expect(screen.getAllByText("skills").length).toBeGreaterThan(0);
    expect(screen.getByText("noise budget")).toBeTruthy();
    expect(screen.getByText("schedule pressure")).toBeTruthy();
    expect(screen.getAllByText("model gpt-5 · task ops · 1 fallback").length).toBeGreaterThan(0);
    expect(screen.getAllByText("1 private skill · 2 triggers").length).toBeGreaterThan(0);
  });

  it("shows effective runtime noise guardrails for system agents", async () => {
    render(
      withUI(
        <AgentDetail
          slug="guardian-health"
          profile={{
            id: "01G",
            slug: "guardian-health",
            enabled: true,
            system: true,
            kind: "system",
            soul: "Guard the system.",
            noise_policy: { min_notify_severity: "info", min_notify_interval_sec: 0 },
          }}
          runs={[]}
          orders={[]}
          triggers={[]}
          state="manual"
          schedules={[]}
          onClose={() => {}}
          onManage={() => {}}
        />,
      ),
    );

    await waitFor(() => expect(screen.getAllByText("guardian-health").length).toBeGreaterThan(0));
    fireEvent.click(screen.getByRole("button", { name: /Soul/ }));
    expect(screen.getByText("silent on success · no memory writes · notify >= warning · cooldown 28800s · system enforced")).toBeTruthy();
  });

  it("shows silent-on-success as an effective warning notify floor for custom agents", async () => {
    render(
      withUI(
        <AgentDetail
          slug="quiet-worker"
          profile={{
            id: "01Q",
            slug: "quiet-worker",
            enabled: true,
            soul: "Work quietly.",
            noise_policy: { silent_on_success: true },
          }}
          runs={[]}
          orders={[]}
          triggers={[]}
          state="manual"
          schedules={[]}
          onClose={() => {}}
          onManage={() => {}}
        />,
      ),
    );

    await waitFor(() => expect(screen.getAllByText("quiet-worker").length).toBeGreaterThan(0));
    fireEvent.click(screen.getByRole("button", { name: /Soul/ }));
    expect(screen.getByText("silent on success · notify >= warning")).toBeTruthy();
  });

  it("shows mailbox wake subjects on the trigger tab", () => {
    render(
      withUI(
        <AgentDetail
          slug="ops"
          profile={{ id: "01", slug: "ops", enabled: true, soul: "Operate." }}
          runs={[]}
          orders={[
            {
              id: "so-1",
              name: "ops mailbox",
              enabled: true,
              agent: "ops",
              triggers: [{ type: "event", subject: "board.dm.ops" }],
            },
          ]}
          triggers={[]}
          state="manual"
          schedules={[]}
          onClose={() => {}}
          onManage={() => {}}
        />,
      ),
    );

    fireEvent.click(screen.getByRole("button", { name: /Triggers/ }));
    expect(screen.getAllByText("board.dm.ops").length).toBeGreaterThan(0);
    expect(screen.getByText("board.help.ops")).toBeTruthy();
    expect(screen.getByText("board.broadcast")).toBeTruthy();
    expect(screen.getAllByText("ops mailbox").length).toBeGreaterThan(0);
  });

  it("shows acknowledged messages on the agent comms tab", async () => {
    getJSON.mockImplementation((path: string, params?: { ref?: string }) => {
      if (path === "/api/agents/permissions")
        return Promise.resolve({
          wake_access: {
            status: "direct",
            reason: "directly callable",
            direct_callable: true,
            direct_allowed: true,
            schedule_allowed: true,
            channel_allowed: true,
            operator_allowed: true,
            delegation_allowed: true,
            delegation_scope: "any",
          },
          permissions: [],
          config_entries: [],
        });
      if (path === "/api/board")
        return Promise.resolve({
          messages: [
            { id: "seen-1", topic: "dm", from: "ops", to: "writer", text: "handled", acked_by: ["researcher"], ts_unix_ms: 3 },
            { id: "mine-1", topic: "dm", from: "ops", to: "researcher", text: "please check", ts_unix_ms: 2 },
            { id: "other-1", topic: "dm", from: "ops", to: "writer", text: "not mine", ts_unix_ms: 1 },
          ],
        });
      if (path === "/api/agents/repair_status") return Promise.resolve({});
      if (path === "/api/agents/escalations") return Promise.resolve({ escalations: [] });
      if (path === "/api/memory") return Promise.resolve({ records: [] });
      if (path === "/api/skills") return Promise.resolve({ skills: [] });
      if (path === "/api/policy_log") return Promise.resolve({ decisions: [] });
      if (path === "/api/tool_log") return Promise.resolve({ invocations: [] });
      if (path === "/api/policy") return Promise.resolve({});
      if (path === "/api/edict_show") return Promise.resolve({ levels: {} });
      if (path === "/api/tools_catalog") return Promise.resolve({ tools: [] });
      if (path === "/api/catalog") return Promise.resolve({ providers: [] });
      if (path === "/api/chains") return Promise.resolve({ chains: {} });
      if (path === "/api/routing") return Promise.resolve({ chains: {} });
      if (path === "/api/provider_log") return Promise.resolve({ events: [] });
      if (path === "/api/reaper/scan") return Promise.resolve({});
      if (path === "/api/agents/impact") return Promise.resolve({});
      return Promise.resolve({ params });
    });
    render(
      withUI(
        <AgentDetail
          slug="researcher"
          profile={{ id: "01", slug: "researcher", enabled: true, soul: "Research." }}
          runs={[]}
          orders={[]}
          triggers={[]}
          state="manual"
          schedules={[]}
          onClose={() => {}}
          onManage={() => {}}
        />,
      ),
    );

    expect(await screen.findByText("sleeping · inbox 1 · 1 mailbox message waiting · manual or mailbox wake")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: /Comms/ }));
    expect(await screen.findByText("seen by researcher")).toBeTruthy();
    expect(screen.getByText("handled")).toBeTruthy();
    expect(screen.getByText("please check")).toBeTruthy();
    expect(screen.queryByText("not mine")).toBeNull();
    expect(screen.getByText("seen")).toBeTruthy();
  });

  it("replies to a waiting mailbox message as the agent", async () => {
    const onChanged = vi.fn();
    getJSON.mockImplementation((path: string, params?: { ref?: string }) => {
      if (path === "/api/agents/permissions")
        return Promise.resolve({
          wake_access: {
            status: "direct",
            reason: "directly callable",
            direct_callable: true,
            direct_allowed: true,
            schedule_allowed: true,
            channel_allowed: true,
            operator_allowed: true,
            delegation_allowed: true,
            delegation_scope: "any",
          },
          permissions: [],
          config_entries: [],
        });
      if (path === "/api/board")
        return Promise.resolve({
          messages: [
            { id: "q1", topic: "dm", from: "planner", to: "researcher", text: "deploy target?", ts_unix_ms: 3 },
          ],
        });
      if (path === "/api/agents/repair_status") return Promise.resolve({});
      if (path === "/api/agents/escalations") return Promise.resolve({ escalations: [] });
      if (path === "/api/memory") return Promise.resolve({ records: [] });
      if (path === "/api/skills") return Promise.resolve({ skills: [] });
      if (path === "/api/policy_log") return Promise.resolve({ decisions: [] });
      if (path === "/api/tool_log") return Promise.resolve({ invocations: [] });
      if (path === "/api/policy") return Promise.resolve({});
      if (path === "/api/edict_show") return Promise.resolve({ levels: {} });
      if (path === "/api/tools_catalog") return Promise.resolve({ tools: [] });
      if (path === "/api/catalog") return Promise.resolve({ providers: [] });
      if (path === "/api/chains") return Promise.resolve({ chains: {} });
      if (path === "/api/routing") return Promise.resolve({ chains: {} });
      if (path === "/api/provider_log") return Promise.resolve({ events: [] });
      if (path === "/api/reaper/scan") return Promise.resolve({});
      if (path === "/api/agents/impact") return Promise.resolve({});
      return Promise.resolve({ params });
    });
    render(
      withUI(
        <AgentDetail
          slug="researcher"
          profile={{ id: "01", slug: "researcher", enabled: true, soul: "Research." }}
          runs={[]}
          orders={[]}
          triggers={[]}
          state="manual"
          schedules={[]}
          onClose={() => {}}
          onManage={() => {}}
          onChanged={onChanged}
        />,
      ),
    );

    fireEvent.click(screen.getByRole("button", { name: /Comms/ }));
    await waitFor(() => expect(screen.getByText("deploy target?")).toBeTruthy());
    fireEvent.click(screen.getByRole("button", { name: "Reply" }));
    fireEvent.change(screen.getByLabelText("Reply to q1"), { target: { value: "ship us-east" } });
    fireEvent.click(screen.getByRole("button", { name: "Send reply" }));

    await waitFor(() =>
      expect(postJSON).toHaveBeenCalledWith("/api/board/send", {
        from: "researcher",
        reply_to: "q1",
        text: "ship us-east",
      }),
    );
    expect(onChanged).toHaveBeenCalled();
  });

  it("starts an agent-to-agent mailbox message from the comms tab", async () => {
    const onChanged = vi.fn();
    getJSON.mockImplementation((path: string, params?: { ref?: string }) => {
      if (path === "/api/agents/permissions")
        return Promise.resolve({
          wake_access: {
            status: "direct",
            reason: "directly callable",
            direct_callable: true,
            direct_allowed: true,
            schedule_allowed: true,
            channel_allowed: true,
            operator_allowed: true,
            delegation_allowed: true,
            delegation_scope: "any",
          },
          permissions: [],
          config_entries: [],
        });
      if (path === "/api/board") return Promise.resolve({ messages: [] });
      if (path === "/api/agents/repair_status") return Promise.resolve({});
      if (path === "/api/agents/escalations") return Promise.resolve({ escalations: [] });
      if (path === "/api/memory") return Promise.resolve({ records: [] });
      if (path === "/api/skills") return Promise.resolve({ skills: [] });
      if (path === "/api/policy_log") return Promise.resolve({ decisions: [] });
      if (path === "/api/tool_log") return Promise.resolve({ invocations: [] });
      if (path === "/api/policy") return Promise.resolve({});
      if (path === "/api/edict_show") return Promise.resolve({ levels: {} });
      if (path === "/api/tools_catalog") return Promise.resolve({ tools: [] });
      if (path === "/api/catalog") return Promise.resolve({ providers: [] });
      if (path === "/api/chains") return Promise.resolve({ chains: {} });
      if (path === "/api/routing") return Promise.resolve({ chains: {} });
      if (path === "/api/provider_log") return Promise.resolve({ events: [] });
      if (path === "/api/reaper/scan") return Promise.resolve({});
      if (path === "/api/agents/impact") return Promise.resolve({});
      return Promise.resolve({ params });
    });
    render(
      withUI(
        <AgentDetail
          slug="researcher"
          profile={{ id: "01", slug: "researcher", enabled: true, soul: "Research." }}
          runs={[]}
          orders={[]}
          triggers={[]}
          state="manual"
          schedules={[]}
          onClose={() => {}}
          onManage={() => {}}
          onChanged={onChanged}
        />,
      ),
    );

    fireEvent.click(screen.getByRole("button", { name: /Comms/ }));
    fireEvent.change(await screen.findByLabelText("Agent outbox recipient"), { target: { value: "planner" } });
    fireEvent.change(screen.getByLabelText("Agent outbox topic"), { target: { value: "handoff" } });
    fireEvent.change(screen.getByLabelText("Agent outbox message"), { target: { value: "need deploy plan" } });
    fireEvent.click(screen.getByRole("button", { name: "Send as agent" }));

    await waitFor(() =>
      expect(postJSON).toHaveBeenCalledWith("/api/board/send", {
        from: "researcher",
        to: "planner",
        topic: "handoff",
        text: "need deploy plan",
      }),
    );
    expect(onChanged).toHaveBeenCalled();
  });

  it("opens the board with this agent mailbox selected", async () => {
    render(
      withUI(
        <AgentDetail
          slug="researcher"
          profile={{ id: "01", slug: "researcher", enabled: true, soul: "Research." }}
          runs={[]}
          orders={[]}
          triggers={[]}
          state="manual"
          schedules={[]}
          onClose={() => {}}
          onManage={() => {}}
        />,
      ),
    );

    fireEvent.click(screen.getByRole("button", { name: /Comms/ }));
    fireEvent.click(await screen.findByRole("button", { name: /Open Board/ }));
    expect(location.hash).toBe("#board?agent=researcher");
  });

  it("arms an idle mailbox wake from the trigger tab", async () => {
    render(
      withUI(
        <AgentDetail
          slug="ops"
          profile={{ id: "01", slug: "ops", enabled: true, soul: "Operate." }}
          runs={[]}
          orders={[]}
          triggers={[]}
          state="manual"
          schedules={[]}
          onClose={() => {}}
          onManage={() => {}}
        />,
      ),
    );

    fireEvent.click(screen.getByRole("button", { name: /Triggers/ }));
    fireEvent.click(screen.getByRole("button", { name: "Arm Help mailbox wake for ops" }));

    await waitFor(() =>
      expect(postJSON).toHaveBeenCalledWith(
        "/api/standing/add",
        {
          order: expect.objectContaining({
            name: "ops help queue",
            agent: "ops",
            triggers: [{ type: "event", subject: "board.help.ops" }],
            initiative: { mode: "reactive" },
            plan: expect.stringMatching(/help request/),
          }),
        },
      ),
    );
  });

  it("does not arm mailbox wake directly for a managed sub-agent", async () => {
    render(
      withUI(
        <AgentDetail
          slug="worker"
          profile={{
            id: "01",
            slug: "worker",
            enabled: true,
            soul: "Work.",
            parent_agent: "lead",
            direct_callable: false,
          }}
          runs={[]}
          orders={[]}
          triggers={[]}
          state="manual"
          schedules={[]}
          onClose={() => {}}
          onManage={() => {}}
        />,
      ),
    );

    expect((await screen.findAllByText("managed by lead")).length).toBeGreaterThan(0);
    fireEvent.click(screen.getByRole("button", { name: /Triggers/ }));
    const arm = screen.getByRole("button", { name: "Arm DM mailbox wake for worker" }) as HTMLButtonElement;
    expect(arm.disabled).toBe(true);
    expect(screen.getAllByText("channel wake blocked; arm mailbox wake on lead").length).toBeGreaterThan(0);
    fireEvent.click(screen.getAllByRole("button", { name: "Open parent agent lead" })[0]);
    expect(location.hash).toBe("#agent/lead");
    fireEvent.click(arm);
    expect(postJSON).not.toHaveBeenCalledWith("/api/standing/add", expect.anything());
  });

  it("can re-arm a paused bound schedule from the trigger tab", async () => {
    render(
      withUI(
        <AgentDetail
          slug="ops"
          profile={{ id: "01", slug: "ops", enabled: true, soul: "Operate." }}
          runs={[]}
          orders={[]}
          triggers={[]}
          state="manual"
          schedules={[{ id: "sch-1", agent: "ops", enabled: false, cadence: "every 2h" }]}
          onClose={() => {}}
          onManage={() => {}}
        />,
      ),
    );

    fireEvent.click(screen.getByRole("button", { name: /Triggers/ }));
    fireEvent.click(screen.getByRole("button", { name: "Resume schedule sch-1" }));
    await waitFor(() =>
      expect(postAction).toHaveBeenCalledWith("/api/schedule/enable", {
        id: "sch-1",
        enabled: "true",
      }),
    );
  });

  it("shows workflow schedules that run under this agent identity", async () => {
    render(
      withUI(
        <AgentDetail
          slug="ops"
          profile={{ id: "01", slug: "ops", enabled: true, soul: "Operate." }}
          runs={[]}
          orders={[]}
          triggers={[]}
          state="manual"
          schedules={[{ id: "sch-flow", agent: "ops", target: "workflow", workflow: "nightly-sync", enabled: true, cadence: "every 2h" }]}
          onClose={() => {}}
          onManage={() => {}}
        />,
      ),
    );

    fireEvent.click(screen.getByRole("button", { name: /Triggers/ }));
    expect(screen.getByText("runs workflow nightly-sync as ops")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Remove schedule sch-flow" }));
    expect(screen.getByText("Schedule sch-flow will stop: runs workflow nightly-sync as ops.")).toBeTruthy();
  });

  it("can remove a bound schedule from the trigger tab", async () => {
    render(
      withUI(
        <AgentDetail
          slug="ops"
          profile={{ id: "01", slug: "ops", enabled: true, soul: "Operate." }}
          runs={[]}
          orders={[]}
          triggers={[]}
          state="manual"
          schedules={[{ id: "sch-1", agent: "ops", enabled: true, cadence: "every 2h" }]}
          onClose={() => {}}
          onManage={() => {}}
        />,
      ),
    );

    fireEvent.click(screen.getByRole("button", { name: /Triggers/ }));
    fireEvent.click(screen.getByRole("button", { name: "Remove schedule sch-1" }));
    expect(screen.getByText("Remove this schedule binding?")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Remove" }));
    await waitFor(() =>
      expect(postAction).toHaveBeenCalledWith("/api/schedule/remove", {
        id: "sch-1",
      }),
    );
  });

  it("can remove a bound standing order from the trigger tab", async () => {
    render(
      withUI(
        <AgentDetail
          slug="ops"
          profile={{ id: "01", slug: "ops", enabled: true, soul: "Operate." }}
          runs={[]}
          orders={[
            {
              id: "so-1",
              name: "ops mailbox",
              enabled: true,
              agent: "ops",
              triggers: [{ type: "event", subject: "board.dm.ops" }],
            },
          ]}
          triggers={[]}
          state="manual"
          schedules={[]}
          onClose={() => {}}
          onManage={() => {}}
        />,
      ),
    );

    fireEvent.click(screen.getByRole("button", { name: /Triggers/ }));
    fireEvent.click(screen.getByRole("button", { name: "Remove standing order so-1" }));
    expect(screen.getByText("Remove this standing order binding?")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Remove" }));
    await waitFor(() =>
      expect(postAction).toHaveBeenCalledWith("/api/standing/remove", {
        id: "so-1",
      }),
    );
  });

  it("shows impact and removes with selected cascade cleanup", async () => {
    const onChanged = vi.fn();
    render(
      withUI(
        <AgentDetail
          slug="ops"
          profile={{ id: "01", slug: "ops", enabled: true, soul: "Operate." }}
          runs={[]}
          orders={[]}
          triggers={[]}
          state="manual"
          schedules={[]}
          onClose={() => {}}
          onManage={() => {}}
          onChanged={onChanged}
        />,
      ),
    );

    fireEvent.click(screen.getByRole("button", { name: "Manage" }));
    await waitFor(() => expect(screen.getByText("night watch")).toBeTruthy());
    expect(screen.getByText("refresh (sch-1)")).toBeTruthy();
    expect(screen.getByText("ops note (mem-1)")).toBeTruthy();
    expect(screen.getByText("ops shared note (mem-shared-1)")).toBeTruthy();
    expect(screen.getByText("ops skill (skill-1)")).toBeTruthy();
    expect(screen.getByText("agent/ops/runtime [internal]")).toBeTruthy();
    expect(screen.getByText("agents/ops")).toBeTruthy();
    expect(screen.getByText("ops-flow/handoff handoff ops [tool]")).toBeTruthy();
    expect(screen.getByText("ops-worker [parent]")).toBeTruthy();
    expect(screen.getByText("ops-worker: worker watch")).toBeTruthy();
    expect(screen.getByText("ops-worker: worker refresh (sch-2)")).toBeTruthy();
    expect(screen.getByText("ops-worker: worker note (mem-2)")).toBeTruthy();
    expect(screen.getByText("ops-worker: worker shared note (mem-shared-2)")).toBeTruthy();
    expect(screen.getByText("ops-worker: worker skill (skill-2)")).toBeTruthy();
    expect(screen.getByText("ops-worker: agent/ops-worker/runtime [internal]")).toBeTruthy();
    expect(screen.getByText("ops-worker: agents/ops-worker")).toBeTruthy();
    expect(screen.getByText("ops-worker: worker-flow/delegate [tool]")).toBeTruthy();
    expect(screen.getByText("ops-worker: dm received (msg-2)")).toBeTruthy();
    expect(screen.getByText("will clean")).toBeTruthy();
    expect(screen.getByText("will keep")).toBeTruthy();
    expect(screen.getByText("retains dependent resources after identity deletion")).toBeTruthy();
    expect(screen.getByLabelText(/Standing orders/).closest("label")?.textContent).toContain("2");
    expect(screen.getByLabelText(/Schedules/).closest("label")?.textContent).toContain("2");
    expect(screen.getByLabelText(/Private memory/).closest("label")?.textContent).toContain("2");
    expect(screen.getByLabelText(/Private skills/).closest("label")?.textContent).toContain("2");
    expect(screen.getByLabelText(/Agent config/).closest("label")?.textContent).toContain("2");

    fireEvent.click(screen.getByRole("button", { name: "Remove" }));
    expect(screen.getByText("Remove agent ops?")).toBeTruthy();
    expect(screen.getByText(/This permanently deletes the agent identity/)).toBeTruthy();
    expect(screen.getByText(/Risk: retains dependent resources after identity deletion/)).toBeTruthy();
    expect(postJSON).not.toHaveBeenCalledWith("/api/agents/remove", expect.anything());
    const removeButtons = screen.getAllByRole("button", { name: "Remove" });
    fireEvent.click(removeButtons[removeButtons.length - 1]);
    await waitFor(() =>
      expect(postJSON).toHaveBeenCalledWith("/api/agents/remove", {
        ref: "ops",
        cascade: { standing: true, schedules: true, memory: true, authored_memory: false, skills: true, config: true, workspace: true, subagents: true },
      }),
    );
    expect(await screen.findByText("ops removed")).toBeTruthy();
    expect(screen.getByText(/identity profile deleted · cleaned 1 standing, 1 schedule, 1 private memory, 1 skill, 1 config · retired 1 dependent sub-agent · audit retained by event log/)).toBeTruthy();
    expect(onChanged).toHaveBeenCalled();
  });

  it("does not remove an agent when the hard-delete confirmation is cancelled", async () => {
    render(
      withUI(
        <AgentDetail
          slug="ops"
          profile={{ id: "01", slug: "ops", enabled: true, soul: "Operate." }}
          runs={[]}
          orders={[]}
          triggers={[]}
          state="manual"
          schedules={[]}
          onClose={() => {}}
          onManage={() => {}}
        />,
      ),
    );

    fireEvent.click(screen.getByRole("button", { name: "Manage" }));
    await waitFor(() => expect(screen.getByText("night watch")).toBeTruthy());
    fireEvent.click(screen.getByRole("button", { name: "Remove" }));
    expect(screen.getByText("Remove agent ops?")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    await waitFor(() => expect(screen.queryByText("Remove agent ops?")).toBeNull());
    expect(postJSON).not.toHaveBeenCalledWith("/api/agents/remove", expect.anything());
  });

  it("blocks removal when dependent sub-agents would be orphaned", async () => {
    render(
      withUI(
        <AgentDetail
          slug="ops"
          profile={{ id: "01", slug: "ops", enabled: true, soul: "Operate." }}
          runs={[]}
          orders={[]}
          triggers={[]}
          state="manual"
          schedules={[]}
          onClose={() => {}}
          onManage={() => {}}
        />,
      ),
    );

    fireEvent.click(screen.getByRole("button", { name: "Manage" }));
    await waitFor(() => expect(screen.getByText("ops-worker [parent]")).toBeTruthy());
    expect(screen.getByText(/Remove plan: delete identity; clean 1 standing, 1 schedule, 1 private memory, 1 skill, 1 config, shared config access refs, 1 workspace, 1 sub-agent, 1 sub-agent standing, 1 sub-agent schedule, 1 sub-agent private memory, 1 sub-agent skill, 1 sub-agent config, 1 sub-agent workspace; keep 1 authored shared memory, 1 workflow reference, 1 sub-agent authored shared memory, 1 sub-agent workflow reference/)).toBeTruthy();

    fireEvent.click(screen.getByLabelText(/Dependent sub-agents/));
    expect(screen.getByLabelText(/Private memory/).closest("label")?.textContent).toContain("1");
    expect(screen.getByText("blocked: dependent sub-agents would be orphaned")).toBeTruthy();
    expect(screen.getByText(/Dependent sub-agents must be retired with this removal/)).toBeTruthy();
    const remove = screen.getByRole("button", { name: "Remove" }) as HTMLButtonElement;
    expect(remove.disabled).toBe(true);
    fireEvent.click(remove);
    expect(postJSON).not.toHaveBeenCalledWith("/api/agents/remove", expect.anything());
  });
});

describe("AgentDetail capability control", () => {
  it("shows repair command center status from backend repair history", async () => {
    getJSON.mockImplementation((path: string, params?: { ref?: string }) => {
      if (path === "/api/agents/repair_status")
        return Promise.resolve({
          cooldown_sec: 90,
          latest: {
            phase: "failed",
            mode: "degraded",
            error: "tool denied",
            incident_id: "inc-repair-1",
          },
          history: [
            { seq: 1, phase: "failed", mode: "degraded", error: "tool denied", ts_unix_ms: 10 },
          ],
        });
      if (path === "/api/agents/permissions")
        return Promise.resolve({
          wake_access: { status: "direct", direct_callable: true, direct_allowed: true, schedule_allowed: true, channel_allowed: true, operator_allowed: true, delegation_allowed: true, delegation_scope: "any" },
          permissions: [],
          config_entries: [],
        });
      if (path === "/api/memory") return Promise.resolve({ records: [] });
      if (path === "/api/skills") return Promise.resolve({ skills: [] });
      if (path === "/api/policy_log") return Promise.resolve({ decisions: [] });
      if (path === "/api/tool_log") return Promise.resolve({ invocations: [] });
      if (path === "/api/policy") return Promise.resolve({});
      if (path === "/api/edict_show") return Promise.resolve({ levels: {} });
      if (path === "/api/tools_catalog") return Promise.resolve({ tools: [] });
      if (path === "/api/board") return Promise.resolve({ messages: [] });
      if (path === "/api/catalog") return Promise.resolve({ providers: [] });
      if (path === "/api/chains") return Promise.resolve({ chains: {} });
      if (path === "/api/routing") return Promise.resolve({ chains: {} });
      if (path === "/api/provider_log") return Promise.resolve({ events: [] });
      if (path === "/api/reaper/scan") return Promise.resolve({});
      if (path === "/api/agents/escalations") return Promise.resolve({ escalations: [] });
      if (path === "/api/agents/impact") return Promise.resolve({});
      return Promise.resolve({ params });
    });
    render(
      withUI(
        <AgentDetail
          slug="ops"
          profile={{
            id: "01",
            slug: "ops",
            enabled: true,
            soul: "Operate.",
            retry_policy: { max_attempts: 3 },
            health_policy: { doctor_agent: "guardian-doctor" },
            self_repair: { enabled: true, max_attempts: 2, escalate_to: "lead" },
          }}
          runs={[]}
          orders={[]}
          triggers={[]}
          state="manual"
          schedules={[]}
          onClose={() => {}}
          onManage={() => {}}
        />,
      ),
    );

    fireEvent.click(screen.getByRole("button", { name: /Diagnostics/ }));
    expect((await screen.findAllByText(/doctor guardian-doctor/)).length).toBeGreaterThan(0);
    expect(screen.getByText("Repair operations")).toBeTruthy();
    expect(screen.getByText("repair failing")).toBeTruthy();
    expect(screen.getAllByTitle(/up to 3 attempts/).length).toBeGreaterThan(0);
    expect(screen.getAllByText("failed · doctor · tool denied").length).toBeGreaterThan(0);
    expect(screen.getAllByText("cooldown 90s").length).toBeGreaterThan(0);
  });

  it("requests a governed repair run from the diagnostics tab", async () => {
    const onChanged = vi.fn();
    postJSON.mockResolvedValueOnce({ correlation_id: "repair-1" });
    render(
      withUI(
        <AgentDetail
          slug="ops"
          profile={{ id: "01", slug: "ops", enabled: true, soul: "Operate." }}
          runs={[]}
          orders={[]}
          triggers={[]}
          state="manual"
          schedules={[]}
          onClose={() => {}}
          onManage={() => {}}
          onChanged={onChanged}
        />,
      ),
    );

    fireEvent.click(screen.getByRole("button", { name: /Diagnostics/ }));
    fireEvent.click(await screen.findByRole("button", { name: "Repair now" }));

    await waitFor(() =>
      expect(postJSON).toHaveBeenCalledWith("/api/agents/repair", {
        ref: "ops",
        reason: "operator requested repair from ops identity page",
      }),
    );
    expect(onChanged).toHaveBeenCalled();
  });

  it("requests a governed repair run from the repair tab", async () => {
    const onChanged = vi.fn();
    postJSON.mockResolvedValueOnce({ correlation_id: "repair-2" });
    render(
      withUI(
        <AgentDetail
          slug="ops"
          profile={{ id: "01", slug: "ops", enabled: true, soul: "Operate." }}
          runs={[]}
          orders={[]}
          triggers={[]}
          state="manual"
          schedules={[]}
          onClose={() => {}}
          onManage={() => {}}
          onChanged={onChanged}
        />,
      ),
    );

    fireEvent.click(screen.getByRole("button", { name: /Repair/ }));
    fireEvent.click(await screen.findByRole("button", { name: "Repair now" }));

    await waitFor(() =>
      expect(postJSON).toHaveBeenCalledWith("/api/agents/repair", {
        ref: "ops",
        reason: "operator requested governed repair from ops repair tab",
      }),
    );
    expect(onChanged).toHaveBeenCalled();
  });

  it("blocks governed repair from the repair tab for managed sub-agents", async () => {
    render(
      withUI(
        <AgentDetail
          slug="worker"
          profile={{ id: "01", slug: "worker", enabled: true, kind: "subagent", parent_agent: "lead", direct_callable: false }}
          runs={[]}
          orders={[]}
          triggers={[]}
          state="manual"
          schedules={[]}
          onClose={() => {}}
          onManage={() => {}}
        />,
      ),
    );

    fireEvent.click(screen.getByRole("button", { name: /Repair/ }));
    const repair = await screen.findByRole("button", { name: "Repair now" }) as HTMLButtonElement;
    expect(repair.disabled).toBe(true);
    expect(repair.title).toBe("managed sub-agent; request repair through its parent/owner");
    fireEvent.click(repair);
    expect(postJSON).not.toHaveBeenCalledWith("/api/agents/repair", expect.anything());
  });

  it("saves a model change without dropping lifecycle, tasklist, or policy fields", async () => {
    const onChanged = vi.fn();
    render(
      withUI(
        <AgentDetail
          slug="ops"
          profile={{
            id: "01",
            slug: "ops",
            name: "Ops",
            enabled: true,
            soul: "Operate.",
            instructions: ["stay quiet"],
            model: "gpt-5",
            fallbacks: ["gpt-4.1"],
            task_type: "ops",
            owner_agent: "lead",
            parent_agent: "lead",
            direct_callable: false,
            lifecycle: { mode: "cycle", completed_cycles: 2, max_cycles: 5 },
            tasklist: [{ id: "task-1", title: "check queue", scope: "cycle", status: "todo" }],
            retry_policy: { max_attempts: 3 },
            health_policy: { doctor_agent: "guardian-doctor" },
            self_repair: { enabled: true, max_attempts: 2, escalate_to: "lead" },
            noise_policy: { silent_on_success: true, disable_memory_writes: true, min_notify_severity: "warning", min_notify_interval_sec: 3600 },
            tool_allow: ["memory"],
            tool_deny: ["notify"],
            trust_ceiling: "L2",
            config_overrides: { AGEZT_MODE: "quiet" },
            memory_scope: "agent:ops",
            workdir: "agents/ops",
            max_cost_mc: 100,
            max_daily_mc: 500,
            description: "Ops agent",
          }}
          runs={[]}
          orders={[]}
          triggers={[]}
          state="manual"
          schedules={[]}
          onClose={() => {}}
          onManage={() => {}}
          onChanged={onChanged}
        />,
      ),
    );

    fireEvent.click(screen.getByRole("button", { name: /Model/ }));
    fireEvent.click(screen.getByTitle("Choose model"));
    fireEvent.click(await screen.findByText("GPT-4.1"));
    fireEvent.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() =>
      expect(postJSON).toHaveBeenCalledWith("/api/agents/edit", {
        ref: "ops",
        profile: expect.objectContaining({
          name: "Ops",
          soul: "Operate.",
          instructions: ["stay quiet"],
          model: "gpt-4.1",
          fallbacks: ["gpt-4.1"],
          task_type: "ops",
          owner_agent: "lead",
          parent_agent: "lead",
          direct_callable: false,
          lifecycle: { mode: "cycle", completed_cycles: 2, max_cycles: 5 },
          tasklist: [{ id: "task-1", title: "check queue", scope: "cycle", status: "todo" }],
          retry_policy: { max_attempts: 3 },
          health_policy: { doctor_agent: "guardian-doctor" },
          self_repair: { enabled: true, max_attempts: 2, escalate_to: "lead" },
          noise_policy: { silent_on_success: true, disable_memory_writes: true, min_notify_severity: "warning", min_notify_interval_sec: 3600 },
          tool_allow: ["memory"],
          tool_deny: ["notify"],
          trust_ceiling: "L2",
          config_overrides: { AGEZT_MODE: "quiet" },
          memory_scope: "agent:ops",
          workdir: "agents/ops",
          max_cost_mc: 100,
          max_daily_mc: 500,
          description: "Ops agent",
        }),
      }),
    );
    expect(onChanged).toHaveBeenCalled();
  });

  it("updates tool policy without dropping the existing profile identity", async () => {
    const onChanged = vi.fn();
    render(
      withUI(
        <AgentDetail
          slug="ops"
          profile={{
            id: "01",
            slug: "ops",
            enabled: true,
            soul: "Operate.",
            model: "gpt-5",
            direct_callable: false,
            tool_allow: ["memory"],
            tool_deny: ["notify"],
            trust_ceiling: "L2",
            config_overrides: { AGEZT_MODE: "old" },
            memory_scope: "agent/ops",
            max_cost_mc: 50_000_000,
            max_daily_mc: 500_000_000,
          }}
          runs={[]}
          orders={[]}
          triggers={[]}
          state="manual"
          schedules={[]}
          onClose={() => {}}
          onManage={() => {}}
          onChanged={onChanged}
        />,
      ),
    );

    fireEvent.click(screen.getByRole("button", { name: /Diagnostics/ }));
    expect((await screen.findAllByText("memory")).length).toBeGreaterThan(0);
    expect(screen.getAllByText("not in agent allowlist").length).toBeGreaterThan(0);
    expect(screen.getByText("managed authority")).toBeTruthy();
    expect(screen.getByText("1 direct · 0 ask · 2 blocked · workflow absent · data lake absent · config 2/3 · trust L2")).toBeTruthy();
    expect(screen.getByText("Authority manifest")).toBeTruthy();
    expect(screen.getByText("managed authority manifest")).toBeTruthy();
    expect(screen.getAllByText("agent identity boundary").length).toBeGreaterThan(0);
    expect(screen.getAllByText("wakes and workflows invoke through ops policy").length).toBeGreaterThan(0);
    expect(screen.getByText("workflow absent")).toBeTruthy();
    expect(screen.getByText("Risk passport")).toBeTruthy();
    expect(screen.getByText("governed authority")).toBeTruthy();
    expect(screen.getByText(/direct memory · ask none · blocked (shell, fetch|fetch, shell) · config 2\/3 · trust L2/)).toBeTruthy();
    expect(screen.getByText("Authority snapshot")).toBeTruthy();
    expect(screen.getByText("high-impact tools blocked")).toBeTruthy();
    expect(screen.getByText("fetch, shell · 2 blocked/hidden · trust L2")).toBeTruthy();
    expect(screen.getByText("Authority ledger")).toBeTruthy();
    expect(screen.getByText("governed tools")).toBeTruthy();
    expect(screen.getAllByText("$0.0500 / $0.5000").length).toBeGreaterThan(0);
    expect(screen.getAllByText("shared workspace").length).toBeGreaterThan(0);
    expect(screen.getByText("Config center")).toBeTruthy();
    expect(screen.getAllByText("2/3 visible · 1 owned · 1 blocked").length).toBeGreaterThan(0);
    expect(screen.getByText("1 runtime override")).toBeTruthy();
    expect(screen.getByText("Config center access")).toBeTruthy();
    expect(screen.getByText("agent/ops/runtime")).toBeTruthy();
    expect(screen.getByText("owned · allowlisted")).toBeTruthy();
    expect(screen.getByText("secret:value")).toBeTruthy();
    expect(screen.getByText("excluded")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Allow ops for secret:value" }));
    await waitFor(() =>
      expect(postJSON).toHaveBeenCalledWith("/api/configcenter/access", {
        key: "secret:value",
        allowed_agents: ["ops"],
        excluded_agents: [],
      }),
    );
    fireEvent.change(screen.getByLabelText("Trust ceiling"), { target: { value: "L3" } });
    fireEvent.change(screen.getByLabelText("Tool allow"), { target: { value: "memory, shell" } });
    fireEvent.change(screen.getByLabelText("Tool deny"), { target: { value: "notify, browser" } });
    fireEvent.change(screen.getByLabelText("Memory scope"), { target: { value: "team/ops" } });
    fireEvent.change(screen.getByLabelText("Workspace subdir"), { target: { value: "agents/ops-lab" } });
    fireEvent.change(screen.getByLabelText("Max/run ($)"), { target: { value: "0.25" } });
    fireEvent.change(screen.getByLabelText("Max/day ($)"), { target: { value: "2.5" } });
    fireEvent.change(screen.getByLabelText("Config overrides"), { target: { value: "AGEZT_MODE=new\nAGEZT_PROVIDER=openai" } });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => expect(postJSON).toHaveBeenCalledWith(
      "/api/agents/capabilities",
      expect.objectContaining({
        ref: "ops",
        trust_ceiling: "L3",
        tool_allow: ["memory", "shell"],
        tool_deny: ["notify", "browser"],
        memory_scope: "team/ops",
        workdir: "agents/ops-lab",
        max_cost_mc: 250_000_000,
        max_daily_mc: 2_500_000_000,
        config_overrides: { AGEZT_MODE: "new", AGEZT_PROVIDER: "openai" },
      }),
    ));
    expect(onChanged).toHaveBeenCalled();
  });

  it("surfaces workflow-chain tool access in the capability panel", async () => {
    getJSON.mockImplementation((path: string) => {
      if (path === "/api/agents/permissions")
        return Promise.resolve({
          wake_access: { status: "direct", direct_callable: true, direct_allowed: true, schedule_allowed: true, channel_allowed: true, operator_allowed: true, delegation_allowed: true, delegation_scope: "any" },
          permissions: [
            { name: "workflow", capability: "workflow", allowed: true, ask: true, status: "L2", source: "edict", reason: "requires approval" },
          ],
          config_entries: [],
        });
      if (path === "/api/tools_catalog") return Promise.resolve({ tools: [] });
      if (path === "/api/memory") return Promise.resolve({ records: [] });
      if (path === "/api/skills") return Promise.resolve({ skills: [] });
      if (path === "/api/policy_log") return Promise.resolve({ decisions: [] });
      if (path === "/api/tool_log") return Promise.resolve({ invocations: [] });
      if (path === "/api/policy") return Promise.resolve({});
      if (path === "/api/edict_show") return Promise.resolve({ levels: {} });
      if (path === "/api/board") return Promise.resolve({ messages: [] });
      if (path === "/api/chains") return Promise.resolve({ chains: {} });
      if (path === "/api/routing") return Promise.resolve({ chains: {} });
      if (path === "/api/provider_log") return Promise.resolve({ events: [] });
      if (path === "/api/reaper/scan") return Promise.resolve({});
      if (path === "/api/agents/repair_status") return Promise.resolve({});
      if (path === "/api/agents/escalations") return Promise.resolve({ escalations: [] });
      if (path === "/api/agents/impact") return Promise.resolve({});
      return Promise.resolve({});
    });
    render(
      withUI(
        <AgentDetail
          slug="ops"
          profile={{ id: "01", slug: "ops", enabled: true, soul: "Operate." }}
          runs={[]}
          orders={[]}
          triggers={[]}
          state="manual"
          schedules={[]}
          onClose={() => {}}
          onManage={() => {}}
        />,
      ),
    );

    fireEvent.click(screen.getByRole("button", { name: /Diagnostics/ }));
    expect(await screen.findByText("workflow chains ask-gated")).toBeTruthy();
    expect(screen.getAllByText("requires approval").length).toBeGreaterThan(0);
  });

  it("edits tool policy from the effective access table", async () => {
    const onChanged = vi.fn();
    render(
      withUI(
        <AgentDetail
          slug="ops"
          profile={{
            id: "01",
            slug: "ops",
            enabled: true,
            soul: "Operate.",
            tool_allow: ["memory"],
            tool_deny: ["notify"],
            trust_ceiling: "L2",
          }}
          runs={[]}
          orders={[]}
          triggers={[]}
          state="manual"
          schedules={[]}
          onClose={() => {}}
          onManage={() => {}}
          onChanged={onChanged}
        />,
      ),
    );

    fireEvent.click(screen.getByRole("button", { name: /Diagnostics/ }));
    expect((await screen.findAllByText("shell")).length).toBeGreaterThan(0);
    fireEvent.click(screen.getByRole("button", { name: "Allow shell" }));
    fireEvent.click(screen.getByRole("button", { name: "Deny fetch" }));
    fireEvent.click(screen.getByRole("button", { name: "Clear memory policy" }));
    fireEvent.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() =>
      expect(postJSON).toHaveBeenCalledWith(
        "/api/agents/capabilities",
        expect.objectContaining({
          ref: "ops",
          tool_allow: ["shell"],
          tool_deny: ["notify", "fetch"],
        }),
      ),
    );
  });

  it("rejects overlapping allow and deny tool policies before saving", async () => {
    render(
      withUI(
        <AgentDetail
          slug="ops"
          profile={{
            id: "01",
            slug: "ops",
            enabled: true,
            soul: "Operate.",
            tool_allow: ["memory"],
            tool_deny: [],
            trust_ceiling: "L2",
          }}
          runs={[]}
          orders={[]}
          triggers={[]}
          state="manual"
          schedules={[]}
          onClose={() => {}}
          onManage={() => {}}
          onChanged={() => {}}
        />,
      ),
    );

    fireEvent.click(screen.getByRole("button", { name: /Diagnostics/ }));
    fireEvent.change(await screen.findByLabelText("Tool deny"), { target: { value: "memory" } });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => expect(screen.getByText("Tool memory cannot be both allowed and denied")).toBeTruthy());
    expect(postJSON).not.toHaveBeenCalledWith("/api/agents/capabilities", expect.anything());
  });

  it("rejects invalid capability budget inputs before saving", async () => {
    render(
      withUI(
        <AgentDetail
          slug="ops"
          profile={{
            id: "01",
            slug: "ops",
            enabled: true,
            soul: "Operate.",
            trust_ceiling: "L2",
          }}
          runs={[]}
          orders={[]}
          triggers={[]}
          state="manual"
          schedules={[]}
          onClose={() => {}}
          onManage={() => {}}
          onChanged={() => {}}
        />,
      ),
    );

    fireEvent.click(screen.getByRole("button", { name: /Diagnostics/ }));
    fireEvent.change(await screen.findByLabelText("Max/run ($)"), { target: { value: "-1" } });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => expect(screen.getByText("Max/run must be a dollar amount like 0.05")).toBeTruthy());
    expect(postJSON).not.toHaveBeenCalledWith("/api/agents/capabilities", expect.anything());
  });

  it("deduplicates tool policy entries before saving", async () => {
    render(
      withUI(
        <AgentDetail
          slug="ops"
          profile={{
            id: "01",
            slug: "ops",
            enabled: true,
            soul: "Operate.",
            tool_allow: [],
            tool_deny: [],
            trust_ceiling: "L2",
          }}
          runs={[]}
          orders={[]}
          triggers={[]}
          state="manual"
          schedules={[]}
          onClose={() => {}}
          onManage={() => {}}
          onChanged={() => {}}
        />,
      ),
    );

    fireEvent.click(screen.getByRole("button", { name: /Diagnostics/ }));
    fireEvent.change(await screen.findByLabelText("Tool allow"), { target: { value: "memory, Memory, shell" } });
    fireEvent.change(screen.getByLabelText("Tool deny"), { target: { value: "notify, NOTIFY" } });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() =>
      expect(postJSON).toHaveBeenCalledWith(
        "/api/agents/capabilities",
        expect.objectContaining({
          ref: "ops",
          tool_allow: ["memory", "shell"],
          tool_deny: ["notify"],
        }),
      ),
    );
  });

  it("applies capability policy presets before saving", async () => {
    render(
      withUI(
        <AgentDetail
          slug="guardian-health"
          profile={{
            id: "01",
            slug: "guardian-health",
            enabled: true,
            system: true,
            soul: "Watch health.",
            tool_allow: ["memory"],
            tool_deny: ["shell"],
            trust_ceiling: "L4",
            noise_policy: { min_notify_severity: "info", min_notify_interval_sec: 0 },
          }}
          runs={[]}
          orders={[]}
          triggers={[]}
          state="manual"
          schedules={[]}
          onClose={() => {}}
          onManage={() => {}}
          onChanged={() => {}}
        />,
      ),
    );

    fireEvent.click(screen.getByRole("button", { name: /Diagnostics/ }));
    fireEvent.click(await screen.findByRole("button", { name: "Quiet system preset" }));
    fireEvent.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() =>
      expect(postJSON).toHaveBeenCalledWith(
        "/api/agents/capabilities",
        expect.objectContaining({
          ref: "guardian-health",
          trust_ceiling: "L2",
          tool_allow: [],
          tool_deny: ["shell", "memory"],
          memory_scope: "system/guardian-health",
          max_cost_mc: 50_000_000,
          max_daily_mc: 50_000_000,
          noise_policy: {
            silent_on_success: true,
            disable_memory_writes: true,
            min_notify_severity: "warning",
            min_notify_interval_sec: 28800,
          },
        }),
      ),
    );
  });

  it("applies high-impact lockdown without removing low-risk allowlisted tools", async () => {
    render(
      withUI(
        <AgentDetail
          slug="builder"
          profile={{
            id: "02",
            slug: "builder",
            enabled: true,
            soul: "Build.",
            tool_allow: ["memory", "shell", "workflow"],
            tool_deny: ["notify"],
            trust_ceiling: "L4",
            noise_policy: { min_notify_severity: "info", min_notify_interval_sec: 30 },
          }}
          runs={[]}
          orders={[]}
          triggers={[]}
          state="manual"
          schedules={[]}
          onClose={() => {}}
          onManage={() => {}}
          onChanged={() => {}}
        />,
      ),
    );

    fireEvent.click(screen.getByRole("button", { name: /Diagnostics/ }));
    fireEvent.click(await screen.findByRole("button", { name: "High-impact lockdown" }));
    fireEvent.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() =>
      expect(postJSON).toHaveBeenCalledWith(
        "/api/agents/capabilities",
        expect.objectContaining({
          ref: "builder",
          trust_ceiling: "L2",
          tool_allow: ["memory"],
          tool_deny: ["shell", "workflow", "fetch", "db", "browser", "mcp", "file", "tool_forge", "homeassistant", "notify"],
          noise_policy: expect.objectContaining({
            silent_on_success: true,
            min_notify_severity: "warning",
            min_notify_interval_sec: 3600,
          }),
        }),
      ),
    );
  });

  it("applies the open lab preset by clearing agent-local restrictions", async () => {
    render(
      withUI(
        <AgentDetail
          slug="builder"
          profile={{
            id: "02",
            slug: "builder",
            enabled: true,
            soul: "Build.",
            tool_allow: ["memory"],
            tool_deny: ["shell"],
            trust_ceiling: "L1",
            noise_policy: {
              silent_on_success: true,
              disable_memory_writes: true,
              min_notify_severity: "critical",
              min_notify_interval_sec: 999,
            },
          }}
          runs={[]}
          orders={[]}
          triggers={[]}
          state="manual"
          schedules={[]}
          onClose={() => {}}
          onManage={() => {}}
          onChanged={() => {}}
        />,
      ),
    );

    fireEvent.click(screen.getByRole("button", { name: /Diagnostics/ }));
    fireEvent.click(await screen.findByRole("button", { name: "Open lab preset" }));
    fireEvent.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() =>
      expect(postJSON).toHaveBeenCalledWith(
        "/api/agents/capabilities",
        expect.objectContaining({
          ref: "builder",
          trust_ceiling: "L4",
          tool_allow: [],
          tool_deny: [],
          max_cost_mc: 0,
          max_daily_mc: 0,
          noise_policy: {
            silent_on_success: false,
            disable_memory_writes: false,
            min_notify_severity: "info",
            min_notify_interval_sec: 0,
          },
        }),
      ),
    );
  });

  it("shows backend wake/delegation policy for managed sub-agents", async () => {
    render(
      withUI(
        <AgentDetail
          slug="worker"
          profile={{
            id: "03",
            slug: "worker",
            enabled: true,
            soul: "Handle delegated work.",
            managed: true,
            direct_callable: false,
            parent_agent: "lead",
            kind: "subagent",
          }}
          runs={[]}
          orders={[]}
          triggers={[]}
          state="manual"
          schedules={[]}
          onClose={() => {}}
          onManage={() => {}}
          onChanged={() => {}}
        />,
      ),
    );

    expect((await screen.findAllByText("managed by lead")).length).toBeGreaterThan(0);
    fireEvent.click(screen.getByRole("button", { name: /Diagnostics/ }));
    expect((await screen.findAllByText("managed by lead")).length).toBeGreaterThan(0);
    expect(screen.getAllByText("operator").length).toBeGreaterThanOrEqual(1);
    expect(screen.getAllByText("schedule").length).toBeGreaterThanOrEqual(1);
    expect(screen.getAllByText("channel").length).toBeGreaterThanOrEqual(1);
    expect(screen.getAllByText("delegation").length).toBeGreaterThanOrEqual(1);
    expect(screen.getByText("manager: lead")).toBeTruthy();
    expect(screen.getAllByText("blocked").length).toBeGreaterThanOrEqual(3);
    expect(screen.getAllByText("allowed").length).toBeGreaterThanOrEqual(1);
    expect((screen.getByRole("button", { name: "Wake worker" }) as HTMLButtonElement).disabled).toBe(true);
  });
});

describe("AgentDetail tasklist controls", () => {
  it("summarizes durable task status inside the identity task board", () => {
    render(
      withUI(
        <AgentDetail
          slug="ops"
          profile={{
            id: "01",
            slug: "ops",
            enabled: true,
            soul: "Operate.",
            tasklist: [
              { id: "task-1", title: "check queue", scope: "cycle", status: "todo" },
              { id: "task-2", title: "drain queue", scope: "cycle", status: "doing" },
              { id: "task-3", title: "fix deploy", scope: "cycle", status: "blocked" },
              { id: "task-4", title: "write report", scope: "total", status: "done" },
            ],
          }}
          runs={[]}
          orders={[]}
          triggers={[]}
          state="manual"
          schedules={[]}
          onClose={() => {}}
          onManage={() => {}}
        />,
      ),
    );

    fireEvent.click(screen.getByRole("button", { name: /Soul/ }));
    expect(screen.getByText("1 todo · 1 doing · 1 blocked")).toBeTruthy();
    expect(screen.getByText("1 done")).toBeTruthy();
    expect(screen.getByText("fix deploy")).toBeTruthy();
  });

  it("adds a new cycle task from the identity page", async () => {
    const onChanged = vi.fn();
    render(
      withUI(
        <AgentDetail
          slug="ops"
          profile={{ id: "01", slug: "ops", enabled: true, soul: "Operate." }}
          runs={[]}
          orders={[]}
          triggers={[]}
          state="manual"
          schedules={[]}
          onClose={() => {}}
          onManage={() => {}}
          onChanged={onChanged}
        />,
      ),
    );

    fireEvent.click(screen.getByRole("button", { name: /Soul/ }));
    fireEvent.change(screen.getByLabelText("New agent task"), { target: { value: "check queue" } });
    fireEvent.change(screen.getByLabelText("New task scope"), { target: { value: "cycle" } });
    fireEvent.click(screen.getByRole("button", { name: /Add task/ }));

    await waitFor(() =>
      expect(postJSON).toHaveBeenCalledWith("/api/agents/task", {
        ref: "ops",
        op: "add",
        title: "check queue",
        scope: "cycle",
        status: "todo",
      }),
    );
    expect(onChanged).toHaveBeenCalled();
  });

  it("updates one durable agent task without rewriting the whole profile", async () => {
    const onChanged = vi.fn();
    render(
      withUI(
        <AgentDetail
          slug="ops"
          profile={{
            id: "01",
            slug: "ops",
            enabled: true,
            soul: "Operate.",
            tasklist: [{ id: "task-1", title: "check queue", scope: "cycle", status: "todo" }],
          }}
          runs={[]}
          orders={[]}
          triggers={[]}
          state="manual"
          schedules={[]}
          onClose={() => {}}
          onManage={() => {}}
          onChanged={onChanged}
        />,
      ),
    );

    fireEvent.click(screen.getByRole("button", { name: /Soul/ }));
    expect(screen.getByText("check queue")).toBeTruthy();
    fireEvent.click(screen.getByTitle("Mark done"));
    await waitFor(() =>
      expect(postJSON).toHaveBeenCalledWith("/api/agents/task", {
        ref: "ops",
        op: "update",
        id: "task-1",
        status: "done",
      }),
    );
    expect(onChanged).toHaveBeenCalled();
  });

  it("removes one durable agent task from the identity page", async () => {
    const onChanged = vi.fn();
    render(
      withUI(
        <AgentDetail
          slug="ops"
          profile={{
            id: "01",
            slug: "ops",
            enabled: true,
            soul: "Operate.",
            tasklist: [{ id: "task-1", title: "check queue", scope: "total", status: "todo" }],
          }}
          runs={[]}
          orders={[]}
          triggers={[]}
          state="manual"
          schedules={[]}
          onClose={() => {}}
          onManage={() => {}}
          onChanged={onChanged}
        />,
      ),
    );

    fireEvent.click(screen.getByRole("button", { name: /Soul/ }));
    fireEvent.click(screen.getByTitle("Remove task"));
    await waitFor(() =>
      expect(postJSON).toHaveBeenCalledWith("/api/agents/task", {
        ref: "ops",
        op: "remove",
        id: "task-1",
      }),
    );
    expect(onChanged).toHaveBeenCalled();
  });
});
