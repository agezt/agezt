import { useEffect, useState, type ReactNode } from "react";
import { Activity as ActivityIcon, ScrollText, Anchor, Brain, Bot, Coins, Cpu, Wrench, Skull, Archive, Repeat, Gauge } from "lucide-react";
import { postJSON } from "@/app/api";
import { cn, fmtAgo } from "@/app/utils";
import { money } from "@/app/format";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { useUI } from "@/components/ui/feedback";
import { type AgentProfile } from "@/features/agents/components/Roster";
import { type FleetState, type ApiOrder, type ApiSchedule } from "@/lib/fleet";
import { type AgentConfigOverrideSummary, type MemoryRecord, type SkillLite } from "@/lib/agentdetail";
import type {
  AgentCapabilitiesResponseConfigEntriesItem ,
  AgentCapabilitiesResponseGovernance ,
  AgentCapabilitiesResponsePermissionsItem ,
  AgentCapabilitiesResponseWakeAccess ,
  AgentPermissionsResponse ,
  AgentRepairResponse ,
  ApprovalsLogResponseApprovalsItem ,
  BoardReadResponseMessagesItem ,
  EdictLogResponseDecisionsItem ,
  EdictStatsResponse ,
  RoutingGetResponse ,
  ToolListResponseToolsItem ,
  ToolLogResponseInvocationsItem,
} from "@/app/api/types.generated";

// Diagnostics row shapes (mirrors of /api/policy_log + /api/tool_log rows).
export type PolicyDecision = {
  ts_unix_ms?: EdictLogResponseDecisionsItem["ts_unix_ms"];
  actor?: EdictLogResponseDecisionsItem["actor"];
  correlation_id?: EdictLogResponseDecisionsItem["correlation_id"];
  tool?: EdictLogResponseDecisionsItem["tool"];
  capability?: EdictLogResponseDecisionsItem["capability"];
  allow?: EdictLogResponseDecisionsItem["allow"];
  reason?: EdictLogResponseDecisionsItem["reason"];
  hard_denied?: EdictLogResponseDecisionsItem["hard_denied"];};
export type ToolInvocation = {
  ts_unix_ms?: ToolLogResponseInvocationsItem["ts_unix_ms"];
  actor?: ToolLogResponseInvocationsItem["actor"];
  correlation_id?: ToolLogResponseInvocationsItem["correlation_id"];
  tool?: ToolLogResponseInvocationsItem["tool"];
  error?: ToolLogResponseInvocationsItem["error"];
  output?: ToolLogResponseInvocationsItem["output"];
  duration_ms?: ToolLogResponseInvocationsItem["duration_ms"];};
export type ApprovalDecision = {
  ts_unix_ms?: ApprovalsLogResponseApprovalsItem["ts_unix_ms"];
  approval_id?: ApprovalsLogResponseApprovalsItem["approval_id"];
  actor?: ApprovalsLogResponseApprovalsItem["actor"];
  correlation_id?: ApprovalsLogResponseApprovalsItem["correlation_id"];
  capability?: ApprovalsLogResponseApprovalsItem["capability"];
  tool?: ApprovalsLogResponseApprovalsItem["tool"];
  reason?: ApprovalsLogResponseApprovalsItem["reason"];
  status?: ApprovalsLogResponseApprovalsItem["status"];
  resolved_by?: ApprovalsLogResponseApprovalsItem["resolved_by"];};
export interface PolicyStats {
  // The daemon's own counters, spelled as accesses into the generated
  // catalog — so a renamed counter is a compile error in the browser, the way
  // it already is in the daemon.
  denial_rate?: EdictStatsResponse["denial_rate"];
  denied?: EdictStatsResponse["denied"];
  hard_denied?: EdictStatsResponse["hard_denied"];
  // allow_rate is the browser's own: the daemon counts denials, and the
  // browser derives the allowance rate from the same row.
  allow_rate?: number;
  allowed?: EdictStatsResponse["allowed"];
  total?: EdictStatsResponse["total"];
}
export type ToolCatalogRow = {
  name: ToolListResponseToolsItem["name"];
  description?: ToolListResponseToolsItem["description"];
  capability?: ToolListResponseToolsItem["capability"];};
// AgentPermissionRow is a capability row as the Diagnostics tab reads it.
// AgentPermissionRow is a capability row as the Diagnostics tab reads it.
export type AgentPermissionRow = {
  name: AgentCapabilitiesResponsePermissionsItem["name"];
  description?: AgentCapabilitiesResponsePermissionsItem["description"];
  capability?: AgentCapabilitiesResponsePermissionsItem["capability"];
  allowed?: AgentCapabilitiesResponsePermissionsItem["allowed"];
  ask?: AgentCapabilitiesResponsePermissionsItem["ask"];
  status?: AgentCapabilitiesResponsePermissionsItem["status"];
  source?: AgentCapabilitiesResponsePermissionsItem["source"];
  reason?: AgentCapabilitiesResponsePermissionsItem["reason"];
  level?: AgentCapabilitiesResponsePermissionsItem["level"];
};

export type AgentPermissionsSnapshot = {
  permissions?: AgentPermissionsResponse["permissions"];
  config_entries?: AgentPermissionsResponse["config_entries"];
  wake_access?: AgentPermissionsResponse["wake_access"];
  governance?: AgentPermissionsResponse["governance"];
  allowed_count?: AgentPermissionsResponse["allowed_count"];
  count?: AgentPermissionsResponse["count"];};

// AgentGovernanceSnapshot is the governance block as the Diagnostics tab reads it.
type AgentGovernanceSnapshot = {
  summary?: AgentCapabilitiesResponseGovernance["summary"];
  risk?: AgentCapabilitiesResponseGovernance["risk"];
  system_enforced?: AgentCapabilitiesResponseGovernance["system_enforced"];
  authority_boundary?: AgentCapabilitiesResponseGovernance["authority_boundary"];
  execution_boundary?: AgentCapabilitiesResponseGovernance["execution_boundary"];
  permission_passport?: AgentCapabilitiesResponseGovernance["permission_passport"];
  tool_policy?: AgentCapabilitiesResponseGovernance["tool_policy"];
  memory_policy?: AgentCapabilitiesResponseGovernance["memory_policy"];
  memory_writes?: AgentCapabilitiesResponseGovernance["memory_writes"];
  trust_ceiling?: AgentCapabilitiesResponseGovernance["trust_ceiling"];
  tool_count?: AgentCapabilitiesResponseGovernance["tool_count"];
  allowed_count?: AgentCapabilitiesResponseGovernance["allowed_count"];
  ask_count?: AgentCapabilitiesResponseGovernance["ask_count"];
  blocked_count?: AgentCapabilitiesResponseGovernance["blocked_count"];
  direct_tools?: AgentCapabilitiesResponseGovernance["direct_tools"];
  ask_tools?: AgentCapabilitiesResponseGovernance["ask_tools"];
  blocked_tools?: AgentCapabilitiesResponseGovernance["blocked_tools"];
  tool_allow_count?: AgentCapabilitiesResponseGovernance["tool_allow_count"];
  tool_deny_count?: AgentCapabilitiesResponseGovernance["tool_deny_count"];
  config_count?: AgentCapabilitiesResponseGovernance["config_count"];
  config_visible_count?: AgentCapabilitiesResponseGovernance["config_visible_count"];
  config_owned_count?: AgentCapabilitiesResponseGovernance["config_owned_count"];
  config_hidden_count?: AgentCapabilitiesResponseGovernance["config_hidden_count"];
  visible_configs?: AgentCapabilitiesResponseGovernance["visible_configs"];
  hidden_configs?: AgentCapabilitiesResponseGovernance["hidden_configs"];
  memory_scope?: AgentCapabilitiesResponseGovernance["memory_scope"];
  max_cost_mc?: AgentCapabilitiesResponseGovernance["max_cost_mc"];
  max_daily_mc?: AgentCapabilitiesResponseGovernance["max_daily_mc"];
  noise_silent_on_success?: AgentCapabilitiesResponseGovernance["noise_silent_on_success"];
  noise_disable_memory_writes?: AgentCapabilitiesResponseGovernance["noise_disable_memory_writes"];
  noise_min_notify_severity?: AgentCapabilitiesResponseGovernance["noise_min_notify_severity"];
  noise_min_notify_interval_sec?: AgentCapabilitiesResponseGovernance["noise_min_notify_interval_sec"];
};

// AgentWakeAccess is the wake-permission block as the Diagnostics tab reads it.
export type AgentWakeAccess = {
  status?: AgentCapabilitiesResponseWakeAccess["status"];
  reason?: AgentCapabilitiesResponseWakeAccess["reason"];
  direct_callable?: AgentCapabilitiesResponseWakeAccess["direct_callable"];
  direct_allowed?: AgentCapabilitiesResponseWakeAccess["direct_allowed"];
  schedule_allowed?: AgentCapabilitiesResponseWakeAccess["schedule_allowed"];
  channel_allowed?: AgentCapabilitiesResponseWakeAccess["channel_allowed"];
  operator_allowed?: AgentCapabilitiesResponseWakeAccess["operator_allowed"];
  delegation_allowed?: AgentCapabilitiesResponseWakeAccess["delegation_allowed"];
  delegation_scope?: AgentCapabilitiesResponseWakeAccess["delegation_scope"];
  delegation_sources?: AgentCapabilitiesResponseWakeAccess["delegation_sources"];
  manager?: AgentCapabilitiesResponseWakeAccess["manager"];
  owner_agent?: AgentCapabilitiesResponseWakeAccess["owner_agent"];
  parent_agent?: AgentCapabilitiesResponseWakeAccess["parent_agent"];
};

// AgentConfigPermissionRow is a config-visibility row as the Diagnostics tab reads it.
export type AgentConfigPermissionRow = {
  key: AgentCapabilitiesResponseConfigEntriesItem["key"];
  rating?: AgentCapabilitiesResponseConfigEntriesItem["rating"];
  visible?: AgentCapabilitiesResponseConfigEntriesItem["visible"];
  owned?: AgentCapabilitiesResponseConfigEntriesItem["owned"];
  source?: AgentCapabilitiesResponseConfigEntriesItem["source"];
  reason?: AgentCapabilitiesResponseConfigEntriesItem["reason"];
  allowed_agents?: AgentCapabilitiesResponseConfigEntriesItem["allowed_agents"];
  excluded_agents?: AgentCapabilitiesResponseConfigEntriesItem["excluded_agents"];
  description?: AgentCapabilitiesResponseConfigEntriesItem["description"];
};

export type AgentWakeResult = {
  accepted?: AgentRepairResponse["accepted"];
  agent?: AgentRepairResponse["agent"];
  correlation_id?: AgentRepairResponse["correlation_id"];};

// Board message (mirror of /api/board rows) for the Comms tab. The read view
// emits ts_unix_ms (not ts_ms) and omits from/to when empty.
export type BoardMessage = {
  id?: BoardReadResponseMessagesItem["id"];
  topic?: BoardReadResponseMessagesItem["topic"];
  from?: BoardReadResponseMessagesItem["from"];
  to?: BoardReadResponseMessagesItem["to"];
  reply_to?: BoardReadResponseMessagesItem["reply_to"];
  text?: BoardReadResponseMessagesItem["text"];
  ts_unix_ms?: BoardReadResponseMessagesItem["ts_unix_ms"];
  help?: BoardReadResponseMessagesItem["help"];
  acked_by?: BoardReadResponseMessagesItem["acked_by"];};
export const EMPTY_BOARD_MESSAGES: BoardMessage[] = [];
// Routing snapshot (mirror of /api/routing) for the Model tab.
export type RoutingInfo = {
  chains?: RoutingGetResponse["chains"];
};
// Six grouped tabs (declutter law): Wiring absorbs triggers+comms, Mind absorbs
// soul+memory+skills+files, Diagnostics absorbs diag+repair.
export type DetailTab =
  | "overview"
  | "activity"
  | "wiring"
  | "mind"
  | "model"
  | "diag";

const TABS: { id: DetailTab; label: string; icon: typeof Bot }[] = [
  { id: "overview", label: "Overview", icon: Bot },
  { id: "activity", label: "Activity", icon: ActivityIcon },
  { id: "wiring", label: "Wiring", icon: Anchor },
  { id: "mind", label: "Mind", icon: Brain },
  { id: "model", label: "Model", icon: Cpu },
  { id: "diag", label: "Diagnostics", icon: Gauge },
];

export const PRIMARY_TABS: DetailTab[] = ["overview", "activity", "wiring", "mind", "model", "diag"];

const TAB_BY_ID = new Map(TABS.map((tab) => [tab.id, tab]));

export function DetailOptionPicker<T extends string>({
  label,
  value,
  options,
  onChange,
  columns = "sm:grid-cols-2",
}: {
  label: string;
  value: T;
  options: { value: T; label: string; detail?: string; icon?: ReactNode }[];
  onChange: (value: T) => void;
  columns?: string;
}) {
  return (
    <div className="flex flex-col gap-1 text-[11px] text-muted">
      <span>{label}</span>
      <div className={cn("grid gap-1.5", columns)} role="group" aria-label={label}>
        {options.map((option) => {
          const selected = value === option.value;
          return (
            <button
              key={option.value}
              type="button"
              aria-pressed={selected}
              onClick={() => onChange(option.value)}
              className={cn(
                "flex min-h-9 items-start gap-2 rounded-lg border px-2.5 py-2 text-left text-xs transition",
                selected
                  ? "border-accent bg-accent/10 text-foreground"
                  : "border-border bg-card/45 text-muted hover:border-accent/40 hover:text-foreground",
              )}
            >
              {option.icon && <span className="mt-0.5 shrink-0 text-accent">{option.icon}</span>}
              <span className="min-w-0">
                <span className="block truncate font-semibold">{option.label}</span>
                {option.detail && <span className="mt-0.5 block truncate text-xs text-muted">{option.detail}</span>}
              </span>
            </button>
          );
        })}
      </div>
    </div>
  );
}


// ───────────────────────────── sub-views ─────────────────────────────

export function StatePill({
  state,
  label,
  running,
}: {
  state: FleetState;
  label?: string;
  running?: boolean;
}) {
  const cls =
    running || state === "running"
      ? "text-accent"
      : state === "armed"
        ? "text-good"
        : state === "paused" || state === "retired"
          ? "text-muted"
          : "text-foreground/70";
  return (
    <span
      className={cn(
        "inline-flex items-center gap-1 text-xs uppercase tracking-normal",
        cls,
      )}
    >
      {(running || state === "running") && (
        <span className="size-1.5 animate-pulse rounded-full bg-accent" />
      )}
      {label || state}
    </span>
  );
}

export function AgentDetailTabButton({
  id,
  active,
  counts,
  onSelect,
}: {
  id: DetailTab;
  active: boolean;
  counts: Parameters<typeof tabCount>[1];
  onSelect: (tab: DetailTab) => void;
}) {
  const t = TAB_BY_ID.get(id);
  if (!t) return null;
  const count = tabCount(t.id, counts);
  return (
    <button
      aria-pressed={active}
      aria-current={active ? "page" : undefined}
      onClick={() => onSelect(t.id)}
      className={cn(
        "flex items-center gap-1.5 rounded-lg px-3 py-2 text-sm font-medium transition-colors focus-glow",
        active ? "bg-card text-foreground shadow-e1" : "text-muted hover:bg-card/60 hover:text-foreground",
      )}
    >
      <t.icon className="size-4" />
      {t.label}
      {count !== undefined && count > 0 && (
        <span className={cn(
          "ml-0.5 rounded bg-panel px-1 font-mono text-xs text-muted",
        )}>
          {count}
        </span>
      )}
    </button>
  );
}

export function AgentNowPanel({
  phase,
  detail,
  correlationId,
  tool,
  model,
  since,
  last,
  onInspect,
}: {
  phase: string;
  detail: string;
  correlationId?: string;
  tool?: string;
  model?: string;
  since?: number;
  last?: number;
  onInspect?: () => void;
}) {
  return (
    <div
      title={detail}
      className="grid min-h-[68px] grid-cols-[auto_1fr_auto] items-center gap-2 rounded-lg border border-accent/40 bg-accent/10 p-2"
    >
      <div className="grid size-10 place-items-center rounded-lg bg-accent/15 text-accent">
        <ActivityIcon className="size-5" />
      </div>
      <div className="min-w-0">
        <div className="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1">
          <span className="text-xs font-semibold uppercase tracking-normal text-accent">Now</span>
          <span className="truncate text-sm font-semibold text-foreground">{phase}</span>
          {correlationId && (
            <span className="font-mono text-xs text-muted">{correlationId}</span>
          )}
        </div>
        <div className="mt-0.5 truncate text-xs text-muted">{detail}</div>
        {(tool || model) && (
          <div className="mt-1 flex min-w-0 flex-wrap items-center gap-1.5 text-xs text-muted">
            {tool && (
              <span className="rounded-md border border-border bg-card px-1.5 py-0.5">
                tool <span className="font-mono text-foreground/80">{tool}</span>
              </span>
            )}
            {model && (
              <span className="rounded-md border border-border bg-card px-1.5 py-0.5">
                model <span className="font-mono text-foreground/80">{model}</span>
              </span>
            )}
          </div>
        )}
        {(since || last) && (
          <div className="mt-0.5 truncate text-xs text-muted/85">
            {since ? `since ${fmtAgo(since)}` : ""}
            {since && last ? " · " : ""}
            {last ? `last event ${fmtAgo(last)}` : ""}
          </div>
        )}
      </div>
      {onInspect && (
        <Button
          size="sm"
          variant="ghost"
          onClick={onInspect}
          title="Inspect the active run in this agent"
          aria-label="Inspect active run"
        >
          <ScrollText className="size-3.5" /> Inspect
        </Button>
      )}
    </div>
  );
}

export function Row({ label, value }: { label: string; value: React.ReactNode }) {
  if (value == null || value === "") return null;
  return (
    <div className="flex gap-2 text-xs">
      <span className="w-28 shrink-0 text-muted">{label}</span>
      <span className="min-w-0 flex-1 break-words">{value}</span>
    </div>
  );
}

export function LifecycleConfigEditor({
  slug,
  profile,
  busy,
  onChanged,
}: {
  slug: string;
  profile: AgentProfile;
  busy?: boolean;
  onChanged: () => void;
}) {
  const ui = useUI();
  const currentMode =
    profile.lifecycle?.mode ||
    (profile.lifecycle?.retire_on_complete ? "retire_on_complete" : "persistent");
  const [mode, setMode] = useState<NonNullable<NonNullable<AgentProfile["lifecycle"]>["mode"]>>(currentMode);
  const [maxCycles, setMaxCycles] = useState(
    profile.lifecycle?.max_cycles ? String(profile.lifecycle.max_cycles) : "",
  );
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    setMode(currentMode);
    setMaxCycles(profile.lifecycle?.max_cycles ? String(profile.lifecycle.max_cycles) : "");
  }, [currentMode, profile.lifecycle?.max_cycles]);

  const parsedMax = Number.parseInt(maxCycles || "0", 10);
  const validMax = maxCycles.trim() === "" || (Number.isFinite(parsedMax) && parsedMax >= 0);
  const effectiveMode = mode === "persistent" && validMax && parsedMax > 0 ? "cycle" : mode;
  const dirty =
    effectiveMode !== currentMode ||
    (validMax ? Math.max(0, parsedMax || 0) : 0) !== (profile.lifecycle?.max_cycles || 0);

  async function saveLifecycle() {
    if (!validMax) {
      ui.toast("Max cycles must be a non-negative number", "error");
      return;
    }
    const max = Math.max(0, parsedMax || 0);
    setSaving(true);
    try {
      await postJSON("/api/agents/edit", {
        ref: slug,
        profile: editableAgentProfile(profile, {
          lifecycle: {
            mode: effectiveMode,
            retire_on_complete: effectiveMode === "retire_on_complete",
            max_cycles: effectiveMode === "retire_on_complete" ? 0 : max,
            completed_cycles: profile.lifecycle?.completed_cycles || 0,
          },
        }),
      });
      ui.toast(`${slug} lifecycle updated`, "success");
      onChanged();
    } catch (e) {
      ui.toast((e as Error).message, "error");
    } finally {
      setSaving(false);
    }
  }

  return (
    <div className="rounded-md border border-border bg-panel/40 p-2">
      <div className="mb-1 flex items-center gap-1.5 text-xs font-semibold uppercase tracking-normal text-muted">
        <Skull className="size-3" /> lifecycle contract
      </div>
      <div className="flex flex-wrap items-end gap-2">
        <div className="min-w-[18rem] flex-1">
          <DetailOptionPicker
            label="Agent lifecycle mode"
            value={mode}
            onChange={(next) => {
              setMode(next);
              if (next === "retire_on_complete") setMaxCycles("");
            }}
            columns="sm:grid-cols-3"
            options={[
              { value: "persistent", label: "Persistent", detail: "stays alive", icon: <ActivityIcon className="size-3.5" /> },
              { value: "cycle", label: "Cycle", detail: "repeat wakes", icon: <Repeat className="size-3.5" /> },
              { value: "retire_on_complete", label: "One-shot", detail: "retire on done", icon: <Archive className="size-3.5" /> },
            ]}
          />
        </div>
        <label className="flex flex-col gap-1 text-[11px] text-muted">
          Max cycles
          <input
            value={maxCycles}
            onChange={(e) => setMaxCycles(e.target.value)}
            aria-label="Agent max cycles"
            inputMode="numeric"
            disabled={mode === "retire_on_complete"}
            placeholder="0 = unlimited"
            className="w-28 rounded-md border border-border bg-card px-2 py-1 text-xs text-foreground outline-none placeholder:text-muted/60 focus-visible:border-accent disabled:opacity-50"
          />
        </label>
        <Button
          type="button"
          size="sm"
          disabled={busy || saving || !dirty || !validMax}
          onClick={saveLifecycle}
        >
          {saving ? <Wrench className="size-3.5 animate-spin" /> : <Skull className="size-3.5" />} Save lifecycle
        </Button>
        {!validMax && <span className="text-xs text-bad">invalid max cycles</span>}
        {dirty && validMax && <span className="text-xs text-warn">unsaved</span>}
      </div>
    </div>
  );
}

export function MiniPolicy({ label, allowed, note }: { label: string; allowed: boolean; note?: string }) {
  return (
    <div className="rounded-md border border-border bg-card/55 px-2 py-1.5">
      <div className="flex items-center gap-1.5">
        <Badge variant={allowed ? "good" : "bad"}>{allowed ? "allowed" : "blocked"}</Badge>
        <span className="text-xs font-semibold uppercase tracking-normal text-muted">{label}</span>
      </div>
      {note && <div className="mt-1 truncate text-xs text-muted" title={note}>{note}</div>}
    </div>
  );
}

export function RepairCommandCell({
  label,
  value,
  tone = "muted",
}: {
  label: string;
  value: string;
  tone?: "good" | "bad" | "warn" | "muted" | "accent";
}) {
  return (
    <div
      title={value}
      className={cn(
        "min-w-0 rounded-md border border-border bg-card/55 px-2 py-1.5",
        tone === "good" && "border-good/30 bg-good/5",
        tone === "bad" && "border-bad/30 bg-bad/5",
        tone === "warn" && "border-warn/40 bg-warn/10",
        tone === "accent" && "border-accent/40 bg-accent/5",
      )}
    >
      <div className="text-xs font-semibold uppercase tracking-normal text-muted">{label}</div>
      <div
        className={cn(
          "mt-0.5 truncate text-[11px] text-foreground/85",
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

export function ConfigOverrideBox({
  summary,
}: {
  summary: AgentConfigOverrideSummary;
}) {
  return (
    <div className="rounded-lg border border-border bg-panel/30 p-2.5">
      <div className="mb-1 text-xs uppercase tracking-normal text-muted">
        agent config overrides
      </div>
      {summary.runtime.length > 0 && (
        <div className="space-y-1">
          <div className="text-xs uppercase tracking-normal text-muted">
            runtime behavior
          </div>
          <ul className="space-y-1 text-[11px]">
            {summary.runtime.map((row) => (
              <li
                key={row.key}
                className="rounded-md border border-border bg-card/40 p-2"
              >
                <div className="flex flex-wrap items-center gap-2">
                  <span className="font-mono text-foreground/85">
                    {row.key}={row.value}
                  </span>
                  <Badge variant={row.valid ? "accent" : "bad"}>
                    {row.valid ? row.label : "invalid"}
                  </Badge>
                </div>
                <div
                  className={cn("mt-1 text-muted", !row.valid && "text-bad")}
                >
                  {row.valid ? row.effect : row.issue}
                </div>
              </li>
            ))}
          </ul>
        </div>
      )}
      {summary.generic.length > 0 && (
        <div className={cn("space-y-1", summary.runtime.length > 0 && "mt-2")}>
          <div className="text-xs uppercase tracking-normal text-muted">
            generic overlay
          </div>
          <ul className="space-y-1 font-mono text-[11px] text-foreground/85">
            {summary.generic.map(({ key, value }) => (
              <li key={key} className="break-all">
                {key}={value}
              </li>
            ))}
          </ul>
        </div>
      )}
    </div>
  );
}

// VisualStat - larger, more prominent stat display with icon emphasis
export function Stat({
  icon: Icon,
  label,
  value,
  detail,
  accent,
  tone,
}: {
  icon: typeof Bot;
  label: string;
  value: React.ReactNode;
  detail?: React.ReactNode;
  accent?: boolean;
  tone?: "good" | "warn" | "bad" | "accent" | "muted";
}) {
  const iconColor = tone === "good" ? "text-good" : tone === "warn" ? "text-warn" : tone === "bad" ? "text-bad" : tone === "accent" ? "text-accent" : "text-muted/60";
  const borderColor = tone === "good" ? "border-good/40 bg-good/10" : tone === "warn" ? "border-warn/40 bg-warn/15" : tone === "bad" ? "border-bad/40 bg-bad/10" : tone === "accent" ? "border-accent/40 bg-accent/15" : accent ? "border-accent/40" : "border-border/50 bg-panel/30";
  const valueColor = tone === "good" ? "text-good" : tone === "warn" ? "text-warn" : tone === "bad" ? "text-bad" : tone === "accent" ? "text-accent" : "text-foreground";

  return (
    <div
      className={cn(
        "rounded-xl border p-3 shadow-sm",
        borderColor,
      )}
    >
      <div className="flex items-center gap-2">
        <div className={cn(
          "flex h-9 w-9 items-center justify-center rounded-lg bg-panel/50",
          tone === "good" && "bg-good/10",
          tone === "warn" && "bg-warn/15",
          tone === "bad" && "bg-bad/10",
          tone === "accent" && "bg-accent/15",
        )}>
          <Icon className={cn("size-5", iconColor)} />
        </div>
        <span className="text-xs font-semibold uppercase tracking-normal text-muted">{label}</span>
      </div>
      <div className={cn("mt-2 text-xl font-bold tabular-nums tracking-normal", valueColor)}>
        {value}
      </div>
      {detail ? (
        <div className="mt-1 line-clamp-2 text-[11px] leading-relaxed text-muted/80">
          {detail}
        </div>
      ) : null}
    </div>
  );
}

// BudgetBar shows spend against a ceiling (microcents). No ceiling → just the
// amount. Over budget → the bar goes red.
export function BudgetBar({
  label,
  spentMc,
  capMc,
}: {
  label: string;
  spentMc: number;
  capMc?: number;
}) {
  const pct = capMc && capMc > 0 ? Math.min(100, (spentMc / capMc) * 100) : 0;
  const over = capMc != null && capMc > 0 && spentMc > capMc;
  return (
    <div className="rounded-xl border border-border/50 bg-panel/30 p-3 shadow-sm">
      <div className="mb-2 flex items-center justify-between">
        <span className="flex items-center gap-1.5 text-xs font-semibold uppercase tracking-normal text-muted">
          <Coins className="size-3.5" /> {label}
        </span>
        <span className={cn(
          "font-mono text-xs font-medium",
          over ? "text-bad" : "text-foreground/80"
        )}>
          {money(spentMc)}
          {capMc && capMc > 0 ? ` / ${money(capMc)}` : " uncapped"}
        </span>
      </div>
      {capMc != null && capMc > 0 && (
        <div className="h-2 overflow-hidden rounded-full bg-panel">
          <div
            className={cn(
              "h-full rounded-full transition-all duration-500",
              over ? "bg-gradient-to-r from-bad to-warn" : "bg-gradient-to-r from-accent to-good"
            )}
            style={{ width: `${Math.max(2, pct)}%` }}
          />
        </div>
      )}
    </div>
  );
}

export function editableAgentProfile(profile: AgentProfile, patch: Partial<AgentProfile> = {}) {
  return {
    name: profile.name || "",
    soul: profile.soul || "",
    instructions: profile.instructions || [],
    model: profile.model || "",
    fallbacks: profile.fallbacks || [],
    task_type: profile.task_type || "",
    max_cost_mc: profile.max_cost_mc || 0,
    max_daily_mc: profile.max_daily_mc || 0,
    memory_scope: profile.memory_scope || "",
    workdir: profile.workdir || "",
    owner_agent: profile.owner_agent || "",
    parent_agent: profile.parent_agent || "",
    direct_callable: profile.direct_callable,
    retry_policy: profile.retry_policy,
    health_policy: profile.health_policy,
    self_repair: profile.self_repair,
    noise_policy: profile.noise_policy,
    tool_allow: profile.tool_allow || [],
    tool_deny: profile.tool_deny || [],
    trust_ceiling: profile.trust_ceiling || "",
    execution_profile: profile.execution_profile || "",
    config_overrides: profile.config_overrides || {},
    lifecycle: profile.lifecycle || {},
    tasklist: profile.tasklist || [],
    description: profile.description || "",
    ...patch,
  };
}


// tabCount returns the badge number for a tab (undefined = no badge).
function tabCount(
  id: DetailTab,
  data: {
    memory: MemoryRecord[] | null;
    skills: SkillLite[] | null;
    orders: ApiOrder[];
    schedules: ApiSchedule[];
    comms: BoardMessage[] | null;
    denials: PolicyDecision[] | null;
    toolErrors: ToolInvocation[] | null;
  },
): number | undefined {
  switch (id) {
    case "wiring":
      return data.orders.length + data.schedules.length + (data.comms?.length || 0);
    case "diag":
      return (data.denials?.length || 0) + (data.toolErrors?.length || 0);
    default:
      return undefined;
  }
}
