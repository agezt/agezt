// @vitest-environment jsdom
import { describe, expect, it } from "vitest";

import { repairReadinessPassport, stripForEdit } from "@/components/AgentRepair";
import type { AgentProfile } from "@/views/Roster";

describe("stripForEdit", () => {
  it("preserves hierarchy, resilience, lifecycle, tasklist, and policy fields", () => {
    const profile: AgentProfile = {
      id: "01",
      slug: "ops",
      name: "Ops",
      soul: "Operate.",
      instructions: ["stay quiet"],
      model: "gpt-5",
      fallbacks: ["gpt-4.1"],
      task_type: "ops",
      max_cost_mc: 100,
      max_daily_mc: 500,
      memory_scope: "agent:ops",
      workdir: "agents/ops",
      owner_agent: "lead",
      parent_agent: "lead",
      direct_callable: false,
      retry_policy: { max_attempts: 3, backoff: "exponential" },
      health_policy: { doctor_agent: "guardian-doctor", failure_threshold: 2 },
      self_repair: { enabled: true, max_attempts: 2, escalate_to: "lead" },
      noise_policy: {
        silent_on_success: true,
        disable_memory_writes: true,
        min_notify_severity: "warning",
        min_notify_interval_sec: 3600,
      },
      tool_allow: ["memory"],
      tool_deny: ["notify"],
      trust_ceiling: "L2",
      config_overrides: { AGEZT_MODE: "quiet" },
      lifecycle: { mode: "cycle", completed_cycles: 2, max_cycles: 5 },
      tasklist: [{ id: "task-1", title: "check queue", scope: "cycle", status: "todo" }],
      description: "Ops agent",
      enabled: true,
      retired: false,
    };

    expect(stripForEdit(profile)).toMatchObject({
      name: "Ops",
      soul: "Operate.",
      instructions: ["stay quiet"],
      model: "gpt-5",
      fallbacks: ["gpt-4.1"],
      task_type: "ops",
      max_cost_mc: 100,
      max_daily_mc: 500,
      memory_scope: "agent:ops",
      workdir: "agents/ops",
      owner_agent: "lead",
      parent_agent: "lead",
      direct_callable: false,
      retry_policy: { max_attempts: 3, backoff: "exponential" },
      health_policy: { doctor_agent: "guardian-doctor", failure_threshold: 2 },
      self_repair: { enabled: true, max_attempts: 2, escalate_to: "lead" },
      noise_policy: {
        silent_on_success: true,
        disable_memory_writes: true,
        min_notify_severity: "warning",
        min_notify_interval_sec: 3600,
      },
      tool_allow: ["memory"],
      tool_deny: ["notify"],
      trust_ceiling: "L2",
      config_overrides: { AGEZT_MODE: "quiet" },
      lifecycle: { mode: "cycle", completed_cycles: 2, max_cycles: 5 },
      tasklist: [{ id: "task-1", title: "check queue", scope: "cycle", status: "todo" }],
      description: "Ops agent",
    });
  });
});

describe("repairReadinessPassport", () => {
  it("summarizes repair route, evidence, and wake blockers", () => {
    expect(repairReadinessPassport({
      retry_policy: { max_attempts: 3 },
      health_policy: { doctor_agent: "guardian-doctor" },
      self_repair: { enabled: true, max_attempts: 2, escalate_to: "lead" },
    }, 4)).toEqual({
      value: "ready · 4 evidence",
      detail: "retry 3x · doctor guardian-doctor · self-repair 2x · escalate lead",
      tone: "good",
    });
    expect(repairReadinessPassport({ kind: "subagent", parent_agent: "lead" }, 1)).toEqual({
      value: "manager repair",
      detail: "request repair through lead",
      tone: "warn",
    });
    expect(repairReadinessPassport({ retired: true }, 1)).toEqual({
      value: "repair blocked",
      detail: "revive this agent before requesting repair",
      tone: "bad",
    });
    expect(repairReadinessPassport({}, 0)).toEqual({
      value: "ready · proactive",
      detail: "single attempt · no doctor · self-repair off",
      tone: "warn",
    });
  });
});
