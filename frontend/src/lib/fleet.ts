// fleet.ts (M952) — the unified agent census. The Agents page used to be purely
// run-centric: it only showed runs that were happening *now*, so at rest the
// page was empty and you couldn't see what force you actually have. This module
// folds every durable agent/automation archetype the system knows about into one
// normalized FleetEntity, each carrying — front and centre — HOW it gets
// triggered (manual / cron / event / webhook / cadence / delegation). It is a
// pure, unit-tested adapter over the existing list endpoints; no daemon needs to
// be running for the catalogue to be full.
//
// Archetypes (and their source endpoint):
//   roster    — /api/agents     persistent identities (woken by delegation, or a
//                                standing order / schedule bound to them)
//   standing  — /api/standing    goals that fire on a cron schedule or an event
//   schedule  — /api/schedules   cadence intents (interval/daily/once/window/…)
//   workflow  — /api/workflows   DAGs whose trigger node is manual/cron/event/webhook
//   system    — always-on engines: Pulse (heartbeat), Reaper (sentinel), Overseer

export type FleetKind = "roster" | "standing" | "schedule" | "workflow" | "system";

// How an entity comes alive. "delegation" = only ever invoked by another agent or
// a manual run; "manual" = an operator has to press go; the rest fire themselves.
export type TriggerMode = "manual" | "cron" | "event" | "webhook" | "continuous" | "cadence" | "delegation";

export interface FleetTrigger {
  mode: TriggerMode;
  /** Compact human label for the card chip, e.g. "0 9 * * *", "on task.failed". */
  label: string;
  /** Plain-language "what makes this run", for the detail panel. */
  needs: string;
  /** Where the trigger comes from (the standing order's name, the schedule id…). */
  via?: string;
}

// The resting state of an entity, used for the colour dot + state pill.
//   running — a run is happening right now
//   armed   — enabled and has an automatic trigger; will fire on its own
//   manual  — enabled but only fires when you (or another agent) ask
//   paused  — disabled
//   retired — in the graveyard
export type FleetState = "running" | "armed" | "manual" | "paused" | "retired";

export interface FleetEntity {
  key: string; // unique across kinds, e.g. "roster:researcher", "standing:01H…"
  kind: FleetKind;
  name: string;
  slug: string; // identity seed for AgentAvatar (hue + initials)
  enabled: boolean;
  retired?: boolean;
  running: boolean;
  state: FleetState;
  model?: string;
  description?: string;
  triggers: FleetTrigger[];
  nextRunMs?: number;
  lastRunMs?: number;
  fires?: number;
  /** NAV view id to deep-link "Manage" to, or "" if there is no editor page. */
  manageView: "roster" | "standing" | "schedules" | "flow" | "overseer" | "";
  /** The raw source object, for the detail panel to read kind-specific fields. */
  raw: unknown;
}

export interface FleetCensus {
  roster: number;
  standing: number;
  schedule: number;
  workflow: number;
  system: number;
  running: number;
  armed: number;
  total: number;
}

// ─────────────────────────── input shapes ───────────────────────────
// Loose mirrors of the list-endpoint JSON. Extra fields are ignored; missing
// optional ones degrade gracefully.

export interface FleetRun {
  correlation_id?: string;
  status?: string;
  agent?: string;
  started_unix_ms?: number;
}

export interface ApiProfile {
  slug: string;
  name?: string;
  model?: string;
  enabled: boolean;
  retired?: boolean;
  soul?: string;
  description?: string;
  task_type?: string;
  memory_scope?: string;
  workdir?: string;
  max_cost_mc?: number;
}

export interface ApiTrigger {
  type?: string;
  schedule?: string;
  subject?: string;
}

export interface ApiOrder {
  id: string;
  name?: string;
  enabled: boolean;
  agent?: string;
  triggers?: ApiTrigger[];
  plan?: string;
  initiative?: { mode?: string };
}

export interface ApiSchedule {
  id: string;
  intent?: string;
  cadence?: string;
  mode?: string;
  source?: string;
  enabled: boolean;
  next_run_unix?: number;
  last_status?: string;
  fires?: number;
}

export interface ApiWorkflow {
  id?: string;
  name: string;
  description?: string;
  enabled?: boolean;
  trigger_kind?: string;
  trigger_detail?: string;
  node_count?: number;
}

export interface ApiPulse {
  running?: boolean;
  paused?: boolean;
  cadence_sec?: number;
  dial?: string;
}

// ─────────────────────────── small pure helpers ───────────────────────────

// statusKind normalizes the daemon's many run statuses into the four buckets the
// UI colours + filters by. Canonical home (re-exported from views/Agents for the
// run gallery). Pure + unit-tested.
export type StatusKind = "running" | "done" | "failed" | "other";
export function statusKind(status?: string): StatusKind {
  const s = (status || "").toLowerCase();
  if (s === "running" || s === "active" || s === "in_progress") return "running";
  if (s === "completed" || s === "done" || s === "succeeded" || s === "ok") return "done";
  if (s === "failed" || s === "error" || s === "cancelled" || s === "canceled" || s === "halted") return "failed";
  return "other";
}

// scheduleAgentSlug extracts the roster slug a cadence intent runs as: cadence
// entries have no agent field, so "--agent <slug>" inside the intent is the
// binding (mirrors how the daemon resolves it at fire time). Pure + unit-tested.
export function scheduleAgentSlug(intent?: string): string {
  const m = (intent || "").match(/--agent[= ]+([\w.-]+)/);
  return m ? m[1] : "";
}

// ─────────────────────────── trigger derivation ───────────────────────────

// rosterTriggers folds the wake sources that will fire a roster agent: every
// enabled standing order bound to it (cron/event) plus every enabled schedule
// that runs "--agent <slug>". Empty ⇒ the agent only wakes by delegation.
export function rosterTriggers(slug: string, orders: ApiOrder[], schedules: ApiSchedule[]): FleetTrigger[] {
  const out: FleetTrigger[] = [];
  for (const o of orders) {
    if (!o.enabled || o.agent !== slug) continue;
    const via = o.name || o.id;
    for (const t of o.triggers || []) {
      if (t.type === "cron" && t.schedule)
        out.push({ mode: "cron", label: t.schedule, needs: `Fires on the schedule “${t.schedule}”.`, via });
      else if (t.type === "event" && t.subject)
        out.push({ mode: "event", label: `on ${t.subject}`, needs: `Fires when the event “${t.subject}” occurs.`, via });
    }
  }
  for (const s of schedules) {
    if (s.enabled && scheduleAgentSlug(s.intent) === slug)
      out.push({
        mode: "cadence",
        label: s.cadence || s.id,
        needs: `Runs on a schedule (${s.cadence || "cadence"}).`,
        via: s.id,
      });
  }
  return out;
}

function workflowTrigger(w: ApiWorkflow): FleetTrigger {
  const kind = (w.trigger_kind || "manual").toLowerCase();
  const detail = w.trigger_detail || "";
  switch (kind) {
    case "cron":
      return { mode: "cron", label: detail || "schedule", needs: `Fires on a schedule${detail ? ` (${detail})` : ""}.` };
    case "event":
      return { mode: "event", label: detail ? `on ${detail}` : "event", needs: `Fires when the event “${detail}” occurs.` };
    case "webhook":
      return { mode: "webhook", label: "webhook", needs: "Fires when an external caller POSTs to its webhook URL." };
    default:
      return { mode: "manual", label: "manual", needs: "Run it from Flow Studio — it has no automatic trigger." };
  }
}

function scheduleTrigger(s: ApiSchedule): FleetTrigger {
  const mode: TriggerMode = (s.mode || "").toLowerCase() === "continuous" ? "continuous" : "cadence";
  const label = s.cadence || s.mode || "schedule";
  const needs =
    mode === "continuous"
      ? "Loops continuously — re-runs a cooldown after each completion."
      : `Fires automatically (${label}). You can also run it now.`;
  return { mode, label, needs };
}

// ─────────────────────────── system engines ───────────────────────────

function systemEntities(pulse?: ApiPulse): FleetEntity[] {
  const pulseRunning = pulse ? !!pulse.running && !pulse.paused : true;
  const cadence = pulse?.cadence_sec ? `every ${pulse.cadence_sec}s` : "heartbeat";
  return [
    {
      key: "system:pulse",
      kind: "system",
      name: "Pulse",
      slug: "pulse",
      enabled: pulse ? !!pulse.running : true,
      running: pulseRunning,
      state: pulse && pulse.paused ? "paused" : "armed",
      description: "The proactive heartbeat — ticks on a cadence, fans out to observers, and briefs you on what changed.",
      triggers: [
        { mode: "cadence", label: cadence, needs: `Always on. Thinks on a cadence tick (${cadence}); you can also beat it now.` },
      ],
      manageView: "",
      raw: pulse || null,
    },
    {
      key: "system:reaper",
      kind: "system",
      name: "Reaper",
      slug: "reaper",
      enabled: true,
      running: false,
      state: "armed",
      description: "The sentinel — surfaces dead-agent candidates and stale artifacts. Read-only; retiring stays operator-gated.",
      triggers: [
        { mode: "cadence", label: "pulse observer", needs: "Runs as a Pulse observer each tick; scanning is read-only." },
      ],
      manageView: "",
      raw: null,
    },
    {
      key: "system:overseer",
      kind: "system",
      name: "Overseer",
      slug: "overseer",
      enabled: true,
      running: true,
      state: "running",
      description: "The always-on supervisor — live view of running agents, the roster, and the help board.",
      triggers: [{ mode: "cadence", label: "live", needs: "Always on. Watches the live event stream; no trigger needed." }],
      manageView: "overseer",
      raw: null,
    },
  ];
}

// ─────────────────────────── the builder ───────────────────────────

// buildFleet is the heart of the census: it turns the four durable list
// endpoints (+ optional pulse status) into one sorted FleetEntity[] where every
// agent/automation is visible at rest with its trigger spelled out. Sort order:
// running first, then armed (will fire on its own), then by kind, then name.
// Retired roster agents are kept (as state "retired") so the graveyard is still
// visible — the page is a complete inventory. Pure + unit-tested.
export function buildFleet(
  profiles: ApiProfile[],
  orders: ApiOrder[],
  schedules: ApiSchedule[],
  workflows: ApiWorkflow[],
  runs: FleetRun[],
  pulse?: ApiPulse,
): FleetEntity[] {
  const out: FleetEntity[] = [];

  // Roster identities.
  for (const p of profiles) {
    let running = false;
    let lastRunMs: number | undefined;
    for (const r of runs) {
      if ((r.agent || "") !== p.slug) continue;
      if (statusKind(r.status) === "running") running = true;
      if ((r.started_unix_ms || 0) > (lastRunMs || 0)) lastRunMs = r.started_unix_ms;
    }
    const triggers = p.retired ? [] : rosterTriggers(p.slug, orders, schedules);
    const state: FleetState = p.retired
      ? "retired"
      : running
        ? "running"
        : !p.enabled
          ? "paused"
          : triggers.length > 0
            ? "armed"
            : "manual";
    if (triggers.length === 0 && !p.retired)
      triggers.push({
        mode: "delegation",
        label: "manual / delegated",
        needs: `Run it yourself (agt run --agent ${p.slug}) or let another agent delegate to it.`,
      });
    out.push({
      key: `roster:${p.slug}`,
      kind: "roster",
      name: p.name || p.slug,
      slug: p.slug,
      enabled: p.enabled,
      retired: p.retired,
      running,
      state,
      model: p.model,
      description: p.description,
      triggers,
      lastRunMs,
      manageView: "roster",
      raw: p,
    });
  }

  // Standing orders.
  for (const o of orders) {
    const triggers: FleetTrigger[] = [];
    for (const t of o.triggers || []) {
      if (t.type === "cron" && t.schedule)
        triggers.push({ mode: "cron", label: t.schedule, needs: `Fires on the schedule “${t.schedule}”.`, via: o.agent });
      else if (t.type === "event" && t.subject)
        triggers.push({ mode: "event", label: `on ${t.subject}`, needs: `Fires when the event “${t.subject}” occurs.`, via: o.agent });
    }
    if (triggers.length === 0)
      triggers.push({ mode: "manual", label: "manual", needs: "No automatic trigger — fire it yourself." });
    out.push({
      key: `standing:${o.id}`,
      kind: "standing",
      name: o.name || o.id,
      slug: `so-${o.id}`,
      enabled: o.enabled,
      running: false,
      state: !o.enabled ? "paused" : triggers.some((t) => t.mode !== "manual") ? "armed" : "manual",
      description: o.plan,
      triggers,
      manageView: "standing",
      raw: o,
    });
  }

  // Schedules (cadence intents).
  for (const s of schedules) {
    out.push({
      key: `schedule:${s.id}`,
      kind: "schedule",
      name: s.intent || s.id,
      slug: `sc-${s.id}`,
      enabled: s.enabled,
      running: false,
      state: !s.enabled ? "paused" : "armed",
      description: s.source ? `source: ${s.source}` : undefined,
      triggers: [scheduleTrigger(s)],
      nextRunMs: s.next_run_unix ? s.next_run_unix * 1000 : undefined,
      fires: s.fires,
      manageView: "schedules",
      raw: s,
    });
  }

  // Workflows.
  for (const w of workflows) {
    const trig = workflowTrigger(w);
    out.push({
      key: `workflow:${w.id || w.name}`,
      kind: "workflow",
      name: w.name,
      slug: `wf-${w.id || w.name}`,
      enabled: w.enabled !== false,
      running: false,
      state: w.enabled === false ? "paused" : trig.mode === "manual" ? "manual" : "armed",
      description: w.description,
      triggers: [trig],
      manageView: "flow",
      raw: w,
    });
  }

  // Always-on system engines.
  out.push(...systemEntities(pulse));

  const KIND_ORDER: Record<FleetKind, number> = { roster: 0, standing: 1, schedule: 2, workflow: 3, system: 4 };
  const stateRank = (e: FleetEntity) => (e.running ? 0 : e.state === "armed" ? 1 : e.state === "manual" ? 2 : 3);
  out.sort((a, b) => {
    const sr = stateRank(a) - stateRank(b);
    if (sr !== 0) return sr;
    const ko = KIND_ORDER[a.kind] - KIND_ORDER[b.kind];
    if (ko !== 0) return ko;
    return a.name.localeCompare(b.name);
  });
  return out;
}

// fleetCensus tallies the catalogue for the summary band. Pure + unit-tested.
export function fleetCensus(items: FleetEntity[]): FleetCensus {
  const c: FleetCensus = { roster: 0, standing: 0, schedule: 0, workflow: 0, system: 0, running: 0, armed: 0, total: items.length };
  for (const e of items) {
    c[e.kind]++;
    if (e.running) c.running++;
    if (e.state === "armed") c.armed++;
  }
  return c;
}
