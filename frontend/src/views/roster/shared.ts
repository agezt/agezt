import { fmtDue } from "@/lib/utils";
import type { AgentRuntimeStatus } from "@/lib/agentdetail";

export interface AgentProfile {
  id: string;
  slug: string;
  name?: string;
  soul?: string;
  instructions?: string[];
  model?: string;
  fallbacks?: string[];
  task_type?: string;
  max_cost_mc?: number;
  max_daily_mc?: number;
  memory_scope?: string;
  workdir?: string;
  owner_agent?: string;
  parent_agent?: string;
  direct_callable?: boolean;
  retry_policy?: { max_attempts?: number; backoff?: string; base_delay_sec?: number; max_delay_sec?: number; retry_on?: string[] };
  health_policy?: { stale_after_sec?: number; failure_window?: number; failure_threshold?: number; doctor_agent?: string };
  self_repair?: { enabled?: boolean; max_attempts?: number; escalate_to?: string };
  noise_policy?: {
    silent_on_success?: boolean;
    disable_memory_writes?: boolean;
    min_notify_severity?: "info" | "warning" | "critical";
    min_notify_interval_sec?: number;
  };
  tool_allow?: string[];
  tool_deny?: string[];
  trust_ceiling?: "L0" | "L1" | "L2" | "L3" | "L4";
  execution_profile?: string;
  config_overrides?: Record<string, string>;
  status?: AgentRuntimeStatus;
  lifecycle?: AgentLifecycle;
  tasklist?: AgentTask[];
  description?: string;
  kind?: "system" | "custom" | "subagent";
  managed?: boolean;
  enabled?: boolean;
  retired?: boolean;
  retired_ms?: number;
  retired_reason?: string;
  system?: boolean; // shipped internal guardian (M961) — protected from removal
}

export interface AgentLifecycle {
  mode?: "persistent" | "cycle" | "retire_on_complete";
  retire_on_complete?: boolean;
  max_cycles?: number;
  completed_cycles?: number;
}

export interface AgentTask {
  id?: string;
  title: string;
  description?: string;
  scope?: "cycle" | "total";
  status?: "todo" | "doing" | "done" | "blocked" | "retired";
}

export interface RosterBoardMessage {
  id?: string;
  from?: string;
  to?: string;
  reply_to?: string;
  acked_by?: string[];
  ts_unix_ms?: number;
}

export interface RosterSchedule {
  id: string;
  intent?: string;
  agent?: string;
  target?: string;
  mode?: string;
  interval_sec?: number;
  enabled?: boolean;
  frequency_warning?: string;
}

export interface AgentRetireResult {
  standing_paused?: number;
  schedules_paused?: number;
}

export type AgentReviveResult = AgentRetireResult;
export type AgentEnableResult = AgentRetireResult;

export interface AgentRemoveResult {
  removed?: boolean;
  standing_removed?: number;
  schedules_removed?: number;
  memories_forgotten?: number;
  authored_memories_forgotten?: number;
  skills_archived?: number;
  configs_deleted?: number;
  configs_access_pruned?: number;
  workspaces_deleted?: number;
  subagents_retired?: number;
  subagents_retired_slugs?: string[];
  mailbox_messages_retained?: number;
  workflow_refs_retained?: number;
  subagent_workflow_refs_retained?: number;
}

export function agentRetireToast(slug: string, res?: AgentRetireResult): string {
  const standing = res?.standing_paused || 0;
  const schedules = res?.schedules_paused || 0;
  if (standing + schedules === 0) return `${slug} retired to the graveyard`;
  return `${slug} retired to the graveyard (${standing} standing, ${schedules} schedule paused)`;
}

export function agentReviveToast(slug: string, res?: AgentReviveResult): string {
  const standing = res?.standing_paused || 0;
  const schedules = res?.schedules_paused || 0;
  if (standing + schedules === 0) return `${slug} revived (paused)`;
  return `${slug} revived (paused; ${standing} standing, ${schedules} schedule still paused)`;
}

export function agentEnableToast(slug: string, enabled: boolean, res?: AgentEnableResult): string {
  if (!enabled) return `${slug} paused`;
  const standing = res?.standing_paused || 0;
  const schedules = res?.schedules_paused || 0;
  if (standing + schedules === 0) return `${slug} resumed`;
  return `${slug} resumed (${standing} standing, ${schedules} schedule still paused)`;
}

export function agentRemoveToast(slug: string, res: AgentRemoveResult): string {
  if (!res.removed) return `${slug} was not removed`;
  const retiredSlugs = (res.subagents_retired_slugs || []).filter(Boolean);
  const retiredList = retiredSlugs.length > 0
    ? `; retired: ${retiredSlugs.slice(0, 3).join(", ")}${retiredSlugs.length > 3 ? ` +${retiredSlugs.length - 3}` : ""}`
    : "";
  const configAccess = res.configs_access_pruned ? `, ${res.configs_access_pruned} shared config access pruned` : "";
  const workflowRefs = (res.workflow_refs_retained || res.subagent_workflow_refs_retained)
    ? `, ${res.workflow_refs_retained || 0} workflow ref retained, ${res.subagent_workflow_refs_retained || 0} subagent workflow ref retained`
    : "";
  return `${slug} removed (identity deleted; audit retained; ${res.standing_removed || 0} standing, ${res.schedules_removed || 0} schedule, ${res.memories_forgotten || 0} private memory, ${res.authored_memories_forgotten || 0} authored shared memory, ${res.skills_archived || 0} skill, ${res.configs_deleted || 0} config${configAccess}, ${res.workspaces_deleted || 0} workspace, ${res.subagents_retired || 0} subagent, ${res.mailbox_messages_retained || 0} mailbox/audit retained${workflowRefs}${retiredList})`;
}

export function trustRank(level?: string): number {
  const normalized = (level || "L4").trim().toUpperCase();
  const match = /^L([0-4])$/.exec(normalized);
  return match ? Number(match[1]) : 4;
}


// slugOk mirrors the kernel's roster slug rule (lowercase, digit/letter first,
// then letters/digits/dot/dash/underscore, ≤64) so the form can validate before
// the round-trip. Pure + unit-tested.
export function slugOk(s: string): boolean {
  return /^[a-z0-9][a-z0-9._-]{0,63}$/.test(s);
}

// usdToMc converts a dollar string ("0.50", "$0.50") to USD-microcents
// ($1 = 1e9, the kernel's budget unit). Returns null for blank, NaN, or
// negative input. Pure + unit-tested.
export function usdToMc(s: string): number | null {
  const t = s.trim().replace(/^\$/, "");
  if (t === "") return 0;
  const v = Number(t);
  if (!Number.isFinite(v) || v < 0) return null;
  return Math.round(v * 1e9);
}

export function agentIdentityKind(profile: Pick<AgentProfile, "system" | "kind" | "managed" | "direct_callable">): "system" | "custom" | "subagent" {
  if (profile.system || profile.kind === "system") return "system";
  if (profile.kind === "subagent" || profile.managed || profile.direct_callable === false) return "subagent";
  return "custom";
}

export function formatWakeDue(ms: number, now = Date.now()): string {
  return fmtDue(ms, now);
}
