import { scheduleAgentSlug } from "@/lib/fleet";
import { trustRank, type AgentProfile, type RosterSchedule } from "./shared";

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
