import type { AgentCardRuntimeSummary } from "@/lib/agentdetail";
import { agentIdentityKind, type AgentProfile, type AgentTask, type RosterBoardMessage } from "./shared";

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
