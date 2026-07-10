import { agentIdentityKind, type AgentProfile } from "./shared";
import { agentSchedulePressurePassport, systemGuardianSafetySummary } from "./guardians";

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
