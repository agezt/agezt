// agentdetail.ts (M953) — pure helpers for the per-agent Command Center deep
// panel (components/AgentDetail). Kept separate from lib/fleet.ts (the M952
// census adapter, reused as-is) so the deep panel adds no surface to it. All
// functions are pure + unit-tested; the React component owns fetching/rendering.

// MemoryRecord mirrors the wire shape of /api/memory records (recordView in
// kernel/controlplane/memory.go). Only the fields the panel reads are typed.
export interface MemoryRecord {
  id?: string;
  type?: string;
  subject?: string;
  content?: string;
  confidence?: number;
  created_ms?: number;
  last_seen_ms?: number;
  tags?: Record<string, string>;
  added_by?: string;
}

// SkillLite mirrors the /api/skills list fields the panel reads.
export interface SkillLite {
  id?: string;
  name?: string;
  description?: string;
  status?: string;
  agent?: string;
  triggers?: string[];
}

// CorrelatedRow is any diagnostics row that can be attributed to a run/agent
// (policy decisions, tool invocations). Both carry a correlation id and an actor.
export interface CorrelatedRow {
  correlation_id?: string;
  actor?: string;
}

// RunLite is the subset of /api/runs the panel folds for spend/last-active.
export interface RunLite {
  correlation_id?: string;
  agent?: string;
  status?: string;
  spent_mc?: number;
  started_unix_ms?: number;
}

export interface ReaperDeadAgent {
  slug: string;
  name?: string;
  last_active_ms?: number;
}

export interface ReaperDegradedAgent {
  slug: string;
  name?: string;
  failures?: number;
  window?: number;
  threshold?: number;
  doctor_agent?: string;
  self_repair_enabled?: boolean;
  escalate_to?: string;
  last_failure_ms?: number;
  last_reason?: string;
}

export interface ReaperMisconfiguredAgent {
  slug: string;
  name?: string;
  issues?: string[];
  doctor_agent?: string;
  self_repair_enabled?: boolean;
  escalate_to?: string;
}

export interface ReaperRoutingPressureAgent {
  slug: string;
  name?: string;
  count?: number;
  threshold?: number;
  window_sec?: number;
  doctor_agent?: string;
  self_repair_enabled?: boolean;
  escalate_to?: string;
  last_fallback_ms?: number;
  last_reason?: string;
  last_failed_model?: string;
  last_next_model?: string;
  task_type?: string;
}

export interface ReaperRoutingForcedProbationAgent {
  slug: string;
  name?: string;
  count?: number;
  threshold?: number;
  window_sec?: number;
  doctor_agent?: string;
  self_repair_enabled?: boolean;
  escalate_to?: string;
  last_fallback_ms?: number;
  last_forced_ms?: number;
  last_reason?: string;
  task_type?: string;
  forced_chain?: string[];
  routing_force_generation?: number;
}

export interface ReaperRoutingForcedFailedAgent {
  slug: string;
  name?: string;
  count?: number;
  threshold?: number;
  window_sec?: number;
  doctor_agent?: string;
  self_repair_enabled?: boolean;
  escalate_to?: string;
  last_fallback_ms?: number;
  last_forced_ms?: number;
  last_reason?: string;
  task_type?: string;
  forced_chain?: string[];
  routing_force_generation?: number;
}

export interface ReaperRoutingForcedExhaustedAgent {
  slug: string;
  count?: number;
  threshold?: number;
  window_sec?: number;
  doctor_agent?: string;
  self_repair_enabled?: boolean;
  escalate_to?: string;
  last_fallback_ms?: number;
  last_forced_ms?: number;
  last_reason?: string;
  task_type?: string;
  forced_chain?: string[];
  routing_force_generation?: number;
}

export interface ReaperRoutingUnstableAgent {
  slug: string;
  name?: string;
  count?: number;
  threshold?: number;
  window_sec?: number;
  doctor_agent?: string;
  self_repair_enabled?: boolean;
  escalate_to?: string;
  last_rollback_ms?: number;
  task_type?: string;
  current_chain?: string[];
  previous_chain?: string[];
  last_reason?: string;
}

export interface ReaperReport {
  dead_agents?: ReaperDeadAgent[];
  degraded_agents?: ReaperDegradedAgent[];
  misconfigured_agents?: ReaperMisconfiguredAgent[];
  routing_pressure_agents?: ReaperRoutingPressureAgent[];
  routing_forced_probation_agents?: ReaperRoutingForcedProbationAgent[];
  routing_forced_failed_agents?: ReaperRoutingForcedFailedAgent[];
  routing_forced_exhausted_agents?: ReaperRoutingForcedExhaustedAgent[];
  routing_unstable_agents?: ReaperRoutingUnstableAgent[];
}

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

export interface AgentRuntimeOverride {
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

export interface AgentRuntimeStatus {
  health_state?: "healthy" | "degraded" | "misconfigured" | "stale" | "retired" | "unstable" | "stabilizing" | "force_failed" | "force_exhausted";
  health_label?: string;
  repair_mode?: "misconfigured" | "degraded" | string;
  repair_state?: "idle" | "queued" | "completed" | "failed" | string;
  repair_label?: string;
  invalid_runtime_overrides?: number;
  misconfiguration_count?: number;
  repair_inflight?: number;
  repair_next_eligible_ms?: number;
  repair_last_ts_ms?: number;
  repair_last_error?: string;
  repair_self_attempt?: number;
  repair_self_max_attempts?: number;
  config_issues?: string[];
  repair_incident_id?: string;
  repair_root_incident_id?: string;
  repair_parent_incident_id?: string;
  repair_root_agent?: string;
  repair_chain_depth?: number;
  self_repair_enabled?: boolean;
  routing_fallback_count?: number;
  routing_last_reason?: string;
  routing_last_failed?: string;
  routing_last_next?: string;
  routing_last_ts_ms?: number;
  routing_force_generation?: number;
  retry_count?: number;
  retry_last_reason?: string;
  retry_last_ts_ms?: number;
  retry_next_attempt?: number;
  retry_max_attempts?: number;
  escalation_open_count?: number;
  escalation_acked_count?: number;
  wake_schedule_count?: number;
  wake_standing_count?: number;
  wake_event_subjects?: string[];
  next_wake_ms?: number;
  next_wake_label?: string;
  active_run_count?: number;
  active_correlation_id?: string;
  active_intent?: string;
  active_started_ms?: number;
  active_model?: string;
  active_spent_mc?: number;
  active_phase?: string;
  active_last_event_ms?: number;
  active_last_event_kind?: string;
  active_detail?: string;
  active_tool?: string;
  active_iter?: number;
  active_wake_source?: string;
  active_wake_reason?: string;
  active_schedule_id?: string;
  active_standing_id?: string;
  active_standing_name?: string;
  active_trigger_subject?: string;
  active_parent_correlation?: string;
  operational_state?: "running" | "sleeping" | "paused" | "retired" | string;
  operational_label?: string;
  last_activity_ms?: number;
  last_activity_kind?: string;
  last_activity_correlation_id?: string;
  last_activity_summary?: string;
  last_autonomy_runbook?: AgentLastAutonomyRunbook;
  // Causality: board message id -> the wake it triggered (the standing/mailbox
  // fire's run correlation + timestamp). Lets the comms tab mark which message
  // actually woke the agent and link it to the run. Derived from the journal.
  mailbox_wakes?: Record<string, MailboxWakeRef>;
  // Enforcement audit: how many tool calls the runtime REFUSED for this agent
  // (policy.decision allow=false), plus the most recent one. Proves the policy is
  // enforced at runtime, not merely displayed.
  policy_denied_count?: number;
  policy_denied_last_tool?: string;
  policy_denied_last_reason?: string;
  policy_denied_last_capability?: string;
  policy_denied_last_hard?: boolean;
  policy_denied_last_ms?: number;
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

export interface ProviderRoutingRow {
  ts_unix_ms?: number;
  kind?: string;
  primary?: string;
  chain?: string;
  task_type?: string;
  failed?: string;
  next?: string;
  reason?: string;
  scope?: string;
}

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

export interface AgentRepairEvent {
  seq?: number;
  ts_unix_ms?: number;
  correlation_id?: string;
  mode?: "misconfigured" | "degraded" | string;
  phase?: string;
  reason?: string;
  fingerprint?: string;
  self_repair_attempt?: number;
  self_repair_max_attempts?: number;
  issues?: string[];
  applied?: string[];
  answer?: string;
  error?: string;
  target_agent?: string;
  target_correlation?: string;
  resolution?:
    | "handled"
    | "paused"
    | "retired"
    | "delegated"
    | "force_chain"
    | "blocked"
    | string;
  resolution_summary?: string;
  delegate_to?: string;
  delegated_by?: string;
  root_agent?: string;
  chain_depth?: number;
  incident_id?: string;
  root_incident_id?: string;
  parent_incident_id?: string;
  next_eligible_ms?: number;
  routing_task_type?: string;
  routing_task_model_chain?: string[];
  previous_routing_task_model_chain?: string[];
  routing_force_generation?: number;
  previous_routing_force_generation?: number;
}

export interface AgentRepairContract {
  retry_attempts?: number;
  retry_backoff?: string;
  retry_on?: string[];
  doctor_agent?: string;
  failure_threshold?: number;
  self_repair_enabled?: boolean;
  self_repair_attempts?: number;
  escalate_to?: string;
  cooldown_sec?: number;
  authority_boundary?: string;
}

export interface AgentRepairNextAction {
  action?: string;
  label?: string;
  detail?: string;
  tone?: string;
  correlation_id?: string;
  fingerprint?: string;
  phase?: string;
  next_eligible_ms?: number;
  delegate_to?: string;
}

export interface AgentRepairStatus {
  slug?: string;
  cooldown_sec?: number;
  next_eligible_ms?: number;
  inflight_count?: number;
  history?: AgentRepairEvent[];
  inflight?: AgentRepairEvent[];
  latest?: AgentRepairEvent;
  contract?: AgentRepairContract;
  next_action?: AgentRepairNextAction;
}

export interface AgentRepairSnapshot {
  state: "idle" | "queued" | "completed" | "failed";
  label: string;
  detail: string;
  mode?: "misconfigured" | "degraded" | string;
  nextEligibleMs?: number;
}

export interface AgentEscalation {
  message_id?: string;
  from?: string;
  to?: string;
  text?: string;
  ts_unix_ms?: number;
  status?: "open" | "acked" | "answered" | string;
  reply_count?: number;
  acked?: boolean;
  source_agent?: string;
  mode?: "misconfigured" | "degraded" | string;
  wake_phase?: string;
  wake_reason?: string;
  wake_error?: string;
  wake_correlation_id?: string;
  fingerprint?: string;
  resolution?:
    | "handled"
    | "paused"
    | "retired"
    | "delegated"
    | "force_chain"
    | "blocked"
    | string;
  resolution_summary?: string;
  delegate_to?: string;
  origin_kind?: "doctor" | "delegated" | string;
  origin_agent?: string;
  root_agent?: string;
  chain_depth?: number;
  incident_id?: string;
  root_incident_id?: string;
  parent_incident_id?: string;
}

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
      configIssues: misconfigured?.issues,
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
