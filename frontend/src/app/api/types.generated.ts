// SPDX-License-Identifier: MIT

// GENERATED FILE — do not edit by hand.
//
// Source of truth: the daemon's operation registry (kernel/controlplane), the
// same registry docs/openapi.json is generated from.
// Regenerate:       go run ./tools/openapi
// Verify:          go run ./tools/openapi -check
//
// Every payload type below is DERIVED from the registry's JSON Schema. The
// browser does not get to name a payload: it names a PATH, and the path says
// which operation answers it and therefore which shape comes back.
//
// The browser's view is LENIENT on responses and STRICT on requests: a
// response field may read as absent, a request field the registry marks
// required must be sent. NULL folds into absent here, because the browser
// reads a null field and a missing field the same way. The daemon's exact
// contract — every field required, every null arm — stays in
// docs/openapi.json, which is the document a caller outside the browser
// reads.

/** AcpAgentsResponseAgentsItem */
export interface AcpAgentsResponseAgentsItem {
  active: boolean;
  archive?: string;
  authors?: string[];
  bin: string;
  command: string;
  compatible?: boolean;
  description: string;
  distributions?: string[];
  docs?: string;
  icon?: string;
  install?: string;
  installed: boolean;
  installed_version?: string;
  license?: string;
  name: string;
  path?: string;
  registered?: boolean;
  registry_version?: string;
  repository?: string;
  runnable?: boolean;
  runner?: string;
  slug: string;
  version?: string;
  website?: string;
}

/** AcpAgentsResponseClientsItem */
export interface AcpAgentsResponseClientsItem {
  category: string;
  description?: string;
  name: string;
  url?: string;
}

/** AcpAgentsResponse */
export interface AcpAgentsResponse {
  active_command?: string;
  agents: AcpAgentsResponseAgentsItem[] | null;
  arch?: string;
  client_count?: number;
  clients?: AcpAgentsResponseClientsItem[];
  clients_cached?: boolean;
  clients_error?: string;
  clients_fetched_at?: string;
  clients_revision?: string;
  clients_source?: string;
  compatible_count?: number;
  installed_count: number;
  missing_count: number;
  os: string;
  platform?: string;
  registered_count?: number;
  registry_cached?: boolean;
  registry_error?: string;
  registry_fetched_at?: string;
  registry_url?: string;
  registry_version?: string;
  runnable_count?: number;
}

/** AgentListResponseProfilesItemHealthPolicy */
export interface AgentListResponseProfilesItemHealthPolicy {
  doctor_agent?: string;
  failure_threshold?: number;
  failure_window?: number;
  stale_after_sec?: number;
}

/** AgentListResponseProfilesItemLifecycle */
export interface AgentListResponseProfilesItemLifecycle {
  completed_cycles?: number;
  last_completed_run?: string;
  max_cycles?: number;
  mode?: string;
  retire_on_complete?: boolean;
}

/** AgentListResponseProfilesItemNoisePolicy */
export interface AgentListResponseProfilesItemNoisePolicy {
  disable_memory_writes?: boolean;
  min_notify_interval_sec?: number;
  min_notify_severity?: string;
  silent_on_success?: boolean;
}

/** AgentListResponseProfilesItemRetryPolicy */
export interface AgentListResponseProfilesItemRetryPolicy {
  backoff?: string;
  base_delay_sec?: number;
  max_attempts?: number;
  max_delay_sec?: number;
  retry_on?: string[];
}

/** AgentListResponseProfilesItemSelfRepair */
export interface AgentListResponseProfilesItemSelfRepair {
  enabled?: boolean;
  escalate_to?: string;
  max_attempts?: number;
}

/** AgentListResponseProfilesItemStatus */
export interface AgentListResponseProfilesItemStatus {
  active_correlation_id?: string;
  active_detail?: string;
  active_intent?: string;
  active_iter?: number;
  active_last_event_kind?: string;
  active_last_event_ms?: number;
  active_model?: string;
  active_parent_correlation?: string;
  active_phase?: string;
  active_run_count: number;
  active_schedule_id?: string;
  active_spent_mc?: number;
  active_standing_id?: string;
  active_standing_name?: string;
  active_started_ms?: number;
  active_tool?: string;
  active_trigger_subject?: string;
  active_wake_reason?: string;
  active_wake_source?: string;
  config_issues?: string[];
  escalation_acked_count: number;
  escalation_open_count: number;
  health_failures?: number;
  health_label: string;
  health_state: string;
  health_threshold?: number;
  health_window?: number;
  invalid_runtime_overrides: number;
  last_active_ms?: number;
  last_activity_correlation_id?: string;
  last_activity_kind?: string;
  last_activity_ms?: number;
  last_activity_summary?: string;
  last_autonomy_runbook?: Record<string, any>;
  last_failure_ms?: number;
  mailbox_wakes?: Record<string, any>;
  misconfiguration_count: number;
  next_wake_label?: string;
  next_wake_ms?: number;
  operational_label: string;
  operational_state: string;
  policy_denied_count?: number;
  policy_denied_last_capability?: string;
  policy_denied_last_hard?: boolean;
  policy_denied_last_ms?: number;
  policy_denied_last_reason?: string;
  policy_denied_last_tool?: string;
  repair_chain_depth?: number;
  repair_incident_id?: string;
  repair_inflight: number;
  repair_label: string;
  repair_last_correlation_id: string;
  repair_last_error?: string;
  repair_last_ts_ms: number;
  repair_mode?: string;
  repair_next_eligible_ms: number;
  repair_parent_incident_id?: string;
  repair_root_agent?: string;
  repair_root_incident_id?: string;
  repair_self_attempt?: number;
  repair_self_max_attempts?: number;
  repair_state: string;
  retry_count: number;
  retry_last_reason?: string;
  retry_last_ts_ms?: number;
  retry_max_attempts?: number;
  retry_next_attempt?: number;
  routing_current_chain?: string[];
  routing_fallback_count: number;
  routing_force_generation?: number;
  routing_forced_chain?: string[];
  routing_last_failed?: string;
  routing_last_next?: string;
  routing_last_reason?: string;
  routing_last_ts_ms?: number;
  routing_previous_chain?: string[];
  routing_task_type?: string;
  self_repair_enabled: boolean;
  wake_event_subjects?: string[];
  wake_schedule_count?: number;
  wake_standing_count?: number;
}

/** AgentListResponseProfilesItemTasklistItem */
export interface AgentListResponseProfilesItemTasklistItem {
  created_ms?: number;
  description?: string;
  id?: string;
  scope?: string;
  status?: string;
  title: string;
  updated_ms?: number;
}

/** AgentListResponseProfilesItem */
export interface AgentListResponseProfilesItem {
  config_overrides?: Record<string, string>;
  created_ms: number;
  description?: string;
  direct_callable?: boolean;
  enabled: boolean;
  execution_profile?: string;
  fallbacks?: string[];
  health_policy?: AgentListResponseProfilesItemHealthPolicy;
  id: string;
  instructions?: string[];
  kind: string;
  lifecycle?: AgentListResponseProfilesItemLifecycle;
  managed: boolean;
  max_cost_mc?: number;
  max_daily_mc?: number;
  memory_scope?: string;
  model?: string;
  name?: string;
  noise_policy?: AgentListResponseProfilesItemNoisePolicy;
  owner_agent?: string;
  parent_agent?: string;
  retired?: boolean;
  retired_ms?: number;
  retired_reason?: string;
  retry_policy?: AgentListResponseProfilesItemRetryPolicy;
  self_repair?: AgentListResponseProfilesItemSelfRepair;
  slug: string;
  soul?: string;
  status?: AgentListResponseProfilesItemStatus;
  system?: boolean;
  task_type?: string;
  tasklist?: AgentListResponseProfilesItemTasklistItem[];
  tool_allow?: string[];
  tool_deny?: string[];
  trust_ceiling?: string;
  updated_ms: number;
  workdir?: string;
}

/** AgentListResponse */
export interface AgentListResponse {
  count: number;
  enabled_count: number;
  next_cursor?: string;
  profiles: AgentListResponseProfilesItem[] | null;
  total: number;
}

/** AgentActivityResponseActivityItem */
export interface AgentActivityResponseActivityItem {
  correlation_id: string;
  kind: string;
  seq: number;
  summary: string;
  ts_unix_ms: number;
}

/** AgentActivityResponse */
export interface AgentActivityResponse {
  activity: AgentActivityResponseActivityItem[] | null;
  count: number;
  next_cursor?: string;
  slug: string;
  total: number;
}

/** AgentAddResponse */
export interface AgentAddResponse {
  profile: AgentListResponseProfilesItem;
}

/** AgentAddRequest */
export interface AgentAddRequest {
  profile?: Record<string, any>;
}

/** AgentCapabilitiesResponseConfigEntriesItem */
export interface AgentCapabilitiesResponseConfigEntriesItem {
  allowed_agents: string[] | null;
  description?: string;
  excluded_agents: string[] | null;
  key: string;
  owned?: boolean;
  rating: string;
  reason: string;
  source: string;
  visible: boolean;
}

/** AgentCapabilitiesResponseGovernance */
export interface AgentCapabilitiesResponseGovernance {
  allowed_count: number;
  ask_count: number;
  ask_tools: string[] | null;
  authority_boundary: string;
  blocked_count: number;
  blocked_tools: string[] | null;
  config_count: number;
  config_hidden_count: number;
  config_owned_count: number;
  config_visible_count: number;
  direct_tools: string[] | null;
  execution_boundary: string;
  hidden_configs: string[] | null;
  max_cost_mc: number;
  max_daily_mc: number;
  memory_policy: string;
  memory_scope: string;
  memory_writes: string;
  noise_disable_memory_writes: boolean;
  noise_min_notify_interval_sec: number;
  noise_min_notify_severity: string;
  noise_silent_on_success: boolean;
  permission_passport: string;
  risk: string;
  summary: string;
  system_enforced: boolean;
  tool_allow_count: number;
  tool_count: number;
  tool_deny_count: number;
  tool_policy: string;
  trust_ceiling: string;
  visible_configs: string[] | null;
}

/** AgentCapabilitiesResponsePermissionsItem */
export interface AgentCapabilitiesResponsePermissionsItem {
  allowed: boolean;
  ask: boolean;
  capability: string;
  description: string;
  hard_denied?: boolean;
  level: string;
  name: string;
  reason: string;
  requires_approval?: boolean;
  source: string;
  status: string;
}

/** AgentCapabilitiesResponseWakeAccess */
export interface AgentCapabilitiesResponseWakeAccess {
  channel_allowed: boolean;
  delegation_allowed: boolean;
  delegation_scope: string;
  delegation_sources: string[] | null;
  direct_allowed: boolean;
  direct_callable: boolean;
  enabled: boolean;
  kind: string;
  manager: string;
  operator_allowed: boolean;
  owner_agent: string;
  parent_agent: string;
  reason: string;
  retired: boolean;
  schedule_allowed: boolean;
  status: string;
  system: boolean;
}

/** AgentCapabilitiesResponse */
export interface AgentCapabilitiesResponse {
  allowed_count: number;
  config_entries: AgentCapabilitiesResponseConfigEntriesItem[] | null;
  count: number;
  governance: AgentCapabilitiesResponseGovernance;
  permissions: AgentCapabilitiesResponsePermissionsItem[] | null;
  profile: AgentListResponseProfilesItem;
  slug: string;
  tool_allow: string[] | null;
  tool_deny: string[] | null;
  trust_ceiling: string;
  wake_access: AgentCapabilitiesResponseWakeAccess;
}

/** AgentCapabilitiesRequest */
export interface AgentCapabilitiesRequest {
  config_overrides?: Record<string, any>;
  max_cost_mc?: number;
  max_daily_mc?: number;
  memory_scope?: string;
  noise_policy?: Record<string, any>;
  ref?: string;
  tool_allow?: string[];
  tool_deny?: string[];
  trust_ceiling?: string;
  workdir?: string;
}

/** AgentEditRequest */
export interface AgentEditRequest {
  profile?: Record<string, any>;
  ref?: string;
}

/** AgentSetEnabledResponse */
export interface AgentSetEnabledResponse {
  profile: AgentListResponseProfilesItem;
  schedules_paused?: number;
  standing_paused?: number;
}

/** AgentEscalationsResponseEscalationsItem */
export interface AgentEscalationsResponseEscalationsItem {
  acked: boolean;
  chain_depth: number;
  delegate_to: string;
  fingerprint: string;
  from: string;
  incident_id: string;
  message_id: string;
  mode: string;
  origin_agent: string;
  origin_kind: string;
  parent_incident_id: string;
  reply_count: number;
  resolution: string;
  resolution_summary: string;
  root_agent: string;
  root_incident_id: string;
  source_agent: string;
  status: string;
  text: string;
  to: string;
  ts_unix_ms: number;
  wake_correlation_id: string;
  wake_error: string;
  wake_phase: string;
  wake_reason: string;
}

/** AgentEscalationsResponse */
export interface AgentEscalationsResponse {
  count: number;
  escalations: AgentEscalationsResponseEscalationsItem[] | null;
  next_cursor?: string;
  open_count: number;
  slug: string;
}

/** AgentImpactResponse */
export interface AgentImpactResponse {
  authored_shared_memories: string[] | null;
  authored_shared_memory_count: number;
  config_count: number;
  configs: string[] | null;
  mailbox_message_count: number;
  mailbox_messages: string[] | null;
  memories: string[] | null;
  memory_count: number;
  schedule_count: number;
  schedules: string[] | null;
  skill_count: number;
  skills: string[] | null;
  slug: string;
  standing_count: number;
  standing_orders: string[] | null;
  subagent_authored_shared_memories: string[] | null;
  subagent_authored_shared_memory_count: number;
  subagent_config_count: number;
  subagent_configs: string[] | null;
  subagent_count: number;
  subagent_mailbox_message_count: number;
  subagent_mailbox_messages: string[] | null;
  subagent_memories: string[] | null;
  subagent_memory_count: number;
  subagent_schedule_count: number;
  subagent_schedules: string[] | null;
  subagent_skill_count: number;
  subagent_skills: string[] | null;
  subagent_standing_count: number;
  subagent_standing_orders: string[] | null;
  subagent_workflow_ref_count: number;
  subagent_workflow_refs: string[] | null;
  subagent_workspace_count: number;
  subagent_workspaces: string[] | null;
  subagents: string[] | null;
  workflow_ref_count: number;
  workflow_refs: string[] | null;
  workspace_count: number;
  workspaces: string[] | null;
}

/** AgentPermissionsResponse */
export interface AgentPermissionsResponse {
  allowed_count: number;
  config_entries: AgentCapabilitiesResponseConfigEntriesItem[] | null;
  count: number;
  governance: AgentCapabilitiesResponseGovernance;
  permissions: AgentCapabilitiesResponsePermissionsItem[] | null;
  slug: string;
  tool_allow: string[] | null;
  tool_deny: string[] | null;
  trust_ceiling: string;
  wake_access: AgentCapabilitiesResponseWakeAccess;
}

/** AgentRemoveResponse */
export interface AgentRemoveResponse {
  authored_memories_forgotten?: number;
  configs_access_pruned?: number;
  configs_deleted?: number;
  mailbox_messages_retained?: number;
  mailbox_messages_retained_refs?: string[];
  memories_forgotten?: number;
  removed: boolean;
  schedules_removed?: number;
  skills_archived?: number;
  standing_removed?: number;
  subagent_workflow_refs_retained?: number;
  subagent_workflow_refs_retained_labels?: string[];
  subagents_retired?: number;
  subagents_retired_slugs?: string[];
  workflow_refs_retained?: number;
  workflow_refs_retained_labels?: string[];
  workspaces_deleted?: number;
}

/** AgentRemoveRequest */
export interface AgentRemoveRequest {
  cascade?: Record<string, any>;
  ref?: string;
}

/** AgentRepairResponse */
export interface AgentRepairResponse {
  accepted: boolean;
  agent: string;
  correlation_id: string;
}

/** AgentRepairRequest */
export interface AgentRepairRequest {
  incident_id?: string;
  parent_incident_id?: string;
  reason?: string;
  ref?: string;
  root_incident_id?: string;
}

/** AgentRepairStatusResponseContract */
export interface AgentRepairStatusResponseContract {
  authority_boundary: string;
  cooldown_sec: number;
  doctor_agent: string;
  escalate_to: string;
  failure_threshold: number;
  retry_attempts: number;
  retry_backoff: string;
  retry_on: string[] | null;
  self_repair_attempts: number;
  self_repair_enabled: boolean;
}

/** AgentRepairStatusResponseHistoryItem */
export interface AgentRepairStatusResponseHistoryItem {
  answer: string;
  applied: string[] | null;
  chain_depth: number;
  correlation_id: string;
  delegate_to: string;
  delegated_by: string;
  error: string;
  fingerprint: string;
  incident_id: string;
  issues: string[] | null;
  mailbox_message_id: string;
  mode: string;
  next_eligible_ms: number;
  parent_incident_id: string;
  phase: string;
  previous_routing_force_generation: number;
  previous_routing_task_model_chain: string[] | null;
  reason: string;
  resolution: string;
  resolution_summary: string;
  root_agent: string;
  root_incident_id: string;
  routing_force_generation: number;
  routing_task_model_chain: string[] | null;
  routing_task_type: string;
  self_repair_attempt: number;
  self_repair_max_attempts: number;
  seq: number;
  target_agent: string;
  target_correlation: string;
  ts_unix_ms: number;
}

/** AgentRepairStatusResponseLatest */
export interface AgentRepairStatusResponseLatest {
  answer: string;
  applied: string[] | null;
  chain_depth: number;
  correlation_id: string;
  delegate_to: string;
  delegated_by: string;
  error: string;
  fingerprint: string;
  incident_id: string;
  issues: string[] | null;
  mailbox_message_id: string;
  mode: string;
  next_eligible_ms: number;
  parent_incident_id: string;
  phase: string;
  previous_routing_force_generation: number;
  previous_routing_task_model_chain: string[] | null;
  reason: string;
  resolution: string;
  resolution_summary: string;
  root_agent: string;
  root_incident_id: string;
  routing_force_generation: number;
  routing_task_model_chain: string[] | null;
  routing_task_type: string;
  self_repair_attempt: number;
  self_repair_max_attempts: number;
  seq: number;
  target_agent: string;
  target_correlation: string;
  ts_unix_ms: number;
}

/** AgentRepairStatusResponseNextAction */
export interface AgentRepairStatusResponseNextAction {
  action: string;
  correlation_id?: string;
  delegate_to?: string;
  detail: string;
  fingerprint?: string;
  label: string;
  next_eligible_ms?: number;
  phase?: string;
  tone: string;
}

/** AgentRepairStatusResponse */
export interface AgentRepairStatusResponse {
  contract: AgentRepairStatusResponseContract;
  cooldown_sec: number;
  count: number;
  history: AgentRepairStatusResponseHistoryItem[] | null;
  inflight: AgentRepairStatusResponseHistoryItem[] | null;
  inflight_count: number;
  latest?: AgentRepairStatusResponseLatest;
  next_action: AgentRepairStatusResponseNextAction;
  next_cursor?: string;
  next_eligible_ms?: number;
  slug: string;
  total: number;
}

/** AgentResolveResponse */
export interface AgentResolveResponse {
  agent: string;
  applied: boolean;
  correlation_id: string;
  resolution: string;
}

/** AgentResolveRequest */
export interface AgentResolveRequest {
  delegate_to?: string;
  incident_id?: string;
  parent_incident_id?: string;
  ref?: string;
  resolution?: string;
  root_incident_id?: string;
  summary?: string;
  task_model_chain?: string[];
  task_type?: string;
}

/** AgentRetireResponseImpactSummary */
export interface AgentRetireResponseImpactSummary {
  authored_shared_memories: string[] | null;
  authored_shared_memory_count: number;
  config_count: number;
  configs: string[] | null;
  mailbox_message_count: number;
  mailbox_messages: string[] | null;
  memories: string[] | null;
  memory_count: number;
  schedule_count: number;
  schedules: string[] | null;
  schedules_paused: number;
  skill_count: number;
  skills: string[] | null;
  slug: string;
  standing_count: number;
  standing_orders: string[] | null;
  standing_paused: number;
  subagent_authored_shared_memories: string[] | null;
  subagent_authored_shared_memory_count: number;
  subagent_config_count: number;
  subagent_configs: string[] | null;
  subagent_count: number;
  subagent_mailbox_message_count: number;
  subagent_mailbox_messages: string[] | null;
  subagent_memories: string[] | null;
  subagent_memory_count: number;
  subagent_schedule_count: number;
  subagent_schedules: string[] | null;
  subagent_skill_count: number;
  subagent_skills: string[] | null;
  subagent_standing_count: number;
  subagent_standing_orders: string[] | null;
  subagent_workflow_ref_count: number;
  subagent_workflow_refs: string[] | null;
  subagent_workspace_count: number;
  subagent_workspaces: string[] | null;
  subagents: string[] | null;
  workflow_ref_count: number;
  workflow_refs: string[] | null;
  workspace_count: number;
  workspaces: string[] | null;
}

/** AgentRetireResponse */
export interface AgentRetireResponse {
  impact: string[] | null;
  impact_summary: AgentRetireResponseImpactSummary | null;
  profile: AgentListResponseProfilesItem;
  schedules_paused: number;
  standing_paused: number;
}

/** AgentReviveResponse */
export interface AgentReviveResponse {
  profile: AgentListResponseProfilesItem;
  schedules_paused: number;
  standing_paused: number;
}

/** AgentTaskUpdateResponse */
export interface AgentTaskUpdateResponse {
  profile: AgentListResponseProfilesItem;
  task: AgentListResponseProfilesItemTasklistItem;
  updated: boolean;
}

/** AgentTaskUpdateRequest */
export interface AgentTaskUpdateRequest {
  description?: string;
  id?: string;
  op?: string;
  ref?: string;
  scope?: string;
  status?: string;
  task?: Record<string, any>;
  title?: string;
}

/** AgentWakeRequest */
export interface AgentWakeRequest {
  incident_id?: string;
  intent?: string;
  parent_incident_id?: string;
  reason?: string;
  ref?: string;
  root_incident_id?: string;
}

/** ApprovalsResponsePendingItem */
export interface ApprovalsResponsePendingItem {
  actor: string;
  affected_resources: string[] | null;
  ambiguity_score: number;
  canonical_intent: string;
  capability: string;
  confidence: number;
  confirmation_prompt: string;
  correlation_id: string;
  created_unix: number;
  effect_class: string;
  harmful_interpretation: string;
  id: string;
  input: string;
  predicted_effects: string[] | null;
  reason: string;
  regret_axes: Record<string, number> | null;
  rollback_notes: string;
  timeout_unix: number;
  tool_name: string;
}

/** ApprovalsResponse */
export interface ApprovalsResponse {
  count: number;
  pending: ApprovalsResponsePendingItem[] | null;
}

/** ApprovalsLogResponseApprovalsItem */
export interface ApprovalsLogResponseApprovalsItem {
  actor: string;
  approval_id: string;
  capability: string;
  correlation_id: string;
  reason: string;
  resolved_by: string;
  seq: number;
  status: string;
  tool: string;
  ts_unix_ms: number;
}

/** ApprovalsLogResponse */
export interface ApprovalsLogResponse {
  approvals: ApprovalsLogResponseApprovalsItem[] | null;
  count: number;
  next_cursor: string;
}

/** ArtifactCollectResponseCandidatesItem */
export interface ArtifactCollectResponseCandidatesItem {
  created_ms: number;
  id: string;
  kind: string;
  name: string;
  size: number;
  source: string;
}

/** ArtifactCollectResponse */
export interface ArtifactCollectResponse {
  bytes: number;
  candidates?: ArtifactCollectResponseCandidatesItem[];
  count: number;
  cutoff_ms: number;
  dry_run: boolean;
  older_than_days: number;
}

/** ArtifactDeleteResponse */
export interface ArtifactDeleteResponse {
  deleted: boolean;
  id: string;
}

/** ArtifactListResponseEntriesItem */
export interface ArtifactListResponseEntriesItem {
  caption: string;
  corr: string;
  created_ms: number;
  id: string;
  kind: string;
  mime: string;
  name: string;
  ref: string;
  sender: string;
  size: number;
  source: string;
}

/** ArtifactListResponse */
export interface ArtifactListResponse {
  count: number;
  entries: ArtifactListResponseEntriesItem[] | null;
}

/** AttentionResponseItemsItem */
export interface AttentionResponseItemsItem {
  href: string;
  id: string;
  kind: string;
  summary: string;
  ts: number;
}

/** AttentionResponse */
export interface AttentionResponse {
  count: number;
  items: AttentionResponseItemsItem[] | null;
}

/** AutonomyFeedResponseItemsItem */
export interface AutonomyFeedResponseItemsItem {
  agent?: string;
  category: string;
  chain_depth?: number;
  correlation_id: string;
  delegate_to?: string;
  delegated_by?: string;
  detail?: string;
  incident_id?: string;
  kind: string;
  mode?: string;
  parent_incident_id?: string;
  phase?: string;
  resolution?: string;
  root_agent?: string;
  root_incident_id?: string;
  routing_force_generation?: number;
  routing_task_model_chain?: string[];
  routing_task_type?: string;
  seq: number;
  subject: string;
  target_agent?: string;
  title: string;
  ts_unix_ms: number;
}

/** AutonomyFeedResponse */
export interface AutonomyFeedResponse {
  count: number;
  items: AutonomyFeedResponseItemsItem[] | null;
}

/** BoardReadResponseMessagesItem */
export interface BoardReadResponseMessagesItem {
  acked_by?: string[];
  from?: string;
  help?: boolean;
  id?: string;
  reply_to?: string;
  text: string;
  to?: string;
  topic: string;
  ts_unix_ms: number;
}

/** BoardReadResponse */
export interface BoardReadResponse {
  count: number;
  messages: BoardReadResponseMessagesItem[] | null;
  next_cursor?: string;
  topics: Record<string, number> | null;
  total: number;
}

/** BoardAckResponse */
export interface BoardAckResponse {
  acked: boolean;
  by: string;
  id: string;
}

/** BoardAckRequest */
export interface BoardAckRequest {
  by?: string;
  id?: string;
}

/** BoardHelpResponse */
export interface BoardHelpResponse {
  count: number;
  open_help: BoardReadResponseMessagesItem[] | null;
}

/** BoardSendResponse */
export interface BoardSendResponse {
  correlation_id?: string;
  sent: BoardReadResponseMessagesItem;
}

/** BoardSendRequest */
export interface BoardSendRequest {
  correlation_id?: string;
  from?: string;
  help?: boolean;
  reply_to?: string;
  text: string;
  to?: string;
  topic?: string;
}

/** BudgetResponsePerTaskItem */
export interface BudgetResponsePerTaskItem {
  ceiling_mc: number;
  spent_mc: number;
  task_type: string;
}

/** BudgetResponse */
export interface BudgetResponse {
  ceiling_mc: number;
  per_task: BudgetResponsePerTaskItem[] | null;
  spent_mc: number;
  strict_pricing: boolean;
  utc_date: string;
}

/** CancelRunResponse */
export interface CancelRunResponse {
  cancelled: boolean;
  correlation: string;
}

/** CatalogListResponseProvidersItemModelsItem */
export interface CatalogListResponseProvidersItemModelsItem {
  context: number;
  cost_input_mc_per_mtok?: number;
  cost_input_usd_per_mtok?: number;
  cost_output_mc_per_mtok?: number;
  cost_output_usd_per_mtok?: number;
  family: string;
  grammar_constrained_decoding: boolean;
  id: string;
  name: string;
  output: number;
  reasoning: boolean;
  schema_constrained_decoding: boolean;
  strict_tool_args: boolean;
  tool_call: boolean;
}

/** CatalogListResponseProvidersItem */
export interface CatalogListResponseProvidersItem {
  api: string;
  credentialed: boolean;
  doc: string;
  env: string[] | null;
  family: string;
  id: string;
  model_count: number;
  models: CatalogListResponseProvidersItemModelsItem[] | null;
  name: string;
}

/** CatalogListResponse */
export interface CatalogListResponse {
  api_source_url: string;
  api_synced_at: string;
  local_source: string;
  local_synced_at: string;
  provider_count: number;
  providers: CatalogListResponseProvidersItem[] | null;
  sources: string[] | null;
}

/** CatalogSyncResponse */
export interface CatalogSyncResponse {
  bytes: number;
  duration_ms: number;
  model_count: number;
  provider_count: number;
  provider_reload_error?: string;
  providers_reloaded: boolean;
  url: string;
}

/** CatalogSyncRequest */
export interface CatalogSyncRequest {
  timeout_s?: number;
  url?: string;
}

/** ChainsGetResponse */
export interface ChainsGetResponse {
  chains: Record<string, string[] | null> | null;
  default: string;
  usage: Record<string, any> | null;
}

/** ChainsSetResponse */
export interface ChainsSetResponse {
  applied: string;
  chain_count: number;
  default: string;
  saved: boolean;
  unknown_models?: string[];
}

/** ChainsSetRequest */
export interface ChainsSetRequest {
  chains?: Record<string, any>;
  default?: string;
}

/** ChannelAccountRemoveResponse */
export interface ChannelAccountRemoveResponse {
  applied: string;
  kind: string;
  label: string;
  removed: number;
}

/** ChannelAccountRemoveRequest */
export interface ChannelAccountRemoveRequest {
  kind?: string;
  label?: string;
}

/** ChannelAccountSetResponse */
export interface ChannelAccountSetResponse {
  applied: string;
  env: string;
  kind: string;
  label: string;
  saved: boolean;
}

/** ChannelAccountSetRequest */
export interface ChannelAccountSetRequest {
  kind?: string;
  label?: string;
  name?: string;
  value?: string;
}

/** ChannelOauthStartResponse */
export interface ChannelOauthStartResponse {
  authorize_url: string;
  state: string;
}

/** ChannelOauthStartRequest */
export interface ChannelOauthStartRequest {
  client_id?: string;
  client_secret?: string;
  instance_url?: string;
  kind?: string;
  label?: string;
  redirect_uri?: string;
}

/** ChannelOauthStatusResponse */
export interface ChannelOauthStatusResponse {
  error?: string;
  kind?: string;
  label?: string;
  status: string;
}

/** ChannelOauthStatusRequest */
export interface ChannelOauthStatusRequest {
  state?: string;
}

/** ChannelListResponseChannelsItemAccountsItemFieldsItem */
export interface ChannelListResponseChannelsItemAccountsItemFieldsItem {
  env: string;
  env_pinned: boolean;
  help: string;
  label: string;
  required: boolean;
  secret: boolean;
  set: boolean;
  value?: string;
}

/** ChannelListResponseChannelsItemAccountsItemProbe */
export interface ChannelListResponseChannelsItemAccountsItemProbe {
  configured: boolean;
  live: boolean;
  mode: string;
  note: string;
  roundtrip_ready: boolean;
  roundtrip_status: string;
}

/** ChannelListResponseChannelsItemAccountsItem */
export interface ChannelListResponseChannelsItemAccountsItem {
  configured: boolean;
  fields: ChannelListResponseChannelsItemAccountsItemFieldsItem[] | null;
  label: string;
  live: boolean;
  probe: ChannelListResponseChannelsItemAccountsItemProbe;
}

/** ChannelListResponseChannelsItemMedia */
export interface ChannelListResponseChannelsItemMedia {
  image_in?: boolean;
  image_out?: boolean;
  voice_in?: boolean;
  voice_out?: boolean;
}

/** ChannelListResponseChannelsItemProbe */
export interface ChannelListResponseChannelsItemProbe {
  accounts: number;
  configured_accounts: number;
  live_accounts: number;
  mode: string;
  roundtrip_ready: boolean;
  roundtrip_status: string;
}

/** ChannelListResponseChannelsItem */
export interface ChannelListResponseChannelsItem {
  accounts: ChannelListResponseChannelsItemAccountsItem[] | null;
  config_section: string;
  configured: boolean;
  connect_method: string;
  description: string;
  display: string;
  docs_url: string;
  duplex: boolean;
  fields: ChannelListResponseChannelsItemAccountsItemFieldsItem[] | null;
  kind: string;
  live: boolean;
  media: ChannelListResponseChannelsItemMedia;
  probe: ChannelListResponseChannelsItemProbe;
  setup_steps: string[] | null;
  transport: string;
}

/** ChannelListResponseMediaMatrix */
export interface ChannelListResponseMediaMatrix {
  image_in: number;
  image_out: number;
  voice_in: number;
  voice_out: number;
}

/** ChannelListResponseProbeMatrix */
export interface ChannelListResponseProbeMatrix {
  configured: number;
  live: number;
  needs_setup: number;
  restart_needed: number;
  roundtrip_ready: number;
  total: number;
}

/** ChannelListResponse */
export interface ChannelListResponse {
  channels: ChannelListResponseChannelsItem[] | null;
  count: number;
  media_matrix: ChannelListResponseMediaMatrix;
  probe_matrix: ChannelListResponseProbeMatrix;
}

/** ChatSummarizeResponse */
export interface ChatSummarizeResponse {
  summary: string;
  turns: number;
}

/** ChatSummarizeRequest */
export interface ChatSummarizeRequest {
  model?: string;
  turns?: Record<string, any>[];
}

/** ConfigResponsePaths */
export interface ConfigResponsePaths {
  base: string;
  catalog: string;
  journal: string;
  runtime: string;
  state: string;
  vault: string;
}

/** ConfigResponseRouting */
export interface ConfigResponseRouting {
  model_overrides?: Record<string, string>;
  requires?: Record<string, string[] | null>;
  routes?: Record<string, string[] | null>;
}

/** ConfigResponse */
export interface ConfigResponse {
  ask_policy: string;
  env: Record<string, boolean> | null;
  model: string;
  paths: ConfigResponsePaths;
  plugin_count: number;
  routing?: ConfigResponseRouting;
  system_prompt_set: boolean;
  tool_count: number;
}

/** ConfigSchemaResponseReloadBoundariesItem */
export interface ConfigSchemaResponseReloadBoundariesItem {
  apply: string;
  envs: string[] | null;
}

/** ConfigSchemaResponseSectionsItemFieldsItem */
export interface ConfigSchemaResponseSectionsItemFieldsItem {
  apply: string;
  env: string;
  help?: string;
  label: string;
  locked?: boolean;
  options?: string[];
  read_only?: boolean;
  required: boolean;
  secret: boolean;
  type: string;
}

/** ConfigSchemaResponseSectionsItem */
export interface ConfigSchemaResponseSectionsItem {
  fields: ConfigSchemaResponseSectionsItemFieldsItem[] | null;
  help?: string;
  id: string;
  locked?: boolean;
  name: string;
  source?: string;
}

/** ConfigSchemaResponse */
export interface ConfigSchemaResponse {
  reload_boundaries: ConfigSchemaResponseReloadBoundariesItem[] | null;
  sections: ConfigSchemaResponseSectionsItem[] | null;
}

/** ConfigSchemaRegisterResponse */
export interface ConfigSchemaRegisterResponse {
  applied: string;
  id: string;
  registered: boolean;
}

/** ConfigSchemaRegisterRequest */
export interface ConfigSchemaRegisterRequest {
  force?: boolean;
  id?: string;
  name?: string;
  section?: Record<string, any>;
  value?: string;
}

/** ConfigSchemaUnregisterResponse */
export interface ConfigSchemaUnregisterResponse {
  id: string;
  removed: boolean;
}

/** ConfigSetResponse */
export interface ConfigSetResponse {
  applied: string;
  env: string;
  env_pinned?: boolean;
  reload_error?: string;
  saved: boolean;
}

/** ConfigSetRequest */
export interface ConfigSetRequest {
  name?: string;
  value?: string;
}

/** ConfigValuesResponseFieldsItem */
export interface ConfigValuesResponseFieldsItem {
  env: string;
  env_pinned: boolean;
  secret: boolean;
  set: boolean;
  value?: string;
}

/** ConfigValuesResponse */
export interface ConfigValuesResponse {
  fields: ConfigValuesResponseFieldsItem[] | null;
}

/** ConfigcenterAccessResponseEntry */
export interface ConfigcenterAccessResponseEntry {
  access_policy?: string;
  allowed_agents?: string[];
  created_at: number;
  description?: string;
  excluded_agents?: string[];
  key: string;
  masked?: boolean;
  rating: string;
  tags?: string[];
  updated_at: number;
  value: string;
  version: number;
}

/** ConfigcenterAccessResponse */
export interface ConfigcenterAccessResponse {
  entry: ConfigcenterAccessResponseEntry;
}

/** ConfigcenterAccessRequest */
export interface ConfigcenterAccessRequest {
  agent_id?: string;
  allowed_agents?: string | string[];
  description?: string;
  excluded_agents?: string | string[];
  key?: string;
  rating?: string;
  since?: string;
  value?: string;
}

/** ConfigcenterDeleteResponse */
export interface ConfigcenterDeleteResponse {
  deleted: boolean;
}

/** ConfigcenterListResponse */
export interface ConfigcenterListResponse {
  count: number;
  entries: ConfigcenterAccessResponseEntry[] | null;
}

/** ConfigcenterSetRatingResponse */
export interface ConfigcenterSetRatingResponse {
  override: boolean;
}

/** CouncilAskResponseMembersItem */
export interface CouncilAskResponseMembersItem {
  model: string;
  seat: string;
}

/** CouncilAskResponseOpinionsItem */
export interface CouncilAskResponseOpinionsItem {
  error?: string;
  model: string;
  round: number;
  seat: string;
  text: string;
}

/** CouncilAskResponse */
export interface CouncilAskResponse {
  as_of: string;
  brief: string;
  consensus: string;
  correlation_id: string;
  dissent: string;
  members: CouncilAskResponseMembersItem[] | null;
  opinions: CouncilAskResponseOpinionsItem[] | null;
  question: string;
  rounds: number;
}

/** CouncilAskRequest */
export interface CouncilAskRequest {
  corr?: string;
  question?: string;
  rounds?: number | string;
}

/** CouncilMembersResponse */
export interface CouncilMembersResponse {
  count: number;
  members: CouncilAskResponseMembersItem[] | null;
}

/** CouncilSetResponse */
export interface CouncilSetResponse {
  applied: string;
  member_count: number;
  saved: boolean;
  unknown_models?: string[];
}

/** CouncilSetRequestMembersItem */
export interface CouncilSetRequestMembersItem {
  model?: string;
  seat?: string;
}

/** CouncilSetRequest */
export interface CouncilSetRequest {
  members?: CouncilSetRequestMembersItem[];
}

/** DataCollectionsResponseCollectionsItemFieldsItem */
export interface DataCollectionsResponseCollectionsItemFieldsItem {
  label: string;
  name: string;
  type: string;
}

/** DataCollectionsResponseCollectionsItem */
export interface DataCollectionsResponseCollectionsItem {
  builtin: boolean;
  count: number;
  created_by: string;
  created_ms: number;
  desc: string;
  fields: DataCollectionsResponseCollectionsItemFieldsItem[] | null;
  icon: string;
  name: string;
  system: boolean;
  title: string;
  view: string;
}

/** DataCollectionsResponse */
export interface DataCollectionsResponse {
  collections: DataCollectionsResponseCollectionsItem[] | null;
  count: number;
}

/** DataInsertResponseRecord */
export interface DataInsertResponseRecord {
  created_by: string;
  created_ms: number;
  fields: Record<string, any> | null;
  id: string;
  updated_by: string;
  updated_ms: number;
}

/** DataInsertResponse */
export interface DataInsertResponse {
  record: DataInsertResponseRecord;
}

/** DataInsertRequest */
export interface DataInsertRequest {
  collection?: string;
  record?: Record<string, any>;
}

/** DataRecordsResponse */
export interface DataRecordsResponse {
  collection: string;
  count: number;
  records: DataInsertResponseRecord[] | null;
  schema: DataCollectionsResponseCollectionsItem;
}

/** DataUpdateRequest */
export interface DataUpdateRequest {
  collection?: string;
  id?: string;
  record?: Record<string, any>;
}

/** DecideResponse */
export interface DecideResponse {
  decision: string;
  id: string;
  ok: boolean;
}

/** EdictDenyAddResponse */
export interface EdictDenyAddResponse {
  applies_to: string[] | null;
  count: number;
  name: string;
  substring: string;
}

/** EdictDenyRmResponse */
export interface EdictDenyRmResponse {
  count: number;
  removed: boolean;
}

/** EdictSetLevelResponse */
export interface EdictSetLevelResponse {
  capability: string;
  from: string;
  to: string;
}

/** EdictSetModeResponse */
export interface EdictSetModeResponse {
  from: string;
  to: string;
}

/** EdictTestResponse */
export interface EdictTestResponse {
  capability: string;
  decision: string;
  hard_denied: boolean;
  hard_deny_rule: string;
  level: string;
  reason: string;
  requires_approval: boolean;
  would_ask: boolean;
}

/** EdictShowResponseHardDenyItem */
export interface EdictShowResponseHardDenyItem {
  applies_to: string[] | null;
  name: string;
  substring: string;
}

/** EdictShowResponse */
export interface EdictShowResponse {
  ask_policy: string;
  hard_deny: EdictShowResponseHardDenyItem[] | null;
  levels: Record<string, string> | null;
}

/** ExecutionProfileCheckResponseChecksItem */
export interface ExecutionProfileCheckResponseChecksItem {
  backend: string;
  backend_available: boolean;
  degraded: boolean;
  detail: string;
  id: string;
  next: string;
  profile_id: string;
  routed: boolean;
  status: string;
  title: string;
}

/** ExecutionProfileCheckResponse */
export interface ExecutionProfileCheckResponse {
  checks: ExecutionProfileCheckResponseChecksItem[] | null;
  count: number;
  fail_count: number;
  host_arch: string;
  host_os: string;
  ok_count: number;
  routable_run_profiles: string[] | null;
  warning_count: number;
}

/** ExecutionProfilesResponseProfilesItemSecretPolicy */
export interface ExecutionProfilesResponseProfilesItemSecretPolicy {
  detail: string;
  metadata_forwarded: boolean;
  mode: string;
  scope: string;
  valid: boolean;
  values_forwarded: boolean;
}

/** ExecutionProfilesResponseProfilesItem */
export interface ExecutionProfilesResponseProfilesItem {
  backends: string[] | null;
  browser_access: string;
  cleanup: string;
  degrade_reason: string;
  degraded: boolean;
  effective_isolation: string;
  environment: string;
  filesystem: string;
  id: string;
  limits: string[] | null;
  name: string;
  network: string;
  notes: string[] | null;
  policy_capability: string;
  requested_isolation: string;
  routed: boolean;
  secret_policy?: ExecutionProfilesResponseProfilesItemSecretPolicy;
  secrets: string;
  status: string;
  summary: string;
  tools: string[] | null;
}

/** ExecutionProfilesResponse */
export interface ExecutionProfilesResponse {
  count: number;
  degraded_count: number;
  host_arch: string;
  host_os: string;
  profiles: ExecutionProfilesResponseProfilesItem[] | null;
  routed_count: number;
  supported_count: number;
}

/** HaltResponse */
export interface HaltResponse {
  halted: boolean;
  ok: boolean;
  reason: string;
}

/** InboxResponseThreadsItemMessagesItem */
export interface InboxResponseThreadsItemMessagesItem {
  direction: string;
  event_id: string;
  sender?: string;
  text: string;
  ts_unix_ms: number;
}

/** InboxResponseThreadsItem */
export interface InboxResponseThreadsItem {
  channel_id: string;
  channel_kind: string;
  correlation_id: string;
  last_ts_unix_ms: number;
  messages: InboxResponseThreadsItemMessagesItem[] | null;
}

/** InboxResponse */
export interface InboxResponse {
  channel?: string;
  count: number;
  next_cursor?: string;
  threads: (InboxResponseThreadsItem | null)[] | null;
  total: number;
}

/** JournalGrepResponseEventsItem */
export interface JournalGrepResponseEventsItem {
  actor: string;
  causation_id?: string;
  correlation_id?: string;
  hash?: string;
  id: string;
  kind: string;
  payload?: any;
  prev_hash: string;
  seq: number;
  subject: string;
  tags?: Record<string, string>;
  ts_unix_ms: number;
}

/** JournalGrepResponse */
export interface JournalGrepResponse {
  count: number;
  events: JournalGrepResponseEventsItem[] | null;
  head: number;
}

/** MarketListResponsePacksItem */
export interface MarketListResponsePacksItem {
  builtin: boolean;
  category?: string;
  description?: string;
  downloads?: number;
  featured?: boolean;
  installed: boolean;
  marketplace: string;
  mcp_count: number;
  name: string;
  sha256?: string;
  signed?: boolean;
  skill_count: number;
  source?: string;
  tags?: string[];
  tool_count: number;
  update_available?: boolean;
  version: string;
}

/** MarketListResponse */
export interface MarketListResponse {
  count: number;
  packs: MarketListResponsePacksItem[] | null;
}

/** MarketInstallResponse */
export interface MarketInstallResponse {
  installed_ms: number;
  marketplace: string;
  mcp_servers?: string[];
  name: string;
  skill_ids?: string[];
  tool_reqs?: string[];
  unsigned?: boolean;
  version: string;
  vet_verdict?: string;
}

/** MarketShowResponsePackMcpServersItem */
export interface MarketShowResponsePackMcpServersItem {
  args?: string[];
  command?: string;
  created_ms: number;
  description?: string;
  enabled: boolean;
  env?: Record<string, string>;
  headers?: Record<string, string>;
  id: string;
  lazy?: boolean;
  name: string;
  tool_allow?: string[];
  updated_ms: number;
  url?: string;
}

/** MarketShowResponsePackSignature */
export interface MarketShowResponsePackSignature {
  pubkey: string;
  sha256: string;
  sig: string;
  signed_at: number;
}

/** MarketShowResponsePackSkillsItem */
export interface MarketShowResponsePackSkillsItem {
  resources?: Record<string, string | null>;
  skill_md: string;
}

/** MarketShowResponsePack */
export interface MarketShowResponsePack {
  author?: string;
  category?: string;
  description?: string;
  keywords?: string[];
  mcp_servers?: MarketShowResponsePackMcpServersItem[];
  name: string;
  signature?: MarketShowResponsePackSignature;
  skills?: MarketShowResponsePackSkillsItem[];
  tags?: string[];
  tool_requirements?: string[];
  version: string;
}

/** MarketShowResponseSkillsItem */
export interface MarketShowResponseSkillsItem {
  description?: string;
  name?: string;
  skill_md: string;
}

/** MarketShowResponseVetFindingsItem */
export interface MarketShowResponseVetFindingsItem {
  detail: string;
  rule: string;
  severity: string;
  where: string;
}

/** MarketShowResponseVet */
export interface MarketShowResponseVet {
  findings?: MarketShowResponseVetFindingsItem[];
  verdict: string;
}

/** MarketShowResponse */
export interface MarketShowResponse {
  installed: boolean;
  installed_at: number;
  mcp_count: number;
  mcp_servers: string[] | null;
  pack: MarketShowResponsePack;
  skill_count: number;
  skills: MarketShowResponseSkillsItem[] | null;
  tool_count: number;
  tools: string[] | null;
  vet: MarketShowResponseVet;
}

/** MarketAddSourceResponse */
export interface MarketAddSourceResponse {
  added_ms?: number;
  name: string;
  pubkey?: string;
  url: string;
}

/** MarketAddSourceRequest */
export interface MarketAddSourceRequest {
  marketplace?: string;
  name?: string;
  pubkey?: string;
  url?: string;
  version?: string;
}

/** MarketRemoveSourceResponse */
export interface MarketRemoveSourceResponse {
  name: string;
  removed: boolean;
}

/** MarketSourcesResponse */
export interface MarketSourcesResponse {
  count: number;
  sources: MarketAddSourceResponse[] | null;
}

/** MarketSyncResponseResultsItem */
export interface MarketSyncResponseResultsItem {
  bytes: number;
  fetched_ms: number;
  packs: number;
  source: string;
  url: string;
}

/** MarketSyncResponse */
export interface MarketSyncResponse {
  packs: number;
  partial_error?: string;
  results: MarketSyncResponseResultsItem[] | null;
  synced: number;
}

/** MarketUninstallResponse */
export interface MarketUninstallResponse {
  uninstalled: string;
}

/** McpListResponseServersItem */
export interface McpListResponseServersItem {
  args?: string[];
  attached: boolean;
  command?: string;
  created_ms: number;
  description?: string;
  enabled: boolean;
  env_keys?: string[];
  header_keys?: string[];
  id: string;
  lazy?: boolean;
  name: string;
  tool_allow?: string[];
  tool_count?: number;
  transport: string;
  updated_ms: number;
  url?: string;
}

/** McpListResponse */
export interface McpListResponse {
  attached_count: number;
  count: number;
  servers: McpListResponseServersItem[] | null;
}

/** McpAddResponse */
export interface McpAddResponse {
  server: McpListResponseServersItem;
}

/** McpAddRequest */
export interface McpAddRequest {
  enabled?: boolean | string;
  ref?: string;
  server?: Record<string, any>;
}

/** McpAttachResponse */
export interface McpAttachResponse {
  server: McpListResponseServersItem;
  tools: string[] | null;
}

/** McpDetachResponse */
export interface McpDetachResponse {
  detached: boolean;
}

/** McpRemoveResponse */
export interface McpRemoveResponse {
  removed: boolean;
}

/** MemoryListResponseRecordsItem */
export interface MemoryListResponseRecordsItem {
  added_by?: string;
  confidence: number;
  content: string;
  created_ms: number;
  evidence?: string;
  expired?: boolean;
  expires_ms?: number;
  half_life_ms?: number;
  id: string;
  last_seen_ms: number;
  source_event?: string;
  subject: string;
  superseded_by?: string;
  suspended?: boolean;
  suspended_ms?: number;
  suspended_reason?: string;
  tags?: Record<string, string>;
  tombstoned?: boolean;
  type: string;
  updated_by?: string;
}

/** MemoryListResponse */
export interface MemoryListResponse {
  count: number;
  next_cursor?: string;
  records: MemoryListResponseRecordsItem[] | null;
  total: number;
}

/** MemoryAddResponse */
export interface MemoryAddResponse {
  created: boolean;
  evidence: string;
  id: string;
  subject: string;
  type: string;
}

/** MemoryAddRequest */
export interface MemoryAddRequest {
  confidence?: number;
  content: string;
  evidence?: string;
  half_life_ms?: number;
  subject?: string;
  tags?: Record<string, string>;
  type?: string;
}

/** MemoryAuditResponseContradictionsItem */
export interface MemoryAuditResponseContradictionsItem {
  ids: string[] | null;
  key: string;
  scope?: string;
  subject?: string;
  type?: string;
}

/** MemoryAuditResponse */
export interface MemoryAuditResponse {
  contradiction_load: number;
  contradictions?: MemoryAuditResponseContradictionsItem[];
  expired: number;
  expired_ids?: string[];
  suspended: number;
  suspended_ids?: string[];
  total: number;
  usable: number;
}

/** MemoryBulkForgetResponse */
export interface MemoryBulkForgetResponse {
  forgotten: number;
  not_found: number;
}

/** MemoryBulkForgetRequest */
export interface MemoryBulkForgetRequest {
  ids: string[];
}

/** MemoryCleanResponseDecisionsItem */
export interface MemoryCleanResponseDecisionsItem {
  id: string;
  reason: string;
  subject?: string;
}

/** MemoryCleanResponse */
export interface MemoryCleanResponse {
  decisions?: MemoryCleanResponseDecisionsItem[];
  dry_run: boolean;
  hard_deleted: boolean;
  rejected: number;
  removed: number;
  scanned: number;
}

/** MemoryForgetResponse */
export interface MemoryForgetResponse {
  forgotten: boolean;
}

/** MemoryPromoteResponse */
export interface MemoryPromoteResponse {
  id: string;
  promoted: boolean;
  subject?: string;
}

/** MemoryPruneResponseStats */
export interface MemoryPruneResponseStats {
  active: number;
  expired: number;
  prunable: number;
  superseded: number;
  suspended: number;
  tombstoned: number;
  total: number;
}

/** MemoryPruneResponse */
export interface MemoryPruneResponse {
  cutoff_ms: number;
  dry_run: boolean;
  older_than_days: number;
  prunable?: number;
  pruned?: number;
  stats: MemoryPruneResponseStats;
}

/** MemorySupersedeResponse */
export interface MemorySupersedeResponse {
  new_id: string;
  old_id: string;
  subject: string;
  superseded: boolean;
  type: string;
}

/** MemorySupersedeRequest */
export interface MemorySupersedeRequest {
  confidence?: number;
  content: string;
  evidence?: string;
  half_life_ms?: number;
  old_id: string;
  subject?: string;
  tags?: Record<string, string>;
  type?: string;
}

/** MemoryTidyResponse */
export interface MemoryTidyResponse {
  collapsed: number;
  dry_run: boolean;
}

/** MemoryLogResponseOpsItem */
export interface MemoryLogResponseOpsItem {
  id: string;
  op: string;
  seq: number;
  subject: string;
  ts_unix_ms: number;
  type: string;
}

/** MemoryLogResponse */
export interface MemoryLogResponse {
  count: number;
  next_cursor: string;
  ops: MemoryLogResponseOpsItem[] | null;
}

/** NetguardLogResponseBlocksItem */
export interface NetguardLogResponseBlocksItem {
  ip: string;
  reason: string;
  seq: number;
  tool: string;
  ts_unix_ms: number;
}

/** NetguardLogResponse */
export interface NetguardLogResponse {
  blocks: NetguardLogResponseBlocksItem[] | null;
  count: number;
  next_cursor: string;
}

/** NodeRegistryResponseNodesItem */
export interface NodeRegistryResponseNodesItem {
  auth?: string;
  capabilities?: string[];
  error?: string;
  id: string;
  local: boolean;
  model?: string;
  model_count?: number;
  name: string;
  reachable: boolean;
  status: string;
  url?: string;
  version?: string;
}

/** NodeRegistryResponse */
export interface NodeRegistryResponse {
  count: number;
  error?: string;
  nodes: NodeRegistryResponseNodesItem[] | null;
  peer_count: number;
  remote_run_registered?: boolean;
}

/** PersonaGetResponse */
export interface PersonaGetResponse {
  set: boolean;
  system: string;
}

/** PersonaSetResponse */
export interface PersonaSetResponse {
  applied: string;
  length: number;
  saved: boolean;
  set: boolean;
}

/** PersonaSetRequest */
export interface PersonaSetRequest {
  system?: string;
}

/** PlanGenerateResponse */
export interface PlanGenerateResponse {
  node_count: number;
  plan_json: string;
}

/** PlanGenerateRequest */
export interface PlanGenerateRequest {
  intent?: string;
  model?: string;
}

/** PlanRefineRequest */
export interface PlanRefineRequest {
  feedback?: string;
  model?: string;
  plan_json?: string;
}

/** PlanResponse */
export interface PlanResponse {
  node_outputs: Record<string, any> | null;
  plan_id: string;
}

/** PlanHistoryResponsePlansItem */
export interface PlanHistoryResponsePlansItem {
  correlation_id: string;
  duration_ms: number;
  node_count: number;
  plan_name: string;
  started_unix_ms: number;
  status: string;
}

/** PlanHistoryResponse */
export interface PlanHistoryResponse {
  count: number;
  next_cursor: string;
  plans: PlanHistoryResponsePlansItem[] | null;
}

/** EdictStatsResponse */
export interface EdictStatsResponse {
  allowed: number;
  denial_rate: number;
  denied: number;
  denied_by_capability: Record<string, number> | null;
  hard_denied: number;
  total: number;
  window_ms: number;
}

/** EdictLogResponseDecisionsItem */
export interface EdictLogResponseDecisionsItem {
  actor: string;
  allow: boolean;
  capability: string;
  correlation_id: string;
  hard_denied: boolean;
  reason: string;
  seq: number;
  tool: string;
  ts_unix_ms: number;
}

/** EdictLogResponse */
export interface EdictLogResponse {
  count: number;
  decisions: EdictLogResponseDecisionsItem[] | null;
  next_cursor: string;
}

/** ProfileRebuildResponse */
export interface ProfileRebuildResponse {
  correlation_id: string;
  facets: string[] | null;
  facets_written: number;
  input_records: number;
}

/** PromptsGetResponsePromptsItem */
export interface PromptsGetResponsePromptsItem {
  text: string;
  title: string;
}

/** PromptsGetResponse */
export interface PromptsGetResponse {
  prompts: PromptsGetResponsePromptsItem[] | null;
}

/** PromptsSetResponse */
export interface PromptsSetResponse {
  count: number;
  saved: boolean;
}

/** PromptsSetRequest */
export interface PromptsSetRequest {
  prompts?: Record<string, any>[];
}

/** ProviderConnectResponse */
export interface ProviderConnectResponse {
  added: boolean;
  exists: boolean;
  note?: string;
  provider_id: string;
  providers_reloaded: boolean;
  reload_error?: string;
}

/** ProviderConnectRequest */
export interface ProviderConnectRequest {
  api?: string;
  env?: string;
  id?: string;
  model?: string;
  name?: string;
  npm?: string;
}

/** ProviderKeyListResponseKeysItem */
export interface ProviderKeyListResponseKeysItem {
  active: boolean;
  label: string;
  last4: string;
}

/** ProviderKeyListResponse */
export interface ProviderKeyListResponse {
  env: string;
  keys: ProviderKeyListResponseKeysItem[] | null;
  provider: string;
}

/** ProviderKeyActivateResponse */
export interface ProviderKeyActivateResponse {
  active: boolean;
  env: string;
  label: string;
  provider: string;
  reload_error?: string;
}

/** ProviderKeyAddResponse */
export interface ProviderKeyAddResponse {
  active_changed: boolean;
  added: boolean;
  env: string;
  label: string;
  provider: string;
  reload_error?: string;
}

/** ProviderKeyAddRequest */
export interface ProviderKeyAddRequest {
  active?: boolean;
  env?: string;
  label?: string;
  provider?: string;
  value?: string;
}

/** ProviderKeyRemoveResponse */
export interface ProviderKeyRemoveResponse {
  env: string;
  label: string;
  provider: string;
  reload_error?: string;
  removed: boolean;
  was_active: boolean;
}

/** ProviderOauthImportResponse */
export interface ProviderOauthImportResponse {
  account: string;
  connected: boolean;
  default_model: string;
  email: string;
  models: string[] | null;
  ok: boolean;
}

/** ProviderOauthImportRequest */
export interface ProviderOauthImportRequest {
  path?: string;
}

/** ProviderOauthLogoutResponse */
export interface ProviderOauthLogoutResponse {
  connected: boolean;
  ok: boolean;
}

/** ProviderOauthStartRequest */
export interface ProviderOauthStartRequest {
  provider?: string;
}

/** ProviderOauthStatusResponse */
export interface ProviderOauthStatusResponse {
  account: string;
  connected: boolean;
  default_model: string;
  email: string;
  error: string;
  models: string[] | null;
  status: string;
}

/** ProviderProbeResponse */
export interface ProviderProbeResponse {
  authorized?: boolean;
  error?: string;
  http_status?: number;
  models?: number;
  ok: boolean;
  reachable?: boolean;
}

/** ProviderProbeRequest */
export interface ProviderProbeRequest {
  key?: string;
  url?: string;
}

/** ProviderReloadResponse */
export interface ProviderReloadResponse {
  note?: string;
  provider_count: number;
  providers_reloaded: boolean;
}

/** ProviderLogResponseEventsItem */
export interface ProviderLogResponseEventsItem {
  chain?: string;
  failed?: string;
  kind: string;
  next?: string;
  primary?: string;
  reason?: string;
  scope?: string;
  seq: number;
  task_type?: string;
  ts_unix_ms: number;
}

/** ProviderLogResponse */
export interface ProviderLogResponse {
  count: number;
  events: ProviderLogResponseEventsItem[] | null;
  next_cursor: string;
}

/** PulseBeatResponse */
export interface PulseBeatResponse {
  triggered: boolean;
}

/** PulseCadenceResponse */
export interface PulseCadenceResponse {
  cadence_ms: number;
}

/** PulseDialResponse */
export interface PulseDialResponse {
  dial: string;
}

/** PulseFlushResponse */
export interface PulseFlushResponse {
  flushed: number;
}

/** PulsePauseResponse */
export interface PulsePauseResponse {
  paused: boolean;
}

/** PulseProbeResponse */
export interface PulseProbeResponse {
  added: boolean;
  observer: string;
}

/** PulseQuietResponse */
export interface PulseQuietResponse {
  quiet: string;
}

/** PulseUnwatchResponse */
export interface PulseUnwatchResponse {
  removed: number;
}

/** RatelimitLogResponseThrottlesItem */
export interface RatelimitLogResponseThrottlesItem {
  limit_per_min: number;
  seq: number;
  ts_unix_ms: number;
  used: number;
}

/** RatelimitLogResponse */
export interface RatelimitLogResponse {
  count: number;
  next_cursor: string;
  throttles: RatelimitLogResponseThrottlesItem[] | null;
}

/** ReaperScanResponseDeadAgentsItem */
export interface ReaperScanResponseDeadAgentsItem {
  last_active_ms: number;
  name: string;
  slug: string;
}

/** ReaperScanResponseDegradedAgentsItem */
export interface ReaperScanResponseDegradedAgentsItem {
  doctor_agent: string;
  escalate_to: string;
  failures: number;
  last_failure_ms: number;
  last_reason: string;
  name: string;
  self_repair_enabled: boolean;
  slug: string;
  threshold: number;
  window: number;
}

/** ReaperScanResponseMisconfiguredAgentsItem */
export interface ReaperScanResponseMisconfiguredAgentsItem {
  doctor_agent: string;
  escalate_to: string;
  issues: string[] | null;
  name: string;
  self_repair_enabled: boolean;
  slug: string;
}

/** ReaperScanResponseRetryPressureAgentsItem */
export interface ReaperScanResponseRetryPressureAgentsItem {
  count: number;
  doctor_agent: string;
  escalate_to: string;
  last_reason: string;
  last_retry_ms: number;
  max_attempts: number;
  name: string;
  next_attempt: number;
  self_repair_enabled: boolean;
  slug: string;
  threshold: number;
  window_sec: number;
}

/** ReaperScanResponseRoutingForcedExhaustedAgentsItem */
export interface ReaperScanResponseRoutingForcedExhaustedAgentsItem {
  count: number;
  doctor_agent: string;
  escalate_to: string;
  forced_chain: string[] | null;
  last_fallback_ms: number;
  last_forced_ms: number;
  last_reason: string;
  name: string;
  routing_force_generation: number;
  self_repair_enabled: boolean;
  slug: string;
  task_type: string;
  threshold: number;
  window_sec: number;
}

/** ReaperScanResponseRoutingPressureAgentsItem */
export interface ReaperScanResponseRoutingPressureAgentsItem {
  count: number;
  doctor_agent: string;
  escalate_to: string;
  last_failed_model: string;
  last_fallback_ms: number;
  last_next_model: string;
  last_reason: string;
  name: string;
  self_repair_enabled: boolean;
  slug: string;
  task_type: string;
  threshold: number;
  window_sec: number;
}

/** ReaperScanResponseRoutingUnstableAgentsItem */
export interface ReaperScanResponseRoutingUnstableAgentsItem {
  count: number;
  current_chain: string[] | null;
  doctor_agent: string;
  escalate_to: string;
  last_reason: string;
  last_rollback_ms: number;
  name: string;
  previous_chain: string[] | null;
  self_repair_enabled: boolean;
  slug: string;
  task_type: string;
  threshold: number;
  window_sec: number;
}

/** ReaperScanResponse */
export interface ReaperScanResponse {
  dead_agents: ReaperScanResponseDeadAgentsItem[] | null;
  dead_count: number;
  degraded_agents: ReaperScanResponseDegradedAgentsItem[] | null;
  degraded_count: number;
  idle_days: number;
  misconfigured_agents: ReaperScanResponseMisconfiguredAgentsItem[] | null;
  misconfigured_count: number;
  retry_pressure_agents: ReaperScanResponseRetryPressureAgentsItem[] | null;
  retry_pressure_count: number;
  routing_forced_exhausted_agents: ReaperScanResponseRoutingForcedExhaustedAgentsItem[] | null;
  routing_forced_exhausted_count: number;
  routing_forced_failed_agents: ReaperScanResponseRoutingForcedExhaustedAgentsItem[] | null;
  routing_forced_failed_count: number;
  routing_forced_probation_agents: ReaperScanResponseRoutingForcedExhaustedAgentsItem[] | null;
  routing_forced_probation_count: number;
  routing_pressure_agents: ReaperScanResponseRoutingPressureAgentsItem[] | null;
  routing_pressure_count: number;
  routing_unstable_agents: ReaperScanResponseRoutingUnstableAgentsItem[] | null;
  routing_unstable_count: number;
  stale_artifacts: number;
  stale_bytes: number;
  stale_days: number;
}

/** RedactTestResponse */
export interface RedactTestResponse {
  categories: string[] | null;
  enabled: boolean;
  literal_hit: boolean;
  redacted: string;
  would_redact: boolean;
}

/** RedactTestRequest */
export interface RedactTestRequest {
  text?: string;
}

/** ResearchAskResponseClaimsItem */
export interface ResearchAskResponseClaimsItem {
  note: string;
  source_ids: string[] | null;
  text: string;
  verdict: string;
}

/** ResearchAskResponseSourcesItem */
export interface ResearchAskResponseSourcesItem {
  hash: string;
  id: string;
  rank: number;
  title: string;
  url: string;
}

/** ResearchAskResponse */
export interface ResearchAskResponse {
  cited_sources: number;
  claims: ResearchAskResponseClaimsItem[] | null;
  confidence: number;
  correlation_id: string;
  markdown: string;
  notes: string[] | null;
  question: string;
  sources: ResearchAskResponseSourcesItem[] | null;
  sub_questions: string[] | null;
  verified: boolean;
}

/** ResearchAskRequest */
export interface ResearchAskRequest {
  corr?: string;
  max_sources?: number | string;
  max_sub_questions?: number | string;
  max_verify_claims?: number | string;
  question?: string;
  verify?: boolean;
}

/** RoutingGetResponseActivityValue */
export interface RoutingGetResponseActivityValue {
  fallbacks: number;
  last_failed: string;
  last_ms: number;
  last_next: string;
  last_reason: string;
}

/** RoutingGetResponse */
export interface RoutingGetResponse {
  activity: Record<string, RoutingGetResponseActivityValue> | null;
  chains: Record<string, string[] | null> | null;
  task_types: string[] | null;
}

/** RoutingSetResponse */
export interface RoutingSetResponse {
  applied: string;
  saved: boolean;
  task_count: number;
  unknown_models?: string[];
}

/** RoutingSetRequest */
export interface RoutingSetRequest {
  chains?: Record<string, any>;
}

/** RunResponse */
export interface RunResponse {
  agent?: string;
  answer?: string;
  correlation_id?: string;
  cost_cap?: string;
  dry_run?: boolean;
  execution_profile?: string;
  execution_profile_source?: string;
  intent?: string;
  iters?: number;
  model?: string;
  model_known?: boolean;
  model_source?: string;
  remote_peer?: string;
  spent_mc?: number;
  supports_tools?: boolean;
  supports_vision?: boolean;
  system_source?: string;
  tenant?: string;
  timeout?: string;
  tools?: string[];
  tools_dropped?: string[];
  tools_mode?: string;
  warden_profile?: string;
  warnings?: string[];
}

/** RunPauseResponse */
export interface RunPauseResponse {
  correlation: string;
  ok: boolean;
}

/** RunSteerResponse */
export interface RunSteerResponse {
  accepted: boolean;
  correlation: string;
  mode: string;
}

/** RunsListResponseRunsItem */
export interface RunsListResponseRunsItem {
  agent: string;
  answer_preview: string;
  completed_unix_ms: number;
  correlation_id: string;
  duration_ms: number;
  intent: string;
  iters: number;
  model: string;
  parent_correlation: string;
  phase?: string;
  reason: string;
  spent_mc: number;
  started_unix_ms: number;
  status: string;
  tool?: string;
}

/** RunsListResponse */
export interface RunsListResponse {
  count: number;
  next_cursor: string;
  runs: RunsListResponseRunsItem[] | null;
}

/** SandboxListResponseProjectsItemFilesItem */
export interface SandboxListResponseProjectsItemFilesItem {
  bytes: number;
  name: string;
}

/** SandboxListResponseProjectsItem */
export interface SandboxListResponseProjectsItem {
  file_count: number;
  files: SandboxListResponseProjectsItemFilesItem[] | null;
  modified_unix: number;
  name: string;
  total_bytes: number;
}

/** SandboxListResponse */
export interface SandboxListResponse {
  count: number;
  projects: SandboxListResponseProjectsItem[] | null;
}

/** SandboxDeleteResponse */
export interface SandboxDeleteResponse {
  deleted: string;
}

/** SandboxFileResponse */
export interface SandboxFileResponse {
  bytes: number;
  content: string;
  file: string;
  project: string;
  truncated: boolean;
}

/** ScheduleAddResponse */
export interface ScheduleAddResponse {
  agent: string;
  assure: number;
  at_minutes: number;
  cadence: string;
  created_unix: number;
  days: number;
  enabled: boolean;
  end_minutes: number;
  execution_authority: string;
  execution_contract: string;
  executor: string;
  fires: number;
  frequency_warning?: string;
  id: string;
  identity_owner: string;
  intent: string;
  interval_sec: number;
  last_fired_unix_ms?: number;
  last_reason?: string;
  last_run_unix: number;
  last_status?: string;
  llm_boundary: string;
  mode: string;
  model: string;
  next_run_unix: number;
  payload: any;
  payload_contract: string;
  source: string;
  system_task: string;
  target: string;
  target_error?: string;
  target_status?: string;
  tool: string;
  tz: string;
  uses_llm: boolean;
  workflow: string;
}

/** ScheduleAddRequest */
export interface ScheduleAddRequest {
  agent?: string;
  at_minutes?: number;
  cooldown_sec?: number;
  count?: number;
  cursor?: string;
  days?: number;
  enabled?: boolean | string;
  id?: string;
  intent?: string;
  interval_sec?: number;
  limit?: number;
  model?: string;
  once_at_unix?: number;
  payload?: Record<string, any> | any[] | string | number | boolean;
  since_ms?: number;
  status?: string;
  system_task?: string;
  target?: string;
  tool?: string;
  tz?: string;
  window_end?: number;
  window_start?: number;
  workflow?: string;
}

/** ScheduleEditResponse */
export interface ScheduleEditResponse {
  agent?: string;
  assure?: number;
  at_minutes?: number;
  cadence?: string;
  created_unix?: number;
  days?: number;
  enabled?: boolean;
  end_minutes?: number;
  execution_authority?: string;
  execution_contract?: string;
  executor?: string;
  fires?: number;
  frequency_warning?: string;
  id?: string;
  identity_owner?: string;
  intent?: string;
  interval_sec?: number;
  last_fired_unix_ms?: number;
  last_reason?: string;
  last_run_unix?: number;
  last_status?: string;
  llm_boundary?: string;
  mode?: string;
  model?: string;
  next_run_unix?: number;
  payload?: any;
  payload_contract?: string;
  source?: string;
  system_task?: string;
  target?: string;
  target_error?: string;
  target_status?: string;
  tool?: string;
  tz?: string;
  updated: boolean;
  uses_llm?: boolean;
  workflow?: string;
}

/** ScheduleEnableResponse */
export interface ScheduleEnableResponse {
  action: string;
  enabled: boolean;
  id: string;
  updated: boolean;
}

/** ScheduleFiresResponseFiresItem */
export interface ScheduleFiresResponseFiresItem {
  action: string;
  agent: string;
  answer_preview: string;
  autonomy_runbook?: Record<string, any>;
  category: string;
  correlation_id: string;
  duration_ms: number;
  effect_class: string;
  executor: string;
  fired_unix_ms: number;
  intent: string;
  model: string;
  reason: string;
  schedule_id: string;
  spent_mc: number;
  status: string;
  system_task: string;
  target: string;
  tool: string;
  uses_llm: boolean;
  workflow: string;
}

/** ScheduleFiresResponse */
export interface ScheduleFiresResponse {
  count: number;
  fires: ScheduleFiresResponseFiresItem[] | null;
  next_cursor: string;
}

/** ScheduleSystemTasksResponseSystemTaskInfoItem */
export interface ScheduleSystemTasksResponseSystemTaskInfoItem {
  category?: string;
  description: string;
  effect?: string;
  effect_class?: string;
  executor?: string;
  label: string;
  name: string;
  recommended_interval_sec?: number;
  uses_llm: boolean;
}

/** ScheduleSystemTasksResponse */
export interface ScheduleSystemTasksResponse {
  count: number;
  system_task_info: ScheduleSystemTasksResponseSystemTaskInfoItem[] | null;
  system_tasks: string[] | null;
}

/** ScheduleTestResponseForecastsItem */
export interface ScheduleTestResponseForecastsItem {
  unix: number;
}

/** ScheduleTestResponse */
export interface ScheduleTestResponse {
  cadence?: string;
  count?: number;
  enabled?: boolean;
  forecasts?: ScheduleTestResponseForecastsItem[];
  found: boolean;
  id?: string;
  mode?: string;
}

/** ScheduleListResponse */
export interface ScheduleListResponse {
  count: number;
  schedules: ScheduleAddResponse[] | null;
}

/** SendResponse */
export interface SendResponse {
  channel: string;
  sent: boolean;
  to: string;
}

/** SendRequest */
export interface SendRequest {
  channel?: string;
  text?: string;
  to?: string;
}

/** SkillArchiveResponse */
export interface SkillArchiveResponse {
  id: string;
  reason: string;
  status: string;
}

/** SkillFilesResponse */
export interface SkillFilesResponse {
  count: number;
  dir: string;
  files: string[] | null;
  id: string;
  name: string;
}

/** SkillImportResponse */
export interface SkillImportResponse {
  created: boolean;
  id: string;
  name: string;
  resources: string[] | null;
  status: string;
}

/** SkillImportRequest */
export interface SkillImportRequest {
  agent?: string;
  body?: string;
  description?: string;
  name?: string;
  resources?: Record<string, string>;
  tools_required?: string[];
  triggers?: string[];
}

/** SkillPromoteResponse */
export interface SkillPromoteResponse {
  id: string;
  status: string;
}

/** SkillRevertResponse */
export interface SkillRevertResponse {
  id: string;
  restored: string;
}

/** SkillShareResponse */
export interface SkillShareResponse {
  id: string;
  name?: string;
  shared: boolean;
}

/** SkillListResponseSkillsItemMetrics */
export interface SkillListResponseSkillsItemMetrics {
  failures: number;
  last_used_ms: number;
  shadow_evals: number;
  shadow_wins: number;
  successes: number;
  uses: number;
}

/** SkillListResponseSkillsItem */
export interface SkillListResponseSkillsItem {
  agent: string;
  body?: string;
  created_ms: number;
  description: string;
  id: string;
  last_seen_ms: number;
  lineage?: string[];
  metrics: SkillListResponseSkillsItemMetrics;
  name: string;
  resources?: string[];
  source_event?: string;
  status: string;
  tools_required?: string[];
  triggers?: string[];
  version: string;
}

/** SkillListResponse */
export interface SkillListResponse {
  active_count: number;
  count: number;
  skills: SkillListResponseSkillsItem[] | null;
}

/** SkillHygieneResponseIdleItem */
export interface SkillHygieneResponseIdleItem {
  agent: string;
  body?: string;
  created_ms: number;
  description: string;
  id: string;
  last_seen_ms: number;
  last_used_ms: number;
  lineage?: string[];
  metrics: SkillListResponseSkillsItemMetrics;
  name: string;
  resources?: string[];
  source_event?: string;
  status: string;
  tools_required?: string[];
  triggers?: string[];
  uses: number;
  version: string;
}

/** SkillHygieneResponse */
export interface SkillHygieneResponse {
  active: number;
  idle: SkillHygieneResponseIdleItem[] | null;
  idle_count: number;
  idle_days: number;
  total: number;
}

/** SpendTodayResponse */
export interface SpendTodayResponse {
  total: number;
}

/** StandingListResponseOrdersItemInitiative */
export interface StandingListResponseOrdersItemInitiative {
  budget_per_run_mc?: number;
  max_trust?: string;
  mode: string;
}

/** StandingListResponseOrdersItemTriggersItem */
export interface StandingListResponseOrdersItemTriggersItem {
  schedule?: string;
  subject?: string;
  type: string;
}

/** StandingListResponseOrdersItem */
export interface StandingListResponseOrdersItem {
  agent?: string;
  assure?: number;
  briefing_channel?: string;
  briefing_disposition_min?: string;
  cooldown_sec?: number;
  created_ms: number;
  enabled: boolean;
  frequency_warning?: string;
  id: string;
  initiative: StandingListResponseOrdersItemInitiative;
  name: string;
  observers?: string[];
  plan?: string;
  scope_entities?: string[];
  target_error?: string;
  target_status?: string;
  triggers: StandingListResponseOrdersItemTriggersItem[] | null;
  updated_ms: number;
}

/** StandingListResponse */
export interface StandingListResponse {
  count: number;
  enabled_count: number;
  orders: StandingListResponseOrdersItem[] | null;
}

/** StandingAddResponse */
export interface StandingAddResponse {
  order: StandingListResponseOrdersItem;
}

/** StandingAddRequest */
export interface StandingAddRequest {
  agent?: string;
  assure?: number;
  briefing_min?: string;
  cooldown_sec?: number;
  enabled?: boolean;
  id?: string;
  max_trust?: string;
  mode?: string;
  name?: string;
  order?: Record<string, any>;
  plan?: string;
}

/** StandingEditResponseOrder */
export interface StandingEditResponseOrder {
  agent?: string;
  assure?: number;
  briefing_channel?: string;
  briefing_disposition_min?: string;
  cooldown_sec?: number;
  created_ms: number;
  enabled: boolean;
  frequency_warning?: string;
  id: string;
  initiative: StandingListResponseOrdersItemInitiative;
  name: string;
  observers?: string[];
  plan?: string;
  scope_entities?: string[];
  target_error?: string;
  target_status?: string;
  triggers: StandingListResponseOrdersItemTriggersItem[] | null;
  updated_ms: number;
}

/** StandingEditResponse */
export interface StandingEditResponse {
  order?: StandingEditResponseOrder;
  updated: boolean;
}

/** StandingFireResponse */
export interface StandingFireResponse {
  fired: boolean;
  id: string;
}

/** StandingWhyResponseEventsItem */
export interface StandingWhyResponseEventsItem {
  correlation_id: string;
  id: string;
  kind: string;
  payload: Record<string, any> | null;
  seq: number;
  ts_unix_ms: number;
}

/** StandingWhyResponse */
export interface StandingWhyResponse {
  count: number;
  events: StandingWhyResponseEventsItem[] | null;
  id: string;
}

/** StatusResponseChannelsItem */
export interface StatusResponseChannelsItem {
  addr: string;
  allowlist: number;
  inbound: boolean;
  kind: string;
}

/** StatusResponseDelegation */
export interface StatusResponseDelegation {
  enabled: boolean;
  max_depth: number;
  max_fanout: number;
  max_spend_microcents: number;
  max_total: number;
}

/** StatusResponseHttpServersItem */
export interface StatusResponseHttpServersItem {
  addr: string;
  loopback: boolean;
  name: string;
}

/** StatusResponseModelFallbacks */
export interface StatusResponseModelFallbacks {
  count: number;
  last_ms: number;
  last_reason: string;
}

/** StatusResponseSchedules */
export interface StatusResponseSchedules {
  enabled: number;
  resident: boolean;
  running: number;
  total: number;
}

/** StatusResponse */
export interface StatusResponse {
  active_runs: number;
  active_skills: number;
  channels?: StatusResponseChannelsItem[];
  cred_chain?: string;
  daemon: string;
  delegation: StatusResponseDelegation;
  halted: boolean;
  http_servers?: StatusResponseHttpServersItem[];
  journal_head: number;
  memory_records: number;
  model: string;
  model_fallbacks: StatusResponseModelFallbacks;
  pending_approvals: number;
  protocol: number;
  provider_fallbacks: StatusResponseModelFallbacks;
  schedules: StatusResponseSchedules;
  tenants?: number;
  tools: number;
  uptime_seconds: number;
  world_entities: number;
}

/** ChatSuggestionsResponseSuggestionsItem */
export interface ChatSuggestionsResponseSuggestionsItem {
  category: string;
  icon?: string;
  id: string;
  label: string;
  prompt: string;
}

/** ChatSuggestionsResponse */
export interface ChatSuggestionsResponse {
  suggestions: ChatSuggestionsResponseSuggestionsItem[] | null;
}

/** ToolLogResponseInvocationsItem */
export interface ToolLogResponseInvocationsItem {
  actor: string;
  call_id: string;
  correlation_id: string;
  directive_like: boolean;
  directive_matches: string[] | null;
  duration_ms: number;
  error: boolean;
  input: string;
  not_executed?: boolean;
  observation_source: string;
  observation_trust: string;
  output: string;
  seq: number;
  tool: string;
  ts_unix_ms: number;
}

/** ToolLogResponse */
export interface ToolLogResponse {
  count: number;
  invocations: ToolLogResponseInvocationsItem[] | null;
  next_cursor: string;
}

/** ToolboxInstallResponse */
export interface ToolboxInstallResponse {
  failed: string[] | null;
  installed: string[] | null;
  skipped: string[] | null;
}

/** ToolListResponseToolsItem */
export interface ToolListResponseToolsItem {
  capability: string;
  description: string;
  effect_class: string;
  name: string;
  rollback_mode: string;
  rollback_notes: string;
}

/** ToolListResponse */
export interface ToolListResponse {
  count: number;
  tools: ToolListResponseToolsItem[] | null;
}

/** VersionResponse */
export interface VersionResponse {
  agezt: string;
  build_modified: boolean;
  built: string;
  protocol_version: number;
  revision: string;
}

/** WardenLogResponseExecutionsItem */
export interface WardenLogResponseExecutionsItem {
  argv0?: string;
  downgraded?: boolean;
  duration_ms?: number;
  exit_code?: number;
  kind: string;
  profile: string;
  reason?: string;
  requested?: string;
  seq: number;
  timed_out?: boolean;
  ts_unix_ms: number;
}

/** WardenLogResponse */
export interface WardenLogResponse {
  count: number;
  executions: WardenLogResponseExecutionsItem[] | null;
  next_cursor: string;
}

/** WebhookLogResponseDeliveriesItem */
export interface WebhookLogResponseDeliveriesItem {
  attempts: number;
  error?: string;
  event_kind: string;
  ok: boolean;
  seq: number;
  status?: number;
  ts_unix_ms: number;
  url: string;
}

/** WebhookLogResponse */
export interface WebhookLogResponse {
  count: number;
  deliveries: WebhookLogResponseDeliveriesItem[] | null;
  next_cursor: string;
}

/** WhatsappgwQrResponse */
export interface WhatsappgwQrResponse {
  error?: string;
  http_status?: number;
  ok: boolean;
  qr?: string;
}

/** WhatsappgwQrRequest */
export interface WhatsappgwQrRequest {
  backend?: string;
  key?: string;
  session?: string;
  url?: string;
}

/** WhatsappgwStatusResponse */
export interface WhatsappgwStatusResponse {
  connected?: boolean;
  error?: string;
  http_status?: number;
  ok: boolean;
  status?: string;
}

/** WorkflowListResponseWorkflowsItemLastRun */
export interface WorkflowListResponseWorkflowsItemLastRun {
  at_ms: number;
  duration_ms?: number;
  status: string;
}

/** WorkflowListResponseWorkflowsItem */
export interface WorkflowListResponseWorkflowsItem {
  created_ms: number;
  description?: string;
  edge_count: number;
  enabled: boolean;
  id: string;
  last_run?: WorkflowListResponseWorkflowsItemLastRun;
  name: string;
  node_count: number;
  trigger_detail?: string;
  trigger_kind: string;
  updated_ms: number;
}

/** WorkflowListResponse */
export interface WorkflowListResponse {
  count: number;
  enabled_count: number;
  workflows: WorkflowListResponseWorkflowsItem[] | null;
}

/** WorkflowDraftResponseWorkflowEdgesItem */
export interface WorkflowDraftResponseWorkflowEdgesItem {
  from: string;
  port?: string;
  to: string;
}

/** WorkflowDraftResponseWorkflowNodesItem */
export interface WorkflowDraftResponseWorkflowNodesItem {
  config?: any;
  id: string;
  label?: string;
  retries?: number;
  retry_delay_sec?: number;
  timeout_sec?: number;
  type: string;
  x?: number;
  y?: number;
}

/** WorkflowDraftResponseWorkflow */
export interface WorkflowDraftResponseWorkflow {
  created_ms: number;
  description?: string;
  edge_count: number;
  edges?: WorkflowDraftResponseWorkflowEdgesItem[];
  enabled: boolean;
  id: string;
  last_run?: WorkflowListResponseWorkflowsItemLastRun;
  name: string;
  node_count: number;
  nodes: WorkflowDraftResponseWorkflowNodesItem[] | null;
  trigger_detail?: string;
  trigger_kind: string;
  updated_ms: number;
}

/** WorkflowDraftResponse */
export interface WorkflowDraftResponse {
  correlation_id: string;
  workflow: WorkflowDraftResponseWorkflow;
}

/** WorkflowDraftRequest */
export interface WorkflowDraftRequest {
  async?: boolean;
  data?: Record<string, any>;
  description?: string;
  enabled?: boolean | string;
  instruction?: string;
  limit?: number;
  name?: string;
  node?: string;
  payload?: Record<string, any> | any[] | string | number | boolean;
  reason?: string;
  ref?: string;
  secret?: string;
  with_runs?: boolean | string;
  workflow?: Record<string, any>;
}

/** WorkflowSetEnabledResponse */
export interface WorkflowSetEnabledResponse {
  workflow: WorkflowListResponseWorkflowsItem;
}

/** WorkflowRunResponse */
export interface WorkflowRunResponse {
  accepted?: boolean;
  async?: boolean;
  correlation_id: string;
  executed?: string[];
  outputs?: Record<string, any>;
  workflow?: string;
}

/** WorkflowRunsResponseRunsItemNodeEventsItem */
export interface WorkflowRunsResponseRunsItemNodeEventsItem {
  attempts?: number;
  error?: string;
  handled?: boolean;
  input?: string;
  label?: string;
  node: string;
  ok: boolean;
  output?: string;
  output_truncated?: boolean;
  port?: string;
  ts_ms: number;
  type?: string;
}

/** WorkflowRunsResponseRunsItem */
export interface WorkflowRunsResponseRunsItem {
  agent?: string;
  correlation_id: string;
  error?: string;
  executed?: any[];
  finished_ms?: number;
  node_events: WorkflowRunsResponseRunsItemNodeEventsItem[] | null;
  parent_correlation_id?: string;
  runner?: string;
  schedule_id?: string;
  source?: string;
  standing_id?: string;
  started_ms: number;
  status: string;
  trigger_subject?: string;
}

/** WorkflowRunsResponse */
export interface WorkflowRunsResponse {
  count: number;
  runs: WorkflowRunsResponseRunsItem[] | null;
  workflow: string;
}

/** WorkflowSaveResponse */
export interface WorkflowSaveResponse {
  created: boolean;
  workflow: WorkflowDraftResponseWorkflow;
}

/** WorkflowShowResponse */
export interface WorkflowShowResponse {
  workflow: WorkflowDraftResponseWorkflow;
}

/** WorkflowTemplatesResponseTemplatesItem */
export interface WorkflowTemplatesResponseTemplatesItem {
  category: string;
  description: string;
  name: string;
  node_count: number;
  title: string;
  workflow: WorkflowDraftResponseWorkflow;
}

/** WorkflowTemplatesResponse */
export interface WorkflowTemplatesResponse {
  count: number;
  templates: WorkflowTemplatesResponseTemplatesItem[] | null;
}

/** WorkflowTestNodeResponse */
export interface WorkflowTestNodeResponse {
  attempts: number;
  correlation_id: string;
  output: any;
  port: string;
}

/** WorldListResponseEdgesItem */
export interface WorldListResponseEdgesItem {
  from: string;
  id: string;
  to: string;
  verb: string;
  weight: number;
}

/** WorldListResponseEntitiesItem */
export interface WorldListResponseEntitiesItem {
  aliases?: string[];
  attrs?: Record<string, string>;
  created_ms: number;
  id: string;
  kind: string;
  last_seen_ms: number;
  name: string;
  source_event?: string;
  superseded_by?: string;
  tombstoned?: boolean;
  weight: number;
}

/** WorldListResponse */
export interface WorldListResponse {
  count: number;
  edges: WorldListResponseEdgesItem[] | null;
  entities: WorldListResponseEntitiesItem[] | null;
  relation_count: number;
}

/** WorldAddResponse */
export interface WorldAddResponse {
  created: boolean;
  id: string;
  kind: string;
  name: string;
}

/** WorldAddRequest */
export interface WorldAddRequest {
  aliases?: string[];
  attrs?: Record<string, string>;
  kind?: string;
  name: string;
}

/** WorldEditResponse */
export interface WorldEditResponse {
  id?: string;
  kind?: string;
  name?: string;
  updated: boolean;
}

/** WorldEditRequest */
export interface WorldEditRequest {
  aliases?: string[];
  attrs?: Record<string, string>;
  id: string;
}

/** WorldRelateResponse */
export interface WorldRelateResponse {
  from: string;
  id: string;
  to: string;
  verb: string;
}

/** WorldLogResponseOpsItem */
export interface WorldLogResponseOpsItem {
  label: string;
  op: string;
  seq: number;
  ts_unix_ms: number;
  what: string;
}

/** WorldLogResponse */
export interface WorldLogResponse {
  count: number;
  next_cursor: string;
  ops: WorldLogResponseOpsItem[] | null;
}

/** AuthmetaResponse */
export interface AuthmetaResponse {
  authed: boolean;
  password_required: boolean;
}

/** LoginResponse */
export interface LoginResponse {
  ok: boolean;
  password_required?: boolean;
}

/** LogoutResponse */
export interface LogoutResponse {
  ok: boolean;
}

/** RollbackCheckpointsResponseCheckpointsItem */
export interface RollbackCheckpointsResponseCheckpointsItem {
  action: string;
  applied_ms?: number;
  before?: Record<string, any>;
  before_status?: string;
  created_ms: number;
  id: string;
  kind: string;
  reason?: string;
  run_id?: string;
  subject_id: string;
  subject_name?: string;
}

/** RollbackCheckpointsResponse */
export interface RollbackCheckpointsResponse {
  checkpoints: RollbackCheckpointsResponseCheckpointsItem[];
  count: number;
  run_id?: string;
}

/** RollbackApplyResponse */
export interface RollbackApplyResponse {
  applied: boolean;
  checkpoint: RollbackCheckpointsResponseCheckpointsItem;
  reason?: string;
  result?: Record<string, any>;
}

/** SseTokenResponse */
export interface SseTokenResponse {
  token: string;
}

/** VoiceStatusResponseStt */
export interface VoiceStatusResponseStt {
  configured: boolean;
}

/** VoiceStatusResponse */
export interface VoiceStatusResponse {
  stt: VoiceStatusResponseStt;
  tts: VoiceStatusResponseStt;
}

/** FilesTreeResponseNodesItem */
export interface FilesTreeResponseNodesItem {
  modified_ms?: number;
  name: string;
  path: string;
  size?: number;
  type: string;
}

/** FilesTreeResponse */
export interface FilesTreeResponse {
  nodes: FilesTreeResponseNodesItem[];
  root: string;
}

/** WorkflowHookResponse */
export interface WorkflowHookResponse {
  accepted?: boolean;
  correlation_id: string | null;
  executed?: any;
  ok?: boolean;
  outputs?: any;
  workflow: string | null;
}

/** ConsoleErrorResponse */
export interface ConsoleErrorResponse {
  error: string;
}

/** AgentAddResponseProfileHealthPolicy is the same shape as AgentListResponseProfilesItemHealthPolicy. */
export type AgentAddResponseProfileHealthPolicy = AgentListResponseProfilesItemHealthPolicy;

/** AgentAddResponseProfileLifecycle is the same shape as AgentListResponseProfilesItemLifecycle. */
export type AgentAddResponseProfileLifecycle = AgentListResponseProfilesItemLifecycle;

/** AgentAddResponseProfileNoisePolicy is the same shape as AgentListResponseProfilesItemNoisePolicy. */
export type AgentAddResponseProfileNoisePolicy = AgentListResponseProfilesItemNoisePolicy;

/** AgentAddResponseProfileRetryPolicy is the same shape as AgentListResponseProfilesItemRetryPolicy. */
export type AgentAddResponseProfileRetryPolicy = AgentListResponseProfilesItemRetryPolicy;

/** AgentAddResponseProfileSelfRepair is the same shape as AgentListResponseProfilesItemSelfRepair. */
export type AgentAddResponseProfileSelfRepair = AgentListResponseProfilesItemSelfRepair;

/** AgentAddResponseProfileStatus is the same shape as AgentListResponseProfilesItemStatus. */
export type AgentAddResponseProfileStatus = AgentListResponseProfilesItemStatus;

/** AgentAddResponseProfileTasklistItem is the same shape as AgentListResponseProfilesItemTasklistItem. */
export type AgentAddResponseProfileTasklistItem = AgentListResponseProfilesItemTasklistItem;

/** AgentAddResponseProfile is the same shape as AgentListResponseProfilesItem. */
export type AgentAddResponseProfile = AgentListResponseProfilesItem;

/** AgentCapabilitiesResponseProfileHealthPolicy is the same shape as AgentListResponseProfilesItemHealthPolicy. */
export type AgentCapabilitiesResponseProfileHealthPolicy = AgentListResponseProfilesItemHealthPolicy;

/** AgentCapabilitiesResponseProfileLifecycle is the same shape as AgentListResponseProfilesItemLifecycle. */
export type AgentCapabilitiesResponseProfileLifecycle = AgentListResponseProfilesItemLifecycle;

/** AgentCapabilitiesResponseProfileNoisePolicy is the same shape as AgentListResponseProfilesItemNoisePolicy. */
export type AgentCapabilitiesResponseProfileNoisePolicy = AgentListResponseProfilesItemNoisePolicy;

/** AgentCapabilitiesResponseProfileRetryPolicy is the same shape as AgentListResponseProfilesItemRetryPolicy. */
export type AgentCapabilitiesResponseProfileRetryPolicy = AgentListResponseProfilesItemRetryPolicy;

/** AgentCapabilitiesResponseProfileSelfRepair is the same shape as AgentListResponseProfilesItemSelfRepair. */
export type AgentCapabilitiesResponseProfileSelfRepair = AgentListResponseProfilesItemSelfRepair;

/** AgentCapabilitiesResponseProfileStatus is the same shape as AgentListResponseProfilesItemStatus. */
export type AgentCapabilitiesResponseProfileStatus = AgentListResponseProfilesItemStatus;

/** AgentCapabilitiesResponseProfileTasklistItem is the same shape as AgentListResponseProfilesItemTasklistItem. */
export type AgentCapabilitiesResponseProfileTasklistItem = AgentListResponseProfilesItemTasklistItem;

/** AgentCapabilitiesResponseProfile is the same shape as AgentListResponseProfilesItem. */
export type AgentCapabilitiesResponseProfile = AgentListResponseProfilesItem;

/** AgentEditResponseProfileHealthPolicy is the same shape as AgentListResponseProfilesItemHealthPolicy. */
export type AgentEditResponseProfileHealthPolicy = AgentListResponseProfilesItemHealthPolicy;

/** AgentEditResponseProfileLifecycle is the same shape as AgentListResponseProfilesItemLifecycle. */
export type AgentEditResponseProfileLifecycle = AgentListResponseProfilesItemLifecycle;

/** AgentEditResponseProfileNoisePolicy is the same shape as AgentListResponseProfilesItemNoisePolicy. */
export type AgentEditResponseProfileNoisePolicy = AgentListResponseProfilesItemNoisePolicy;

/** AgentEditResponseProfileRetryPolicy is the same shape as AgentListResponseProfilesItemRetryPolicy. */
export type AgentEditResponseProfileRetryPolicy = AgentListResponseProfilesItemRetryPolicy;

/** AgentEditResponseProfileSelfRepair is the same shape as AgentListResponseProfilesItemSelfRepair. */
export type AgentEditResponseProfileSelfRepair = AgentListResponseProfilesItemSelfRepair;

/** AgentEditResponseProfileStatus is the same shape as AgentListResponseProfilesItemStatus. */
export type AgentEditResponseProfileStatus = AgentListResponseProfilesItemStatus;

/** AgentEditResponseProfileTasklistItem is the same shape as AgentListResponseProfilesItemTasklistItem. */
export type AgentEditResponseProfileTasklistItem = AgentListResponseProfilesItemTasklistItem;

/** AgentEditResponseProfile is the same shape as AgentListResponseProfilesItem. */
export type AgentEditResponseProfile = AgentListResponseProfilesItem;

/** AgentEditResponse is the same shape as AgentAddResponse. */
export type AgentEditResponse = AgentAddResponse;

/** AgentSetEnabledResponseProfileHealthPolicy is the same shape as AgentListResponseProfilesItemHealthPolicy. */
export type AgentSetEnabledResponseProfileHealthPolicy = AgentListResponseProfilesItemHealthPolicy;

/** AgentSetEnabledResponseProfileLifecycle is the same shape as AgentListResponseProfilesItemLifecycle. */
export type AgentSetEnabledResponseProfileLifecycle = AgentListResponseProfilesItemLifecycle;

/** AgentSetEnabledResponseProfileNoisePolicy is the same shape as AgentListResponseProfilesItemNoisePolicy. */
export type AgentSetEnabledResponseProfileNoisePolicy = AgentListResponseProfilesItemNoisePolicy;

/** AgentSetEnabledResponseProfileRetryPolicy is the same shape as AgentListResponseProfilesItemRetryPolicy. */
export type AgentSetEnabledResponseProfileRetryPolicy = AgentListResponseProfilesItemRetryPolicy;

/** AgentSetEnabledResponseProfileSelfRepair is the same shape as AgentListResponseProfilesItemSelfRepair. */
export type AgentSetEnabledResponseProfileSelfRepair = AgentListResponseProfilesItemSelfRepair;

/** AgentSetEnabledResponseProfileStatus is the same shape as AgentListResponseProfilesItemStatus. */
export type AgentSetEnabledResponseProfileStatus = AgentListResponseProfilesItemStatus;

/** AgentSetEnabledResponseProfileTasklistItem is the same shape as AgentListResponseProfilesItemTasklistItem. */
export type AgentSetEnabledResponseProfileTasklistItem = AgentListResponseProfilesItemTasklistItem;

/** AgentSetEnabledResponseProfile is the same shape as AgentListResponseProfilesItem. */
export type AgentSetEnabledResponseProfile = AgentListResponseProfilesItem;

/** AgentPermissionsResponseConfigEntriesItem is the same shape as AgentCapabilitiesResponseConfigEntriesItem. */
export type AgentPermissionsResponseConfigEntriesItem = AgentCapabilitiesResponseConfigEntriesItem;

/** AgentPermissionsResponseGovernance is the same shape as AgentCapabilitiesResponseGovernance. */
export type AgentPermissionsResponseGovernance = AgentCapabilitiesResponseGovernance;

/** AgentPermissionsResponsePermissionsItem is the same shape as AgentCapabilitiesResponsePermissionsItem. */
export type AgentPermissionsResponsePermissionsItem = AgentCapabilitiesResponsePermissionsItem;

/** AgentPermissionsResponseWakeAccess is the same shape as AgentCapabilitiesResponseWakeAccess. */
export type AgentPermissionsResponseWakeAccess = AgentCapabilitiesResponseWakeAccess;

/** AgentRepairStatusResponseInflightItem is the same shape as AgentRepairStatusResponseHistoryItem. */
export type AgentRepairStatusResponseInflightItem = AgentRepairStatusResponseHistoryItem;

/** AgentRetireResponseProfileHealthPolicy is the same shape as AgentListResponseProfilesItemHealthPolicy. */
export type AgentRetireResponseProfileHealthPolicy = AgentListResponseProfilesItemHealthPolicy;

/** AgentRetireResponseProfileLifecycle is the same shape as AgentListResponseProfilesItemLifecycle. */
export type AgentRetireResponseProfileLifecycle = AgentListResponseProfilesItemLifecycle;

/** AgentRetireResponseProfileNoisePolicy is the same shape as AgentListResponseProfilesItemNoisePolicy. */
export type AgentRetireResponseProfileNoisePolicy = AgentListResponseProfilesItemNoisePolicy;

/** AgentRetireResponseProfileRetryPolicy is the same shape as AgentListResponseProfilesItemRetryPolicy. */
export type AgentRetireResponseProfileRetryPolicy = AgentListResponseProfilesItemRetryPolicy;

/** AgentRetireResponseProfileSelfRepair is the same shape as AgentListResponseProfilesItemSelfRepair. */
export type AgentRetireResponseProfileSelfRepair = AgentListResponseProfilesItemSelfRepair;

/** AgentRetireResponseProfileStatus is the same shape as AgentListResponseProfilesItemStatus. */
export type AgentRetireResponseProfileStatus = AgentListResponseProfilesItemStatus;

/** AgentRetireResponseProfileTasklistItem is the same shape as AgentListResponseProfilesItemTasklistItem. */
export type AgentRetireResponseProfileTasklistItem = AgentListResponseProfilesItemTasklistItem;

/** AgentRetireResponseProfile is the same shape as AgentListResponseProfilesItem. */
export type AgentRetireResponseProfile = AgentListResponseProfilesItem;

/** AgentReviveResponseProfileHealthPolicy is the same shape as AgentListResponseProfilesItemHealthPolicy. */
export type AgentReviveResponseProfileHealthPolicy = AgentListResponseProfilesItemHealthPolicy;

/** AgentReviveResponseProfileLifecycle is the same shape as AgentListResponseProfilesItemLifecycle. */
export type AgentReviveResponseProfileLifecycle = AgentListResponseProfilesItemLifecycle;

/** AgentReviveResponseProfileNoisePolicy is the same shape as AgentListResponseProfilesItemNoisePolicy. */
export type AgentReviveResponseProfileNoisePolicy = AgentListResponseProfilesItemNoisePolicy;

/** AgentReviveResponseProfileRetryPolicy is the same shape as AgentListResponseProfilesItemRetryPolicy. */
export type AgentReviveResponseProfileRetryPolicy = AgentListResponseProfilesItemRetryPolicy;

/** AgentReviveResponseProfileSelfRepair is the same shape as AgentListResponseProfilesItemSelfRepair. */
export type AgentReviveResponseProfileSelfRepair = AgentListResponseProfilesItemSelfRepair;

/** AgentReviveResponseProfileStatus is the same shape as AgentListResponseProfilesItemStatus. */
export type AgentReviveResponseProfileStatus = AgentListResponseProfilesItemStatus;

/** AgentReviveResponseProfileTasklistItem is the same shape as AgentListResponseProfilesItemTasklistItem. */
export type AgentReviveResponseProfileTasklistItem = AgentListResponseProfilesItemTasklistItem;

/** AgentReviveResponseProfile is the same shape as AgentListResponseProfilesItem. */
export type AgentReviveResponseProfile = AgentListResponseProfilesItem;

/** AgentTaskUpdateResponseProfileHealthPolicy is the same shape as AgentListResponseProfilesItemHealthPolicy. */
export type AgentTaskUpdateResponseProfileHealthPolicy = AgentListResponseProfilesItemHealthPolicy;

/** AgentTaskUpdateResponseProfileLifecycle is the same shape as AgentListResponseProfilesItemLifecycle. */
export type AgentTaskUpdateResponseProfileLifecycle = AgentListResponseProfilesItemLifecycle;

/** AgentTaskUpdateResponseProfileNoisePolicy is the same shape as AgentListResponseProfilesItemNoisePolicy. */
export type AgentTaskUpdateResponseProfileNoisePolicy = AgentListResponseProfilesItemNoisePolicy;

/** AgentTaskUpdateResponseProfileRetryPolicy is the same shape as AgentListResponseProfilesItemRetryPolicy. */
export type AgentTaskUpdateResponseProfileRetryPolicy = AgentListResponseProfilesItemRetryPolicy;

/** AgentTaskUpdateResponseProfileSelfRepair is the same shape as AgentListResponseProfilesItemSelfRepair. */
export type AgentTaskUpdateResponseProfileSelfRepair = AgentListResponseProfilesItemSelfRepair;

/** AgentTaskUpdateResponseProfileStatus is the same shape as AgentListResponseProfilesItemStatus. */
export type AgentTaskUpdateResponseProfileStatus = AgentListResponseProfilesItemStatus;

/** AgentTaskUpdateResponseProfileTasklistItem is the same shape as AgentListResponseProfilesItemTasklistItem. */
export type AgentTaskUpdateResponseProfileTasklistItem = AgentListResponseProfilesItemTasklistItem;

/** AgentTaskUpdateResponseProfile is the same shape as AgentListResponseProfilesItem. */
export type AgentTaskUpdateResponseProfile = AgentListResponseProfilesItem;

/** AgentTaskUpdateResponseTask is the same shape as AgentListResponseProfilesItemTasklistItem. */
export type AgentTaskUpdateResponseTask = AgentListResponseProfilesItemTasklistItem;

/** AgentWakeResponse is the same shape as AgentRepairResponse. */
export type AgentWakeResponse = AgentRepairResponse;

/** BoardHelpResponseOpenHelpItem is the same shape as BoardReadResponseMessagesItem. */
export type BoardHelpResponseOpenHelpItem = BoardReadResponseMessagesItem;

/** BoardSendResponseSent is the same shape as BoardReadResponseMessagesItem. */
export type BoardSendResponseSent = BoardReadResponseMessagesItem;

/** ChannelListResponseChannelsItemFieldsItem is the same shape as ChannelListResponseChannelsItemAccountsItemFieldsItem. */
export type ChannelListResponseChannelsItemFieldsItem = ChannelListResponseChannelsItemAccountsItemFieldsItem;

/** ConfigSchemaUnregisterRequest is the same shape as ConfigSchemaRegisterRequest. */
export type ConfigSchemaUnregisterRequest = ConfigSchemaRegisterRequest;

/** ConfigcenterDeleteRequest is the same shape as ConfigcenterAccessRequest. */
export type ConfigcenterDeleteRequest = ConfigcenterAccessRequest;

/** ConfigcenterListResponseEntriesItem is the same shape as ConfigcenterAccessResponseEntry. */
export type ConfigcenterListResponseEntriesItem = ConfigcenterAccessResponseEntry;

/** ConfigcenterSetRatingRequest is the same shape as ConfigcenterAccessRequest. */
export type ConfigcenterSetRatingRequest = ConfigcenterAccessRequest;

/** ConfigcenterSetResponseEntry is the same shape as ConfigcenterAccessResponseEntry. */
export type ConfigcenterSetResponseEntry = ConfigcenterAccessResponseEntry;

/** ConfigcenterSetResponse is the same shape as ConfigcenterAccessResponse. */
export type ConfigcenterSetResponse = ConfigcenterAccessResponse;

/** ConfigcenterSetRequest is the same shape as ConfigcenterAccessRequest. */
export type ConfigcenterSetRequest = ConfigcenterAccessRequest;

/** CouncilMembersResponseMembersItem is the same shape as CouncilAskResponseMembersItem. */
export type CouncilMembersResponseMembersItem = CouncilAskResponseMembersItem;

/** DataDeleteResponse is the same shape as ArtifactDeleteResponse. */
export type DataDeleteResponse = ArtifactDeleteResponse;

/** DataRecordsResponseRecordsItem is the same shape as DataInsertResponseRecord. */
export type DataRecordsResponseRecordsItem = DataInsertResponseRecord;

/** DataRecordsResponseSchemaFieldsItem is the same shape as DataCollectionsResponseCollectionsItemFieldsItem. */
export type DataRecordsResponseSchemaFieldsItem = DataCollectionsResponseCollectionsItemFieldsItem;

/** DataRecordsResponseSchema is the same shape as DataCollectionsResponseCollectionsItem. */
export type DataRecordsResponseSchema = DataCollectionsResponseCollectionsItem;

/** DataUpdateResponseRecord is the same shape as DataInsertResponseRecord. */
export type DataUpdateResponseRecord = DataInsertResponseRecord;

/** DataUpdateResponse is the same shape as DataInsertResponse. */
export type DataUpdateResponse = DataInsertResponse;

/** MarketRemoveSourceRequest is the same shape as MarketAddSourceRequest. */
export type MarketRemoveSourceRequest = MarketAddSourceRequest;

/** MarketSourcesResponseSourcesItem is the same shape as MarketAddSourceResponse. */
export type MarketSourcesResponseSourcesItem = MarketAddSourceResponse;

/** MarketSyncRequest is the same shape as MarketAddSourceRequest. */
export type MarketSyncRequest = MarketAddSourceRequest;

/** McpAddResponseServer is the same shape as McpListResponseServersItem. */
export type McpAddResponseServer = McpListResponseServersItem;

/** McpAttachResponseServer is the same shape as McpListResponseServersItem. */
export type McpAttachResponseServer = McpListResponseServersItem;

/** McpSetEnabledResponseServer is the same shape as McpListResponseServersItem. */
export type McpSetEnabledResponseServer = McpListResponseServersItem;

/** McpSetEnabledResponse is the same shape as McpAddResponse. */
export type McpSetEnabledResponse = McpAddResponse;

/** PlanRefineResponse is the same shape as PlanGenerateResponse. */
export type PlanRefineResponse = PlanGenerateResponse;

/** ProviderOauthStartResponse is the same shape as ChannelOauthStartResponse. */
export type ProviderOauthStartResponse = ChannelOauthStartResponse;

/** ProviderOauthStatusRequest is the same shape as ChannelOauthStatusRequest. */
export type ProviderOauthStatusRequest = ChannelOauthStatusRequest;

/** PulseResumeResponse is the same shape as PulsePauseResponse. */
export type PulseResumeResponse = PulsePauseResponse;

/** PulseWatchResponse is the same shape as PulseProbeResponse. */
export type PulseWatchResponse = PulseProbeResponse;

/** ReaperScanResponseRoutingForcedFailedAgentsItem is the same shape as ReaperScanResponseRoutingForcedExhaustedAgentsItem. */
export type ReaperScanResponseRoutingForcedFailedAgentsItem = ReaperScanResponseRoutingForcedExhaustedAgentsItem;

/** ReaperScanResponseRoutingForcedProbationAgentsItem is the same shape as ReaperScanResponseRoutingForcedExhaustedAgentsItem. */
export type ReaperScanResponseRoutingForcedProbationAgentsItem = ReaperScanResponseRoutingForcedExhaustedAgentsItem;

/** ResumeResponse is the same shape as HaltResponse. */
export type ResumeResponse = HaltResponse;

/** RunResumeResponse is the same shape as RunPauseResponse. */
export type RunResumeResponse = RunPauseResponse;

/** RunStepResponse is the same shape as RunPauseResponse. */
export type RunStepResponse = RunPauseResponse;

/** ScheduleEditRequest is the same shape as ScheduleAddRequest. */
export type ScheduleEditRequest = ScheduleAddRequest;

/** ScheduleRmResponse is the same shape as McpRemoveResponse. */
export type ScheduleRmResponse = McpRemoveResponse;

/** ScheduleRunResponse is the same shape as PulseBeatResponse. */
export type ScheduleRunResponse = PulseBeatResponse;

/** ScheduleListResponseSchedulesItem is the same shape as ScheduleAddResponse. */
export type ScheduleListResponseSchedulesItem = ScheduleAddResponse;

/** SkillQuarantineResponse is the same shape as SkillPromoteResponse. */
export type SkillQuarantineResponse = SkillPromoteResponse;

/** SkillHygieneResponseIdleItemMetrics is the same shape as SkillListResponseSkillsItemMetrics. */
export type SkillHygieneResponseIdleItemMetrics = SkillListResponseSkillsItemMetrics;

/** StandingAddResponseOrderInitiative is the same shape as StandingListResponseOrdersItemInitiative. */
export type StandingAddResponseOrderInitiative = StandingListResponseOrdersItemInitiative;

/** StandingAddResponseOrderTriggersItem is the same shape as StandingListResponseOrdersItemTriggersItem. */
export type StandingAddResponseOrderTriggersItem = StandingListResponseOrdersItemTriggersItem;

/** StandingAddResponseOrder is the same shape as StandingListResponseOrdersItem. */
export type StandingAddResponseOrder = StandingListResponseOrdersItem;

/** StandingEditResponseOrderInitiative is the same shape as StandingListResponseOrdersItemInitiative. */
export type StandingEditResponseOrderInitiative = StandingListResponseOrdersItemInitiative;

/** StandingEditResponseOrderTriggersItem is the same shape as StandingListResponseOrdersItemTriggersItem. */
export type StandingEditResponseOrderTriggersItem = StandingListResponseOrdersItemTriggersItem;

/** StandingEditRequest is the same shape as StandingAddRequest. */
export type StandingEditRequest = StandingAddRequest;

/** StandingSetEnabledResponseOrderInitiative is the same shape as StandingListResponseOrdersItemInitiative. */
export type StandingSetEnabledResponseOrderInitiative = StandingListResponseOrdersItemInitiative;

/** StandingSetEnabledResponseOrderTriggersItem is the same shape as StandingListResponseOrdersItemTriggersItem. */
export type StandingSetEnabledResponseOrderTriggersItem = StandingListResponseOrdersItemTriggersItem;

/** StandingSetEnabledResponseOrder is the same shape as StandingListResponseOrdersItem. */
export type StandingSetEnabledResponseOrder = StandingListResponseOrdersItem;

/** StandingSetEnabledResponse is the same shape as StandingAddResponse. */
export type StandingSetEnabledResponse = StandingAddResponse;

/** StandingRemoveResponse is the same shape as ConfigSchemaUnregisterResponse. */
export type StandingRemoveResponse = ConfigSchemaUnregisterResponse;

/** StatusResponseProviderFallbacks is the same shape as StatusResponseModelFallbacks. */
export type StatusResponseProviderFallbacks = StatusResponseModelFallbacks;

/** WhatsappgwStatusRequest is the same shape as WhatsappgwQrRequest. */
export type WhatsappgwStatusRequest = WhatsappgwQrRequest;

/** WorkflowDraftResponseWorkflowLastRun is the same shape as WorkflowListResponseWorkflowsItemLastRun. */
export type WorkflowDraftResponseWorkflowLastRun = WorkflowListResponseWorkflowsItemLastRun;

/** WorkflowSetEnabledResponseWorkflowLastRun is the same shape as WorkflowListResponseWorkflowsItemLastRun. */
export type WorkflowSetEnabledResponseWorkflowLastRun = WorkflowListResponseWorkflowsItemLastRun;

/** WorkflowSetEnabledResponseWorkflow is the same shape as WorkflowListResponseWorkflowsItem. */
export type WorkflowSetEnabledResponseWorkflow = WorkflowListResponseWorkflowsItem;

/** WorkflowRefineResponseWorkflowEdgesItem is the same shape as WorkflowDraftResponseWorkflowEdgesItem. */
export type WorkflowRefineResponseWorkflowEdgesItem = WorkflowDraftResponseWorkflowEdgesItem;

/** WorkflowRefineResponseWorkflowLastRun is the same shape as WorkflowListResponseWorkflowsItemLastRun. */
export type WorkflowRefineResponseWorkflowLastRun = WorkflowListResponseWorkflowsItemLastRun;

/** WorkflowRefineResponseWorkflowNodesItem is the same shape as WorkflowDraftResponseWorkflowNodesItem. */
export type WorkflowRefineResponseWorkflowNodesItem = WorkflowDraftResponseWorkflowNodesItem;

/** WorkflowRefineResponseWorkflow is the same shape as WorkflowDraftResponseWorkflow. */
export type WorkflowRefineResponseWorkflow = WorkflowDraftResponseWorkflow;

/** WorkflowRefineResponse is the same shape as WorkflowDraftResponse. */
export type WorkflowRefineResponse = WorkflowDraftResponse;

/** WorkflowRefineRequest is the same shape as WorkflowDraftRequest. */
export type WorkflowRefineRequest = WorkflowDraftRequest;

/** WorkflowRemoveResponse is the same shape as McpRemoveResponse. */
export type WorkflowRemoveResponse = McpRemoveResponse;

/** WorkflowRunRequest is the same shape as WorkflowDraftRequest. */
export type WorkflowRunRequest = WorkflowDraftRequest;

/** WorkflowSaveResponseWorkflowEdgesItem is the same shape as WorkflowDraftResponseWorkflowEdgesItem. */
export type WorkflowSaveResponseWorkflowEdgesItem = WorkflowDraftResponseWorkflowEdgesItem;

/** WorkflowSaveResponseWorkflowLastRun is the same shape as WorkflowListResponseWorkflowsItemLastRun. */
export type WorkflowSaveResponseWorkflowLastRun = WorkflowListResponseWorkflowsItemLastRun;

/** WorkflowSaveResponseWorkflowNodesItem is the same shape as WorkflowDraftResponseWorkflowNodesItem. */
export type WorkflowSaveResponseWorkflowNodesItem = WorkflowDraftResponseWorkflowNodesItem;

/** WorkflowSaveResponseWorkflow is the same shape as WorkflowDraftResponseWorkflow. */
export type WorkflowSaveResponseWorkflow = WorkflowDraftResponseWorkflow;

/** WorkflowSaveRequest is the same shape as WorkflowDraftRequest. */
export type WorkflowSaveRequest = WorkflowDraftRequest;

/** WorkflowShowResponseWorkflowEdgesItem is the same shape as WorkflowDraftResponseWorkflowEdgesItem. */
export type WorkflowShowResponseWorkflowEdgesItem = WorkflowDraftResponseWorkflowEdgesItem;

/** WorkflowShowResponseWorkflowLastRun is the same shape as WorkflowListResponseWorkflowsItemLastRun. */
export type WorkflowShowResponseWorkflowLastRun = WorkflowListResponseWorkflowsItemLastRun;

/** WorkflowShowResponseWorkflowNodesItem is the same shape as WorkflowDraftResponseWorkflowNodesItem. */
export type WorkflowShowResponseWorkflowNodesItem = WorkflowDraftResponseWorkflowNodesItem;

/** WorkflowShowResponseWorkflow is the same shape as WorkflowDraftResponseWorkflow. */
export type WorkflowShowResponseWorkflow = WorkflowDraftResponseWorkflow;

/** WorkflowTemplatesResponseTemplatesItemWorkflowEdgesItem is the same shape as WorkflowDraftResponseWorkflowEdgesItem. */
export type WorkflowTemplatesResponseTemplatesItemWorkflowEdgesItem = WorkflowDraftResponseWorkflowEdgesItem;

/** WorkflowTemplatesResponseTemplatesItemWorkflowLastRun is the same shape as WorkflowListResponseWorkflowsItemLastRun. */
export type WorkflowTemplatesResponseTemplatesItemWorkflowLastRun = WorkflowListResponseWorkflowsItemLastRun;

/** WorkflowTemplatesResponseTemplatesItemWorkflowNodesItem is the same shape as WorkflowDraftResponseWorkflowNodesItem. */
export type WorkflowTemplatesResponseTemplatesItemWorkflowNodesItem = WorkflowDraftResponseWorkflowNodesItem;

/** WorkflowTemplatesResponseTemplatesItemWorkflow is the same shape as WorkflowDraftResponseWorkflow. */
export type WorkflowTemplatesResponseTemplatesItemWorkflow = WorkflowDraftResponseWorkflow;

/** WorkflowTestNodeRequest is the same shape as WorkflowDraftRequest. */
export type WorkflowTestNodeRequest = WorkflowDraftRequest;

/** WorldForgetResponse is the same shape as MemoryForgetResponse. */
export type WorldForgetResponse = MemoryForgetResponse;

/** RollbackApplyResponseCheckpoint is the same shape as RollbackCheckpointsResponseCheckpointsItem. */
export type RollbackApplyResponseCheckpoint = RollbackCheckpointsResponseCheckpointsItem;

/** VoiceStatusResponseTts is the same shape as VoiceStatusResponseStt. */
export type VoiceStatusResponseTts = VoiceStatusResponseStt;

/**
 * OperationResponseMap binds a path to the shape the console answers it
 * with. A path that is not in this map is not a path the console
 * answers for an operation.
 */
export interface OperationResponseMap {
  "/api/acp/agents": AcpAgentsResponse;
  "/api/agents": AgentListResponse;
  "/api/agents/activity": AgentActivityResponse;
  "/api/agents/add": AgentAddResponse;
  "/api/agents/capabilities": AgentCapabilitiesResponse;
  "/api/agents/edit": AgentEditResponse;
  "/api/agents/enable": AgentSetEnabledResponse;
  "/api/agents/escalations": AgentEscalationsResponse;
  "/api/agents/impact": AgentImpactResponse;
  "/api/agents/permissions": AgentPermissionsResponse;
  "/api/agents/remove": AgentRemoveResponse;
  "/api/agents/repair": AgentRepairResponse;
  "/api/agents/repair_status": AgentRepairStatusResponse;
  "/api/agents/resolve": AgentResolveResponse;
  "/api/agents/retire": AgentRetireResponse;
  "/api/agents/revive": AgentReviveResponse;
  "/api/agents/task": AgentTaskUpdateResponse;
  "/api/agents/wake": AgentWakeResponse;
  "/api/approvals": ApprovalsResponse;
  "/api/approvals_log": ApprovalsLogResponse;
  "/api/artifact/collect": ArtifactCollectResponse;
  "/api/artifact/delete": ArtifactDeleteResponse;
  "/api/artifacts": ArtifactListResponse;
  "/api/attention": AttentionResponse;
  "/api/autonomy": AutonomyFeedResponse;
  "/api/board": BoardReadResponse;
  "/api/board/ack": BoardAckResponse;
  "/api/board/help": BoardHelpResponse;
  "/api/board/send": BoardSendResponse;
  "/api/budget": BudgetResponse;
  "/api/cancel_run": CancelRunResponse;
  "/api/catalog": CatalogListResponse;
  "/api/catalog/sync": CatalogSyncResponse;
  "/api/chains": ChainsGetResponse;
  "/api/chains/set": ChainsSetResponse;
  "/api/channel/account/remove": ChannelAccountRemoveResponse;
  "/api/channel/account/set": ChannelAccountSetResponse;
  "/api/channel/oauth/start": ChannelOauthStartResponse;
  "/api/channel/oauth/status": ChannelOauthStatusResponse;
  "/api/channels": ChannelListResponse;
  "/api/chat/summarize": ChatSummarizeResponse;
  "/api/config": ConfigResponse;
  "/api/config/schema": ConfigSchemaResponse;
  "/api/config/schema/register": ConfigSchemaRegisterResponse;
  "/api/config/schema/unregister": ConfigSchemaUnregisterResponse;
  "/api/config/set": ConfigSetResponse;
  "/api/config/values": ConfigValuesResponse;
  "/api/configcenter/access": ConfigcenterAccessResponse;
  "/api/configcenter/delete": ConfigcenterDeleteResponse;
  "/api/configcenter/list": ConfigcenterListResponse;
  "/api/configcenter/rating": ConfigcenterSetRatingResponse;
  "/api/configcenter/set": ConfigcenterSetResponse;
  "/api/council/ask": CouncilAskResponse;
  "/api/council/members": CouncilMembersResponse;
  "/api/council/set": CouncilSetResponse;
  "/api/data/collections": DataCollectionsResponse;
  "/api/data/delete": DataDeleteResponse;
  "/api/data/insert": DataInsertResponse;
  "/api/data/records": DataRecordsResponse;
  "/api/data/update": DataUpdateResponse;
  "/api/decide": DecideResponse;
  "/api/edict/deny_add": EdictDenyAddResponse;
  "/api/edict/deny_rm": EdictDenyRmResponse;
  "/api/edict/set_level": EdictSetLevelResponse;
  "/api/edict/set_mode": EdictSetModeResponse;
  "/api/edict/test": EdictTestResponse;
  "/api/edict_show": EdictShowResponse;
  "/api/execution_profile_check": ExecutionProfileCheckResponse;
  "/api/execution_profiles": ExecutionProfilesResponse;
  "/api/halt": HaltResponse;
  "/api/inbox": InboxResponse;
  "/api/journal": JournalGrepResponse;
  "/api/market": MarketListResponse;
  "/api/market/install": MarketInstallResponse;
  "/api/market/show": MarketShowResponse;
  "/api/market/source/add": MarketAddSourceResponse;
  "/api/market/source/remove": MarketRemoveSourceResponse;
  "/api/market/sources": MarketSourcesResponse;
  "/api/market/sync": MarketSyncResponse;
  "/api/market/uninstall": MarketUninstallResponse;
  "/api/mcp": McpListResponse;
  "/api/mcp/add": McpAddResponse;
  "/api/mcp/attach": McpAttachResponse;
  "/api/mcp/detach": McpDetachResponse;
  "/api/mcp/enable": McpSetEnabledResponse;
  "/api/mcp/remove": McpRemoveResponse;
  "/api/memory": MemoryListResponse;
  "/api/memory/add": MemoryAddResponse;
  "/api/memory/audit": MemoryAuditResponse;
  "/api/memory/bulk_forget": MemoryBulkForgetResponse;
  "/api/memory/clean": MemoryCleanResponse;
  "/api/memory/forget": MemoryForgetResponse;
  "/api/memory/promote": MemoryPromoteResponse;
  "/api/memory/prune": MemoryPruneResponse;
  "/api/memory/supersede": MemorySupersedeResponse;
  "/api/memory/tidy": MemoryTidyResponse;
  "/api/memory_log": MemoryLogResponse;
  "/api/netguard_log": NetguardLogResponse;
  "/api/nodes": NodeRegistryResponse;
  "/api/persona": PersonaGetResponse;
  "/api/persona/set": PersonaSetResponse;
  "/api/plan/generate": PlanGenerateResponse;
  "/api/plan/refine": PlanRefineResponse;
  "/api/plan/run": PlanResponse;
  "/api/plan_history": PlanHistoryResponse;
  "/api/policy": EdictStatsResponse;
  "/api/policy_log": EdictLogResponse;
  "/api/profile/rebuild": ProfileRebuildResponse;
  "/api/prompts": PromptsGetResponse;
  "/api/prompts/set": PromptsSetResponse;
  "/api/provider/connect": ProviderConnectResponse;
  "/api/provider/keys": ProviderKeyListResponse;
  "/api/provider/keys/activate": ProviderKeyActivateResponse;
  "/api/provider/keys/add": ProviderKeyAddResponse;
  "/api/provider/keys/remove": ProviderKeyRemoveResponse;
  "/api/provider/oauth/import": ProviderOauthImportResponse;
  "/api/provider/oauth/logout": ProviderOauthLogoutResponse;
  "/api/provider/oauth/start": ProviderOauthStartResponse;
  "/api/provider/oauth/status": ProviderOauthStatusResponse;
  "/api/provider/probe": ProviderProbeResponse;
  "/api/provider/reload": ProviderReloadResponse;
  "/api/provider_log": ProviderLogResponse;
  "/api/pulse": Record<string, any> | null;
  "/api/pulse/beat": PulseBeatResponse;
  "/api/pulse/cadence": PulseCadenceResponse;
  "/api/pulse/dial": PulseDialResponse;
  "/api/pulse/flush": PulseFlushResponse;
  "/api/pulse/pause": PulsePauseResponse;
  "/api/pulse/probe": PulseProbeResponse;
  "/api/pulse/quiet": PulseQuietResponse;
  "/api/pulse/resume": PulseResumeResponse;
  "/api/pulse/unwatch": PulseUnwatchResponse;
  "/api/pulse/watch": PulseWatchResponse;
  "/api/ratelimit_log": RatelimitLogResponse;
  "/api/reaper/scan": ReaperScanResponse;
  "/api/redact/test": RedactTestResponse;
  "/api/research/ask": ResearchAskResponse;
  "/api/resume": ResumeResponse;
  "/api/routing": RoutingGetResponse;
  "/api/routing/set": RoutingSetResponse;
  "/api/run": RunResponse;
  "/api/run/pause": RunPauseResponse;
  "/api/run/resume": RunResumeResponse;
  "/api/run/steer": RunSteerResponse;
  "/api/run/step": RunStepResponse;
  "/api/runs": RunsListResponse;
  "/api/sandbox": SandboxListResponse;
  "/api/sandbox/delete": SandboxDeleteResponse;
  "/api/sandbox_file": SandboxFileResponse;
  "/api/schedule/add": ScheduleAddResponse;
  "/api/schedule/edit": ScheduleEditResponse;
  "/api/schedule/enable": ScheduleEnableResponse;
  "/api/schedule/fires": ScheduleFiresResponse;
  "/api/schedule/remove": ScheduleRmResponse;
  "/api/schedule/run": ScheduleRunResponse;
  "/api/schedule/system_tasks": ScheduleSystemTasksResponse;
  "/api/schedule/test": ScheduleTestResponse;
  "/api/schedules": ScheduleListResponse;
  "/api/send": SendResponse;
  "/api/skill/archive": SkillArchiveResponse;
  "/api/skill/files": SkillFilesResponse;
  "/api/skill/import": SkillImportResponse;
  "/api/skill/promote": SkillPromoteResponse;
  "/api/skill/quarantine": SkillQuarantineResponse;
  "/api/skill/revert": SkillRevertResponse;
  "/api/skill/share": SkillShareResponse;
  "/api/skills": SkillListResponse;
  "/api/skills/hygiene": SkillHygieneResponse;
  "/api/spend/today": SpendTodayResponse;
  "/api/standing": StandingListResponse;
  "/api/standing/add": StandingAddResponse;
  "/api/standing/edit": StandingEditResponse;
  "/api/standing/enable": StandingSetEnabledResponse;
  "/api/standing/fire": StandingFireResponse;
  "/api/standing/remove": StandingRemoveResponse;
  "/api/standing/why": StandingWhyResponse;
  "/api/status": StatusResponse;
  "/api/suggestions": ChatSuggestionsResponse;
  "/api/tool_log": ToolLogResponse;
  "/api/toolbox/install": ToolboxInstallResponse;
  "/api/tools_catalog": ToolListResponse;
  "/api/version": VersionResponse;
  "/api/warden_log": WardenLogResponse;
  "/api/webhook_log": WebhookLogResponse;
  "/api/whatsappgw/qr": WhatsappgwQrResponse;
  "/api/whatsappgw/status": WhatsappgwStatusResponse;
  "/api/workflows": WorkflowListResponse;
  "/api/workflows/draft": WorkflowDraftResponse;
  "/api/workflows/enable": WorkflowSetEnabledResponse;
  "/api/workflows/refine": WorkflowRefineResponse;
  "/api/workflows/remove": WorkflowRemoveResponse;
  "/api/workflows/run": WorkflowRunResponse;
  "/api/workflows/runs": WorkflowRunsResponse;
  "/api/workflows/save": WorkflowSaveResponse;
  "/api/workflows/show": WorkflowShowResponse;
  "/api/workflows/templates": WorkflowTemplatesResponse;
  "/api/workflows/test_node": WorkflowTestNodeResponse;
  "/api/world": WorldListResponse;
  "/api/world/add": WorldAddResponse;
  "/api/world/edit": WorldEditResponse;
  "/api/world/forget": WorldForgetResponse;
  "/api/world/relate": WorldRelateResponse;
  "/api/world_log": WorldLogResponse;
}

/**
 * OperationRequestMap binds a path to the body the console accepts for
 * it. Only the paths that take a JSON body appear here.
 */
export interface OperationRequestMap {
  "/api/agents/add": AgentAddRequest;
  "/api/agents/capabilities": AgentCapabilitiesRequest;
  "/api/agents/edit": AgentEditRequest;
  "/api/agents/remove": AgentRemoveRequest;
  "/api/agents/repair": AgentRepairRequest;
  "/api/agents/resolve": AgentResolveRequest;
  "/api/agents/task": AgentTaskUpdateRequest;
  "/api/agents/wake": AgentWakeRequest;
  "/api/board/ack": BoardAckRequest;
  "/api/board/send": BoardSendRequest;
  "/api/catalog/sync": CatalogSyncRequest;
  "/api/chains/set": ChainsSetRequest;
  "/api/channel/account/remove": ChannelAccountRemoveRequest;
  "/api/channel/account/set": ChannelAccountSetRequest;
  "/api/channel/oauth/start": ChannelOauthStartRequest;
  "/api/channel/oauth/status": ChannelOauthStatusRequest;
  "/api/chat/summarize": ChatSummarizeRequest;
  "/api/config/schema/register": ConfigSchemaRegisterRequest;
  "/api/config/schema/unregister": ConfigSchemaUnregisterRequest;
  "/api/config/set": ConfigSetRequest;
  "/api/configcenter/access": ConfigcenterAccessRequest;
  "/api/configcenter/delete": ConfigcenterDeleteRequest;
  "/api/configcenter/rating": ConfigcenterSetRatingRequest;
  "/api/configcenter/set": ConfigcenterSetRequest;
  "/api/council/ask": CouncilAskRequest;
  "/api/council/set": CouncilSetRequest;
  "/api/data/insert": DataInsertRequest;
  "/api/data/update": DataUpdateRequest;
  "/api/market/source/add": MarketAddSourceRequest;
  "/api/market/source/remove": MarketRemoveSourceRequest;
  "/api/market/sync": MarketSyncRequest;
  "/api/mcp/add": McpAddRequest;
  "/api/memory/add": MemoryAddRequest;
  "/api/memory/bulk_forget": MemoryBulkForgetRequest;
  "/api/memory/supersede": MemorySupersedeRequest;
  "/api/persona/set": PersonaSetRequest;
  "/api/plan/generate": PlanGenerateRequest;
  "/api/plan/refine": PlanRefineRequest;
  "/api/prompts/set": PromptsSetRequest;
  "/api/provider/connect": ProviderConnectRequest;
  "/api/provider/keys/add": ProviderKeyAddRequest;
  "/api/provider/oauth/import": ProviderOauthImportRequest;
  "/api/provider/oauth/logout": Record<string, any>;
  "/api/provider/oauth/start": ProviderOauthStartRequest;
  "/api/provider/oauth/status": ProviderOauthStatusRequest;
  "/api/provider/probe": ProviderProbeRequest;
  "/api/provider/reload": Record<string, any>;
  "/api/redact/test": RedactTestRequest;
  "/api/research/ask": ResearchAskRequest;
  "/api/routing/set": RoutingSetRequest;
  "/api/schedule/add": ScheduleAddRequest;
  "/api/schedule/edit": ScheduleEditRequest;
  "/api/send": SendRequest;
  "/api/skill/import": SkillImportRequest;
  "/api/standing/add": StandingAddRequest;
  "/api/standing/edit": StandingEditRequest;
  "/api/whatsappgw/qr": WhatsappgwQrRequest;
  "/api/whatsappgw/status": WhatsappgwStatusRequest;
  "/api/workflows/draft": WorkflowDraftRequest;
  "/api/workflows/refine": WorkflowRefineRequest;
  "/api/workflows/run": WorkflowRunRequest;
  "/api/workflows/save": WorkflowSaveRequest;
  "/api/workflows/test_node": WorkflowTestNodeRequest;
  "/api/world/add": WorldAddRequest;
  "/api/world/edit": WorldEditRequest;
}

/**
 * NativeResponseMap binds a console-native path to the shape the console
 * answers it with. The shape is DERIVED from the row the route names in
 * the contract, so the browser cannot spell a field the console does not
 * serve. A native path that names no row has no shape the browser can
 * name.
 */
export interface NativeResponseMap {
  "/api/authmeta": AuthmetaResponse;
  "/api/login": LoginResponse;
  "/api/logout": LogoutResponse;
  "/api/rollback/checkpoints": RollbackCheckpointsResponse;
  "/api/rollback/apply": RollbackApplyResponse;
  "/api/sse-token": SseTokenResponse;
  "/api/voice/status": VoiceStatusResponse;
  "/api/files/tree": FilesTreeResponse;
  "/hooks/": WorkflowHookResponse;
}

/**
 * ResponseOf is the shape the console answers a path with. A path that binds
 * no operation has no shape the browser can name, so it resolves to any —
 * unless it is a console-native path, whose row the contract names.
 */
export type ResponseOf<P extends string> = P extends keyof OperationResponseMap
  ? OperationResponseMap[P]
  : P extends keyof NativeResponseMap
  ? NativeResponseMap[P]
  : any;

/**
 * RequestOf is the body the console accepts for a path. A path that takes no
 * JSON body has no body the browser can name, so it resolves to any.
 */
export type RequestOf<P extends string> = P extends keyof OperationRequestMap
  ? OperationRequestMap[P]
  : any;
