import { plural } from "./shared";

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
