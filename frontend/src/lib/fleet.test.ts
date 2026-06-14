import { describe, it, expect } from "vitest";
import {
  statusKind,
  scheduleAgentSlug,
  rosterTriggers,
  buildFleet,
  fleetCensus,
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
  it("extracts the --agent binding from a cadence intent", () => {
    expect(scheduleAgentSlug("run the digest --agent researcher")).toBe("researcher");
    expect(scheduleAgentSlug("--agent=ops-watcher check disks")).toBe("ops-watcher");
    expect(scheduleAgentSlug("plain intent, default persona")).toBe("");
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
  { id: "sch2", intent: "--agent researcher noop", cadence: "hourly", enabled: false },
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
    // ord1 cron + ord1 event (sch2 is disabled so no cadence wake)
    expect(res.map((t) => t.mode)).toEqual(["cron", "event"]);
    expect(res[0].via).toBe("Nightly digest");

    const ops = rosterTriggers("ops", orders, schedules);
    // ord2 disabled; sch1 enabled cadence
    expect(ops.map((t) => t.mode)).toEqual(["cadence"]);
    expect(ops[0].via).toBe("sch1");
  });
});

describe("buildFleet", () => {
  const fleet = buildFleet(profiles, orders, schedules, workflows, runs, { running: true, cadence_sec: 60 });
  const byKey = (k: string) => fleet.find((e) => e.key === k)!;

  it("includes every archetype + the three system engines", () => {
    expect(fleet.filter((e) => e.kind === "roster")).toHaveLength(4); // retired kept
    expect(fleet.filter((e) => e.kind === "standing")).toHaveLength(2);
    expect(fleet.filter((e) => e.kind === "schedule")).toHaveLength(2);
    expect(fleet.filter((e) => e.kind === "workflow")).toHaveLength(2);
    expect(fleet.filter((e) => e.kind === "system").map((e) => e.slug).sort()).toEqual(["overseer", "pulse", "reaper"]);
  });

  it("derives roster state + triggers", () => {
    const ops = byKey("roster:ops");
    expect(ops.running).toBe(true);
    expect(ops.state).toBe("running");

    const res = byKey("roster:researcher");
    expect(res.running).toBe(false);
    expect(res.state).toBe("armed");
    expect(res.lastRunMs).toBe(300);
    expect(res.triggers.map((t) => t.mode)).toEqual(["cron", "event"]);

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
    expect(byKey("schedule:sch2").state).toBe("paused");
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
    expect(c.schedule).toBe(2);
    expect(c.workflow).toBe(2);
    expect(c.system).toBe(3);
    expect(c.total).toBe(13);
    expect(c.running).toBeGreaterThanOrEqual(2); // ops run + overseer
  });
});
