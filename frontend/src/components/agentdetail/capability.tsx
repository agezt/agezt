import { Waypoints } from "lucide-react";
import { cn } from "@/lib/utils";
import { money } from "@/lib/format";
import { type AgentControlCenterEntry, type AgentProfile } from "@/views/Roster";
import { highImpactToolNames } from "@/lib/fleet";
import { agentScope } from "@/lib/agentdetail";
import { AgentConfigPermissionRow, AgentPermissionRow, AgentPermissionsSnapshot, AgentWakeAccess, ToolCatalogRow } from "@/components/agentdetail/shared";

export interface EffectiveToolPermission {
  name: string;
  description: string;
  capability: string;
  allowed: boolean;
  ask: boolean;
  label: string;
  reason: string;
}

interface WorkflowToolAccessSummary {
  label: string;
  detail: string;
  tone: "good" | "warn" | "bad" | "muted";
}

export interface AgentControlInterventionSummary {
  label: string;
  detail: string;
  tone: "good" | "warn" | "bad" | "muted";
}

export interface AgentAuthorityContractSummary {
  label: string;
  detail: string;
  tone: "good" | "warn" | "bad" | "muted";
}

export interface AgentCapabilityRiskPassport {
  label: string;
  detail: string;
  tone: "good" | "warn" | "bad" | "muted";
}

export interface AgentAuthorityLedgerEntry {
  label: string;
  value: string;
  detail: string;
  tone: "good" | "warn" | "bad" | "muted";
}

export interface AgentAuthorityManifest {
  label: string;
  detail: string;
  tone: "good" | "warn" | "bad" | "muted";
  fields: {
    boundary: string;
    tools: string;
    workflow: string;
    data: string;
    config: string;
    memory: string;
    execution: string;
  };
}

export interface AgentRuntimeDoctorLedgerEntry {
  label: string;
  value: string;
  detail: string;
  tone: "good" | "warn" | "bad" | "accent" | "muted";
}

export interface AgentEntityContractEntry {
  label: string;
  value: string;
  detail: string;
  tone: "good" | "warn" | "bad" | "accent" | "muted";
}

export interface AgentAutonomyRunbookEntry {
  label: string;
  value: string;
  detail: string;
  tone: "good" | "warn" | "bad" | "accent" | "muted";
}

function compactNameList(items: string[] | undefined, fallback: string, max = 3): string {
  const clean = (items || []).map((item) => item.trim()).filter(Boolean);
  if (clean.length === 0) return fallback;
  const head = clean.slice(0, max).join(", ");
  const rest = clean.length - max;
  return rest > 0 ? `${head} +${rest}` : head;
}

export function agentControlInterventionSummary(entries: AgentControlCenterEntry[]): AgentControlInterventionSummary {
  const bad = entries.filter((entry) => entry.tone === "bad");
  const warn = entries.filter((entry) => entry.tone === "warn");
  const named = (rows: AgentControlCenterEntry[]) => rows.map((entry) => entry.label).join(", ");
  if (bad.length > 0) {
    return {
      label: "control blocked",
      detail: `${named(bad)} require intervention; open capability control to repair access, trust, memory, config, or noise policy`,
      tone: "bad",
    };
  }
  if (warn.length > 0) {
    return {
      label: "control review",
      detail: `${named(warn)} need review; open capability control to adjust tool allow/deny, trust, memory, config, or noise policy`,
      tone: "warn",
    };
  }
  if (entries.length === 0) {
    return {
      label: "control unknown",
      detail: "control center has not loaded identity policy entries yet",
      tone: "muted",
    };
  }
  return {
    label: "control ready",
    detail: "tools, trust, data lake, memory, config, and noise policy have explicit operating posture",
    tone: "good",
  };
}

export function agentAuthorityContractSummary(
  profile: Pick<AgentProfile, "trust_ceiling" | "tool_allow" | "tool_deny">,
  permissions: EffectiveToolPermission[],
  configEntries: Pick<AgentConfigPermissionRow, "visible">[] = [],
): AgentAuthorityContractSummary {
  const trust = profile.trust_ceiling || "L4";
  const direct = permissions.filter((row) => row.allowed && !row.ask).length;
  const ask = permissions.filter((row) => row.allowed && row.ask).length;
  const blocked = permissions.filter((row) => !row.allowed).length;
  const workflow = permissions.find((row) => row.name === "workflow");
  const db = permissions.find((row) => row.name === "db");
  const configVisible = configEntries.filter((row) => row.visible).length;
  const explicit = (profile.tool_allow || []).length + (profile.tool_deny || []).length;
  const workflowText = workflow ? (workflow.allowed ? (workflow.ask ? "workflow ask" : "workflow direct") : "workflow blocked") : "workflow absent";
  const dataText = db ? (db.allowed ? (db.ask ? "data lake ask" : "data lake direct") : "data lake blocked") : "data lake absent";
  const configText = configEntries.length > 0 ? `config ${configVisible}/${configEntries.length}` : "config unknown";
  if (permissions.length === 0) {
    return {
      label: explicit > 0 || trust !== "L4" ? "configured authority" : "authority unknown",
      detail: `${explicit} explicit rule${explicit === 1 ? "" : "s"} · ${configText} · trust ${trust}`,
      tone: explicit > 0 || trust !== "L4" ? "warn" : "muted",
    };
  }
  const open = blocked === 0 && ask === 0 && explicit === 0 && trust === "L4";
  const locked = direct === 0 && ask === 0 && blocked > 0;
  return {
    label: locked ? "locked authority" : open ? "open authority" : ask > 0 || blocked > 0 || explicit > 0 || trust !== "L4" ? "managed authority" : "direct authority",
    detail: `${direct} direct · ${ask} ask · ${blocked} blocked · ${workflowText} · ${dataText} · ${configText} · trust ${trust}`,
    tone: locked ? "bad" : open ? "warn" : ask > 0 || blocked > 0 || explicit > 0 || trust !== "L4" ? "good" : "muted",
  };
}

export function agentAuthorityManifest(
  profile: Pick<AgentProfile, "trust_ceiling" | "tool_allow" | "tool_deny" | "memory_scope" | "noise_policy" | "system" | "kind">,
  permissions: EffectiveToolPermission[],
  snapshot: Pick<AgentPermissionsSnapshot, "config_entries" | "governance"> | null,
  slug = "agent",
): AgentAuthorityManifest {
  const governance = snapshot?.governance;
  const trust = profile.trust_ceiling || governance?.trust_ceiling || "L4";
  const directRows = permissions.filter((row) => row.allowed && !row.ask);
  const askRows = permissions.filter((row) => row.allowed && row.ask);
  const blockedRows = permissions.filter((row) => !row.allowed);
  const highDirect = highImpactToolNames(directRows.map((row) => row.name));
  const workflow = permissions.find((row) => row.name.toLowerCase() === "workflow");
  const data = permissions.find((row) => ["db", "data", "datalake"].includes(row.name.toLowerCase()));
  const memoryTool = permissions.find((row) => row.name.toLowerCase() === "memory");
  const configEntries = snapshot?.config_entries || [];
  const visibleConfig = configEntries.filter((row) => row.visible).length;
  const ownedConfig = configEntries.filter((row) => row.owned).length;
  const hiddenConfig = configEntries.length - visibleConfig;
  const explicit = (profile.tool_allow || []).length + (profile.tool_deny || []).length;
  const boundary =
    governance?.authority_boundary ||
    (profile.system || profile.kind === "system" ? "system guardian boundary" : "agent identity boundary");
  const tools =
    governance?.tool_policy ||
    (permissions.length > 0
      ? `${directRows.length} direct · ${askRows.length} ask · ${blockedRows.length} blocked · trust ${trust}`
      : `${explicit} explicit rule${explicit === 1 ? "" : "s"} · trust ${trust}`);
  const workflowText = workflow ? (workflow.allowed ? (workflow.ask ? "workflow ask" : "workflow direct") : "workflow blocked") : "workflow absent";
  const dataText = data ? (data.allowed ? (data.ask ? "data lake ask" : "data lake direct") : "data lake blocked") : "data lake absent";
  const configText = snapshot
    ? [
        `${visibleConfig}/${configEntries.length} visible`,
        ownedConfig > 0 ? `${ownedConfig} owned` : "",
        hiddenConfig > 0 ? `${hiddenConfig} blocked` : "",
      ].filter(Boolean).join(" · ")
    : "config loading";
  const memoryWrites =
    governance?.memory_writes ||
    (profile.noise_policy?.disable_memory_writes || memoryTool?.allowed === false ? "writes disabled" : "writes enabled");
  const memoryText = governance?.memory_policy || `${agentScope(slug, profile.memory_scope)} · ${memoryWrites}`;
  const execution = governance?.execution_boundary || "schedules, workflows, tools, and mailbox wake invoke through this policy";
  const fields = {
    boundary,
    tools,
    workflow: workflowText,
    data: dataText,
    config: configText,
    memory: memoryText,
    execution,
  };
  const locked = directRows.length === 0 && askRows.length === 0 && blockedRows.length > 0;
  const open = (governance?.risk === "open" || (blockedRows.length === 0 && askRows.length === 0 && explicit === 0 && trust === "L4")) && permissions.length > 0;
  const managed = askRows.length > 0 || blockedRows.length > 0 || explicit > 0 || trust !== "L4" || hiddenConfig > 0;
  const tone = locked || trust === "L0" ? "bad" : open || highDirect.length > 0 ? "warn" : managed ? "good" : "muted";
  const label =
    locked || trust === "L0"
      ? "locked authority manifest"
      : profile.system || profile.kind === "system" || governance?.risk === "system_guardian"
        ? "system authority manifest"
        : open
          ? "open authority manifest"
          : managed
            ? "managed authority manifest"
            : "authority manifest";
  return {
    label,
    detail: Object.values(fields).join(" · "),
    tone,
    fields,
  };
}

export function agentAuthorityLedger(
  profile: Pick<AgentProfile, "trust_ceiling" | "tool_allow" | "tool_deny" | "memory_scope" | "workdir" | "max_cost_mc" | "max_daily_mc" | "noise_policy">,
  permissions: EffectiveToolPermission[],
  snapshot: Pick<AgentPermissionsSnapshot, "config_entries" | "governance"> | null,
  slug = "agent",
): AgentAuthorityLedgerEntry[] {
  const directRows = permissions.filter((row) => row.allowed && !row.ask);
  const askRows = permissions.filter((row) => row.allowed && row.ask);
  const blockedRows = permissions.filter((row) => !row.allowed);
  const highDirect = highImpactToolNames(directRows.map((row) => row.name));
  const highAsk = highImpactToolNames(askRows.map((row) => row.name));
  const highBlocked = highImpactToolNames(blockedRows.map((row) => row.name));
  const configEntries = snapshot?.config_entries || [];
  const governance = snapshot?.governance;
  const visibleConfig = configEntries.filter((row) => row.visible).length;
  const ownedConfig = configEntries.filter((row) => row.owned).length;
  const hiddenConfig = configEntries.length - visibleConfig;
  const memoryScope = agentScope(slug, profile.memory_scope);
  const memoryTool = permissions.find((row) => row.name.toLowerCase() === "memory");
  const memoryBlocked = profile.noise_policy?.disable_memory_writes || memoryTool?.allowed === false;
  const workspace = profile.workdir?.trim() || "shared workspace";
  const runCap = profile.max_cost_mc || 0;
  const dailyCap = profile.max_daily_mc || 0;
  const noise = profile.noise_policy || {};
  const notify = noise.min_notify_severity || "info";
  const cooldown = noise.min_notify_interval_sec || 0;
  const trust = profile.trust_ceiling || "L4";
  const toolValue =
    permissions.length === 0
      ? "tools loading"
      : highDirect.length > 0
        ? `direct high-impact ${highDirect.length}`
        : highAsk.length > 0
          ? `ask high-impact ${highAsk.length}`
          : highBlocked.length > 0 || blockedRows.length > 0
            ? "governed tools"
            : "broad tools";
  const toolDetail =
    permissions.length === 0
      ? `${(profile.tool_allow || []).length} allow rules, ${(profile.tool_deny || []).length} deny rules`
      : [
          `${directRows.length} direct`,
          `${askRows.length} ask`,
          `${blockedRows.length} blocked`,
          `direct: ${compactNameList(governance?.direct_tools ?? directRows.map((row) => row.name), "none")}`,
          `ask: ${compactNameList(governance?.ask_tools ?? askRows.map((row) => row.name), "none")}`,
          `blocked: ${compactNameList(governance?.blocked_tools ?? blockedRows.map((row) => row.name), "none")}`,
          highDirect.length > 0 ? `high direct: ${highDirect.join(", ")}` : "",
          highAsk.length > 0 ? `high ask: ${highAsk.join(", ")}` : "",
          highBlocked.length > 0 ? `high blocked: ${highBlocked.join(", ")}` : "",
        ].filter(Boolean).join(" · ");
  return [
    {
      label: "tools",
      value: toolValue,
      detail: toolDetail,
      tone: highDirect.length > 0 || (permissions.length > 0 && blockedRows.length === 0 && askRows.length === 0 && (profile.tool_allow || []).length === 0 && trust === "L4")
        ? "warn"
        : highAsk.length > 0 || blockedRows.length > 0 || (profile.tool_allow || []).length > 0 || (profile.tool_deny || []).length > 0
          ? "good"
          : "muted",
    },
    {
      label: "config",
      value: snapshot ? `${visibleConfig}/${configEntries.length} visible` : "config loading",
      detail: snapshot
        ? [
            ownedConfig > 0 ? `${ownedConfig} owned` : "no owned config",
            hiddenConfig > 0 ? `${hiddenConfig} blocked` : "no blocked config",
            `visible: ${compactNameList(governance?.visible_configs ?? configEntries.filter((row) => row.visible).map((row) => row.key), "none", 2)}`,
            `hidden: ${compactNameList(governance?.hidden_configs ?? configEntries.filter((row) => !row.visible).map((row) => row.key), "none", 2)}`,
          ].join(" · ")
        : "config center access has not loaded yet",
      tone: hiddenConfig > 0 ? "warn" : ownedConfig > 0 || visibleConfig > 0 ? "good" : "muted",
    },
    {
      label: "memory",
      value: memoryBlocked ? "writes blocked" : memoryScope,
      detail: memoryBlocked ? `scope ${memoryScope} · memory writes disabled or tool denied` : `scope ${memoryScope} · memory writes available`,
      tone: memoryBlocked ? "warn" : "good",
    },
    {
      label: "workspace",
      value: workspace,
      detail: workspace === "shared workspace" ? "agent has no isolated workspace subdir" : "agent workspace subdir is pinned on identity",
      tone: workspace === "shared workspace" ? "warn" : "good",
    },
    {
      label: "budget",
      value: runCap > 0 || dailyCap > 0 ? `${runCap > 0 ? money(runCap) : "no run cap"} / ${dailyCap > 0 ? money(dailyCap) : "no day cap"}` : "uncapped",
      detail: runCap > 0 || dailyCap > 0 ? "spend caps are stored on the agent identity" : "no run or daily spend ceiling is set",
      tone: runCap > 0 && dailyCap > 0 ? "good" : "warn",
    },
    {
      label: "noise",
      value: noise.silent_on_success ? `quiet >= ${notify}` : `notify >= ${notify}`,
      detail: [
        noise.silent_on_success ? "success notifications silent" : "success notifications enabled",
        noise.disable_memory_writes ? "memory writes disabled" : "memory writes enabled",
        cooldown > 0 ? `cooldown ${cooldown}s` : "no cooldown",
      ].join(" · "),
      tone: noise.silent_on_success && notify !== "info" && cooldown >= 3600 ? "good" : "warn",
    },
    {
      label: "trust",
      value: trust,
      detail: snapshot?.governance?.summary || `trust ceiling ${trust}`,
      tone: trust === "L4" ? "warn" : trust === "L0" ? "bad" : "good",
    },
  ];
}

export function agentCapabilityRiskPassport(
  profile: Pick<AgentProfile, "trust_ceiling" | "tool_allow" | "tool_deny">,
  permissions: EffectiveToolPermission[],
  snapshot: Pick<AgentPermissionsSnapshot, "config_entries" | "governance"> | null,
): AgentCapabilityRiskPassport {
  const trust = profile.trust_ceiling || "L4";
  const explicitRules = (profile.tool_allow || []).length + (profile.tool_deny || []).length;
  const directRows = permissions.filter((row) => row.allowed && !row.ask);
  const askRows = permissions.filter((row) => row.allowed && row.ask);
  const blockedRows = permissions.filter((row) => !row.allowed);
  const highDirect = highImpactToolNames(directRows.map((row) => row.name));
  const highAsk = highImpactToolNames(askRows.map((row) => row.name));
  const highBlocked = highImpactToolNames(blockedRows.map((row) => row.name));
  const configEntries = snapshot?.config_entries || [];
  const visibleConfig = configEntries.filter((row) => row.visible).length;
  const configText = configEntries.length > 0 ? `config ${visibleConfig}/${configEntries.length}` : "config unknown";
  const governanceRisk = snapshot?.governance?.risk || "";
  const backendDirect = compactNameList(snapshot?.governance?.direct_tools ?? directRows.map((row) => row.name), "none");
  const backendAsk = compactNameList(snapshot?.governance?.ask_tools ?? askRows.map((row) => row.name), "none");
  const backendBlocked = compactNameList(snapshot?.governance?.blocked_tools ?? blockedRows.map((row) => row.name), "none");
  if (permissions.length === 0 && !snapshot) {
    return {
      label: "risk loading",
      detail: `${explicitRules} explicit rule${explicitRules === 1 ? "" : "s"} · trust ${trust} · ${configText}`,
      tone: "muted",
    };
  }
  if (directRows.length === 0 && askRows.length === 0 && blockedRows.length > 0) {
    return {
      label: "locked down",
      detail: `${blockedRows.length} blocked/hidden tools (${backendBlocked}) · ${configText} · trust ${trust}`,
      tone: "bad",
    };
  }
  if (highDirect.length > 0 || governanceRisk === "open") {
    return {
      label: "high authority",
      detail: `${highDirect.length > 0 ? `direct high-impact: ${highDirect.join(", ")}` : "open governance"} · direct ${backendDirect} · ask ${backendAsk} · ${configText} · trust ${trust}`,
      tone: "warn",
    };
  }
  if (highAsk.length > 0) {
    return {
      label: "guarded high authority",
      detail: `ask-gated high-impact: ${highAsk.join(", ")} · direct ${backendDirect} · ask ${backendAsk} · ${configText} · trust ${trust}`,
      tone: "warn",
    };
  }
  if (highBlocked.length > 0 || explicitRules > 0 || trust !== "L4" || governanceRisk === "restricted") {
    return {
      label: "governed authority",
      detail: `direct ${backendDirect} · ask ${backendAsk} · blocked ${backendBlocked} · ${configText} · trust ${trust}`,
      tone: "good",
    };
  }
  return {
    label: "broad authority",
    detail: `direct ${backendDirect} · ask ${backendAsk} · blocked ${backendBlocked} · ${configText} · trust ${trust}`,
    tone: "warn",
  };
}

export function workflowToolAccessSummary(permissions: EffectiveToolPermission[]): WorkflowToolAccessSummary {
  const row = permissions.find((p) => p.name === "workflow");
  if (!row) {
    return {
      label: "workflow tool not registered",
      detail: "agent cannot author or run workflow chains through the workflow tool in this daemon snapshot",
      tone: "muted",
    };
  }
  if (row.allowed && row.ask) {
    return {
      label: "workflow chains ask-gated",
      detail: row.reason || "workflow tool requires approval before this agent can author or run chains",
      tone: "warn",
    };
  }
  if (row.allowed) {
    return {
      label: "workflow chains available",
      detail: row.reason || "agent can author and run durable workflow chains through the workflow tool",
      tone: "good",
    };
  }
  return {
    label: "workflow chains blocked",
    detail: row.reason || "workflow tool is blocked or hidden for this agent",
    tone: "bad",
  };
}

export function WorkflowToolAccessCard({ summary }: { summary: WorkflowToolAccessSummary }) {
  const tone =
    summary.tone === "good"
      ? "border-good/30 bg-good/5 text-good"
      : summary.tone === "warn"
        ? "border-warn/30 bg-warn/5 text-warn"
        : summary.tone === "bad"
          ? "border-bad/30 bg-bad/5 text-bad"
          : "border-border bg-card/55 text-muted";
  return (
    <div className={cn("mb-2 flex min-w-0 items-start gap-2 rounded-md border px-2 py-1.5 text-[11px]", tone)}>
      <Waypoints className="mt-0.5 size-3 shrink-0" />
      <div className="min-w-0">
        <div className="font-medium text-foreground/85">{summary.label}</div>
        <div className="truncate text-muted" title={summary.detail}>{summary.detail}</div>
      </div>
    </div>
  );
}

interface PermissionPassport {
  level: "tight" | "open" | "unknown";
  detail: string;
  policy: string[];
}

interface ToolAuthoritySnapshot {
  headline: string;
  detail: string;
  tone: "good" | "warn" | "bad" | "muted";
}

export interface WakeAccessSummary {
  status: string;
  passport: string;
  detail: string;
  tone: "good" | "warn" | "bad" | "muted";
  operatorAllowed: boolean;
  scheduleAllowed: boolean;
  channelAllowed: boolean;
  delegationAllowed: boolean;
  delegationDetail: string;
}

export function agentDelegationPassportDetail(
  profile: Pick<AgentProfile, "enabled" | "retired" | "kind" | "managed" | "direct_callable" | "parent_agent" | "owner_agent">,
  access?: {
    status?: string;
    reason?: string;
    manager?: string;
    operator_allowed?: boolean;
    schedule_allowed?: boolean;
    channel_allowed?: boolean;
    delegation_allowed?: boolean;
    delegation_scope?: string;
    delegation_sources?: string[];
  },
): { value: string; detail: string; tone: "good" | "warn" | "bad" | "muted" } {
  const managed = agentManagedSubagent(profile) || access?.status === "managed";
  const manager = access?.manager || profile.parent_agent || profile.owner_agent || "";
  if (profile.retired) {
    return { value: "graveyard · blocked", detail: "graveyard agent cannot be woken until revived", tone: "muted" };
  }
  if (profile.enabled === false) {
    return { value: "paused · blocked", detail: "paused agent cannot be woken until resumed", tone: "bad" };
  }
  if (managed) {
    const sources = access?.delegation_sources?.length ? access.delegation_sources : manager ? [manager] : [];
    const allowed = access?.delegation_allowed ?? sources.length > 0;
    const value = sources.length > 0 ? `manager-only · ${sources.join(", ")}` : "manager-only · no manager";
    const detail =
      access?.reason ||
      (allowed
        ? `direct operator, schedule, and channel wake are blocked; delegation is accepted from ${sources.join(", ")}`
        : "direct wake is blocked and no parent/owner delegation source is configured");
    return { value, detail, tone: allowed ? "warn" : "bad" };
  }
  const directAllowed = {
    operator: access?.operator_allowed ?? true,
    schedule: access?.schedule_allowed ?? true,
    channel: access?.channel_allowed ?? true,
    delegation: access?.delegation_allowed ?? true,
  };
  const allowed = Object.entries(directAllowed)
    .filter(([, ok]) => ok)
    .map(([name]) => name);
  const blocked = Object.entries(directAllowed)
    .filter(([, ok]) => !ok)
    .map(([name]) => name);
  const value = blocked.length === 0 ? "operator/schedule/channel" : `allowed ${allowed.join(", ") || "none"}`;
  const detail = blocked.length === 0
    ? "operator, schedule, channel, and delegation wake paths are available"
    : `blocked ${blocked.join(", ")}; allowed ${allowed.join(", ") || "none"}`;
  return { value, detail, tone: blocked.length === 0 ? "good" : allowed.length > 0 ? "warn" : "bad" };
}

export function summarizeWakeAccess(profile: AgentProfile, access?: AgentWakeAccess): WakeAccessSummary {
  const managed = agentManagedSubagent(profile) || access?.status === "managed";
  const direct =
    access?.direct_allowed ??
    (profile.enabled !== false && !profile.retired && !managed);
  const schedule = access?.schedule_allowed ?? direct;
  const channel = access?.channel_allowed ?? direct;
  const operator = access?.operator_allowed ?? direct;
  const delegation =
    access?.delegation_allowed ??
    (profile.enabled !== false && !profile.retired && (profile.direct_callable !== false || !!profile.parent_agent || !!profile.owner_agent));
  const sources = access?.delegation_sources || [profile.parent_agent || "", profile.owner_agent || ""].filter(Boolean);
  const manager = access?.manager || profile.parent_agent || profile.owner_agent || "";
  const status = access?.status || (profile.retired ? "retired" : profile.enabled === false ? "paused" : managed ? "managed" : "direct");
  const delegationDetail =
    access?.delegation_scope === "manager" || managed
      ? sources.length > 0
        ? `manager: ${sources.join(", ")}`
        : "manager only"
      : "any lead";
  const directBits = [
    operator ? "operator" : "",
    schedule ? "schedule" : "",
    channel ? "channel" : "",
  ].filter(Boolean);
  const passport = managed
    ? `managed by ${manager || "parent/owner"}`
    : directBits.length === 3
      ? "direct wake"
      : directBits.length > 0
        ? `direct: ${directBits.join(", ")}`
        : status;
  const detail =
    access?.reason ||
    (managed
      ? `Direct wake is blocked; delegation is accepted from ${delegationDetail}.`
      : "Operator, schedule, channel, and delegation wake paths are available.");
  return {
    status,
    passport,
    detail,
    tone: profile.retired || profile.enabled === false ? "bad" : managed ? "warn" : "good",
    operatorAllowed: operator,
    scheduleAllowed: schedule,
    channelAllowed: channel,
    delegationAllowed: delegation,
    delegationDetail,
  };
}

export function agentManagedSubagent(profile: Pick<AgentProfile, "kind" | "managed" | "direct_callable">): boolean {
  return profile.kind === "subagent" || !!profile.managed || profile.direct_callable === false;
}

export function summarizePermissionPassport(
  profile: AgentProfile,
  permissions: EffectiveToolPermission[],
  loaded: boolean,
): PermissionPassport {
  const allow = profile.tool_allow || [];
  const deny = profile.tool_deny || [];
  const policy = [
    `trust ${profile.trust_ceiling || "L4"}`,
    allow.length > 0 ? `allow ${allow.length}` : "all tools visible",
    deny.length > 0 ? `deny ${deny.length}` : "deny 0",
    profile.noise_policy?.disable_memory_writes ? "memory writes off" : "",
    profile.noise_policy?.min_notify_severity ? `notify >= ${profile.noise_policy.min_notify_severity}` : "",
    profile.config_overrides && Object.keys(profile.config_overrides).length > 0
      ? `${Object.keys(profile.config_overrides).length} config override${Object.keys(profile.config_overrides).length === 1 ? "" : "s"}`
      : "",
  ].filter(Boolean);
  if (!loaded) {
    return {
      level: allow.length > 0 || deny.length > 0 || profile.trust_ceiling ? "tight" : "unknown",
      detail: "permission table loading",
      policy,
    };
  }
  if (permissions.length === 0) {
    return {
      level: allow.length > 0 || deny.length > 0 || profile.trust_ceiling ? "tight" : "unknown",
      detail: "no registered tools reported for this daemon",
      policy,
    };
  }
  const askRows = permissions.filter((row) => row.allowed && row.ask);
  const allowedRows = permissions.filter((row) => row.allowed && !row.ask);
  const blockedRows = permissions.filter((row) => !row.allowed);
  const ask = askRows.length;
  const allowed = allowedRows.length;
  const blocked = blockedRows.length;
  const highAllowed = highImpactToolNames(allowedRows.map((row) => row.name));
  const highAsk = highImpactToolNames(askRows.map((row) => row.name));
  const highBlocked = highImpactToolNames(blockedRows.map((row) => row.name));
  const open = allow.length === 0 && deny.length === 0 && (profile.trust_ceiling || "L4") === "L4" && blocked === 0;
  const highImpact =
    highAllowed.length > 0
      ? `high-impact allowed: ${highAllowed.join(", ")}`
      : highAsk.length > 0
        ? `high-impact ask-gated: ${highAsk.join(", ")}`
        : highBlocked.length > 0
          ? `high-impact blocked: ${highBlocked.join(", ")}`
          : "no high-impact tools registered";
  return {
    level: open ? "open" : "tight",
    detail: `${highImpact} · ${allowed} allowed, ${ask} ask-gated, ${blocked} blocked or hidden out of ${permissions.length} tool${permissions.length === 1 ? "" : "s"}`,
    policy,
  };
}

export function agentToolAuthoritySnapshot(
  profile: Pick<AgentProfile, "trust_ceiling" | "tool_allow" | "tool_deny">,
  permissions: EffectiveToolPermission[],
  loaded: boolean,
): ToolAuthoritySnapshot {
  const ceiling = profile.trust_ceiling || "L4";
  const allow = (profile.tool_allow || []).map((x) => x.trim()).filter(Boolean);
  const deny = (profile.tool_deny || []).map((x) => x.trim()).filter(Boolean);
  const allowedRows = permissions.filter((row) => row.allowed && !row.ask);
  const askRows = permissions.filter((row) => row.allowed && row.ask);
  const blockedRows = permissions.filter((row) => !row.allowed);
  const highAllowed = highImpactToolNames(allowedRows.map((row) => row.name));
  const highAsk = highImpactToolNames(askRows.map((row) => row.name));
  const highBlocked = highImpactToolNames(blockedRows.map((row) => row.name));
  const configuredHighAllow = highImpactToolNames(allow);
  const configuredHighDeny = highImpactToolNames(deny);
  if (!loaded) {
    return {
      headline: "permission table loading",
      detail: allow.length > 0 || deny.length > 0 ? `${allow.length} allow, ${deny.length} deny · trust ${ceiling}` : `trust ${ceiling}`,
      tone: "muted",
    };
  }
  if (permissions.length === 0) {
    if (configuredHighAllow.length > 0) {
      return {
        headline: "high-impact allow configured",
        detail: `${configuredHighAllow.join(", ")} · catalog unavailable · trust ${ceiling}`,
        tone: "warn",
      };
    }
    return {
      headline: "no registered tools",
      detail: allow.length > 0 || deny.length > 0 ? `${allow.length} allow, ${deny.length} deny · trust ${ceiling}` : `trust ${ceiling}`,
      tone: "muted",
    };
  }
  if (highAllowed.length > 0) {
    return {
      headline: "high-impact tools allowed",
      detail: `${highAllowed.join(", ")} · ${allowedRows.length}/${permissions.length} direct · trust ${ceiling}`,
      tone: "warn",
    };
  }
  if (highAsk.length > 0) {
    return {
      headline: "high-impact tools ask-gated",
      detail: `${highAsk.join(", ")} · ${askRows.length} ask-gated · trust ${ceiling}`,
      tone: "warn",
    };
  }
  if (highBlocked.length > 0 || configuredHighDeny.length > 0) {
    const names = highBlocked.length > 0 ? highBlocked : configuredHighDeny;
    return {
      headline: "high-impact tools blocked",
      detail: `${names.join(", ")} · ${blockedRows.length} blocked/hidden · trust ${ceiling}`,
      tone: "good",
    };
  }
  const open = allow.length === 0 && deny.length === 0 && ceiling === "L4" && blockedRows.length === 0;
  return {
    headline: open ? "open broad access" : "restricted low-impact access",
    detail: `${allowedRows.length} direct, ${askRows.length} ask-gated, ${blockedRows.length} blocked/hidden · trust ${ceiling}`,
    tone: open ? "warn" : "good",
  };
}

export function agentConfigAccessSummary(snapshot: AgentPermissionsSnapshot | null): string {
  if (!snapshot) return "loading";
  const entries = snapshot.config_entries || [];
  if (entries.length === 0) return "0 config entries";
  const visible = entries.filter((row) => row.visible).length;
  const owned = entries.filter((row) => row.owned).length;
  const blocked = entries.length - visible;
  return [
    `${visible}/${entries.length} visible`,
    owned > 0 ? `${owned} owned` : "",
    blocked > 0 ? `${blocked} blocked` : "",
  ].filter(Boolean).join(" · ");
}

export function agentConfigAuthorityContract(
  profile: Pick<AgentProfile, "config_overrides">,
  snapshot: Pick<AgentPermissionsSnapshot, "config_entries"> | null,
): { value: string; detail: string; tone: "good" | "warn" | "bad" | "muted" } {
  const overrides = Object.keys(profile.config_overrides || {}).length;
  if (!snapshot) {
    return {
      value: overrides > 0 ? `${overrides} local override${overrides === 1 ? "" : "s"} · config loading` : "config loading",
      detail: "config center access has not loaded yet",
      tone: "muted",
    };
  }
  const entries = snapshot.config_entries || [];
  const visible = entries.filter((row) => row.visible).length;
  const owned = entries.filter((row) => row.owned).length;
  const blocked = entries.length - visible;
  const hiddenSecret = entries.filter((row) => !row.visible && (row.rating || "").toLowerCase() === "secret").length;
  const allowlisted = entries.filter((row) => row.visible && row.source === "config_allowed").length;
  const excluded = entries.filter((row) => row.source === "config_excluded").length;
  const value = [
    `${overrides} local override${overrides === 1 ? "" : "s"}`,
    entries.length > 0 ? `${visible}/${entries.length} visible` : "0 center entries",
    owned > 0 ? `${owned} owned` : "",
    blocked > 0 ? `${blocked} blocked` : "",
  ].filter(Boolean).join(" · ");
  const detail = [
    overrides > 0 ? "agent-local runtime config is set on the identity" : "no agent-local runtime overrides",
    entries.length > 0 ? `${visible} visible config center entr${visible === 1 ? "y" : "ies"}` : "no config center entries reported",
    owned > 0 ? `${owned} owned by this agent` : "no owned entries",
    allowlisted > 0 ? `${allowlisted} allowlisted` : "",
    excluded > 0 ? `${excluded} excluded` : "",
    hiddenSecret > 0 ? `${hiddenSecret} hidden secret${hiddenSecret === 1 ? "" : "s"}` : "",
  ].filter(Boolean).join(" · ");
  return {
    value,
    detail,
    tone: hiddenSecret > 0 || blocked > 0
      ? "warn"
      : overrides > 0 || owned > 0 || allowlisted > 0
        ? "good"
        : "muted",
  };
}

export function agentGovernancePassport(
  snapshot: AgentPermissionsSnapshot | null,
  fallback: string,
): { detail: string; tone: "good" | "bad" | "warn" | "muted" } {
  const governance = snapshot?.governance;
  if (!snapshot) return { detail: "loading", tone: "muted" };
  if (!governance) return { detail: fallback || "policy unknown", tone: "muted" };
  const detail = governance.summary || fallback || "policy unknown";
  if (governance.risk === "open") return { detail, tone: "warn" };
  if (governance.risk === "system_guardian") return { detail, tone: "good" };
  if (governance.blocked_count || governance.ask_count || governance.tool_allow_count || governance.tool_deny_count || governance.trust_ceiling && governance.trust_ceiling !== "L4") {
    return { detail, tone: "good" };
  }
  return { detail, tone: "muted" };
}

export function effectiveToolPermissions(
  tools: ToolCatalogRow[],
  profile: AgentProfile,
  levels: Record<string, string>,
): EffectiveToolPermission[] {
  const allow = new Set((profile.tool_allow || []).map((x) => x.toLowerCase()));
  const deny = new Set((profile.tool_deny || []).map((x) => x.toLowerCase()));
  return [...tools]
    .sort((a, b) => a.name.localeCompare(b.name))
    .map((tool) => {
      const name = tool.name || "";
      const cap = tool.capability || "";
      const nameKey = name.toLowerCase();
      const level = cap ? levels[cap] || "" : "";
      if (deny.has(nameKey)) {
        return effectiveToolRow(tool, false, false, "denied", "agent denylist");
      }
      if (allow.size > 0 && !allow.has(nameKey)) {
        return effectiveToolRow(tool, false, false, "hidden", "not in agent allowlist");
      }
      if (level === "L0") {
        return effectiveToolRow(tool, false, false, "denied", "global Edict L0");
      }
      if (level === "L1" || level === "L2" || level === "L3") {
        return effectiveToolRow(tool, true, true, level, `global Edict ${level}`);
      }
      return effectiveToolRow(tool, true, false, level || "allowed", level ? `global Edict ${level}` : "no explicit Edict level");
    });
}

export function permissionRowFromSnapshot(row: AgentPermissionRow): EffectiveToolPermission {
  const status = row.status || (row.allowed ? "allowed" : "denied");
  return {
    name: row.name || "",
    description: row.description || "",
    capability: row.capability || "",
    allowed: row.allowed === true,
    ask: row.ask === true,
    label: status,
    reason: row.reason || row.source || "",
  };
}

export function configAccessLabel(row: AgentConfigPermissionRow): string {
  const scope =
    row.source === "config_allowed"
      ? row.visible
        ? "allowlisted"
        : "not allowlisted"
      : row.source === "config_excluded"
        ? "excluded"
        : "global";
  return [row.owned ? "owned" : "", scope].filter(Boolean).join(" · ");
}

export function configAccessDetail(row: AgentConfigPermissionRow): string {
  return [
    row.reason || "",
    (row.allowed_agents || []).length > 0 ? `allow: ${(row.allowed_agents || []).join(", ")}` : "",
    (row.excluded_agents || []).length > 0 ? `deny: ${(row.excluded_agents || []).join(", ")}` : "",
  ].filter(Boolean).join(" · ");
}

function effectiveToolRow(
  tool: ToolCatalogRow,
  allowed: boolean,
  ask: boolean,
  label: string,
  reason: string,
): EffectiveToolPermission {
  return {
    name: tool.name || "",
    description: tool.description || "",
    capability: tool.capability || "",
    allowed,
    ask,
    label,
    reason,
  };
}

export function splitCsv(s: string): string[] {
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

export const HIGH_IMPACT_LOCKDOWN_TOOLS = [
  "shell",
  "workflow",
  "fetch",
  "db",
  "browser",
  "mcp",
  "file",
  "tool_forge",
  "homeassistant",
  "notify",
];

export function normalizeNoiseToolPolicy(allow: string[], deny: string[], disableMemoryWrites: boolean): { allow: string[]; deny: string[] } {
  if (!disableMemoryWrites) return { allow, deny };
  return {
    allow: allow.filter((tool) => tool.trim().toLowerCase() !== "memory"),
    deny: addListItem(deny, "memory"),
  };
}

export function mcToUsdInput(mc?: number): string {
  return mc && mc > 0 ? String(mc / 1e9) : "";
}

export function usdToMcInput(value: string): number | null {
  const raw = value.trim().replace(/^\$/, "");
  if (!raw) return 0;
  const parsed = Number.parseFloat(raw);
  if (!Number.isFinite(parsed) || parsed < 0) return null;
  return Math.round(parsed * 1e9);
}

export function toolPolicyOverlap(allow: string[], deny: string[]): string {
  const allowed = new Set(allow.map((x) => x.trim().toLowerCase()).filter(Boolean));
  for (const item of deny) {
    const clean = item.trim();
    if (clean && allowed.has(clean.toLowerCase())) return clean;
  }
  return "";
}

function csvText(items: string[]): string {
  return items.join(", ");
}

export function addCsvItem(text: string, item: string): string {
  const clean = item.trim();
  if (!clean) return text;
  const lower = clean.toLowerCase();
  const items = splitCsv(text).filter((x) => x.toLowerCase() !== lower);
  items.push(clean);
  return csvText(items);
}

export function removeCsvItem(text: string, item: string): string {
  const lower = item.trim().toLowerCase();
  if (!lower) return text;
  return csvText(splitCsv(text).filter((x) => x.toLowerCase() !== lower));
}

export function addCsvItems(text: string, items: string[]): string {
  return items.reduce((acc, item) => addCsvItem(acc, item), text);
}

export function removeCsvItems(text: string, items: string[]): string {
  return items.reduce((acc, item) => removeCsvItem(acc, item), text);
}

export function addListItem(items: string[], item: string): string[] {
  const clean = item.trim();
  if (!clean) return items;
  const lower = clean.toLowerCase();
  return [...items.filter((x) => x.trim().toLowerCase() !== lower), clean];
}

export function removeListItem(items: string[], item: string): string[] {
  const lower = item.trim().toLowerCase();
  if (!lower) return items;
  return items.filter((x) => x.trim().toLowerCase() !== lower);
}

export function configOverridesText(config?: Record<string, string>): string {
  return Object.entries(config || {})
    .map(([k, v]) => `${k}=${v}`)
    .join("\n");
}

export function parseConfigOverridesText(text: string): Record<string, string> | string {
  const out: Record<string, string> = {};
  for (const raw of text.split(/\r?\n/)) {
    const line = raw.trim();
    if (!line) continue;
    const eq = line.indexOf("=");
    if (eq <= 0) return `invalid config override: ${line}`;
    const key = line.slice(0, eq).trim();
    const value = line.slice(eq + 1).trim();
    if (!key) return `invalid config override: ${line}`;
    out[key] = value;
  }
  return out;
}

