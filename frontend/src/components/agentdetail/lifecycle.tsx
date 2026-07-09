import { fmtDateTime, fmtDue, clip } from "@/lib/utils";
import { agentHierarchySummary, agentLifecycleSummary, agentRemovalCascadePreset, agentTaskContractSummary, agentTaskProgressSummary, systemGuardianSafetySummary, type AgentControlCenterEntry, type AgentProfile, type AgentRemoveResult, type AgentRetireResult, type AgentReviveResult } from "@/views/Roster";
import { type ApiSchedule } from "@/lib/fleet";
import { agentScope, summarizeEscalations, incidentLineageLabel, lastAutonomyRunbookSourceLabel, type AgentHealthSnapshot, type AgentRepairEvent, type AgentRepairStatus, type AgentCardRuntimeSummary } from "@/lib/agentdetail";
import { AgentAutonomyRunbookEntry, AgentEntityContractEntry, AgentRuntimeDoctorLedgerEntry, WakeAccessSummary, agentManagedSubagent } from "@/components/agentdetail/capability";

export interface AgentImpactSummary {
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
}

export interface AgentRemovalCascade {
	standing: boolean;
	schedules: boolean;
	memory: boolean;
	authored_memory: boolean;
  skills: boolean;
  config: boolean;
  workspace?: boolean;
	subagents: boolean;
}

type ResolvedAgentRemovalCascade = AgentRemovalCascade & { workspace: boolean };

export function agentDetailRemovalCascadePreset(mode: "clean_all" | "keep_all"): ResolvedAgentRemovalCascade {
	return { workspace: false, ...agentRemovalCascadePreset(mode) };
}

export interface AgentRemovalImpactPlan {
  clean: string[];
  keep: string[];
  blockedBySubagents: boolean;
}

export function agentRemovalRiskLabel(plan: AgentRemovalImpactPlan): string {
  if (plan.blockedBySubagents) return "blocked: dependent sub-agents would be orphaned";
  if (plan.keep.length > 0) return "retains dependent resources after identity deletion";
  if (plan.clean.length > 0) return "cleans selected owned resources with identity deletion";
  return "identity-only removal";
}

export interface AgentLifecycleInterventionSummary {
  disposition: string;
  retire: string;
  remove: string;
  tone: "good" | "warn" | "bad" | "muted";
}

export interface AgentLifecycleLedgerEntry {
  label: string;
  value: string;
  detail: string;
  tone: "good" | "warn" | "bad" | "muted";
}

export interface AgentLifecycleActionResultSummary {
  label: string;
  detail: string;
  tone: "good" | "warn" | "bad" | "muted";
}

type AgentLifecycleActionKind = "retire" | "revive" | "remove";

export function agentLifecycleActionResultSummary(
  kind: AgentLifecycleActionKind,
  slug: string,
  result?: AgentRetireResult | AgentReviveResult | AgentRemoveResult | null,
): AgentLifecycleActionResultSummary {
  if (kind === "retire") {
    const res = (result || {}) as AgentRetireResult;
    const standing = res.standing_paused || 0;
    const schedules = res.schedules_paused || 0;
    return {
      label: `${slug} retired`,
      detail: [
        "identity moved to graveyard",
        `${standing} standing wake${standing === 1 ? "" : "s"} paused`,
        `${schedules} schedule wake${schedules === 1 ? "" : "s"} paused`,
        "soul, memory, skills, config, mailbox, workspace and audit remain inspectable",
      ].join(" · "),
      tone: "muted",
    };
  }
  if (kind === "revive") {
    const res = (result || {}) as AgentReviveResult;
    const standing = res.standing_paused || 0;
    const schedules = res.schedules_paused || 0;
    return {
      label: `${slug} revived`,
      detail: [
        "identity returned from graveyard in paused service",
        standing + schedules > 0
          ? `${standing} standing and ${schedules} schedule wake routes remain paused`
          : "no paused wake routes reported",
        "operator must explicitly resume or re-arm wakes",
      ].join(" · "),
      tone: "good",
    };
  }
  const res = (result || {}) as AgentRemoveResult;
  if (!res.removed) {
    return {
      label: `${slug} not removed`,
      detail: "identity deletion was not applied",
      tone: "warn",
    };
  }
  const cleaned = [
    res.standing_removed ? `${res.standing_removed} standing` : "",
    res.schedules_removed ? `${res.schedules_removed} schedule` : "",
    res.memories_forgotten ? `${res.memories_forgotten} private memory` : "",
    res.authored_memories_forgotten ? `${res.authored_memories_forgotten} authored shared memory` : "",
    res.skills_archived ? `${res.skills_archived} skill` : "",
    res.configs_deleted ? `${res.configs_deleted} config` : "",
    res.configs_access_pruned ? `${res.configs_access_pruned} shared config access refs` : "",
    res.workspaces_deleted ? `${res.workspaces_deleted} workspace` : "",
  ].filter(Boolean);
  const retained = [
    res.mailbox_messages_retained ? `${res.mailbox_messages_retained} mailbox/audit messages` : "",
    res.workflow_refs_retained ? `${res.workflow_refs_retained} workflow refs` : "",
    res.subagent_workflow_refs_retained ? `${res.subagent_workflow_refs_retained} sub-agent workflow refs` : "",
  ].filter(Boolean);
  const retiredSlugs = (res.subagents_retired_slugs || []).filter(Boolean);
  return {
    label: `${slug} removed`,
    detail: [
      "identity profile deleted",
      cleaned.length > 0 ? `cleaned ${cleaned.join(", ")}` : "no owned cleanup reported",
      res.subagents_retired
        ? `retired ${res.subagents_retired} dependent sub-agent${res.subagents_retired === 1 ? "" : "s"}${retiredSlugs.length > 0 ? ` (${retiredSlugs.slice(0, 4).join(", ")}${retiredSlugs.length > 4 ? ` +${retiredSlugs.length - 4}` : ""})` : ""}`
        : "no dependent sub-agents retired",
      retained.length > 0 ? `retained ${retained.join(", ")}` : "audit retained by event log",
    ].join(" · "),
    tone: retained.length > 0 || (res.subagents_retired || 0) > 0 ? "warn" : "good",
  };
}

export function agentLifecycleInterventionSummary(
  profile: Pick<AgentProfile, "retired" | "system">,
  plan: AgentRemovalImpactPlan,
): AgentLifecycleInterventionSummary {
  if (profile.retired) {
    return {
      disposition: "graveyard identity",
      retire: "revive returns the identity to paused service; logs, memory, skills, config, mailbox, and workspace stay inspectable",
      remove: profile.system
        ? "hard remove is blocked for system identities"
        : plan.blockedBySubagents
          ? "remove blocked until dependent sub-agents are included"
          : `remove deletes the identity${plan.clean.length > 0 ? ` and cleans ${plan.clean.join(", ")}` : " without dependent cleanup"}`,
      tone: profile.system ? "muted" : plan.blockedBySubagents ? "bad" : "warn",
    };
  }
  return {
    disposition: profile.system ? "protected system identity" : "active identity",
    retire: "retire moves the identity to the graveyard and pauses its direct standing/schedule wakes while preserving audit, soul, memory, skills, config, mailbox, and workspace",
    remove: profile.system
      ? "hard remove is blocked for system identities; pause or retire instead"
      : plan.blockedBySubagents
        ? "remove blocked until dependent sub-agents are included so they do not run orphaned"
        : plan.keep.length > 0
          ? `remove deletes the identity, cleans ${plan.clean.length || 0} groups, and leaves ${plan.keep.join(", ")}`
          : plan.clean.length > 0
            ? `remove deletes the identity and cleans ${plan.clean.join(", ")}`
            : "remove deletes only the identity",
    tone: profile.system ? "muted" : plan.blockedBySubagents ? "bad" : plan.keep.length > 0 ? "warn" : "good",
  };
}

export function agentLifecycleDecisionLedger(
  profile: Pick<AgentProfile, "retired" | "system" | "lifecycle" | "tasklist">,
  plan: AgentRemovalImpactPlan,
): AgentLifecycleLedgerEntry[] {
  const lifecycle = agentLifecycleSummary(profile);
  const tasks = agentTaskProgressSummary(profile.tasklist) || "no durable tasks";
  const disposition = profile.retired ? "graveyard" : profile.system ? "protected" : "alive";
  return [
    {
      label: "state",
      value: `${disposition} · ${lifecycle}`,
      detail: profile.retired
        ? "identity sleeps in the graveyard until revived"
        : profile.system
          ? "system identity can be paused or retired but not hard removed"
          : "identity can run, sleep, retire, or be removed by operator decision",
      tone: profile.retired ? "muted" : profile.system ? "warn" : "good",
    },
    {
      label: "tasks",
      value: tasks,
      detail: "cycle tasks repeat on wake; total tasks persist until done, blocked, retired, or identity removal",
      tone: tasks === "no durable tasks" ? "muted" : "good",
    },
    {
      label: "retire",
      value: profile.retired ? "revive available" : "graveyard available",
      detail: profile.retired
        ? "revive returns the identity to paused service without deleting memory, skills, config, mailbox, or workspace"
        : "retire stops wake routes while preserving soul, logs, memory, skills, config, mailbox, and workspace",
      tone: profile.retired ? "good" : "muted",
    },
    {
      label: "remove",
      value: profile.system
        ? "blocked"
        : plan.blockedBySubagents
          ? "blocked by sub-agents"
          : plan.keep.length > 0
            ? `${plan.clean.length} clean · ${plan.keep.length} keep`
            : plan.clean.length > 0
              ? `${plan.clean.length} clean`
              : "identity only",
      detail: profile.system
        ? "hard remove is blocked for system identities"
        : plan.blockedBySubagents
          ? "dependent sub-agents must be retired with this removal before the identity can be deleted"
          : plan.keep.length > 0
            ? `remove would clean ${plan.clean.join(", ") || "nothing"} and keep ${plan.keep.join(", ")}`
            : plan.clean.length > 0
              ? `remove would clean ${plan.clean.join(", ")}`
              : "remove deletes only the identity profile",
      tone: profile.system ? "muted" : plan.blockedBySubagents ? "bad" : plan.keep.length > 0 ? "warn" : "good",
    },
  ];
}

export function agentScheduleBindingTitle(s: Pick<ApiSchedule, "target" | "intent" | "workflow" | "tool" | "system_task">, slug: string): string {
  if (s.target === "workflow") return `runs workflow ${s.workflow || "selected workflow"} as ${slug}`;
  if (s.target === "tool") return `invokes tool ${s.tool || "selected tool"} as ${slug}`;
  if (s.target === "system_task") return `runs system task ${s.system_task || "selected task"}`;
  return s.intent ? `wakes ${slug}: ${clip(s.intent, 80)}` : `wakes ${slug}`;
}

export function agentRemovalImpactPlan(impact: AgentImpactSummary, cascade: AgentRemovalCascade): AgentRemovalImpactPlan {
  const c = { workspace: false, ...cascade };
  const standing = impact.standing_orders || [];
  const scheduleItems = impact.schedules || [];
  const memoryItems = impact.memories || [];
  const authoredMemoryItems = impact.authored_shared_memories || [];
  const skillItems = impact.skills || [];
  const configItems = impact.configs || [];
  const workspaceItems = impact.workspaces || [];
  const workflowRefs = impact.workflow_refs || [];
  const mailboxItems = impact.mailbox_messages || [];
  const subagentItems = impact.subagents || [];
  const subagentStanding = impact.subagent_standing_orders || [];
  const subagentSchedules = impact.subagent_schedules || [];
  const subagentMemories = impact.subagent_memories || [];
  const subagentAuthoredMemories = impact.subagent_authored_shared_memories || [];
  const subagentSkills = impact.subagent_skills || [];
  const subagentConfigs = impact.subagent_configs || [];
  const subagentWorkspaces = impact.subagent_workspaces || [];
  const subagentWorkflowRefs = impact.subagent_workflow_refs || [];
  const subagentMailboxMessages = impact.subagent_mailbox_messages || [];
  return {
    clean: [
      c.standing && standing.length > 0 ? `${standing.length} standing` : "",
      c.schedules && scheduleItems.length > 0 ? `${scheduleItems.length} schedule` : "",
      c.memory && memoryItems.length > 0 ? `${memoryItems.length} private memory` : "",
      c.authored_memory && authoredMemoryItems.length > 0 ? `${authoredMemoryItems.length} authored shared memory` : "",
      c.skills && skillItems.length > 0 ? `${skillItems.length} skill` : "",
      c.config && configItems.length > 0 ? `${configItems.length} config` : "",
      c.config ? "shared config access refs" : "",
      c.workspace && workspaceItems.length > 0 ? `${workspaceItems.length} workspace` : "",
      c.subagents && subagentItems.length > 0 ? `${subagentItems.length} sub-agent` : "",
      c.subagents && c.standing && subagentStanding.length > 0 ? `${subagentStanding.length} sub-agent standing` : "",
      c.subagents && c.schedules && subagentSchedules.length > 0 ? `${subagentSchedules.length} sub-agent schedule` : "",
      c.subagents && c.memory && subagentMemories.length > 0 ? `${subagentMemories.length} sub-agent private memory` : "",
      c.subagents && c.authored_memory && subagentAuthoredMemories.length > 0 ? `${subagentAuthoredMemories.length} sub-agent authored shared memory` : "",
      c.subagents && c.skills && subagentSkills.length > 0 ? `${subagentSkills.length} sub-agent skill` : "",
      c.subagents && c.config && subagentConfigs.length > 0 ? `${subagentConfigs.length} sub-agent config` : "",
      c.subagents && c.workspace && subagentWorkspaces.length > 0 ? `${subagentWorkspaces.length} sub-agent workspace` : "",
    ].filter(Boolean),
    keep: [
      !c.standing && standing.length > 0 ? `${standing.length} standing` : "",
      !c.schedules && scheduleItems.length > 0 ? `${scheduleItems.length} schedule` : "",
      !c.memory && memoryItems.length > 0 ? `${memoryItems.length} private memory` : "",
      !c.authored_memory && authoredMemoryItems.length > 0 ? `${authoredMemoryItems.length} authored shared memory` : "",
      !c.skills && skillItems.length > 0 ? `${skillItems.length} skill` : "",
      !c.config && configItems.length > 0 ? `${configItems.length} config` : "",
      !c.config && (configItems.length > 0 || subagentConfigs.length > 0) ? "shared config access refs" : "",
      !c.workspace && workspaceItems.length > 0 ? `${workspaceItems.length} workspace` : "",
      workflowRefs.length > 0 ? `${workflowRefs.length} workflow reference` : "",
      mailboxItems.length > 0 ? `${mailboxItems.length} mailbox/audit messages` : "",
      (!c.subagents || !c.standing) && subagentStanding.length > 0 ? `${subagentStanding.length} sub-agent standing` : "",
      (!c.subagents || !c.schedules) && subagentSchedules.length > 0 ? `${subagentSchedules.length} sub-agent schedule` : "",
      (!c.subagents || !c.memory) && subagentMemories.length > 0 ? `${subagentMemories.length} sub-agent private memory` : "",
      (!c.subagents || !c.authored_memory) && subagentAuthoredMemories.length > 0 ? `${subagentAuthoredMemories.length} sub-agent authored shared memory` : "",
      (!c.subagents || !c.skills) && subagentSkills.length > 0 ? `${subagentSkills.length} sub-agent skill` : "",
      (!c.subagents || !c.config) && subagentConfigs.length > 0 ? `${subagentConfigs.length} sub-agent config` : "",
      (!c.subagents || !c.workspace) && subagentWorkspaces.length > 0 ? `${subagentWorkspaces.length} sub-agent workspace` : "",
      subagentWorkflowRefs.length > 0 ? `${subagentWorkflowRefs.length} sub-agent workflow reference` : "",
      subagentMailboxMessages.length > 0 ? `${subagentMailboxMessages.length} sub-agent mailbox/audit messages` : "",
    ].filter(Boolean),
    blockedBySubagents: subagentItems.length > 0 && !c.subagents,
  };
}


export function agentLifecycleDetail(profile: AgentProfile): string {
  if (profile.retired) return "graveyard; will not wake until revived";
  const lifecycle = profile.lifecycle;
  const mode =
    lifecycle?.mode ||
    (lifecycle?.retire_on_complete
      ? "retire_on_complete"
      : "persistent");
  const completed = lifecycle?.completed_cycles || 0;
  const max = lifecycle?.max_cycles || 0;
  const effectiveMode = mode === "persistent" && max > 0 ? "cycle" : mode;
  if (effectiveMode === "cycle") {
    if (max > 0) return `${completed}/${max} cycles complete; retires at max cycles`;
    return completed > 0
      ? `${completed} cycles complete; repeats on each wake`
      : "no completed cycles yet; repeats on each wake";
  }
  if (effectiveMode === "retire_on_complete") {
    return "retires after the next successful completion";
  }
  return "stays alive after successful runs";
}

export function agentNoisePolicyLabel(profile: AgentProfile): string {
  const policy = profile.noise_policy;
  if (!policy && !profile.system) return "default";
  const effective = effectiveAgentNoisePolicy(profile);
  const parts: string[] = [];
  if (effective.silent_on_success) parts.push("silent on success");
  if (effective.disable_memory_writes) parts.push("no memory writes");
  if (effective.min_notify_severity) parts.push(`notify >= ${effective.min_notify_severity}`);
  if (effective.min_notify_interval_sec) parts.push(`cooldown ${effective.min_notify_interval_sec}s`);
  const label = parts.length ? parts.join(" · ") : "default";
  return profile.system ? `${label} · system enforced` : label;
}

function effectiveAgentNoisePolicy(profile: AgentProfile): NonNullable<AgentProfile["noise_policy"]> {
  const policy = profile.noise_policy || {};
  const minSeverity = (profile.system || policy.silent_on_success) && notifySeverityRank(policy.min_notify_severity) < notifySeverityRank("warning")
    ? "warning"
    : policy.min_notify_severity;
  if (!profile.system) return { ...policy, min_notify_severity: minSeverity };
  const minInterval = Math.max(policy.min_notify_interval_sec || 0, 8 * 3600);
  return {
    ...policy,
    silent_on_success: true,
    disable_memory_writes: true,
    min_notify_severity: minSeverity,
    min_notify_interval_sec: minInterval,
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
    case "":
      return 1;
    default:
      return 0;
  }
}

export function agentRetryPolicyDetail(profile: Pick<AgentProfile, "retry_policy">): string {
  const policy = profile.retry_policy;
  const max = policy?.max_attempts || 0;
  if (max <= 1) return "single attempt; no run-level retry";
  const parts = [`up to ${Math.min(max, 10)} attempts`];
  parts.push(`backoff ${policy?.backoff || "none"}`);
  if (policy?.base_delay_sec || policy?.max_delay_sec) {
    const base = policy.base_delay_sec || 0;
    const cap = policy.max_delay_sec || 0;
    parts.push(cap > 0 ? `delay ${base}s..${cap}s` : `delay ${base}s`);
  } else {
    parts.push("no delay");
  }
  const retryOn = (policy?.retry_on || []).map((x) => x.trim()).filter(Boolean);
  parts.push(retryOn.length > 0 ? `retry on ${retryOn.join(", ")}` : "retry on error, timeout");
  return parts.join(" · ");
}

export function agentRepairCommandSummary(
  profile: Pick<AgentProfile, "retry_policy" | "health_policy" | "self_repair">,
  repairStatus?: AgentRepairStatus | null,
): { contract: string; latest: string; cooldown: string } {
  const contract = [
    agentRetryPolicyDetail(profile),
    profile.health_policy?.doctor_agent ? `doctor ${profile.health_policy.doctor_agent}` : "no doctor",
    profile.self_repair?.enabled
      ? `self-repair on${profile.self_repair.max_attempts ? ` ${profile.self_repair.max_attempts}x` : ""}`
      : "self-repair off",
    profile.self_repair?.escalate_to ? `escalate ${profile.self_repair.escalate_to}` : "",
  ].filter(Boolean).join(" · ");
  const latest = repairStatus?.latest
    ? [
        repairStatus.latest.phase || "event",
        repairStatus.latest.mode === "degraded" ? "doctor" : repairStatus.latest.mode === "misconfigured" ? "config" : repairStatus.latest.mode || "",
        repairStatus.latest.error || repairStatus.latest.reason || "",
      ].filter(Boolean).join(" · ")
    : "no repair events yet";
  const nextEligible = repairStatus?.next_eligible_ms || repairStatus?.latest?.next_eligible_ms || 0;
  const cooldown = repairStatus?.cooldown_sec
    ? `cooldown ${repairStatus.cooldown_sec}s`
    : nextEligible && nextEligible > Date.now()
      ? `next eligible ${fmtDateTime(nextEligible)}`
      : "eligible now";
  return { contract, latest, cooldown };
}

function repairAttemptLineage(latest?: AgentRepairEvent): string {
  if (!latest) return "";
  const attempt =
    latest.self_repair_attempt && latest.self_repair_max_attempts
      ? `attempt ${latest.self_repair_attempt}/${latest.self_repair_max_attempts}`
      : latest.self_repair_attempt
        ? `attempt ${latest.self_repair_attempt}`
        : "";
  return [attempt, incidentLineageLabel(latest)].filter(Boolean).join(" · ");
}

export function agentRepairOperationsSummary(
  profile: Pick<AgentProfile, "retry_policy" | "health_policy" | "self_repair">,
  repairStatus?: AgentRepairStatus | null,
): { label: string; detail: string; tone: "good" | "warn" | "bad" | "muted" } {
  const command = agentRepairCommandSummary(profile, repairStatus);
  const latest = repairStatus?.latest;
  const phase = String(latest?.phase || "").toLowerCase();
  const inflight = repairStatus?.inflight_count || 0;
  const guarded =
    (profile.retry_policy?.max_attempts || 0) > 1 ||
    !!profile.health_policy?.doctor_agent ||
    !!profile.self_repair?.enabled;
  const parts = [
    command.contract,
    command.latest !== "no repair events yet" ? `latest ${command.latest}` : "",
    repairAttemptLineage(latest),
    command.cooldown !== "eligible now" ? command.cooldown : "",
  ].filter(Boolean);
  if (inflight > 0 || phase === "queued") {
    return {
      label: "repair in flight",
      detail: [`${inflight || 1} active repair`, ...parts].join(" · "),
      tone: "warn",
    };
  }
  if (phase === "failed" || phase === "attempts_exhausted" || String(latest?.error || "").trim()) {
    return {
      label: phase === "attempts_exhausted" ? "repair exhausted" : "repair failing",
      detail: parts.join(" · ") || "latest autonomous repair failed",
      tone: "bad",
    };
  }
  if (command.cooldown !== "eligible now") {
    return {
      label: "cooldown active",
      detail: parts.join(" · "),
      tone: "warn",
    };
  }
  if (guarded) {
    return {
      label: "repair guarded",
      detail: parts.join(" · "),
      tone: "good",
    };
  }
  return {
    label: "manual repair",
    detail: parts.join(" · ") || "no autonomous retry, doctor, or self-repair policy configured",
    tone: "muted",
  };
}

export function agentRepairDecisionSummary(
  repairStatus?: AgentRepairStatus | null,
): { label: string; detail: string; tone: "good" | "warn" | "bad" | "muted" | "accent" } {
  if (!repairStatus) {
    return {
      label: "decision loading",
      detail: "repair status has not arrived yet",
      tone: "muted",
    };
  }
  const action = repairStatus.next_action;
  const contract = repairStatus.contract;
  const actionKey = String(action?.action || "").toLowerCase();
  const retryOn = (contract?.retry_on || []).map((x) => x.trim()).filter(Boolean);
  const detail = [
    action?.detail || "",
    action?.phase ? `phase ${action.phase}` : "",
    action?.fingerprint ? `fingerprint ${action.fingerprint}` : "",
    action?.delegate_to ? `delegate ${action.delegate_to}` : "",
    contract ? `retry ${contract.retry_attempts || 1}x ${contract.retry_backoff || "none"}` : "",
    retryOn.length > 0 ? `signals ${retryOn.join(", ")}` : "",
    contract?.doctor_agent ? `doctor ${contract.doctor_agent}` : "",
    contract?.failure_threshold ? `threshold ${contract.failure_threshold}` : "",
    contract?.self_repair_enabled
      ? `self-repair ${contract.self_repair_attempts || 0}x`
      : contract
        ? "self-repair off"
        : "",
    contract?.escalate_to ? `escalate ${contract.escalate_to}` : "",
    contract?.cooldown_sec ? `cooldown ${contract.cooldown_sec}s` : "",
    contract?.authority_boundary || "",
  ].filter(Boolean).join(" · ");
  const rawTone = String(action?.tone || "").toLowerCase();
  const tone =
    rawTone === "good" || rawTone === "warn" || rawTone === "bad" || rawTone === "muted" || rawTone === "accent"
      ? rawTone
      : actionKey === "wait_inflight"
        ? "accent"
        : actionKey === "cooldown"
          ? "warn"
          : actionKey === "escalate_owner" || actionKey === "operator_resolution" || actionKey === "revive_required"
            ? "bad"
            : contract?.self_repair_enabled || contract?.doctor_agent
              ? "good"
              : "muted";
  return {
    label: action?.label || action?.action || "manual repair",
    detail: detail || "no repair decision available",
    tone,
  };
}

export function agentHealthContractLedger(
  profile: Pick<AgentProfile, "retry_policy" | "health_policy" | "self_repair" | "retired" | "enabled">,
  health: AgentHealthSnapshot,
  repairStatus?: AgentRepairStatus | null,
  runtime?: Pick<AgentCardRuntimeSummary, "retryCount" | "retryText" | "retryDetail" | "repairInflight" | "nextRepairEligibleMs"> | null,
): AgentControlCenterEntry[] {
  const retryMax = profile.retry_policy?.max_attempts || 1;
  const retryOn = (profile.retry_policy?.retry_on || []).map((x) => x.trim()).filter(Boolean);
  const latest = repairStatus?.latest;
  const nextEligible = repairStatus?.next_eligible_ms || latest?.next_eligible_ms || runtime?.nextRepairEligibleMs || 0;
  const repairInflight = repairStatus?.inflight_count || runtime?.repairInflight || 0;
  const selfRepairOn = profile.self_repair?.enabled === true;
  const doctor = profile.health_policy?.doctor_agent || health.doctorAgent || "";
  const retryDetail = [
    agentRetryPolicyDetail(profile),
    retryOn.length > 0 ? `signals ${retryOn.join(", ")}` : "signals error, timeout",
    runtime?.retryText && runtime.retryText !== "no retries" ? runtime.retryText : "",
    runtime?.retryDetail || "",
  ].filter(Boolean).join(" · ");
  const doctorDetail = [
    doctor ? `doctor ${doctor}` : "no doctor agent assigned",
    profile.health_policy?.failure_threshold ? `threshold ${profile.health_policy.failure_threshold}` : "",
    profile.health_policy?.failure_window ? `window ${profile.health_policy.failure_window}` : "",
    profile.health_policy?.stale_after_sec ? `stale after ${profile.health_policy.stale_after_sec}s` : "",
    health.detail,
  ].filter(Boolean).join(" · ");
  const repairDetail = [
    selfRepairOn
      ? `enabled${profile.self_repair?.max_attempts ? ` · ${profile.self_repair.max_attempts} attempts` : ""}`
      : "disabled",
    profile.self_repair?.escalate_to ? `escalates to ${profile.self_repair.escalate_to}` : "no escalation owner",
    repairInflight > 0 ? `${repairInflight} inflight` : "",
    latest?.phase ? `latest ${latest.phase}` : "",
    latest?.error || latest?.reason || "",
    nextEligible && nextEligible > Date.now() ? `next eligible ${fmtDateTime(nextEligible)}` : "",
  ].filter(Boolean).join(" · ");
  const lifeDetail = profile.retired
    ? "graveyard agents stay asleep until revived; retry, doctor, and repair are suspended"
    : profile.enabled === false
      ? "paused agents keep the contract but do not wake until resumed"
      : repairInflight > 0
        ? "repair loop owns the current wake"
        : "eligible for schedule, event, mailbox, delegation, or operator wake under this contract";
  return [
    {
      label: "retry",
      value: retryMax > 1 ? `${retryMax} attempts` : "single attempt",
      detail: retryDetail,
      tone: retryMax > 1 ? "good" : "warn",
    },
    {
      label: "doctor",
      value: doctor || "manual",
      detail: doctorDetail,
      tone: doctor ? (health.state === "healthy" ? "good" : "warn") : "warn",
    },
    {
      label: "self-repair",
      value: selfRepairOn ? "armed" : "manual",
      detail: repairDetail,
      tone: repairInflight > 0 || nextEligible > Date.now() ? "warn" : selfRepairOn ? "good" : "muted",
    },
    {
      label: "wake guard",
      value: profile.retired ? "graveyard" : profile.enabled === false ? "paused" : "active",
      detail: lifeDetail,
      tone: profile.retired || profile.enabled === false ? "muted" : "good",
    },
  ];
}

export function agentOperationsPassport(
  profile: Pick<AgentProfile, "enabled" | "retired" | "direct_callable" | "kind" | "managed" | "parent_agent" | "owner_agent">,
  runtime: Pick<AgentCardRuntimeSummary, "activeRunCount" | "operationalText" | "wakeText" | "wakeDetail" | "nextWakeMs">,
  mailbox: { value: string; detail: string; tone: "good" | "warn" | "bad" | "muted" },
  authority: { detail: string; level?: "open" | "tight" | "scoped" | string },
  config: { value: string; detail: string; tone: "good" | "warn" | "bad" | "muted" },
  repair: { label: string; detail: string; tone: "good" | "warn" | "bad" | "muted" },
): { value: string; detail: string; tone: "good" | "warn" | "bad" | "muted" } {
  const manager = profile.parent_agent || profile.owner_agent || "";
  const managed = agentManagedSubagent(profile);
  const presence = profile.retired
    ? "graveyard"
    : profile.enabled === false
      ? "paused"
      : runtime.activeRunCount > 0
        ? "awake"
        : runtime.operationalText || "sleeping";
  const route = managed
    ? manager
      ? `managed by ${manager}`
      : "managed without owner"
    : "direct callable";
  const wake = [
    runtime.wakeText || "manual wake",
    runtime.wakeDetail || "",
    runtime.nextWakeMs ? `next ${fmtDue(runtime.nextWakeMs)}` : "",
  ].filter(Boolean).join(" · ");
  const bad =
    profile.retired ||
    profile.enabled === false ||
    mailbox.tone === "bad" ||
    config.tone === "bad" ||
    repair.tone === "bad" ||
    (managed && !manager);
  const warn =
    mailbox.tone === "warn" ||
    config.tone === "warn" ||
    repair.tone === "warn" ||
    authority.level === "open";
  const autonomous =
    !bad &&
    mailbox.tone === "good" &&
    (repair.tone === "good" || repair.label === "repair guarded") &&
    authority.level !== "open";
  const value = bad
    ? "operator attention"
    : runtime.activeRunCount > 0
      ? "operating now"
      : autonomous
        ? "autonomous ready"
        : warn
          ? "guarded standby"
          : "standby";
  const detail = [
    presence,
    route,
    wake,
    mailbox.value,
    repair.label,
    authority.detail,
    config.value,
  ].filter(Boolean).join(" · ");
  return {
    value,
    detail,
    tone: bad ? "bad" : warn ? "warn" : autonomous ? "good" : "muted",
  };
}

export function agentEntityContractLedger(
  slug: string,
  profile: Pick<
    AgentProfile,
    | "enabled"
    | "retired"
    | "system"
    | "kind"
    | "managed"
    | "direct_callable"
    | "parent_agent"
    | "owner_agent"
    | "lifecycle"
    | "tasklist"
    | "workdir"
    | "memory_scope"
    | "tool_allow"
    | "tool_deny"
    | "config_overrides"
  >,
  runtime: Pick<AgentCardRuntimeSummary, "activeRunCount" | "operationalText" | "wakeText" | "wakeDetail" | "nextWakeMs">,
  mailbox: { value: string; detail: string; tone: "good" | "warn" | "bad" | "muted" },
  authority: { detail: string; level?: "open" | "tight" | "scoped" | string },
  config: { value: string; detail: string; tone: "good" | "warn" | "bad" | "muted" },
  repair: { label: string; detail: string; tone: "good" | "warn" | "bad" | "muted" },
): AgentEntityContractEntry[] {
  const managed = agentManagedSubagent(profile);
  const manager = profile.parent_agent || profile.owner_agent || "";
  const kind = profile.system || profile.kind === "system"
    ? "system agent"
    : profile.kind === "subagent"
      ? "subagent"
      : "agent";
  const presence = profile.retired
    ? "graveyard"
    : profile.enabled === false
      ? "paused"
      : runtime.activeRunCount > 0
        ? "awake"
        : runtime.operationalText || "sleeping";
  const wake = [
    runtime.wakeText || mailbox.value || "manual wake",
    runtime.wakeDetail || mailbox.detail || "",
    runtime.nextWakeMs ? `next ${fmtDue(runtime.nextWakeMs)}` : "",
  ].filter(Boolean).join(" · ");
  const resources = agentResourcePassportDetail(profile, slug);
  return [
    {
      label: "identity",
      value: managed ? `${kind} · managed` : `${kind} · direct`,
      detail: [slug, agentHierarchySummary(profile), agentTaskContractSummary(profile)].filter(Boolean).join(" · "),
      tone: profile.retired || profile.enabled === false ? "bad" : managed ? "accent" : "good",
    },
    {
      label: "wake",
      value: presence,
      detail: wake,
      tone: profile.retired || profile.enabled === false ? "bad" : runtime.activeRunCount > 0 ? "accent" : mailbox.tone,
    },
    {
      label: "authority",
      value: authority.level === "open" ? "open capability" : authority.level === "tight" ? "tight capability" : "scoped capability",
      detail: authority.detail,
      tone: authority.level === "open" ? "warn" : authority.level === "tight" ? "good" : "muted",
    },
    {
      label: "ownership",
      value: managed ? (manager ? `leader ${manager}` : "leader missing") : "operator callable",
      detail: managed
        ? manager
          ? `sub-agent wake is routed through ${manager}`
          : "managed sub-agent has no parent or owner configured"
        : "operator, schedule, mailbox, workflow, or another agent may wake it when policy allows",
      tone: managed && !manager ? "bad" : managed ? "accent" : "good",
    },
    {
      label: "resources",
      value: resources.split(" · ").slice(0, 2).join(" · "),
      detail: resources,
      tone: resources.includes("blocked") || resources.includes("not allowlisted") ? "warn" : "good",
    },
    {
      label: "recovery",
      value: repair.label,
      detail: [repair.detail, config.value, config.detail].filter(Boolean).join(" · "),
      tone: repair.tone === "bad" || config.tone === "bad"
        ? "bad"
        : repair.tone === "warn" || config.tone === "warn"
          ? "warn"
          : repair.tone === "good"
            ? "good"
            : "muted",
    },
  ];
}

export function agentAutonomyRunbook(
  profile: Pick<
    AgentProfile,
    | "enabled"
    | "retired"
    | "kind"
    | "managed"
    | "direct_callable"
    | "parent_agent"
    | "owner_agent"
    | "retry_policy"
    | "health_policy"
    | "self_repair"
    | "lifecycle"
  >,
  runtime: Pick<AgentCardRuntimeSummary, "activeRunCount" | "activePhase" | "operationalText" | "wakeText" | "wakeDetail" | "nextWakeMs" | "lastAutonomyRunbook">,
  mailbox: { value: string; detail: string; tone: "good" | "warn" | "bad" | "muted" },
  wakePolicy: Pick<WakeAccessSummary, "status" | "passport" | "detail" | "operatorAllowed" | "scheduleAllowed" | "channelAllowed" | "delegationAllowed" | "delegationDetail" | "tone">,
  repair: { label: string; detail: string; tone: "good" | "warn" | "bad" | "muted" },
): AgentAutonomyRunbookEntry[] {
  const manager = profile.parent_agent || profile.owner_agent || "";
  const managed = agentManagedSubagent(profile);
  const directWakeCount = [wakePolicy.operatorAllowed, wakePolicy.scheduleAllowed, wakePolicy.channelAllowed].filter(Boolean).length;
  const active = runtime.activeRunCount || 0;
  const lastRunbook = runtime.lastAutonomyRunbook;
  const retryMax = profile.retry_policy?.max_attempts || 1;
  const doctor = profile.health_policy?.doctor_agent || "";
  const selfRepair = profile.self_repair?.enabled === true;
  const lifecycleMode = profile.retired
    ? "graveyard"
    : profile.enabled === false
      ? "paused"
      : profile.lifecycle?.mode === "retire_on_complete" || profile.lifecycle?.retire_on_complete
        ? "one-shot"
        : profile.lifecycle?.mode === "cycle"
          ? "cycle"
          : "persistent";
  return [
    {
      label: "trigger",
      value: profile.retired ? "blocked" : profile.enabled === false ? "paused" : directWakeCount > 0 ? `${directWakeCount} direct routes` : "delegation only",
      detail: [wakePolicy.passport, wakePolicy.detail].filter(Boolean).join(" · "),
      tone: profile.retired || profile.enabled === false ? "bad" : directWakeCount > 0 ? "good" : wakePolicy.delegationAllowed ? "warn" : "bad",
    },
    {
      label: "route",
      value: managed ? (manager ? `leader ${manager}` : "leader missing") : "self-owned",
      detail: managed
        ? manager
          ? `sub-agent receives wake through ${manager}; direct schedule/operator/channel wake stays blocked`
          : "managed sub-agent has no parent/owner route"
        : "agent owns its wake contract; schedules, mailbox, workflows, and operators invoke the same identity",
      tone: managed && !manager ? "bad" : managed ? "accent" : "good",
    },
    {
      label: "mailbox",
      value: mailbox.value,
      detail: mailbox.detail,
      tone: mailbox.tone === "bad" ? "bad" : mailbox.tone === "warn" ? "warn" : mailbox.tone === "good" ? "good" : "muted",
    },
    {
      label: "execution",
      value: active > 0 ? `awake${runtime.activePhase ? ` · ${runtime.activePhase}` : ""}` : runtime.operationalText || "sleeping",
      detail: [
        active > 0 ? `${active} active run${active === 1 ? "" : "s"}` : "sleeps between wake events",
        runtime.wakeText || "",
        runtime.wakeDetail || "",
        runtime.nextWakeMs ? `next ${fmtDue(runtime.nextWakeMs)}` : "",
        lastRunbook?.phase ? `last contract ${lastRunbook.phase}` : "",
        lastAutonomyRunbookSourceLabel(lastRunbook),
        lastRunbook?.correlation_id ? `corr ${lastRunbook.correlation_id}` : "",
      ].filter(Boolean).join(" · "),
      tone: active > 0 ? "accent" : profile.retired || profile.enabled === false ? "bad" : "muted",
    },
    {
      label: "recovery",
      value: selfRepair ? "self-repair" : doctor ? "doctor" : retryMax > 1 ? "retry" : "manual",
      detail: [
        retryMax > 1 ? `${retryMax} retry attempts` : "single attempt",
        doctor ? `doctor ${doctor}` : "",
        selfRepair ? `self-repair ${profile.self_repair?.max_attempts || 1}x` : "",
        lastRunbook?.recovery_contract ? `journal ${lastRunbook.recovery_contract}` : "",
        repair.label,
        repair.detail,
      ].filter(Boolean).join(" · "),
      tone: repair.tone === "bad" ? "bad" : selfRepair || doctor || retryMax > 1 ? "good" : "warn",
    },
    {
      label: "sleep",
      value: lifecycleMode,
      detail: lifecycleMode === "graveyard"
        ? "identity remains inspectable but cannot wake until revived"
        : lifecycleMode === "paused"
          ? "identity keeps memory and settings but wake routes are suspended"
          : lifecycleMode === "one-shot"
            ? "retires after completing its total task contract"
            : lifecycleMode === "cycle"
              ? "wakes repeatedly and runs every-cycle tasks"
              : [
                  "returns to sleep after each run and waits for the next wake event",
                  lastRunbook?.sleep_contract ? `journal ${lastRunbook.sleep_contract}` : "",
                ].filter(Boolean).join(" · "),
      tone: lifecycleMode === "graveyard" || lifecycleMode === "paused" ? "bad" : lifecycleMode === "one-shot" ? "warn" : "good",
    },
  ];
}

export function agentRuntimeDoctorLedger(
  runtime: Partial<Pick<
    AgentCardRuntimeSummary,
    | "activeRunCount"
    | "activePhase"
    | "liveDetail"
    | "retryText"
    | "retryDetail"
    | "retryTone"
    | "escalationText"
    | "repairIncidentDetail"
  >>,
  repair: { label: string; detail: string; tone: "good" | "warn" | "bad" | "muted" },
  readiness: { value: string; detail: string; tone: "good" | "warn" | "bad" | "muted" },
  escalation: Pick<ReturnType<typeof summarizeEscalations>, "openCount" | "ackedCount" | "doctorOpenCount" | "delegatedOpenCount">,
): AgentRuntimeDoctorLedgerEntry[] {
  const active = runtime.activeRunCount || 0;
  const escalationValue = escalation.openCount > 0
    ? `${escalation.openCount} open`
    : escalation.ackedCount > 0
      ? `${escalation.ackedCount} acked`
      : "none";
  return [
    {
      label: "live",
      value: active > 0 ? `${active} awake${runtime.activePhase ? ` · ${runtime.activePhase}` : ""}` : "sleeping",
      detail: runtime.liveDetail || (active > 0 ? "active run is in progress" : "no active run in the current runtime snapshot"),
      tone: active > 0 ? "accent" : "muted",
    },
    {
      label: "retry",
      value: runtime.retryText || "no retry",
      detail: runtime.retryDetail || "no whole-run retry pressure in the current runtime snapshot",
      tone: runtime.retryTone === "bad" ? "bad" : runtime.retryText ? "warn" : "muted",
    },
    {
      label: "repair",
      value: repair.label,
      detail: [repair.detail, readiness.value, readiness.detail, runtime.repairIncidentDetail].filter(Boolean).join(" · "),
      tone: repair.tone === "bad" ? "bad" : repair.tone === "warn" ? "warn" : repair.tone === "good" ? "good" : readiness.tone,
    },
    {
      label: "escalation",
      value: escalationValue,
      detail: [
        `${escalation.openCount} open`,
        `${escalation.ackedCount} acked`,
        escalation.doctorOpenCount > 0 ? `${escalation.doctorOpenCount} doctor` : "",
        escalation.delegatedOpenCount > 0 ? `${escalation.delegatedOpenCount} delegated` : "",
        runtime.escalationText || "",
      ].filter(Boolean).join(" · "),
      tone: escalation.openCount > 0 ? "bad" : escalation.ackedCount > 0 ? "accent" : "muted",
    },
  ];
}

export function agentSystemGuardianContract(
  profile: Pick<AgentProfile, "system" | "kind" | "slug" | "noise_policy" | "tool_deny" | "max_daily_mc" | "max_cost_mc" | "memory_scope" | "trust_ceiling" | "status">,
): { value: string; detail: string; tone: "good" | "warn" | "bad" } | null {
  if (!profile.system && profile.kind !== "system") return null;
  const safety = systemGuardianSafetySummary({ ...profile, system: true });
  const wakeSchedules = profile.status?.wake_schedule_count || 0;
  const scheduleDetail = wakeSchedules > 0 ? ` · ${wakeSchedules} wake schedule${wakeSchedules === 1 ? "" : "s"}` : "";
  if (!safety.startsWith("review:")) {
    return {
      value: "quiet guardian",
      detail: `${safety}${scheduleDetail}`,
      tone: "good",
    };
  }
  const issues = safety.replace(/^review:\s*/, "");
  const critical = [
    "memory writes enabled",
    "no daily cap",
    "no run cap",
    "daily cap too high",
    "run cap too high",
    "trust above L2",
  ].some((issue) => issues.includes(issue));
  return {
    value: critical ? "guardian intervention" : "guardian review",
    detail: `${issues}${scheduleDetail}`,
    tone: critical ? "bad" : "warn",
  };
}

export function agentResourcePassportDetail(
  profile: Pick<AgentProfile, "workdir" | "memory_scope" | "tool_allow" | "tool_deny" | "config_overrides">,
  slug: string,
): string {
  const allow = (profile.tool_allow || []).map((x) => x.trim().toLowerCase()).filter(Boolean);
  const deny = (profile.tool_deny || []).map((x) => x.trim().toLowerCase()).filter(Boolean);
  const workspace = profile.workdir?.trim() ? `workspace ${profile.workdir.trim()}` : "shared workspace";
  const memory = `memory ${agentScope(slug, profile.memory_scope)}`;
  const dbDenied = deny.includes("db") || deny.includes("data") || deny.includes("datalake");
  const dbAllowlisted = allow.length === 0 || allow.includes("db") || allow.includes("data") || allow.includes("datalake");
  const data = dbDenied ? "data lake blocked" : dbAllowlisted ? "data lake via db" : "data lake not allowlisted";
  const cfgCount = Object.keys(profile.config_overrides || {}).length;
  const config = cfgCount > 0 ? `${cfgCount} config override${cfgCount === 1 ? "" : "s"}` : "default config";
  return `${workspace} · ${memory} · ${data} · ${config}`;
}

