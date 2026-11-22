import { describe, it, expect } from "vitest";
import {
  incidentActionContext,
  incidentDelegateCandidates,
  incidentForceChainPresets,
  previewIncidentForceChain,
  incidentResolutionDelegateDraft,
  incidentResolutionForceDraft,
  incidentResolutionHistory,
  incidentResolutionPresets,
  incidentMatches,
  incidentMetaFromAutonomy,
  incidentMetaFromEvent,
  incidentRef,
  incidentRootId,
  validateIncidentDelegateTarget,
} from "@/lib/incidents";

describe("incidents helpers", () => {
  it("extracts incident metadata from events and autonomy items", () => {
    expect(
      incidentMetaFromEvent({
        payload: {
          incident_id: "child-1",
          root_incident_id: "root-1",
          parent_incident_id: "root-0",
        },
      } as any),
    ).toEqual({
      incidentId: "child-1",
      rootIncidentId: "root-1",
      parentIncidentId: "root-0",
    });
    expect(
      incidentMetaFromAutonomy({
        incident_id: "child-1",
        root_incident_id: "root-1",
        parent_incident_id: "root-0",
      } as any),
    ).toEqual({
      incidentId: "child-1",
      rootIncidentId: "root-1",
      parentIncidentId: "root-0",
    });
  });

  it("resolves incident refs and matching", () => {
    const meta = {
      incidentId: "child-1",
      rootIncidentId: "root-1",
      parentIncidentId: "root-0",
    };
    expect(incidentRef(meta)).toBe("child-1");
    expect(incidentRootId(meta)).toBe("root-1");
    expect(incidentMatches(meta, "child-1")).toBe(true);
    expect(incidentMatches(meta, "root-1")).toBe(true);
    expect(incidentMatches(meta, "root-0")).toBe(true);
    expect(incidentMatches(meta, "other")).toBe(false);
  });

  it("derives the root agent, active owner, and config issues for an incident", () => {
    const ctx = incidentActionContext(
      [
        {
          category: "doctor",
          agent: "guardian-doctor",
          root_agent: "builder",
          target_agent: "lead",
          ts_unix_ms: 10,
        },
        {
          category: "doctor",
          agent: "lead",
          root_agent: "builder",
          delegate_to: "infra-lead",
          delegated_by: "lead",
          ts_unix_ms: 20,
        },
      ] as any,
      [
        { slug: "builder", owner_agent: "ceo", parent_agent: "lead" },
        { slug: "infra-lead" },
      ],
      [
        {
          payload: {
            agent: "builder",
            issues: ["AGEZT_MAX_ITER: must be an integer"],
          },
        },
      ] as any,
    );
    expect(ctx.rootSlug).toBe("builder");
    expect(ctx.ownerSlug).toBe("infra-lead");
    expect(ctx.configIssues).toEqual(["AGEZT_MAX_ITER: must be an integer"]);
  });

  it("marks forced-chain exhaustion incidents as human-required with a constrained resolution set", () => {
    const ctx = incidentActionContext(
      [
        {
          category: "doctor",
          root_agent: "builder",
          target_agent: "lead",
          phase: "routing_force_exhausted_detected",
          mode: "routing_forced_exhausted",
          routing_task_type: "code",
          routing_task_model_chain: ["gpt-5", "gpt-4.1"],
          routing_force_generation: 3,
          ts_unix_ms: 30,
        },
      ] as any,
      [{ slug: "builder", parent_agent: "lead" }, { slug: "lead", enabled: true }],
      [],
    );
    expect(ctx.humanRequired).toBe(true);
    expect(ctx.policyKind).toBe("routing_force_exhausted");
    expect(ctx.allowedResolutions).toEqual([
      "paused",
      "retired",
      "delegated",
      "force_chain",
    ]);
    expect(ctx.routingTaskType).toBe("code");
    expect(ctx.routingTaskModelChain).toEqual(["gpt-5", "gpt-4.1"]);
    expect(ctx.routingForceGeneration).toBe(3);
    expect(ctx.suppressDoctorRerun).toBe(true);
    expect(ctx.preferOwnerWake).toBe(true);
  });

  it("builds exhausted-routing resolution note presets", () => {
    const presets = incidentResolutionPresets({
      rootSlug: "builder",
      ownerSlug: "lead",
      configIssues: [],
      humanRequired: true,
      policyKind: "routing_force_exhausted",
      policyLabel: "Forced-chain exhaustion",
      policySummary: "owner-forced chain exhausted · task code · chain gpt-5 → gpt-4.1 · generation 3",
      allowedResolutions: ["paused", "retired", "delegated", "force_chain"],
      routingTaskType: "code",
      routingTaskModelChain: ["gpt-5", "gpt-4.1"],
      routingForceGeneration: 3,
      suppressDoctorRerun: true,
      preferOwnerWake: true,
    });
    expect(presets.map((row) => row.key)).toEqual([
      "pause",
      "retire",
      "delegate",
      "force_chain",
    ]);
    expect(presets[3]?.text).toContain("Do not reuse the exhausted chain.");
  });

  it("extracts incident-scoped operator resolution history", () => {
    const rows = incidentResolutionHistory(
      [
        {
          id: "e1",
          subject: "agent.resolve",
          ts_unix_ms: 20,
          payload: {
            incident_id: "child-1",
            root_incident_id: "root-1",
            phase: "completed",
            resolution: "force_chain",
            routing_task_type: "code",
            routing_task_model_chain: ["gpt-4.1", "deepseek-chat"],
            routing_force_generation: 2,
          },
        },
        {
          id: "e2",
          subject: "agent.resolve",
          ts_unix_ms: 10,
          payload: {
            incident_id: "other",
            root_incident_id: "other",
            phase: "requested",
            resolution: "paused",
          },
        },
      ] as any,
      "root-1",
    );
    expect(rows).toHaveLength(1);
    expect(rows[0]).toMatchObject({
      id: "e1",
      phase: "completed",
      resolution: "force_chain",
      routingTaskType: "code",
      routingTaskModelChain: ["gpt-4.1", "deepseek-chat"],
      routingForceGeneration: 2,
    });
  });

  it("builds reusable drafts from resolution history rows", () => {
    expect(
      incidentResolutionForceDraft({
        id: "e1",
        subject: "agent.resolve",
        resolution: "force_chain",
        routingTaskType: "code",
        routingTaskModelChain: ["gpt-4.1", "deepseek-chat"],
        resolutionSummary: "prior chain worked better",
        payload: {},
      }),
    ).toEqual({
      taskType: "code",
      chainText: "gpt-4.1, deepseek-chat",
      summary: "prior chain worked better",
    });
    expect(
      incidentResolutionDelegateDraft({
        id: "e2",
        subject: "agent.resolve",
        resolution: "delegated",
        delegateTo: "infra-lead",
        resolutionSummary: "infra owner should take this",
        payload: {},
      }),
    ).toEqual({
      delegateTo: "infra-lead",
      summary: "infra owner should take this",
    });
  });

  it("rejects delegate targets that loop back into the same ownership path", () => {
    expect(
      validateIncidentDelegateTarget("builder", {
        rootSlug: "builder",
        ownerSlug: "lead",
        profiles: [{ slug: "builder" }, { slug: "lead" }, { slug: "infra-lead" }],
      }),
    ).toMatchObject({
      valid: false,
      reason: "delegate target cannot be the root agent",
    });
    expect(
      validateIncidentDelegateTarget("lead", {
        rootSlug: "builder",
        ownerSlug: "lead",
        profiles: [{ slug: "builder" }, { slug: "lead" }, { slug: "infra-lead" }],
      }),
    ).toMatchObject({
      valid: false,
      reason: "delegate target already owns this incident",
    });
    expect(
      validateIncidentDelegateTarget("infra-lead", {
        rootSlug: "builder",
        ownerSlug: "lead",
        profiles: [{ slug: "builder" }, { slug: "lead" }, { slug: "infra-lead" }],
      }),
    ).toMatchObject({
      valid: true,
      normalizedTarget: "infra-lead",
    });
    expect(
      validateIncidentDelegateTarget("graveyard", {
        rootSlug: "builder",
        ownerSlug: "lead",
        profiles: [{ slug: "builder" }, { slug: "lead" }, { slug: "graveyard", retired: true }],
      }),
    ).toMatchObject({
      valid: false,
      reason: "delegate target is retired",
    });
    expect(
      validateIncidentDelegateTarget("worker", {
        rootSlug: "builder",
        ownerSlug: "lead",
        profiles: [{ slug: "builder" }, { slug: "lead" }, { slug: "worker", direct_callable: false }],
      }),
    ).toMatchObject({
      valid: false,
      reason: "delegate target is a managed sub-agent",
    });
  });

  it("lists valid delegate candidates before blocked roster targets", () => {
    const rows = incidentDelegateCandidates(
      [
        { slug: "builder" },
        { slug: "ops-lead", enabled: true },
        { slug: "lead" },
        { slug: "graveyard", retired: true },
        { slug: "worker", direct_callable: false },
        { slug: "infra-lead", name: "Infra Lead", enabled: true },
        { slug: "analyst", enabled: false },
      ],
      { rootSlug: "builder", ownerSlug: "lead", preferredSlugs: ["ops-lead", "infra-lead"] },
    );
    expect(rows[0]).toMatchObject({ slug: "ops-lead", valid: true, preferred: true });
    expect(rows[1]).toMatchObject({ slug: "infra-lead", valid: true, name: "Infra Lead", preferred: true });
    expect(rows[2]).toMatchObject({ slug: "analyst", valid: true, enabled: false });
    expect(rows.slice(3)).toEqual(
      expect.arrayContaining([
        expect.objectContaining({ slug: "builder", valid: false }),
        expect.objectContaining({ slug: "lead", valid: false }),
        expect.objectContaining({ slug: "graveyard", valid: false, reason: "delegate target is retired" }),
        expect.objectContaining({ slug: "worker", valid: false, reason: "delegate target is a managed sub-agent" }),
      ]),
    );
  });

  it("orders delegate candidates by lower live load after preferred/enabled gates", () => {
    const rows = incidentDelegateCandidates(
      [
        { slug: "builder" },
        { slug: "lead" },
        {
          slug: "busy-owner",
          enabled: true,
          status: { health_state: "healthy", escalation_open_count: 3 },
        },
        {
          slug: "degraded-owner",
          enabled: true,
          status: { health_state: "degraded", escalation_open_count: 0 },
        },
        {
          slug: "quiet-owner",
          enabled: true,
          status: { health_state: "healthy", escalation_open_count: 0 },
        },
      ],
      { rootSlug: "builder", ownerSlug: "lead" },
    ).filter((row) => row.valid);
    expect(rows.map((row) => row.slug)).toEqual([
      "quiet-owner",
      "degraded-owner",
      "busy-owner",
    ]);
  });

  it("builds unique force-chain presets from resolution history", () => {
    const rows = incidentResolutionHistory(
      [
        {
          id: "e1",
          subject: "agent.resolve",
          ts_unix_ms: 20,
          payload: {
            incident_id: "root-1",
            root_incident_id: "root-1",
            phase: "completed",
            resolution: "force_chain",
            resolution_summary: "try a tighter code chain",
            routing_task_type: "code",
            routing_task_model_chain: ["gpt-4.1", "deepseek-chat"],
            routing_force_generation: 2,
          },
        },
        {
          id: "e2",
          subject: "agent.resolve",
          ts_unix_ms: 10,
          payload: {
            incident_id: "root-1",
            root_incident_id: "root-1",
            phase: "completed",
            resolution: "force_chain",
            resolution_summary: "duplicate row",
            routing_task_type: "code",
            routing_task_model_chain: ["gpt-4.1", "deepseek-chat"],
            routing_force_generation: 1,
          },
        },
        {
          id: "e3",
          subject: "agent.resolve",
          ts_unix_ms: 9,
          payload: {
            incident_id: "root-1",
            root_incident_id: "root-1",
            phase: "completed",
            resolution: "force_chain",
            resolution_summary: "route research elsewhere",
            routing_task_type: "research",
            routing_task_model_chain: ["gpt-5", "grok-4"],
            routing_force_generation: 1,
          },
        },
      ] as any,
      "root-1",
    );
    expect(incidentForceChainPresets(rows)).toEqual([
      {
        key: "code::gpt-4.1, deepseek-chat",
        taskType: "code",
        chainText: "gpt-4.1, deepseek-chat",
        summary: "try a tighter code chain",
        generation: 2,
      },
      {
        key: "research::gpt-5, grok-4",
        taskType: "research",
        chainText: "gpt-5, grok-4",
        summary: "route research elsewhere",
        generation: 1,
      },
    ]);
  });

  it("rejects force-chain proposals that exactly match the exhausted chain and shows a diff otherwise", () => {
    expect(
      previewIncidentForceChain("code", ["gpt-5", "gpt-4.1"], "gpt-5, gpt-4.1"),
    ).toMatchObject({
      valid: false,
      sameAsCurrent: true,
      reason:
        "proposed chain matches the exhausted chain; branch to a genuinely new chain",
    });
    expect(
      previewIncidentForceChain(
        "code",
        ["gpt-5", "gpt-4.1"],
        "gpt-4.1, deepseek-chat",
      ),
    ).toMatchObject({
      valid: true,
      sameAsCurrent: false,
      proposedChain: ["gpt-4.1", "deepseek-chat"],
      added: ["deepseek-chat"],
      removed: ["gpt-5"],
    });
  });
});
