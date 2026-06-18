import { describe, expect, it } from "vitest";
import { applyAgentLivePatches, reduceAgentLivePatchMap, shouldReloadAgentCatalog } from "@/lib/agentlive";
import type { AgentRuntimeStatus } from "@/lib/agentdetail";

describe("shouldReloadAgentCatalog", () => {
  it("reloads on roster, task, and auto-repair events", () => {
    expect(shouldReloadAgentCatalog({ kind: "roster.updated" })).toBe(true);
    expect(shouldReloadAgentCatalog({ kind: "task.failed" })).toBe(true);
    expect(shouldReloadAgentCatalog({ subject: "doctor.auto_repair" })).toBe(true);
    expect(shouldReloadAgentCatalog({ kind: "tool.invoked" })).toBe(false);
  });
});

describe("reduceAgentLivePatchMap", () => {
  it("tracks queued and completed repair events", () => {
    let state = reduceAgentLivePatchMap({}, {
      subject: "doctor.auto_repair",
      ts_unix_ms: 100,
      payload: {
        target_agent: "builder",
        mode: "degraded",
        phase: "queued",
        self_repair_attempt: 2,
        self_repair_max_attempts: 3,
        incident_id: "inc-child-12345678",
        root_incident_id: "inc-root-12345678",
        parent_incident_id: "inc-parent-12345678",
        root_agent: "architect",
        chain_depth: 1,
      },
    });
    expect(state.builder?.status?.repair_state).toBe("queued");
    expect(state.builder?.status?.repair_inflight).toBe(1);
    expect(state.builder?.status?.repair_mode).toBe("degraded");
    expect(state.builder?.status?.repair_label).toBe("doctor queued");
    expect(state.builder?.status?.repair_self_attempt).toBe(2);
    expect(state.builder?.status?.repair_self_max_attempts).toBe(3);
    expect(state.builder?.status?.repair_incident_id).toBe("inc-child-12345678");
    expect(state.builder?.status?.repair_root_incident_id).toBe("inc-root-12345678");
    expect(state.builder?.status?.repair_parent_incident_id).toBe("inc-parent-12345678");
    expect(state.builder?.status?.repair_root_agent).toBe("architect");
    expect(state.builder?.status?.repair_chain_depth).toBe(1);

    state = reduceAgentLivePatchMap(state, {
      subject: "doctor.auto_repair",
      ts_unix_ms: 200,
      payload: { agent: "builder", phase: "completed" },
    });
    expect(state.builder?.status?.repair_state).toBe("completed");
    expect(state.builder?.status?.repair_inflight).toBe(0);
    expect(state.builder?.status?.repair_incident_id).toBe("inc-child-12345678");
  });

  it("tracks roster retire state immediately", () => {
    const state = reduceAgentLivePatchMap({}, {
      kind: "roster.updated",
      payload: { slug: "builder", retired: true, enabled: false },
    });
    expect(state.builder?.retired).toBe(true);
    expect(state.builder?.enabled).toBe(false);
    expect(state.builder?.status?.health_state).toBe("retired");
  });

  it("turns task lifecycle events into immediate running and sleeping status", () => {
    let state = reduceAgentLivePatchMap({}, {
      kind: "task.received",
      correlation_id: "corr-1",
      ts_unix_ms: 1_000,
      actor: "agent-run-123",
      payload: {
        agent: "builder",
        intent: "ship the patch",
        model: "gpt-5",
        wake_source: "schedule",
        schedule_id: "sched-1",
      },
    });
    expect(state.builder?.status).toMatchObject({
      active_run_count: 1,
      active_correlation_id: "corr-1",
      active_intent: "ship the patch",
      active_model: "gpt-5",
      active_phase: "starting",
      operational_state: "running",
      operational_label: "starting",
      active_wake_source: "schedule",
      active_schedule_id: "sched-1",
    });

    state = reduceAgentLivePatchMap(state, {
      kind: "tool.invoked",
      correlation_id: "corr-1",
      ts_unix_ms: 1_500,
      actor: "agent-run-123",
      payload: { tool: "shell", iter: 2 },
    });
    expect(state.builder?.status).toMatchObject({
      active_phase: "using tool",
      active_tool: "shell",
      active_iter: 2,
      active_detail: "iter 2 · shell",
      operational_state: "running",
    });

    state = reduceAgentLivePatchMap(state, {
      kind: "task.completed",
      correlation_id: "corr-1",
      ts_unix_ms: 2_000,
      actor: "agent-run-123",
      payload: {},
    });
    expect(state.builder?.status).toMatchObject({
      active_run_count: 0,
      active_phase: "completed",
      operational_state: "sleeping",
      operational_label: "sleeping",
      last_activity_summary: "run completed",
    });
  });

  it("does not let an old terminal event close a newer live correlation", () => {
    const running = reduceAgentLivePatchMap({}, {
      kind: "task.received",
      correlation_id: "new-run",
      payload: { agent: "builder", intent: "current" },
    });
    const got = reduceAgentLivePatchMap(running, {
      kind: "task.completed",
      correlation_id: "old-run",
      payload: { agent: "builder" },
    });
    expect(got.builder?.status?.active_correlation_id).toBe("new-run");
    expect(got.builder?.status?.operational_state).toBe("running");
  });
});

describe("applyAgentLivePatches", () => {
  it("merges live patch status into the list and filters removed profiles", () => {
    const got = applyAgentLivePatches(
      [
        { slug: "builder", enabled: true, status: { repair_inflight: 0 } as AgentRuntimeStatus },
        { slug: "ghost", enabled: true },
      ],
      {
        builder: { status: { repair_inflight: 1, repair_state: "queued" } },
        ghost: { removed: true },
      },
    );
    expect(got).toHaveLength(1);
    expect(got[0].status?.repair_inflight).toBe(1);
    expect(got[0].status?.repair_state).toBe("queued");
  });
});
