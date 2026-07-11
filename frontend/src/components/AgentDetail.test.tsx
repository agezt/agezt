// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
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

import { AgentDetail } from "@/components/AgentDetail";
import {
  agentLifecycleActionResultSummary,
  agentLifecycleDecisionLedger,
  agentLifecycleInterventionSummary,
  agentRemovalImpactPlan,
  agentRemovalRiskLabel,
  agentRepairCommandSummary,
  agentRepairDecisionSummary,
  agentRepairOperationsSummary,
  agentRetryPolicyDetail,
  agentScheduleBindingTitle,
} from "@/components/agentdetail/lifecycle";
import {
  agentBoardMessages,
  agentInboxPrioritySummary,
  agentMailboxSubjects,
  mailboxSubjectBinding,
  mailboxWakeArmIssue,
  messageAckedBy,
  messageAckedByLabel,
  operatorWakeIssue,
  waitingForAgent,
} from "@/components/agentdetail/comms";
import { normalizeNoiseToolPolicy, workflowToolAccessSummary } from "@/components/agentdetail/capability";
import { UIProvider } from "@/components/ui/feedback";

const withUI = (node: ReactNode) => <UIProvider>{node}</UIProvider>;

function chooseDetailOption(group: string, name: RegExp | string) {
  fireEvent.click(within(screen.getByRole("group", { name: group })).getByRole("button", { name }));
}

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

describe("agentScheduleBindingTitle", () => {
  it("distinguishes agent wake schedules from workflow/tool schedules running as an agent", () => {
    expect(agentScheduleBindingTitle({ intent: "check disks" }, "ops")).toBe("wakes ops: check disks");
    expect(agentScheduleBindingTitle({ target: "workflow", workflow: "nightly-sync" }, "ops")).toBe("runs workflow nightly-sync as ops");
    expect(agentScheduleBindingTitle({ target: "tool", tool: "shell" }, "ops")).toBe("invokes tool shell as ops");
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
});

describe("AgentDetail shell", () => {
  it("renders exactly the six grouped detail tabs", () => {
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

    const tablist = screen.getByRole("tablist", { name: "ops detail sections" });
    const tabs = within(tablist).getAllByRole("button");
    expect(tabs.map((b) => b.textContent)).toEqual([
      "Overview",
      "Activity",
      "Wiring",
      "Mind",
      "Model",
      "Diagnostics",
    ]);
    // The old top-level tabs were folded into the six groups.
    expect(within(tablist).queryByRole("button", { name: /Soul|Triggers|Comms|Memory|Skills|Repair|Files/ })).toBeNull();
    expect(within(tablist).getByRole("button", { name: /Overview/ }).getAttribute("aria-pressed")).toBe("true");
  });

  it("shows the glance metrics, model card, identity card, and lifecycle disclosure", () => {
    render(
      withUI(
        <AgentDetail
          slug="ops"
          profile={{ id: "01", slug: "ops", enabled: true, soul: "Operate.", model: "gpt-5", fallbacks: ["gpt-4.1"] }}
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

    // Glance layer: one MetricWidget per fact.
    expect(screen.getByText("Presence")).toBeTruthy();
    expect(screen.getByText("Next wake")).toBeTruthy();
    expect(screen.getByText("Runs")).toBeTruthy();
    expect(screen.getByText("Spend today")).toBeTruthy();
    expect(screen.getByText("Health")).toBeTruthy();
    // Model card with the in-place editor.
    expect(screen.getByTitle("Edit model and fallback chain")).toBeTruthy();
    // Overview holds the single identity card and the lifecycle disclosure.
    expect(screen.getByText("identity")).toBeTruthy();
    expect(screen.getByText("How does this run?")).toBeTruthy();
    expect(screen.getByText("today's spend")).toBeTruthy();
    expect(screen.getByText("per-run ceiling")).toBeTruthy();
    expect(screen.getByRole("button", { name: "Lifecycle intervention" })).toBeTruthy();
    expect(screen.getByRole("button", { name: /Manage in Roster/ })).toBeTruthy();
    // The old prose walls are gone.
    expect(screen.queryByText("All details")).toBeNull();
    expect(screen.queryByText("More actions")).toBeNull();
    expect(screen.queryByText("Operations passport")).toBeNull();
    expect(screen.queryByText("Agent entity contract")).toBeNull();
    expect(screen.queryByText("Autonomy runbook")).toBeNull();
  });

  it("clicking the failure headline focuses the run on the Activity tab", async () => {
    render(
      withUI(
        <AgentDetail
          slug="ops"
          profile={{ id: "01", slug: "ops", enabled: true, soul: "Operate." }}
          runs={[{ correlation_id: "run-bad", agent: "ops", status: "failed", started_unix_ms: Date.now() - 60_000 }]}
          orders={[]}
          triggers={[]}
          state="manual"
          schedules={[]}
          onClose={() => {}}
          onManage={() => {}}
        />,
      ),
    );

    fireEvent.click(screen.getByRole("button", { name: /Most recent failure/ }));
    const tablist = screen.getByRole("tablist", { name: "ops detail sections" });
    expect(within(tablist).getByRole("button", { name: /Activity/ }).getAttribute("aria-pressed")).toBe("true");
    await waitFor(() => expect(screen.getByText(/focused run/)).toBeTruthy());
    expect(screen.getAllByText(/run-bad/).length).toBeGreaterThan(0);
  });

  it("pauses and resumes the agent from the header action rail", async () => {
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

    fireEvent.click(screen.getByRole("button", { name: "Pause" }));
    await waitFor(() =>
      expect(postAction).toHaveBeenCalledWith("/api/agents/enable", {
        ref: "ops",
        enabled: "false",
      }),
    );
    expect(onChanged).toHaveBeenCalled();

    postAction.mockClear();
    rerender(
      withUI(
        <AgentDetail
          slug="ops"
          profile={{ id: "01", slug: "ops", enabled: false, soul: "Operate." }}
          runs={[]}
          orders={[]}
          triggers={[]}
          state="paused"
          schedules={[]}
          onClose={() => {}}
          onManage={() => {}}
          onChanged={onChanged}
        />,
      ),
    );
    fireEvent.click(screen.getByRole("button", { name: "Resume" }));
    await waitFor(() =>
      expect(postAction).toHaveBeenCalledWith("/api/agents/enable", {
        ref: "ops",
        enabled: "true",
      }),
    );
  });

  it("edits the model from the header model card", async () => {
    const onChanged = vi.fn();
    render(
      withUI(
        <AgentDetail
          slug="ops"
          profile={{ id: "01", slug: "ops", enabled: true, soul: "Operate.", model: "gpt-5" }}
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

    fireEvent.click(screen.getByTitle("Edit model and fallback chain"));
    fireEvent.click(screen.getByTitle("Choose model"));
    fireEvent.click(await screen.findByText("GPT-4.1"));

    await waitFor(() =>
      expect(postJSON).toHaveBeenCalledWith("/api/agents/edit", {
        ref: "ops",
        profile: expect.objectContaining({
          model: "gpt-4.1",
          soul: "Operate.",
        }),
      }),
    );
    expect(onChanged).toHaveBeenCalled();
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

    fireEvent.click(screen.getByRole("button", { name: "Wake ops" }));
    await waitFor(() =>
      expect(postAction).toHaveBeenCalledWith("/api/agents/wake", {
        ref: "ops",
        reason: "manual operator wake",
      }),
    );
    // The wake's correlation id is focused on the Activity tab.
    const tablist = screen.getByRole("tablist", { name: "ops detail sections" });
    await waitFor(() =>
      expect(within(tablist).getByRole("button", { name: /Activity/ }).getAttribute("aria-pressed")).toBe("true"),
    );
    await waitFor(() => expect(screen.getByText(/focused run/)).toBeTruthy());
    expect(getJSON).toHaveBeenCalledWith("/api/journal", {
      correlation_id: "wake-1",
      limit: "500",
    });
  });

  it("offers the quiet-guardian action only for system agents", async () => {
    const { unmount } = render(
      withUI(
        <AgentDetail
          slug="guardian-health"
          profile={{
            id: "01G",
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

    const quiet = screen.getByRole("button", { name: "Quiet guardian guardian-health" });
    expect(quiet.title).toBe("Apply quiet system guardian policy to this agent");
    unmount();

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
    expect(screen.queryByRole("button", { name: "Quiet guardian ops" })).toBeNull();
  });

  it("can quiet a noisy system guardian from the header action rail", async () => {
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

    fireEvent.click(screen.getByRole("button", { name: "Quiet guardian guardian-routing" }));

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

  it("keeps lifecycle removal impact reachable from the overview disclosure", async () => {
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

    // The panel lives inside the Overview "Lifecycle intervention" disclosure;
    // children stay mounted, and the summary button expands it.
    fireEvent.click(screen.getByRole("button", { name: "Lifecycle intervention" }));
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
    // The panel refetches impact (and resets cascade defaults) once the aux
    // memory/skills fetches settle — wait for that before overriding cascade.
    await waitFor(() =>
      expect(getJSON.mock.calls.filter((c) => c[0] === "/api/agents/impact").length).toBeGreaterThanOrEqual(2),
    );
    await waitFor(() =>
      expect((screen.getByLabelText(/Dependent sub-agents/) as HTMLInputElement).checked).toBe(true),
    );
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

    fireEvent.click(screen.getByRole("button", { name: "Lifecycle intervention" }));
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
    fireEvent.click(await screen.findByRole("button", { name: "Open wake owner lead" }));
    expect(location.hash).toBe("#agent/lead");
    fireEvent.click(wake);
    expect(postAction).not.toHaveBeenCalledWith("/api/agents/wake", expect.anything());
  });

  it("surfaces the active run in the Now panel and focuses it on Activity", async () => {
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

    expect(screen.getByText("Now")).toBeTruthy();
    expect(screen.getAllByText("using tool").length).toBeGreaterThan(0);
    expect(screen.getAllByText(/check disks/).length).toBeGreaterThan(0);
    expect(screen.getAllByText("shell.exec").length).toBeGreaterThan(0);
    expect(screen.getAllByText("gpt-5").length).toBeGreaterThan(0);
    expect(screen.getAllByText(/corr-1/).length).toBeGreaterThan(0);
    // Identity card on the overview reflects the cycle lifecycle contract.
    expect(screen.getAllByText(/1\/3 cycles complete; retires at max cycles/).length).toBeGreaterThan(0);
    // The Now panel's Inspect opens the focused run on the Activity tab.
    fireEvent.click(screen.getByTitle("Inspect the active run in this agent"));
    expect(screen.getByText(/focused run/)).toBeTruthy();
    await waitFor(() =>
      expect(getJSON).toHaveBeenCalledWith("/api/journal", {
        correlation_id: "corr-1",
        limit: "500",
      }),
    );
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

  it("edits lifecycle contract from the mind tab without dropping agent fields", async () => {
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

    fireEvent.click(screen.getByRole("button", { name: /Mind/ }));
    chooseDetailOption("Agent lifecycle mode", /Cycle/);
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

  it("pauses frequent schedules from the header action rail", async () => {
    render(
      withUI(
        <AgentDetail
          slug="ops"
          profile={{ id: "01", slug: "ops", enabled: true, soul: "Operate." }}
          runs={[]}
          orders={[]}
          triggers={[]}
          state="manual"
          schedules={[{ id: "sch-fast", agent: "ops", enabled: true, mode: "interval", interval_sec: 600, cadence: "every 10m" }]}
          onClose={() => {}}
          onManage={() => {}}
        />,
      ),
    );

    expect(screen.getByText("Pause wakes")).toBeTruthy();
    fireEvent.click(screen.getByLabelText("Pause frequent schedules for ops"));
    fireEvent.click(await screen.findByRole("button", { name: "Pause schedules" }));
    await waitFor(() =>
      expect(postAction).toHaveBeenCalledWith("/api/schedule/enable", { id: "sch-fast", enabled: "false" }),
    );
  });

  it("shows mailbox wake subjects on the wiring tab", () => {
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

    fireEvent.click(screen.getByRole("button", { name: /Wiring/ }));
    expect(screen.getAllByText("board.dm.ops").length).toBeGreaterThan(0);
    expect(screen.getByText("board.help.ops")).toBeTruthy();
    expect(screen.getByText("board.broadcast")).toBeTruthy();
    expect(screen.getAllByText("ops mailbox").length).toBeGreaterThan(0);
  });

  it("shows acknowledged messages on the agent wiring tab", async () => {
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

    fireEvent.click(screen.getByRole("button", { name: /Wiring/ }));
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

    fireEvent.click(screen.getByRole("button", { name: /Wiring/ }));
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

  it("starts an agent-to-agent mailbox message from the wiring tab", async () => {
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

    fireEvent.click(screen.getByRole("button", { name: /Wiring/ }));
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

    fireEvent.click(screen.getByRole("button", { name: /Wiring/ }));
    fireEvent.click(await screen.findByRole("button", { name: /Open Board/ }));
    expect(location.hash).toBe("#board?agent=researcher");
  });

  it("arms an idle mailbox wake from the wiring tab", async () => {
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

    fireEvent.click(screen.getByRole("button", { name: /Wiring/ }));
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

    fireEvent.click(screen.getByRole("button", { name: /Wiring/ }));
    expect((await screen.findAllByText("channel wake blocked; arm mailbox wake on lead")).length).toBeGreaterThan(0);
    const arm = screen.getByRole("button", { name: "Arm DM mailbox wake for worker" }) as HTMLButtonElement;
    expect(arm.disabled).toBe(true);
    fireEvent.click(screen.getAllByRole("button", { name: "Open parent agent lead" })[0]);
    expect(location.hash).toBe("#agent/lead");
    fireEvent.click(arm);
    expect(postJSON).not.toHaveBeenCalledWith("/api/standing/add", expect.anything());
  });

  it("can re-arm a paused bound schedule from the wiring tab", async () => {
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

    fireEvent.click(screen.getByRole("button", { name: /Wiring/ }));
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

    fireEvent.click(screen.getByRole("button", { name: /Wiring/ }));
    expect(screen.getAllByText("runs workflow nightly-sync as ops").length).toBeGreaterThan(0);
    fireEvent.click(screen.getByRole("button", { name: "Remove schedule sch-flow" }));
    expect(screen.getByText("Schedule sch-flow will stop: runs workflow nightly-sync as ops.")).toBeTruthy();
  });

  it("can remove a bound schedule from the wiring tab", async () => {
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

    fireEvent.click(screen.getByRole("button", { name: /Wiring/ }));
    fireEvent.click(screen.getByRole("button", { name: "Remove schedule sch-1" }));
    expect(screen.getByText("Remove this schedule binding?")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Remove" }));
    await waitFor(() =>
      expect(postAction).toHaveBeenCalledWith("/api/schedule/remove", {
        id: "sch-1",
      }),
    );
  });

  it("can remove a bound standing order from the wiring tab", async () => {
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

    fireEvent.click(screen.getByRole("button", { name: /Wiring/ }));
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

    // Wait for the post-aux impact refetch so the cascade toggle below is not
    // reset back to defaults after we uncheck it.
    await waitFor(() =>
      expect(getJSON.mock.calls.filter((c) => c[0] === "/api/agents/impact").length).toBeGreaterThanOrEqual(2),
    );
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
    expect(screen.getAllByText("repair failing").length).toBeGreaterThan(0);
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
    // The Diagnostics tab renders DiagTab's repair center first, then the
    // AgentRepair workbench stacked below — two "Repair now" entry points.
    const repairButtons = await screen.findAllByRole("button", { name: "Repair now" });
    expect(repairButtons.length).toBe(2);
    fireEvent.click(repairButtons[0]);

    await waitFor(() =>
      expect(postJSON).toHaveBeenCalledWith("/api/agents/repair", {
        ref: "ops",
        reason: "operator requested repair from ops identity page",
      }),
    );
    expect(onChanged).toHaveBeenCalled();
  });

  it("requests a governed repair run from the repair workbench under diagnostics", async () => {
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

    fireEvent.click(screen.getByRole("button", { name: /Diagnostics/ }));
    const repairButtons = await screen.findAllByRole("button", { name: "Repair now" });
    fireEvent.click(repairButtons[repairButtons.length - 1]);

    await waitFor(() =>
      expect(postJSON).toHaveBeenCalledWith("/api/agents/repair", {
        ref: "ops",
        reason: "operator requested governed repair from ops repair tab",
      }),
    );
    expect(onChanged).toHaveBeenCalled();
  });

  it("blocks governed repair for managed sub-agents", async () => {
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

    fireEvent.click(screen.getByRole("button", { name: /Diagnostics/ }));
    const repairButtons = await screen.findAllByRole("button", { name: "Repair now" }) as HTMLButtonElement[];
    for (const btn of repairButtons) {
      expect(btn.disabled).toBe(true);
      expect(btn.title).toContain("managed sub-agent; request repair through its parent/owner");
      fireEvent.click(btn);
    }
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
    chooseDetailOption("Trust ceiling", /L3/);
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
  it("summarizes durable task status inside the mind tab task board", () => {
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

    fireEvent.click(screen.getByRole("button", { name: /Mind/ }));
    expect(screen.getByText("1 todo · 1 doing · 1 blocked")).toBeTruthy();
    expect(screen.getByText("1 done")).toBeTruthy();
    expect(screen.getByText("fix deploy")).toBeTruthy();
  });

  it("adds a new cycle task from the mind tab", async () => {
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

    fireEvent.click(screen.getByRole("button", { name: /Mind/ }));
    fireEvent.change(screen.getByLabelText("New agent task"), { target: { value: "check queue" } });
    chooseDetailOption("New task scope", /Every cycle/);
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

    fireEvent.click(screen.getByRole("button", { name: /Mind/ }));
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

  it("removes one durable agent task from the mind tab", async () => {
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

    fireEvent.click(screen.getByRole("button", { name: /Mind/ }));
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
