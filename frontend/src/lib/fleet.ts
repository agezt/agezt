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
//   schedule  — /api/schedules   cron-like jobs (agent task/workflow/system task/tool)
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
  /** A shipped internal guardian (roster.System, M961) — protected + badged. */
  system?: boolean;
  agentClass?: "system" | "custom" | "subagent";
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
  directAgents: number;
  subagents: number;
  standing: number;
  schedule: number;
  workflow: number;
  system: number;
  running: number;
  repair: number;
  graveyard: number;
  armed: number;
  total: number;
}

export type FleetEntityFilter = "all" | FleetKind | "running" | "guardians" | "graveyard" | "direct" | "subagents" | "repair";

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
  system?: boolean;
  kind?: "system" | "custom" | "subagent";
  managed?: boolean;
  owner_agent?: string;
  parent_agent?: string;
  direct_callable?: boolean;
  soul?: string;
  description?: string;
  task_type?: string;
  memory_scope?: string;
  workdir?: string;
  max_cost_mc?: number;
  max_daily_mc?: number;
  tool_allow?: string[];
  tool_deny?: string[];
  trust_ceiling?: "L0" | "L1" | "L2" | "L3" | "L4";
  lifecycle?: {
    mode?: "persistent" | "cycle" | "retire_on_complete";
    retire_on_complete?: boolean;
    max_cycles?: number;
    completed_cycles?: number;
  };
  tasklist?: {
    id?: string;
    title?: string;
    scope?: "cycle" | "total";
    status?: "todo" | "doing" | "done" | "blocked" | "retired";
  }[];
  retry_policy?: { max_attempts?: number; backoff?: string; base_delay_sec?: number; max_delay_sec?: number; retry_on?: string[] };
  health_policy?: { stale_after_sec?: number; failure_window?: number; failure_threshold?: number; doctor_agent?: string };
  self_repair?: { enabled?: boolean; max_attempts?: number; escalate_to?: string };
  status?: {
    health_state?: "healthy" | "degraded" | "misconfigured" | "stale" | "retired" | "unstable" | "stabilizing" | "force_failed";
    health_label?: string;
    repair_state?: string;
    repair_label?: string;
    invalid_runtime_overrides?: number;
    repair_inflight?: number;
    repair_next_eligible_ms?: number;
    routing_fallback_count?: number;
    routing_last_reason?: string;
    routing_last_failed?: string;
    routing_last_next?: string;
    routing_last_ts_ms?: number;
    retry_count?: number;
    retry_last_reason?: string;
    retry_last_ts_ms?: number;
    retry_next_attempt?: number;
    retry_max_attempts?: number;
  };
}

interface ApiTrigger {
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
  model?: string;
  agent?: string;
  target?: string;
  workflow?: string;
  system_task?: string;
  tool?: string;
  payload?: unknown;
  cadence?: string;
  mode?: string;
  interval_sec?: number;
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
  cadence_ms?: number; // the daemon reports the heartbeat period in ms
  cadence_sec?: number; // tolerated fallback
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

// scheduleAgentSlug extracts the legacy roster binding from an old cadence
// intent. New schedules carry `agent` as a structured field; this remains for
// imported/pre-existing rows.
export function scheduleAgentSlug(intent?: string): string {
  const m = (intent || "").match(/--agent[= ]+([\w.-]+)/);
  return m ? m[1] : "";
}

// ─────────────────────────── trigger derivation ───────────────────────────

// rosterTriggers folds the wake sources that will fire a roster agent: every
// enabled standing order bound to it (cron/event) plus every enabled schedule
// whose structured agent field names it, including workflow/tool schedules that
// execute under the agent's identity. Empty ⇒ the agent only wakes by delegation.
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
    if (!s.enabled) continue;
    const structuredAgent = (s.agent || "") === slug;
    const legacyAgent = !s.target && scheduleAgentSlug(s.intent) === slug;
    if (structuredAgent || legacyAgent) {
      out.push({ ...scheduleTrigger(s), via: s.id });
    }
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
  if (s.target === "workflow" && s.workflow) {
    return { mode, label, needs: `Runs workflow "${s.workflow}" automatically (${label}).` };
  }
  if (s.target === "system_task" && s.system_task) {
    return { mode, label, needs: `Runs system task "${s.system_task}" automatically (${label}).` };
  }
  if (s.target === "tool" && s.tool) {
    return { mode, label, needs: `Runs tool "${s.tool}" automatically (${label}).` };
  }
  const needs =
    mode === "continuous"
      ? "Loops continuously — re-runs a cooldown after each completion."
      : `Fires automatically (${label}). You can also run it now.`;
  return { mode, label, needs };
}

export function scheduleJobTitle(s: Pick<ApiSchedule, "id" | "intent" | "agent" | "target" | "workflow" | "system_task" | "tool">): string {
  if (s.target === "workflow" && s.workflow) return `Run workflow ${s.workflow}`;
  if (s.target === "system_task" && s.system_task) return `Run system task ${s.system_task}`;
  if (s.target === "tool" && s.tool) return `Run tool ${s.tool}`;
  if (s.agent && s.intent) return `Wake ${s.agent}: ${s.intent}`;
  return s.intent || s.id;
}

export function fleetAgentHierarchyLabel(
  profile: Pick<ApiProfile, "kind" | "managed" | "direct_callable" | "owner_agent" | "parent_agent" | "system">,
): string {
  if (profile.system || profile.kind === "system") return "system guardian";
  const manager = (profile.parent_agent || profile.owner_agent || "").trim();
  if (profile.kind === "subagent" || profile.managed || profile.direct_callable === false) {
    return manager ? `sub-agent of ${manager}` : "managed sub-agent";
  }
  if (profile.parent_agent) return `direct · parent ${profile.parent_agent}`;
  if (profile.owner_agent) return `direct · owner ${profile.owner_agent}`;
  return "direct agent";
}

export function fleetAgentTaskContractLabel(profile: Pick<ApiProfile, "lifecycle" | "tasklist" | "retired">): string {
  if (profile.retired) return "graveyard";
  const lifecycle = profile.lifecycle;
  const rawMode = lifecycle?.mode || (lifecycle?.retire_on_complete ? "retire_on_complete" : "persistent");
  const max = lifecycle?.max_cycles || 0;
  const mode = rawMode === "persistent" && max > 0 ? "cycle" : rawMode;
  const tasks = (profile.tasklist || []).filter((t) => t.status !== "retired");
  const cycle = tasks.filter((t) => (t.scope || "total") === "cycle").length;
  const total = tasks.filter((t) => (t.scope || "total") === "total").length;
  const doing = tasks.filter((t) => t.status === "doing").length;
  const blocked = tasks.filter((t) => t.status === "blocked").length;
  const done = tasks.filter((t) => t.status === "done").length;
  const work = tasks.length > 0 ? `${cycle} cycle/${total} total` : "no tasks";
  const progress = [
    doing > 0 ? `${doing} doing` : "",
    blocked > 0 ? `${blocked} blocked` : "",
    done > 0 ? `${done} done` : "",
  ].filter(Boolean);
  if (mode === "retire_on_complete") return `one-shot · ${work}${progress.length ? ` · ${progress.join(" · ")}` : ""}`;
  if (mode === "cycle") {
    const completed = lifecycle?.completed_cycles || 0;
    const cycles = max > 0 ? `${completed}/${max}` : completed > 0 ? `${completed} done` : "open";
    return `cycle ${cycles} · ${work}${progress.length ? ` · ${progress.join(" · ")}` : ""}`;
  }
  return `persistent · ${work}${progress.length ? ` · ${progress.join(" · ")}` : ""}`;
}

export function fleetAgentCapabilityLabel(profile: Pick<ApiProfile, "system" | "trust_ceiling" | "tool_allow" | "tool_deny">): string {
  const ceiling = profile.trust_ceiling || "L4";
  const allow = (profile.tool_allow || []).filter(Boolean);
  const deny = (profile.tool_deny || []).filter(Boolean);
  if (profile.system && allow.length === 0) return `system quiet · deny ${Math.max(deny.length, 1)} · ${ceiling}`;
  if (allow.length > 0) return `allow ${allow.length}${deny.length > 0 ? ` / deny ${deny.length}` : ""} · ${ceiling}`;
  if (deny.length > 0) return `deny ${deny.length} · ${ceiling}`;
  return ceiling === "L4" ? "open tools · L4" : `trust ${ceiling}`;
}

const HIGH_IMPACT_TOOL_RE = /(shell|exec|code|workflow|file|delete|browser|mcp|db|database|fetch|net|http|tool_forge|homeassistant|notify)/i;

export function highImpactToolNames(names: string[], limit = 3): string[] {
  const out: string[] = [];
  const seen = new Set<string>();
  for (const raw of names) {
    const name = raw.trim();
    const key = name.toLowerCase();
    if (!name || seen.has(key) || !HIGH_IMPACT_TOOL_RE.test(name)) continue;
    out.push(name);
    seen.add(key);
    if (out.length >= limit) break;
  }
  return out;
}

export function fleetAgentAuthorityLabel(profile: Pick<ApiProfile, "system" | "trust_ceiling" | "tool_allow" | "tool_deny">): string {
  const ceiling = profile.trust_ceiling || "L4";
  const allow = (profile.tool_allow || []).map((x) => x.trim()).filter(Boolean);
  const deny = (profile.tool_deny || []).map((x) => x.trim()).filter(Boolean);
  const highImpact = highImpactToolNames(allow);
  const blockedHighImpact = highImpactToolNames(deny);
  if (highImpact.length > 0) return `high-impact allow: ${highImpact.join(", ")} · ${ceiling}`;
  if (allow.length > 0) return `allowlisted ${allow.length} · ${ceiling}`;
  if (profile.system) return `system-governed high-impact · notify gated · ${ceiling}`;
  if (blockedHighImpact.length > 0) return `broad minus ${blockedHighImpact.join(", ")} · ${ceiling}`;
  if (deny.length > 0) return `broad minus ${deny.length} · ${ceiling}`;
  return ceiling === "L4" ? "open high-impact tools · L4" : `trust-gated high-impact · ${ceiling}`;
}

export function fleetAgentResilienceLabel(profile: Pick<ApiProfile, "retry_policy" | "health_policy" | "self_repair">): string {
  const parts = [
    profile.retry_policy?.max_attempts ? `retry ${profile.retry_policy.max_attempts}x` : "",
    profile.health_policy?.doctor_agent ? `doctor ${profile.health_policy.doctor_agent}` : "",
    profile.self_repair?.enabled ? `self-repair${profile.self_repair.max_attempts ? ` ${profile.self_repair.max_attempts}x` : ""}` : "",
    profile.self_repair?.escalate_to ? `escalate ${profile.self_repair.escalate_to}` : "",
  ].filter(Boolean);
  return parts.join(" · ") || "manual recovery";
}

export interface FleetAgentIdentityCardSummary {
  label: string;
  detail: string;
  tone: "good" | "warn" | "bad" | "accent" | "muted";
}

export function fleetAgentIdentityCardSummary(
  profile: ApiProfile,
  state: FleetState = "manual",
  running = false,
): FleetAgentIdentityCardSummary {
  const hierarchy = fleetAgentHierarchyLabel(profile);
  const task = fleetAgentTaskContractLabel(profile);
  const authority = fleetAgentAuthorityLabel(profile);
  const repair = fleetAgentResilienceLabel(profile);
  const status = profile.retired
    ? "graveyard"
    : running || state === "running"
      ? "awake"
      : profile.enabled === false || state === "paused"
        ? "paused"
        : state === "armed"
          ? "sleeping until trigger"
          : "sleeping";
  const detail = [task, authority, repair].filter(Boolean).join(" · ");
  if (profile.retired || state === "retired") {
    return { label: `${hierarchy} · graveyard`, detail: [task, "revive/remove"].join(" · "), tone: "muted" };
  }
  if (running || state === "running") {
    return { label: `${hierarchy} · ${status}`, detail, tone: "accent" };
  }
  if (profile.enabled === false || state === "paused") {
    return { label: `${hierarchy} · paused`, detail, tone: "warn" };
  }
  if (authority.startsWith("open high-impact") || authority.startsWith("high-impact allow")) {
    return { label: `${hierarchy} · ${status}`, detail, tone: "warn" };
  }
  return { label: `${hierarchy} · ${status}`, detail, tone: "good" };
}

function scheduleDescription(s: ApiSchedule): string | undefined {
  const parts = [
    s.intent && scheduleJobTitle(s) !== s.intent ? `label: ${s.intent}` : "",
    s.agent ? `as ${s.agent}` : "",
    s.model ? `model: ${s.model}` : "",
    s.source ? `source: ${s.source}` : "",
  ].filter(Boolean);
  return parts.length > 0 ? parts.join(" | ") : undefined;
}

// ─────────────────────────── system engines ───────────────────────────

function systemEntities(pulse?: ApiPulse): FleetEntity[] {
  const pulseRunning = pulse ? !!pulse.running && !pulse.paused : true;
  const secs = pulse?.cadence_ms ? Math.round(pulse.cadence_ms / 1000) : pulse?.cadence_sec;
  const cadence = secs ? `every ${secs}s` : "heartbeat";
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
      system: p.system,
      agentClass: p.kind || (p.system ? "system" : p.managed || p.direct_callable === false ? "subagent" : "custom"),
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

  // Schedules (cron-like jobs).
  for (const s of schedules) {
    out.push({
      key: `schedule:${s.id}`,
      kind: "schedule",
      name: scheduleJobTitle(s),
      slug: `sc-${s.id}`,
      enabled: s.enabled,
      running: false,
      state: !s.enabled ? "paused" : "armed",
      description: scheduleDescription(s),
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

export function filterFleetEntities(items: FleetEntity[], filter: FleetEntityFilter, query = ""): FleetEntity[] {
  const q = query.trim().toLowerCase();
  return items.filter((e) => {
    if (filter === "running" && !e.running) return false;
    if (filter === "guardians" && !e.system) return false;
    if (filter === "graveyard" && e.state !== "retired") return false;
    if (filter === "direct" && (e.kind !== "roster" || e.agentClass !== "custom")) return false;
    if (filter === "subagents" && (e.kind !== "roster" || e.agentClass !== "subagent")) return false;
    if (filter === "repair" && !fleetEntityNeedsRepair(e)) return false;
    if (
      filter !== "all" &&
      filter !== "running" &&
      filter !== "guardians" &&
      filter !== "graveyard" &&
      filter !== "direct" &&
      filter !== "subagents" &&
      filter !== "repair" &&
      e.kind !== filter
    ) return false;
    if (!q) return true;
    return (
      e.name.toLowerCase().includes(q) ||
      (e.description || "").toLowerCase().includes(q) ||
      (e.model || "").toLowerCase().includes(q) ||
      e.triggers.some((t) => `${t.mode} ${t.label}`.toLowerCase().includes(q))
    );
  });
}

export function fleetEntityNeedsRepair(e: FleetEntity): boolean {
  if (e.kind !== "roster" || !e.raw || typeof e.raw !== "object") return false;
  const status = (e.raw as ApiProfile).status;
  const repairState = (status?.repair_state || "").toLowerCase();
  const healthState = (status?.health_state || "").toLowerCase();
  return (
    (status?.repair_inflight || 0) > 0 ||
    (status?.retry_count || 0) > 0 ||
    repairState === "queued" ||
    repairState === "failed" ||
    repairState === "attempts_exhausted" ||
    healthState === "degraded" ||
    healthState === "misconfigured" ||
    healthState === "unstable" ||
    healthState === "force_failed" ||
    healthState === "force_exhausted"
  );
}

// fleetCensus tallies the catalogue for the summary band. Pure + unit-tested.
export function fleetCensus(items: FleetEntity[]): FleetCensus {
  const c: FleetCensus = { roster: 0, directAgents: 0, subagents: 0, standing: 0, schedule: 0, workflow: 0, system: 0, running: 0, repair: 0, graveyard: 0, armed: 0, total: items.length };
  for (const e of items) {
    c[e.kind]++;
    if (e.kind === "roster" && e.agentClass === "custom") c.directAgents++;
    if (e.kind === "roster" && e.agentClass === "subagent") c.subagents++;
    if (e.running) c.running++;
    if (fleetEntityNeedsRepair(e)) c.repair++;
    if (e.state === "retired") c.graveyard++;
    if (e.state === "armed") c.armed++;
  }
  return c;
}
