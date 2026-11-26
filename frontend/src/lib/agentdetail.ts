// agentdetail.ts (M953) — pure helpers for the per-agent Command Center deep
// panel (components/AgentDetail). Kept separate from lib/fleet.ts (the M952
// census adapter, reused as-is) so the deep panel adds no surface to it. All
// functions are pure + unit-tested; the React component owns fetching/rendering.

import type {
  AgentEscalationsResponseEscalationsItem ,
  AgentListResponseProfilesItemStatus ,
  AgentRepairStatusResponse ,
  AgentRepairStatusResponseContract ,
  AgentRepairStatusResponseHistoryItem ,
  AgentRepairStatusResponseNextAction ,
  JournalGrepResponseEventsItem ,
  MemoryListResponseRecordsItem ,
  ProviderLogResponseEventsItem ,
  ReaperScanResponse ,
  ReaperScanResponseDeadAgentsItem ,
  ReaperScanResponseDegradedAgentsItem ,
  ReaperScanResponseMisconfiguredAgentsItem ,
  ReaperScanResponseRoutingForcedExhaustedAgentsItem ,
  ReaperScanResponseRoutingPressureAgentsItem ,
  ReaperScanResponseRoutingUnstableAgentsItem ,
  RunsListResponseRunsItem ,
  SkillListResponseSkillsItem,
} from "@/app/api/types.generated";

// MemoryRecord mirrors the wire shape of /api/memory records (recordView in
// kernel/controlplane/memory.go). Only the fields the panel reads are typed.
export type MemoryRecord = {
  id?: MemoryListResponseRecordsItem["id"];
  type?: MemoryListResponseRecordsItem["type"];
  subject?: MemoryListResponseRecordsItem["subject"];
  content?: MemoryListResponseRecordsItem["content"];
  confidence?: MemoryListResponseRecordsItem["confidence"];
  created_ms?: MemoryListResponseRecordsItem["created_ms"];
  last_seen_ms?: MemoryListResponseRecordsItem["last_seen_ms"];
  tags?: MemoryListResponseRecordsItem["tags"];
  added_by?: MemoryListResponseRecordsItem["added_by"];};

// SkillLite mirrors the /api/skills list fields the panel reads.
export type SkillLite = {
  id?: SkillListResponseSkillsItem["id"];
  name?: SkillListResponseSkillsItem["name"];
  description?: SkillListResponseSkillsItem["description"];
  status?: SkillListResponseSkillsItem["status"];
  agent?: SkillListResponseSkillsItem["agent"];
  triggers?: SkillListResponseSkillsItem["triggers"];};

// CorrelatedRow is any diagnostics row that can be attributed to a run/agent
// (policy decisions, tool invocations). Both carry a correlation id and an actor.
export interface CorrelatedRow {
  correlation_id?: JournalGrepResponseEventsItem["correlation_id"];
  actor?: JournalGrepResponseEventsItem["actor"];}

// RunLite is the subset of /api/runs the panel folds for spend/last-active.
export type RunLite = {
  correlation_id?: RunsListResponseRunsItem["correlation_id"];
  agent?: RunsListResponseRunsItem["agent"];
  status?: RunsListResponseRunsItem["status"];
  spent_mc?: RunsListResponseRunsItem["spent_mc"];
  started_unix_ms?: RunsListResponseRunsItem["started_unix_ms"];};

// The reaper's rows are the daemon's own scan rows: the browser names the
// fields it reads, the daemon names what they are.
type ReaperDeadAgent = {
  slug: ReaperScanResponseDeadAgentsItem["slug"];
  name?: ReaperScanResponseDeadAgentsItem["name"];
  last_active_ms?: ReaperScanResponseDeadAgentsItem["last_active_ms"];
};

type ReaperDegradedAgent = {
  slug: ReaperScanResponseDegradedAgentsItem["slug"];
  name?: ReaperScanResponseDegradedAgentsItem["name"];
  failures?: ReaperScanResponseDegradedAgentsItem["failures"];
  window?: ReaperScanResponseDegradedAgentsItem["window"];
  threshold?: ReaperScanResponseDegradedAgentsItem["threshold"];
  doctor_agent?: ReaperScanResponseDegradedAgentsItem["doctor_agent"];
  self_repair_enabled?: ReaperScanResponseDegradedAgentsItem["self_repair_enabled"];
  escalate_to?: ReaperScanResponseDegradedAgentsItem["escalate_to"];
  last_failure_ms?: ReaperScanResponseDegradedAgentsItem["last_failure_ms"];
  last_reason?: ReaperScanResponseDegradedAgentsItem["last_reason"];
};

type ReaperMisconfiguredAgent = {
  slug: ReaperScanResponseMisconfiguredAgentsItem["slug"];
  name?: ReaperScanResponseMisconfiguredAgentsItem["name"];
  issues?: ReaperScanResponseMisconfiguredAgentsItem["issues"];
  doctor_agent?: ReaperScanResponseMisconfiguredAgentsItem["doctor_agent"];
  self_repair_enabled?: ReaperScanResponseMisconfiguredAgentsItem["self_repair_enabled"];
  escalate_to?: ReaperScanResponseMisconfiguredAgentsItem["escalate_to"];
};

type ReaperRoutingPressureAgent = {
  slug: ReaperScanResponseRoutingPressureAgentsItem["slug"];
  name?: ReaperScanResponseRoutingPressureAgentsItem["name"];
  count?: ReaperScanResponseRoutingPressureAgentsItem["count"];
  threshold?: ReaperScanResponseRoutingPressureAgentsItem["threshold"];
  window_sec?: ReaperScanResponseRoutingPressureAgentsItem["window_sec"];
  doctor_agent?: ReaperScanResponseRoutingPressureAgentsItem["doctor_agent"];
  self_repair_enabled?: ReaperScanResponseRoutingPressureAgentsItem["self_repair_enabled"];
  escalate_to?: ReaperScanResponseRoutingPressureAgentsItem["escalate_to"];
  last_fallback_ms?: ReaperScanResponseRoutingPressureAgentsItem["last_fallback_ms"];
  last_reason?: ReaperScanResponseRoutingPressureAgentsItem["last_reason"];
  last_failed_model?: ReaperScanResponseRoutingPressureAgentsItem["last_failed_model"];
  last_next_model?: ReaperScanResponseRoutingPressureAgentsItem["last_next_model"];
  task_type?: ReaperScanResponseRoutingPressureAgentsItem["task_type"];
};

// The daemon reports the three forced-routing verdicts (probation, failed,
// exhausted) with ONE row shape, so the browser names one row too.
type ReaperRoutingForcedAgent = {
  slug: ReaperScanResponseRoutingForcedExhaustedAgentsItem["slug"];
  name?: ReaperScanResponseRoutingForcedExhaustedAgentsItem["name"];
  count?: ReaperScanResponseRoutingForcedExhaustedAgentsItem["count"];
  threshold?: ReaperScanResponseRoutingForcedExhaustedAgentsItem["threshold"];
  window_sec?: ReaperScanResponseRoutingForcedExhaustedAgentsItem["window_sec"];
  doctor_agent?: ReaperScanResponseRoutingForcedExhaustedAgentsItem["doctor_agent"];
  self_repair_enabled?: ReaperScanResponseRoutingForcedExhaustedAgentsItem["self_repair_enabled"];
  escalate_to?: ReaperScanResponseRoutingForcedExhaustedAgentsItem["escalate_to"];
  last_fallback_ms?: ReaperScanResponseRoutingForcedExhaustedAgentsItem["last_fallback_ms"];
  last_forced_ms?: ReaperScanResponseRoutingForcedExhaustedAgentsItem["last_forced_ms"];
  last_reason?: ReaperScanResponseRoutingForcedExhaustedAgentsItem["last_reason"];
  task_type?: ReaperScanResponseRoutingForcedExhaustedAgentsItem["task_type"];
  forced_chain?: ReaperScanResponseRoutingForcedExhaustedAgentsItem["forced_chain"];
  routing_force_generation?: ReaperScanResponseRoutingForcedExhaustedAgentsItem["routing_force_generation"];
};

type ReaperRoutingUnstableAgent = {
  slug: ReaperScanResponseRoutingUnstableAgentsItem["slug"];
  name?: ReaperScanResponseRoutingUnstableAgentsItem["name"];
  count?: ReaperScanResponseRoutingUnstableAgentsItem["count"];
  threshold?: ReaperScanResponseRoutingUnstableAgentsItem["threshold"];
  window_sec?: ReaperScanResponseRoutingUnstableAgentsItem["window_sec"];
  doctor_agent?: ReaperScanResponseRoutingUnstableAgentsItem["doctor_agent"];
  self_repair_enabled?: ReaperScanResponseRoutingUnstableAgentsItem["self_repair_enabled"];
  escalate_to?: ReaperScanResponseRoutingUnstableAgentsItem["escalate_to"];
  last_rollback_ms?: ReaperScanResponseRoutingUnstableAgentsItem["last_rollback_ms"];
  task_type?: ReaperScanResponseRoutingUnstableAgentsItem["task_type"];
  current_chain?: ReaperScanResponseRoutingUnstableAgentsItem["current_chain"];
  previous_chain?: ReaperScanResponseRoutingUnstableAgentsItem["previous_chain"];
  last_reason?: ReaperScanResponseRoutingUnstableAgentsItem["last_reason"];
};

export type ReaperReport = {
  dead_agents?: ReaperDeadAgent[] | null;
  degraded_agents?: ReaperDegradedAgent[] | null;
  misconfigured_agents?: ReaperMisconfiguredAgent[] | null;
  routing_pressure_agents?: ReaperRoutingPressureAgent[] | null;
  routing_forced_probation_agents?: ReaperRoutingForcedAgent[] | null;
  routing_forced_failed_agents?: ReaperRoutingForcedAgent[] | null;
  routing_forced_exhausted_agents?: ReaperRoutingForcedAgent[] | null;
  routing_unstable_agents?: ReaperRoutingUnstableAgent[] | null;
};

export interface AgentHealthSnapshot {
  state: "healthy" | "degraded" | "misconfigured" | "stale" | "retired" | "unstable" | "stabilizing" | "force_failed" | "force_exhausted";
  label: string;
  detail: string;
  doctorAgent?: string;
  selfRepairEnabled?: boolean;
  escalateTo?: string;
  lastActiveMs?: number;
  lastFailureMs?: number;
  configIssues?: string[];
}

interface AgentRuntimeOverride {
  key: string;
  value: string;
  label: string;
  effect: string;
  valid: boolean;
  issue?: string;
}

export interface AgentConfigOverrideSummary {
  runtime: AgentRuntimeOverride[];
  generic: Array<{ key: string; value: string }>;
}

export interface AgentRuntimeStatus
  extends Omit<
    Partial<AgentListResponseProfilesItemStatus>,
    "last_autonomy_runbook" | "mailbox_wakes"
  > {
  // Causality: board message id -> the wake it triggered (the standing/mailbox
  // fire's run correlation + timestamp). Lets the comms tab mark which message
  // actually woke the agent and link it to the run. Derived from the journal.
  // The daemon sends this record untyped, so the browser names its own shape.
  mailbox_wakes?: Record<string, MailboxWakeRef>;
  // The daemon sends this record untyped too; the browser reads it as a runbook.
  last_autonomy_runbook?: AgentLastAutonomyRunbook;
}

export interface MailboxWakeRef {
  correlation_id?: string;
  ts_unix_ms?: number;
  trigger_subject?: string;
}

// mailboxWakeFor returns the wake a board message triggered, if any — the bridge
// the comms tab uses to badge "this message woke the agent".
export function mailboxWakeFor(
  wakes: Record<string, MailboxWakeRef> | undefined,
  messageID: string | undefined,
): MailboxWakeRef | undefined {
  if (!wakes || !messageID) return undefined;
  return wakes[messageID];
}

export interface AgentLastAutonomyRunbook {
  identity_kind?: string;
  trigger_contract?: string;
  route_contract?: string;
  recovery_contract?: string;
  sleep_contract?: string;
  direct_callable?: boolean;
  delegation_manager?: string;
  retry_attempts?: number;
  self_repair_enabled?: boolean;
  self_repair_attempts?: number;
  doctor_agent?: string;
  phase?: string;
  // Wake provenance folded in from the firing event (manual agent.wake carries
  // none; schedule.fired sets source=schedule; standing.fired sets source=standing).
  source?: string;
  schedule_id?: string;
  standing_id?: string;
  standing_name?: string;
  trigger_subject?: string;
  // Mailbox-wake provenance: set when a standing order matched a board.* subject
  // (board.dm/help/broadcast/<topic>) — i.e. a message woke the agent.
  wake_via?: string;
  mailbox_message_id?: string;
  mailbox_from?: string;
  mailbox_to?: string;
  mailbox_reply_to?: string;
  mailbox_help?: boolean;
  // Delegated-wake provenance: set when a leader/manager spawned this agent as a
  // sub-agent (source=delegated). parent_correlation_id is the lead run;
  // correlation_id points at this agent's child run.
  delegated_by?: string;
  parent_correlation_id?: string;
  // Doctor-wake provenance: set when auto-repair woke this agent to handle another
  // agent's incident (source=doctor). doctor_for is the agent being repaired.
  doctor_for?: string;
  doctor_mode?: string;
  incident_id?: string;
  correlation_id?: string;
  ts_unix_ms?: number;
}

// lastAutonomyRunbookSourceLabel renders the wake provenance of the latest folded
// runbook ("via schedule <id>" / "via standing <name|id>") so the agent detail can
// say not just which contract last woke the agent but which trigger fired it.
// Manual operator wakes carry no source and yield "".
export function lastAutonomyRunbookSourceLabel(rb?: AgentLastAutonomyRunbook): string {
  if (!rb) return "";
  if (rb.wake_via === "mailbox") {
    const from = (rb.mailbox_from || "").trim();
    return from ? `via mailbox from ${from}` : "via mailbox";
  }
  const src = (rb.source || "").trim();
  if (src === "schedule") {
    const id = (rb.schedule_id || "").trim();
    return id ? `via schedule ${id}` : "via schedule";
  }
  if (src === "standing") {
    const who = (rb.standing_name || rb.standing_id || "").trim();
    return who ? `via standing ${who}` : "via standing";
  }
  if (src === "delegated") {
    const by = (rb.delegated_by || "").trim();
    return by ? `via delegation by ${by}` : "via delegation";
  }
  if (src === "doctor") {
    const forAgent = (rb.doctor_for || "").trim();
    return forAgent ? `via doctor for ${forAgent}` : "via doctor";
  }
  return src ? `via ${src}` : "";
}

export type ProviderRoutingRow = {
  ts_unix_ms?: ProviderLogResponseEventsItem["ts_unix_ms"];
  kind?: ProviderLogResponseEventsItem["kind"];
  primary?: ProviderLogResponseEventsItem["primary"];
  chain?: ProviderLogResponseEventsItem["chain"];
  task_type?: ProviderLogResponseEventsItem["task_type"];
  failed?: ProviderLogResponseEventsItem["failed"];
  next?: ProviderLogResponseEventsItem["next"];
  reason?: ProviderLogResponseEventsItem["reason"];
  scope?: ProviderLogResponseEventsItem["scope"];};

export interface ProviderRoutingSummary {
  kindLabel: string;
  kindTone: "bad" | "muted";
  stateLabel: string;
  stateTone: "bad" | "good";
  primaryText: string;
  primaryModel?: string;
  failedModel?: string;
  nextModel?: string;
  secondaryText?: string;
  detail?: string;
}

export interface AgentCardRuntimeSummary {
  healthText?: string;
  healthTone: "good" | "bad" | "muted";
  repairKindText?: string;
  repairKindTone: "accent" | "warn" | "muted";
  repairText?: string;
  repairDetail?: string;
  repairTone: "good" | "bad" | "accent" | "muted";
  repairIncidentText?: string;
  repairIncidentId?: string;
  repairIncidentDetail?: string;
  routingText?: string;
  routingTone: "good" | "bad" | "muted";
  routingDetail?: string;
  retryText?: string;
  retryTone: "bad" | "muted";
  retryDetail?: string;
  escalationText?: string;
  escalationTone: "bad" | "accent" | "muted";
  liveText?: string;
  liveDetail?: string;
  liveTone: "accent" | "muted";
  wakeText?: string;
  wakeDetail?: string;
  wakeTone: "accent" | "muted";
  escalationOpenCount: number;
  escalationAckedCount: number;
  activeRunCount: number;
  activeCorrelationId?: string;
  activeStartedMs?: number;
  activeLastEventMs?: number;
  activePhase?: string;
  activeTool?: string;
  activeModel?: string;
  activeContextDetail?: string;
  operationalText?: string;
  operationalState?: string;
  lastActivityMs?: number;
  lastActivitySummary?: string;
  lastAutonomyRunbook?: AgentLastAutonomyRunbook;
  wakeScheduleCount: number;
  wakeStandingCount: number;
  routingFallbackCount: number;
  retryCount: number;
  invalidRuntimeOverrides: number;
  configIssues: string[];
  repairInflight: number;
  nextRepairEligibleMs?: number;
  nextWakeMs?: number;
}

// The repair rows are the daemon's own status rows: the browser names the
// fields it reads, the daemon names what they are.
export type AgentRepairEvent = {
  seq?: AgentRepairStatusResponseHistoryItem["seq"];
  ts_unix_ms?: AgentRepairStatusResponseHistoryItem["ts_unix_ms"];
  correlation_id?: AgentRepairStatusResponseHistoryItem["correlation_id"];
  mode?: AgentRepairStatusResponseHistoryItem["mode"];
  phase?: AgentRepairStatusResponseHistoryItem["phase"];
  reason?: AgentRepairStatusResponseHistoryItem["reason"];
  fingerprint?: AgentRepairStatusResponseHistoryItem["fingerprint"];
  self_repair_attempt?: AgentRepairStatusResponseHistoryItem["self_repair_attempt"];
  self_repair_max_attempts?: AgentRepairStatusResponseHistoryItem["self_repair_max_attempts"];
  issues?: AgentRepairStatusResponseHistoryItem["issues"];
  applied?: AgentRepairStatusResponseHistoryItem["applied"];
  answer?: AgentRepairStatusResponseHistoryItem["answer"];
  error?: AgentRepairStatusResponseHistoryItem["error"];
  target_agent?: AgentRepairStatusResponseHistoryItem["target_agent"];
  target_correlation?: AgentRepairStatusResponseHistoryItem["target_correlation"];
  resolution?: AgentRepairStatusResponseHistoryItem["resolution"];
  resolution_summary?: AgentRepairStatusResponseHistoryItem["resolution_summary"];
  delegate_to?: AgentRepairStatusResponseHistoryItem["delegate_to"];
  delegated_by?: AgentRepairStatusResponseHistoryItem["delegated_by"];
  root_agent?: AgentRepairStatusResponseHistoryItem["root_agent"];
  chain_depth?: AgentRepairStatusResponseHistoryItem["chain_depth"];
  incident_id?: AgentRepairStatusResponseHistoryItem["incident_id"];
  root_incident_id?: AgentRepairStatusResponseHistoryItem["root_incident_id"];
  parent_incident_id?: AgentRepairStatusResponseHistoryItem["parent_incident_id"];
  next_eligible_ms?: AgentRepairStatusResponseHistoryItem["next_eligible_ms"];
  routing_task_type?: AgentRepairStatusResponseHistoryItem["routing_task_type"];
  routing_task_model_chain?: AgentRepairStatusResponseHistoryItem["routing_task_model_chain"];
  previous_routing_task_model_chain?: AgentRepairStatusResponseHistoryItem["previous_routing_task_model_chain"];
  routing_force_generation?: AgentRepairStatusResponseHistoryItem["routing_force_generation"];
  previous_routing_force_generation?: AgentRepairStatusResponseHistoryItem["previous_routing_force_generation"];
};

type AgentRepairContract = {
  retry_attempts?: AgentRepairStatusResponseContract["retry_attempts"];
  retry_backoff?: AgentRepairStatusResponseContract["retry_backoff"];
  retry_on?: AgentRepairStatusResponseContract["retry_on"];
  doctor_agent?: AgentRepairStatusResponseContract["doctor_agent"];
  failure_threshold?: AgentRepairStatusResponseContract["failure_threshold"];
  self_repair_enabled?: AgentRepairStatusResponseContract["self_repair_enabled"];
  self_repair_attempts?: AgentRepairStatusResponseContract["self_repair_attempts"];
  escalate_to?: AgentRepairStatusResponseContract["escalate_to"];
  cooldown_sec?: AgentRepairStatusResponseContract["cooldown_sec"];
  authority_boundary?: AgentRepairStatusResponseContract["authority_boundary"];
};

type AgentRepairNextAction = {
  action?: AgentRepairStatusResponseNextAction["action"];
  label?: AgentRepairStatusResponseNextAction["label"];
  detail?: AgentRepairStatusResponseNextAction["detail"];
  tone?: AgentRepairStatusResponseNextAction["tone"];
  correlation_id?: AgentRepairStatusResponseNextAction["correlation_id"];
  fingerprint?: AgentRepairStatusResponseNextAction["fingerprint"];
  phase?: AgentRepairStatusResponseNextAction["phase"];
  next_eligible_ms?: AgentRepairStatusResponseNextAction["next_eligible_ms"];
  delegate_to?: AgentRepairStatusResponseNextAction["delegate_to"];
};

export type AgentRepairStatus = {
  slug?: AgentRepairStatusResponse["slug"];
  cooldown_sec?: AgentRepairStatusResponse["cooldown_sec"];
  next_eligible_ms?: AgentRepairStatusResponse["next_eligible_ms"];
  inflight_count?: AgentRepairStatusResponse["inflight_count"];
  // The browser reads the daemon's rows LENIENTLY — every field may be
  // absent — so the browser's own types stay, and the pin that holds them to
  // the daemon's rows is the assertion in app/api/payloadtypes.ratchet.test.ts
  // (a renamed daemon row is a compile error there).
  history?: AgentRepairEvent[] | null;
  inflight?: AgentRepairEvent[] | null;
  latest?: AgentRepairEvent;
  contract?: AgentRepairContract;
  next_action?: AgentRepairNextAction;
};

export interface AgentRepairSnapshot {
  state: "idle" | "queued" | "completed" | "failed";
  label: string;
  detail: string;
  mode?: "misconfigured" | "degraded" | string;
  nextEligibleMs?: number;
}

export type AgentEscalation = {
  message_id?: AgentEscalationsResponseEscalationsItem["message_id"];
  from?: AgentEscalationsResponseEscalationsItem["from"];
  to?: AgentEscalationsResponseEscalationsItem["to"];
  text?: AgentEscalationsResponseEscalationsItem["text"];
  ts_unix_ms?: AgentEscalationsResponseEscalationsItem["ts_unix_ms"];
  status?: AgentEscalationsResponseEscalationsItem["status"];
  reply_count?: AgentEscalationsResponseEscalationsItem["reply_count"];
  acked?: AgentEscalationsResponseEscalationsItem["acked"];
  source_agent?: AgentEscalationsResponseEscalationsItem["source_agent"];
  mode?: AgentEscalationsResponseEscalationsItem["mode"];
  wake_phase?: AgentEscalationsResponseEscalationsItem["wake_phase"];
  wake_reason?: AgentEscalationsResponseEscalationsItem["wake_reason"];
  wake_error?: AgentEscalationsResponseEscalationsItem["wake_error"];
  wake_correlation_id?: AgentEscalationsResponseEscalationsItem["wake_correlation_id"];
  fingerprint?: AgentEscalationsResponseEscalationsItem["fingerprint"];
  resolution?: AgentEscalationsResponseEscalationsItem["resolution"];
  resolution_summary?: AgentEscalationsResponseEscalationsItem["resolution_summary"];
  delegate_to?: AgentEscalationsResponseEscalationsItem["delegate_to"];
  origin_kind?: AgentEscalationsResponseEscalationsItem["origin_kind"];
  origin_agent?: AgentEscalationsResponseEscalationsItem["origin_agent"];
  root_agent?: AgentEscalationsResponseEscalationsItem["root_agent"];
  chain_depth?: AgentEscalationsResponseEscalationsItem["chain_depth"];
  incident_id?: AgentEscalationsResponseEscalationsItem["incident_id"];
  root_incident_id?: AgentEscalationsResponseEscalationsItem["root_incident_id"];
  parent_incident_id?: AgentEscalationsResponseEscalationsItem["parent_incident_id"];};

export interface AgentEscalationSummary {
  openCount: number;
  ackedCount: number;
  doctorOpenCount: number;
  delegatedOpenCount: number;
  latest?: AgentEscalation;
}

export interface AgentOperationalTask {
  id: string;
  title: string;
  description?: string;
  scope: "total";
  status: "todo" | "doing" | "done" | "blocked";
  source: "escalation";
  ts_unix_ms?: number;
}

export function escalationChainLabel(
  row:
    | Pick<
        AgentEscalation,
        "origin_kind" | "origin_agent" | "root_agent" | "chain_depth"
      >
    | null
    | undefined,
): string {
  if (!row) return "";
  const bits: string[] = [];
  if (row.origin_kind === "delegated") {
    bits.push(`delegated by ${row.origin_agent || "agent"}`);
  } else if (row.origin_agent) {
    bits.push(`doctor ${row.origin_agent}`);
  }
  if (row.root_agent) bits.push(`root ${row.root_agent}`);
  if (typeof row.chain_depth === "number" && row.chain_depth > 0) {
    bits.push(`hop ${row.chain_depth}`);
  }
  return bits.join(" · ");
}

export function incidentLineageLabel(
  row:
    | Pick<
        AgentRepairEvent &
          AgentEscalation & {
            to?: string;
            target_agent?: string;
            delegate_to?: string;
            root_agent?: string;
            chain_depth?: number;
            incident_id?: string;
          },
        | "root_agent"
        | "chain_depth"
        | "incident_id"
        | "delegate_to"
        | "target_agent"
        | "to"
      >
    | null
    | undefined,
): string {
  if (!row) return "";
  const bits: string[] = [];
  if (row.root_agent) bits.push(`root ${row.root_agent}`);
  if (typeof row.chain_depth === "number") {
    bits.push(row.chain_depth > 0 ? `hop ${row.chain_depth}` : "hop 0");
  }
  if (row.incident_id) bits.push(`incident ${shortIncidentId(row.incident_id)}`);
  const owner = firstNonBlank(
    row.delegate_to,
    row.target_agent,
    row.to && row.to !== "*" ? row.to : "",
  );
  if (owner) bits.push(`next owner ${owner}`);
  return bits.join(" · ");
}

function shortIncidentId(id?: string): string {
  const raw = String(id || "").trim();
  if (!raw) return "";
  if (raw.length <= 18) return raw;
  return raw.slice(0, 18);
}

function firstNonBlank(...items: Array<string | undefined | null>): string {
  for (const item of items) {
    const value = String(item || "").trim();
    if (value) return value;
  }
  return "";
}

// agentScope resolves the memory scope an agent writes to: its explicit
// memory_scope, or — when blank — its slug (the kernel default, M915).
export function agentScope(slug: string, memoryScope?: string): string {
  const s = (memoryScope || "").trim();
  return s !== "" ? s : slug;
}

// agentCorrelations is the set of run correlation ids belonging to an agent
// (runs started AS this agent). Used to attribute journal-derived diagnostics
// (policy denials, tool errors) back to the agent.
export function agentCorrelations(runs: RunLite[], slug: string): Set<string> {
  const out = new Set<string>();
  for (const r of runs) {
    if (r.agent === slug && r.correlation_id) out.add(r.correlation_id);
  }
  return out;
}

// filterByCorrelation keeps diagnostics rows attributable to the agent: either
// the row's correlation belongs to one of the agent's runs, or the row's actor
// is the agent slug directly. Pure; preserves input order.
export function filterByCorrelation<T extends CorrelatedRow>(
  rows: T[],
  corrs: Set<string>,
  slug: string,
): T[] {
  return rows.filter(
    (r) =>
      (r.correlation_id != null && corrs.has(r.correlation_id)) ||
      r.actor === slug,
  );
}

// filterAgentMemory keeps records private to this agent: tagged with the agent's
// scope, or authored by the agent. Shared records (empty/absent scope) are
// excluded — they show in the global Memory view, not the agent's private one.
export function filterAgentMemory(
  records: MemoryRecord[],
  slug: string,
  memoryScope?: string,
): MemoryRecord[] {
  const scope = agentScope(slug, memoryScope);
  return records.filter(
    (r) => (r.tags?.scope || "") === scope || r.added_by === slug,
  );
}

// filterAgentSkills keeps skills owned by (private to) the agent (Skill.Agent
// == slug), mirroring the Roster per-agent skill count (M943).
export function filterAgentSkills(
  skills: SkillLite[],
  slug: string,
): SkillLite[] {
  return skills.filter((s) => s.agent === slug);
}

export interface AgentSummary {
  runs: number;
  totalSpentMc: number;
  lastStartedMs?: number;
}

// summarizeAgent folds an agent's own runs into the headline counters the
// Overview tab shows (run count, total spend, most-recent start).
export function summarizeAgent(runs: RunLite[], slug: string): AgentSummary {
  let count = 0;
  let totalSpentMc = 0;
  let lastStartedMs: number | undefined;
  for (const r of runs) {
    if (r.agent !== slug) continue;
    count++;
    totalSpentMc += r.spent_mc || 0;
    if (
      r.started_unix_ms != null &&
      (lastStartedMs == null || r.started_unix_ms > lastStartedMs)
    ) {
      lastStartedMs = r.started_unix_ms;
    }
  }
  return { runs: count, totalSpentMc, lastStartedMs };
}

// agentRunTrend folds an agent's runs into a runs-per-day series over the last
// `days` days (oldest → newest) — the header MetricWidget's trend sparkline.
export function agentRunTrend(
  runs: RunLite[],
  slug: string,
  days = 14,
  nowMs = Date.now(),
): number[] {
  const dayMs = 86_400_000;
  const out = new Array<number>(days).fill(0);
  const start = nowMs - (days - 1) * dayMs;
  for (const r of runs) {
    if (r.agent !== slug || !r.started_unix_ms) continue;
    const idx = Math.floor((r.started_unix_ms - start) / dayMs);
    if (idx >= 0 && idx < days) out[idx]++;
  }
  return out;
}

// lastFailure returns the most recent failed run for the agent (the "ne bok
// yedi" headline), or undefined when the agent has no failures.
export function lastFailure(
  runs: RunLite[],
  slug: string,
): RunLite | undefined {
  let worst: RunLite | undefined;
  for (const r of runs) {
    if (r.agent !== slug) continue;
    if ((r.status || "").toLowerCase() !== "failed") continue;
    if (!worst || (r.started_unix_ms || 0) > (worst.started_unix_ms || 0))
      worst = r;
  }
  return worst;
}

// healthSnapshot folds the reaper/degraded scan and the profile lifecycle into a
// single per-agent health line for the Command Center. Graveyard beats everything,
// then degraded, then stale/dead candidate, else healthy.
export function healthSnapshot(
  slug: string,
  retired: boolean | undefined,
  report?: ReaperReport | null,
): AgentHealthSnapshot {
  if (retired) {
    return {
      state: "retired",
      label: "graveyard",
      detail:
        "retired identity — inspectable, but it will not wake until revived",
    };
  }
  const misconfigured = (report?.misconfigured_agents || []).find(
    (r) => r.slug === slug,
  );
  const degraded = (report?.degraded_agents || []).find((r) => r.slug === slug);
  const routing = (report?.routing_pressure_agents || []).find(
    (r) => r.slug === slug,
  );
  const forced = (report?.routing_forced_probation_agents || []).find(
    (r) => r.slug === slug,
  );
  const forcedFailed = (report?.routing_forced_failed_agents || []).find(
    (r) => r.slug === slug,
  );
  const forcedExhausted = (report?.routing_forced_exhausted_agents || []).find(
    (r) => r.slug === slug,
  );
  const unstable = (report?.routing_unstable_agents || []).find(
    (r) => r.slug === slug,
  );
  if (forcedExhausted) {
    const generation =
      (forcedExhausted.routing_force_generation || 0) > 1
        ? `; generation ${forcedExhausted.routing_force_generation}`
        : "";
    return {
      state: "force_exhausted",
      label: "forced chain exhausted",
      detail: `${forcedExhausted.count ?? 0} model-chain fallback hop(s) in the last ${forcedExhausted.window_sec ?? 0}s after repeated forced-chain retries${forcedExhausted.task_type ? `; task ${forcedExhausted.task_type}` : ""}${forcedExhausted.forced_chain?.length ? `; forced ${forcedExhausted.forced_chain.join(" → ")}` : ""}${generation}${forcedExhausted.last_reason ? `; ${forcedExhausted.last_reason}` : ""}`,
      doctorAgent: forcedExhausted.doctor_agent,
      selfRepairEnabled: forcedExhausted.self_repair_enabled,
      escalateTo: forcedExhausted.escalate_to,
      lastFailureMs: forcedExhausted.last_forced_ms,
    };
  }
  if (forcedFailed) {
    const generation =
      (forcedFailed.routing_force_generation || 0) > 1
        ? `; generation ${forcedFailed.routing_force_generation}`
        : "";
    return {
      state: "force_failed",
      label: "forced chain failed",
      detail: `${forcedFailed.count ?? 0} model-chain fallback hop(s) in the last ${forcedFailed.window_sec ?? 0}s after forced-chain probation expired${forcedFailed.task_type ? `; task ${forcedFailed.task_type}` : ""}${forcedFailed.forced_chain?.length ? `; forced ${forcedFailed.forced_chain.join(" → ")}` : ""}${generation}${forcedFailed.last_reason ? `; ${forcedFailed.last_reason}` : ""}`,
      doctorAgent: forcedFailed.doctor_agent,
      selfRepairEnabled: forcedFailed.self_repair_enabled,
      escalateTo: forcedFailed.escalate_to,
      lastFailureMs: forcedFailed.last_forced_ms,
    };
  }
  if (unstable) {
    return {
      state: "unstable",
      label: "unstable routing",
      detail: `${unstable.count ?? 0} rollback-linked routing instability event(s) in the last ${unstable.window_sec ?? 0}s${unstable.task_type ? `; task ${unstable.task_type}` : ""}${unstable.current_chain?.length ? `; current ${unstable.current_chain.join(" → ")}` : ""}${unstable.previous_chain?.length ? `; previous ${unstable.previous_chain.join(" → ")}` : ""}${unstable.last_reason ? `; ${unstable.last_reason}` : ""}`,
      doctorAgent: unstable.doctor_agent,
      selfRepairEnabled: unstable.self_repair_enabled,
      escalateTo: unstable.escalate_to,
      lastFailureMs: unstable.last_rollback_ms,
    };
  }
  if (degraded) {
    const failures = degraded.failures ?? 0;
    const threshold = degraded.threshold ?? 0;
    const window = degraded.window ?? 0;
    const issueCount = misconfigured?.issues?.length ?? 0;
    return {
      state: "degraded",
      label: "degraded",
      detail: `${failures} failed run(s) in the last ${window} judged runs${threshold > 0 ? `; threshold ${threshold}` : ""}${issueCount > 0 ? `; ${issueCount} config/hierarchy issue(s)` : ""}`,
      doctorAgent: degraded.doctor_agent || misconfigured?.doctor_agent,
      selfRepairEnabled:
        degraded.self_repair_enabled ?? misconfigured?.self_repair_enabled,
      escalateTo: degraded.escalate_to || misconfigured?.escalate_to,
      lastFailureMs: degraded.last_failure_ms,
      configIssues: misconfigured?.issues ?? undefined,
    };
  }
  if (misconfigured) {
    const issues = misconfigured.issues || [];
    return {
      state: "misconfigured",
      label: "misconfigured",
      detail: `${issues.length || 1} config/hierarchy issue(s) need repair before this agent is reliable`,
      doctorAgent: misconfigured.doctor_agent,
      selfRepairEnabled: misconfigured.self_repair_enabled,
      escalateTo: misconfigured.escalate_to,
      configIssues: issues,
    };
  }
  if (forced) {
    const generation =
      (forced.routing_force_generation || 0) > 1
        ? `; generation ${forced.routing_force_generation}`
        : "";
    return {
      state: "stabilizing",
      label: "forced-chain probation",
      detail: `${forced.count ?? 0} model-chain fallback hop(s) in the last ${forced.window_sec ?? 0}s, but the owner-forced ${forced.task_type || "task"} chain is still on probation${forced.forced_chain?.length ? `; forced ${forced.forced_chain.join(" → ")}` : ""}${generation}${forced.last_reason ? `; ${forced.last_reason}` : ""}`,
      doctorAgent: forced.doctor_agent,
      selfRepairEnabled: forced.self_repair_enabled,
      escalateTo: forced.escalate_to,
      lastFailureMs: forced.last_forced_ms,
    };
  }
  if (routing) {
    return {
      state: "degraded",
      label: "fallback pressure",
      detail: `${routing.count ?? 0} model-chain fallback hop(s) in the last ${routing.window_sec ?? 0}s${routing.threshold ? `; threshold ${routing.threshold}` : ""}${routing.last_failed_model || routing.last_next_model ? `; latest ${routing.last_failed_model || "?"} → ${routing.last_next_model || "?"}` : ""}${routing.last_reason ? `; ${routing.last_reason}` : ""}`,
      doctorAgent: routing.doctor_agent,
      selfRepairEnabled: routing.self_repair_enabled,
      escalateTo: routing.escalate_to,
      lastFailureMs: routing.last_fallback_ms,
    };
  }
  const dead = (report?.dead_agents || []).find((r) => r.slug === slug);
  if (dead) {
    return {
      state: "stale",
      label: "stale",
      detail: dead.last_active_ms
        ? "idle past the reaper threshold"
        : "never woke and is past the reaper grace window",
      lastActiveMs: dead.last_active_ms,
    };
  }
  return {
    state: "healthy",
    label: "healthy",
    detail: "no reaper or degradation signal is active",
  };
}

export function summarizeAutoRepair(
  status?: AgentRepairStatus | null,
): AgentRepairSnapshot {
  const latest = status?.latest;
  if (!latest || !latest.phase) {
    return {
      state: "idle",
      label: "idle",
      detail: "no autonomous repair attempt has been recorded yet",
    };
  }
  const issues = latest.issues?.length ?? 0;
  const nextEligibleMs = latest.next_eligible_ms ?? status?.next_eligible_ms;
  const mode = latest.mode || "misconfigured";
  const forceGenSuffix =
    (latest.routing_force_generation || 0) > 1
      ? ` (gen ${latest.routing_force_generation})`
      : "";
  const modeLabel =
    mode === "degraded" ? "doctor" : mode === "routing" ? "routing repair" : mode === "routing_unstable" ? "unstable routing" : mode === "routing_forced_failed" ? "forced chain failed" : mode === "routing_forced_exhausted" ? "forced chain exhausted" : "repair";
  const attemptLabel =
    latest.self_repair_attempt && latest.self_repair_max_attempts
      ? `${latest.self_repair_attempt}/${latest.self_repair_max_attempts}`
      : "";
  switch (latest.phase) {
    case "routing_force_exhausted_detected":
      return {
        state: "failed",
        label: "forced chain exhausted",
        detail:
          latest.routing_task_type
            ? `owner-forced ${latest.routing_task_type} chain stayed unstable through repeated forced generations; escalated for human-grade ownership${forceGenSuffix}`
            : `owner-forced chain stayed unstable through repeated forced generations; escalated for human-grade ownership${forceGenSuffix}`,
        mode,
        nextEligibleMs,
      };
    case "routing_forced_failed_detected":
      return {
        state: "failed",
        label: "forced chain failed",
        detail:
          latest.routing_task_type
            ? `owner-forced ${latest.routing_task_type} chain stayed under fallback pressure after probation; escalated again${forceGenSuffix}`
            : `owner-forced chain stayed under fallback pressure after probation; escalated again${forceGenSuffix}`,
        mode,
        nextEligibleMs,
      };
    case "routing_unstable_detected":
      return {
        state: "failed",
        label: "unstable routing",
        detail:
          latest.routing_task_type
            ? `routing stayed unstable after rollback for ${latest.routing_task_type}; escalated to the owner/doctor chain`
            : "routing stayed unstable after rollback; escalated to the owner/doctor chain",
        mode,
        nextEligibleMs,
      };
    case "attempts_exhausted":
      return {
        state: "failed",
        label: "repair exhausted",
        detail: attemptLabel
          ? `self-repair attempt budget exhausted (${attemptLabel}); escalated to owner/parent governance`
          : "self-repair attempt budget exhausted; escalated to owner/parent governance",
        mode,
        nextEligibleMs,
      };
    case "queued":
      return {
        state: "queued",
        label: mode === "degraded" ? "doctor queued" : mode === "routing" ? "routing queued" : "repair queued",
        detail:
          mode === "degraded"
            ? "autonomous doctor run is queued or in flight for a degraded agent"
            : mode === "routing"
            ? "autonomous routing repair is queued or in flight for repeated model fallback pressure"
            : `autonomous repair is queued or in flight${issues > 0 ? ` for ${issues} issue(s)` : ""}`,
        mode,
        nextEligibleMs,
      };
    case "routing_rollback_queued":
      return {
        state: "queued",
        label: "rollback queued",
        detail:
          latest.routing_task_type && latest.routing_task_model_chain?.length
            ? `routing rollback is queued to restore ${latest.routing_task_type} → ${latest.routing_task_model_chain.join(" → ")}`
            : latest.routing_task_type
              ? `routing rollback is queued for ${latest.routing_task_type}`
              : "routing rollback is queued",
        mode,
        nextEligibleMs,
      };
    case "completed":
      return {
        state: "completed",
        label:
          mode === "degraded"
            ? "doctor repaired"
            : mode === "routing"
              ? "routing stabilized"
              : "repaired",
        detail:
          mode === "degraded"
            ? latest.applied?.length
              ? `doctor run completed and applied ${latest.applied.length} profile change(s)`
              : "doctor run completed without a persisted profile change"
            : mode === "routing"
              ? latest.routing_task_type && latest.routing_task_model_chain?.length
                ? `routing repair rewrote ${latest.routing_task_type} → ${latest.routing_task_model_chain.join(" → ")}`
                : latest.routing_task_type
                  ? `routing repair rewrote the ${latest.routing_task_type} task chain`
                  : latest.applied?.length
                    ? `routing repair completed and applied ${latest.applied.length} profile change(s)`
                    : "routing repair completed without a persisted profile change"
            : latest.applied?.length
              ? `completed and applied ${latest.applied.length} profile change(s)`
              : "completed without a persisted profile change",
        mode,
        nextEligibleMs,
      };
    case "routing_rollback_completed":
      return {
        state: "completed",
        label: "rolled back",
        detail:
          latest.routing_task_type && latest.routing_task_model_chain?.length
            ? `routing rollback restored ${latest.routing_task_type} → ${latest.routing_task_model_chain.join(" → ")}${
                latest.previous_routing_task_model_chain?.length
                  ? ` from ${latest.previous_routing_task_model_chain.join(" → ")}`
                  : ""
              }`
            : latest.routing_task_type
              ? `routing rollback restored the ${latest.routing_task_type} task chain`
              : "routing rollback completed",
        mode,
        nextEligibleMs,
      };
    case "failed":
      return {
        state: "failed",
        label:
          mode === "degraded"
            ? "doctor failed"
            : mode === "routing"
              ? "routing failed"
              : "repair failed",
        detail: latest.error
          ? `autonomous ${modeLabel} failed: ${latest.error}`
          : `autonomous ${modeLabel} failed`,
        mode,
        nextEligibleMs,
      };
    case "routing_rollback_failed":
      return {
        state: "failed",
        label: "rollback failed",
        detail: latest.error
          ? `deterministic routing rollback failed: ${latest.error}`
          : latest.reason
            ? `deterministic routing rollback failed: ${latest.reason}`
            : "deterministic routing rollback failed",
        mode,
        nextEligibleMs,
      };
    case "resolution_failed":
      return {
        state: "failed",
        label: latest.resolution
          ? `${latest.resolution} failed`
          : "resolution failed",
        detail: latest.reason
          ? `deterministic follow-up after the escalation failed: ${latest.reason}`
          : "deterministic follow-up after the escalation failed",
        mode,
        nextEligibleMs,
      };
    case "delegation_queued":
      return {
        state: "queued",
        label: "delegation queued",
        detail: latest.delegate_to
          ? `the escalation was delegated onward to ${latest.delegate_to}${latest.resolution_summary ? `: ${latest.resolution_summary}` : ""}`
          : "the escalation was delegated onward",
        mode,
        nextEligibleMs,
      };
    case "delegation_woke":
      return {
        state: "completed",
        label: "delegation woke",
        detail: latest.delegate_to
          ? `${latest.delegate_to} was woken to take the delegated follow-up${latest.resolution_summary ? `: ${latest.resolution_summary}` : ""}`
          : "a delegated agent was woken to take the follow-up",
        mode,
        nextEligibleMs,
      };
    case "delegation_failed":
      return {
        state: "failed",
        label: "delegation failed",
        detail: latest.delegate_to
          ? `delegated wake for ${latest.delegate_to} failed${latest.reason ? `: ${latest.reason}` : ""}`
          : `delegated wake failed${latest.reason ? `: ${latest.reason}` : ""}`,
        mode,
        nextEligibleMs,
      };
    case "escalation_answered":
      if (latest.resolution === "delegated") {
        return {
          state: "completed",
          label: "manager delegated",
          detail: latest.delegate_to
            ? `${latest.target_agent || "an owner/parent agent"} delegated the escalation to ${latest.delegate_to}${latest.resolution_summary ? `: ${latest.resolution_summary}` : ""}`
            : `${latest.target_agent || "an owner/parent agent"} delegated the escalation${latest.resolution_summary ? `: ${latest.resolution_summary}` : ""}`,
          mode,
          nextEligibleMs,
        };
      }
      if (latest.resolution === "force_chain") {
        return {
          state: "completed",
          label: "manager forced chain",
          detail:
            latest.routing_task_type && latest.routing_task_model_chain?.length
              ? `${latest.target_agent || "an owner/parent agent"} forced ${latest.routing_task_type} → ${latest.routing_task_model_chain.join(" → ")}${forceGenSuffix}${latest.resolution_summary ? `: ${latest.resolution_summary}` : ""}`
              : `${latest.target_agent || "an owner/parent agent"} forced a routing chain${forceGenSuffix}${latest.resolution_summary ? `: ${latest.resolution_summary}` : ""}`,
          mode,
          nextEligibleMs,
        };
      }
      if (latest.resolution === "retired") {
        return {
          state: "completed",
          label: "manager retired",
          detail: `${latest.target_agent || "an owner/parent agent"} retired the agent after the escalation${latest.resolution_summary ? `: ${latest.resolution_summary}` : ""}`,
          mode,
          nextEligibleMs,
        };
      }
      if (latest.resolution === "paused") {
        return {
          state: "completed",
          label: "manager paused",
          detail: `${latest.target_agent || "an owner/parent agent"} paused the agent after the escalation${latest.resolution_summary ? `: ${latest.resolution_summary}` : ""}`,
          mode,
          nextEligibleMs,
        };
      }
      if (latest.resolution === "blocked") {
        return {
          state: "failed",
          label: "manager blocked",
          detail: `${latest.target_agent || "an owner/parent agent"} marked the escalation blocked${latest.resolution_summary ? `: ${latest.resolution_summary}` : ""}`,
          mode,
          nextEligibleMs,
        };
      }
      return {
        state: "completed",
        label: "manager answered",
        detail: latest.target_agent
          ? `${latest.target_agent} answered the escalation thread and closed the handoff${latest.resolution_summary ? `: ${latest.resolution_summary}` : ""}`
          : `an owner/parent agent answered the escalation thread and closed the handoff${latest.resolution_summary ? `: ${latest.resolution_summary}` : ""}`,
        mode,
        nextEligibleMs,
      };
    case "resolution_applied":
      if (latest.resolution === "force_chain") {
        return {
          state: "completed",
          label: "manager forced chain",
          detail:
            latest.routing_task_type && latest.routing_task_model_chain?.length
              ? `${latest.target_agent || "an owner/parent agent"} applied a forced ${latest.routing_task_type} → ${latest.routing_task_model_chain.join(" → ")}${forceGenSuffix}${latest.resolution_summary ? `: ${latest.resolution_summary}` : ""}`
              : `${latest.target_agent || "an owner/parent agent"} applied a forced routing chain${forceGenSuffix}${latest.resolution_summary ? `: ${latest.resolution_summary}` : ""}`,
          mode,
          nextEligibleMs,
        };
      }
      if (latest.resolution === "retired") {
        return {
          state: "completed",
          label: "manager retired",
          detail: `${latest.target_agent || "an owner/parent agent"} retirement was applied${latest.resolution_summary ? `: ${latest.resolution_summary}` : ""}`,
          mode,
          nextEligibleMs,
        };
      }
      if (latest.resolution === "paused") {
        return {
          state: "completed",
          label: "manager paused",
          detail: `${latest.target_agent || "an owner/parent agent"} pause was applied${latest.resolution_summary ? `: ${latest.resolution_summary}` : ""}`,
          mode,
          nextEligibleMs,
        };
      }
      return {
        state: "completed",
        label: "manager applied",
        detail: `${latest.target_agent || "an owner/parent agent"} applied the escalation resolution${latest.resolution_summary ? `: ${latest.resolution_summary}` : ""}`,
        mode,
        nextEligibleMs,
      };
    case "escalation_woke":
      return {
        state: "failed",
        label: "manager woke",
        detail: latest.target_agent
          ? `self-repair failed, then woke ${latest.target_agent} to take over`
          : "self-repair failed, then woke an owner/parent agent to take over",
        mode,
        nextEligibleMs,
      };
    case "escalation_skipped":
      return {
        state: "failed",
        label: "wake skipped",
        detail:
          latest.reason ||
          "self-repair failed, but no eligible owner/parent agent could be woken",
        mode,
        nextEligibleMs,
      };
    case "escalation_failed":
      return {
        state: "failed",
        label: "wake failed",
        detail:
          latest.reason ||
          "self-repair failed, and the owner/parent wake attempt also failed",
        mode,
        nextEligibleMs,
      };
    default:
      return {
        state: "idle",
        label: latest.phase,
        detail: latest.reason || "autonomous repair event recorded",
        mode,
        nextEligibleMs,
      };
  }
}

export interface AgentPolicyDenialSummary {
  count: number;
  text: string;
  detail: string;
  tone: "bad" | "warn" | "muted";
}

// summarizeAgentPolicyDenials renders the runtime's tool-refusal audit for one
// agent: how many tool calls policy actually blocked, and the most recent one.
// Empty/zero is the healthy "nothing refused" case.
export function summarizeAgentPolicyDenials(
  status?: Pick<
    AgentRuntimeStatus,
    | "policy_denied_count"
    | "policy_denied_last_tool"
    | "policy_denied_last_reason"
    | "policy_denied_last_capability"
    | "policy_denied_last_hard"
  > | null,
): AgentPolicyDenialSummary {
  const count = status?.policy_denied_count ?? 0;
  if (count <= 0) {
    return {
      count: 0,
      text: "no tool calls refused",
      detail: "policy allowed every tool call this agent attempted",
      tone: "muted",
    };
  }
  const tool = (status?.policy_denied_last_tool || "a tool").trim();
  const cap = (status?.policy_denied_last_capability || "").trim();
  const reason = (status?.policy_denied_last_reason || "").trim();
  const hard = status?.policy_denied_last_hard === true;
  return {
    count,
    text: `${count} tool call${count === 1 ? "" : "s"} refused`,
    detail: [`last: ${tool}${cap ? ` (${cap})` : ""}`, hard ? "hard-denied" : "", reason]
      .filter(Boolean)
      .join(" · "),
    tone: hard ? "bad" : "warn",
  };
}

export interface WakeLineage {
  label: string;
  incidentId?: string;
  parentCorrelationId?: string;
}

// wakeLineage turns the latest folded wake runbook into a navigable lineage: the
// human source label plus the targets an operator can follow — a doctor wake's
// incident (deep-linkable) and a delegated wake's parent/lead run correlation.
export function wakeLineage(rb?: AgentLastAutonomyRunbook): WakeLineage {
  const label = lastAutonomyRunbookSourceLabel(rb);
  if (!rb) return { label: "" };
  return {
    label,
    incidentId: (rb.incident_id || "").trim() || undefined,
    parentCorrelationId: (rb.parent_correlation_id || "").trim() || undefined,
  };
}

// EscalationCausalityLineage is the comms causality lineage for a delegated or
// doctor escalation: it ties the board message that surfaced the incident to the
// wake/run it actually triggered (wakeCorrelationId) and the full incident chain
// (root → parent → current) so an operator can drill from "this message woke the
// agent" through to the run and the originating incident. Mirrors mailboxWakeFor
// (message → mailbox wake) for the delegated/doctor escalation route.
export interface EscalationCausalityLineage {
  // doctor | delegated — empty when the row carries no wake provenance at all.
  origin: "doctor" | "delegated" | "";
  // the run the escalation woke on this agent (target_correlation); the link from
  // the message to the causally downstream run. undefined when the wake failed or
  // the envelope predates correlation capture.
  wakeCorrelationId?: string;
  // the incident chain — root (originating agent), parent (delegation hop), and
  // current (this agent's incident). All deep-linkable.
  rootIncidentId?: string;
  parentIncidentId?: string;
  incidentId?: string;
  // who the escalation was handed to next (delegated resolutions only).
  nextOwner?: string;
  // provenance for the chain label — the agent whose failure started it and how
  // many delegation hops deep this wake sits.
  rootAgent?: string;
  chainDepth?: number;
  // human-readable causality line, e.g. "woke via doctor for builder" /
  // "woke via delegation by lead (hop 2)".
  label: string;
}

// escalationCausalityLineage folds a delegated/doctor escalation row into the
// structured, navigable causality the Comms tab renders. The label is the
// "message → wake" headline; the ids are the drill targets. Returns an empty
// origin (label "") when the row carries no wake provenance, so callers can gate
// rendering on `lineage.origin` alone.
export function escalationCausalityLineage(
  row:
    | Pick<
        AgentEscalation,
        | "origin_kind"
        | "origin_agent"
        | "wake_correlation_id"
        | "incident_id"
        | "root_incident_id"
        | "parent_incident_id"
        | "delegate_to"
        | "root_agent"
        | "chain_depth"
      >
    | null
    | undefined,
): EscalationCausalityLineage {
  const base: EscalationCausalityLineage = {
    origin: "",
    label: "",
  };
  if (!row) return base;
  const origin = (row.origin_kind || "").trim();
  if (origin !== "doctor" && origin !== "delegated") return base;
  const originAgent = (row.origin_agent || "").trim();
  const rootAgent = (row.root_agent || "").trim();
  const wakeCorr = (row.wake_correlation_id || "").trim() || undefined;
  const rootIncident = (row.root_incident_id || "").trim() || undefined;
  const parentIncident = (row.parent_incident_id || "").trim() || undefined;
  const incident = (row.incident_id || "").trim() || undefined;
  const nextOwner = (row.delegate_to || "").trim() || undefined;
  const depth =
    typeof row.chain_depth === "number" ? row.chain_depth : undefined;

  // Causality headline: "woke via <route> [by/for <agent>][ (hop N)]".
  const route =
    origin === "delegated"
      ? originAgent
        ? `via delegation by ${originAgent}`
        : "via delegation"
      : originAgent
        ? `via doctor for ${originAgent}`
        : "via doctor";
  const bits: string[] = [`woke ${route}`];
  if (typeof depth === "number" && depth > 0) bits.push(`hop ${depth}`);
  const label = bits.join(" · ");

  return {
    origin,
    wakeCorrelationId: wakeCorr,
    rootIncidentId: rootIncident,
    parentIncidentId: parentIncident,
    incidentId: incident,
    nextOwner,
    rootAgent: rootAgent || undefined,
    chainDepth: depth,
    label,
  };
}

export function summarizeAgentRuntimeStatus(
  status?: AgentRuntimeStatus | null,
): AgentCardRuntimeSummary {
  const configIssues = Array.isArray(status?.config_issues) ? status.config_issues : [];
  const misconfigurationCount =
    status?.misconfiguration_count ?? status?.invalid_runtime_overrides ?? configIssues.length;
  const repairInflight = status?.repair_inflight ?? 0;
  const routingFallbackCount = status?.routing_fallback_count ?? 0;
  let healthText: string | undefined;
  let healthTone: AgentCardRuntimeSummary["healthTone"] = "good";
  switch (status?.health_state) {
    case "force_failed":
      healthText = status.health_label || "forced chain failed";
      healthTone = "bad";
      break;
    case "force_exhausted":
      healthText = status.health_label || "forced chain exhausted";
      healthTone = "bad";
      break;
    case "stabilizing":
      healthText = status.health_label || "forced-chain probation";
      healthTone = "muted";
      break;
    case "unstable":
      healthText = status.health_label || "unstable routing";
      healthTone = "bad";
      break;
    case "degraded":
      healthText = status.health_label || "degraded";
      healthTone = "bad";
      break;
    case "misconfigured":
      healthText =
        misconfigurationCount > 0
          ? `cfg/links !${misconfigurationCount}`
          : status.health_label || "misconfigured";
      healthTone = "bad";
      break;
    case "stale":
      healthText = status.health_label || "stale";
      healthTone = "muted";
      break;
    case "retired":
      healthText = status.health_label || "graveyard";
      healthTone = "muted";
      break;
  }

  let repairText: string | undefined;
  let repairTone: AgentCardRuntimeSummary["repairTone"] = "muted";
  const repairMode = status?.repair_mode || "misconfigured";
  const repairState = status?.repair_state || "";
  const repairQueued =
    repairState === "queued" || repairState === "routing_rollback_queued";
  const repairFailed =
    repairState === "failed" || repairState === "routing_rollback_failed" || repairState === "routing_unstable_detected" || repairState === "routing_forced_failed_detected" || repairState === "routing_force_exhausted_detected" || repairState === "attempts_exhausted";
  const repairCompleted =
    repairState === "completed" || repairState === "routing_rollback_completed" || repairState === "resolution_applied";
  let repairKindText: string | undefined;
  let repairKindTone: AgentCardRuntimeSummary["repairKindTone"] = "muted";
  if (repairInflight > 0 || repairQueued) {
    repairKindText =
      repairMode === "degraded"
        ? "doctor"
        : repairMode === "routing" || repairMode === "routing_unstable" || repairMode === "routing_forced_failed" || repairMode === "routing_forced_exhausted"
          ? "routing"
          : "repair";
    repairKindTone = repairMode === "degraded" ? "warn" : "accent";
    repairText =
      repairInflight > 0
        ? `${repairMode === "degraded" ? "doctor" : repairMode === "routing" ? "routing" : "repair"} ${repairInflight}`
        : repairState === "routing_rollback_queued"
          ? "rollback queued"
        : repairMode === "degraded"
          ? "doctor queued"
          : repairMode === "routing"
            ? "routing queued"
          : "repair queued";
    repairTone = "accent";
  } else if (repairFailed) {
    repairKindText =
      repairMode === "degraded"
        ? "doctor"
        : repairMode === "routing" || repairMode === "routing_unstable" || repairMode === "routing_forced_failed" || repairMode === "routing_forced_exhausted"
          ? "routing"
          : "repair";
    repairKindTone = repairMode === "degraded" ? "warn" : "accent";
    repairText = status?.repair_label || (repairState === "attempts_exhausted" ? "repair exhausted" : repairState === "routing_unstable_detected" ? "unstable routing" : repairState === "routing_forced_failed_detected" ? "forced chain failed" : repairState === "routing_force_exhausted_detected" ? "forced chain exhausted" : "repair failed");
    repairTone = "bad";
  } else if (repairCompleted) {
    repairKindText =
      repairMode === "degraded"
        ? "doctor"
        : repairMode === "routing" || repairMode === "routing_unstable" || repairMode === "routing_forced_failed" || repairMode === "routing_forced_exhausted"
          ? "routing"
          : "repair";
    repairKindTone = repairMode === "degraded" ? "warn" : "accent";
    repairText = status?.repair_label || (repairState === "resolution_applied" ? "manager applied" : "repaired");
    repairTone = "good";
  }
  const repairDetail = repairText
    ? [
        status?.repair_self_attempt && status?.repair_self_max_attempts ? `attempt ${status.repair_self_attempt}/${status.repair_self_max_attempts}` : "",
        status?.repair_last_error ? `last error: ${status.repair_last_error}` : "",
        status?.repair_next_eligible_ms ? `next eligible ${new Date(status.repair_next_eligible_ms).toLocaleString()}` : "",
      ].filter(Boolean).join(" · ")
    : undefined;
  const repairIncidentId = status?.repair_root_incident_id || status?.repair_incident_id || undefined;
  const repairIncidentText = repairIncidentId ? "incident" : undefined;
  const repairIncidentDetail = repairIncidentId
    ? [
        status?.repair_root_agent ? `root ${status.repair_root_agent}` : "",
        typeof status?.repair_chain_depth === "number" ? `hop ${status.repair_chain_depth}` : "",
        status?.repair_incident_id ? `incident ${shortIncidentId(status.repair_incident_id)}` : "",
        status?.repair_root_incident_id && status.repair_root_incident_id !== status.repair_incident_id
          ? `root ${shortIncidentId(status.repair_root_incident_id)}`
          : "",
      ].filter(Boolean).join(" · ")
    : undefined;
  const routingText =
    routingFallbackCount > 0 ? `fallback ${routingFallbackCount}` : undefined;
  const routingTone: AgentCardRuntimeSummary["routingTone"] =
    routingFallbackCount > 0 ? "bad" : "muted";
  const routingDetail =
    routingFallbackCount > 0
      ? [status?.routing_last_failed, status?.routing_last_next]
          .filter(Boolean)
          .join(" → ") || status?.routing_last_reason
      : undefined;
  const retryCount = status?.retry_count ?? 0;
  const retryText = retryCount > 0 ? `retry ${retryCount}` : undefined;
  const retryTone: AgentCardRuntimeSummary["retryTone"] = retryCount > 0 ? "bad" : "muted";
  const retryDetail = retryCount > 0
    ? [
        status?.retry_next_attempt && status?.retry_max_attempts
          ? `attempt ${status.retry_next_attempt}/${status.retry_max_attempts}`
          : "",
        status?.retry_last_reason || "",
        status?.retry_last_ts_ms ? `last ${new Date(status.retry_last_ts_ms).toLocaleString()}` : "",
      ].filter(Boolean).join(" · ")
    : undefined;
  const escalationOpenCount = status?.escalation_open_count ?? 0;
  const escalationAckedCount = status?.escalation_acked_count ?? 0;
  const escalationText =
    escalationOpenCount > 0
      ? `esc ${escalationOpenCount}`
      : escalationAckedCount > 0
        ? `acked ${escalationAckedCount}`
        : undefined;
  const escalationTone: AgentCardRuntimeSummary["escalationTone"] =
    escalationOpenCount > 0 ? "bad" : escalationAckedCount > 0 ? "accent" : "muted";
  const activeRunCount = status?.active_run_count ?? 0;
  const liveText = activeRunCount > 0 ? `running ${activeRunCount}` : undefined;
  const operationalState = status?.operational_state || (activeRunCount > 0 ? "running" : "sleeping");
  const operationalText = status?.operational_label || operationalState;
  const activeContextDetail = activeRunCount > 0
    ? [
        status?.active_wake_source ? `source: ${status.active_wake_source}` : "",
        status?.active_wake_reason ? `reason: ${status.active_wake_reason}` : "",
        status?.active_schedule_id ? `schedule: ${status.active_schedule_id}` : "",
        status?.active_standing_name
          ? `standing: ${status.active_standing_name}`
          : status?.active_standing_id
            ? `standing: ${status.active_standing_id}`
            : "",
        status?.active_trigger_subject ? `trigger: ${status.active_trigger_subject}` : "",
        status?.active_parent_correlation ? `parent: ${status.active_parent_correlation}` : "",
      ].filter(Boolean).join(" · ")
    : undefined;
  const liveDetail = activeRunCount > 0
    ? [
        status?.active_phase,
        status?.active_intent,
        status?.active_detail,
        status?.active_tool ? `tool: ${status.active_tool}` : "",
        activeContextDetail,
        status?.active_model ? `model: ${status.active_model}` : "",
        status?.active_correlation_id ? `corr: ${status.active_correlation_id}` : "",
      ].filter(Boolean).join(" · ")
    : undefined;
  const wakeScheduleCount = status?.wake_schedule_count ?? 0;
  const wakeStandingCount = status?.wake_standing_count ?? 0;
  const wakeTotal = wakeScheduleCount + wakeStandingCount;
  const wakeText = wakeTotal > 0 ? `wake ${wakeTotal}` : undefined;
  const wakeDetail = wakeTotal > 0
    ? [
        wakeScheduleCount > 0 ? `${wakeScheduleCount} schedule` : "",
        wakeStandingCount > 0 ? `${wakeStandingCount} standing` : "",
        status?.next_wake_label ? `next: ${status.next_wake_label}` : "",
        (status?.wake_event_subjects || []).length > 0 ? `events: ${(status?.wake_event_subjects || []).slice(0, 3).join(", ")}` : "",
      ].filter(Boolean).join(" · ")
    : undefined;

  return {
    healthText,
    healthTone,
    repairKindText,
    repairKindTone,
    repairText,
    repairDetail,
    repairTone,
    repairIncidentText,
    repairIncidentId,
    repairIncidentDetail,
    routingText,
    routingTone,
    routingDetail,
    retryText,
    retryTone,
    retryDetail,
    escalationText,
    escalationTone,
    liveText,
    liveDetail,
    liveTone: activeRunCount > 0 ? "accent" : "muted",
    wakeText,
    wakeDetail,
    wakeTone: wakeTotal > 0 ? "accent" : "muted",
    escalationOpenCount,
    escalationAckedCount,
    activeRunCount,
    activeCorrelationId: status?.active_correlation_id,
    activeStartedMs: status?.active_started_ms,
    activeLastEventMs: status?.active_last_event_ms,
    activePhase: status?.active_phase,
    activeTool: status?.active_tool,
    activeModel: status?.active_model,
    activeContextDetail,
    operationalText,
    operationalState,
    lastActivityMs: status?.last_activity_ms,
    lastActivitySummary: status?.last_activity_summary,
    lastAutonomyRunbook: status?.last_autonomy_runbook,
    wakeScheduleCount,
    wakeStandingCount,
    routingFallbackCount,
    retryCount,
    invalidRuntimeOverrides: misconfigurationCount,
    configIssues,
    repairInflight,
    nextRepairEligibleMs: status?.repair_next_eligible_ms,
    nextWakeMs: status?.next_wake_ms,
  };
}

// FleetCardIssues collapses an agent's runtime-status signals (health, routing
// fallbacks, retries, escalations, in-flight repair) into ONE indicator for the
// calm identity card: a count, the worst tone, and a "·"-joined detail for the
// tooltip. Healthy → count 0 (the card shows no issue chip).
export interface FleetCardIssues {
  count: number;
  tone: "bad" | "warn" | "accent" | "none";
  detail: string;
}

export function fleetCardIssueSummary(rs: AgentCardRuntimeSummary | null | undefined): FleetCardIssues {
  if (!rs) return { count: 0, tone: "none", detail: "" };
  const parts: { label: string; tone: "bad" | "warn" | "accent" }[] = [];
  if (rs.healthTone === "bad" && rs.healthText) parts.push({ label: rs.healthText, tone: "bad" });
  if (rs.routingFallbackCount > 0) parts.push({ label: `routing fallback ×${rs.routingFallbackCount}`, tone: "bad" });
  if (rs.retryCount > 0) parts.push({ label: `retry ×${rs.retryCount}`, tone: "bad" });
  if (rs.escalationOpenCount > 0) {
    parts.push({
      label: `${rs.escalationOpenCount} open escalation${rs.escalationOpenCount === 1 ? "" : "s"}`,
      tone: rs.escalationTone === "bad" ? "bad" : "warn",
    });
  }
  if (rs.repairInflight > 0) parts.push({ label: rs.repairText || `repairing ×${rs.repairInflight}`, tone: "accent" });
  const tone: FleetCardIssues["tone"] = parts.some((p) => p.tone === "bad")
    ? "bad"
    : parts.some((p) => p.tone === "warn")
      ? "warn"
      : parts.some((p) => p.tone === "accent")
        ? "accent"
        : "none";
  return { count: parts.length, tone, detail: parts.map((p) => p.label).join(" · ") };
}

export function summarizeProviderRoutingRow(
  row: ProviderRoutingRow,
): ProviderRoutingSummary {
  if (row.kind === "fallback") {
    return {
      kindLabel: "fallback",
      kindTone: "bad",
      stateLabel: "degraded",
      stateTone: "bad",
      primaryText: `${row.failed || "?"} → ${row.next || "?"}`,
      failedModel: row.failed || undefined,
      nextModel: row.next || undefined,
      secondaryText: [row.task_type, row.scope].filter(Boolean).join(" · ") || undefined,
      detail: row.reason || undefined,
    };
  }
  return {
    kindLabel: row.kind || "route",
    kindTone: "muted",
    stateLabel: "normal route",
    stateTone: "good",
    primaryText: row.primary || "?",
    primaryModel: row.primary || undefined,
    secondaryText: [row.task_type, row.scope].filter(Boolean).join(" · ") || undefined,
    detail: row.chain || row.reason || undefined,
  };
}

export function summarizeEscalations(
  rows?: AgentEscalation[] | null,
): AgentEscalationSummary {
  let openCount = 0;
  let ackedCount = 0;
  let doctorOpenCount = 0;
  let delegatedOpenCount = 0;
  let latest: AgentEscalation | undefined;
  for (const row of rows || []) {
    if (!latest || (row.ts_unix_ms || 0) > (latest.ts_unix_ms || 0))
      latest = row;
    if (row.status === "open") {
      openCount += 1;
      if (row.origin_kind === "delegated") delegatedOpenCount += 1;
      else doctorOpenCount += 1;
    }
    if (row.status === "acked") ackedCount += 1;
  }
  return { openCount, ackedCount, doctorOpenCount, delegatedOpenCount, latest };
}

export function escalationOperationalTasks(
  rows?: AgentEscalation[] | null,
): AgentOperationalTask[] {
  const out: AgentOperationalTask[] = [];
  for (const row of rows || []) {
    const id = String(row.message_id || "").trim();
    if (!id) continue;
    const source = String(row.source_agent || "unknown").trim();
    const mode = row.mode === "degraded" ? "degraded doctor" : "config repair";
    let status: AgentOperationalTask["status"] = "todo";
    let action = "take ownership";
    if (row.status === "acked") {
      status = "doing";
      action = "acknowledged, follow through";
    } else if (row.status === "answered") {
      status = "done";
      action = "answered";
    }
    if (row.wake_phase === "escalation_failed") {
      status = "blocked";
      action = "wake failed, inspect manually";
    }
    if (row.resolution === "delegated" && row.delegate_to) {
      action = `delegated to ${row.delegate_to}`;
    } else if (row.resolution === "force_chain" && row.resolution_summary) {
      action = row.resolution_summary;
    } else if (row.resolution === "retired") {
      action = "retired";
    } else if (row.resolution === "paused") {
      action = "paused";
    } else if (row.resolution === "blocked") {
      status = "blocked";
      action = "blocked, inspect manually";
    }
    const chain = escalationChainLabel(row);
    const incident = incidentLineageLabel(row);
    out.push({
      id: `escalation:${id}`,
      title: `Handle escalation for ${source}`,
      description: `${mode} · ${action}${chain ? ` · ${chain}` : ""}${incident ? ` · ${incident}` : ""}${row.resolution_summary ? ` · ${row.resolution_summary}` : row.text ? ` · ${row.text}` : ""}`,
      scope: "total",
      status,
      source: "escalation",
      ts_unix_ms: row.ts_unix_ms,
    });
  }
  out.sort((a, b) => (b.ts_unix_ms || 0) - (a.ts_unix_ms || 0));
  return out;
}

const BOOL_TRUE = new Set(["1", "true", "yes", "on", "enabled"]);
const BOOL_FALSE = new Set(["0", "false", "no", "off", "disabled"]);
const DURATION_RE = /^([0-9]+(?:\.[0-9]+)?(ns|us|µs|ms|s|m|h))+$/;

function parseBool(v: string): boolean | null {
  const key = v.trim().toLowerCase();
  if (BOOL_TRUE.has(key)) return true;
  if (BOOL_FALSE.has(key)) return false;
  return null;
}

function parseIntStrict(v: string): number | null {
  const n = Number(v.trim());
  return Number.isInteger(n) ? n : null;
}

function runtimeOverride(
  key: string,
  value: string,
): AgentRuntimeOverride | null {
  const raw = value.trim();
  switch (key) {
    case "AGEZT_MODEL":
      return {
        key,
        value,
        label: "model",
        effect: raw
          ? `forces primary model to ${raw}`
          : "empty model override will be ignored",
        valid: raw !== "",
        issue: raw !== "" ? undefined : "value is blank",
      };
    case "AGEZT_MAX_ITER": {
      const n = parseIntStrict(raw);
      return {
        key,
        value,
        label: "max iter",
        effect:
          n == null
            ? "invalid max-iter override"
            : `caps the loop at ${n} tool round(s)`,
        valid: n != null,
        issue: n == null ? "must be an integer" : undefined,
      };
    }
    case "AGEZT_MAX_AUTO_CONTINUE": {
      const n = parseIntStrict(raw);
      return {
        key,
        value,
        label: "auto continue",
        effect:
          n == null
            ? "invalid auto-continue override"
            : n < 0
              ? "disables automatic continuation"
              : `allows ${n} auto-continue hop(s)`,
        valid: n != null,
        issue: n == null ? "must be an integer" : undefined,
      };
    }
    case "AGEZT_AUTO_CONTINUE_WAIT":
      return {
        key,
        value,
        label: "auto continue wait",
        effect: DURATION_RE.test(raw)
          ? `waits ${raw} between auto-continue hops`
          : "invalid duration override",
        valid: DURATION_RE.test(raw),
        issue: DURATION_RE.test(raw)
          ? undefined
          : "must be a Go duration like 250ms, 2s, 1m30s",
      };
    case "AGEZT_PARALLEL_TOOLS": {
      const n = parseIntStrict(raw);
      return {
        key,
        value,
        label: "parallel tools",
        effect:
          n == null
            ? "invalid parallel-tools override"
            : n <= 1
              ? "forces sequential tool execution"
              : `runs up to ${n} tool call(s) in parallel`,
        valid: n != null,
        issue: n == null ? "must be an integer" : undefined,
      };
    }
    case "AGEZT_TOOL_DISCOVERY_MAX": {
      const n = parseIntStrict(raw);
      return {
        key,
        value,
        label: "tool discovery",
        effect:
          n == null
            ? "invalid tool-discovery override"
            : `surfaces up to ${n} discovered tool(s) per query`,
        valid: n != null,
        issue: n == null ? "must be an integer" : undefined,
      };
    }
    case "AGEZT_CONTEXT_BUDGET": {
      const n = parseIntStrict(raw);
      return {
        key,
        value,
        label: "context budget",
        effect:
          n == null
            ? "invalid context-budget override"
            : `caps runtime context budget at ${n}`,
        valid: n != null,
        issue: n == null ? "must be an integer" : undefined,
      };
    }
    case "AGEZT_OBSERVATION_DELTAS": {
      const b = parseBool(raw);
      return {
        key,
        value,
        label: "observation deltas",
        effect:
          b == null
            ? "invalid observation-deltas override"
            : b
              ? "emits delta-style observations"
              : "keeps full observation payloads",
        valid: b != null,
        issue:
          b == null ? "must be a boolean like true/false or 1/0" : undefined,
      };
    }
    case "AGEZT_DISABLE_HEURISTIC_BYPASS": {
      const b = parseBool(raw);
      return {
        key,
        value,
        label: "heuristic bypass",
        effect:
          b == null
            ? "invalid heuristic-bypass override"
            : b
              ? "disables heuristic fast-path bypass"
              : "keeps heuristic fast-path bypass enabled",
        valid: b != null,
        issue:
          b == null ? "must be a boolean like true/false or 1/0" : undefined,
      };
    }
    default:
      return null;
  }
}

// summarizeConfigOverrides separates the agent's generic config overlay from the
// runtime knobs that materially change autonomy behaviour in this process.
export function summarizeConfigOverrides(
  overrides?: Record<string, string>,
): AgentConfigOverrideSummary {
  const runtime: AgentRuntimeOverride[] = [];
  const generic: Array<{ key: string; value: string }> = [];
  for (const [key, value] of Object.entries(overrides || {}).sort(([a], [b]) =>
    a.localeCompare(b),
  )) {
    const row = runtimeOverride(key, value);
    if (row) runtime.push(row);
    else generic.push({ key, value });
  }
  return { runtime, generic };
}
