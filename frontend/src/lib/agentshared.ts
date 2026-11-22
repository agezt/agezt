import { fmtDue } from "@/app/utils";
import type { AgentRuntimeStatus } from "@/lib/agentdetail";
import type {
  AgentListResponseProfilesItem,
  AgentListResponseProfilesItemLifecycle,
  AgentListResponseProfilesItemTasklistItem,
} from "@/app/api/types.generated";

// AgentProfile is the browser's view of one profile from /api/agents. Every
// field's TYPE comes from the daemon's registry: the browser decides which
// fields it reads, the daemon decides what they are. A field the daemon
// renamed breaks this file at compile time, which is the whole point.
export interface AgentProfile {
  id?: AgentListResponseProfilesItem["id"];
  slug: AgentListResponseProfilesItem["slug"];
  name?: AgentListResponseProfilesItem["name"];
  soul?: AgentListResponseProfilesItem["soul"];
  instructions?: AgentListResponseProfilesItem["instructions"];
  model?: AgentListResponseProfilesItem["model"];
  fallbacks?: AgentListResponseProfilesItem["fallbacks"];
  task_type?: AgentListResponseProfilesItem["task_type"];
  max_cost_mc?: AgentListResponseProfilesItem["max_cost_mc"];
  max_daily_mc?: AgentListResponseProfilesItem["max_daily_mc"];
  memory_scope?: AgentListResponseProfilesItem["memory_scope"];
  workdir?: AgentListResponseProfilesItem["workdir"];
  owner_agent?: AgentListResponseProfilesItem["owner_agent"];
  parent_agent?: AgentListResponseProfilesItem["parent_agent"];
  direct_callable?: AgentListResponseProfilesItem["direct_callable"];
  retry_policy?: AgentListResponseProfilesItem["retry_policy"];
  health_policy?: AgentListResponseProfilesItem["health_policy"];
  self_repair?: AgentListResponseProfilesItem["self_repair"];
  noise_policy?: AgentListResponseProfilesItem["noise_policy"];
  tool_allow?: AgentListResponseProfilesItem["tool_allow"];
  tool_deny?: AgentListResponseProfilesItem["tool_deny"];
  trust_ceiling?: AgentListResponseProfilesItem["trust_ceiling"];
  execution_profile?: AgentListResponseProfilesItem["execution_profile"];
  config_overrides?: AgentListResponseProfilesItem["config_overrides"];
  status?: AgentRuntimeStatus;
  lifecycle?: AgentLifecycle;
  tasklist?: AgentTask[];
  description?: AgentListResponseProfilesItem["description"];
  kind?: AgentListResponseProfilesItem["kind"];
  managed?: AgentListResponseProfilesItem["managed"];
  enabled?: AgentListResponseProfilesItem["enabled"];
  retired?: AgentListResponseProfilesItem["retired"];
  retired_ms?: AgentListResponseProfilesItem["retired_ms"];
  retired_reason?: AgentListResponseProfilesItem["retired_reason"];
  system?: AgentListResponseProfilesItem["system"]; // shipped internal guardian (M961) — protected from removal
}

export interface AgentLifecycle {
  mode?: AgentListResponseProfilesItemLifecycle["mode"];
  retire_on_complete?: AgentListResponseProfilesItemLifecycle["retire_on_complete"];
  max_cycles?: AgentListResponseProfilesItemLifecycle["max_cycles"];
  completed_cycles?: AgentListResponseProfilesItemLifecycle["completed_cycles"];
}

export interface AgentTask {
  id?: AgentListResponseProfilesItemTasklistItem["id"];
  title: AgentListResponseProfilesItemTasklistItem["title"];
  description?: AgentListResponseProfilesItemTasklistItem["description"];
  scope?: AgentListResponseProfilesItemTasklistItem["scope"];
  status?: AgentListResponseProfilesItemTasklistItem["status"];
}

export type RosterBoardMessage = {
  id?: string;
  from?: string;
  to?: string;
  reply_to?: string;
  acked_by?: string[];
  ts_unix_ms?: number;
};

export type RosterSchedule = {
  id: string;
  intent?: string;
  agent?: string;
  target?: string;
  mode?: string;
  interval_sec?: number;
  enabled?: boolean;
  frequency_warning?: string;
};

export type AgentRetireResult = {
  standing_paused?: number;
  schedules_paused?: number;
};

export type AgentReviveResult = AgentRetireResult;
export type AgentEnableResult = AgentRetireResult;

export type AgentRemoveResult = {
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
};

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
