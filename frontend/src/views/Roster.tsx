import { useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { Users, RefreshCw, Pause, Play, Trash2, Plus, X, Pencil, Bot, Archive, ArchiveRestore, Skull, Activity, Sparkles, IdCard, ShieldCheck, Zap, Wrench, Megaphone, ListTree, Mail, CalendarClock } from "lucide-react";
import { getJSON, postAction, postJSON } from "@/lib/api";
import { openAgent } from "@/lib/agentnav";
import { openIncident } from "@/lib/incidentnav";
import { cn, fmtDateTime, fmtDue } from "@/lib/utils";
import { money } from "@/lib/format";
import { Button } from "@/components/ui/button";
import { Advanced } from "@/components/ui/disclosure";
import { useUI, type ConfirmOptions } from "@/components/ui/feedback";
import { SkeletonList } from "@/components/ui/skeleton";
import { EmptyState } from "@/components/ui/empty";
import { Badge } from "@/components/ui/badge";
import { PageHeader } from "@/components/ui/page-header";
import { ErrorText } from "@/components/JsonView";
import { AgentAvatar } from "@/components/AgentAvatar";
import { AgentActivity } from "@/components/AgentActivity";
import { ModelPicker } from "@/components/ModelPicker";
import { isChainRef } from "@/lib/chains";
import { highImpactToolNames, scheduleAgentSlug, type ApiOrder } from "@/lib/fleet";
import { summarizeConfigOverrides, summarizeAgentRuntimeStatus, type AgentCardRuntimeSummary, type AgentRuntimeStatus } from "@/lib/agentdetail";
import { useEvents } from "@/lib/events";
import { applyAgentLivePatches, reduceAgentLivePatchMap, shouldReloadAgentCatalog, type AgentLivePatchMap } from "@/lib/agentlive";

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

export function agentNoisePolicySummary(profile: Pick<AgentProfile, "noise_policy">): string {
  const policy = profile.noise_policy;
  if (!policy) return "";
  const parts: string[] = [];
  if (policy.silent_on_success) parts.push("silent on success");
  if (policy.disable_memory_writes) parts.push("no memory writes");
  if (policy.min_notify_severity) parts.push(`notify >= ${policy.min_notify_severity}`);
  if (policy.min_notify_interval_sec) parts.push(`cooldown ${policy.min_notify_interval_sec}s`);
  return parts.join(" · ");
}

const SYSTEM_GUARDIAN_RUN_CAP_MC = 50_000_000;
const SYSTEM_GUARDIAN_DAILY_CAP_MC = 50_000_000;

function trustRank(level?: string): number {
  const normalized = (level || "L4").trim().toUpperCase();
  const match = /^L([0-4])$/.exec(normalized);
  return match ? Number(match[1]) : 4;
}

function clampSystemGuardianCap(value: number | undefined, cap: number): number {
  if (!value || value <= 0) return cap;
  return Math.min(value, cap);
}

export function systemGuardianSafetySummary(profile: Pick<AgentProfile, "system" | "noise_policy" | "tool_deny" | "max_daily_mc" | "max_cost_mc" | "memory_scope" | "trust_ceiling"> & { slug?: string }): string {
  if (!profile.system) return "";
  const issues = systemGuardianSafetyIssues(profile);
  if (issues.length > 0) return `review: ${issues.join(", ")}`;
  return "quiet, memory off, notify >= warning, cooldown >=8h, capped, trust <= L2";
}

export function systemGuardianSafetyIssues(profile: Pick<AgentProfile, "system" | "noise_policy" | "tool_deny" | "max_daily_mc" | "max_cost_mc" | "memory_scope" | "trust_ceiling"> & { slug?: string }): string[] {
  if (!profile.system) return [];
  const issues: string[] = [];
  const policy = profile.noise_policy;
  const memoryDenied = (profile.tool_deny || []).some((tool) => tool.trim().toLowerCase() === "memory");
  const memoryWritesOff = !!policy?.disable_memory_writes || memoryDenied;
  const memoryScope = (profile.memory_scope || "").trim();
  const expectedScope = profile.slug ? `system/${profile.slug}` : "";
  if (!policy) {
    issues.push("no noise policy");
  }
  const minNotifySeverity = policy?.min_notify_severity || "";
  const notifyCooldownSec = policy?.min_notify_interval_sec || 0;
  if (policy && !policy.silent_on_success) issues.push("success notifications enabled");
  if (!memoryWritesOff) issues.push("memory writes enabled");
  if (notifySeverityRank(minNotifySeverity) < notifySeverityRank("warning")) issues.push("notify below warning");
  if (notifyCooldownSec < 8 * 3600) issues.push("notify cooldown <8h");
  if ((profile.max_daily_mc || 0) <= 0) issues.push("no daily cap");
  if ((profile.max_daily_mc || 0) > SYSTEM_GUARDIAN_DAILY_CAP_MC) issues.push("daily cap too high");
  if ((profile.max_cost_mc || 0) <= 0) issues.push("no run cap");
  if ((profile.max_cost_mc || 0) > SYSTEM_GUARDIAN_RUN_CAP_MC) issues.push("run cap too high");
  if (trustRank(profile.trust_ceiling) > trustRank("L2")) issues.push("trust above L2");
  if (expectedScope && memoryScope !== expectedScope) issues.push("memory scope not isolated");
  return issues;
}

export function systemGuardianNoiseContract(
  profile: Pick<AgentProfile, "system" | "noise_policy" | "tool_deny" | "max_daily_mc" | "max_cost_mc" | "memory_scope" | "trust_ceiling"> & { slug?: string },
  schedule: Pick<ReturnType<typeof agentSchedulePressurePassport>, "frequent" | "active" | "detail"> = { frequent: 0, active: 0, detail: "no schedules" },
): { label: string; detail: string; tone: "good" | "warn" | "muted"; issues: string[] } {
  if (!profile.system) {
    return {
      label: "custom noise",
      detail: agentNoisePolicySummary(profile) || "default notification and memory policy",
      tone: "muted",
      issues: [],
    };
  }
  const issues = systemGuardianSafetyIssues(profile);
  const frequent = schedule.frequent || 0;
  const scheduleIssue = frequent > 0 ? `${frequent}/${schedule.active || frequent} frequent schedule${frequent === 1 ? "" : "s"}` : "";
  const allIssues = [...issues, scheduleIssue].filter(Boolean);
  if (allIssues.length > 0) {
    return {
      label: "guardian noise review",
      detail: `${allIssues.join(", ")} · quiet action disables memory writes, raises notify threshold/cooldown, caps spend, lowers trust, and pauses frequent schedules`,
      tone: "warn",
      issues: allIssues,
    };
  }
  return {
    label: "guardian quiet contract",
    detail: `memory off · notify >= warning · cooldown >=8h · capped · trust <= L2 · ${schedule.detail}`,
    tone: "good",
    issues: [],
  };
}

export function systemGuardianRiskSummary(profiles: (Pick<AgentProfile, "system" | "retired" | "noise_policy" | "tool_deny" | "max_daily_mc" | "max_cost_mc" | "memory_scope" | "trust_ceiling"> & { slug?: string })[]): string {
  const guardians = profiles.filter((p) => p.system && !p.retired);
  if (guardians.length === 0) return "";
  const review = guardians.filter((p) => systemGuardianSafetySummary(p).startsWith("review:")).length;
  if (review > 0) return `${review}/${guardians.length} guardian review`;
  return `${guardians.length} guardian quiet`;
}

export function agentNoiseBudgetPassport(
  profile: Pick<AgentProfile, "system" | "noise_policy" | "tool_deny" | "max_daily_mc" | "max_cost_mc" | "memory_scope" | "status" | "trust_ceiling"> & { slug?: string },
): { detail: string; tone: "good" | "warn" | "muted" } {
  const wakeSchedules = profile.status?.wake_schedule_count || 0;
  if (profile.system) {
    const safety = systemGuardianSafetySummary(profile);
    if (safety.startsWith("review:")) {
      const issues = safety.replace(/^review:\s*/, "");
      return {
        detail: `noise review: ${issues}${wakeSchedules > 1 ? ` · ${wakeSchedules} scheduled wakes` : ""}`,
        tone: "warn",
      };
    }
    return {
      detail: `quiet budget · memory off · notify >= warning · ${wakeSchedules} schedule${wakeSchedules === 1 ? "" : "s"}`,
      tone: "good",
    };
  }
  const policy = agentNoisePolicySummary(profile);
  if (policy) {
    return {
      detail: `${policy}${wakeSchedules > 0 ? ` · ${wakeSchedules} schedule${wakeSchedules === 1 ? "" : "s"}` : ""}`,
      tone: "good",
    };
  }
  return {
    detail: wakeSchedules > 0 ? `default noise · ${wakeSchedules} schedule${wakeSchedules === 1 ? "" : "s"}` : "default noise",
    tone: "muted",
  };
}

export function agentSchedulePressurePassport(
  profile: Pick<AgentProfile, "slug" | "system" | "kind">,
  schedules: RosterSchedule[],
): { detail: string; tone: "good" | "warn" | "muted"; active: number; frequent: number; frequentIds: string[] } {
  const bound = schedules.filter((s) => scheduleTargetsAgent(s, profile.slug));
  const active = bound.filter((s) => s.enabled !== false);
  const threshold = profile.system || profile.kind === "system" ? 8 * 3600 : 15 * 60;
  const intervalSchedules = active
    .map((s) => ({ schedule: s, sec: s.interval_sec || 0 }))
    .filter((row) => row.sec > 0 && (!row.schedule.mode || ["interval", "window", "continuous"].includes(row.schedule.mode)));
  const frequent = intervalSchedules.filter((row) => row.sec < threshold || !!row.schedule.frequency_warning);
  const fastest = intervalSchedules.reduce<number | null>((min, row) => (min == null || row.sec < min ? row.sec : min), null);
  if (active.length === 0) {
    return { detail: "no schedules", tone: "muted", active: 0, frequent: 0, frequentIds: [] };
  }
  const activeLabel = `${active.length} schedule${active.length === 1 ? "" : "s"}`;
  const fastestLabel = fastest ? `fastest ${scheduleIntervalLabel(fastest)}` : "no interval cadence";
  if (frequent.length > 0) {
    return {
      detail: `schedule pressure: ${frequent.length}/${active.length} frequent · ${fastestLabel}`,
      tone: "warn",
      active: active.length,
      frequent: frequent.length,
      frequentIds: frequent.map((row) => row.schedule.id),
    };
  }
  return {
    detail: `${activeLabel} · ${fastestLabel}`,
    tone: "good",
    active: active.length,
    frequent: 0,
    frequentIds: [],
  };
}

function scheduleTargetsAgent(s: RosterSchedule, slug: string): boolean {
  if (s.agent === slug) return true;
  return !s.target && scheduleAgentSlug(s.intent) === slug;
}

function scheduleIntervalLabel(sec: number): string {
  if (sec % 86400 === 0) return `${sec / 86400}d`;
  if (sec % 3600 === 0) return `${sec / 3600}h`;
  if (sec % 60 === 0) return `${sec / 60}m`;
  return `${sec}s`;
}

export function noisySystemGuardians(profiles: AgentProfile[]): AgentProfile[] {
  return profiles.filter((p) => !!p.system && !p.retired && systemGuardianSafetySummary(p).startsWith("review:"));
}

export function guardianQuietPolicyPayload(profile: AgentProfile): Record<string, unknown> {
  const policy = profile.noise_policy || {};
  const runCap = clampSystemGuardianCap(profile.max_cost_mc, SYSTEM_GUARDIAN_RUN_CAP_MC);
  const dailyCap = clampSystemGuardianCap(profile.max_daily_mc, SYSTEM_GUARDIAN_DAILY_CAP_MC);
  const toolAllow = uniqueLowerTools(profile.tool_allow || []).filter((tool) => tool !== "memory");
  const toolDeny = uniqueLowerTools([...(profile.tool_deny || []), "memory"]);
  return {
    ref: profile.slug,
    memory_scope: profile.memory_scope?.startsWith("system/") ? profile.memory_scope : `system/${profile.slug}`,
    max_cost_mc: runCap,
    max_daily_mc: dailyCap,
    trust_ceiling: trustRank(profile.trust_ceiling) > trustRank("L2") ? "L2" : profile.trust_ceiling || "L2",
    tool_allow: toolAllow,
    tool_deny: toolDeny,
    noise_policy: {
      silent_on_success: true,
      disable_memory_writes: true,
      min_notify_severity: notifySeverityRank(policy.min_notify_severity) < notifySeverityRank("warning")
        ? "warning"
        : policy.min_notify_severity || "warning",
      min_notify_interval_sec: Math.max(policy.min_notify_interval_sec || 0, 8 * 3600),
    },
    config_overrides: profile.config_overrides || {},
  };
}

function uniqueLowerTools(tools: string[]): string[] {
  const seen = new Set<string>();
  const out: string[] = [];
  for (const raw of tools) {
    const tool = raw.trim().toLowerCase();
    if (!tool || seen.has(tool)) continue;
    seen.add(tool);
    out.push(tool);
  }
  return out;
}

type GuardianSchedulePressure = Record<string, Pick<ReturnType<typeof agentSchedulePressurePassport>, "frequent" | "frequentIds"> | undefined>;

export function guardianQuietingSummary(
  profiles: AgentProfile[],
  schedulePressure: GuardianSchedulePressure = {},
): { label: string; detail: string; tone: "good" | "warn" | "muted"; quietTargets: number; frequentSchedules: number } {
  const noisy = noisySystemGuardians(profiles);
  const activeGuardians = profiles.filter((p) => p.system && !p.retired);
  const active = activeGuardians.length;
  const frequentIds = new Set<string>();
  for (const profile of activeGuardians) {
    for (const id of schedulePressure[profile.slug]?.frequentIds || []) {
      if (id) frequentIds.add(id);
    }
  }
  const frequentSchedules = frequentIds.size || activeGuardians.reduce((sum, p) => sum + (schedulePressure[p.slug]?.frequent || 0), 0);
  if (active === 0) {
    return {
      label: "no active guardians",
      detail: "system guardians are not installed or are retired",
      tone: "muted",
      quietTargets: 0,
      frequentSchedules: 0,
    };
  }
  if (noisy.length === 0 && frequentSchedules === 0) {
    return {
      label: "guardians quiet",
      detail: `${active} active guardian${active === 1 ? "" : "s"} · memory off · notify >= warning · cooldown >=8h`,
      tone: "good",
      quietTargets: 0,
      frequentSchedules: 0,
    };
  }
  if (noisy.length === 0) {
    return {
      label: "guardian schedule pressure",
      detail: `${active} active guardian${active === 1 ? "" : "s"} quiet · ${frequentSchedules} frequent guardian schedule${frequentSchedules === 1 ? "" : "s"} will be paused`,
      tone: "warn",
      quietTargets: 0,
      frequentSchedules,
    };
  }
  const scheduleDetail = frequentSchedules > 0
    ? ` · ${frequentSchedules} frequent guardian schedule${frequentSchedules === 1 ? "" : "s"} will be paused`
    : "";
  return {
    label: `${noisy.length}/${active} guardian${active === 1 ? "" : "s"} need quieting`,
    detail: `quiet action enforces memory off, isolated system memory scope, notify >= warning, cooldown >=8h, run/day caps, trust <= L2${scheduleDetail}`,
    tone: "warn",
    quietTargets: noisy.length,
    frequentSchedules,
  };
}

function notifySeverityRank(severity?: string): number {
  switch ((severity || "").trim().toLowerCase()) {
    case "critical":
      return 3;
    case "warning":
    case "warn":
      return 2;
    case "info":
      return 1;
    default:
      return 0;
  }
}

export function agentLifecycleSummary(profile: Pick<AgentProfile, "lifecycle">): string {
  const lifecycle = profile.lifecycle;
  const mode = lifecycle?.mode || (lifecycle?.retire_on_complete ? "retire_on_complete" : "persistent");
  const completed = lifecycle?.completed_cycles || 0;
  const max = lifecycle?.max_cycles || 0;
  if (mode === "cycle") {
    if (max > 0) return `cycle ${completed}/${max}`;
    if (completed > 0) return `cycle ${completed} done`;
    return "cycle";
  }
  if (mode === "retire_on_complete") return "one-shot";
  return "persistent";
}

export function agentGraveyardSummary(profile: Pick<AgentProfile, "retired" | "retired_ms">): string {
  if (!profile.retired) return "";
  return profile.retired_ms ? `graveyard since ${fmtDateTime(profile.retired_ms)}` : "graveyard";
}

export function agentLifecycleDispositionPassport(
  profile: Pick<AgentProfile, "retired" | "retired_ms" | "retired_reason" | "lifecycle">,
): { value: string; detail: string; tone: "good" | "warn" | "muted" } {
  if (profile.retired) {
    const since = profile.retired_ms ? ` since ${fmtDateTime(profile.retired_ms)}` : "";
    const reason = profile.retired_reason?.trim();
    return {
      value: "graveyard · revive/remove",
      detail: `retired${since}; soul, logs, memory, skills, config, and workspace remain inspectable until remove${reason ? `; reason: ${reason}` : ""}`,
      tone: "muted",
    };
  }
  const lifecycle = profile.lifecycle;
  const mode = lifecycle?.mode || (lifecycle?.retire_on_complete ? "retire_on_complete" : "persistent");
  if (mode === "retire_on_complete") {
    return {
      value: "one-shot · retires on completion",
      detail: "this identity is expected to finish its assigned total task contract, then move to the graveyard",
      tone: "warn",
    };
  }
  if (mode === "cycle" || (lifecycle?.max_cycles || 0) > 0) {
    const done = lifecycle?.completed_cycles || 0;
    const max = lifecycle?.max_cycles || 0;
    return {
      value: max > 0 ? `cycle · ${done}/${max}` : done > 0 ? `cycle · ${done} done` : "cycle · unlimited",
      detail: max > 0
        ? "this identity wakes repeatedly and retires when the configured cycle count is reached"
        : "this identity wakes repeatedly until an operator, doctor, or task rule retires it",
      tone: "good",
    };
  }
  return {
    value: "persistent · alive",
    detail: "this identity stays available across runs until an operator, doctor, or lifecycle rule retires it",
    tone: "good",
  };
}

export function agentGraveyardStats(profiles: AgentProfile[]): {
  count: number;
  custom: number;
  system: number;
  subagents: number;
  withReason: number;
  oldest?: AgentProfile;
} {
  const retired = profiles.filter((p) => p.retired);
  const sorted = [...retired].sort((a, b) => {
    const am = a.retired_ms || Number.MAX_SAFE_INTEGER;
    const bm = b.retired_ms || Number.MAX_SAFE_INTEGER;
    if (am !== bm) return am - bm;
    return a.slug.localeCompare(b.slug);
  });
  return {
    count: retired.length,
    custom: retired.filter((p) => agentIdentityKind(p) === "custom").length,
    system: retired.filter((p) => agentIdentityKind(p) === "system").length,
    subagents: retired.filter((p) => agentIdentityKind(p) === "subagent").length,
    withReason: retired.filter((p) => !!p.retired_reason?.trim()).length,
    oldest: sorted[0],
  };
}

export function agentGraveyardCleanupPassport(
  profiles: Pick<AgentProfile, "retired" | "system" | "retired_reason">[],
): { label: string; detail: string; tone: "good" | "warn" | "muted" } {
  const retired = profiles.filter((p) => p.retired);
  if (retired.length === 0) {
    return {
      label: "graveyard empty",
      detail: "no retired identities are waiting for revive or removal",
      tone: "muted",
    };
  }
  const protectedCount = retired.filter((p) => p.system).length;
  const removable = retired.length - protectedCount;
  const missingReason = retired.filter((p) => !p.retired_reason?.trim()).length;
  const label = removable > 0
    ? `${removable} removable`
    : protectedCount > 0
      ? "system protected"
      : "review only";
  const detail = [
    `${retired.length} retired`,
    removable > 0 ? `${removable} can be removed` : "",
    protectedCount > 0 ? `${protectedCount} system protected` : "",
    missingReason > 0 ? `${missingReason} without reason` : "all have reasons",
  ].filter(Boolean).join(" · ");
  return {
    label,
    detail,
    tone: removable > 0 || missingReason > 0 ? "warn" : "good",
  };
}

export function agentTaskProgressSummary(tasks?: AgentTask[]): string {
  const active = (tasks || []).filter((t) => t.status !== "retired");
  if (active.length === 0) return "";
  const cycle = active.filter((t) => (t.scope || "total") === "cycle").length;
  const total = active.filter((t) => (t.scope || "total") === "total").length;
  const doing = active.filter((t) => t.status === "doing").length;
  const blocked = active.filter((t) => t.status === "blocked").length;
  const done = active.filter((t) => t.status === "done").length;
  const parts = [`${cycle} cycle`, `${total} total`];
  if (doing > 0) parts.push(`${doing} doing`);
  if (blocked > 0) parts.push(`${blocked} blocked`);
  if (done > 0) parts.push(`${done} done`);
  return parts.join(" / ");
}

export function agentTaskContractSummary(profile: Pick<AgentProfile, "lifecycle" | "tasklist" | "retired">): string {
  if (profile.retired) return "graveyard · will not wake until revived";
  const lifecycle = profile.lifecycle;
  const max = lifecycle?.max_cycles || 0;
  const rawMode = lifecycle?.mode || (lifecycle?.retire_on_complete ? "retire_on_complete" : "persistent");
  const mode = rawMode === "persistent" && max > 0 ? "cycle" : rawMode;
  const active = (profile.tasklist || []).filter((t) => t.status !== "retired");
  const cycle = active.filter((t) => (t.scope || "total") === "cycle").length;
  const total = active.filter((t) => (t.scope || "total") === "total").length;
  const doneTotal = active.filter((t) => (t.scope || "total") === "total" && t.status === "done").length;
  const blocked = active.filter((t) => t.status === "blocked").length;
  const taskPart = active.length > 0 ? `${cycle} cycle / ${total} total tasks` : "no durable tasks";
  const blockedPart = blocked > 0 ? ` · ${blocked} blocked` : "";

  if (mode === "retire_on_complete") {
    const progress = total > 0 ? ` · ${doneTotal}/${total} total done` : "";
    return `one-shot agent · retires on completion · ${taskPart}${progress}${blockedPart}`;
  }
  if (mode === "cycle") {
    const completed = lifecycle?.completed_cycles || 0;
    const cyclePart = max > 0 ? ` · ${completed}/${max} cycles` : completed > 0 ? ` · ${completed} cycles done` : "";
    const retirePart = max > 0 ? " · retires at max cycles" : "";
    return `cycle agent · repeats on each wake${cyclePart}${retirePart} · ${taskPart}${blockedPart}`;
  }
  return `persistent agent · stays alive after runs · ${taskPart}${blockedPart}`;
}

export function agentHierarchySummary(profile: Pick<AgentProfile, "kind" | "direct_callable" | "managed" | "owner_agent" | "parent_agent">): string {
  const owner = profile.owner_agent?.trim();
  const parent = profile.parent_agent?.trim();
  if (profile.kind === "subagent" || profile.managed || profile.direct_callable === false) {
    const manager = parent || owner;
    return manager ? `managed by ${manager}` : "managed sub-agent";
  }
  if (parent) return `direct · parent ${parent}`;
  if (owner) return `direct · owner ${owner}`;
  return "direct";
}

export function agentHierarchyTreePassport(
  profile: Pick<AgentProfile, "slug" | "kind" | "direct_callable" | "managed" | "owner_agent" | "parent_agent">,
  profiles: Pick<AgentProfile, "slug" | "owner_agent" | "parent_agent" | "retired">[] = [],
): { value: string; detail: string; tone: "good" | "warn" | "bad" | "muted" } {
  const slug = profile.slug?.trim();
  const owner = profile.owner_agent?.trim();
  const parent = profile.parent_agent?.trim();
  const manager = parent || owner;
  const managed = profile.kind === "subagent" || profile.managed || profile.direct_callable === false;
  const slugKey = slug.toLowerCase();
  const children = profiles.filter((candidate) => {
    if (!slugKey || candidate.slug === profile.slug) return false;
    const candidateOwner = candidate.owner_agent?.trim().toLowerCase();
    const candidateParent = candidate.parent_agent?.trim().toLowerCase();
    return candidateOwner === slugKey || candidateParent === slugKey;
  });
  const activeChildren = children.filter((candidate) => !candidate.retired);
  const retiredChildren = children.length - activeChildren.length;
  const childPart = activeChildren.length > 0
    ? `${activeChildren.length} ${plural(activeChildren.length, "dependent", "dependents")}`
    : "no dependents";
  const lineagePart = managed
    ? manager
      ? `parent/owner ${manager}`
      : "manager missing"
    : parent
      ? `parent ${parent}`
      : owner
        ? `owner ${owner}`
        : "root callable";
  const childNames = activeChildren.slice(0, 3).map((candidate) => candidate.slug).join(", ");
  const childDetail = activeChildren.length > 0
    ? `children: ${childNames}${activeChildren.length > 3 ? ` +${activeChildren.length - 3}` : ""}`
    : "no active children";
  const retiredDetail = retiredChildren > 0 ? ` · ${retiredChildren} retired` : "";
  if (managed) {
    return {
      value: manager ? `managed by ${manager}` : "managed · no manager",
      detail: `${lineagePart} · ${childPart} · ${childDetail}${retiredDetail}`,
      tone: manager ? "warn" : "bad",
    };
  }
  return {
    value: activeChildren.length > 0 ? `leader · ${childPart}` : `direct · ${childPart}`,
    detail: `${lineagePart} · ${childPart} · ${childDetail}${retiredDetail}`,
    tone: activeChildren.length > 0 ? "good" : "muted",
  };
}

export function agentDelegationPassport(
  profile: Pick<AgentProfile, "enabled" | "retired" | "kind" | "direct_callable" | "managed" | "owner_agent" | "parent_agent">,
): { detail: string; tone: "good" | "warn" | "bad" | "muted" } {
  const owner = profile.owner_agent?.trim();
  const parent = profile.parent_agent?.trim();
  const manager = parent || owner;
  const managed = profile.kind === "subagent" || profile.managed || profile.direct_callable === false;
  if (profile.retired) return { detail: "graveyard · wake blocked", tone: "muted" };
  if (profile.enabled === false) return { detail: "paused · wake blocked", tone: "bad" };
  if (managed) {
    return {
      detail: manager ? `manager-only wake · ${manager}` : "manager-only wake · no manager",
      tone: manager ? "warn" : "bad",
    };
  }
  const lineage = parent ? ` · parent ${parent}` : owner ? ` · owner ${owner}` : "";
  return { detail: `operator/schedule/channel wake${lineage}`, tone: "good" };
}

export function agentWakeRoutePassport(
  profile: Pick<AgentProfile, "enabled" | "retired" | "kind" | "direct_callable" | "managed" | "owner_agent" | "parent_agent">,
): { value: string; detail: string; tone: "good" | "warn" | "bad" | "muted" } {
  const owner = profile.owner_agent?.trim();
  const parent = profile.parent_agent?.trim();
  const manager = parent || owner;
  const managed = profile.kind === "subagent" || profile.managed || profile.direct_callable === false;
  if (profile.retired) {
    return {
      value: "wake blocked",
      detail: "operator blocked · schedule blocked · channel blocked · delegation blocked · revive required",
      tone: "muted",
    };
  }
  if (profile.enabled === false) {
    return {
      value: "wake blocked",
      detail: "operator blocked · schedule blocked · channel blocked · delegation blocked · agent paused",
      tone: "bad",
    };
  }
  if (managed) {
    return {
      value: manager ? `manager-only · ${manager}` : "manager-only · no manager",
      detail: manager
        ? `operator blocked · schedule blocked · channel blocked · delegation allowed from ${manager}`
        : "operator blocked · schedule blocked · channel blocked · delegation blocked · parent/owner missing",
      tone: manager ? "warn" : "bad",
    };
  }
  const lineage = parent ? ` · parent ${parent}` : owner ? ` · owner ${owner}` : "";
  return {
    value: "direct wake",
    detail: `operator allowed · schedule allowed · channel allowed · delegation allowed${lineage}`,
    tone: "good",
  };
}

export function agentIdentityDossier(
  profile: Pick<AgentProfile, "system" | "kind" | "managed" | "direct_callable" | "owner_agent" | "parent_agent" | "lifecycle" | "tasklist" | "retired">,
): string {
  const kind = agentIdentityKind(profile);
  const command = agentHierarchySummary(profile);
  const contract = agentTaskContractSummary(profile);
  return `${kind} identity · ${command} · ${contract}`;
}

export function agentCapabilityPassportSummary(profile: Pick<AgentProfile, "system" | "trust_ceiling" | "tool_allow" | "tool_deny">): string {
  const ceiling = (profile.trust_ceiling || "L4").trim();
  const allow = (profile.tool_allow || []).map((x) => x.trim()).filter(Boolean);
  const deny = (profile.tool_deny || []).map((x) => x.trim()).filter(Boolean);
  const highImpact = highImpactToolNames(allow);
  const blockedHighImpact = highImpactToolNames(deny);
  if (highImpact.length > 0) {
    return `high-impact allow · ${highImpact.join(", ")} · ${ceiling}`;
  }
  if (allow.length > 0) {
    return `limited · ${allow.length} ${plural(allow.length, "allow", "allows")}${deny.length > 0 ? ` · ${deny.length} ${plural(deny.length, "deny", "denies")}` : ""} · ${ceiling}`;
  }
  if (profile.system) {
    return `system-governed · notify gated · ${ceiling}`;
  }
  if (blockedHighImpact.length > 0) {
    return `broad minus · ${blockedHighImpact.join(", ")} · ${ceiling}`;
  }
  if (deny.length > 0) {
    return `guarded · ${deny.length} ${plural(deny.length, "deny", "denies")} · ${ceiling}`;
  }
  if (ceiling !== "L4") return `trust-gated high-impact · ${ceiling}`;
  return "open high-impact · L4";
}

export interface AgentControlCenterEntry {
  label: string;
  value: string;
  detail: string;
  tone: "good" | "warn" | "bad" | "accent" | "muted";
}

export function agentControlCenterLedger(
  profile: Pick<AgentProfile, "system" | "slug" | "trust_ceiling" | "tool_allow" | "tool_deny" | "memory_scope" | "workdir" | "config_overrides" | "noise_policy">,
  invalidRuntimeOverrides = 0,
): AgentControlCenterEntry[] {
  const allow = (profile.tool_allow || []).map((x) => x.trim()).filter(Boolean);
  const deny = (profile.tool_deny || []).map((x) => x.trim()).filter(Boolean);
  const allowLower = allow.map((x) => x.toLowerCase());
  const denyLower = deny.map((x) => x.toLowerCase());
  const highAllow = highImpactToolNames(allow);
  const highDeny = highImpactToolNames(deny);
  const ceiling = (profile.trust_ceiling || "L4").trim() || "L4";
  const dataDenied = denyLower.includes("db") || denyLower.includes("data") || denyLower.includes("datalake");
  const dataAllowed = allow.length === 0 || allowLower.includes("db") || allowLower.includes("data") || allowLower.includes("datalake");
  const memoryDenied = denyLower.includes("memory") || !!profile.noise_policy?.disable_memory_writes;
  const cfgCount = Object.keys(profile.config_overrides || {}).length;
  const quietBits = [
    profile.noise_policy?.silent_on_success ? "silent success" : "",
    profile.noise_policy?.disable_memory_writes ? "memory off" : "",
    profile.noise_policy?.min_notify_severity ? `notify >= ${profile.noise_policy.min_notify_severity}` : "",
    profile.noise_policy?.min_notify_interval_sec ? `cooldown ${profile.noise_policy.min_notify_interval_sec}s` : "",
  ].filter(Boolean);
  return [
    {
      label: "tools",
      value: allow.length > 0
        ? `allow ${allow.length}${highAllow.length ? ` · ${highAllow.join(", ")}` : ""}`
        : deny.length > 0
          ? `broad minus ${deny.length}`
          : "broad tools",
      detail: [
        allow.length > 0 ? `allow: ${allow.join(", ")}` : "allow: all registered tools",
        deny.length > 0 ? `deny: ${deny.join(", ")}${highDeny.length ? ` (high-impact: ${highDeny.join(", ")})` : ""}` : "deny: none",
      ].join(" · "),
      tone: allow.length > 0 || deny.length > 0 || profile.system ? "good" : "warn",
    },
    {
      label: "trust",
      value: ceiling,
      detail: ceiling === "L4" ? "high-impact actions can run under normal policy" : `trust ceiling ${ceiling}; higher-risk actions must be denied or escalated`,
      tone: ceiling === "L4" ? "warn" : trustRank(ceiling) <= trustRank("L2") ? "good" : "accent",
    },
    {
      label: "data lake",
      value: dataDenied ? "blocked" : dataAllowed ? "available" : "not allowlisted",
      detail: dataDenied ? "db/data/datalake access is denied" : dataAllowed ? "agent can use data-lake style tools when registered" : "allowlist omits db/data/datalake tools",
      tone: dataDenied || !dataAllowed ? "warn" : "good",
    },
    {
      label: "memory",
      value: memoryDenied ? "writes off" : `scope ${profile.memory_scope || profile.slug || "own"}`,
      detail: memoryDenied ? `memory writes disabled · scope ${profile.memory_scope || profile.slug || "own"}` : `memory scope ${profile.memory_scope || profile.slug || "own"}`,
      tone: memoryDenied ? "warn" : "good",
    },
    {
      label: "config",
      value: cfgCount > 0 ? `${cfgCount} override${cfgCount === 1 ? "" : "s"}${invalidRuntimeOverrides ? ` · !${invalidRuntimeOverrides}` : ""}` : "default",
      detail: cfgCount > 0 ? `agent-local config: ${Object.keys(profile.config_overrides || {}).sort().join(", ")}` : "uses daemon/default config center values",
      tone: invalidRuntimeOverrides > 0 ? "warn" : cfgCount > 0 ? "good" : "muted",
    },
    {
      label: "noise",
      value: quietBits.length > 0 ? quietBits.join(" · ") : "default",
      detail: quietBits.length > 0 ? quietBits.join(" · ") : "no agent-specific notification or memory quieting policy",
      tone: quietBits.length > 0 ? "good" : profile.system ? "warn" : "muted",
    },
  ];
}

export function agentResourcePassportSummary(
  profile: Pick<AgentProfile, "slug" | "workdir" | "memory_scope" | "tool_allow" | "tool_deny" | "config_overrides">,
): { detail: string; tone: "good" | "warn" | "muted" } {
  const allow = (profile.tool_allow || []).map((x) => x.trim().toLowerCase()).filter(Boolean);
  const deny = (profile.tool_deny || []).map((x) => x.trim().toLowerCase()).filter(Boolean);
  const workspace = profile.workdir?.trim() ? `workspace ${profile.workdir.trim()}` : "shared workspace";
  const memory = `memory ${(profile.memory_scope || profile.slug || "own scope").trim()}`;
  const dbDenied = deny.includes("db") || deny.includes("data") || deny.includes("datalake");
  const dbOpen = allow.length === 0 || allow.includes("db") || allow.includes("data") || allow.includes("datalake");
  const data = dbDenied ? "data lake blocked" : dbOpen ? "data lake via db" : "data lake not allowlisted";
  const cfgCount = Object.keys(profile.config_overrides || {}).length;
  const cfg = cfgCount > 0 ? `${cfgCount} cfg` : "default config";
  return {
    detail: `${workspace} · ${memory} · ${data} · ${cfg}`,
    tone: dbDenied || !dbOpen ? "warn" : profile.workdir || profile.memory_scope || cfgCount > 0 ? "good" : "muted",
  };
}

export function agentConfigPassportSummary(
  profile: Pick<AgentProfile, "config_overrides" | "noise_policy">,
  invalidRuntimeOverrides = 0,
): string {
  const configCount = Object.keys(profile.config_overrides || {}).length;
  const quiet = profile.noise_policy?.silent_on_success || profile.noise_policy?.disable_memory_writes || profile.noise_policy?.min_notify_severity;
  const parts = [
    configCount > 0 ? `${configCount} cfg${invalidRuntimeOverrides > 0 ? ` · !${invalidRuntimeOverrides}` : ""}` : "default config",
    quiet ? "quiet policy" : "",
    profile.noise_policy?.disable_memory_writes ? "memory off" : "",
  ].filter(Boolean);
  return parts.join(" · ");
}

export function agentResiliencePassportSummary(profile: Pick<AgentProfile, "retry_policy" | "health_policy" | "self_repair">): string {
  const parts: string[] = [];
  if (profile.retry_policy?.max_attempts) parts.push(`retry ${profile.retry_policy.max_attempts}x`);
  if (profile.health_policy?.doctor_agent) parts.push(`doctor ${profile.health_policy.doctor_agent}`);
  if (profile.self_repair?.enabled) parts.push(`self-repair${profile.self_repair.max_attempts ? ` ${profile.self_repair.max_attempts}x` : ""}`);
  if (profile.self_repair?.escalate_to) parts.push(`escalate ${profile.self_repair.escalate_to}`);
  return parts.join(" · ") || "manual recovery";
}

export function agentRepairGovernancePassport(
  profile: Pick<AgentProfile, "retry_policy" | "health_policy" | "self_repair">,
): { value: string; detail: string; tone: "good" | "warn" } {
  const value = agentResiliencePassportSummary(profile);
  const retry = profile.retry_policy;
  const health = profile.health_policy;
  const repair = profile.self_repair;
  const detail = [
    retry?.max_attempts && retry.max_attempts > 1
      ? `run retry ${retry.max_attempts}x${retry.backoff ? ` ${retry.backoff}` : ""}${retry.retry_on?.length ? ` on ${retry.retry_on.join(", ")}` : ""}`
      : "single attempt",
    health?.doctor_agent
      ? `doctor ${health.doctor_agent}${health.failure_threshold ? ` after ${health.failure_threshold} failures` : ""}`
      : "no doctor",
    repair?.enabled
      ? `self-repair${repair.max_attempts ? ` ${repair.max_attempts}x` : ""}${repair.escalate_to ? ` then ${repair.escalate_to}` : ""}`
      : "self-repair off",
  ].join(" · ");
  return {
    value,
    detail,
    tone: value === "manual recovery" ? "warn" : "good",
  };
}

export function agentRepairOperationsPassport(
  profile: Pick<AgentProfile, "retry_policy" | "health_policy" | "self_repair" | "retired">,
  runtime: Partial<Pick<
    AgentCardRuntimeSummary,
    | "repairText"
    | "repairDetail"
    | "repairTone"
    | "repairInflight"
    | "repairKindText"
    | "repairIncidentDetail"
    | "nextRepairEligibleMs"
    | "retryText"
    | "retryDetail"
  >>,
): { value: string; detail: string; tone: "good" | "warn" | "bad" | "accent" | "muted" } {
  if (profile.retired) {
    return { value: "graveyard", detail: "repair blocked until the agent is revived", tone: "muted" };
  }
  const repairInflight = runtime.repairInflight || 0;
  if (repairInflight > 0) {
    return {
      value: runtime.repairText || `repair ${repairInflight}`,
      detail: [runtime.repairKindText || "repair", runtime.repairDetail, runtime.repairIncidentDetail].filter(Boolean).join(" · "),
      tone: "accent",
    };
  }
  if (runtime.repairText) {
    const tone = runtime.repairTone === "bad" ? "bad" : runtime.repairTone === "good" ? "good" : runtime.repairTone === "accent" ? "accent" : "muted";
    return {
      value: runtime.repairText,
      detail: [
        runtime.repairKindText || "repair",
        runtime.repairDetail,
        runtime.repairIncidentDetail,
        runtime.nextRepairEligibleMs ? `next eligible ${fmtDateTime(runtime.nextRepairEligibleMs)}` : "",
      ].filter(Boolean).join(" · "),
      tone,
    };
  }
  if (runtime.retryText) {
    return {
      value: runtime.retryText,
      detail: runtime.retryDetail || "whole-run retry pressure is active",
      tone: "bad",
    };
  }
  const governance = agentRepairGovernancePassport(profile);
  if (governance.value !== "manual recovery") {
    return { value: "repair guarded", detail: governance.detail, tone: "good" };
  }
  return {
    value: "manual repair",
    detail: "no retry, doctor, or self-repair policy configured",
    tone: "warn",
  };
}

export function agentModelRoutePassport(
  profile: Pick<AgentProfile, "model" | "fallbacks" | "task_type">,
): { value: string; detail: string; tone: "good" | "warn" | "muted" } {
  const model = (profile.model || "").trim();
  const task = (profile.task_type || "").trim();
  const fallbacks = (profile.fallbacks || []).map((m) => m.trim()).filter(Boolean);
  if (model && isChainRef(model)) {
    return {
      value: `chain ${model}`,
      detail: [task ? `task ${task}` : "default task", "named chain owns fallback order", "per-agent fallbacks ignored"].join(" · "),
      tone: "good",
    };
  }
  if (model) {
    return {
      value: `model ${model}`,
      detail: [task ? `task ${task}` : "default task", fallbacks.length > 0 ? `${fallbacks.length} fallback${fallbacks.length === 1 ? "" : "s"}: ${fallbacks.join(" → ")}` : "no per-agent fallback"].join(" · "),
      tone: fallbacks.length > 0 ? "good" : "warn",
    };
  }
  return {
    value: "daemon default model",
    detail: [task ? `task ${task}` : "default task", "uses daemon/provider routing; no agent-owned fallback"].join(" · "),
    tone: "muted",
  };
}

export function agentModelPassportSummary(profile: Pick<AgentProfile, "model" | "fallbacks" | "task_type">): string {
  const route = agentModelRoutePassport(profile);
  const fallbacks = (profile.fallbacks || []).map((m) => m.trim()).filter(Boolean);
  const fallback = route.detail.includes("named chain owns fallback order")
    ? "chain-owned fallback"
    : route.detail.includes("no per-agent fallback") || route.detail.includes("no agent-owned fallback")
      ? "no fallback"
      : fallbacks.length > 0
        ? `${fallbacks.length} fallback${fallbacks.length === 1 ? "" : "s"}`
        : "no fallback";
  const task = (profile.task_type || "").trim();
  return [route.value, task ? `task ${task}` : "", fallback].filter(Boolean).join(" · ");
}

export function agentSkillPassportSummary(count: number): string {
  return count > 0 ? `${count} private skill${count === 1 ? "" : "s"}` : "no private skills";
}

export interface AgentIdentityLedgerEntry {
  label: string;
  value: string;
  detail: string;
  tone: "good" | "warn" | "bad" | "accent" | "muted";
}

export function agentIdentityLedger(
  profile: Pick<AgentProfile, "slug" | "name" | "soul" | "system" | "kind" | "managed" | "direct_callable" | "owner_agent" | "parent_agent" | "lifecycle" | "tasklist" | "retired" | "enabled" | "model" | "fallbacks" | "task_type" | "workdir" | "memory_scope" | "tool_allow" | "tool_deny" | "trust_ceiling" | "config_overrides">,
  privateSkillCount = 0,
): AgentIdentityLedgerEntry[] {
  const kind = agentIdentityKind(profile);
  const soul = (profile.soul || "").trim();
  const lifecycle = agentLifecycleDispositionPassport(profile);
  const model = agentModelPassportSummary(profile);
  const resources = agentResourcePassportSummary(profile);
  const authority = agentCapabilityPassportSummary(profile);
  const identityName = profile.name && profile.name !== profile.slug ? profile.name : profile.slug;
  return [
    {
      label: "identity",
      value: `${kind} · ${profile.retired ? "graveyard" : profile.enabled === false ? "paused" : "alive"}`,
      detail: [identityName, agentHierarchySummary(profile), agentTaskProgressSummary(profile.tasklist) || "no tasks"].filter(Boolean).join(" · "),
      tone: profile.retired ? "muted" : profile.enabled === false ? "warn" : kind === "subagent" ? "accent" : "good",
    },
    {
      label: "soul",
      value: soul ? "soul set" : "soul missing",
      detail: soul ? soul.slice(0, 180) : "this agent has no durable soul prompt",
      tone: soul ? "good" : "warn",
    },
    {
      label: "model",
      value: model,
      detail: model,
      tone: profile.model ? "good" : "muted",
    },
    {
      label: "memory",
      value: `memory ${profile.memory_scope || profile.slug || "own scope"}`,
      detail: resources.detail,
      tone: resources.tone,
    },
    {
      label: "authority",
      value: authority,
      detail: [authority, privateSkillCount > 0 ? `${privateSkillCount} private skill${privateSkillCount === 1 ? "" : "s"}` : "no private skills"].join(" · "),
      tone: profile.system || profile.trust_ceiling || (profile.tool_allow || []).length > 0 || (profile.tool_deny || []).length > 0 ? "good" : "warn",
    },
    {
      label: "lifecycle",
      value: lifecycle.value,
      detail: lifecycle.detail,
      tone: lifecycle.tone,
    },
  ];
}

export interface AgentRemovalPlanInput {
  standing: string[];
  schedules: string[];
  memories: string[];
  authoredMemories: string[];
  skills: string[];
  configs: string[];
  workspaces?: string[];
  workflowRefs?: string[];
  mailboxMessages?: string[];
  subagents: string[];
  subagentStanding: string[];
  subagentSchedules: string[];
  subagentMemories: string[];
  subagentAuthoredMemories: string[];
  subagentSkills: string[];
  subagentConfigs: string[];
  subagentWorkspaces?: string[];
  subagentWorkflowRefs?: string[];
  subagentMailboxMessages?: string[];
  cascade: { standing: boolean; schedules: boolean; memory: boolean; authored_memory: boolean; skills: boolean; config: boolean; workspace?: boolean; subagents: boolean };
}

export function agentRemovalCascadePreset(mode: "clean_all" | "keep_all"): AgentRemovalPlanInput["cascade"] {
  const enabled = mode === "clean_all";
  return {
    standing: enabled,
    schedules: enabled,
    memory: enabled,
    authored_memory: enabled,
    skills: enabled,
    config: enabled,
    workspace: enabled,
    subagents: enabled,
  };
}

export function agentRemovalPlan(input: AgentRemovalPlanInput): { cleanupPlan: string[]; keepPlan: string[]; blockedBySubagents: boolean } {
  const c = { workspace: false, ...input.cascade };
  const workspaces = input.workspaces || [];
  const workflowRefs = input.workflowRefs || [];
  const mailboxMessages = input.mailboxMessages || [];
  const subagentWorkspaces = input.subagentWorkspaces || [];
  const subagentWorkflowRefs = input.subagentWorkflowRefs || [];
  const subagentMailboxMessages = input.subagentMailboxMessages || [];
  const cleanupPlan = [
    c.standing && input.standing.length > 0 ? `${input.standing.length} standing` : "",
    c.schedules && input.schedules.length > 0 ? `${input.schedules.length} schedule` : "",
    c.memory && input.memories.length > 0 ? `${input.memories.length} private memory` : "",
    c.authored_memory && input.authoredMemories.length > 0 ? `${input.authoredMemories.length} authored shared memory` : "",
    c.skills && input.skills.length > 0 ? `${input.skills.length} skill` : "",
    c.config && input.configs.length > 0 ? `${input.configs.length} config` : "",
    c.config ? "shared config access refs" : "",
    c.workspace && workspaces.length > 0 ? `${workspaces.length} workspace` : "",
    c.subagents && input.subagents.length > 0 ? `${input.subagents.length} sub-agent` : "",
    c.subagents && c.standing && input.subagentStanding.length > 0 ? `${input.subagentStanding.length} sub-agent standing` : "",
    c.subagents && c.schedules && input.subagentSchedules.length > 0 ? `${input.subagentSchedules.length} sub-agent schedule` : "",
    c.subagents && c.memory && input.subagentMemories.length > 0 ? `${input.subagentMemories.length} sub-agent private memory` : "",
    c.subagents && c.authored_memory && input.subagentAuthoredMemories.length > 0 ? `${input.subagentAuthoredMemories.length} sub-agent authored shared memory` : "",
    c.subagents && c.skills && input.subagentSkills.length > 0 ? `${input.subagentSkills.length} sub-agent skill` : "",
    c.subagents && c.config && input.subagentConfigs.length > 0 ? `${input.subagentConfigs.length} sub-agent config` : "",
    c.subagents && c.workspace && subagentWorkspaces.length > 0 ? `${subagentWorkspaces.length} sub-agent workspace` : "",
  ].filter(Boolean);
  const keepPlan = [
    !c.standing && input.standing.length > 0 ? `${input.standing.length} standing` : "",
    !c.schedules && input.schedules.length > 0 ? `${input.schedules.length} schedule` : "",
    !c.memory && input.memories.length > 0 ? `${input.memories.length} private memory` : "",
    !c.authored_memory && input.authoredMemories.length > 0 ? `${input.authoredMemories.length} authored shared memory` : "",
    !c.skills && input.skills.length > 0 ? `${input.skills.length} skill` : "",
    !c.config && input.configs.length > 0 ? `${input.configs.length} config` : "",
    !c.config && (input.configs.length > 0 || input.subagentConfigs.length > 0) ? "shared config access refs" : "",
    !c.workspace && workspaces.length > 0 ? `${workspaces.length} workspace` : "",
    workflowRefs.length > 0 ? `${workflowRefs.length} workflow reference` : "",
    mailboxMessages.length > 0 ? `${mailboxMessages.length} mailbox/audit messages` : "",
    (!c.subagents || !c.standing) && input.subagentStanding.length > 0 ? `${input.subagentStanding.length} sub-agent standing` : "",
    (!c.subagents || !c.schedules) && input.subagentSchedules.length > 0 ? `${input.subagentSchedules.length} sub-agent schedule` : "",
    (!c.subagents || !c.memory) && input.subagentMemories.length > 0 ? `${input.subagentMemories.length} sub-agent private memory` : "",
    (!c.subagents || !c.authored_memory) && input.subagentAuthoredMemories.length > 0 ? `${input.subagentAuthoredMemories.length} sub-agent authored shared memory` : "",
    (!c.subagents || !c.skills) && input.subagentSkills.length > 0 ? `${input.subagentSkills.length} sub-agent skill` : "",
    (!c.subagents || !c.config) && input.subagentConfigs.length > 0 ? `${input.subagentConfigs.length} sub-agent config` : "",
    (!c.subagents || !c.workspace) && subagentWorkspaces.length > 0 ? `${subagentWorkspaces.length} sub-agent workspace` : "",
    subagentWorkflowRefs.length > 0 ? `${subagentWorkflowRefs.length} sub-agent workflow reference` : "",
    subagentMailboxMessages.length > 0 ? `${subagentMailboxMessages.length} sub-agent mailbox/audit messages` : "",
  ].filter(Boolean);
  return { cleanupPlan, keepPlan, blockedBySubagents: input.subagents.length > 0 && !c.subagents };
}

export function agentRemovalImpactSummary(plan: { cleanupPlan: string[]; keepPlan: string[]; blockedBySubagents: boolean }): string {
  const cleaned = plan.cleanupPlan.length;
  const kept = plan.keepPlan.length;
  const parts = [
    cleaned > 0 ? `${cleaned} cleanup group${cleaned === 1 ? "" : "s"}` : "no cleanup selected",
    kept > 0 ? `${kept} retained group${kept === 1 ? "" : "s"}` : "",
    plan.blockedBySubagents ? "blocked by dependent sub-agent tree" : "",
  ].filter(Boolean);
  return parts.join(" · ");
}

export interface AgentRemovalDecisionSummary {
  label: string;
  detail: string;
  tone: "good" | "warn" | "bad" | "muted";
}

export interface AgentRemovalLifecycleSummary {
  label: string;
  detail: string;
  tone: "good" | "warn" | "bad" | "muted";
}

export interface AgentRemovalLedgerEntry {
  label: string;
  value: string;
  detail: string;
  tone: "good" | "warn" | "bad" | "muted";
}

export interface AgentRemovalDeathCertificate {
  label: string;
  detail: string;
  tone: "good" | "warn" | "bad" | "muted";
  fields: {
    identity: string;
    dependents: string;
    cleanup: string;
    retained: string;
    audit: string;
    guard: string;
  };
}

export interface AgentRemovalCustodySummary {
  deletedGroups: number;
  retainedGroups: number;
  hardRetainedGroups: number;
  subagentsRetired: number;
  label: string;
  detail: string;
  tone: "good" | "warn" | "bad" | "muted";
}

export function agentRemovalDecisionSummary(plan: { cleanupPlan: string[]; keepPlan: string[]; blockedBySubagents: boolean }): AgentRemovalDecisionSummary {
  if (plan.blockedBySubagents) {
    return {
      label: "removal blocked",
      detail: "dependent sub-agent tree would be orphaned; include it so every descendant retires before this identity is deleted",
      tone: "bad",
    };
  }
  if (plan.keepPlan.length > 0) {
    return {
      label: "remove with retained resources",
      detail: `delete identity · clean ${plan.cleanupPlan.length || 0} group${plan.cleanupPlan.length === 1 ? "" : "s"} · keep ${plan.keepPlan.join(", ")}`,
      tone: "warn",
    };
  }
  if (plan.cleanupPlan.length > 0) {
    return {
      label: "remove and clean owned resources",
      detail: `delete identity · clean ${plan.cleanupPlan.join(", ")}`,
      tone: "good",
    };
  }
  return {
    label: "identity-only removal",
    detail: "delete identity profile only; no dependent cleanup selected",
    tone: "muted",
  };
}

export function agentRemovalCustodySummary(input: AgentRemovalPlanInput): AgentRemovalCustodySummary {
  const plan = agentRemovalPlan(input);
  const hardRetained = plan.keepPlan.filter((item) => item.includes("mailbox/audit") || item.includes("workflow reference")).length;
  const operatorRetained = Math.max(0, plan.keepPlan.length - hardRetained);
  const subagentsRetired = input.cascade.subagents ? input.subagents.length : 0;
  if (plan.blockedBySubagents) {
    return {
      deletedGroups: plan.cleanupPlan.length,
      retainedGroups: operatorRetained,
      hardRetainedGroups: hardRetained,
      subagentsRetired,
      label: "custody blocked",
      detail: "dependent sub-agent identities must be included before this profile can be deleted",
      tone: "bad",
    };
  }
  const detail = [
    `${plan.cleanupPlan.length} cleanup group${plan.cleanupPlan.length === 1 ? "" : "s"}`,
    operatorRetained > 0 ? `${operatorRetained} operator-retained group${operatorRetained === 1 ? "" : "s"}` : "no operator-retained owned groups",
    hardRetained > 0 ? `${hardRetained} audit/workflow group${hardRetained === 1 ? "" : "s"} retained by design` : "event log retained",
    subagentsRetired > 0 ? `${subagentsRetired} sub-agent${subagentsRetired === 1 ? "" : "s"} retired` : "",
  ].filter(Boolean).join(" · ");
  return {
    deletedGroups: plan.cleanupPlan.length,
    retainedGroups: operatorRetained,
    hardRetainedGroups: hardRetained,
    subagentsRetired,
    label: operatorRetained > 0 || hardRetained > 0 || subagentsRetired > 0 ? "custody split" : "delete-only custody",
    detail,
    tone: operatorRetained > 0 || subagentsRetired > 0 ? "warn" : plan.cleanupPlan.length > 0 ? "good" : "muted",
  };
}

export function agentRemovalLifecycleSummary(input: AgentRemovalPlanInput): AgentRemovalLifecycleSummary {
  const subagentCount = input.subagents.length;
  const auditCount = (input.mailboxMessages || []).length + (input.subagentMailboxMessages || []).length;
  if (subagentCount > 0 && !input.cascade.subagents) {
    return {
      label: "orphan guard",
      detail: `${subagentCount} dependent sub-agent${subagentCount === 1 ? "" : "s"} would lose their owner; removal is blocked until they are retired with the parent`,
      tone: "bad",
    };
  }
  const subagentPart = subagentCount > 0
    ? `${subagentCount} dependent sub-agent${subagentCount === 1 ? "" : "s"} retired, not deleted`
    : "no dependent sub-agents";
  const auditPart = auditCount > 0
    ? `${auditCount} mailbox/audit record${auditCount === 1 ? "" : "s"} retained`
    : "audit trail retained by event log";
  return {
    label: "identity death plan",
    detail: `profile deleted · ${subagentPart} · ${auditPart}`,
    tone: subagentCount > 0 ? "warn" : "muted",
  };
}

export function agentRemovalDeathCertificate(input: AgentRemovalPlanInput): AgentRemovalDeathCertificate {
  const plan = agentRemovalPlan(input);
  const subagentCount = input.subagents.length;
  const subagentsRetired = input.cascade.subagents ? subagentCount : 0;
  const workflowRefs = (input.workflowRefs || []).length + (input.subagentWorkflowRefs || []).length;
  const auditCount = (input.mailboxMessages || []).length + (input.subagentMailboxMessages || []).length;
  const retainedOwned = plan.keepPlan.filter((item) => !item.includes("mailbox/audit") && !item.includes("workflow reference")).length;
  const fields = {
    identity: "profile deleted; soul, settings, lifecycle removed",
    dependents: subagentCount > 0
      ? input.cascade.subagents
        ? `${subagentCount} retired to graveyard`
        : `${subagentCount} would be orphaned`
      : "no dependent sub-agents",
    cleanup: plan.cleanupPlan.length > 0 ? `${plan.cleanupPlan.length} cleanup ${plural(plan.cleanupPlan.length, "group", "groups")}` : "no owned cleanup",
    retained: [
      retainedOwned > 0 ? `${retainedOwned} owned retained` : "",
      workflowRefs > 0 ? `${workflowRefs} workflow ${plural(workflowRefs, "ref", "refs")} retained` : "",
    ].filter(Boolean).join(" · ") || "no reusable owned refs retained",
    audit: auditCount > 0 ? `${auditCount} mailbox/audit ${plural(auditCount, "record", "records")} retained` : "event log records deletion",
    guard: plan.blockedBySubagents ? "blocked by dependent sub-agents" : "ready",
  };
  if (plan.blockedBySubagents) {
    return {
      label: "death blocked",
      detail: Object.values(fields).join(" · "),
      tone: "bad",
      fields,
    };
  }
  const label = subagentsRetired > 0
    ? "retire dependent tree"
    : plan.cleanupPlan.length > 0
      ? "delete identity and clean custody"
      : "delete identity only";
  return {
    label,
    detail: Object.values(fields).join(" · "),
    tone: subagentsRetired > 0 || retainedOwned > 0 || workflowRefs > 0 ? "warn" : plan.cleanupPlan.length > 0 ? "good" : "muted",
    fields,
  };
}

export function agentRemovalLedger(input: AgentRemovalPlanInput): AgentRemovalLedgerEntry[] {
  const plan = agentRemovalPlan(input);
  const c = { workspace: false, ...input.cascade };
  const auditCount = (input.mailboxMessages || []).length + (input.subagentMailboxMessages || []).length;
  const ownedDirect = input.standing.length + input.schedules.length + input.memories.length + input.authoredMemories.length + input.skills.length + input.configs.length + (input.workspaces || []).length + (input.workflowRefs || []).length;
  const ownedSubagent =
    input.subagentStanding.length +
    input.subagentSchedules.length +
    input.subagentMemories.length +
    input.subagentAuthoredMemories.length +
    input.subagentSkills.length +
    input.subagentConfigs.length +
    (input.subagentWorkspaces || []).length +
    (input.subagentWorkflowRefs || []).length;
  const retainedOwned = plan.keepPlan.filter((item) => !item.includes("mailbox/audit")).length;
  return [
    {
      label: "identity",
      value: "delete profile",
      detail: "soul, settings, lifecycle, and direct identity record are removed",
      tone: "bad",
    },
    {
      label: "sub-agents",
      value: input.subagents.length > 0
        ? c.subagents
          ? `${input.subagents.length} retire`
          : `${input.subagents.length} orphan risk`
        : "none",
      detail: input.subagents.length > 0
        ? c.subagents
          ? "dependent identities are retired to the graveyard, not deleted"
          : "dependent identities would lose their parent/owner; removal is blocked"
        : "no dependent sub-agent tree",
      tone: input.subagents.length === 0 ? "muted" : c.subagents ? "warn" : "bad",
    },
    {
      label: "owned cleanup",
      value: plan.cleanupPlan.length > 0 ? `${plan.cleanupPlan.length} groups` : "none",
      detail: plan.cleanupPlan.length > 0 ? plan.cleanupPlan.join(", ") : "no owned dependent cleanup selected",
      tone: plan.cleanupPlan.length > 0 ? "good" : ownedDirect + ownedSubagent > 0 ? "warn" : "muted",
    },
    {
      label: "retained owned",
      value: retainedOwned > 0 ? `${retainedOwned} groups` : "none",
      detail: retainedOwned > 0 ? plan.keepPlan.filter((item) => !item.includes("mailbox/audit")).join(", ") : "no owned resources intentionally retained",
      tone: retainedOwned > 0 ? "warn" : "good",
    },
    {
      label: "audit trail",
      value: auditCount > 0 ? `${auditCount} retained` : "event log",
      detail: auditCount > 0 ? "mailbox/audit records are retained for inspection" : "identity removal is still recorded in the event log",
      tone: "muted",
    },
    {
      label: "guard",
      value: plan.blockedBySubagents ? "blocked" : "ready",
      detail: plan.blockedBySubagents ? "select dependent sub-agent tree before hard removal" : "hard removal can proceed with the selected cascade",
      tone: plan.blockedBySubagents ? "bad" : "good",
    },
  ];
}

function plural(count: number, one: string, many: string): string {
  return count === 1 ? one : many;
}

export function rosterWakeIssue(profile: Pick<AgentProfile, "enabled" | "retired" | "kind" | "managed" | "direct_callable" | "parent_agent" | "owner_agent">): string {
  if (profile.retired) return "revive this agent before waking it";
  if (profile.enabled === false) return "resume this agent before waking it";
  if (profile.kind === "subagent" || profile.managed || profile.direct_callable === false) {
    const owner = profile.parent_agent || profile.owner_agent;
    return owner ? `managed sub-agent; wake ${owner} instead` : "managed sub-agent; only its parent/owner can wake it";
  }
  return "";
}

export function rosterRepairIssue(profile: Pick<AgentProfile, "retired" | "kind" | "managed" | "direct_callable" | "parent_agent" | "owner_agent">): string {
  if (profile.retired) return "revive this agent before requesting repair";
  if (profile.kind === "subagent" || profile.managed || profile.direct_callable === false) {
    const owner = profile.parent_agent || profile.owner_agent;
    return owner ? `managed sub-agent; request repair through ${owner}` : "managed sub-agent; request repair through its parent/owner";
  }
  return "";
}

export function agentHealthIssueSummary(status: Pick<AgentCardRuntimeSummary, "configIssues">): string {
  const issues = status.configIssues || [];
  if (issues.length === 0) return "";
  const first = issues[0] || "configuration issue";
  if (issues.length === 1) return first;
  return `${issues.length} issues · ${first}`;
}

export interface AgentLifecycleRailStep {
  id: "sleep" | "wake" | "work" | "repair";
  label: string;
  value: string;
  detail?: string;
  tone: "good" | "bad" | "warn" | "accent" | "muted";
}

export function agentLifecycleRail(
  profile: Pick<AgentProfile, "retired" | "enabled" | "tasklist" | "self_repair" | "health_policy">,
  runtime: Pick<
    AgentCardRuntimeSummary,
    | "activeRunCount"
    | "activePhase"
    | "liveDetail"
    | "operationalText"
    | "operationalState"
    | "wakeText"
    | "wakeDetail"
    | "nextWakeMs"
    | "repairText"
    | "repairDetail"
    | "repairTone"
    | "repairInflight"
    | "repairKindText"
    | "retryText"
    | "retryDetail"
    | "lastActivitySummary"
  >,
  wakeIssue = "",
  now = Date.now(),
): AgentLifecycleRailStep[] {
  const taskSummary = agentTaskProgressSummary(profile.tasklist);
  const activeTasks = (profile.tasklist || []).filter((t) => t.status !== "retired" && t.status !== "done").length;
  const sleepValue = profile.retired
    ? "graveyard"
    : runtime.activeRunCount > 0
      ? "awake"
      : profile.enabled === false || runtime.operationalState === "paused"
        ? "paused"
        : runtime.operationalText || "sleeping";
  const sleepTone: AgentLifecycleRailStep["tone"] = profile.retired
    ? "muted"
    : runtime.activeRunCount > 0
      ? "accent"
      : profile.enabled === false || runtime.operationalState === "paused"
        ? "warn"
        : "good";
  const wakeValue = profile.retired
    ? "blocked"
    : wakeIssue
      ? "blocked"
      : runtime.nextWakeMs
        ? fmtDue(runtime.nextWakeMs, now)
        : runtime.wakeText || "manual";
  const wakeTone: AgentLifecycleRailStep["tone"] = profile.retired || wakeIssue ? "warn" : runtime.nextWakeMs || runtime.wakeText ? "accent" : "muted";
  const workValue = runtime.activeRunCount > 0
    ? runtime.activePhase || "running"
    : activeTasks > 0
      ? `${activeTasks} task${activeTasks === 1 ? "" : "s"}`
      : "idle";
  const repairValue = runtime.repairInflight > 0
    ? "repairing"
    : runtime.repairText || runtime.retryText || (profile.self_repair?.enabled ? "self-ready" : profile.health_policy?.doctor_agent ? "doctor" : "manual");
  const repairTone: AgentLifecycleRailStep["tone"] = runtime.repairInflight > 0
    ? "accent"
    : runtime.repairTone === "bad"
      ? "bad"
      : runtime.repairTone === "good"
        ? "good"
        : runtime.retryText
          ? "bad"
        : profile.self_repair?.enabled || profile.health_policy?.doctor_agent
          ? "good"
          : "muted";

  return [
    { id: "sleep", label: "Sleep", value: sleepValue, detail: runtime.lastActivitySummary, tone: sleepTone },
    { id: "wake", label: "Wake", value: wakeValue, detail: runtime.wakeDetail || wakeIssue, tone: wakeTone },
    { id: "work", label: "Work", value: workValue, detail: runtime.liveDetail || taskSummary, tone: runtime.activeRunCount > 0 ? "accent" : activeTasks > 0 ? "good" : "muted" },
    { id: "repair", label: "Repair", value: repairValue, detail: runtime.repairKindText || runtime.retryDetail, tone: repairTone },
  ];
}

export function agentLifeSummary(
  profile: Pick<AgentProfile, "retired" | "enabled" | "kind" | "managed" | "direct_callable" | "parent_agent" | "owner_agent" | "tasklist">,
  runtime: Pick<
    AgentCardRuntimeSummary,
    | "activeRunCount"
    | "activePhase"
    | "activeContextDetail"
    | "operationalText"
    | "operationalState"
    | "wakeText"
    | "wakeDetail"
    | "nextWakeMs"
    | "lastActivitySummary"
  >,
  wakeIssue = "",
  waitingMessages = 0,
  now = Date.now(),
): string {
  const state = profile.retired
    ? "graveyard"
    : runtime.activeRunCount > 0
      ? `awake${runtime.activePhase ? ` · ${runtime.activePhase}` : ""}`
      : profile.enabled === false || runtime.operationalState === "paused"
        ? "paused"
        : runtime.operationalText || "sleeping";
  const manager =
    profile.kind === "subagent" || profile.managed || profile.direct_callable === false
      ? profile.parent_agent || profile.owner_agent || "parent/owner"
      : "";
  const wake = profile.retired
    ? "wake blocked"
    : wakeIssue
      ? manager
        ? `manager wake: ${manager}`
        : "wake blocked"
      : runtime.nextWakeMs
        ? `next ${fmtDue(runtime.nextWakeMs, now)}`
        : runtime.wakeText || "manual wake";
  const activeTasks = (profile.tasklist || []).filter((t) => t.status !== "retired" && t.status !== "done").length;
  const work = runtime.activeRunCount > 0
    ? runtime.activeContextDetail || runtime.wakeDetail || "working"
    : waitingMessages > 0
      ? `${waitingMessages} mailbox waiting`
      : activeTasks > 0
        ? `${activeTasks} task${activeTasks === 1 ? "" : "s"} queued`
        : runtime.lastActivitySummary
          ? `last ${runtime.lastActivitySummary}`
          : "idle";
  return [state, wake, work].filter(Boolean).join(" · ");
}

function fmtElapsed(ms: number | undefined, now = Date.now()): string {
  if (!ms || !Number.isFinite(ms) || ms <= 0) return "";
  const elapsed = Math.max(60_000, now - ms);
  return fmtDue(now + elapsed, now).replace(/^in\s+/, "");
}

export function agentLivePresencePassport(
  profile: Pick<AgentProfile, "retired" | "enabled">,
  runtime: Pick<
    AgentCardRuntimeSummary,
    | "activeRunCount"
    | "activePhase"
    | "activeContextDetail"
    | "liveDetail"
    | "operationalText"
    | "operationalState"
    | "wakeText"
    | "wakeDetail"
    | "nextWakeMs"
    | "lastActivitySummary"
    | "activeStartedMs"
    | "activeLastEventMs"
  >,
  waitingMessages = 0,
  now = Date.now(),
): { value: string; detail: string; tone: "good" | "warn" | "bad" | "muted" } {
  if (profile.retired) {
    return {
      value: "graveyard",
      detail: runtime.lastActivitySummary ? `retired · last ${runtime.lastActivitySummary}` : "retired · wake blocked",
      tone: "muted",
    };
  }
  if (profile.enabled === false || runtime.operationalState === "paused") {
    return {
      value: "paused",
      detail: [
        runtime.operationalText || "paused",
        waitingMessages > 0 ? `${waitingMessages} inbox waiting` : "",
        runtime.wakeDetail || runtime.wakeText || "",
      ].filter(Boolean).join(" · "),
      tone: "warn",
    };
  }
  if ((runtime.activeRunCount || 0) > 0) {
    const since = runtime.activeStartedMs ? `running for ${fmtElapsed(runtime.activeStartedMs, now)}` : "";
    const last = runtime.activeLastEventMs ? `last event ${fmtElapsed(runtime.activeLastEventMs, now)} ago` : "";
    return {
      value: runtime.activePhase ? `awake · ${runtime.activePhase}` : `awake · running ${runtime.activeRunCount}`,
      detail: [
        runtime.liveDetail || runtime.activeContextDetail || "active run",
        since,
        last,
      ].filter(Boolean).join(" · "),
      tone: "good",
    };
  }
  if (runtime.nextWakeMs) {
    return {
      value: `sleeping · wakes ${fmtDue(runtime.nextWakeMs, now)}`,
      detail: [
        runtime.wakeDetail || runtime.wakeText || "scheduled wake",
        waitingMessages > 0 ? `${waitingMessages} inbox waiting` : "",
        runtime.lastActivitySummary ? `last ${runtime.lastActivitySummary}` : "",
      ].filter(Boolean).join(" · "),
      tone: waitingMessages > 0 ? "warn" : "good",
    };
  }
  if (waitingMessages > 0) {
    return {
      value: `sleeping · inbox ${waitingMessages}`,
      detail: [
        `${waitingMessages} mailbox message${waitingMessages === 1 ? "" : "s"} waiting`,
        runtime.wakeDetail || runtime.wakeText || "manual or mailbox wake",
      ].filter(Boolean).join(" · "),
      tone: "warn",
    };
  }
  return {
    value: runtime.operationalText || "sleeping",
    detail: [
      runtime.wakeDetail || runtime.wakeText || "manual wake",
      runtime.lastActivitySummary ? `last ${runtime.lastActivitySummary}` : "idle",
    ].filter(Boolean).join(" · "),
    tone: "muted",
  };
}

export interface AgentIdentityCardSummary {
  label: string;
  detail: string;
  tone: "good" | "warn" | "bad" | "accent" | "muted";
}

export interface AgentRosterIdentityManifestEntry {
  label: string;
  value: string;
  detail: string;
  tone: "good" | "warn" | "bad" | "accent" | "muted";
}

export function agentIdentityCardSummary(
  profile: Pick<AgentProfile, "retired" | "enabled" | "system" | "kind" | "managed" | "direct_callable" | "parent_agent" | "owner_agent" | "lifecycle" | "tasklist">,
  runtime: Partial<Pick<
    AgentCardRuntimeSummary,
    | "activeRunCount"
    | "activePhase"
    | "liveDetail"
    | "activeContextDetail"
    | "operationalText"
    | "operationalState"
    | "wakeText"
    | "wakeDetail"
    | "nextWakeMs"
    | "lastActivitySummary"
  >> = {},
  wakeIssue = "",
  waitingMessages = 0,
  now = Date.now(),
  capability = "",
): AgentIdentityCardSummary {
  const kind = agentIdentityKind(profile);
  const delegation = agentDelegationPassport(profile);
  const lifecycle = agentLifecycleDispositionPassport(profile);
  const tasks = agentTaskProgressSummary(profile.tasklist) || "no tasks";
  const contract = agentTaskContractSummary(profile);
  const mailbox = waitingMessages > 0 ? `${waitingMessages} inbox waiting` : "";
  if (profile.retired) {
    return {
      label: `${kind} · graveyard`,
      detail: [contract, "revive/remove", tasks].filter(Boolean).join(" · "),
      tone: "muted",
    };
  }
  if ((runtime.activeRunCount || 0) > 0) {
    return {
      label: `${kind} · running`,
      detail: [
        runtime.liveDetail || runtime.activeContextDetail || runtime.activePhase || "active run",
        delegation.detail,
        capability,
        mailbox,
        tasks,
      ].filter(Boolean).join(" · "),
      tone: "accent",
    };
  }
  if (profile.enabled === false || runtime.operationalState === "paused") {
    return {
      label: `${kind} · paused`,
      detail: [
        runtime.operationalText || "paused",
        delegation.detail,
        capability,
        mailbox,
        tasks,
      ].filter(Boolean).join(" · "),
      tone: "warn",
    };
  }
  const wake = wakeIssue
    ? wakeIssue
    : runtime.nextWakeMs
      ? `next ${fmtDue(runtime.nextWakeMs, now)}`
      : runtime.wakeDetail || runtime.wakeText || delegation.detail;
  const tone: AgentIdentityCardSummary["tone"] = delegation.tone === "bad"
    ? "bad"
    : delegation.tone === "warn" || waitingMessages > 0
      ? "warn"
      : lifecycle.tone === "good"
        ? "good"
        : "muted";
  return {
    label: `${kind} · sleeping`,
    detail: [
      lifecycle.value,
      wake,
      capability,
      mailbox,
      tasks,
      runtime.lastActivitySummary ? `last ${runtime.lastActivitySummary}` : "",
    ].filter(Boolean).join(" · "),
    tone,
  };
}

export function agentRosterIdentityManifest(
  profile: Pick<AgentProfile, "slug" | "name" | "soul" | "retired" | "enabled" | "system" | "kind" | "managed" | "direct_callable" | "parent_agent" | "owner_agent" | "lifecycle" | "tasklist" | "trust_ceiling" | "tool_allow" | "tool_deny">,
  runtime: Partial<Pick<AgentCardRuntimeSummary, "activeRunCount" | "activePhase" | "operationalText" | "operationalState" | "wakeText" | "wakeDetail" | "nextWakeMs">> = {},
  wakeIssue = "",
  waitingMessages = 0,
  privateSkillCount = 0,
  capability = agentCapabilityPassportSummary(profile),
  now = Date.now(),
): AgentRosterIdentityManifestEntry[] {
  const kind = agentIdentityKind(profile);
  const manager = profile.parent_agent || profile.owner_agent || "";
  const soul = (profile.soul || "").trim();
  const lifecycle = agentLifecycleDispositionPassport(profile);
  const taskProgress = agentTaskProgressSummary(profile.tasklist) || "no tasks";
  const contract = agentTaskContractSummary(profile);
  const running = (runtime.activeRunCount || 0) > 0;
  const state = profile.retired
    ? "graveyard"
    : profile.enabled === false || runtime.operationalState === "paused"
      ? "paused"
      : running
        ? runtime.activePhase || "running"
        : runtime.operationalText || "sleeping";
  const wakeValue = profile.retired
    ? "revive required"
    : profile.enabled === false
      ? "blocked"
      : running
        ? "awake"
        : wakeIssue
          ? "blocked"
          : runtime.nextWakeMs
            ? formatWakeDue(runtime.nextWakeMs, now)
            : waitingMessages > 0
              ? `inbox ${waitingMessages}`
              : runtime.wakeText || (manager ? "manager wake" : "manual wake");
  const wakeDetail = profile.retired
    ? "graveyard identity cannot wake until revived"
    : wakeIssue || runtime.wakeDetail || (manager ? `delegation source ${manager}` : "operator, schedule, channel, or mailbox can wake when policy allows");
  return [
    {
      label: "class",
      value: `${kind} · ${state}`,
      detail: [profile.name && profile.name !== profile.slug ? profile.name : profile.slug, agentHierarchySummary(profile)].filter(Boolean).join(" · "),
      tone: profile.retired ? "muted" : profile.enabled === false ? "warn" : running ? "accent" : kind === "subagent" ? "accent" : "good",
    },
    {
      label: "owner",
      value: manager ? `managed by ${manager}` : profile.system ? "system-owned" : "direct",
      detail: manager ? `parent/owner ${manager}; direct callable ${profile.direct_callable === false ? "no" : "yes"}` : profile.system ? "internal guardian identity" : "operator, schedule, channel, and peer delegation may target this identity",
      tone: manager ? "accent" : profile.system ? "good" : "muted",
    },
    {
      label: "wake",
      value: wakeValue,
      detail: waitingMessages > 0 ? `${wakeDetail} · ${waitingMessages} mailbox waiting` : wakeDetail,
      tone: profile.retired ? "muted" : wakeIssue || profile.enabled === false ? "warn" : running ? "accent" : waitingMessages > 0 ? "warn" : "good",
    },
    {
      label: "soul",
      value: soul ? "soul set" : "soul missing",
      detail: soul ? soul.slice(0, 180) : "durable identity prompt is missing",
      tone: soul ? "good" : "warn",
    },
    {
      label: "contract",
      value: lifecycle.value,
      detail: `${contract} · ${taskProgress}`,
      tone: lifecycle.tone,
    },
    {
      label: "authority",
      value: capability,
      detail: `${capability} · ${privateSkillCount > 0 ? `${privateSkillCount} private skill${privateSkillCount === 1 ? "" : "s"}` : "no private skills"}`,
      tone: capability.includes("open high-impact") || capability.includes("high-impact allow") ? "warn" : "good",
    },
  ];
}

export interface AgentCommandStripItem {
  label: string;
  value: string;
  detail?: string;
  tone: "good" | "warn" | "bad" | "accent" | "muted";
}

export function agentCommandStrip(
  profile: Pick<AgentProfile, "retired" | "enabled" | "kind" | "managed" | "direct_callable" | "parent_agent" | "owner_agent" | "memory_scope" | "workdir" | "tool_allow" | "tool_deny" | "trust_ceiling" | "retry_policy" | "health_policy" | "self_repair" | "model" | "fallbacks" | "task_type"> & { slug?: string },
  runtime: Pick<AgentCardRuntimeSummary, "activeRunCount" | "activePhase" | "wakeText" | "wakeDetail" | "nextWakeMs" | "repairText" | "repairDetail">,
  mailbox: { value: string; detail: string; tone: "good" | "warn" | "accent" | "muted" },
  schedule: { detail: string; tone: "good" | "warn" | "muted" },
  capabilitySummary: string,
  resource: { detail: string; tone: "good" | "warn" | "muted" },
  wakeIssue = "",
  now = Date.now(),
): AgentCommandStripItem[] {
  const owner = profile.parent_agent || profile.owner_agent || "";
  const wakeRoute = agentWakeRoutePassport(profile);
  const wakeValue = profile.retired
    ? "graveyard"
    : runtime.activeRunCount > 0
      ? runtime.activePhase || "awake"
      : runtime.nextWakeMs
        ? formatWakeDue(runtime.nextWakeMs, now)
        : runtime.wakeText || (wakeIssue ? wakeRoute.value : "manual wake");
  const wakeDetail = profile.retired
    ? "revive before any wake source can run this identity"
    : runtime.wakeDetail || (wakeIssue ? `${wakeIssue} · ${wakeRoute.detail}` : wakeRoute.detail) || (owner ? `managed by ${owner}` : "manual, schedule, mailbox, or delegation can wake this identity when allowed");
  const repair = agentRepairGovernancePassport(profile);
  const route = agentModelRoutePassport(profile);
  return [
    {
      label: "wake",
      value: wakeValue,
      detail: wakeDetail,
      tone: profile.retired ? "muted" : runtime.activeRunCount > 0 ? "accent" : wakeIssue ? "warn" : runtime.nextWakeMs || runtime.wakeText ? "good" : wakeRoute.tone,
    },
    {
      label: "mailbox",
      value: mailbox.value,
      detail: mailbox.detail,
      tone: mailbox.tone,
    },
    {
      label: "schedule",
      value: schedule.detail,
      detail: schedule.detail,
      tone: schedule.tone,
    },
    {
      label: "route",
      value: route.value,
      detail: route.detail,
      tone: route.tone,
    },
    {
      label: "authority",
      value: capabilitySummary,
      detail: capabilitySummary,
      tone: (profile.tool_allow || []).length > 0 || (profile.tool_deny || []).length > 0 || (profile.trust_ceiling && profile.trust_ceiling !== "L4") ? "good" : "warn",
    },
    {
      label: "resources",
      value: resource.detail,
      detail: resource.detail,
      tone: resource.tone,
    },
    {
      label: "repair",
      value: runtime.repairText || repair.value,
      detail: runtime.repairDetail || repair.detail,
      tone: runtime.repairText ? "accent" : repair.tone,
    },
  ];
}

export function rosterWaitingMailboxCounts(messages: RosterBoardMessage[], agents: string[] = []): Record<string, number> {
  const answered = new Set(
    messages
      .filter((m) => m.reply_to)
      .map((m) => String(m.reply_to || "").trim())
      .filter(Boolean),
  );
  const broadcastReplies = new Map<string, Set<string>>();
  for (const m of messages) {
    const replyTo = String(m.reply_to || "").trim();
    const from = String(m.from || "").trim().toLowerCase();
    if (!replyTo || !from) continue;
    if (!broadcastReplies.has(replyTo)) broadcastReplies.set(replyTo, new Set());
    broadcastReplies.get(replyTo)?.add(from);
  }
  const roster = agents.map((a) => a.trim().toLowerCase()).filter(Boolean);
  const counts: Record<string, number> = {};
  for (const m of messages) {
    const id = String(m.id || "").trim();
    if (!id || m.reply_to) continue;
    const to = String(m.to || "").trim().toLowerCase();
    const from = String(m.from || "").trim().toLowerCase();
    const acked = new Set((m.acked_by || []).map((a) => a.trim().toLowerCase()).filter(Boolean));
    if (to && to !== "*" && !answered.has(id) && !acked.has(to)) counts[to] = (counts[to] || 0) + 1;
    if (to === "*") {
      const replied = broadcastReplies.get(id) || new Set<string>();
      for (const agent of roster) {
        if (!agent || agent === from || acked.has(agent) || replied.has(agent)) continue;
        counts[agent] = (counts[agent] || 0) + 1;
      }
    }
  }
  return counts;
}

export function rosterMailboxPassport(
  slug: string,
  waiting = 0,
  orders: Pick<ApiOrder, "enabled" | "triggers">[] = [],
): { value: string; detail: string; tone: "good" | "warn" | "muted" } {
  const s = slug.trim().toLowerCase();
  const subjects = [
    { label: "DM", subject: `board.dm.${s}` },
    { label: "Help", subject: `board.help.${s}` },
    { label: "Broadcast", subject: "board.broadcast" },
  ];
  const armed = subjects
    .filter((row) =>
      orders.some((o) =>
        o.enabled !== false &&
        (o.triggers || []).some((t) => (t.type || "").toLowerCase() === "event" && (t.subject || "").trim().toLowerCase() === row.subject),
      ),
    )
    .map((row) => row.label);
  const wakeDetail = armed.length > 0
    ? `wake subjects armed: ${armed.join(", ")}`
    : waiting > 0
      ? "no mailbox wake subjects armed; manual wake required"
      : "no mailbox wake subjects armed";
  return {
    value: waiting > 0
      ? `inbox ${waiting} waiting`
      : armed.length > 0
        ? `mailbox armed · ${armed.join(", ")}`
        : "mailbox manual",
    detail: [
      `${waiting} waiting message${waiting === 1 ? "" : "s"}`,
      wakeDetail,
    ].join(" · "),
    tone: waiting > 0 ? "warn" : armed.length > 0 ? "good" : "muted",
  };
}

// slugOk mirrors the kernel's roster slug rule (lowercase, digit/letter first,
// then letters/digits/dot/dash/underscore, ≤64) so the form can validate before
// the round-trip. Pure + unit-tested.
export function slugOk(s: string): boolean {
  return /^[a-z0-9][a-z0-9._-]{0,63}$/.test(s);
}

// agentHue maps a slug to a stable hue (0–359) so every agent gets a consistent
// colored identity avatar across the UI. The deterministic hue + monogram now
// live in @/lib/agent (M948) so the avatar can be shared; re-exported here for
// existing importers.
import { agentHue, initials } from "@/lib/agent";
export { agentHue, initials };

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

export function sortAgentRoster(profiles: AgentProfile[]): AgentProfile[] {
  return [...profiles].sort((a, b) => {
    const ar = agentRosterRank(a);
    const br = agentRosterRank(b);
    for (let i = 0; i < ar.length; i++) {
      if (ar[i] !== br[i]) return ar[i] < br[i] ? -1 : 1;
    }
    return a.slug.localeCompare(b.slug);
  });
}

export function agentIdentityKind(profile: Pick<AgentProfile, "system" | "kind" | "managed" | "direct_callable">): "system" | "custom" | "subagent" {
  if (profile.system || profile.kind === "system") return "system";
  if (profile.kind === "subagent" || profile.managed || profile.direct_callable === false) return "subagent";
  return "custom";
}

export type RosterFilter = "all" | "attention" | "direct" | "subagents" | "system" | "repair" | "mailbox" | "graveyard" | "paused";

export function agentNeedsRepair(profile: Pick<AgentProfile, "status">): boolean {
  const status = profile.status;
  const health = (status?.health_state || "").toLowerCase();
  const repair = (status?.repair_state || "").toLowerCase();
  return (
    (status?.repair_inflight || 0) > 0 ||
    (status?.retry_count || 0) > 0 ||
    repair === "queued" ||
    repair === "failed" ||
    repair === "attempts_exhausted" ||
    health === "degraded" ||
    health === "misconfigured" ||
    health === "unstable" ||
    health === "force_failed" ||
    health === "force_exhausted"
  );
}

export function agentNeedsAttention(
  profile: AgentProfile,
  mailboxCounts: Record<string, number> = {},
  schedulePressure: Record<string, ReturnType<typeof agentSchedulePressurePassport>> = {},
): boolean {
  if (profile.retired) return false;
  const mailbox = mailboxCounts[profile.slug.toLowerCase()] || mailboxCounts[profile.slug] || 0;
  const pressure = schedulePressure[profile.slug];
  return (
    agentNeedsRepair(profile) ||
    mailbox > 0 ||
    systemGuardianSafetySummary(profile).startsWith("review:") ||
    !!pressure?.frequent
  );
}

export function filterAgentRoster(
  profiles: AgentProfile[],
  filter: RosterFilter,
  mailboxCounts: Record<string, number> = {},
  schedulePressure: Record<string, ReturnType<typeof agentSchedulePressurePassport>> = {},
): AgentProfile[] {
  if (filter === "all") return profiles;
  return profiles.filter((p) => {
    if (filter === "attention") return agentNeedsAttention(p, mailboxCounts, schedulePressure);
    if (filter === "direct") return !p.retired && agentIdentityKind(p) === "custom";
    if (filter === "subagents") return !p.retired && agentIdentityKind(p) === "subagent";
    if (filter === "system") return !p.retired && agentIdentityKind(p) === "system";
    if (filter === "repair") return !p.retired && agentNeedsRepair(p);
    if (filter === "mailbox") return !p.retired && (mailboxCounts[p.slug.toLowerCase()] || mailboxCounts[p.slug] || 0) > 0;
    if (filter === "graveyard") return !!p.retired;
    if (filter === "paused") return p.enabled === false && !p.retired;
    return true;
  });
}

function agentRosterRank(p: AgentProfile): [number, number, number] {
  const lifecycle = p.retired ? 4 : p.enabled === false ? 3 : 0;
  const identity = agentIdentityKind(p);
  const kind = identity === "system" ? 0 : identity === "subagent" ? 2 : 1;
  return [lifecycle, kind, p.slug ? 0 : 1];
}

function nonNegativeInt(s: string, label: string): number | string {
  const t = s.trim();
  if (t === "") return 0;
  const n = Number(t);
  if (!Number.isInteger(n) || n < 0) return `${label} must be a non-negative integer`;
  return n;
}

const inputCls =
  "rounded-md border border-border bg-panel px-2 py-1 text-sm text-foreground outline-none focus-visible:border-accent";

// profileFields collects the shared New/Edit form fields into the wire shape.
// Exported for unit tests (the model/fallbacks mapping, incl. the @chain
// self-contained rule, is the meaningful logic).
export function profileFields(f: {
  name: string;
  soul: string;
  instructions: string;
  model: string;
  fallbacks: string;
  taskType: string;
  maxCost: string;
  maxDaily: string;
  memoryScope: string;
  workdir: string;
  ownerAgent: string;
  parentAgent: string;
  directCallable: string;
  retryAttempts: string;
  retryBackoff: string;
  retryBaseDelay: string;
  retryMaxDelay: string;
  retryOn?: string;
  healthDoctor: string;
  healthFailureThreshold: string;
  selfRepairEnabled: string;
  selfRepairAttempts: string;
  selfRepairEscalate: string;
  trustCeiling: string;
  toolAllow: string;
  toolDeny: string;
  configOverrides: string;
  lifecycleMode: string;
  lifecycleMaxCycles: string;
  cycleTasks: string;
  totalTasks: string;
  description: string;
}): Record<string, unknown> | string {
  const mc = usdToMc(f.maxCost);
  if (mc === null) return "max cost must be a dollar amount like 0.50";
  const dailyMc = usdToMc(f.maxDaily);
  if (dailyMc === null) return "max daily must be a dollar amount like 5.00";
  const retryAttempts = nonNegativeInt(f.retryAttempts, "retry attempts");
  if (typeof retryAttempts === "string") return retryAttempts;
  const retryBaseDelay = nonNegativeInt(f.retryBaseDelay, "retry base delay");
  if (typeof retryBaseDelay === "string") return retryBaseDelay;
  const retryMaxDelay = nonNegativeInt(f.retryMaxDelay, "retry max delay");
  if (typeof retryMaxDelay === "string") return retryMaxDelay;
  const healthFailureThreshold = nonNegativeInt(f.healthFailureThreshold, "health failure threshold");
  if (typeof healthFailureThreshold === "string") return healthFailureThreshold;
  const selfRepairAttempts = nonNegativeInt(f.selfRepairAttempts, "self-repair attempts");
  if (typeof selfRepairAttempts === "string") return selfRepairAttempts;
  const lifecycleMaxCycles = nonNegativeInt(f.lifecycleMaxCycles, "max cycles");
  if (typeof lifecycleMaxCycles === "string") return lifecycleMaxCycles;
  if (f.directCallable === "false" && !f.ownerAgent.trim() && !f.parentAgent.trim()) {
    return "managed sub-agent needs an owner or parent agent";
  }
  const p: Record<string, unknown> = {
    name: f.name.trim(),
    soul: f.soul.trim(),
    instructions: lines(f.instructions),
    model: f.model.trim(),
    task_type: f.taskType.trim(),
    max_cost_mc: mc,
    max_daily_mc: dailyMc,
    memory_scope: f.memoryScope.trim(),
    workdir: f.workdir.trim(),
    owner_agent: f.ownerAgent.trim(),
    parent_agent: f.parentAgent.trim(),
    direct_callable: f.directCallable !== "false",
    description: f.description.trim(),
    trust_ceiling: f.trustCeiling.trim(),
    tool_allow: csvList(f.toolAllow),
    tool_deny: csvList(f.toolDeny),
    config_overrides: configMap(f.configOverrides),
  };
  const tasklist = [
    ...taskLines(f.cycleTasks, "cycle"),
    ...taskLines(f.totalTasks, "total"),
  ];
  if (tasklist.length > 0) p.tasklist = tasklist;
  const lifecycleMode = f.lifecycleMode.trim() || "persistent";
  const effectiveLifecycleMode = lifecycleMode === "persistent" && lifecycleMaxCycles > 0 ? "cycle" : lifecycleMode;
  if (f.lifecycleMode.trim() || lifecycleMaxCycles) {
    p.lifecycle = {
      mode: effectiveLifecycleMode,
      retire_on_complete: effectiveLifecycleMode === "retire_on_complete",
      max_cycles: lifecycleMaxCycles,
    };
  }
  const retryOn = csvList(f.retryOn || "");
  if (retryAttempts || retryBaseDelay || retryMaxDelay || f.retryBackoff.trim() || retryOn.length > 0) {
    p.retry_policy = {
      max_attempts: retryAttempts,
      backoff: f.retryBackoff.trim() || "exponential",
      base_delay_sec: retryBaseDelay,
      max_delay_sec: retryMaxDelay,
      retry_on: retryOn,
    };
  }
  if (f.healthDoctor.trim() || healthFailureThreshold) {
    p.health_policy = {
      doctor_agent: f.healthDoctor.trim(),
      failure_threshold: healthFailureThreshold,
    };
  }
  if (f.selfRepairEnabled === "true" || selfRepairAttempts || f.selfRepairEscalate.trim()) {
    p.self_repair = {
      enabled: f.selfRepairEnabled === "true",
      max_attempts: selfRepairAttempts,
      escalate_to: f.selfRepairEscalate.trim(),
    };
  }
  // A "@chain" model is self-contained — its fallback ladder lives in the chain,
  // so per-agent fallbacks are ignored (and the form hides the field).
  if (!isChainRef(p.model as string)) {
    const fb = f.fallbacks
      .split(",")
      .map((m) => m.trim())
      .filter(Boolean);
    if (fb.length > 0) p.fallbacks = fb;
  }
  return p;
}

function lines(s: string): string[] {
  return s
    .split(/\r?\n/)
    .map((x) => x.trim())
    .filter(Boolean);
}

function csvList(s: string): string[] {
  const seen = new Set<string>();
  const out: string[] = [];
  for (const item of s
    .split(/[,\r\n]+/)
    .map((x) => x.trim())
    .filter(Boolean)) {
    const key = item.toLowerCase();
    if (seen.has(key)) continue;
    seen.add(key);
    out.push(item);
  }
  return out;
}

function configMap(s: string): Record<string, string> {
  const out: Record<string, string> = {};
  for (const raw of s.split(/\r?\n/)) {
    const line = raw.trim();
    if (!line) continue;
    const eq = line.indexOf("=");
    if (eq < 0) {
      out[line] = "";
      continue;
    }
    out[line.slice(0, eq).trim()] = line.slice(eq + 1).trim();
  }
  return out;
}

function configText(m?: Record<string, string>): string {
  if (!m) return "";
  return Object.entries(m)
    .sort(([a], [b]) => a.localeCompare(b))
    .map(([k, v]) => `${k}=${v}`)
    .join("\n");
}

function taskLines(s: string, scope: "cycle" | "total"): AgentTask[] {
  return lines(s).map((title) => ({ title, scope, status: "todo" }));
}

function tasksText(tasks: AgentTask[] | undefined, scope: "cycle" | "total"): string {
  return (tasks || [])
    .filter((t) => (t.scope || "total") === scope && t.status !== "retired")
    .map((t) => t.title)
    .join("\n");
}

// AgentFormFields renders the shared editable fields for New/Edit.
function AgentFormFields(props: {
  state: Record<string, string>;
  set: (key: string, value: string) => void;
}) {
  const { state, set } = props;
  const field = (label: string, key: string, placeholder: string, aria: string) => (
    <label className="flex flex-col gap-1 text-[11px] text-muted">
      {label}
      <input
        value={state[key] || ""}
        onChange={(e) => set(key, e.target.value)}
        placeholder={placeholder}
        aria-label={aria}
        className={inputCls}
      />
    </label>
  );
  const modelIsChain = isChainRef(state.model || "");
  const lifecycleMode = state.lifecycleMode || "persistent";
  const maxCycles = Number.parseInt(state.lifecycleMaxCycles || "0", 10);
  const effectiveLifecycleMode = lifecycleMode === "persistent" && Number.isFinite(maxCycles) && maxCycles > 0 ? "cycle" : lifecycleMode;
  const taskContractPreview = agentTaskContractSummary({
    lifecycle: {
      mode: effectiveLifecycleMode as AgentLifecycle["mode"],
      retire_on_complete: effectiveLifecycleMode === "retire_on_complete",
      max_cycles: Number.isFinite(maxCycles) && maxCycles > 0 ? maxCycles : 0,
    },
    tasklist: [
      ...taskLines(state.cycleTasks || "", "cycle"),
      ...taskLines(state.totalTasks || "", "total"),
    ],
  });
  return (
    <>
      <div className="grid gap-2 sm:grid-cols-2">
        {field("Name", "name", "e.g. The Researcher", "Agent name")}
        <label className="flex flex-col gap-1 text-[11px] text-muted">
          Model
          <div className="flex h-[30px] items-center">
            <ModelPicker value={state.model || ""} activeModel="daemon default" onChange={(id) => set("model", id)} />
          </div>
          {modelIsChain && <span className="text-[10px] text-accent">chain is self-contained — fallbacks come from @{state.model.slice(1)}</span>}
        </label>
        {modelIsChain
          ? null
          : field("Fallback models (comma-separated)", "fallbacks", "m1, m2", "Fallback models")}
        {field("Task type", "taskType", "e.g. research, code", "Task type")}
        {field("Max cost per run (USD)", "maxCost", "e.g. 0.50 — blank = no cap", "Max cost per run")}
        {field("Max cost per day (USD)", "maxDaily", "e.g. 5.00 — blank = no cap", "Max cost per day")}
        {field("Memory scope", "memoryScope", "blank = the slug", "Memory scope")}
        {field("Workdir (workspace-relative)", "workdir", "e.g. research", "Workdir")}
        {field("Owner agent", "ownerAgent", "supervisor slug", "Owner agent")}
        {field("Parent agent", "parentAgent", "leader slug for managed workers", "Parent agent")}
        <label className="flex flex-col gap-1 text-[11px] text-muted">
          Direct call
          <select
            value={state.directCallable || "true"}
            onChange={(e) => set("directCallable", e.target.value)}
            aria-label="Direct call policy"
            className={inputCls}
          >
            <option value="true">Directly callable</option>
            <option value="false">Managed sub-agent only</option>
          </select>
        </label>
        <label className="flex flex-col gap-1 text-[11px] text-muted">
          Lifecycle
          <select
            value={state.lifecycleMode || "persistent"}
            onChange={(e) => set("lifecycleMode", e.target.value)}
            aria-label="Agent lifecycle"
            className={inputCls}
          >
            <option value="persistent">persistent</option>
            <option value="cycle">cycle agent</option>
            <option value="retire_on_complete">retire on completion</option>
          </select>
        </label>
        {field("Max cycles", "lifecycleMaxCycles", "0 = unlimited", "Max cycles")}
        <label className="flex flex-col gap-1 text-[11px] text-muted">
          Trust ceiling
          <select
            value={state.trustCeiling || "L4"}
            onChange={(e) => set("trustCeiling", e.target.value)}
            aria-label="Trust ceiling"
            className={inputCls}
          >
            <option value="L4">L4 allow</option>
            <option value="L3">L3 ask scoped</option>
            <option value="L2">L2 ask first</option>
            <option value="L1">L1 ask always</option>
            <option value="L0">L0 deny</option>
          </select>
        </label>
        {field("Description", "description", "what this agent is for", "Description")}
      </div>
      <Advanced label="Dayanıklılık & onarım" className="mt-2">
      <div className="grid gap-2 sm:grid-cols-3">
        {field("Retry attempts", "retryAttempts", "0 = no run retry", "Retry attempts")}
        <label className="flex flex-col gap-1 text-[11px] text-muted">
          Retry backoff
          <select
            value={state.retryBackoff || "exponential"}
            onChange={(e) => set("retryBackoff", e.target.value)}
            aria-label="Retry backoff"
            className={inputCls}
          >
            <option value="exponential">exponential</option>
            <option value="fixed">fixed</option>
          </select>
        </label>
        {field("Retry base delay (sec)", "retryBaseDelay", "e.g. 30", "Retry base delay")}
        {field("Retry max delay (sec)", "retryMaxDelay", "e.g. 1800", "Retry max delay")}
        {field("Retry on", "retryOn", "error, timeout", "Retry on")}
        {field("Doctor agent", "healthDoctor", "guardian-doctor", "Doctor agent")}
        {field("Failure threshold", "healthFailureThreshold", "e.g. 5", "Failure threshold")}
        <label className="flex flex-col gap-1 text-[11px] text-muted">
          Self repair
          <select
            value={state.selfRepairEnabled || "false"}
            onChange={(e) => set("selfRepairEnabled", e.target.value)}
            aria-label="Self repair"
            className={inputCls}
          >
            <option value="false">off</option>
            <option value="true">enabled</option>
          </select>
        </label>
        {field("Self-repair attempts", "selfRepairAttempts", "e.g. 2", "Self-repair attempts")}
        {field("Escalate to", "selfRepairEscalate", "owner/agent slug", "Escalate to")}
      </div>
      </Advanced>
      <label className="mt-2 flex flex-col gap-1 text-[11px] text-muted">
        Soul — who this agent is (identity core)
        <textarea
          value={state.soul || ""}
          onChange={(e) => set("soul", e.target.value)}
          placeholder="You are Researcher. You dig deep and cite sources."
          aria-label="Agent soul"
          rows={3}
          className={cn(inputCls, "resize-y")}
        />
      </label>
      <label className="mt-2 flex flex-col gap-1 text-[11px] text-muted">
        Standing instructions — durable operating rules
        <textarea
          value={state.instructions || ""}
          onChange={(e) => set("instructions", e.target.value)}
          placeholder="One instruction per line"
          aria-label="Agent instructions"
          rows={3}
          className={cn(inputCls, "resize-y")}
        />
      </label>
      <Advanced label="Araçlar, görevler & geçersiz kılmalar" className="mt-2">
      <div className="grid gap-2 sm:grid-cols-2">
        <label className="flex flex-col gap-1 text-[11px] text-muted">
          Tool allowlist
          <textarea
            value={state.toolAllow || ""}
            onChange={(e) => set("toolAllow", e.target.value)}
            placeholder="shell, memory, mcp_fake_greet"
            aria-label="Tool allowlist"
            rows={2}
            className={cn(inputCls, "resize-y")}
          />
        </label>
        <label className="flex flex-col gap-1 text-[11px] text-muted">
          Tool denylist
          <textarea
            value={state.toolDeny || ""}
            onChange={(e) => set("toolDeny", e.target.value)}
            placeholder="notify, shell"
            aria-label="Tool denylist"
            rows={2}
            className={cn(inputCls, "resize-y")}
          />
        </label>
      </div>
      <label className="mt-2 flex flex-col gap-1 text-[11px] text-muted">
        Agent config overrides
        <textarea
          value={state.configOverrides || ""}
          onChange={(e) => set("configOverrides", e.target.value)}
          placeholder={"AGEZT_X_MODE=agent-only\nAGEZT_X_BATCH=8"}
          aria-label="Agent config overrides"
          rows={3}
          className={cn(inputCls, "resize-y font-mono text-xs")}
        />
      </label>
      <div className="mt-2 grid gap-2 sm:grid-cols-2">
        <label className="flex flex-col gap-1 text-[11px] text-muted">
          Every-cycle tasks
          <textarea
            value={state.cycleTasks || ""}
            onChange={(e) => set("cycleTasks", e.target.value)}
            placeholder="One task per line"
            aria-label="Every-cycle tasks"
            rows={3}
            className={cn(inputCls, "resize-y")}
          />
        </label>
        <label className="flex flex-col gap-1 text-[11px] text-muted">
          Total tasklist
          <textarea
            value={state.totalTasks || ""}
            onChange={(e) => set("totalTasks", e.target.value)}
            placeholder="One task per line"
            aria-label="Total tasklist"
            rows={3}
            className={cn(inputCls, "resize-y")}
          />
        </label>
      </div>
      <div
        className="mt-2 rounded-md border border-border bg-panel/60 px-2.5 py-2 text-xs text-muted"
        aria-label="Task contract preview"
      >
        <div className="mb-0.5 text-[10px] font-semibold uppercase tracking-wider text-muted">
          Task contract
        </div>
        <div className="text-foreground/85">{taskContractPreview}</div>
      </div>
      </Advanced>
    </>
  );
}

// NewAgentForm creates a roster profile (M785). Exported for tests and reuse
// (the M714 "creatable from UI" recipe).
export function NewAgentForm({
  onCreated,
  onError,
}: {
  onCreated: (slug: string) => void;
  onError: (msg: string) => void;
}) {
  const [state, setState] = useState<Record<string, string>>({});
  const [submitting, setSubmitting] = useState(false);
  const set = (k: string, v: string) => setState((s) => ({ ...s, [k]: v }));
  const slug = (state.slug || "").trim();
  const valid = slugOk(slug);

  async function create() {
    if (!valid) return;
    const fields = profileFields({
      name: state.name || "",
      soul: state.soul || "",
      instructions: state.instructions || "",
      model: state.model || "",
      fallbacks: state.fallbacks || "",
      taskType: state.taskType || "",
      maxCost: state.maxCost || "",
      maxDaily: state.maxDaily || "",
      memoryScope: state.memoryScope || "",
      workdir: state.workdir || "",
      ownerAgent: state.ownerAgent || "",
      parentAgent: state.parentAgent || "",
      directCallable: state.directCallable || "true",
      retryAttempts: state.retryAttempts || "",
      retryBackoff: state.retryBackoff || "",
      retryBaseDelay: state.retryBaseDelay || "",
      retryMaxDelay: state.retryMaxDelay || "",
      retryOn: state.retryOn || "",
      healthDoctor: state.healthDoctor || "",
      healthFailureThreshold: state.healthFailureThreshold || "",
      selfRepairEnabled: state.selfRepairEnabled || "",
      selfRepairAttempts: state.selfRepairAttempts || "",
      selfRepairEscalate: state.selfRepairEscalate || "",
      trustCeiling: state.trustCeiling || "L4",
      toolAllow: state.toolAllow || "",
      toolDeny: state.toolDeny || "",
      configOverrides: state.configOverrides || "",
      lifecycleMode: state.lifecycleMode || "",
      lifecycleMaxCycles: state.lifecycleMaxCycles || "",
      cycleTasks: state.cycleTasks || "",
      totalTasks: state.totalTasks || "",
      description: state.description || "",
    });
    if (typeof fields === "string") {
      onError(fields);
      return;
    }
    setSubmitting(true);
    try {
      await postJSON("/api/agents/add", { profile: { slug, ...fields } });
      onCreated(slug);
    } catch (e) {
      onError((e as Error).message);
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div className="glass glow-accent rounded-xl p-3">
      <label className="flex flex-col gap-1 text-[11px] text-muted">
        Slug — the agent's permanent handle (lowercase; cannot be changed later)
        <input
          value={state.slug || ""}
          onChange={(e) => set("slug", e.target.value)}
          placeholder="e.g. researcher"
          aria-label="Agent slug"
          className={cn(inputCls, slug !== "" && !valid && "border-bad")}
        />
      </label>
      <div className="mt-2">
        <AgentFormFields state={state} set={set} />
      </div>
      <div className="mt-3 flex justify-end">
        <Button size="sm" onClick={create} disabled={!valid || submitting}>
          <Plus className="h-3.5 w-3.5" /> Create agent
        </Button>
      </div>
    </div>
  );
}

// EditAgentForm edits a profile's mutable fields (the slug is the agent's
// address — immutable, shown but not editable).
export function EditAgentForm({
  profile,
  onSaved,
  onError,
}: {
  profile: AgentProfile;
  onSaved: (slug: string) => void;
  onError: (msg: string) => void;
}) {
  const [state, setState] = useState<Record<string, string>>({
    name: profile.name || "",
    soul: profile.soul || "",
    instructions: (profile.instructions || []).join("\n"),
    model: profile.model || "",
    fallbacks: (profile.fallbacks || []).join(", "),
    taskType: profile.task_type || "",
    maxCost: profile.max_cost_mc ? String(profile.max_cost_mc / 1e9) : "",
    maxDaily: profile.max_daily_mc ? String(profile.max_daily_mc / 1e9) : "",
    memoryScope: profile.memory_scope || "",
    workdir: profile.workdir || "",
    ownerAgent: profile.owner_agent || "",
    parentAgent: profile.parent_agent || "",
    directCallable: profile.direct_callable === false ? "false" : "true",
    retryAttempts: profile.retry_policy?.max_attempts ? String(profile.retry_policy.max_attempts) : "",
    retryBackoff: profile.retry_policy?.backoff || "exponential",
    retryBaseDelay: profile.retry_policy?.base_delay_sec ? String(profile.retry_policy.base_delay_sec) : "",
    retryMaxDelay: profile.retry_policy?.max_delay_sec ? String(profile.retry_policy.max_delay_sec) : "",
    retryOn: (profile.retry_policy?.retry_on || []).join(", "),
    healthDoctor: profile.health_policy?.doctor_agent || "",
    healthFailureThreshold: profile.health_policy?.failure_threshold ? String(profile.health_policy.failure_threshold) : "",
    selfRepairEnabled: profile.self_repair?.enabled ? "true" : "false",
    selfRepairAttempts: profile.self_repair?.max_attempts ? String(profile.self_repair.max_attempts) : "",
    selfRepairEscalate: profile.self_repair?.escalate_to || "",
    trustCeiling: profile.trust_ceiling || "L4",
    toolAllow: (profile.tool_allow || []).join(", "),
    toolDeny: (profile.tool_deny || []).join(", "),
    configOverrides: configText(profile.config_overrides),
    lifecycleMode: profile.lifecycle?.mode || (profile.lifecycle?.retire_on_complete ? "retire_on_complete" : "persistent"),
    lifecycleMaxCycles: profile.lifecycle?.max_cycles ? String(profile.lifecycle.max_cycles) : "",
    cycleTasks: tasksText(profile.tasklist, "cycle"),
    totalTasks: tasksText(profile.tasklist, "total"),
    description: profile.description || "",
  });
  const [submitting, setSubmitting] = useState(false);
  const set = (k: string, v: string) => setState((s) => ({ ...s, [k]: v }));

  async function save() {
    const fields = profileFields({
      name: state.name,
      soul: state.soul,
      instructions: state.instructions || "",
      model: state.model,
      fallbacks: state.fallbacks,
      taskType: state.taskType,
      maxCost: state.maxCost,
      maxDaily: state.maxDaily,
      memoryScope: state.memoryScope,
      workdir: state.workdir,
      ownerAgent: state.ownerAgent,
      parentAgent: state.parentAgent,
      directCallable: state.directCallable || "true",
      retryAttempts: state.retryAttempts || "",
      retryBackoff: state.retryBackoff || "",
      retryBaseDelay: state.retryBaseDelay || "",
      retryMaxDelay: state.retryMaxDelay || "",
      retryOn: state.retryOn || "",
      healthDoctor: state.healthDoctor || "",
      healthFailureThreshold: state.healthFailureThreshold || "",
      selfRepairEnabled: state.selfRepairEnabled || "",
      selfRepairAttempts: state.selfRepairAttempts || "",
      selfRepairEscalate: state.selfRepairEscalate || "",
      trustCeiling: state.trustCeiling || "L4",
      toolAllow: state.toolAllow || "",
      toolDeny: state.toolDeny || "",
      configOverrides: state.configOverrides || "",
      lifecycleMode: state.lifecycleMode || "",
      lifecycleMaxCycles: state.lifecycleMaxCycles || "",
      cycleTasks: state.cycleTasks || "",
      totalTasks: state.totalTasks || "",
      description: state.description,
    });
    if (typeof fields === "string") {
      onError(fields);
      return;
    }
    setSubmitting(true);
    try {
      await postJSON("/api/agents/edit", { ref: profile.slug, profile: fields });
      onSaved(profile.slug);
    } catch (e) {
      onError((e as Error).message);
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div className="glass glow-accent rounded-xl p-3">
      <div className="text-[11px] text-muted">
        Editing <span className="font-mono text-foreground">{profile.slug}</span> (slug is permanent)
      </div>
      <div className="mt-2">
        <AgentFormFields state={state} set={set} />
      </div>
      <div className="mt-3 flex justify-end">
        <Button size="sm" onClick={save} disabled={submitting}>
          <Pencil className="h-3.5 w-3.5" /> Save
        </Button>
      </div>
    </div>
  );
}

// Roster is the agent-identity console (M785): the durable, named agents
// (M783) — each with its own soul, model, cost ceiling, and memory scope —
// with create/edit/pause/resume/remove governance. Run one from chat or the
// CLI with `agt run --agent <slug>`; the lead delegates to them by name.
export function Roster() {
  const ui = useUI();
  const { subscribe } = useEvents();
  const [profiles, setProfiles] = useState<AgentProfile[] | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [busy, setBusy] = useState<string | null>(null);
  const [showForm, setShowForm] = useState(false);
  const [editing, setEditing] = useState<string | null>(null);
  const [activityFor, setActivityFor] = useState<string | null>(null);
  const [activityFocus, setActivityFocus] = useState<Record<string, string>>({});
  const [rosterFilter, setRosterFilter] = useState<RosterFilter>("all");
  const [retiring, setRetiring] = useState<{
    slug: string;
    reason: string;
    standing: string[];
    schedules: string[];
    memories: string[];
    authoredMemories: string[];
    skills: string[];
    configs: string[];
    workspaces: string[];
    workflowRefs: string[];
    mailboxMessages: string[];
    subagents: string[];
    subagentStanding: string[];
    subagentSchedules: string[];
    subagentMemories: string[];
    subagentAuthoredMemories: string[];
    subagentSkills: string[];
    subagentConfigs: string[];
    subagentWorkspaces: string[];
    subagentWorkflowRefs: string[];
    subagentMailboxMessages: string[];
  } | null>(null);
  const [removing, setRemoving] = useState<{
    slug: string;
    standing: string[];
    schedules: string[];
    memories: string[];
    authoredMemories: string[];
    skills: string[];
    configs: string[];
    workspaces: string[];
    workflowRefs: string[];
    mailboxMessages: string[];
    subagents: string[];
    subagentStanding: string[];
    subagentSchedules: string[];
    subagentMemories: string[];
    subagentAuthoredMemories: string[];
    subagentSkills: string[];
    subagentConfigs: string[];
    subagentWorkspaces: string[];
    subagentWorkflowRefs: string[];
    subagentMailboxMessages: string[];
    cascade: AgentRemovalPlanInput["cascade"];
  } | null>(null);
  // Per-agent private-skill counts (M943): how many skills each agent owns
  // (Skill.Agent == slug), so an operator sees who has learned what before
  // sharing/reassigning (M942) or exporting (`agt skill export --all --agent`).
  const [skillCounts, setSkillCounts] = useState<Record<string, number>>({});
  const [mailboxCounts, setMailboxCounts] = useState<Record<string, number>>({});
  const [standingOrders, setStandingOrders] = useState<ApiOrder[]>([]);
  const [schedulePressure, setSchedulePressure] = useState<Record<string, ReturnType<typeof agentSchedulePressurePassport>>>({});
  const [livePatches, setLivePatches] = useState<AgentLivePatchMap>({});
  // Keep the interval handle so a refresh-on-event nudge can coexist with the
  // poll without leaking timers (preserves the original useRef import).
  const pollRef = useRef<ReturnType<typeof setInterval> | null>(null);

  async function reload() {
    setLoading(true);
    try {
      const [d, sk, bd, sch, st] = await Promise.all([
        getJSON<{ profiles?: AgentProfile[] }>("/api/agents"),
        getJSON<{ skills?: { agent?: string }[] }>("/api/skills").catch(() => ({ skills: [] })),
        getJSON<{ messages?: RosterBoardMessage[] }>("/api/board").catch(() => ({ messages: [] })),
        getJSON<{ schedules?: RosterSchedule[] }>("/api/schedules").catch(() => ({ schedules: [] })),
        getJSON<{ orders?: ApiOrder[] }>("/api/standing").catch(() => ({ orders: [] })),
      ]);
      const nextProfiles = d.profiles || [];
      setProfiles(nextProfiles);
      const counts: Record<string, number> = {};
      for (const s of sk.skills || []) {
        if (s.agent) counts[s.agent] = (counts[s.agent] || 0) + 1;
      }
      setSkillCounts(counts);
      setMailboxCounts(rosterWaitingMailboxCounts(bd?.messages || [], nextProfiles.map((p) => p.slug)));
      setStandingOrders(st?.orders || []);
      setSchedulePressure(Object.fromEntries(nextProfiles.map((p) => [p.slug, agentSchedulePressurePassport(p, sch?.schedules || [])])));
      setErr(null);
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    reload();
    pollRef.current = setInterval(reload, 8000);
    return () => {
      if (pollRef.current) clearInterval(pollRef.current);
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    let t: ReturnType<typeof setTimeout> | undefined;
    const off = subscribe((ev) => {
      setLivePatches((prev) => reduceAgentLivePatchMap(prev, ev));
      if (!shouldReloadAgentCatalog(ev)) return;
      if (t) clearTimeout(t);
      t = setTimeout(() => void reload(), 1200);
    });
    return () => {
      if (t) clearTimeout(t);
      off();
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [subscribe]);

  async function act(
    ref: string,
    path: string,
    params?: Record<string, string>,
    opts?: { confirm?: ConfirmOptions; success?: string },
  ) {
    if (opts?.confirm && !(await ui.confirm(opts.confirm))) return;
    setBusy(ref);
    try {
      await postAction(path, { ref, ...params });
      if (opts?.success) ui.toast(opts.success, "success");
      await reload();
    } catch (e) {
      ui.toast((e as Error).message, "error");
    } finally {
      setBusy(null);
    }
  }

  // retire moves an agent to the graveyard (M846): fetch the impact first (which
  // standing orders fire it) and show it in the confirm, so the effects are
  // explicit before the agent is retired. Recoverable via Revive.
  async function retire(slug: string) {
    let impact: {
      standing_orders?: string[];
      schedules?: string[];
      memories?: string[];
      authored_shared_memories?: string[];
      skills?: string[];
      configs?: string[];
      workspaces?: string[];
      workflow_refs?: string[];
      mailbox_messages?: string[];
      subagents?: string[];
      subagent_standing_orders?: string[];
      subagent_schedules?: string[];
      subagent_memories?: string[];
      subagent_authored_shared_memories?: string[];
      subagent_skills?: string[];
      subagent_configs?: string[];
      subagent_workspaces?: string[];
      subagent_workflow_refs?: string[];
      subagent_mailbox_messages?: string[];
    } = {};
    try {
      impact = await getJSON("/api/agents/impact", { ref: slug });
    } catch {
      // Impact is advisory; proceed without it if the lookup fails.
    }
    setRetiring({
      slug,
      reason: "",
      standing: impact.standing_orders || [],
      schedules: impact.schedules || [],
      memories: impact.memories || [],
      authoredMemories: impact.authored_shared_memories || [],
      skills: impact.skills || [],
      configs: impact.configs || [],
      workspaces: impact.workspaces || [],
      workflowRefs: impact.workflow_refs || [],
      mailboxMessages: impact.mailbox_messages || [],
      subagents: impact.subagents || [],
      subagentStanding: impact.subagent_standing_orders || [],
      subagentSchedules: impact.subagent_schedules || [],
      subagentMemories: impact.subagent_memories || [],
      subagentAuthoredMemories: impact.subagent_authored_shared_memories || [],
      subagentSkills: impact.subagent_skills || [],
      subagentConfigs: impact.subagent_configs || [],
      subagentWorkspaces: impact.subagent_workspaces || [],
      subagentWorkflowRefs: impact.subagent_workflow_refs || [],
      subagentMailboxMessages: impact.subagent_mailbox_messages || [],
    });
  }

  async function confirmRetire() {
    if (!retiring) return;
    const { slug } = retiring;
    const reason = retiring.reason.trim();
    setRetiring(null);
    setBusy(slug);
    try {
      const res = await postAction<AgentRetireResult>(
        "/api/agents/retire",
        reason ? { ref: slug, reason } : { ref: slug },
      );
      ui.toast(agentRetireToast(slug, res), "success");
      await reload();
    } catch (e) {
      ui.toast((e as Error).message, "error");
    } finally {
      setBusy(null);
    }
  }

  async function revive(slug: string) {
    setBusy(slug);
    try {
      const res = await postAction<AgentReviveResult>("/api/agents/revive", { ref: slug });
      ui.toast(agentReviveToast(slug, res), "success");
      await reload();
    } catch (e) {
      ui.toast((e as Error).message, "error");
    } finally {
      setBusy(null);
    }
  }

  async function setAgentEnabled(slug: string, enabled: boolean) {
    setBusy(slug);
    try {
      const res = await postAction<AgentEnableResult>("/api/agents/enable", {
        ref: slug,
        enabled: enabled ? "true" : "false",
      });
      ui.toast(agentEnableToast(slug, enabled, res), "success");
      await reload();
    } catch (e) {
      ui.toast((e as Error).message, "error");
    } finally {
      setBusy(null);
    }
  }

  async function wakeAgent(slug: string) {
    setBusy(slug);
    try {
      const res = await postAction<{ correlation_id?: string }>("/api/agents/wake", { ref: slug, reason: "manual operator wake" });
      ui.toast(`${slug} wake queued`, "success");
      if (res?.correlation_id) setActivityFocus((prev) => ({ ...prev, [slug]: res.correlation_id || "" }));
      setActivityFor(slug);
      await reload();
    } catch (e) {
      ui.toast((e as Error).message, "error");
    } finally {
      setBusy(null);
    }
  }

  async function repairAgent(slug: string) {
    setBusy(slug);
    try {
      const res = await postJSON<{ correlation_id?: string }>("/api/agents/repair", {
        ref: slug,
        reason: `operator requested repair from ${slug} roster card`,
      });
      ui.toast(res?.correlation_id ? `${slug} repair accepted (${res.correlation_id})` : `${slug} repair accepted`, "success");
      if (res?.correlation_id) setActivityFocus((prev) => ({ ...prev, [slug]: res.correlation_id || "" }));
      setActivityFor(slug);
      await reload();
    } catch (e) {
      ui.toast((e as Error).message, "error");
    } finally {
      setBusy(null);
    }
  }

  async function removeAgent(slug: string) {
    let impact: {
      standing_orders?: string[];
      schedules?: string[];
      memories?: string[];
      authored_shared_memories?: string[];
      skills?: string[];
      configs?: string[];
      workspaces?: string[];
      workflow_refs?: string[];
      mailbox_messages?: string[];
      subagents?: string[];
      subagent_standing_orders?: string[];
      subagent_schedules?: string[];
      subagent_memories?: string[];
      subagent_authored_shared_memories?: string[];
      subagent_skills?: string[];
      subagent_configs?: string[];
      subagent_workspaces?: string[];
      subagent_workflow_refs?: string[];
      subagent_mailbox_messages?: string[];
    } = {};
    try {
      impact = await getJSON("/api/agents/impact", { ref: slug });
    } catch {
      // Impact is advisory; the remove call itself remains authoritative.
    }
    const standing = impact.standing_orders || [];
    const schedules = impact.schedules || [];
    const memories = impact.memories || [];
    const authoredMemories = impact.authored_shared_memories || [];
    const skills = impact.skills || [];
    const configs = impact.configs || [];
    const workspaces = impact.workspaces || [];
    const workflowRefs = impact.workflow_refs || [];
    const mailboxMessages = impact.mailbox_messages || [];
    const subagents = impact.subagents || [];
    const subagentStanding = impact.subagent_standing_orders || [];
    const subagentSchedules = impact.subagent_schedules || [];
    const subagentMemories = impact.subagent_memories || [];
    const subagentAuthoredMemories = impact.subagent_authored_shared_memories || [];
    const subagentSkills = impact.subagent_skills || [];
    const subagentConfigs = impact.subagent_configs || [];
    const subagentWorkspaces = impact.subagent_workspaces || [];
    const subagentWorkflowRefs = impact.subagent_workflow_refs || [];
    const subagentMailboxMessages = impact.subagent_mailbox_messages || [];
    const hasSubagents = subagents.length > 0;
    setRemoving({
      slug,
      standing,
      schedules,
      memories,
      authoredMemories,
      skills,
      configs,
      workspaces,
      workflowRefs,
      mailboxMessages,
      subagents,
      subagentStanding,
      subagentSchedules,
      subagentMemories,
      subagentAuthoredMemories,
      subagentSkills,
      subagentConfigs,
      subagentWorkspaces,
      subagentWorkflowRefs,
      subagentMailboxMessages,
      cascade: {
        standing: standing.length > 0,
        schedules: schedules.length > 0,
        memory: memories.length > 0 || (hasSubagents && subagentMemories.length > 0),
        authored_memory: false,
        skills: skills.length > 0 || (hasSubagents && subagentSkills.length > 0),
        config: configs.length > 0 || (hasSubagents && subagentConfigs.length > 0),
        workspace: workspaces.length > 0 || (hasSubagents && subagentWorkspaces.length > 0),
        subagents: hasSubagents,
      },
    });
  }

  async function confirmRemove() {
    if (!removing) return;
    const target = removing;
    setBusy(target.slug);
    try {
      const res = await postJSON<AgentRemoveResult>("/api/agents/remove", { ref: target.slug, cascade: target.cascade });
      setRemoving(null);
      ui.toast(
        agentRemoveToast(target.slug, res),
        res.removed ? "success" : "info",
      );
      await reload();
    } catch (e) {
      ui.toast((e as Error).message, "error");
    } finally {
      setBusy(null);
    }
  }

  async function quietNoisyGuardians() {
    const targets = noisySystemGuardians(list);
    const activeGuardians = list.filter((profile) => profile.system && !profile.retired);
    const frequentScheduleIds = Array.from(new Set(
      activeGuardians.flatMap((profile) => schedulePressure[profile.slug]?.frequentIds || []),
    ));
    if (targets.length === 0 && frequentScheduleIds.length === 0) return;
    setBusy("guardians");
    try {
      await Promise.all(
        targets.map((profile) =>
          postJSON("/api/agents/capabilities", guardianQuietPolicyPayload(profile)),
        ).concat(frequentScheduleIds.map((id) => postAction("/api/schedule/enable", { id, enabled: "false" }))),
      );
      ui.toast(
        `${targets.length} system guardian${targets.length === 1 ? "" : "s"} quieted${frequentScheduleIds.length > 0 ? `; ${frequentScheduleIds.length} frequent schedule${frequentScheduleIds.length === 1 ? "" : "s"} paused` : ""}`,
        "success",
      );
      await reload();
    } catch (e) {
      ui.toast((e as Error).message, "error");
    } finally {
      setBusy(null);
    }
  }

  async function pauseFrequentSchedules(slug: string, scheduleIds: string[]) {
    const ids = Array.from(new Set(scheduleIds.filter(Boolean)));
    if (ids.length === 0) return;
    if (!(await ui.confirm({
      title: `Pause frequent schedules for ${slug}?`,
      message: `${ids.length} schedule${ids.length === 1 ? "" : "s"} will stop waking this agent automatically. Manual wake, mailbox wake, and delegation remain available.`,
      confirmLabel: "Pause schedules",
      danger: false,
    }))) return;
    setBusy(`schedule:${slug}`);
    try {
      await Promise.all(ids.map((id) => postAction("/api/schedule/enable", { id, enabled: "false" })));
      ui.toast(`${ids.length} frequent schedule${ids.length === 1 ? "" : "s"} paused for ${slug}`, "success");
      await reload();
    } catch (e) {
      ui.toast((e as Error).message, "error");
    } finally {
      setBusy(null);
    }
  }

  const list = useMemo(() => sortAgentRoster(applyAgentLivePatches(profiles || [], livePatches)), [profiles, livePatches]);
  const shownList = useMemo(() => filterAgentRoster(list, rosterFilter, mailboxCounts, schedulePressure), [list, mailboxCounts, rosterFilter, schedulePressure]);
  const enabled = list.filter((p) => p.enabled && !p.retired).length;
  const paused = list.filter((p) => !p.enabled && !p.retired).length;
  const graveyard = list.filter((p) => p.retired).length;
  const direct = list.filter((p) => !p.retired && agentIdentityKind(p) === "custom").length;
  const subagents = list.filter((p) => !p.retired && agentIdentityKind(p) === "subagent").length;
  const system = list.filter((p) => !p.retired && agentIdentityKind(p) === "system").length;
  const repair = list.filter((p) => !p.retired && agentNeedsRepair(p)).length;
  const mailboxAgents = list.filter((p) => !p.retired && (mailboxCounts[p.slug.toLowerCase()] || mailboxCounts[p.slug] || 0) > 0).length;
  const mailboxBacklog = list.reduce((sum, p) => sum + (p.retired ? 0 : mailboxCounts[p.slug.toLowerCase()] || mailboxCounts[p.slug] || 0), 0);
  const attention = list.filter((p) => agentNeedsAttention(p, mailboxCounts, schedulePressure)).length;
  const guardianRisk = systemGuardianRiskSummary(list);
  const noisyGuardians = noisySystemGuardians(list);
  const guardianQuieting = guardianQuietingSummary(list, schedulePressure);
  const removalPlan = removing ? agentRemovalPlan(removing) : null;
  const removalDecision = removalPlan ? agentRemovalDecisionSummary(removalPlan) : null;
  const removalLifecycle = removing ? agentRemovalLifecycleSummary(removing) : null;
  const removalCustody = removing ? agentRemovalCustodySummary(removing) : null;
  const removalDeathCertificate = removing ? agentRemovalDeathCertificate(removing) : null;
  const removalLedger = removing ? agentRemovalLedger(removing) : [];
  const removalIncludesSubagents = !!removing?.cascade.subagents;
  const removalToggleItems = removing
    ? {
        standing: removalIncludesSubagents ? [...removing.standing, ...removing.subagentStanding] : removing.standing,
        schedules: removalIncludesSubagents ? [...removing.schedules, ...removing.subagentSchedules] : removing.schedules,
        memory: removalIncludesSubagents ? [...removing.memories, ...removing.subagentMemories] : removing.memories,
        authoredMemory: removalIncludesSubagents ? [...removing.authoredMemories, ...removing.subagentAuthoredMemories] : removing.authoredMemories,
        skills: removalIncludesSubagents ? [...removing.skills, ...removing.subagentSkills] : removing.skills,
        configs: removalIncludesSubagents ? [...removing.configs, ...removing.subagentConfigs] : removing.configs,
        workspaces: removalIncludesSubagents ? [...removing.workspaces, ...removing.subagentWorkspaces] : removing.workspaces,
      }
    : null;
  const graveyardStats = agentGraveyardStats(list);
  const graveyardCleanup = agentGraveyardCleanupPassport(list);
  const graveyardAgents = useMemo(
    () =>
      list
        .filter((p) => p.retired)
        .sort((a, b) => {
          const bm = b.retired_ms || 0;
          const am = a.retired_ms || 0;
          if (bm !== am) return bm - am;
          return a.slug.localeCompare(b.slug);
        })
        .slice(0, 6),
    [list],
  );

  return (
    <div className="space-y-3">
      <PageHeader
        icon={Users}
        title="Agent roster"
        description={
          profiles
            ? `${profiles.length} agent(s) · ${enabled} enabled`
            : "Durable, named agents — each with its own soul, model, budget, and memory scope."
        }
        actions={
          <>
            <Button size="sm" variant="ghost" onClick={reload} disabled={loading} aria-label="Refresh">
              <RefreshCw className={cn("h-3.5 w-3.5", loading && "animate-spin")} />
            </Button>
            {guardianQuieting.tone === "warn" && (
              <Button
                size="sm"
                variant="ghost"
                onClick={quietNoisyGuardians}
                disabled={busy === "guardians"}
                title="Apply quiet policy and pause frequent system guardian schedules"
                aria-label="Quiet noisy guardians"
              >
                <Megaphone className="h-3.5 w-3.5" />
                Quiet guardians
              </Button>
            )}
            <Button size="sm" onClick={() => setShowForm((v) => !v)}>
              {showForm ? <X className="h-3.5 w-3.5" /> : <Plus className="h-3.5 w-3.5" />}
              {showForm ? "Close" : "New agent"}
            </Button>
          </>
        }
      />

      {showForm && (
        <NewAgentForm
          onCreated={(slug) => {
            setShowForm(false);
            ui.toast(`agent ${slug} created`, "success");
            reload();
          }}
          onError={(msg) => ui.toast(msg, "error")}
        />
      )}

      {/* Summary band — the roster at a glance. */}
      {profiles && profiles.length > 0 && (
        <div className="grid grid-cols-2 gap-2 sm:grid-cols-4 xl:grid-cols-8">
          <RosterStat label="agents" value={list.length} />
          <RosterStat label="enabled" value={enabled} accent={enabled > 0} />
          <RosterStat label="paused" value={paused} />
          <RosterStat label="direct" value={direct} />
          <RosterStat label="sub-agents" value={subagents} />
          <RosterStat label="attention" value={attention} tone={attention > 0 ? "warn" : undefined} />
          <RosterStat label="repair" value={repair} tone={repair > 0 ? "warn" : undefined} />
          <RosterStat label="inbox" value={mailboxBacklog} tone={mailboxBacklog > 0 ? "warn" : undefined} />
          <RosterStat label="graveyard" value={graveyard} />
          {guardianRisk && <RosterStat label="guardians" value={guardianRisk} tone={guardianRisk.includes("review") ? "warn" : "accent"} />}
        </div>
      )}

      {guardianRisk && (
        <div
          className={cn(
            "rounded-lg border px-3 py-2 text-xs",
            guardianQuieting.tone === "warn"
              ? "border-warn/40 bg-warn/10"
              : guardianQuieting.tone === "good"
                ? "border-good/30 bg-good/5"
                : "border-border bg-card/55",
          )}
        >
          <div className={cn("font-medium", guardianQuieting.tone === "warn" ? "text-warn" : guardianQuieting.tone === "good" ? "text-good" : "text-muted")}>
            {guardianQuieting.label}
          </div>
          <div className="mt-0.5 text-muted">{guardianQuieting.detail}</div>
          {guardianQuieting.tone === "warn" && (
            <div className="mt-2 flex flex-wrap gap-1.5">
              <Badge variant="warn">{guardianQuieting.quietTargets} quiet target{guardianQuieting.quietTargets === 1 ? "" : "s"}</Badge>
              <Badge variant={guardianQuieting.frequentSchedules > 0 ? "warn" : "default"}>
                {guardianQuieting.frequentSchedules} frequent schedule{guardianQuieting.frequentSchedules === 1 ? "" : "s"}
              </Badge>
              {noisyGuardians.slice(0, 4).map((profile) => (
                <Badge key={profile.slug} variant="default">{profile.slug}</Badge>
              ))}
              {noisyGuardians.length > 4 && <Badge variant="default">+{noisyGuardians.length - 4}</Badge>}
            </div>
          )}
        </div>
      )}

      {profiles && profiles.length > 0 && (
        <div className="flex flex-wrap items-center gap-1.5">
          {([
            ["all", "All", list.length],
            ["attention", "Attention", attention],
            ["direct", "Direct", direct],
            ["subagents", "Sub-agents", subagents],
            ["system", "System", system],
            ["repair", "Repair", repair],
            ["mailbox", "Inbox", mailboxAgents],
            ["paused", "Paused", paused],
            ["graveyard", "Graveyard", graveyard],
          ] as [RosterFilter, string, number][]).map(([id, label, count]) => (
            <button
              key={id}
              onClick={() => setRosterFilter(id)}
              className={cn(
                "inline-flex items-center gap-1.5 rounded-full border px-2.5 py-1 text-xs transition-colors",
                rosterFilter === id ? "border-accent bg-accent/10 text-accent" : "border-border text-muted hover:border-accent",
              )}
            >
              {label}
              <span className="rounded-full bg-card px-1.5 text-[10px] tabular-nums">{count}</span>
            </button>
          ))}
        </div>
      )}

      {graveyardStats.count > 0 && (
        <section className="rounded-xl border border-border bg-card/45 p-3" aria-label="Agent graveyard">
          <div className="flex flex-wrap items-start gap-3">
            <div className="min-w-0 flex-1">
              <div className="flex items-center gap-1.5 text-[10px] font-semibold uppercase tracking-wider text-muted">
                <Skull className="h-3 w-3" /> Agent graveyard
              </div>
              <div className="mt-1 text-sm font-medium text-foreground">
                {graveyardStats.count} retired identit{graveyardStats.count === 1 ? "y" : "ies"}
              </div>
              <div className="mt-1 flex flex-wrap gap-x-3 gap-y-1 text-[11px] text-muted">
                <span>{graveyardStats.custom} custom</span>
                <span>{graveyardStats.subagents} sub-agent</span>
                <span>{graveyardStats.system} system</span>
                <span>{graveyardStats.withReason} with reason</span>
                <span>{graveyardCleanup.detail}</span>
                {graveyardStats.oldest?.retired_ms && (
                  <span>oldest: {graveyardStats.oldest.slug} · {fmtDateTime(graveyardStats.oldest.retired_ms)}</span>
                )}
              </div>
            </div>
            <div
              className={cn(
                "rounded-lg border px-2 py-1.5 text-xs",
                graveyardCleanup.tone === "warn"
                  ? "border-warn/40 bg-warn/10"
                  : graveyardCleanup.tone === "good"
                    ? "border-good/30 bg-good/5"
                    : "border-border bg-panel/50",
              )}
              title={graveyardCleanup.detail}
            >
              <div className={cn("text-[10px] font-semibold uppercase tracking-wider", graveyardCleanup.tone === "warn" ? "text-warn" : graveyardCleanup.tone === "good" ? "text-good" : "text-muted")}>
                Cleanup
              </div>
              <div className="mt-0.5 font-medium text-foreground/85">{graveyardCleanup.label}</div>
            </div>
            <Button size="sm" variant="ghost" onClick={() => setRosterFilter("graveyard")}>
              <Skull className="h-3.5 w-3.5" /> View graveyard
            </Button>
          </div>
          <div className="mt-2 grid gap-1.5 md:grid-cols-2 xl:grid-cols-3">
            {graveyardAgents.map((p) => (
              <div key={p.slug} className="min-w-0 rounded-lg border border-border bg-panel/45 p-2">
                <div className="flex min-w-0 items-start gap-2">
                  <AgentAvatar slug={p.slug} name={p.name} size={28} status="retired" />
                  <div className="min-w-0 flex-1">
                    <button
                      type="button"
                      onClick={() => openAgent(p.slug)}
                      className="truncate font-mono text-xs font-semibold text-foreground hover:underline"
                    >
                      {p.slug}
                    </button>
                    <div className="mt-0.5 truncate text-[11px] text-muted">
                      {p.retired_reason?.trim() || agentGraveyardSummary(p)}
                    </div>
                    <div className="mt-1 text-[10px] text-muted">
                      {agentIdentityKind(p)}{p.retired_ms ? ` · ${fmtDateTime(p.retired_ms)}` : ""}
                    </div>
                  </div>
                </div>
                <div className="mt-2 flex flex-wrap items-center gap-1">
                  <Button
                    size="sm"
                    variant="ghost"
                    aria-label={`Open graveyard identity ${p.slug}`}
                    onClick={() => openAgent(p.slug)}
                  >
                    <IdCard className="h-3.5 w-3.5" /> Open
                  </Button>
                  <Button
                    size="sm"
                    variant="ghost"
                    aria-label={`Revive from graveyard ${p.slug}`}
                    disabled={busy === p.slug}
                    onClick={() => revive(p.slug)}
                  >
                    <ArchiveRestore className="h-3.5 w-3.5" /> Revive
                  </Button>
                  {!p.system && (
                    <Button
                      size="sm"
                      variant="danger"
                      aria-label={`Remove from graveyard ${p.slug}`}
                      disabled={busy === p.slug}
                      onClick={() => removeAgent(p.slug)}
                    >
                      <Trash2 className="h-3.5 w-3.5" /> Remove
                    </Button>
                  )}
                </div>
              </div>
            ))}
          </div>
        </section>
      )}

      {retiring && (
        <div
          role="dialog"
          aria-labelledby="retire-agent-title"
          className="glass rounded-xl border-bad/40 p-3 shadow-e2"
        >
          <div className="flex flex-wrap items-start gap-3">
            <div className="flex min-w-0 flex-1 flex-col gap-1">
              <div id="retire-agent-title" className="text-sm font-semibold text-foreground">
                Retire {retiring.slug} to the graveyard
              </div>
              <p className="text-xs text-muted">
                The profile stays recoverable, but the agent is paused and excluded from delegation until revived.
              </p>
              {retiring.standing.length + retiring.schedules.length + retiring.memories.length + retiring.authoredMemories.length + retiring.skills.length + retiring.configs.length + retiring.workspaces.length + retiring.workflowRefs.length + retiring.mailboxMessages.length + retiring.subagents.length + retiring.subagentStanding.length + retiring.subagentSchedules.length + retiring.subagentMemories.length + retiring.subagentAuthoredMemories.length + retiring.subagentSkills.length + retiring.subagentConfigs.length + retiring.subagentWorkspaces.length + retiring.subagentWorkflowRefs.length + retiring.subagentMailboxMessages.length > 0 && (
                <div className="mt-2 rounded-lg border border-bad/30 bg-bad/5 p-2">
                  <div className="text-xs font-medium text-bad">Retirement impact</div>
                  <div className="mt-2 grid gap-2 md:grid-cols-2">
                    <ImpactList label="Standing orders" count={retiring.standing.length} items={retiring.standing} />
                    <ImpactList label="Schedules" count={retiring.schedules.length} items={retiring.schedules} />
                    <ImpactList label="Private memory" count={retiring.memories.length} items={retiring.memories} note="Kept inspectable; not deleted." />
                    <ImpactList label="Authored shared memory" count={retiring.authoredMemories.length} items={retiring.authoredMemories} note="Shared brain records are kept unless explicitly removed." />
                    <ImpactList label="Private skills" count={retiring.skills.length} items={retiring.skills} note="Kept inspectable; not archived." />
                    <ImpactList label="Agent config" count={retiring.configs.length} items={retiring.configs} note="Kept inspectable; remove can delete owned config entries." />
                    <ImpactList label="Workspace" count={retiring.workspaces.length} items={retiring.workspaces} note="Kept inspectable; remove can delete the agent workdir." />
                    <ImpactList label="Workflow references" count={retiring.workflowRefs.length} items={retiring.workflowRefs} note="Kept inspectable; workflows are reusable chains, not agent identities." />
                    <ImpactList label="Mailbox / audit history" count={retiring.mailboxMessages.length} items={retiring.mailboxMessages} note="Kept inspectable with the retired identity." />
                    <ImpactList label="Dependent sub-agent tree" count={retiring.subagents.length} items={retiring.subagents} note="Parent/owner links remain; remove can retire the full descendant tree." />
                    <ImpactList label="Sub-agent standing orders" count={retiring.subagentStanding.length} items={retiring.subagentStanding} note="Remove can clean these with standing + sub-agent cleanup." />
                    <ImpactList label="Sub-agent schedules" count={retiring.subagentSchedules.length} items={retiring.subagentSchedules} note="Remove can clean these with schedule + sub-agent cleanup." />
                    <ImpactList label="Sub-agent private memory" count={retiring.subagentMemories.length} items={retiring.subagentMemories} note="Remove can clean these with private memory + sub-agent cleanup." />
                    <ImpactList label="Sub-agent authored shared memory" count={retiring.subagentAuthoredMemories.length} items={retiring.subagentAuthoredMemories} note="Remove can clean these with authored shared memory + sub-agent cleanup." />
                    <ImpactList label="Sub-agent skills" count={retiring.subagentSkills.length} items={retiring.subagentSkills} note="Remove can clean these with private skills + sub-agent cleanup." />
                    <ImpactList label="Sub-agent config" count={retiring.subagentConfigs.length} items={retiring.subagentConfigs} note="Remove can clean these with agent config + sub-agent cleanup." />
                    <ImpactList label="Sub-agent workspace" count={retiring.subagentWorkspaces.length} items={retiring.subagentWorkspaces} note="Remove can clean these with workspace + sub-agent cleanup." />
                    <ImpactList label="Sub-agent workflow references" count={retiring.subagentWorkflowRefs.length} items={retiring.subagentWorkflowRefs} note="Kept with workflow graphs; inspect before removing the identity tree." />
                    <ImpactList label="Sub-agent mailbox / audit history" count={retiring.subagentMailboxMessages.length} items={retiring.subagentMailboxMessages} note="Kept inspectable with the dependent retired identities." />
                  </div>
                </div>
              )}
              <label className="mt-2 flex flex-col gap-1 text-[11px] text-muted">
                Retirement reason
                <textarea
                  aria-label="Retirement reason"
                  value={retiring.reason}
                  onChange={(e) => setRetiring((r) => (r ? { ...r, reason: e.target.value } : r))}
                  rows={2}
                  placeholder="why this identity is being retired"
                  className="rounded-md border border-border bg-panel px-2 py-1 text-sm text-foreground outline-none focus-visible:border-accent"
                />
              </label>
            </div>
            <div className="flex shrink-0 gap-2">
              <Button size="sm" variant="ghost" onClick={() => setRetiring(null)}>
                Cancel
              </Button>
              <Button size="sm" variant="danger" disabled={busy === retiring.slug} onClick={confirmRetire}>
                <Archive className="h-3.5 w-3.5" /> Retire
              </Button>
            </div>
          </div>
        </div>
      )}

      {removing && (
        <div role="dialog" aria-labelledby="remove-agent-title" className="glass rounded-xl border-bad/40 p-3 shadow-e2">
          <div className="flex flex-wrap items-start gap-3">
            <div className="min-w-0 flex-1">
              <div id="remove-agent-title" className="text-sm font-semibold text-foreground">
                Remove {removing.slug}
              </div>
              <p className="mt-1 text-xs text-muted">
                This deletes the identity profile. Select which private/owned resources should be cleaned up with it.
              </p>
              {removalDecision && (
                <div
                  className={cn(
                    "mt-3 rounded-lg border px-2 py-1.5 text-xs",
                    removalDecision.tone === "bad"
                      ? "border-bad/40 bg-bad/10"
                      : removalDecision.tone === "warn"
                        ? "border-warn/40 bg-warn/10"
                        : removalDecision.tone === "good"
                          ? "border-good/35 bg-good/5"
                          : "border-border bg-card/55",
                  )}
                >
                  <div
                    className={cn(
                      "font-medium",
                      removalDecision.tone === "bad"
                        ? "text-bad"
                        : removalDecision.tone === "warn"
                          ? "text-warn"
                          : removalDecision.tone === "good"
                            ? "text-good"
                            : "text-muted",
                    )}
                  >
                    {removalDecision.label}
                  </div>
                  <div className="mt-0.5 text-muted">{removalDecision.detail}</div>
                </div>
              )}
              {removalLifecycle && (
                <div
                  className={cn(
                    "mt-2 rounded-lg border px-2 py-1.5 text-xs",
                    removalLifecycle.tone === "bad"
                      ? "border-bad/40 bg-bad/10"
                      : removalLifecycle.tone === "warn"
                        ? "border-warn/40 bg-warn/10"
                        : removalLifecycle.tone === "good"
                          ? "border-good/35 bg-good/5"
                          : "border-border bg-card/55",
                  )}
                >
                  <div
                    className={cn(
                      "font-medium",
                      removalLifecycle.tone === "bad"
                        ? "text-bad"
                        : removalLifecycle.tone === "warn"
                          ? "text-warn"
                          : removalLifecycle.tone === "good"
                            ? "text-good"
                            : "text-muted",
                    )}
                  >
                    {removalLifecycle.label}
                  </div>
                  <div className="mt-0.5 text-muted">{removalLifecycle.detail}</div>
                </div>
              )}
              {removalCustody && (
                <div
                  className={cn(
                    "mt-2 rounded-lg border px-2 py-1.5 text-xs",
                    removalCustody.tone === "bad"
                      ? "border-bad/40 bg-bad/10"
                      : removalCustody.tone === "warn"
                        ? "border-warn/40 bg-warn/10"
                        : removalCustody.tone === "good"
                          ? "border-good/35 bg-good/5"
                          : "border-border bg-card/55",
                  )}
                >
                  <div
                    className={cn(
                      "font-medium",
                      removalCustody.tone === "bad"
                        ? "text-bad"
                        : removalCustody.tone === "warn"
                          ? "text-warn"
                          : removalCustody.tone === "good"
                            ? "text-good"
                            : "text-muted",
                    )}
                  >
                    {removalCustody.label}
                  </div>
                  <div className="mt-0.5 text-muted">{removalCustody.detail}</div>
                  <div className="mt-2 flex flex-wrap gap-1.5">
                    <Badge variant={removalCustody.deletedGroups > 0 ? "good" : "default"}>
                      {removalCustody.deletedGroups} delete group{removalCustody.deletedGroups === 1 ? "" : "s"}
                    </Badge>
                    <Badge variant={removalCustody.retainedGroups > 0 ? "warn" : "default"}>
                      {removalCustody.retainedGroups} operator retained
                    </Badge>
                    <Badge variant="default">
                      {removalCustody.hardRetainedGroups} audit/workflow retained
                    </Badge>
                    <Badge variant={removalCustody.subagentsRetired > 0 ? "warn" : "default"}>
                      {removalCustody.subagentsRetired} sub-agent retire{removalCustody.subagentsRetired === 1 ? "" : "s"}
                    </Badge>
                  </div>
                </div>
              )}
              {removalDeathCertificate && (
                <div
                  aria-label={`${removing.slug} death certificate`}
                  className={cn(
                    "mt-2 rounded-lg border px-2 py-1.5 text-xs",
                    removalDeathCertificate.tone === "bad"
                      ? "border-bad/40 bg-bad/10"
                      : removalDeathCertificate.tone === "warn"
                        ? "border-warn/40 bg-warn/10"
                        : removalDeathCertificate.tone === "good"
                          ? "border-good/35 bg-good/5"
                          : "border-border bg-card/55",
                  )}
                >
                  <div className="flex flex-wrap items-center gap-2">
                    <div className="text-[10px] font-semibold uppercase tracking-wider text-muted">Death certificate</div>
                    <div
                      className={cn(
                        "font-medium",
                        removalDeathCertificate.tone === "bad"
                          ? "text-bad"
                          : removalDeathCertificate.tone === "warn"
                            ? "text-warn"
                            : removalDeathCertificate.tone === "good"
                              ? "text-good"
                              : "text-muted",
                      )}
                    >
                      {removalDeathCertificate.label}
                    </div>
                  </div>
                  <div className="mt-1 text-muted">{removalDeathCertificate.detail}</div>
                  <div className="mt-2 grid gap-1.5 md:grid-cols-3">
                    {Object.entries(removalDeathCertificate.fields).map(([label, value]) => (
                      <div key={label} className="min-w-0 rounded-md border border-border bg-panel/35 px-2 py-1">
                        <div className="truncate text-[9px] font-semibold uppercase tracking-wider text-muted">{label}</div>
                        <div className="mt-0.5 truncate text-[11px] text-foreground" title={value}>
                          {value}
                        </div>
                      </div>
                    ))}
                  </div>
                </div>
              )}
              <div className="mt-3 rounded-lg border border-border bg-card/45 p-2">
                <div className="mb-1 text-[10px] font-semibold uppercase tracking-wider text-muted">Removal ledger</div>
                <div className="grid gap-1.5 md:grid-cols-3">
                  {removalLedger.map((entry) => (
                    <div
                      key={entry.label}
                      className={cn(
                        "min-w-0 rounded-md border px-2 py-1.5 text-[11px]",
                        entry.tone === "bad"
                          ? "border-bad/35 bg-bad/5"
                          : entry.tone === "warn"
                            ? "border-warn/35 bg-warn/10"
                            : entry.tone === "good"
                              ? "border-good/30 bg-good/5"
                              : "border-border bg-panel/35",
                      )}
                    >
                      <div className="truncate text-[9px] font-semibold uppercase tracking-wider text-muted">{entry.label}</div>
                      <div
                        className={cn(
                          "mt-0.5 truncate font-medium",
                          entry.tone === "bad" && "text-bad",
                          entry.tone === "warn" && "text-warn",
                          entry.tone === "good" && "text-good",
                        )}
                        title={entry.detail}
                      >
                        {entry.value}
                      </div>
                      <div className="mt-0.5 truncate text-muted" title={entry.detail}>
                        {entry.detail}
                      </div>
                    </div>
                  ))}
                </div>
              </div>
              <div className="mt-3 grid gap-2 md:grid-cols-2">
                <div className="md:col-span-2 flex flex-wrap items-center gap-2 rounded-lg border border-border bg-card/50 p-2 text-xs text-muted">
                  <span className="mr-auto">Cleanup preset</span>
                  <Button
                    size="sm"
                    variant="ghost"
                    onClick={() => setRemoving((r) => (r ? { ...r, cascade: agentRemovalCascadePreset("clean_all") } : r))}
                  >
                    Clean all
                  </Button>
                  <Button
                    size="sm"
                    variant="ghost"
                    onClick={() => setRemoving((r) => (r ? { ...r, cascade: agentRemovalCascadePreset("keep_all") } : r))}
                  >
                    Keep all
                  </Button>
                </div>
                <CascadeOption
                  label="Standing orders"
                  count={removalToggleItems?.standing.length || 0}
                  checked={removing.cascade.standing}
                  onChange={(v) => setRemoving((r) => (r ? { ...r, cascade: { ...r.cascade, standing: v } } : r))}
                  items={removalToggleItems?.standing || []}
                  note={removalIncludesSubagents ? "Includes dependent sub-agent tree standing orders." : undefined}
                />
                <CascadeOption
                  label="Schedules"
                  count={removalToggleItems?.schedules.length || 0}
                  checked={removing.cascade.schedules}
                  onChange={(v) => setRemoving((r) => (r ? { ...r, cascade: { ...r.cascade, schedules: v } } : r))}
                  items={removalToggleItems?.schedules || []}
                  note={removalIncludesSubagents ? "Includes dependent sub-agent tree schedules." : undefined}
                />
                <CascadeOption
                  label="Private memory"
                  count={removalToggleItems?.memory.length || 0}
                  checked={removing.cascade.memory}
                  onChange={(v) => setRemoving((r) => (r ? { ...r, cascade: { ...r.cascade, memory: v } } : r))}
                  items={removalToggleItems?.memory || []}
                  note={removalIncludesSubagents ? "Includes dependent sub-agent tree private memory." : "Only this agent's private scope."}
                />
                <CascadeOption
                  label="Authored shared memory"
                  count={removalToggleItems?.authoredMemory.length || 0}
                  checked={removing.cascade.authored_memory}
                  onChange={(v) => setRemoving((r) => (r ? { ...r, cascade: { ...r.cascade, authored_memory: v } } : r))}
                  items={removalToggleItems?.authoredMemory || []}
                  note={removalIncludesSubagents ? "Includes dependent sub-agent tree authored shared memory; off by default." : "Shared brain records this agent wrote; off by default."}
                />
                <CascadeOption
                  label="Private skills"
                  count={removalToggleItems?.skills.length || 0}
                  checked={removing.cascade.skills}
                  onChange={(v) => setRemoving((r) => (r ? { ...r, cascade: { ...r.cascade, skills: v } } : r))}
                  items={removalToggleItems?.skills || []}
                  note={removalIncludesSubagents ? "Includes dependent sub-agent tree private skills; shared skills are kept." : "Shared skills are kept."}
                />
                <CascadeOption
                  label="Agent config"
                  count={removalToggleItems?.configs.length || 0}
                  checked={removing.cascade.config}
                  onChange={(v) => setRemoving((r) => (r ? { ...r, cascade: { ...r.cascade, config: v } } : r))}
                  items={removalToggleItems?.configs || []}
                  note={removalIncludesSubagents ? "Includes dependent sub-agent tree config; shared config entries stay with removed agents pruned from access lists." : "Owned config entries are deleted; shared config entries stay, with this agent pruned from access lists."}
                />
                <CascadeOption
                  label="Workspace"
                  count={removalToggleItems?.workspaces.length || 0}
                  checked={!!removing.cascade.workspace}
                  onChange={(v) => setRemoving((r) => (r ? { ...r, cascade: { ...r.cascade, workspace: v } } : r))}
                  items={removalToggleItems?.workspaces || []}
                  note={removalIncludesSubagents ? "Includes dependent sub-agent tree workdirs under the workspace root." : "Deletes only this agent's workspace-relative workdir."}
                />
                <CascadeOption
                  label="Dependent sub-agent tree"
                  count={removing.subagents.length}
                  checked={removing.cascade.subagents}
                  onChange={(v) => setRemoving((r) => (r ? { ...r, cascade: { ...r.cascade, subagents: v } } : r))}
                  items={removing.subagents}
                  note="Retired, not deleted, so identity and logs remain inspectable."
                />
                <ImpactList label="Sub-agent standing orders" count={removing.subagentStanding.length} items={removing.subagentStanding} note="Cleaned when Standing orders and Dependent sub-agent tree are both selected." />
                <ImpactList label="Sub-agent schedules" count={removing.subagentSchedules.length} items={removing.subagentSchedules} note="Cleaned when Schedules and Dependent sub-agent tree are both selected." />
                <ImpactList label="Sub-agent private memory" count={removing.subagentMemories.length} items={removing.subagentMemories} note="Cleaned when Private memory and Dependent sub-agent tree are both selected." />
                <ImpactList label="Sub-agent authored shared memory" count={removing.subagentAuthoredMemories.length} items={removing.subagentAuthoredMemories} note="Cleaned when Authored shared memory and Dependent sub-agent tree are both selected." />
                <ImpactList label="Sub-agent skills" count={removing.subagentSkills.length} items={removing.subagentSkills} note="Cleaned when Private skills and Dependent sub-agent tree are both selected." />
                <ImpactList label="Sub-agent config" count={removing.subagentConfigs.length} items={removing.subagentConfigs} note="Cleaned when Agent config and Dependent sub-agent tree are both selected." />
                <ImpactList label="Sub-agent workspace" count={removing.subagentWorkspaces.length} items={removing.subagentWorkspaces} note="Cleaned when Workspace and Dependent sub-agent tree are both selected." />
                <ImpactList label="Workflow references" count={(removing.workflowRefs || []).length} items={removing.workflowRefs || []} note="Retained; workflows are reusable operator-owned chains, not agent identities." />
                <ImpactList label="Sub-agent workflow references" count={(removing.subagentWorkflowRefs || []).length} items={removing.subagentWorkflowRefs || []} note="Retained with the workflow graph; inspect before deleting this identity tree." />
                <ImpactList label="Mailbox / audit history" count={(removing.mailboxMessages || []).length} items={removing.mailboxMessages || []} note="Retained for inspection; board retention controls aging." />
                <ImpactList label="Sub-agent mailbox / audit history" count={(removing.subagentMailboxMessages || []).length} items={removing.subagentMailboxMessages || []} note="Retained with the retired dependent identities." />
              </div>
              {removalPlan && (
                <div className="mt-3 rounded-md bg-card/70 px-2 py-1.5 text-[11px] text-muted">
                  <div className={cn("mb-1 font-medium", removalPlan.blockedBySubagents ? "text-bad" : "text-foreground")}>
                    {agentRemovalImpactSummary(removalPlan)}
                  </div>
                  Remove plan: delete identity
                  {removalPlan.cleanupPlan.length > 0 ? `; clean ${removalPlan.cleanupPlan.join(", ")}` : "; no dependent cleanup selected"}
                  {removalPlan.keepPlan.length > 0 ? `; keep ${removalPlan.keepPlan.join(", ")}` : ""}
                  {removalPlan.blockedBySubagents ? (
                    <span className="block pt-1 font-medium text-bad">
                      Dependent sub-agent tree must be retired with this removal before the identity can be deleted.
                    </span>
                  ) : null}
                </div>
              )}
            </div>
            <div className="flex shrink-0 gap-2">
              <Button size="sm" variant="ghost" onClick={() => setRemoving(null)}>
                Cancel
              </Button>
              <Button
                size="sm"
                variant="danger"
                disabled={busy === removing.slug || !!removalPlan?.blockedBySubagents}
                title={removalPlan?.blockedBySubagents ? "Dependent sub-agent tree must be selected for cleanup first" : "Remove identity"}
                onClick={confirmRemove}
              >
                <Trash2 className="h-3.5 w-3.5" /> Remove
              </Button>
            </div>
          </div>
        </div>
      )}

      {err && <ErrorText>{err}</ErrorText>}
      {!profiles && !err && <SkeletonList count={3} />}
      {profiles && profiles.length === 0 && !showForm && (
        <EmptyState
          icon={Bot}
          title="No agents yet"
          hint='Create a named agent — its soul, model, and budget — then run it with "agt run --agent <slug>" or delegate to it by name.'
        />
      )}

      {profiles && profiles.length > 0 && shownList.length === 0 && (
        <EmptyState icon={Users} title="No matching agents" hint="Try a different roster filter." />
      )}

      <ul className="grid gap-2.5 sm:grid-cols-2 xl:grid-cols-3">
        {shownList.map((p) => {
          const open = editing === p.slug || activityFor === p.slug;
          const lifecycle = p.lifecycle?.mode || (p.lifecycle?.retire_on_complete ? "retire_on_complete" : "persistent");
          const cycleTasks = (p.tasklist || []).filter((t) => (t.scope || "total") === "cycle" && t.status !== "retired").length;
          const totalTasks = (p.tasklist || []).filter((t) => (t.scope || "total") === "total" && t.status !== "retired").length;
          const taskSummary = agentTaskProgressSummary(p.tasklist);
          const cfgSummary = summarizeConfigOverrides(p.config_overrides);
          const invalidRuntimeOverrides = cfgSummary.runtime.filter((r) => !r.valid).length;
          const runtimeStatus = summarizeAgentRuntimeStatus(p.status);
          const healthIssueSummary = agentHealthIssueSummary(runtimeStatus);
          const noiseSummary = agentNoisePolicySummary(p);
          const guardianSafety = systemGuardianSafetySummary(p);
          const lifecycleSummary = agentLifecycleSummary(p);
          const lifecycleDisposition = agentLifecycleDispositionPassport(p);
          const graveyardSummary = agentGraveyardSummary(p);
          const taskContractSummary = agentTaskContractSummary(p);
          const hierarchySummary = agentHierarchySummary(p);
          const hierarchyTreePassport = agentHierarchyTreePassport(p, list);
          const delegationPassport = agentDelegationPassport(p);
          const identityDossier = agentIdentityDossier(p);
          const capabilitySummary = agentCapabilityPassportSummary(p);
          const resourcePassport = agentResourcePassportSummary(p);
          const configPassportSummary = agentConfigPassportSummary(p, invalidRuntimeOverrides);
          const repairGovernance = agentRepairGovernancePassport(p);
          const repairOperations = agentRepairOperationsPassport(p, runtimeStatus);
          const modelPassportSummary = agentModelPassportSummary(p);
          const privateSkillCount = skillCounts[p.slug] || 0;
          const skillPassportSummary = agentSkillPassportSummary(privateSkillCount);
          const identityLedger = agentIdentityLedger(p, privateSkillCount);
          const noiseBudgetPassport = agentNoiseBudgetPassport(p);
          const schedulePassport = schedulePressure[p.slug] || agentSchedulePressurePassport(p, []);
          const guardianNoiseContract = systemGuardianNoiseContract(p, schedulePassport);
          const wakeIssue = rosterWakeIssue(p);
          const repairIssue = rosterRepairIssue(p);
          const avatarStatus = p.retired
            ? "retired"
            : runtimeStatus.activeRunCount > 0
              ? "running"
              : runtimeStatus.operationalState === "paused" || !p.enabled
                ? "paused"
                : "sleeping";
          const lifecycleRail = agentLifecycleRail(p, runtimeStatus, wakeIssue);
          const waitingMailboxCount = mailboxCounts[p.slug.toLowerCase()] || mailboxCounts[p.slug] || 0;
          const mailboxPassport = rosterMailboxPassport(p.slug, waitingMailboxCount, standingOrders);
          const lifeSummary = agentLifeSummary(p, runtimeStatus, wakeIssue, waitingMailboxCount);
          const livePresence = agentLivePresencePassport(p, runtimeStatus, waitingMailboxCount);
          const identityCardSummary = agentIdentityCardSummary(p, runtimeStatus, wakeIssue, waitingMailboxCount, Date.now(), capabilitySummary);
          const identityManifest = agentRosterIdentityManifest(p, runtimeStatus, wakeIssue, waitingMailboxCount, privateSkillCount, capabilitySummary, Date.now());
          const commandStrip = agentCommandStrip(
            p,
            runtimeStatus,
            mailboxPassport,
            schedulePassport,
            capabilitySummary,
            resourcePassport,
            wakeIssue,
            Date.now(),
          );
          return (
          <li
            key={p.id}
            className={cn(
              "glass flex min-h-[420px] flex-col overflow-hidden rounded-xl shadow-e1 transition-[box-shadow,border-color] hover:shadow-e2",
              open && "sm:col-span-2 xl:col-span-3",
            )}
          >
            <div className="border-b border-border/70 bg-card/35 p-3">
              <div className="flex min-w-0 items-start gap-2.5">
                <button onClick={() => openAgent(p.slug)} title="Open identity page" className="shrink-0">
                  <AgentAvatar slug={p.slug} name={p.name} size={38} status={avatarStatus} />
                </button>
                <div className="min-w-0 flex-1">
                  <div className="flex min-w-0 items-center gap-2">
                    <button
                      onClick={() => openAgent(p.slug)}
                      title="Open identity page"
                      className={cn("truncate font-mono text-sm font-semibold hover:underline", p.retired ? "text-muted line-through" : "text-foreground")}
                    >
                      {p.slug}
                    </button>
                    {p.retired ? (
                      <Badge variant="default" className="inline-flex shrink-0 items-center gap-1 text-muted">
                        <Skull className="h-3 w-3" /> graveyard
                      </Badge>
                    ) : (
                      <Badge variant={p.enabled ? "good" : "default"} className="shrink-0">
                        {p.enabled ? "enabled" : "paused"}
                      </Badge>
                    )}
                  </div>
                  <div className="mt-0.5 min-h-[1rem] truncate text-xs text-muted">
                    {p.name && p.name !== p.slug ? p.name : p.description || "identity profile"}
                  </div>
                </div>
              </div>
              <div
                className={cn(
                  "mt-2 flex min-w-0 items-start gap-1.5 rounded-md border border-border/60 bg-card/55 px-2 py-1.5 text-[11px]",
                  identityCardSummary.tone === "good" && "border-good/25 bg-good/5",
                  identityCardSummary.tone === "bad" && "border-bad/30 bg-bad/5",
                  identityCardSummary.tone === "warn" && "border-warn/35 bg-warn/10",
                  identityCardSummary.tone === "accent" && "border-accent/30 bg-accent/10",
                  identityCardSummary.tone === "muted" && "text-muted",
                )}
                title={identityCardSummary.detail}
              >
                <IdCard
                  className={cn(
                    "mt-0.5 h-3 w-3 shrink-0",
                    identityCardSummary.tone === "good" && "text-good",
                    identityCardSummary.tone === "bad" && "text-bad",
                    identityCardSummary.tone === "warn" && "text-warn",
                    identityCardSummary.tone === "accent" && "text-accent",
                    identityCardSummary.tone === "muted" && "text-muted",
                  )}
                />
                <span
                  className={cn(
                    "shrink-0 font-semibold",
                    identityCardSummary.tone === "good" && "text-good",
                    identityCardSummary.tone === "bad" && "text-bad",
                    identityCardSummary.tone === "warn" && "text-warn",
                    identityCardSummary.tone === "accent" && "text-accent",
                    identityCardSummary.tone === "muted" && "text-foreground/80",
                  )}
                >
                  {identityCardSummary.label}
                </span>
                <span className="min-w-0 truncate text-muted">{identityCardSummary.detail}</span>
              </div>
              <div
                className="mt-2 grid gap-1.5 sm:grid-cols-2 lg:grid-cols-3"
                aria-label={`${p.slug} identity manifest`}
              >
                {identityManifest.map((entry) => (
                  <div
                    key={entry.label}
                    title={entry.detail}
                    className={cn(
                      "min-w-0 rounded-md border border-border/55 bg-panel/40 px-2 py-1.5 text-[11px]",
                      entry.tone === "good" && "border-good/25 bg-good/5",
                      entry.tone === "bad" && "border-bad/30 bg-bad/5",
                      entry.tone === "warn" && "border-warn/35 bg-warn/10",
                      entry.tone === "accent" && "border-accent/30 bg-accent/10",
                    )}
                  >
                    <div className="truncate text-[9px] font-semibold uppercase tracking-wider text-muted/80">
                      {entry.label}
                    </div>
                    <div
                      className={cn(
                        "mt-0.5 truncate font-medium text-foreground/90",
                        entry.tone === "good" && "text-good",
                        entry.tone === "bad" && "text-bad",
                        entry.tone === "warn" && "text-warn",
                        entry.tone === "accent" && "text-accent",
                        entry.tone === "muted" && "text-muted",
                      )}
                    >
                      {entry.value}
                    </div>
                  </div>
                ))}
              </div>
              <div className="mt-2 flex min-w-0 items-start gap-1.5 rounded-md border border-border/60 bg-panel/45 px-2 py-1.5 text-[11px] text-muted" title={identityDossier}>
                <ListTree className="mt-0.5 h-3 w-3 shrink-0 text-accent" />
                <span className="min-w-0 truncate">{identityDossier}</span>
              </div>
              <div
                className="mt-2 grid grid-cols-2 gap-1.5 lg:grid-cols-3"
                aria-label={`${p.slug} identity ledger`}
              >
                {identityLedger.map((entry) => (
                  <div
                    key={entry.label}
                    title={entry.detail}
                    className={cn(
                      "min-w-0 rounded-md border px-2 py-1.5 text-[11px]",
                      entry.tone === "accent" && "border-accent/30 bg-accent/10",
                      entry.tone === "bad" && "border-bad/35 bg-bad/10",
                      entry.tone === "warn" && "border-warn/35 bg-warn/10",
                      entry.tone === "good" && "border-good/30 bg-good/5",
                      entry.tone === "muted" && "border-border bg-card/45",
                    )}
                  >
                    <div className="truncate text-[9px] font-semibold uppercase tracking-wider text-muted">
                      {entry.label}
                    </div>
                    <div
                      className={cn(
                        "mt-0.5 truncate font-medium",
                        entry.tone === "accent" && "text-accent",
                        entry.tone === "bad" && "text-bad",
                        entry.tone === "warn" && "text-warn",
                        entry.tone === "good" && "text-good",
                        entry.tone === "muted" && "text-foreground/80",
                      )}
                    >
                      {entry.value}
                    </div>
                  </div>
                ))}
              </div>
              <RosterCommandStrip items={commandStrip} slug={p.slug} />
              <RosterLifecycleRail steps={lifecycleRail} />
              <div
                className="mt-2 flex min-w-0 items-start gap-1.5 rounded-md border border-border/60 bg-card/45 px-2 py-1.5 text-[11px] text-muted"
                title={lifeSummary}
              >
                <Activity className="mt-0.5 h-3 w-3 shrink-0 text-accent" />
                <span className="min-w-0 truncate">{lifeSummary}</span>
              </div>
              {runtimeStatus.activeRunCount > 0 && (
                <RosterNowBand
                  phase={runtimeStatus.activePhase || runtimeStatus.liveText || "running"}
                  detail={runtimeStatus.liveDetail || runtimeStatus.activeContextDetail || "active run"}
                  since={runtimeStatus.activeStartedMs}
                  last={runtimeStatus.activeLastEventMs}
                />
              )}
              {p.system && (
                <div
                  className={cn(
                    "mt-2 flex min-w-0 items-start gap-1.5 rounded-md border px-2 py-1.5 text-[11px]",
                    guardianNoiseContract.tone === "warn"
                      ? "border-warn/35 bg-warn/10 text-warn"
                      : guardianNoiseContract.tone === "good"
                        ? "border-good/30 bg-good/5 text-good"
                        : "border-border bg-card/45 text-muted",
                  )}
                  title={guardianNoiseContract.detail}
                >
                  <Megaphone className="mt-0.5 h-3 w-3 shrink-0" />
                  <span className="shrink-0 font-medium">{guardianNoiseContract.label}</span>
                  <span className="min-w-0 truncate text-muted">{guardianNoiseContract.detail}</span>
                </div>
              )}
              <div className="mt-2 flex flex-wrap items-center gap-1.5">
                <AgentKindBadge profile={p} />
                {privateSkillCount > 0 && (
                  <IdentityPill className="bg-accent/10 text-accent" title={`${privateSkillCount} skill(s) private to this agent`}>
                    <Sparkles className="h-2.5 w-2.5" /> {privateSkillCount} skill{privateSkillCount === 1 ? "" : "s"}
                  </IdentityPill>
                )}
                {noiseSummary && <IdentityPill title={noiseSummary}>quiet policy</IdentityPill>}
                {guardianSafety && (
                  <IdentityPill className={cn(guardianSafety.startsWith("review:") ? "bg-warn/10 text-warn" : "bg-good/10 text-good")} title={guardianSafety}>
                    system safety
                  </IdentityPill>
                )}
                {p.self_repair?.enabled && <IdentityPill>self-repair</IdentityPill>}
                {p.trust_ceiling && p.trust_ceiling !== "L4" && <IdentityPill>ceiling {p.trust_ceiling}</IdentityPill>}
                {runtimeStatus.healthText && (
                  <IdentityPill
                    className={cn(runtimeStatus.healthTone === "bad" && "bg-bad/10 text-bad", runtimeStatus.healthTone === "muted" && "bg-panel text-muted")}
                    title={healthIssueSummary || undefined}
                  >
                    {runtimeStatus.healthText}
                  </IdentityPill>
                )}
                {runtimeStatus.repairText && (
                  <IdentityPill
                    className={cn(
                      runtimeStatus.repairTone === "accent" && "bg-accent/10 text-accent",
                      runtimeStatus.repairTone === "good" && "bg-good/10 text-good",
                      runtimeStatus.repairTone === "bad" && "bg-bad/10 text-bad",
                    )}
                    title={runtimeStatus.repairDetail || (runtimeStatus.nextRepairEligibleMs ? `next eligible ${new Date(runtimeStatus.nextRepairEligibleMs).toLocaleString()}` : "autonomous self-repair state")}
                  >
                    {runtimeStatus.repairText}
                  </IdentityPill>
                )}
                {runtimeStatus.repairKindText && (
                  <IdentityPill
                    className={cn(
                      runtimeStatus.repairKindTone === "accent" && "bg-accent/10 text-accent",
                      runtimeStatus.repairKindTone === "warn" && "bg-warn/10 text-warn",
                    )}
                    title="repair family"
                  >
                    {runtimeStatus.repairKindText}
                  </IdentityPill>
                )}
                {runtimeStatus.repairIncidentText && runtimeStatus.repairIncidentId && (
                  <button
                    type="button"
                    onClick={() => openIncident(runtimeStatus.repairIncidentId!)}
                    className="inline-flex items-center gap-1 rounded-md border border-border bg-warn/10 px-1.5 py-0.5 text-[10px] font-medium text-warn transition-colors hover:border-warn/50 hover:bg-warn/15"
                    title={runtimeStatus.repairIncidentDetail || "Open repair incident"}
                  >
                    {runtimeStatus.repairIncidentText}
                  </button>
                )}
                {runtimeStatus.routingText && (
                  <IdentityPill
                    className={cn(runtimeStatus.routingTone === "bad" && "bg-bad/10 text-bad")}
                    title={runtimeStatus.routingDetail || "recent model-chain fallback pressure"}
                  >
                    {runtimeStatus.routingText}
                  </IdentityPill>
                )}
                {runtimeStatus.retryText && (
                  <IdentityPill
                    className={cn(runtimeStatus.retryTone === "bad" && "bg-bad/10 text-bad")}
                    title={runtimeStatus.retryDetail || "recent whole-run retry pressure"}
                  >
                    {runtimeStatus.retryText}
                  </IdentityPill>
                )}
                {runtimeStatus.escalationText && (
                  <IdentityPill
                    className={cn(
                      runtimeStatus.escalationTone === "bad" && "bg-bad/10 text-bad",
                      runtimeStatus.escalationTone === "accent" && "bg-accent/10 text-accent",
                    )}
                    title="open owner/escalation workload"
                  >
                    {runtimeStatus.escalationText}
                  </IdentityPill>
                )}
                {waitingMailboxCount > 0 && (
                  <IdentityPill className="bg-accent/10 text-accent" title={`${waitingMailboxCount} mailbox message${waitingMailboxCount === 1 ? "" : "s"} waiting for this agent`}>
                    <Mail className="h-2.5 w-2.5" /> inbox {waitingMailboxCount}
                  </IdentityPill>
                )}
                {runtimeStatus.liveText && (
                  <IdentityPill
                    className={cn(runtimeStatus.liveTone === "accent" && "bg-accent/10 text-accent")}
                    title={[
                      runtimeStatus.liveDetail || "active run",
                      runtimeStatus.activeStartedMs ? `since ${new Date(runtimeStatus.activeStartedMs).toLocaleString()}` : "",
                      runtimeStatus.activeLastEventMs ? `last ${new Date(runtimeStatus.activeLastEventMs).toLocaleString()}` : "",
                    ].filter(Boolean).join(" · ")}
                  >
                    {runtimeStatus.liveText}
                  </IdentityPill>
                )}
                {!runtimeStatus.liveText && runtimeStatus.operationalText && (
                  <IdentityPill
                    className={cn(
                      runtimeStatus.operationalState === "paused" && "bg-warn/10 text-warn",
                      runtimeStatus.operationalState === "sleeping" && "bg-panel text-muted",
                    )}
                    title={runtimeStatus.lastActivitySummary || "agent operational state"}
                  >
                    {runtimeStatus.operationalText}
                  </IdentityPill>
                )}
                {runtimeStatus.wakeText && (
                  <IdentityPill
                    className={cn(runtimeStatus.wakeTone === "accent" && "bg-accent/10 text-accent")}
                    title={runtimeStatus.nextWakeMs ? `${runtimeStatus.wakeDetail || "wake bindings"} · next ${new Date(runtimeStatus.nextWakeMs).toLocaleString()}` : runtimeStatus.wakeDetail || "wake bindings"}
                  >
                    {runtimeStatus.wakeText}
                  </IdentityPill>
                )}
                {(p.config_overrides && Object.keys(p.config_overrides).length > 0) && (
                  <IdentityPill className={cn(invalidRuntimeOverrides > 0 && "bg-bad/10 text-bad")} title={invalidRuntimeOverrides > 0 ? `${invalidRuntimeOverrides} invalid runtime override(s)` : "agent config overrides"}>
                    cfg {Object.keys(p.config_overrides).length}{invalidRuntimeOverrides > 0 ? ` !${invalidRuntimeOverrides}` : ""}
                  </IdentityPill>
                )}
                {lifecycle !== "persistent" && <IdentityPill>{lifecycleSummary}</IdentityPill>}
              </div>
              <div className="mt-2 grid gap-2 lg:grid-cols-3" aria-label={`${p.slug} identity card passports`}>
                <RosterPassportSection label="Identity">
                  <RosterPassportCell
                    label="presence"
                    value={livePresence.value}
                    title={livePresence.detail}
                    tone={livePresence.tone}
                  />
                  <RosterPassportCell
                    label="lifecycle"
                    value={lifecycleDisposition.value}
                    title={lifecycleDisposition.detail}
                    tone={lifecycleDisposition.tone}
                  />
                  <RosterPassportCell
                    label="contract"
                    value={taskContractSummary}
                    title={taskContractSummary}
                  />
                  <RosterPassportCell
                    label="model"
                    value={modelPassportSummary}
                    title={modelPassportSummary}
                    tone={p.model ? "good" : "muted"}
                  />
                  <RosterPassportCell
                    label="skills"
                    value={skillPassportSummary}
                    title={skillPassportSummary}
                    tone={privateSkillCount > 0 ? "good" : "warn"}
                  />
                </RosterPassportSection>
                <RosterPassportSection label="Authority">
                  <RosterPassportCell
                    label="authority"
                    value={capabilitySummary}
                    title={capabilitySummary}
                    tone={(p.tool_allow || []).length > 0 || (p.tool_deny || []).length > 0 || (p.trust_ceiling && p.trust_ceiling !== "L4") ? "good" : "warn"}
                  />
                  <RosterPassportCell
                    label="resources"
                    value={resourcePassport.detail}
                    title={resourcePassport.detail}
                    tone={resourcePassport.tone}
                  />
                  <RosterPassportCell
                    label="config"
                    value={configPassportSummary}
                    title={[configPassportSummary, noiseSummary].filter(Boolean).join(" · ")}
                    tone={invalidRuntimeOverrides > 0 ? "bad" : configPassportSummary === "default config" ? "muted" : "good"}
                  />
                  <RosterPassportCell
                    label="noise"
                    value={noiseBudgetPassport.detail}
                    title={noiseBudgetPassport.detail}
                    tone={noiseBudgetPassport.tone}
                  />
                  <RosterPassportCell
                    label="schedule"
                    value={schedulePassport.detail}
                    title={schedulePassport.detail}
                    tone={schedulePassport.tone}
                  />
                </RosterPassportSection>
                <RosterPassportSection label="Operations">
                  <RosterPassportCell
                    label="mailbox"
                    value={mailboxPassport.value}
                    title={mailboxPassport.detail}
                    tone={mailboxPassport.tone}
                  />
                  <RosterPassportCell
                    label="lineage"
                    value={hierarchyTreePassport.value}
                    title={hierarchyTreePassport.detail}
                    tone={hierarchyTreePassport.tone}
                  />
                  <RosterPassportCell
                    label="delegation"
                    value={delegationPassport.detail}
                    title={[hierarchySummary, hierarchyTreePassport.detail, delegationPassport.detail].join(" · ")}
                    tone={delegationPassport.tone}
                  />
                  <RosterPassportCell
                    label="resilience"
                    value={repairGovernance.value}
                    title={repairGovernance.detail}
                    tone={repairGovernance.tone}
                  />
                  <RosterPassportCell
                    label="repair ops"
                    value={repairOperations.value}
                    title={repairOperations.detail}
                    tone={repairOperations.tone}
                  />
                  <RosterPassportCell
                    label="wake / health"
                    value={[
                      runtimeStatus.nextWakeMs ? formatWakeDue(runtimeStatus.nextWakeMs) : runtimeStatus.wakeText || "manual",
                      runtimeStatus.healthText || (p.retired ? "graveyard" : "nominal"),
                    ].join(" · ")}
                    title={[
                      runtimeStatus.wakeDetail || "",
                      healthIssueSummary || "",
                    ].filter(Boolean).join(" · ") || undefined}
                    tone={runtimeStatus.healthTone === "bad" ? "bad" : runtimeStatus.healthTone === "muted" ? "muted" : "good"}
                  />
                </RosterPassportSection>
              </div>
            </div>

            <div className="flex flex-1 flex-col p-3">
              <div className="mt-2 flex flex-wrap gap-x-3 gap-y-1 text-[11px] text-muted">
                {p.model && <span className="font-mono text-foreground/80">{p.model}</span>}
                {(p.max_cost_mc || 0) > 0 && <span>{money(p.max_cost_mc)}</span>}
                {(p.max_daily_mc || 0) > 0 && <span>{money(p.max_daily_mc)}</span>}
                {p.owner_agent && <span>owner: <span className="font-mono text-foreground/80">{p.owner_agent}</span></span>}
                {p.parent_agent && <span>parent: <span className="font-mono text-foreground/80">{p.parent_agent}</span></span>}
                {p.retry_policy?.max_attempts ? <span>retry: {p.retry_policy.max_attempts}x</span> : null}
                {p.health_policy?.doctor_agent && <span>doctor: <span className="font-mono text-foreground/80">{p.health_policy.doctor_agent}</span></span>}
                {(p.tool_allow || []).length > 0 && <span>allow: {(p.tool_allow || []).slice(0, 3).join(", ")}{(p.tool_allow || []).length > 3 ? "…" : ""}</span>}
                {(p.tool_deny || []).length > 0 && <span>deny: {(p.tool_deny || []).slice(0, 3).join(", ")}{(p.tool_deny || []).length > 3 ? "…" : ""}</span>}
                {(p.config_overrides && Object.keys(p.config_overrides).length > 0) && (
                  <span>
                    cfg: {Object.keys(p.config_overrides).slice(0, 2).join(", ")}{Object.keys(p.config_overrides).length > 2 ? "…" : ""}{invalidRuntimeOverrides > 0 ? ` · invalid ${invalidRuntimeOverrides}` : ""}
                  </span>
                )}
                {graveyardSummary && <span>{graveyardSummary}</span>}
                {guardianSafety && <span>{guardianSafety}</span>}
                {noiseSummary && <span>noise: {noiseSummary}</span>}
                {runtimeStatus.routingText && (
                  <span>
                    route: {runtimeStatus.routingText}{runtimeStatus.routingDetail ? ` · ${runtimeStatus.routingDetail}` : ""}
                  </span>
                )}
                {runtimeStatus.retryText && (
                  <span>
                    retry: {runtimeStatus.retryText}{runtimeStatus.retryDetail ? ` · ${runtimeStatus.retryDetail}` : ""}
                  </span>
                )}
                {runtimeStatus.escalationText && (
                  <span>
                    load: {runtimeStatus.escalationText}
                  </span>
                )}
                {runtimeStatus.liveDetail && (
                  <span>
                    live: {runtimeStatus.liveDetail}
                  </span>
                )}
                {runtimeStatus.lastActivitySummary && (
                  <span>
                    last: {runtimeStatus.lastActivitySummary}{runtimeStatus.lastActivityMs ? ` · ${new Date(runtimeStatus.lastActivityMs).toLocaleString()}` : ""}
                  </span>
                )}
                {runtimeStatus.wakeDetail && (
                  <span>
                    wake: {runtimeStatus.wakeDetail}
                  </span>
                )}
                {(cycleTasks > 0 || totalTasks > 0) && <span>tasks: {taskSummary}</span>}
                {(p.fallbacks || []).length > 0 && <span className="truncate">fallbacks: {(p.fallbacks || []).join(" -> ")}</span>}
                {p.retired && p.retired_reason && <span>reason: {p.retired_reason}</span>}
              </div>

              {p.description && p.name && p.name !== p.slug && <div className="mt-2 text-xs text-muted">{p.description}</div>}
              {p.soul && (
                <div className={cn("mt-2 rounded-md bg-panel px-2 py-1.5 text-xs text-muted whitespace-pre-wrap", !open && "line-clamp-3")}>
                  {p.soul}
                </div>
              )}
            </div>

            <div className="mt-auto flex items-center justify-between gap-2 border-t border-border/70 bg-panel/30 px-3 py-2">
              <span className="truncate font-mono text-[10px] text-muted">{p.id}</span>
              <span className="flex shrink-0 items-center gap-1">
                <Button
                  size="sm"
                  variant="ghost"
                  aria-label={`Identity page for ${p.slug}`}
                  title="Open the full identity page (everything about this agent)"
                  onClick={() => openAgent(p.slug)}
                >
                  <IdCard className="h-3.5 w-3.5" />
                </Button>
                <Button
                  size="sm"
                  variant="ghost"
                  aria-label={`Activity for ${p.slug}`}
                  title="What this agent did (runs, consults, memory, messages)"
                  onClick={() => setActivityFor(activityFor === p.slug ? null : p.slug)}
                >
                  <Activity className="h-3.5 w-3.5" />
                </Button>
                <Button
                  size="sm"
                  variant="ghost"
                  disabled={busy === p.slug || !!wakeIssue}
                  aria-label={`Wake ${p.slug}`}
                  title={wakeIssue || "Wake this agent now"}
                  onClick={() => wakeAgent(p.slug)}
                >
                  <Zap className="h-3.5 w-3.5" />
                </Button>
                <Button
                  size="sm"
                  variant="ghost"
                  disabled={busy === `schedule:${p.slug}` || schedulePassport.frequentIds.length === 0}
                  aria-label={`Pause frequent schedules for ${p.slug}`}
                  title={schedulePassport.frequentIds.length > 0 ? `Pause ${schedulePassport.frequentIds.length} frequent schedule${schedulePassport.frequentIds.length === 1 ? "" : "s"}` : "No frequent schedules"}
                  onClick={() => pauseFrequentSchedules(p.slug, schedulePassport.frequentIds)}
                >
                  <CalendarClock className="h-3.5 w-3.5" />
                </Button>
                <Button
                  size="sm"
                  variant="ghost"
                  disabled={busy === p.slug || !!repairIssue}
                  aria-label={`Repair ${p.slug}`}
                  title={repairIssue || "Request a governed doctor/repair run for this agent"}
                  onClick={() => repairAgent(p.slug)}
                >
                  <Wrench className="h-3.5 w-3.5" />
                </Button>
                <Button
                  size="sm"
                  variant="ghost"
                  aria-label={`Edit ${p.slug}`}
                  onClick={() => setEditing(editing === p.slug ? null : p.slug)}
                >
                  <Pencil className="h-3.5 w-3.5" />
                </Button>
                {!p.retired && (
                  <Button
                    size="sm"
                    variant="ghost"
                    disabled={busy === p.slug}
                    aria-label={p.enabled ? `Pause ${p.slug}` : `Resume ${p.slug}`}
                    onClick={() => setAgentEnabled(p.slug, !p.enabled)}
                  >
                    {p.enabled ? <Pause className="h-3.5 w-3.5" /> : <Play className="h-3.5 w-3.5" />}
                  </Button>
                )}
                {p.retired ? (
                  <Button
                    size="sm"
                    variant="ghost"
                    disabled={busy === p.slug}
                    aria-label={`Revive ${p.slug}`}
                    title="Revive from the graveyard"
                    onClick={() => revive(p.slug)}
                  >
                    <ArchiveRestore className="h-3.5 w-3.5" />
                  </Button>
                ) : (
                  <Button
                    size="sm"
                    variant="ghost"
                    disabled={busy === p.slug}
                    aria-label={`Retire ${p.slug}`}
                    title="Retire to the graveyard"
                    onClick={() => retire(p.slug)}
                  >
                    <Archive className="h-3.5 w-3.5" />
                  </Button>
                )}
                {!p.system && (
                  <Button
                    size="sm"
                    variant="ghost"
                    disabled={busy === p.slug}
                    aria-label={`Remove ${p.slug}`}
                    onClick={() => removeAgent(p.slug)}
                  >
                    <Trash2 className="h-3.5 w-3.5" />
                  </Button>
                )}
              </span>
            </div>
            {editing === p.slug && (
              <div className="border-t border-border/70 p-3">
                <EditAgentForm
                  profile={p}
                  onSaved={(slug) => {
                    setEditing(null);
                    ui.toast(`agent ${slug} updated`, "success");
                    reload();
                  }}
                  onError={(msg) => ui.toast(msg, "error")}
                />
              </div>
            )}
            {activityFor === p.slug && (
              <div className="border-t border-border/70 p-3">
                <AgentActivity slug={p.slug} initialOpenRun={activityFocus[p.slug]} initialTab={activityFocus[p.slug] ? "runs" : "activity"} />
              </div>
            )}
          </li>
          );
        })}
      </ul>
    </div>
  );
}

function RosterStat({ label, value, accent, tone }: { label: string; value: ReactNode; accent?: boolean; tone?: "accent" | "warn" }) {
  const activeTone = tone || (accent ? "accent" : undefined);
  return (
    <div className={cn("rounded-lg border bg-card p-2.5 shadow-e1", activeTone === "accent" ? "border-accent/50" : activeTone === "warn" ? "border-warn/50" : "border-border")}>
      <div className="text-[10px] font-semibold uppercase tracking-wider text-muted">{label}</div>
      <div className={cn("mt-0.5 text-lg font-semibold tabular-nums", activeTone === "accent" && "text-accent", activeTone === "warn" && "text-warn")}>{value}</div>
    </div>
  );
}

function ImpactList({
  label,
  count,
  items,
  note,
}: {
  label: string;
  count: number;
  items: string[];
  note?: string;
}) {
  return (
    <div className="min-w-0 rounded-lg border border-border bg-card/65 p-2 text-xs">
      <div className="flex items-center gap-2">
        <span className="font-medium text-foreground">{label}</span>
        <span className="ml-auto rounded-md bg-panel px-1.5 py-0.5 font-mono text-[10px] text-muted">{count}</span>
      </div>
      {note && <div className="mt-1 text-[11px] text-muted">{note}</div>}
      {items.length > 0 && (
        <ul className="mt-1 max-h-20 space-y-0.5 overflow-auto rounded-md bg-panel/60 px-2 py-1 text-[11px] text-muted">
          {items.slice(0, 8).map((item) => (
            <li key={item} className="truncate" title={item}>
              {item}
            </li>
          ))}
          {items.length > 8 && <li>{items.length - 8} more</li>}
        </ul>
      )}
    </div>
  );
}

function CascadeOption({
  label,
  count,
  checked,
  onChange,
  items,
  note,
}: {
  label: string;
  count: number;
  checked: boolean;
  onChange: (checked: boolean) => void;
  items: string[];
  note?: string;
}) {
  return (
    <label className="flex min-w-0 flex-col gap-2 rounded-lg border border-border bg-panel/35 p-2 text-xs">
      <span className="flex items-center gap-2">
        <input
          type="checkbox"
          checked={checked}
          disabled={count === 0}
          onChange={(e) => onChange(e.target.checked)}
          className="size-3.5"
        />
        <span className="font-medium text-foreground">{label}</span>
        <span className="ml-auto rounded-md bg-card px-1.5 py-0.5 font-mono text-[10px] text-muted">{count}</span>
      </span>
      {note && <span className="text-[11px] text-muted">{note}</span>}
      {items.length > 0 && (
        <ul className="max-h-20 space-y-0.5 overflow-auto rounded-md bg-card/60 px-2 py-1 text-[11px] text-muted">
          {items.slice(0, 8).map((item) => (
            <li key={item} className="truncate" title={item}>
              {item}
            </li>
          ))}
          {items.length > 8 && <li>{items.length - 8} more</li>}
        </ul>
      )}
    </label>
  );
}

function AgentKindBadge({ profile }: { profile: AgentProfile }) {
  const kind = agentIdentityKind(profile);
  if (kind === "system") {
    return (
      <IdentityPill className="bg-accent/15 text-accent" title="Shipped internal guardian — protected from removal">
        <ShieldCheck className="h-2.5 w-2.5" /> guardian
      </IdentityPill>
    );
  }
  if (kind === "subagent") {
    return (
      <IdentityPill className="bg-good/10 text-good" title="Managed sub-agent — woken by its parent/owner agent">
        <Bot className="h-2.5 w-2.5" /> managed sub-agent
      </IdentityPill>
    );
  }
  return (
    <IdentityPill title="User-created custom agent identity">
      <Bot className="h-2.5 w-2.5" /> custom
    </IdentityPill>
  );
}

function IdentityPill({ children, className, title }: { children: ReactNode; className?: string; title?: string }) {
  return (
    <span
      title={title}
      className={cn("inline-flex items-center gap-1 rounded-full bg-panel px-1.5 py-0.5 text-[10px] font-medium text-muted", className)}
    >
      {children}
    </span>
  );
}

function RosterCommandStrip({ items, slug }: { items: AgentCommandStripItem[]; slug: string }) {
  return (
    <div className="mt-2 grid gap-1.5 sm:grid-cols-2 xl:grid-cols-3" aria-label={`${slug} command strip`}>
      {items.map((item) => (
        <div
          key={item.label}
          title={item.detail || item.value}
          className={cn(
            "min-w-0 rounded-md border border-border/60 bg-card/60 px-2 py-1.5",
            item.tone === "good" && "border-good/25 bg-good/5",
            item.tone === "bad" && "border-bad/30 bg-bad/5",
            item.tone === "warn" && "border-warn/35 bg-warn/10",
            item.tone === "accent" && "border-accent/30 bg-accent/10",
            item.tone === "muted" && "bg-panel/45",
          )}
        >
          <div className="flex min-w-0 items-center gap-1.5">
            <span
              className={cn(
                "size-1.5 shrink-0 rounded-full bg-muted/60",
                item.tone === "good" && "bg-good",
                item.tone === "bad" && "bg-bad",
                item.tone === "warn" && "bg-warn",
                item.tone === "accent" && "bg-accent",
              )}
            />
            <span className="truncate text-[9px] font-semibold uppercase tracking-wider text-muted/80">{item.label}</span>
          </div>
          <div
            className={cn(
              "mt-0.5 truncate text-[11px] font-medium text-foreground/90",
              item.tone === "good" && "text-good",
              item.tone === "bad" && "text-bad",
              item.tone === "warn" && "text-warn",
              item.tone === "accent" && "text-accent",
              item.tone === "muted" && "text-muted",
            )}
          >
            {item.value}
          </div>
        </div>
      ))}
    </div>
  );
}

function RosterIdentityLedger({ entries, slug }: { entries: AgentIdentityLedgerEntry[]; slug: string }) {
  return (
    <div className="mt-2 rounded-md border border-border/60 bg-panel/35 p-1.5" aria-label={`${slug} identity ledger`}>
      <div className="mb-1 text-[9px] font-semibold uppercase tracking-wider text-muted/80">Identity ledger</div>
      <div className="grid gap-1 sm:grid-cols-2 xl:grid-cols-3">
        {entries.map((entry) => (
          <div
            key={entry.label}
            title={entry.detail}
            className={cn(
              "min-h-[46px] min-w-0 rounded-md border border-border/50 bg-card/45 px-2 py-1.5",
              entry.tone === "good" && "border-good/25 bg-good/5",
              entry.tone === "bad" && "border-bad/30 bg-bad/5",
              entry.tone === "warn" && "border-warn/35 bg-warn/10",
              entry.tone === "accent" && "border-accent/30 bg-accent/10",
            )}
          >
            <div className="truncate text-[9px] font-semibold uppercase tracking-wider text-muted/80">{entry.label}</div>
            <div
              className={cn(
                "mt-0.5 truncate text-[11px] font-medium text-foreground/90",
                entry.tone === "good" && "text-good",
                entry.tone === "bad" && "text-bad",
                entry.tone === "warn" && "text-warn",
                entry.tone === "accent" && "text-accent",
                entry.tone === "muted" && "text-muted",
              )}
            >
              {entry.value}
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}

function RosterControlCenter({ entries, slug }: { entries: AgentControlCenterEntry[]; slug: string }) {
  return (
    <div className="mt-2 rounded-md border border-border/60 bg-card/35 p-1.5" aria-label={`${slug} control center`}>
      <div className="mb-1 text-[9px] font-semibold uppercase tracking-wider text-muted/80">Control center</div>
      <div className="grid gap-1 sm:grid-cols-2 xl:grid-cols-3">
        {entries.map((entry) => (
          <div
            key={entry.label}
            title={entry.detail}
            className={cn(
              "min-h-[42px] min-w-0 rounded-md border border-border/50 bg-panel/45 px-2 py-1.5",
              entry.tone === "good" && "border-good/25 bg-good/5",
              entry.tone === "bad" && "border-bad/30 bg-bad/5",
              entry.tone === "warn" && "border-warn/35 bg-warn/10",
              entry.tone === "accent" && "border-accent/30 bg-accent/10",
            )}
          >
            <div className="truncate text-[9px] font-semibold uppercase tracking-wider text-muted/80">{entry.label}</div>
            <div
              className={cn(
                "mt-0.5 truncate text-[11px] font-medium text-foreground/90",
                entry.tone === "good" && "text-good",
                entry.tone === "bad" && "text-bad",
                entry.tone === "warn" && "text-warn",
                entry.tone === "accent" && "text-accent",
                entry.tone === "muted" && "text-muted",
              )}
            >
              {entry.value}
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}

function RosterPassportSection({ label, children }: { label: string; children: ReactNode }) {
  return (
    <section className="min-w-0 rounded-lg border border-border/60 bg-card/35 p-2">
      <div className="mb-1.5 text-[9px] font-semibold uppercase tracking-wider text-muted/75">{label}</div>
      <div className="grid gap-1.5 sm:grid-cols-2 lg:grid-cols-1 xl:grid-cols-2">{children}</div>
    </section>
  );
}

function RosterPassportCell({
  label,
  value,
  title,
  tone = "muted",
}: {
  label: string;
  value: string;
  title?: string;
  tone?: "good" | "bad" | "warn" | "accent" | "muted";
}) {
  return (
    <div
      title={title || value}
      className={cn(
        "min-w-0 rounded-md border border-border/60 bg-panel/45 px-2 py-1.5",
        tone === "good" && "border-good/25 bg-good/5",
        tone === "bad" && "border-bad/30 bg-bad/5",
        tone === "warn" && "border-warn/35 bg-warn/10",
        tone === "accent" && "border-accent/30 bg-accent/10",
      )}
    >
      <div className="text-[9px] font-semibold uppercase tracking-wider text-muted/80">{label}</div>
      <div
        className={cn(
          "mt-0.5 truncate text-[11px] text-foreground/90",
          tone === "good" && "text-good",
          tone === "bad" && "text-bad",
          tone === "warn" && "text-warn",
          tone === "accent" && "text-accent",
        )}
      >
        {value}
      </div>
    </div>
  );
}

function RosterLifecycleRail({ steps }: { steps: AgentLifecycleRailStep[] }) {
  return (
    <div className="mt-2 grid gap-1.5 sm:grid-cols-4" aria-label="Agent lifecycle rail">
      {steps.map((step) => (
        <div
          key={step.id}
          title={[step.value, step.detail].filter(Boolean).join(" · ")}
          className={cn(
            "min-w-0 rounded-md border border-border/60 bg-panel/45 px-2 py-1.5",
            step.tone === "good" && "border-good/25 bg-good/5",
            step.tone === "bad" && "border-bad/30 bg-bad/5",
            step.tone === "warn" && "border-warn/35 bg-warn/10",
            step.tone === "accent" && "border-accent/30 bg-accent/10",
          )}
        >
          <div className="flex items-center gap-1.5">
            <span
              className={cn(
                "size-1.5 shrink-0 rounded-full bg-muted/60",
                step.tone === "good" && "bg-good",
                step.tone === "bad" && "bg-bad",
                step.tone === "warn" && "bg-warn",
                step.tone === "accent" && "bg-accent",
              )}
            />
            <span className="truncate text-[9px] font-semibold uppercase tracking-wider text-muted/80">{step.label}</span>
          </div>
          <div
            className={cn(
              "mt-0.5 truncate text-[11px] font-medium text-foreground/90",
              step.tone === "good" && "text-good",
              step.tone === "bad" && "text-bad",
              step.tone === "warn" && "text-warn",
              step.tone === "accent" && "text-accent",
            )}
          >
            {step.value}
          </div>
        </div>
      ))}
    </div>
  );
}

function RosterNowBand({
  phase,
  detail,
  since,
  last,
}: {
  phase: string;
  detail: string;
  since?: number;
  last?: number;
}) {
  const timing = [
    since ? `since ${new Date(since).toLocaleString()}` : "",
    last ? `last ${new Date(last).toLocaleString()}` : "",
  ].filter(Boolean).join(" · ");
  return (
    <div
      title={[detail, timing].filter(Boolean).join(" · ")}
      className="mt-2 grid min-h-[48px] grid-cols-[auto_1fr] gap-2 rounded-md border border-accent/35 bg-accent/10 px-2 py-1.5"
    >
      <div className="grid size-8 place-items-center rounded-md bg-accent/15 text-accent">
        <Activity className="h-4 w-4" />
      </div>
      <div className="min-w-0">
        <div className="flex min-w-0 items-center gap-2">
          <span className="text-[9px] font-semibold uppercase tracking-wider text-accent">Now</span>
          <span className="truncate text-[11px] font-medium text-foreground">{phase}</span>
        </div>
        <div className="mt-0.5 truncate text-[11px] text-muted">{detail}</div>
      </div>
    </div>
  );
}

export function formatWakeDue(ms: number, now = Date.now()): string {
  return fmtDue(ms, now);
}
