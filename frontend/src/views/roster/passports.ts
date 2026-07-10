import { fmtDateTime, fmtDue } from "@/lib/utils";
import { highImpactToolNames, type ApiOrder } from "@/lib/fleet";
import { isChainRef } from "@/lib/chains";
import type { AgentCardRuntimeSummary } from "@/lib/agentdetail";
import { agentIdentityKind, formatWakeDue, plural, trustRank, type AgentProfile, type AgentTask, type RosterBoardMessage } from "./shared";

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
