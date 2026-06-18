import { describe, it, expect } from "vitest";
import {
  statusKind,
  scheduleAgentSlug,
  scheduleJobTitle,
  rosterTriggers,
  buildFleet,
  filterFleetEntities,
  fleetCensus,
  fleetAgentAuthorityLabel,
  fleetAgentCapabilityLabel,
  fleetEntityNeedsRepair,
  fleetAgentHierarchyLabel,
  fleetAgentIdentityCardSummary,
  fleetAgentResilienceLabel,
  fleetAgentTaskContractLabel,
  type ApiProfile,
  type ApiOrder,
  type ApiSchedule,
  type ApiWorkflow,
  type FleetRun,
} from "@/lib/fleet";

describe("statusKind", () => {
  it("buckets the daemon's run statuses", () => {
    expect(statusKind("running")).toBe("running");
    expect(statusKind("in_progress")).toBe("running");
    expect(statusKind("completed")).toBe("done");
    expect(statusKind("ok")).toBe("done");
    expect(statusKind("failed")).toBe("failed");
    expect(statusKind("cancelled")).toBe("failed");
    expect(statusKind("abandoned")).toBe("other");
    expect(statusKind(undefined)).toBe("other");
  });
});

describe("scheduleAgentSlug", () => {
  it("extracts legacy --agent bindings from old cadence intent text", () => {
    expect(scheduleAgentSlug("run the digest --agent researcher")).toBe("researcher");
    expect(scheduleAgentSlug("--agent=ops-watcher check disks")).toBe("ops-watcher");
    expect(scheduleAgentSlug("plain legacy task")).toBe("");
    expect(scheduleAgentSlug(undefined)).toBe("");
  });
});

const profiles: ApiProfile[] = [
  { slug: "researcher", name: "Researcher", model: "m1", enabled: true },
  { slug: "ops", enabled: true },
  { slug: "idle-bot", enabled: false },
  { slug: "ghost", enabled: true, retired: true },
];
const orders: ApiOrder[] = [
  {
    id: "ord1",
    name: "Nightly digest",
    enabled: true,
    agent: "researcher",
    triggers: [
      { type: "cron", schedule: "0 9 * * *" },
      { type: "event", subject: "task.failed" },
    ],
  },
  // Disabled order must not arm anyone.
  { id: "ord2", enabled: false, agent: "ops", triggers: [{ type: "cron", schedule: "* * * * *" }] },
];
const schedules: ApiSchedule[] = [
  { id: "sch1", intent: "summarize inbox --agent ops", cadence: "daily 09:00", enabled: true, next_run_unix: 1000, fires: 4 },
  { id: "sch2", intent: "noop", agent: "researcher", cadence: "hourly", enabled: true },
  { id: "sch3", intent: "Nightly label", target: "workflow", workflow: "nightly-sync", agent: "ops", model: "gpt-5", cadence: "daily 03:00", enabled: true },
  { id: "sch4", target: "system_task", system_task: "catalog_sync", cadence: "daily 04:00", enabled: true },
  { id: "sch5", target: "tool", tool: "shell", cadence: "hourly", enabled: true },
];
const workflows: ApiWorkflow[] = [
  { id: "wf1", name: "deploy", enabled: true, trigger_kind: "webhook" },
  { id: "wf2", name: "draft", enabled: true, trigger_kind: "manual" },
];
const runs: FleetRun[] = [
  { correlation_id: "r1", status: "running", agent: "ops", started_unix_ms: 500 },
  { correlation_id: "r2", status: "completed", agent: "researcher", started_unix_ms: 300 },
];

describe("rosterTriggers", () => {
  it("folds wake sources, ignoring disabled orders/schedules", () => {
    const res = rosterTriggers("researcher", orders, schedules);
    // ord1 cron + ord1 event + sch2 structured agent cadence
    expect(res.map((t) => t.mode)).toEqual(["cron", "event", "cadence"]);
    expect(res[0].via).toBe("Nightly digest");
    expect(res[2].via).toBe("sch2");

    const ops = rosterTriggers("ops", orders, schedules);
    // ord2 disabled; sch1 legacy fallback + sch3 structured workflow schedule running as ops
    expect(ops.map((t) => t.mode)).toEqual(["cadence", "cadence"]);
    expect(ops[0].via).toBe("sch1");
    expect(ops[1].via).toBe("sch3");
    expect(ops[1].needs).toContain('Runs workflow "nightly-sync"');
  });
});

describe("scheduleJobTitle", () => {
  it("names schedules by typed action before optional label text", () => {
    expect(scheduleJobTitle({ id: "s1", target: "workflow", workflow: "nightly-sync", intent: "Nightly label" })).toBe("Run workflow nightly-sync");
    expect(scheduleJobTitle({ id: "s2", target: "system_task", system_task: "catalog_sync" })).toBe("Run system task catalog_sync");
    expect(scheduleJobTitle({ id: "s3", target: "tool", tool: "shell" })).toBe("Run tool shell");
    expect(scheduleJobTitle({ id: "s4", agent: "ops", intent: "check disks" })).toBe("Wake ops: check disks");
    expect(scheduleJobTitle({ id: "s5", intent: "legacy task" })).toBe("legacy task");
  });
});

describe("roster passport labels", () => {
  it("summarizes identity hierarchy and managed sub-agents", () => {
    expect(fleetAgentHierarchyLabel({ system: true })).toBe("system guardian");
    expect(fleetAgentHierarchyLabel({ kind: "subagent", parent_agent: "lead" })).toBe("sub-agent of lead");
    expect(fleetAgentHierarchyLabel({ direct_callable: false, owner_agent: "owner" })).toBe("sub-agent of owner");
    expect(fleetAgentHierarchyLabel({ owner_agent: "team" })).toBe("direct · owner team");
    expect(fleetAgentHierarchyLabel({})).toBe("direct agent");
  });

  it("summarizes lifecycle, task contract, capability and repair posture", () => {
    expect(
      fleetAgentTaskContractLabel({
        lifecycle: { mode: "cycle", completed_cycles: 2, max_cycles: 5 },
        tasklist: [
          { title: "check inbox", scope: "cycle", status: "doing" },
          { title: "finish report", scope: "total", status: "blocked" },
          { title: "old", scope: "total", status: "retired" },
        ],
      }),
    ).toBe("cycle 2/5 · 1 cycle/1 total · 1 doing · 1 blocked");
    expect(fleetAgentTaskContractLabel({ lifecycle: { mode: "retire_on_complete" }, tasklist: [{ title: "ship", scope: "total", status: "done" }] })).toBe(
      "one-shot · 0 cycle/1 total · 1 done",
    );
    expect(fleetAgentCapabilityLabel({ tool_allow: ["memory", "shell"], tool_deny: ["notify"], trust_ceiling: "L2" })).toBe("allow 2 / deny 1 · L2");
    expect(fleetAgentCapabilityLabel({ trust_ceiling: "L3" })).toBe("trust L3");
    expect(fleetAgentCapabilityLabel({ system: true, tool_deny: ["memory"], trust_ceiling: "L4" })).toBe("system quiet · deny 1 · L4");
    expect(fleetAgentAuthorityLabel({ tool_allow: ["memory", "shell"], trust_ceiling: "L2" })).toBe("high-impact allow: shell · L2");
    expect(fleetAgentAuthorityLabel({ tool_allow: ["memory"], trust_ceiling: "L3" })).toBe("allowlisted 1 · L3");
    expect(fleetAgentAuthorityLabel({ system: true, tool_deny: ["memory"], trust_ceiling: "L4" })).toBe("system-governed high-impact · notify gated · L4");
    expect(fleetAgentAuthorityLabel({ tool_deny: ["shell"], trust_ceiling: "L4" })).toBe("broad minus shell · L4");
    expect(fleetAgentAuthorityLabel({})).toBe("open high-impact tools · L4");
    expect(fleetAgentResilienceLabel({ retry_policy: { max_attempts: 3 }, health_policy: { doctor_agent: "doctor" }, self_repair: { enabled: true, max_attempts: 2 } })).toBe(
      "retry 3x · doctor doctor · self-repair 2x",
    );
  });

  it("builds one stable identity-card summary for fleet cards", () => {
    expect(
      fleetAgentIdentityCardSummary({
        slug: "worker",
        enabled: true,
        kind: "subagent",
        parent_agent: "lead",
        tool_deny: ["shell"],
        lifecycle: { mode: "cycle", completed_cycles: 1, max_cycles: 3 },
        tasklist: [{ title: "check queue", scope: "cycle", status: "doing" }],
        retry_policy: { max_attempts: 3 },
      }, "armed"),
    ).toEqual({
      label: "sub-agent of lead · sleeping until trigger",
      detail: "cycle 1/3 · 1 cycle/0 total · 1 doing · broad minus shell · L4 · retry 3x",
      tone: "good",
    });
    expect(
      fleetAgentIdentityCardSummary({ slug: "ops", enabled: true }, "running", true),
    ).toMatchObject({
      label: "direct agent · awake",
      tone: "accent",
    });
    expect(
      fleetAgentIdentityCardSummary({ slug: "guardian", enabled: false, retired: true, system: true }, "retired"),
    ).toEqual({
      label: "system guardian · graveyard",
      detail: "graveyard · revive/remove",
      tone: "muted",
    });
    expect(
      fleetAgentIdentityCardSummary({ slug: "guardian", enabled: true, system: true, tool_deny: ["memory"] }, "armed"),
    ).toEqual({
      label: "system guardian · sleeping until trigger",
      detail: "persistent · no tasks · system-governed high-impact · notify gated · L4 · manual recovery",
      tone: "good",
    });
    expect(fleetAgentIdentityCardSummary({ slug: "open", enabled: true }).tone).toBe("warn");
  });
});

describe("buildFleet", () => {
  const fleet = buildFleet(profiles, orders, schedules, workflows, runs, { running: true, cadence_sec: 60 });
  const byKey = (k: string) => fleet.find((e) => e.key === k)!;

  it("carries the System guardian flag onto roster entities", () => {
    const f = buildFleet(
      [
        { slug: "guardian-health", enabled: true, system: true, kind: "system" },
        { slug: "worker", enabled: true, kind: "subagent", managed: true },
        { slug: "owned-worker", enabled: true, direct_callable: false, owner_agent: "lead" },
        { slug: "plain", enabled: true, kind: "custom" },
      ],
      [],
      [],
      [],
      [],
    );
    expect(f.find((e) => e.slug === "guardian-health")!.system).toBe(true);
    expect(f.find((e) => e.slug === "guardian-health")!.agentClass).toBe("system");
    expect(f.find((e) => e.slug === "worker")!.agentClass).toBe("subagent");
    expect(f.find((e) => e.slug === "owned-worker")!.agentClass).toBe("subagent");
    expect(f.find((e) => e.slug === "plain")!.agentClass).toBe("custom");
    expect(f.find((e) => e.slug === "plain")!.system).toBeFalsy();
  });

  it("includes every archetype + the three system engines", () => {
    expect(fleet.filter((e) => e.kind === "roster")).toHaveLength(4); // retired kept
    expect(fleet.filter((e) => e.kind === "standing")).toHaveLength(2);
    expect(fleet.filter((e) => e.kind === "schedule")).toHaveLength(5);
    expect(fleet.filter((e) => e.kind === "workflow")).toHaveLength(2);
    expect(fleet.filter((e) => e.kind === "system").map((e) => e.slug).sort()).toEqual(["overseer", "pulse", "reaper"]);
  });

  it("labels Pulse cadence from the daemon's cadence_ms", () => {
    const f = buildFleet([], [], [], [], [], { running: true, cadence_ms: 60000 });
    const pulse = f.find((e) => e.key === "system:pulse")!;
    expect(pulse.triggers[0].label).toBe("every 60s");
    expect(pulse.running).toBe(true);
  });

  it("derives roster state + triggers", () => {
    const ops = byKey("roster:ops");
    expect(ops.running).toBe(true);
    expect(ops.state).toBe("running");
    expect(ops.triggers.map((t) => t.via)).toEqual(["sch1", "sch3"]);

    const res = byKey("roster:researcher");
    expect(res.running).toBe(false);
    expect(res.state).toBe("armed");
    expect(res.lastRunMs).toBe(300);
    expect(res.triggers.map((t) => t.mode)).toEqual(["cron", "event", "cadence"]);

    // No wake sources ⇒ a single "delegation" trigger explaining how to run it.
    const idle = byKey("roster:idle-bot");
    expect(idle.state).toBe("paused");
    expect(idle.triggers).toHaveLength(1);
    expect(idle.triggers[0].mode).toBe("delegation");

    const ghost = byKey("roster:ghost");
    expect(ghost.state).toBe("retired");
    expect(ghost.triggers).toHaveLength(0);
  });

  it("derives workflow + schedule triggers", () => {
    expect(byKey("workflow:wf1").triggers[0].mode).toBe("webhook");
    expect(byKey("workflow:wf2").triggers[0].mode).toBe("manual");
    expect(byKey("workflow:wf2").state).toBe("manual");

    const sc = byKey("schedule:sch1");
    expect(sc.triggers[0].mode).toBe("cadence");
    expect(sc.nextRunMs).toBe(1_000_000); // unix → ms
    expect(sc.fires).toBe(4);
    expect(byKey("schedule:sch3").name).toBe("Run workflow nightly-sync");
    expect(byKey("schedule:sch3").description).toContain("label: Nightly label");
    expect(byKey("schedule:sch3").description).toContain("as ops");
    expect(byKey("schedule:sch3").description).toContain("model: gpt-5");
    expect(byKey("schedule:sch3").triggers[0].needs).toContain('Runs workflow "nightly-sync"');
    expect(byKey("schedule:sch4").name).toBe("Run system task catalog_sync");
    expect(byKey("schedule:sch4").triggers[0].needs).toContain('Runs system task "catalog_sync"');
    expect(byKey("schedule:sch5").name).toBe("Run tool shell");
    expect(byKey("schedule:sch5").triggers[0].needs).toContain('Runs tool "shell"');
    expect(byKey("schedule:sch2").state).toBe("armed");
  });

  it("sorts running first, then armed, then by kind", () => {
    // ops is the only running entity (besides system overseer) → near the front.
    const firstRoster = fleet.find((e) => e.kind === "roster")!;
    expect(firstRoster.key).toBe("roster:ops");
  });
});

describe("fleetCensus", () => {
  it("tallies by kind + running/armed", () => {
    const fleet = buildFleet(profiles, orders, schedules, workflows, runs, { running: true, cadence_sec: 60 });
    const c = fleetCensus(fleet);
    expect(c.roster).toBe(4);
    expect(c.standing).toBe(2);
    expect(c.schedule).toBe(5);
    expect(c.workflow).toBe(2);
    expect(c.system).toBe(3);
    expect(c.total).toBe(16);
    expect(c.running).toBeGreaterThanOrEqual(2); // ops run + overseer
    expect(c.graveyard).toBe(1);
    expect(c.directAgents).toBe(4);
    expect(c.subagents).toBe(0);
    expect(c.repair).toBe(0);
  });
});

describe("filterFleetEntities", () => {
  it("makes the graveyard a first-class fleet filter", () => {
    const fleet = buildFleet(profiles, orders, schedules, workflows, runs, { running: true, cadence_sec: 60 });
    expect(filterFleetEntities(fleet, "graveyard").map((e) => e.slug)).toEqual(["ghost"]);
    expect(filterFleetEntities(fleet, "graveyard", "gho").map((e) => e.slug)).toEqual(["ghost"]);
    expect(filterFleetEntities(fleet, "graveyard", "research").map((e) => e.slug)).toEqual([]);
  });

  it("separates direct roster agents from managed sub-agents", () => {
    const fleet = buildFleet(
      [
        { slug: "lead", enabled: true },
        { slug: "worker", enabled: true, kind: "subagent", parent_agent: "lead" },
        { slug: "owned-worker", enabled: true, direct_callable: false, owner_agent: "lead" },
        { slug: "guardian", enabled: true, system: true, kind: "system" },
      ],
      [],
      [],
      [],
      [],
    );
    expect(filterFleetEntities(fleet, "direct").map((e) => e.slug)).toEqual(["lead"]);
    expect(filterFleetEntities(fleet, "subagents").map((e) => e.slug).sort()).toEqual(["owned-worker", "worker"]);
  });

  it("surfaces agents that need repair or have repair work in flight", () => {
    const fleet = buildFleet(
      [
        { slug: "healthy", enabled: true, status: { health_state: "healthy", repair_state: "idle" } },
        { slug: "broken", enabled: true, status: { health_state: "degraded", repair_state: "idle" } },
        { slug: "queued", enabled: true, status: { health_state: "healthy", repair_state: "queued", repair_inflight: 1 } },
        { slug: "retrying", enabled: true, status: { health_state: "healthy", repair_state: "idle", retry_count: 2 } },
      ],
      [],
      [],
      [],
      [],
    );
    expect(fleet.filter(fleetEntityNeedsRepair).map((e) => e.slug).sort()).toEqual(["broken", "queued", "retrying"]);
    expect(filterFleetEntities(fleet, "repair").map((e) => e.slug).sort()).toEqual(["broken", "queued", "retrying"]);
    expect(fleetCensus(fleet).repair).toBe(3);
  });
});
