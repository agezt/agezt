import { describe, expect, it } from "vitest";

import {
  autonomyEventMatches,
  doctorIncidentPhase,
  doctorIncidentGroups,
  doctorIncidentLabel,
  doctorIncidentNodeTitle,
  doctorIncidentSource,
  doctorIncidentSourceLabel,
  doctorIncidentTreeOpsSummary,
  doctorIncidentTrees,
  filterDoctorAutonomy,
} from "@/lib/autonomy";

describe("autonomy helpers", () => {
  it("treats doctor and operator incident subjects plus curated kinds as autonomy events", () => {
    expect(
      autonomyEventMatches({ subject: "doctor.auto_repair", kind: "info" }),
    ).toBe(true);
    expect(autonomyEventMatches({ subject: "agent.repair", kind: "info" })).toBe(
      true,
    );
    expect(autonomyEventMatches({ subject: "agent.wake", kind: "info" })).toBe(
      true,
    );
    expect(
      autonomyEventMatches({ subject: "agent.resolve", kind: "info" }),
    ).toBe(true);
    expect(autonomyEventMatches({ kind: "schedule.fired" })).toBe(true);
    expect(autonomyEventMatches({ kind: "task.received" })).toBe(false);
  });

  it("keeps only doctor rows for the doctor strip", () => {
    const got = filterDoctorAutonomy(
      [
        {
          seq: 1,
          kind: "info",
          category: "doctor",
          title: "a doctor run was queued",
        },
        {
          seq: 2,
          kind: "schedule.fired",
          category: "schedule",
          title: "a schedule fired",
        },
        {
          seq: 3,
          kind: "info",
          category: "doctor",
          title: "a config repair failed",
        },
      ] as any,
      10,
    );
    expect(got.map((row) => row.seq)).toEqual([1, 3]);
  });

  it("groups doctor incidents by root agent and hop depth", () => {
    const got = doctorIncidentGroups(
      [
        {
          seq: 1,
          kind: "info",
          category: "doctor",
          title: "queued",
          root_agent: "builder",
          chain_depth: 0,
          ts_unix_ms: 10,
        },
        {
          seq: 2,
          kind: "info",
          category: "doctor",
          title: "delegated",
          root_agent: "builder",
          chain_depth: 1,
          ts_unix_ms: 20,
        },
        {
          seq: 3,
          kind: "info",
          category: "doctor",
          title: "woke",
          root_agent: "builder",
          chain_depth: 1,
          ts_unix_ms: 30,
        },
        {
          seq: 4,
          kind: "info",
          category: "schedule",
          title: "skip me",
          ts_unix_ms: 40,
        },
      ] as any,
      10,
    );
    expect(got).toHaveLength(2);
    expect(got[0]?.root).toBe("builder");
    expect(got[0]?.depth).toBe(1);
    expect(got[0]?.items.map((row) => row.seq)).toEqual([2, 3]);
    expect(got[1]?.depth).toBe(0);
  });

  it("formats doctor incident lineage labels", () => {
    expect(
      doctorIncidentLabel({
        delegated_by: "lead",
        root_agent: "builder",
        chain_depth: 2,
        delegate_to: "infra-lead",
      }),
    ).toBe("delegated by lead · root builder · hop 2 · to infra-lead");
  });

  it("builds doctor incident trees from incident and parent ids", () => {
    const got = doctorIncidentTrees(
      [
        {
          seq: 1,
          kind: "info",
          category: "doctor",
          title: "a config repair failed",
          root_agent: "builder",
          incident_id: "root-1",
          root_incident_id: "root-1",
          chain_depth: 0,
          ts_unix_ms: 10,
          agent: "builder",
        },
        {
          seq: 2,
          kind: "info",
          category: "doctor",
          title: "a delegated follow-up was queued",
          root_agent: "builder",
          incident_id: "child-1",
          root_incident_id: "root-1",
          parent_incident_id: "root-1",
          chain_depth: 1,
          ts_unix_ms: 20,
          agent: "builder",
          delegate_to: "infra-lead",
        },
      ] as any,
      10,
    );
    expect(got).toHaveLength(1);
    expect(got[0]?.roots).toHaveLength(1);
    expect(got[0]?.roots[0]?.id).toBe("root-1");
    expect(got[0]?.roots[0]?.children[0]?.id).toBe("child-1");
    expect(got[0]?.roots[0]?.children[0]?.parentId).toBe("root-1");
  });

  it("summarizes incident tree operation state", () => {
    const [tree] = doctorIncidentTrees(
      [
        {
          seq: 1,
          subject: "doctor.auto_repair",
          kind: "info",
          category: "doctor",
          title: "routing failed",
          root_agent: "builder",
          incident_id: "root-1",
          root_incident_id: "root-1",
          chain_depth: 0,
          phase: "failed",
          mode: "routing",
          ts_unix_ms: 10,
        },
        {
          seq: 2,
          subject: "doctor.auto_repair",
          kind: "info",
          category: "doctor",
          title: "delegate queued",
          root_agent: "builder",
          incident_id: "child-1",
          root_incident_id: "root-1",
          parent_incident_id: "root-1",
          chain_depth: 1,
          phase: "delegation_queued",
          delegate_to: "infra-lead",
          ts_unix_ms: 20,
        },
        {
          seq: 3,
          subject: "agent.wake",
          kind: "info",
          category: "doctor",
          title: "operator woke owner",
          root_agent: "builder",
          incident_id: "child-1",
          root_incident_id: "root-1",
          parent_incident_id: "root-1",
          chain_depth: 1,
          phase: "requested",
          target_agent: "infra-lead",
          ts_unix_ms: 30,
        },
      ] as any,
      10,
    );

    expect(doctorIncidentTreeOpsSummary(tree!)).toMatchObject({
      label: "needs owner",
      tone: "bad",
      hops: 2,
      maxDepth: 1,
      operatorEvents: 1,
      failureEvents: 1,
      latestPhase: "requested",
    });
    expect(doctorIncidentTreeOpsSummary(tree!).detail).toBe(
      "2 hops / depth 1 / latest requested / 1 operator / 1 failure",
    );
  });

  it("uses delegated target as the doctor incident node title for child hops", () => {
    expect(
      doctorIncidentNodeTitle({
        id: "child-1",
        rootId: "root-1",
        rootAgent: "builder",
        depth: 1,
        items: [],
        children: [],
        latest: {
          seq: 1,
          kind: "info",
          category: "doctor",
          title: "a delegated agent was woken",
          delegate_to: "infra-lead",
          target_agent: "infra-lead",
        },
      } as any),
    ).toBe("infra-lead");
  });

  it("distinguishes doctor and operator incident provenance", () => {
    expect(doctorIncidentSource({ subject: "doctor.auto_repair" })).toBe(
      "doctor",
    );
    expect(doctorIncidentSource({ subject: "agent.wake" })).toBe("operator");
    expect(
      doctorIncidentSourceLabel({
        subject: "doctor.auto_repair",
        mode: "degraded",
      }),
    ).toBe("doctor");
    expect(
      doctorIncidentSourceLabel({
        subject: "agent.repair",
        phase: "requested",
      }),
    ).toBe("operator");
    expect(
      doctorIncidentSourceLabel({
        subject: "agent.resolve",
        phase: "completed",
      }),
    ).toBe("operator");
  });

  it("maps incident phases to compact state badges", () => {
    expect(
      doctorIncidentPhase({
        subject: "doctor.auto_repair",
        phase: "routing_unstable_detected",
        mode: "routing_unstable",
      }),
    ).toEqual({ label: "unstable", tone: "bad" });
    expect(
      doctorIncidentPhase({
        subject: "doctor.auto_repair",
        phase: "routing_force_exhausted_detected",
        mode: "routing_forced_exhausted",
      }),
    ).toEqual({ label: "exhausted", tone: "bad" });
    expect(
      doctorIncidentPhase({
        subject: "doctor.auto_repair",
        phase: "routing_forced_failed_detected",
        mode: "routing_forced_failed",
      }),
    ).toEqual({ label: "forced failed", tone: "bad" });
    expect(
      doctorIncidentPhase({
        subject: "doctor.auto_repair",
        phase: "queued",
        mode: "misconfigured",
      }),
    ).toEqual({ label: "queued", tone: "warn" });
    expect(
      doctorIncidentPhase({
        subject: "doctor.auto_repair",
        phase: "queued",
        mode: "routing",
      }),
    ).toEqual({ label: "routing", tone: "warn" });
    expect(
      doctorIncidentPhase({
        subject: "doctor.auto_repair",
        phase: "routing_rollback_queued",
        mode: "routing",
      }),
    ).toEqual({ label: "rollback", tone: "warn" });
    expect(
      doctorIncidentPhase({
        subject: "doctor.auto_repair",
        phase: "completed",
        mode: "degraded",
      }),
    ).toEqual({ label: "repaired", tone: "good" });
    expect(
      doctorIncidentPhase({
        subject: "doctor.auto_repair",
        phase: "completed",
        mode: "routing",
      }),
    ).toEqual({ label: "rerouted", tone: "good" });
    expect(
      doctorIncidentPhase({
        subject: "doctor.auto_repair",
        phase: "routing_rollback_completed",
        mode: "routing",
      }),
    ).toEqual({ label: "rolled back", tone: "good" });
    expect(
      doctorIncidentPhase({
        subject: "doctor.auto_repair",
        phase: "resolution_applied",
      }),
    ).toEqual({ label: "applied", tone: "good" });
    expect(
      doctorIncidentPhase({
        subject: "agent.wake",
        phase: "requested",
      }),
    ).toEqual({ label: "requested", tone: "accent" });
    expect(
      doctorIncidentPhase({
        subject: "doctor.auto_repair",
        mode: "routing",
        phase: "failed",
      }),
    ).toEqual({ label: "routing failed", tone: "bad" });
    expect(
      doctorIncidentPhase({
        subject: "doctor.auto_repair",
        mode: "routing",
        phase: "routing_rollback_failed",
      }),
    ).toEqual({ label: "rollback failed", tone: "bad" });
    expect(
      doctorIncidentPhase({
        subject: "doctor.auto_repair",
        phase: "delegation_failed",
      }),
    ).toEqual({ label: "delegate failed", tone: "bad" });
  });
});
