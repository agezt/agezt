import { Waypoints } from "lucide-react";
import { cn } from "@/lib/utils";
import { type AgentProfile } from "@/views/Roster";
import { AgentConfigPermissionRow, AgentPermissionRow, AgentWakeAccess, ToolCatalogRow } from "@/components/agentdetail/shared";

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

