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

