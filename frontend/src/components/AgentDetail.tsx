import { useEffect, useMemo, useState, type ReactNode } from "react";
import {
  X,
  Activity as ActivityIcon,
  ScrollText,
  Anchor,
  Brain,
  Sparkles,
  ShieldCheck,
  FolderOpen,
  Bot,
  Coins,
  Clock,
  ArrowUpRight,
  Play,
  Pause,
  Flame,
  AlertTriangle,
  Share2,
  CalendarClock,
  Zap,
  ChevronRight,
  Mail,
  Cpu,
  Wrench,
  ArrowRight,
  Waypoints,
  Send,
  CheckCheck,
  CornerDownRight,
  LifeBuoy,
  Megaphone,
  Skull,
  Archive,
  ArchiveRestore,
  Trash2,
  ListTree,
  Plus,
  Repeat,
  IdCard,
  Heart,
  Gauge,
  HardDrive,
  FileCode,
  CheckCircle,
  AlertCircle,
  XCircle,
  MinusCircle,
} from "lucide-react";
import { getJSON, postAction, postJSON } from "@/lib/api";
import { cn, fmtTime, fmtDateTime, fmtAgo, fmtDue, clip } from "@/lib/utils";
import { money } from "@/lib/format";
import { Button } from "@/components/ui/button";
import { Badge, statusVariant } from "@/components/ui/badge";
import { SkeletonList } from "@/components/ui/skeleton";
import { EmptyState } from "@/components/ui/empty";
import { Disclosure } from "@/components/ui/disclosure";
import { ErrorText } from "@/components/JsonView";
import { useUI, type ConfirmOptions } from "@/components/ui/feedback";
import { useEvents } from "@/lib/events";
import { AgentAvatar } from "@/components/AgentAvatar";
import { AgentActivity } from "@/components/AgentActivity";
import { RunDetailLoader } from "@/components/RunDetail";
import { AgentRepair, repairReadinessPassport } from "@/components/AgentRepair";
import { ModelPicker } from "@/components/ModelPicker";
import { ModelChip } from "@/components/ModelChip";
import { TriggerChip } from "@/components/Fleet";
import { openAgent } from "@/lib/agentnav";
import { openIncident } from "@/lib/incidentnav";
import { agentCommandStrip, agentControlCenterLedger, agentEnableToast, agentHierarchySummary, agentIdentityCardSummary, agentLifecycleDispositionPassport, agentLifecycleSummary, agentLivePresencePassport, agentModelPassportSummary, agentModelRoutePassport, agentNoiseBudgetPassport, agentRemovalCascadePreset, agentRemoveToast, agentRetireToast, agentReviveToast, agentSchedulePressurePassport, agentSkillPassportSummary, agentTaskContractSummary, agentTaskProgressSummary, guardianQuietPolicyPayload, systemGuardianSafetySummary, type AgentCommandStripItem, type AgentControlCenterEntry, type AgentEnableResult, type AgentProfile, type AgentRemoveResult, type AgentRetireResult, type AgentReviveResult } from "@/views/Roster";
import {
  scheduleAgentSlug,
  highImpactToolNames,
  type FleetTrigger,
  type FleetState,
  type ApiOrder,
  type ApiSchedule,
} from "@/lib/fleet";
import {
  agentScope,
  agentCorrelations,
  filterByCorrelation,
  filterAgentMemory,
  filterAgentSkills,
  summarizeAgent,
  lastFailure,
  healthSnapshot,
  summarizeConfigOverrides,
  summarizeAutoRepair,
  summarizeAgentRuntimeStatus,
  summarizeEscalations,
  escalationOperationalTasks,
  escalationChainLabel,
  incidentLineageLabel,
  summarizeProviderRoutingRow,
  summarizeAgentPolicyDenials,
  lastAutonomyRunbookSourceLabel,
  wakeLineage,
  escalationCausalityLineage,
  mailboxWakeFor,
  type MailboxWakeRef,
  type EscalationCausalityLineage,
  type ReaperReport,
  type AgentHealthSnapshot,
  type AgentConfigOverrideSummary,
  type AgentEscalation,
  type AgentOperationalTask,
  type AgentRepairEvent,
  type AgentRepairStatus,
  type AgentRepairSnapshot,
  type AgentCardRuntimeSummary,
  type MemoryRecord,
  type SkillLite,
  type RunLite,
  type ProviderRoutingRow,
} from "@/lib/agentdetail";
import { isChainRef, chainName, type ChainsState } from "@/lib/chains";
import { type ModelCatalog } from "@/lib/models";

// Diagnostics row shapes (mirrors of /api/policy_log + /api/tool_log rows).
interface PolicyDecision {
  ts_unix_ms?: number;
  actor?: string;
  correlation_id?: string;
  tool?: string;
  capability?: string;
  allow?: boolean;
  reason?: string;
  hard_denied?: boolean;
}
interface ToolInvocation {
  ts_unix_ms?: number;
  actor?: string;
  correlation_id?: string;
  tool?: string;
  error?: boolean;
  output?: string;
  duration_ms?: number;
}
interface ApprovalDecision {
  ts_unix_ms?: number;
  approval_id?: string;
  actor?: string;
  correlation_id?: string;
  capability?: string;
  tool?: string;
  reason?: string;
  status?: "pending" | "granted" | "denied" | "timeout" | string;
  resolved_by?: string;
}
interface PolicyStats {
  denial_rate?: number;
  denied?: number;
  hard_denied?: number;
  allow_rate?: number;
  allowed?: number;
  total?: number;
}
interface ToolCatalogRow {
  name: string;
  description?: string;
  capability?: string;
}
interface AgentPermissionRow {
  name: string;
  description?: string;
  capability?: string;
  allowed?: boolean;
  ask?: boolean;
  status?: string;
  source?: string;
  reason?: string;
  level?: string;
}
interface AgentPermissionsSnapshot {
  permissions?: AgentPermissionRow[];
  config_entries?: AgentConfigPermissionRow[];
  wake_access?: AgentWakeAccess;
  governance?: AgentGovernanceSnapshot;
  allowed_count?: number;
  count?: number;
}
interface AgentGovernanceSnapshot {
  summary?: string;
  risk?: "open" | "restricted" | "governed" | "system_guardian" | string;
  system_enforced?: boolean;
  authority_boundary?: string;
  execution_boundary?: string;
  permission_passport?: string;
  tool_policy?: string;
  memory_policy?: string;
  memory_writes?: string;
  trust_ceiling?: string;
  tool_count?: number;
  allowed_count?: number;
  ask_count?: number;
  blocked_count?: number;
  direct_tools?: string[];
  ask_tools?: string[];
  blocked_tools?: string[];
  tool_allow_count?: number;
  tool_deny_count?: number;
  config_count?: number;
  config_visible_count?: number;
  config_owned_count?: number;
  config_hidden_count?: number;
  visible_configs?: string[];
  hidden_configs?: string[];
  memory_scope?: string;
  max_cost_mc?: number;
  max_daily_mc?: number;
  noise_silent_on_success?: boolean;
  noise_disable_memory_writes?: boolean;
  noise_min_notify_severity?: string;
  noise_min_notify_interval_sec?: number;
}
interface AgentWakeAccess {
  status?: string;
  reason?: string;
  direct_callable?: boolean;
  direct_allowed?: boolean;
  schedule_allowed?: boolean;
  channel_allowed?: boolean;
  operator_allowed?: boolean;
  delegation_allowed?: boolean;
  delegation_scope?: "any" | "manager" | string;
  delegation_sources?: string[];
  manager?: string;
  owner_agent?: string;
  parent_agent?: string;
}
interface AgentConfigPermissionRow {
  key: string;
  rating?: string;
  visible?: boolean;
  owned?: boolean;
  source?: string;
  reason?: string;
  allowed_agents?: string[];
  excluded_agents?: string[];
  description?: string;
}
interface AgentWakeResult {
  accepted?: boolean;
  agent?: string;
  correlation_id?: string;
}

// Board message (mirror of /api/board rows) for the Comms tab. The read view
// emits ts_unix_ms (not ts_ms) and omits from/to when empty.
interface BoardMessage {
  id?: string;
  topic?: string;
  from?: string;
  to?: string;
  reply_to?: string;
  text?: string;
  ts_unix_ms?: number;
  help?: boolean;
  acked_by?: string[];
}
const EMPTY_BOARD_MESSAGES: BoardMessage[] = [];
// Routing snapshot (mirror of /api/routing) for the Model tab.
interface RoutingInfo {
  chains?: Record<string, string[]>;
}
type DetailTab =
  | "overview"
  | "soul"
  | "triggers"
  | "model"
  | "activity"
  | "comms"
  | "memory"
  | "skills"
  | "diag"
  | "files"
  | "repair";

const TABS: { id: DetailTab; label: string; icon: typeof Bot }[] = [
  { id: "overview", label: "Overview", icon: Bot },
  { id: "activity", label: "Activity", icon: ActivityIcon },
  { id: "triggers", label: "Triggers", icon: Anchor },
  { id: "comms", label: "Comms", icon: Mail },
  { id: "soul", label: "Soul", icon: Heart },
  { id: "model", label: "Model", icon: Cpu },
  { id: "memory", label: "Memory", icon: Brain },
  { id: "skills", label: "Skills", icon: Sparkles },
  { id: "repair", label: "Repair", icon: Wrench },
  { id: "diag", label: "Diagnostics", icon: Gauge },
  { id: "files", label: "Files", icon: FileCode },
];

const PRIMARY_TABS: DetailTab[] = ["overview", "activity", "triggers", "comms", "soul", "model", "memory", "skills", "repair", "diag", "files"];

const TAB_BY_ID = new Map(TABS.map((tab) => [tab.id, tab]));

function DetailOptionPicker<T extends string>({
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
                  : "border-border bg-card/45 text-muted hover:border-accent/50 hover:text-foreground",
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

// AgentDetail (M953) — the per-agent Command Center: one screen that answers
// "what is this agent, how is it triggered, what has it done, what does it know,
// what is it allowed to do, and what went wrong". Reuses the M941 activity
// drill-in (components/AgentActivity), the M952 fleet trigger chips, and the
// shared avatar/format helpers; reads only existing endpoints. Rendered in the
// Agents → Fleet tab when a roster entity is opened.
export function AgentDetail({
  slug,
  profile,
  runs,
  orders,
  triggers,
  state,
  schedules = [],
  page = false,
  onClose,
  onManage,
  onLive,
  onChanged,
}: {
  slug: string;
  profile: AgentProfile;
  runs: RunLite[];
  orders: ApiOrder[];
  triggers: FleetTrigger[];
  state: FleetState;
  // Schedules that may run as this agent — used for the Triggers "upcoming
  // fires" forecast. Optional so the embedded Fleet panel works without it.
  schedules?: ApiSchedule[];
  // page mode (M960): rendered as the full-page AgentPage rather than the
  // embedded Fleet panel — hides the in-panel close X and the "open full page"
  // shortcut (you're already there).
  page?: boolean;
  onClose: () => void;
  onManage: (view: string) => void;
  onLive?: () => void;
  onChanged?: () => void;
}) {
  const ui = useUI();
  const { subscribe } = useEvents();
  const [tab, setTab] = useState<DetailTab>("overview");
  const [bump, setBump] = useState(0);
  const [busy, setBusy] = useState(false);
  const [taskBusy, setTaskBusy] = useState<string | null>(null);
  const [activityFocusRun, setActivityFocusRun] = useState<string | undefined>();

  // Aux data fetched best-effort on mount / agent change / live nudge. One
  // failing endpoint never blanks the rest (Promise.allSettled).
  const [memory, setMemory] = useState<MemoryRecord[] | null>(null);
  const [skills, setSkills] = useState<SkillLite[] | null>(null);
  const [policy, setPolicy] = useState<PolicyDecision[] | null>(null);
  const [tools, setTools] = useState<ToolInvocation[] | null>(null);
  const [approvals, setApprovals] = useState<ApprovalDecision[] | null>(null);
  const [posture, setPosture] = useState<PolicyStats | null>(null);
  const [askPolicy, setAskPolicy] = useState<string | null>(null);
  const [edictLevels, setEdictLevels] = useState<Record<string, string>>({});
  const [toolCatalog, setToolCatalog] = useState<ToolCatalogRow[] | null>(null);
  const [agentPermissions, setAgentPermissions] = useState<AgentPermissionsSnapshot | null>(null);
  const [board, setBoard] = useState<BoardMessage[] | null>(null);
  const [routing, setRouting] = useState<RoutingInfo | null>(null);
  const [provLog, setProvLog] = useState<ProviderRoutingRow[] | null>(null);
  const [reaper, setReaper] = useState<ReaperReport | null>(null);
  const [repairStatus, setRepairStatus] = useState<AgentRepairStatus | null>(
    null,
  );
  const [escalations, setEscalations] = useState<AgentEscalation[] | null>(
    null,
  );
  const [modelPickerOpen, setModelPickerOpen] = useState(false);

  useEffect(() => {
    let alive = true;
    Promise.allSettled([
      getJSON<{ records?: MemoryRecord[] }>("/api/memory"),
      getJSON<{ skills?: SkillLite[] }>("/api/skills"),
      getJSON<{ decisions?: PolicyDecision[] }>("/api/policy_log", {
        limit: "200",
      }),
      getJSON<{ invocations?: ToolInvocation[] }>("/api/tool_log", {
        limit: "200",
      }),
      getJSON<{ approvals?: ApprovalDecision[] }>("/api/approvals_log", {
        limit: "200",
      }),
      getJSON<PolicyStats>("/api/policy"),
      getJSON<{ ask_policy?: string; levels?: Record<string, string> }>("/api/edict_show"),
      getJSON<{ tools?: ToolCatalogRow[] }>("/api/tools_catalog"),
      getJSON<AgentPermissionsSnapshot>("/api/agents/permissions", { ref: slug }),
      getJSON<{ messages?: BoardMessage[] }>("/api/board"),
      getJSON<RoutingInfo>("/api/routing"),
      getJSON<{ events?: ProviderRoutingRow[] }>("/api/provider_log", {
        limit: "200",
      }),
      getJSON<ReaperReport>("/api/reaper/scan"),
      getJSON<AgentRepairStatus>("/api/agents/repair_status", {
        ref: slug,
        limit: "12",
      }),
      getJSON<{ escalations?: AgentEscalation[] }>("/api/agents/escalations", {
        ref: slug,
        limit: "12",
      }),
    ]).then((res) => {
      if (!alive) return;
      const [m, sk, pl, tl, al, po, ed, tc, ap, bd, rt, pv, rp, rs, es] = res;
      setMemory(m.status === "fulfilled" ? m.value.records || [] : []);
      setSkills(sk.status === "fulfilled" ? sk.value.skills || [] : []);
      setPolicy(pl.status === "fulfilled" ? pl.value.decisions || [] : []);
      setTools(tl.status === "fulfilled" ? tl.value.invocations || [] : []);
      setApprovals(al.status === "fulfilled" ? al.value.approvals || [] : []);
      setPosture(po.status === "fulfilled" ? po.value : null);
      setAskPolicy(
        ed.status === "fulfilled" ? (ed.value.ask_policy ?? null) : null,
      );
      setEdictLevels(ed.status === "fulfilled" ? ed.value.levels || {} : {});
      setToolCatalog(tc.status === "fulfilled" ? tc.value.tools || [] : []);
      setAgentPermissions(ap.status === "fulfilled" ? ap.value : null);
      setBoard(bd.status === "fulfilled" ? bd.value.messages || [] : []);
      setRouting(rt.status === "fulfilled" ? rt.value : null);
      setProvLog(pv.status === "fulfilled" ? pv.value.events || [] : []);
      setReaper(rp.status === "fulfilled" ? rp.value : null);
      setRepairStatus(rs.status === "fulfilled" ? rs.value : null);
      setEscalations(
        es.status === "fulfilled" ? es.value.escalations || [] : [],
      );
    });
    return () => {
      alive = false;
    };
  }, [slug, bump]);

  // Live: refetch aux on any event attributable to this agent (debounced).
  useEffect(() => {
    let t: ReturnType<typeof setTimeout> | undefined;
    const off = subscribe((e) => {
      const autoRepairForAgent =
        e.subject === "doctor.auto_repair" &&
        (e.payload?.agent === slug || e.payload?.target_agent === slug);
      if (e.actor !== slug && !autoRepairForAgent) return;
      if (t) clearTimeout(t);
      t = setTimeout(() => setBump((b) => b + 1), 1500);
    });
    return () => {
      if (t) clearTimeout(t);
      off();
    };
  }, [slug, subscribe]);

  const corrs = useMemo(() => agentCorrelations(runs, slug), [runs, slug]);
  const myMemory = useMemo(
    () =>
      memory ? filterAgentMemory(memory, slug, profile.memory_scope) : null,
    [memory, slug, profile.memory_scope],
  );
  const mySkills = useMemo(
    () => (skills ? filterAgentSkills(skills, slug) : null),
    [skills, slug],
  );
  const myDenials = useMemo(
    () =>
      policy
        ? filterByCorrelation(policy, corrs, slug).filter(
            (d) => d.allow === false,
          )
        : null,
    [policy, corrs, slug],
  );
  const myToolErrors = useMemo(
    () =>
      tools
        ? filterByCorrelation(tools, corrs, slug).filter((t) => t.error)
        : null,
    [tools, corrs, slug],
  );
  const myApprovals = useMemo(
    () => (approvals ? filterByCorrelation(approvals, corrs, slug) : null),
    [approvals, corrs, slug],
  );
  const myOrders = useMemo(
    () => orders.filter((o) => o.agent === slug),
    [orders, slug],
  );
  const mySchedules = useMemo(
    () =>
      schedules.filter(
        (s) =>
          ((s.agent || "") === slug || scheduleAgentSlug(s.intent) === slug),
      ),
    [schedules, slug],
  );
  const summary = useMemo(() => summarizeAgent(runs, slug), [runs, slug]);
  const fail = useMemo(() => lastFailure(runs, slug), [runs, slug]);
  const health = useMemo(
    () => healthSnapshot(slug, profile.retired, reaper),
    [slug, profile.retired, reaper],
  );
  const overrides = useMemo(
    () => summarizeConfigOverrides(profile.config_overrides),
    [profile.config_overrides],
  );
  const repair = useMemo(
    () => summarizeAutoRepair(repairStatus),
    [repairStatus],
  );
  const repairEvidenceCount = (fail ? 1 : 0) + (myDenials?.length || 0) + (myToolErrors?.length || 0) + overrides.runtime.filter((row) => !row.valid).length;
  const repairReadiness = repairReadinessPassport(profile, repairEvidenceCount);
  const runtimeStatus = useMemo(
    () => summarizeAgentRuntimeStatus(profile.status),
    [profile.status],
  );
  const healthContract = useMemo(
    () => agentHealthContractLedger(profile, health, repairStatus, runtimeStatus),
    [profile, health, repairStatus, runtimeStatus],
  );
  const escalation = useMemo(
    () => summarizeEscalations(escalations),
    [escalations],
  );
  const escalationTasks = useMemo(
    () => escalationOperationalTasks(escalations),
    [escalations],
  );
  const activeEscalationTasks = useMemo(
    () => escalationTasks.filter((task) => task.status !== "done"),
    [escalationTasks],
  );
  const wakeIssue = operatorWakeIssue(profile);
  // Comms: board messages this agent sent, received, or broadcast.
  const myComms = useMemo(
    () => (board ? agentBoardMessages(board, slug) : null),
    [board, slug],
  );
  // Model: routing/fallback events are daemon-wide, so surface the ones relevant
  // to THIS agent — its primary model or its task type (best-effort, since the
  // events carry no per-agent correlation).
  const myProvLog = useMemo(() => {
    if (!provLog) return null;
    const mdl = profile.model;
    const tt = profile.task_type;
    const rel = provLog.filter(
      (r) =>
        (mdl &&
          (r.primary === mdl ||
            r.failed === mdl ||
            r.next === mdl ||
            (r.chain || "").split(",").includes(mdl))) ||
        (tt && r.task_type === tt),
    );
    // If nothing is attributable but routing is happening, still show the latest
    // few fallbacks so "is my model failing over" is answerable.
    return rel.length > 0 ? rel : provLog.filter((r) => r.kind === "fallback");
  }, [provLog, profile.model, profile.task_type]);
  const repairTaskChain = useMemo(
    () => (profile.task_type ? routing?.chains?.[profile.task_type] : undefined),
    [routing, profile.task_type],
  );

  async function mutateTask(id: string, op: "update" | "remove", status?: string) {
    if (!id) return;
    const key = `${op}:${id}:${status || ""}`;
    setTaskBusy(key);
    try {
      await postJSON("/api/agents/task", {
        ref: slug,
        op,
        id,
        ...(status ? { status } : {}),
      });
      ui.toast(op === "remove" ? "Task removed" : `Task marked ${status}`, "success");
      onChanged?.();
      setBump((b) => b + 1);
    } catch (e) {
      ui.toast((e as Error).message, "error");
    } finally {
      setTaskBusy(null);
    }
  }

  async function addTask(title: string, scope: "cycle" | "total") {
    const clean = title.trim();
    if (!clean) return;
    const key = `add:${scope}`;
    setTaskBusy(key);
    try {
      await postJSON("/api/agents/task", {
        ref: slug,
        op: "add",
        title: clean,
        scope,
        status: "todo",
      });
      ui.toast("Task added", "success");
      onChanged?.();
      setBump((b) => b + 1);
    } catch (e) {
      ui.toast((e as Error).message, "error");
    } finally {
      setTaskBusy(null);
    }
  }

  async function action(
    path: string,
    params: Record<string, string>,
    success: string,
    opts?: { confirm?: ConfirmOptions },
  ) {
    if (opts?.confirm && !(await ui.confirm(opts.confirm))) return;
    setBusy(true);
    try {
      await postAction(path, params);
      ui.toast(success, "success");
      setBump((b) => b + 1);
      onChanged?.();
    } catch (e) {
      ui.toast((e as Error).message, "error");
    } finally {
      setBusy(false);
    }
  }

  async function createMailboxWake(kind: "dm" | "help" | "broadcast", subject: string) {
    setBusy(true);
    const name =
      kind === "dm"
        ? `${slug} mailbox`
        : kind === "help"
          ? `${slug} help queue`
          : `${slug} broadcast inbox`;
    const plan =
      kind === "help"
        ? "Read the triggering help request by id from the trigger payload, resolve or route it, reply with the outcome, then stop."
        : kind === "broadcast"
          ? "Read the triggering broadcast board message by id from the trigger payload, decide whether action is needed, reply or ack as appropriate, then stop."
          : "Read the triggering board message by id from the trigger payload, handle the request, reply if a reply is needed, then ack the message.";
    try {
      await postJSON("/api/standing/add", {
        order: {
          name,
          agent: slug,
          triggers: [{ type: "event", subject }],
          plan,
          initiative: { mode: "reactive" },
        },
      });
      ui.toast(`${name} armed`, "success");
      onChanged?.();
      setBump((b) => b + 1);
    } catch (e) {
      ui.toast((e as Error).message, "error");
    } finally {
      setBusy(false);
    }
  }

  async function wakeAgentNow() {
    setBusy(true);
    try {
      const res = await postAction<AgentWakeResult>("/api/agents/wake", {
        ref: slug,
        reason: "manual operator wake",
      });
      ui.toast(`${slug} wake queued`, "success");
      if (res.correlation_id) {
        setActivityFocusRun(res.correlation_id);
        setTab("activity");
      }
      setBump((b) => b + 1);
      onChanged?.();
    } catch (e) {
      ui.toast((e as Error).message, "error");
    } finally {
      setBusy(false);
    }
  }

  async function saveModel(modelId: string) {
    setBusy(true);
    try {
      await postJSON("/api/agents/edit", {
        ref: slug,
        profile: editableAgentProfile(profile, {
          model: modelId,
        }),
      });
      ui.toast(`${slug} model updated`, "success");
      setModelPickerOpen(false);
      onChanged?.();
      setBump((b) => b + 1);
    } catch (e) {
      ui.toast((e as Error).message, "error");
    } finally {
      setBusy(false);
    }
  }

  async function pauseFrequentSchedules(scheduleIds: string[]) {
    const ids = Array.from(new Set(scheduleIds.filter(Boolean)));
    if (ids.length === 0) return;
    if (!(await ui.confirm({
      title: `Pause frequent schedules for ${slug}?`,
      message: `${ids.length} schedule${ids.length === 1 ? "" : "s"} will stop waking this agent automatically. Manual wake, mailbox wake, and delegation remain available.`,
      confirmLabel: "Pause schedules",
    }))) return;
    setBusy(true);
    try {
      await Promise.all(ids.map((id) => postAction("/api/schedule/enable", { id, enabled: "false" })));
      ui.toast(`${ids.length} frequent schedule${ids.length === 1 ? "" : "s"} paused for ${slug}`, "success");
      setBump((b) => b + 1);
      onChanged?.();
    } catch (e) {
      ui.toast((e as Error).message, "error");
    } finally {
      setBusy(false);
    }
  }

  async function quietSystemGuardian() {
    if (!profile.system && profile.kind !== "system") return;
    setBusy(true);
    try {
      await postJSON("/api/agents/capabilities", guardianQuietPolicyPayload({ ...profile, system: true }));
      ui.toast(`${slug} quiet guardian policy applied`, "success");
      setBump((b) => b + 1);
      onChanged?.();
    } catch (e) {
      ui.toast((e as Error).message, "error");
    } finally {
      setBusy(false);
    }
  }

  async function setAgentEnabled(enabled: boolean) {
    setBusy(true);
    try {
      const res = await postAction<AgentEnableResult>("/api/agents/enable", {
        ref: slug,
        enabled: enabled ? "true" : "false",
      });
      ui.toast(agentEnableToast(slug, enabled, res), "success");
      setBump((b) => b + 1);
      onChanged?.();
    } catch (e) {
      ui.toast((e as Error).message, "error");
    } finally {
      setBusy(false);
    }
  }

  async function setAgentRetired(retired: boolean) {
    setBusy(true);
    try {
      const path = retired ? "/api/agents/retire" : "/api/agents/revive";
      const res = await postAction<AgentRetireResult | AgentReviveResult>(path, {
        ref: slug,
        ...(retired ? { reason: "operator retired from agent identity header" } : {}),
      });
      ui.toast(retired ? agentRetireToast(slug, res) : agentReviveToast(slug, res), "success");
      setBump((b) => b + 1);
      onChanged?.();
    } catch (e) {
      ui.toast((e as Error).message, "error");
    } finally {
      setBusy(false);
    }
  }

  const running = runtimeStatus.activeRunCount > 0 || state === "running";
  const avatarStatus = profile.retired
    ? "retired"
    : running
      ? "running"
      : runtimeStatus.operationalState === "paused" || !profile.enabled
        ? "paused"
        : "sleeping";
  const stateLabel = running
    ? runtimeStatus.operationalText || "running"
    : runtimeStatus.operationalText || state;
  const taskContract = agentTaskContractSummary(profile);
  const lifecycleDisposition = agentLifecycleDispositionPassport(profile);
  const modelRoute = agentModelRoutePassport(profile);
  const modelPassport = agentModelPassportSummary(profile);
  const skillPassport = agentDetailSkillPassport(mySkills);
  const noiseBudgetPassport = agentNoiseBudgetPassport(profile);
  const schedulePassport = agentSchedulePressurePassport(profile, mySchedules);
  const controlCenterLedger = agentControlCenterLedger(profile, overrides.runtime.filter((row) => !row.valid).length);
  const controlIntervention = agentControlInterventionSummary(controlCenterLedger);
  const headerPermissions = useMemo(
    () =>
      toolCatalog && toolCatalog.length > 0
        ? effectiveToolPermissions(toolCatalog, profile, edictLevels)
        : agentPermissions?.permissions
          ? agentPermissions.permissions.map(permissionRowFromSnapshot)
          : [],
    [agentPermissions, edictLevels, profile, toolCatalog],
  );
  const headerPermissionPassport = useMemo(
    () => summarizePermissionPassport(profile, headerPermissions, !!toolCatalog || !!agentPermissions),
    [agentPermissions, headerPermissions, profile, toolCatalog],
  );
  const configAccess = agentConfigAccessSummary(agentPermissions);
  const governancePassport = agentGovernancePassport(agentPermissions, headerPermissionPassport.detail);
  const wakeAccess = agentPermissions?.wake_access;
  const wakePolicy = summarizeWakeAccess(profile, wakeAccess);
  const delegationPassport = agentDelegationPassportDetail(profile, wakeAccess);
  const wakeOwner =
    wakeAccess?.manager ||
    (agentManagedSubagent(profile)
      ? profile.parent_agent || profile.owner_agent || ""
      : "");
  const waitingMailboxCount = myComms ? waitingForAgent(myComms, slug).length : 0;
  const mailboxPassport = agentMailboxPassport(slug, myComms || [], myOrders);
  const policyDenials = summarizeAgentPolicyDenials(profile.status);
  const livePresence = agentLivePresencePassport(profile, runtimeStatus, waitingMailboxCount);
  const identityCard = agentIdentityCardSummary(
    profile,
    runtimeStatus,
    wakeIssue,
    waitingMailboxCount,
    Date.now(),
    governancePassport.detail || headerPermissionPassport.detail,
  );
  const commandStrip = agentCommandStrip(
    profile,
    runtimeStatus,
    mailboxPassport,
    schedulePassport,
    governancePassport.detail || headerPermissionPassport.detail,
    {
      detail: agentResourcePassportDetail(profile, slug),
      tone: profile.workdir || profile.memory_scope || (profile.tool_allow || []).length > 0 || Object.keys(profile.config_overrides || {}).length > 0 ? "good" : "muted",
    },
    wakeIssue,
    Date.now(),
  );

  return (
    <section className="flex min-h-0 flex-col gap-3 rounded-lg border border-border bg-card p-3">
      {/* Header */}
      <div className="grid gap-3 lg:grid-cols-[minmax(0,1fr)_auto]">
        <div
          className={cn(
            "min-w-0 rounded-xl border border-border bg-panel/35 p-3",
            identityCard.tone === "accent" && "border-accent/40 bg-accent/10",
            identityCard.tone === "good" && "border-good/30 bg-good/5",
            identityCard.tone === "warn" && "border-warn/40 bg-warn/10",
            identityCard.tone === "bad" && "border-bad/35 bg-bad/5",
          )}
        >
          <div className="flex min-w-0 items-start gap-3">
            <AgentAvatar
              slug={slug}
              name={profile.name}
              size={48}
              status={avatarStatus}
            />
            <div className="min-w-0 flex-1">
              <div className="flex flex-wrap items-center gap-2">
                <span className="font-mono text-base font-semibold text-foreground">
                  {slug}
                </span>
                {profile.name && profile.name !== slug && (
                  <span className="text-sm text-muted">{profile.name}</span>
                )}
                <StatePill state={state} label={stateLabel} running={running} />
              </div>
              {profile.description && (
                <p className="mt-1 max-w-3xl text-sm leading-6 text-foreground/80">
                  {profile.description}
                </p>
              )}
              <div className="mt-2 grid gap-2 sm:grid-cols-2 lg:grid-cols-4">
                <AgentDetailHeroFact
                  icon={ActivityIcon}
                  label="Presence"
                  value={livePresence.value}
                  detail={livePresence.detail}
                  tone={livePresence.tone}
                />
                <AgentDetailHeroFact
                  icon={CalendarClock}
                  label="Next wake"
                  value={runtimeStatus.nextWakeMs ? fmtDue(runtimeStatus.nextWakeMs) : runtimeStatus.wakeText || "manual"}
                  detail={runtimeStatus.wakeDetail || schedulePassport.detail}
                  tone={schedulePassport.tone}
                />
                <AgentDetailHeroFact
                  icon={IdCard}
                  label="Agent identity card"
                  value={identityCard.label}
                  detail={identityCard.detail}
                  tone={identityCard.tone}
                />
                <div className="col-span-2 flex items-center gap-2 rounded-lg border border-border/60 bg-panel/25 p-2 sm:col-span-1">
                  <Cpu className="size-4 shrink-0 text-muted" />
                  <div className="min-w-0 flex-1">
                    <div className="text-xs font-medium uppercase tracking-normal text-muted">Model &amp; Fallback</div>
                    <div className={cn(
                      "truncate text-xs font-medium",
                      modelRoute.tone === "good" && "text-good",
                      modelRoute.tone === "warn" && "text-warn",
                      modelRoute.tone === "muted" && "text-muted",
                    )}>
                      {modelRoute.value}
                    </div>
                    {modelRoute.detail && (
                      <div className="truncate text-xs text-muted">{modelRoute.detail}</div>
                    )}
                  </div>
                  <Button
                    variant="ghost"
                    size="sm"
                    className="shrink-0"
                    onClick={() => setModelPickerOpen(true)}
                    title="Edit model and fallback chain"
                  >
                    <Wrench className="size-3.5" />
                  </Button>
                </div>
              </div>
              <div className="sr-only">{livePresence.value} · {livePresence.detail}</div>
              {wakeIssue && (
                <p className="mt-2 flex flex-wrap items-center gap-1 text-[11px] text-muted">
                  <span>{wakeIssue}</span>
                  {wakeOwner && (
                    <button
                      type="button"
                      className="font-mono text-accent hover:underline"
                      aria-label={`Open wake owner ${wakeOwner}`}
                      onClick={() => openAgent(wakeOwner)}
                    >
                      Open {wakeOwner}
                    </button>
                  )}
                </p>
              )}
            </div>
          </div>
        </div>

        {modelPickerOpen && (
          <ModelPicker
            value={profile.model || ""}
            onChange={saveModel}
          />
        )}

        <div className="flex flex-wrap items-start justify-end gap-1 rounded-xl border border-border bg-panel/25 p-2 lg:max-w-[20rem]">
          <Button
            variant="ghost"
            size="sm"
            disabled={busy || !!wakeIssue}
            title={wakeIssue || "Wake this agent now"}
            aria-label={`Wake ${slug}`}
            onClick={wakeAgentNow}
          >
            <Zap className="size-3.5" /> Wake
          </Button>
          {running && onLive && (
            <Button
              variant="ghost"
              size="sm"
              onClick={onLive}
              title="View live delegation tree"
            >
              <ActivityIcon className="size-3.5" /> Live
            </Button>
          )}
          <Disclosure
            className="min-w-full rounded-lg border border-border/60 bg-card/40 px-1 py-0.5"
            summary={<span className="text-[11px] font-medium text-muted">More actions</span>}
          >
            <div className="flex flex-wrap gap-1 p-1.5">
              <Button
                variant="ghost"
                size="sm"
                disabled={busy || schedulePassport.frequentIds.length === 0}
                title={schedulePassport.frequentIds.length > 0 ? `Pause ${schedulePassport.frequentIds.length} frequent schedule${schedulePassport.frequentIds.length === 1 ? "" : "s"}` : "No frequent schedules"}
                aria-label={`Pause frequent schedules for ${slug}`}
                onClick={() => pauseFrequentSchedules(schedulePassport.frequentIds)}
              >
                <CalendarClock className="size-3.5" /> Pause wakes
              </Button>
              {!profile.retired && (
                <Button
                  variant="ghost"
                  size="sm"
                  disabled={busy}
                  title={profile.enabled ? "Pause agent" : "Resume agent"}
                  onClick={() => setAgentEnabled(!profile.enabled)}
                >
                  {profile.enabled ? (
                    <Pause className="size-3.5" />
                  ) : (
                    <Play className="size-3.5" />
                  )}
                  {profile.enabled ? "Pause" : "Resume"}
                </Button>
              )}
              {profile.retired ? (
                <Button
                  variant="ghost"
                  size="sm"
                  disabled={busy}
                  title="Revive from the graveyard"
                  aria-label={`Revive ${slug}`}
                  onClick={() => setAgentRetired(false)}
                >
                  <ArchiveRestore className="size-3.5" /> Revive
                </Button>
              ) : (
                <Button
                  variant="ghost"
                  size="sm"
                  disabled={busy}
                  title="Retire to the graveyard"
                  aria-label={`Retire ${slug}`}
                  onClick={() => setAgentRetired(true)}
                >
                  <Archive className="size-3.5" /> Retire
                </Button>
              )}
              <Button
                variant="ghost"
                size="sm"
                title="Open lifecycle intervention and removal impact"
                aria-label={`Lifecycle ${slug}`}
                onClick={() => setTab("overview")}
              >
                <Skull className="size-3.5" /> Lifecycle
              </Button>
            </div>
          </Disclosure>
          {!page && (
            <Button
              variant="ghost"
              size="sm"
              onClick={() => openAgent(slug)}
              title="Open this agent's full identity page"
            >
              <ArrowUpRight className="size-3.5" /> Page
            </Button>
          )}
          {!page && (
            <button
              onClick={onClose}
              className="rounded-md border border-border p-1.5 text-muted hover:border-accent hover:text-foreground"
              title="Close"
            >
              <X className="size-3.5" />
            </button>
          )}
        </div>
      </div>

      {running && (
        <AgentNowPanel
          phase={runtimeStatus.activePhase || runtimeStatus.operationalText || "running"}
          detail={runtimeStatus.liveDetail || runtimeStatus.activeContextDetail || "active run"}
          correlationId={runtimeStatus.activeCorrelationId}
          tool={runtimeStatus.activeTool}
          model={runtimeStatus.activeModel}
          since={runtimeStatus.activeStartedMs}
          last={runtimeStatus.activeLastEventMs}
          onInspect={
            runtimeStatus.activeCorrelationId
              ? () => {
                  setActivityFocusRun(runtimeStatus.activeCorrelationId);
                  setTab("activity");
                }
              : undefined
          }
        />
      )}

      {/* Quick-detail strip — the first thing you see when you need more than the hero. */}
      <Disclosure
        className="rounded-lg border border-border/40 bg-panel/20 px-1.5 py-0.5"
        summary={<span className="text-xs font-medium text-muted/70 uppercase tracking-normal">All details</span>}
      >
        <div className="flex flex-col gap-2 p-1.5">
          {/* One-line metric chips */}
          <div className="flex flex-wrap gap-1.5">
            <CompactChip label="Type" value={profile.system ? "system" : profile.kind || "custom"} />
            <CompactChip label="Lifecycle" value={lifecycleDisposition.value} tone={lifecycleDisposition.tone} />
            <CompactChip label="Presence" value={livePresence.value} tone={livePresence.tone} />
            <CompactChip label="Model" value={modelPassport} tone={profile.model ? "good" : "muted"} />
            <CompactChip label="Skills" value={skillPassport.value} tone={skillPassport.tone} />
            <CompactChip label="Schedule" value={schedulePassport.detail} tone={schedulePassport.tone} />
            <CompactChip label="Mailbox" value={mailboxPassport.value} tone={mailboxPassport.tone} />
            <CompactChip label="Call policy" value={wakePolicy.passport} tone={wakePolicy.tone} />
            <CompactChip label="Delegation" value={delegationPassport.value} tone={delegationPassport.tone} />
            <CompactChip label="Task contract" value={taskContract} />
            {policyDenials.count > 0 && (
              <CompactChip label="Denials" value={policyDenials.text} tone={policyDenials.tone === "bad" ? "bad" : "warn"} />
            )}
            <CompactChip label="Governance" value={governancePassport.detail} tone={governancePassport.tone} />
            <CompactChip label="Control" value={controlIntervention.label} tone={controlIntervention.tone} />
            <CompactChip label="Config access" value={configAccess} />
            <CompactChip label="Noise" value={noiseBudgetPassport.detail} tone={noiseBudgetPassport.tone} />
            <CompactChip label="Resilience" value={repairReadiness.value} tone={repairReadiness.tone} />
            <CompactChip label="Capability" value={headerPermissionPassport.detail} tone={headerPermissionPassport.level === "open" ? "warn" : headerPermissionPassport.level === "tight" ? "good" : "muted"} />
            <CompactChip label="Health" value={health.label} tone={health.state === "healthy" ? "good" : health.state === "retired" ? "muted" : "bad"} />
          </div>
        </div>
      </Disclosure>

      {/* Tabs — all sections in one clean row */}
      <div
        className="rounded-xl border border-border bg-panel/20 px-3 py-2.5"
        role="tablist"
        aria-label={`${slug} detail sections`}
      >
        <div className="flex flex-wrap gap-1.5">
          {PRIMARY_TABS.map((id) => (
            <AgentDetailTabButton
              key={id}
              id={id}
              active={tab === id}
              counts={{
                memory: myMemory,
                skills: mySkills,
                orders: myOrders,
                schedules: mySchedules,
                comms: myComms,
                denials: myDenials,
                toolErrors: myToolErrors,
              }}
              onSelect={setTab}
            />
          ))}
        </div>
      </div>

      <div className="min-h-0 overflow-auto">
        {tab === "overview" && (
          <Overview
            slug={slug}
            profile={profile}
            triggers={triggers}
            orders={myOrders}
            summary={summary}
            runtimeStatus={runtimeStatus}
            runs={runs}
            fail={fail}
            health={health}
            healthContract={healthContract}
            repair={repair}
            repairStatus={repairStatus}
            repairReadiness={repairReadiness}
            escalations={escalations}
            escalation={escalation}
            escalationTasks={activeEscalationTasks}
            memory={myMemory}
            skills={mySkills}
            schedules={mySchedules}
            mailboxMessages={myComms || EMPTY_BOARD_MESSAGES}
            toolCatalog={toolCatalog}
            edictLevels={edictLevels}
            agentPermissions={agentPermissions}
            livePresence={livePresence}
            commandStrip={commandStrip}
            busy={busy}
            onLifecycleChanged={() => {
              setBump((b) => b + 1);
              onChanged?.();
            }}
            onManage={onManage}
            onView={setTab}
            onFocusRun={setActivityFocusRun}
            onQuietGuardian={quietSystemGuardian}
          />
        )}

        {tab === "soul" && (
          <div className="space-y-2">
            <Row label="model route" value={modelPassport} />
            <Row label="skills" value={skillPassport.value} />
            <Row label="task type" value={profile.task_type || "—"} />
            <Row
              label="lifecycle"
              value={
                <span>
                  {agentLifecycleSummary(profile)} · {agentLifecycleDetail(profile)}
                </span>
              }
            />
            <LifecycleConfigEditor
              slug={slug}
              profile={profile}
              busy={busy}
              onChanged={() => {
                setBump((b) => b + 1);
                onChanged?.();
              }}
            />
            <Row label="contract" value={taskContract} />
            <Row label="call policy" value={agentHierarchySummary(profile)} />
            {/* Identity essentials lead; the operational/policy/runtime knobs fold
                so the Soul tab reads as "who is this agent", not a config ledger. */}
            <Disclosure
              summary={<span className="text-xs uppercase tracking-normal text-muted">Operational config, policy &amp; runtime</span>}
            >
              <div className="space-y-2 pt-1">
                <Row label="noise budget" value={noiseBudgetPassport.detail} />
                <Row label="schedule pressure" value={schedulePassport.detail} />
                <Row label="delegation" value={delegationPassport.detail} />
                <Row label="trust ceiling" value={profile.trust_ceiling || "L4"} />
                <Row
                  label="memory scope"
                  value={
                    <span className="font-mono">
                      {agentScope(slug, profile.memory_scope)}
                    </span>
                  }
                />
                <Row
                  label="workdir"
                  value={
                    profile.workdir ? (
                      <span className="font-mono">{profile.workdir}</span>
                    ) : (
                      "—"
                    )
                  }
                />
                <Row label="retry" value={agentRetryPolicyDetail(profile)} />
                <Row
                  label="doctor"
                  value={
                    profile.health_policy?.doctor_agent ? (
                      <span className="font-mono">
                        {profile.health_policy.doctor_agent}
                      </span>
                    ) : (
                      "—"
                    )
                  }
                />
                <Row
                  label="self-repair"
                  value={
                    profile.self_repair?.enabled
                      ? `enabled${profile.self_repair?.max_attempts ? ` · ${profile.self_repair.max_attempts} attempts` : ""}`
                      : "off"
                  }
                />
                <Row label="noise policy" value={agentNoisePolicyLabel(profile)} />
                <Row
                  label="state"
                  value={
                    <span>
                      {runtimeStatus.operationalText || state}
                      {runtimeStatus.liveDetail ? ` · ${runtimeStatus.liveDetail}` : ""}
                    </span>
                  }
                />
                <Row
                  label="last activity"
                  value={
                    runtimeStatus.lastActivitySummary
                      ? `${runtimeStatus.lastActivitySummary}${runtimeStatus.lastActivityMs ? ` · ${fmtAgo(runtimeStatus.lastActivityMs)}` : ""}`
                      : "—"
                  }
                />
                <Row
                  label="next wake"
                  value={
                    runtimeStatus.nextWakeMs ? (
                      <span>
                        {fmtDue(runtimeStatus.nextWakeMs)}
                        {runtimeStatus.wakeDetail ? (
                          <span className="text-muted"> · {runtimeStatus.wakeDetail}</span>
                        ) : null}
                      </span>
                    ) : (
                      "—"
                    )
                  }
                />
              </div>
            </Disclosure>
            <div>
              <div className="mb-1 text-xs uppercase tracking-normal text-muted">
                soul — identity core
              </div>
              {profile.soul ? (
                <pre className="max-h-[28rem] overflow-auto whitespace-pre-wrap rounded-md bg-panel p-2.5 font-mono text-[11px] text-foreground/85">
                  {profile.soul}
                </pre>
              ) : (
                <div className="text-xs text-muted">
                  no soul set — this agent inherits the default daemon identity
                </div>
              )}
            </div>
            {(profile.instructions || []).length > 0 && (
              <div>
                <div className="mb-1 text-xs uppercase tracking-normal text-muted">
                  standing instructions
                </div>
                <ul className="space-y-1 rounded-md bg-panel p-2.5 text-xs text-foreground/85">
                  {(profile.instructions || []).map((ins, i) => (
                    <li key={`${i}-${ins}`}>{ins}</li>
                  ))}
                </ul>
              </div>
            )}
            <AgentTaskList
              tasks={profile.tasklist || []}
              busy={taskBusy}
              onAction={(id, op, status) => mutateTask(id, op, status)}
              onAdd={addTask}
            />
            {activeEscalationTasks.length > 0 && (
              <OperationalTaskList tasks={activeEscalationTasks} />
            )}
            {((profile.tool_allow || []).length > 0 ||
              (profile.tool_deny || []).length > 0) && (
              <div className="grid gap-2 sm:grid-cols-2">
                <ToolPolicyBox
                  title="tool allowlist"
                  items={profile.tool_allow || []}
                  empty="all advertised tools"
                />
                <ToolPolicyBox
                  title="tool denylist"
                  items={profile.tool_deny || []}
                  empty="none blocked"
                />
              </div>
            )}
            {((profile.config_overrides &&
              Object.keys(profile.config_overrides).length > 0) ||
              overrides.runtime.length > 0) && (
              <ConfigOverrideBox summary={overrides} />
            )}
            <Button
              variant="ghost"
              size="sm"
              onClick={() => onManage("roster")}
            >
              Edit in Roster <ArrowUpRight className="size-3.5" />
            </Button>
          </div>
        )}

        {tab === "triggers" && (
          <TriggersTab
            slug={slug}
            profile={profile}
            wakeAccess={wakeAccess}
            orders={myOrders}
            schedules={mySchedules}
            triggers={triggers}
            busy={busy}
            onAction={action}
            onCreateMailboxWake={createMailboxWake}
            onManage={onManage}
          />
        )}

        {tab === "model" && (
          <ModelTab
            slug={slug}
            profile={profile}
            routing={routing}
            provLog={myProvLog}
            onManage={onManage}
            onChanged={() => {
              setBump((b) => b + 1);
              onChanged?.();
            }}
          />
        )}

        {tab === "activity" && (
          <AgentActivity
            slug={slug}
            initialOpenRun={activityFocusRun}
            initialTab="activity"
          />
        )}

        {tab === "comms" && (
          <CommsTab
            slug={slug}
            messages={myComms}
            escalations={escalations}
            wokeMessages={profile.status?.mailbox_wakes}
            onFocusRun={(correlationId) => {
              setActivityFocusRun(correlationId);
              setTab("activity");
            }}
            onManage={onManage}
            onChanged={() => {
              setBump((b) => b + 1);
              onChanged?.();
            }}
          />
        )}

        {tab === "memory" && (
          <MemoryTab
            records={myMemory}
            scope={agentScope(slug, profile.memory_scope)}
            busy={busy}
            onAction={action}
            onManage={onManage}
          />
        )}

        {tab === "skills" && (
          <SkillsTab
            skills={mySkills}
            busy={busy}
            onAction={action}
            onManage={onManage}
          />
        )}

        {tab === "diag" && (
          <DiagTab
            slug={slug}
            profile={profile}
            posture={posture}
            askPolicy={askPolicy}
            edictLevels={edictLevels}
            toolCatalog={toolCatalog}
            agentPermissions={agentPermissions}
            wakePolicy={wakePolicy}
            denials={myDenials}
            approvals={myApprovals}
            toolErrors={myToolErrors}
            fail={fail}
            health={health}
            overrides={overrides}
            repair={repair}
            repairStatus={repairStatus}
            busy={busy}
            onChanged={() => {
              setBump((b) => b + 1);
              onChanged?.();
            }}
          />
        )}

        {tab === "files" && (
          <FilesTab workdir={profile.workdir} skills={mySkills} />
        )}

        {tab === "repair" && (
          <AgentRepair
            slug={slug}
            profile={profile}
            fail={fail}
            denials={myDenials}
            toolErrors={myToolErrors}
            runs={summary.runs}
            configIssues={health.configIssues}
            taskModelChain={repairTaskChain}
            onApplied={() => {
              setBump((b) => b + 1);
              onChanged?.();
            }}
          />
        )}
      </div>
    </section>
  );
}

// ───────────────────────────── sub-views ─────────────────────────────

function StatePill({
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

function AgentDetailTabButton({
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

function AgentDetailHeroFact({
  icon: Icon,
  label,
  value,
  detail,
  tone,
}: {
  icon: typeof Bot;
  label: string;
  value: string;
  detail?: string;
  tone?: "good" | "warn" | "bad" | "accent" | "muted";
}) {
  return (
    <div
      className={cn(
        "min-w-0 rounded-xl border border-border/60 bg-card/40 px-3 py-3",
        tone === "good" && "border-good/40 bg-good/10 shadow-sm shadow-good/20",
        tone === "warn" && "border-warn/50 bg-warn/15 shadow-sm shadow-warn/20",
        tone === "bad" && "border-bad/45 bg-bad/10 shadow-sm shadow-bad/20",
        tone === "accent" && "border-accent/45 bg-accent/15 shadow-sm shadow-accent/20",
        tone === "muted" && "border-border/40 bg-panel/30",
      )}
      title={detail || value}
    >
      <div className="flex items-center gap-2">
        <Icon className={cn(
          "size-6 shrink-0",
          tone === "good" && "text-good",
          tone === "warn" && "text-warn",
          tone === "bad" && "text-bad",
          tone === "accent" && "text-accent",
          tone === "muted" && "text-muted/70",
        )} />
        <span className="truncate text-xs font-medium uppercase tracking-normal text-muted">{label}</span>
      </div>
      <div className={cn(
        "mt-2 truncate text-lg font-bold tracking-normal",
        tone === "good" && "text-good",
        tone === "warn" && "text-warn",
        tone === "bad" && "text-bad",
        tone === "accent" && "text-accent",
        tone === "muted" && "text-muted",
      )}>
        {value}
      </div>
      {detail && <div className="mt-1 line-clamp-2 text-xs leading-relaxed text-muted/80">{detail}</div>}
    </div>
  );
}

function AgentNowPanel({
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
      className="grid min-h-[68px] grid-cols-[auto_1fr_auto] items-center gap-2 rounded-lg border border-accent/35 bg-accent/10 p-2"
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

/* CompactChip — a single inline metric for the "All details" strip.
   Replaces the old passport-grid + command-strip + control-center trio
   with one compact, scannable row of chips. */
function CompactChip({
  label,
  value,
  tone = "muted",
}: {
  label: string;
  value: string;
  tone?: "good" | "bad" | "warn" | "accent" | "muted";
}) {
  return (
    <span
      className={cn(
        "inline-flex items-baseline gap-1 rounded-md border border-border/60 bg-card/50 px-2 py-1 text-xs",
        tone === "good" && "border-good/30 bg-good/5 text-good",
        tone === "bad" && "border-bad/30 bg-bad/5 text-bad",
        tone === "warn" && "border-warn/30 bg-warn/8 text-warn",
        tone === "accent" && "border-accent/25 bg-accent/8 text-accent",
      )}
    >
      <span className="font-medium text-muted">{label}</span>
      <span>{value}</span>
    </span>
  );
}

function Row({ label, value }: { label: string; value: React.ReactNode }) {
  if (value == null || value === "") return null;
  return (
    <div className="flex gap-2 text-xs">
      <span className="w-28 shrink-0 text-muted">{label}</span>
      <span className="min-w-0 flex-1 break-words">{value}</span>
    </div>
  );
}

function agentDetailSkillPassport(skills: SkillLite[] | null): { value: string; tone: "good" | "warn" | "muted" } {
  if (!skills) return { value: "skills loading", tone: "muted" };
  const triggerCount = skills.reduce((sum, skill) => sum + (skill.triggers || []).length, 0);
  const base = agentSkillPassportSummary(skills.length);
  return {
    value: triggerCount > 0 ? `${base} · ${triggerCount} trigger${triggerCount === 1 ? "" : "s"}` : base,
    tone: skills.length > 0 ? "good" : "warn",
  };
}

function LifecycleConfigEditor({
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

function MiniPolicy({ label, allowed, note }: { label: string; allowed: boolean; note?: string }) {
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

function RepairCommandCell({
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
        tone === "good" && "border-good/25 bg-good/5",
        tone === "bad" && "border-bad/30 bg-bad/5",
        tone === "warn" && "border-warn/35 bg-warn/10",
        tone === "accent" && "border-accent/35 bg-accent/5",
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

function ConfigOverrideBox({
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
function Stat({
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
  const borderColor = tone === "good" ? "border-good/40 bg-good/10" : tone === "warn" ? "border-warn/50 bg-warn/15" : tone === "bad" ? "border-bad/45 bg-bad/10" : tone === "accent" ? "border-accent/45 bg-accent/15" : accent ? "border-accent/50" : "border-border/50 bg-panel/30";
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
function BudgetBar({
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

function AgentDetailCommandStrip({ items, slug }: { items: AgentCommandStripItem[]; slug: string }) {
  return (
    <div className="grid gap-1.5 sm:grid-cols-2 xl:grid-cols-3" aria-label={`${slug} detail command strip`}>
      {items.map((item) => (
        <div
          key={item.label}
          title={item.detail || item.value}
          className={cn(
            "min-w-0 rounded-md border border-border/60 bg-card/55 px-2 py-1.5",
            item.tone === "good" && "border-good/25 bg-good/5",
            item.tone === "bad" && "border-bad/30 bg-bad/5",
            item.tone === "warn" && "border-warn/35 bg-warn/10",
            item.tone === "accent" && "border-accent/30 bg-accent/10",
            item.tone === "muted" && "bg-panel/45",
          )}
        >
          <div className="flex min-w-0 items-center gap-1.5">
            <span
              className={cn(
                "size-1.5 shrink-0 rounded-full bg-muted/60",
                item.tone === "good" && "bg-good",
                item.tone === "bad" && "bg-bad",
                item.tone === "warn" && "bg-warn",
                item.tone === "accent" && "bg-accent",
              )}
            />
            <span className="truncate text-[9px] font-semibold uppercase tracking-normal text-muted/80">{item.label}</span>
          </div>
          <div
            className={cn(
              "mt-0.5 truncate text-[11px] font-medium text-foreground/90",
              item.tone === "good" && "text-good",
              item.tone === "bad" && "text-bad",
              item.tone === "warn" && "text-warn",
              item.tone === "accent" && "text-accent",
              item.tone === "muted" && "text-muted",
            )}
          >
            {item.value}
          </div>
        </div>
      ))}
    </div>
  );
}

function Overview({
  slug,
  profile,
  triggers,
  orders,
  summary,
  runtimeStatus,
  runs,
  fail,
  health,
  healthContract,
  repair,
  repairStatus,
  repairReadiness,
  escalations,
  escalation,
  escalationTasks,
  memory,
  skills,
  schedules,
  mailboxMessages,
  toolCatalog,
  edictLevels,
  agentPermissions,
  livePresence,
  commandStrip,
  busy,
  onLifecycleChanged,
  onManage,
  onView,
  onFocusRun,
  onQuietGuardian,
}: {
  slug: string;
  profile: AgentProfile;
  triggers: FleetTrigger[];
  orders: ApiOrder[];
  summary: { runs: number; totalSpentMc: number; lastStartedMs?: number };
  runtimeStatus: AgentCardRuntimeSummary;
  runs: RunLite[];
  fail?: RunLite;
  health: AgentHealthSnapshot;
  healthContract: AgentControlCenterEntry[];
  repair: AgentRepairSnapshot;
  repairStatus: AgentRepairStatus | null;
  repairReadiness: { value: string; detail: string; tone: "good" | "warn" | "bad" | "muted" };
  escalations: AgentEscalation[] | null;
  escalation: {
    openCount: number;
    ackedCount: number;
    doctorOpenCount: number;
    delegatedOpenCount: number;
    latest?: AgentEscalation;
  };
  escalationTasks: AgentOperationalTask[];
  memory: MemoryRecord[] | null;
  skills: SkillLite[] | null;
  schedules: ApiSchedule[];
  mailboxMessages: BoardMessage[];
  toolCatalog: ToolCatalogRow[] | null;
  edictLevels: Record<string, string>;
  agentPermissions: AgentPermissionsSnapshot | null;
  livePresence: { value: string; detail: string; tone: "good" | "warn" | "bad" | "muted" };
  commandStrip: AgentCommandStripItem[];
  busy: boolean;
  onLifecycleChanged: () => void;
  onManage: (view: string) => void;
  onView: (t: DetailTab) => void;
  onFocusRun: (correlationId: string | undefined) => void;
  onQuietGuardian: () => void;
}) {
  const [showActiveRun, setShowActiveRun] = useState(false);
  // Today's spend for this agent (client-side fold over its runs started today).
  const todayMs = useMemo(() => {
    const d = new Date();
    d.setHours(0, 0, 0, 0);
    return d.getTime();
  }, []);
  const todaySpent = useMemo(
    () =>
      runs
        .filter((r) => r.agent === slug && (r.started_unix_ms || 0) >= todayMs)
        .reduce((s, r) => s + (r.spent_mc || 0), 0),
    [runs, slug, todayMs],
  );
  const lifecycleMailboxMessages = useMemo(() => mailboxMessages || [], [mailboxMessages]);
  const permissions = useMemo(
    () =>
        toolCatalog && toolCatalog.length > 0
          ? effectiveToolPermissions(toolCatalog, profile, edictLevels)
          : agentPermissions?.permissions
        ? agentPermissions.permissions.map(permissionRowFromSnapshot)
        : [],
    [agentPermissions, edictLevels, profile, toolCatalog],
  );
  const permissionPassport = useMemo(
    () => summarizePermissionPassport(profile, permissions, !!toolCatalog || !!agentPermissions),
    [agentPermissions, permissions, profile, toolCatalog],
  );
  const configContract = useMemo(
    () => agentConfigAuthorityContract(profile, agentPermissions),
    [agentPermissions, profile],
  );
  const taskProgress = agentTaskProgressSummary(profile.tasklist);
  const mailboxWakeContract = useMemo(
    () => agentMailboxWakeContract(slug, orders, profile, agentPermissions?.wake_access),
    [agentPermissions?.wake_access, orders, profile, slug],
  );
  const wakePolicy = useMemo(
    () => summarizeWakeAccess(profile, agentPermissions?.wake_access),
    [agentPermissions?.wake_access, profile],
  );
  const repairOperations = useMemo(
    () => agentRepairOperationsSummary(profile, repairStatus),
    [profile, repairStatus],
  );
  const runtimeDoctorLedger = useMemo(
    () => agentRuntimeDoctorLedger(runtimeStatus, repairOperations, repairReadiness, escalation),
    [escalation, repairOperations, repairReadiness, runtimeStatus],
  );
  const operationsPassport = useMemo(
    () =>
      agentOperationsPassport(
        profile,
        runtimeStatus,
        mailboxWakeContract,
        permissionPassport,
        configContract,
        repairOperations,
      ),
    [configContract, mailboxWakeContract, permissionPassport, profile, repairOperations, runtimeStatus],
  );
  const entityContract = useMemo(
    () =>
      agentEntityContractLedger(
        slug,
        profile,
        runtimeStatus,
        mailboxWakeContract,
        permissionPassport,
        configContract,
        repairOperations,
      ),
    [configContract, mailboxWakeContract, permissionPassport, profile, repairOperations, runtimeStatus, slug],
  );
  const autonomyRunbook = useMemo(
    () =>
      agentAutonomyRunbook(
        profile,
        runtimeStatus,
        mailboxWakeContract,
        wakePolicy,
        repairOperations,
      ),
    [mailboxWakeContract, profile, repairOperations, runtimeStatus, wakePolicy],
  );
  const lastWakeLineage = wakeLineage(profile.status?.last_autonomy_runbook);
  const systemGuardianContract = useMemo(
    () => agentSystemGuardianContract(profile),
    [profile],
  );

  return (
    <div className="space-y-3">
      {profile.retired && (
        <div className="rounded-lg bg-panel/40 p-2.5">
          <div className="mb-1 flex items-center gap-1.5 text-xs font-semibold uppercase tracking-normal text-muted">
            <Skull className="size-3" /> lifecycle
          </div>
          <div className="space-y-1 text-xs text-muted">
            <div>
              This identity is in the graveyard
              {profile.retired_ms
                ? ` since ${fmtDateTime(profile.retired_ms)}`
                : ""}
              . Its soul, logs, memory, skills, and mailbox remain inspectable;
              schedules and delegation will not wake it until it is revived.
            </div>
            {profile.retired_reason && (
              <div className="text-foreground/80">
                Reason: {profile.retired_reason}
              </div>
            )}
          </div>
        </div>
      )}

      <Disclosure
        summary={<span className="text-[11px] font-medium text-foreground/90">Contract &amp; activity records</span>}
      >
        <div className="space-y-3">
      <AgentDetailCommandStrip items={commandStrip} slug={slug} />

      <AgentEntityContract entries={entityContract} slug={slug} />

      <AutonomyRunbook entries={autonomyRunbook} slug={slug} />

      {lastWakeLineage.label && (lastWakeLineage.incidentId || lastWakeLineage.parentCorrelationId) && (
        <div className="flex flex-wrap items-center gap-2 text-xs text-muted">
          <span>last wake {lastWakeLineage.label}</span>
          {lastWakeLineage.incidentId && (
            <button
              onClick={() => openIncident(lastWakeLineage.incidentId!)}
              className="inline-flex items-center gap-1 rounded bg-card px-1.5 py-0.5 text-accent transition-colors hover:text-accent2"
              title="Open the incident that woke this agent"
            >
              <Zap className="size-3" /> incident {clip(lastWakeLineage.incidentId, 24)}
            </button>
          )}
          {lastWakeLineage.parentCorrelationId && (
            <span
              className="rounded bg-card px-1.5 py-0.5 font-mono text-xs"
              title="The lead/parent run that delegated this wake"
            >
              parent run {clip(lastWakeLineage.parentCorrelationId, 24)}
            </span>
          )}
        </div>
      )}

      <LifecycleInterventionPanel
        slug={slug}
        profile={profile}
        schedules={schedules}
        memory={memory}
        skills={skills}
        mailboxMessages={lifecycleMailboxMessages}
        busy={busy}
        onChanged={onLifecycleChanged}
      />

      <div
        className={cn(
          "rounded-lg bg-panel/40 p-2.5",
          operationsPassport.tone === "good" && "bg-good/8",
          operationsPassport.tone === "warn" && "bg-warn/10",
          operationsPassport.tone === "bad" && "bg-bad/8",
        )}
        title={operationsPassport.detail}
      >
        <div className="mb-1 flex items-center gap-1.5 text-xs font-semibold uppercase tracking-normal text-muted">
          <IdCard className="size-3" /> Operations passport
        </div>
        <div
          className={cn(
            "text-xs font-medium text-foreground",
            operationsPassport.tone === "good" && "text-good",
            operationsPassport.tone === "warn" && "text-warn",
            operationsPassport.tone === "bad" && "text-bad",
          )}
        >
          {operationsPassport.value}
        </div>
        <div className="mt-1 text-[11px] text-muted">{operationsPassport.detail}</div>
        <RuntimeDoctorLedger entries={runtimeDoctorLedger} slug={slug} />
      </div>

      {systemGuardianContract && (
        <div
          className={cn(
            "rounded-lg border border-border bg-panel/40 p-2.5",
            systemGuardianContract.tone === "good" && "border-good/30 bg-good/5",
            systemGuardianContract.tone === "warn" && "border-warn/40 bg-warn/10",
            systemGuardianContract.tone === "bad" && "border-bad/35 bg-bad/5",
          )}
          title={systemGuardianContract.detail}
        >
          <div className="mb-1 flex items-center gap-1.5 text-xs font-semibold uppercase tracking-normal text-muted">
            <Megaphone className="size-3" /> System guardian contract
            {systemGuardianContract.tone !== "good" && (
              <Button
                className="ml-auto"
                size="sm"
                variant="ghost"
                disabled={busy}
                onClick={onQuietGuardian}
                title="Apply quiet system guardian policy to this agent"
              >
                <Megaphone className="size-3.5" /> Quiet guardian
              </Button>
            )}
          </div>
          <div
            className={cn(
              "text-xs font-medium text-foreground",
              systemGuardianContract.tone === "good" && "text-good",
              systemGuardianContract.tone === "warn" && "text-warn",
              systemGuardianContract.tone === "bad" && "text-bad",
            )}
          >
            {systemGuardianContract.value}
          </div>
          <div className="mt-1 text-[11px] text-muted">{systemGuardianContract.detail}</div>
        </div>
      )}

      {/* Operational state - visual dashboard */}
      <div className="rounded-xl border border-border/60 bg-panel/30 p-3">
        <div className="mb-3 flex items-center gap-2 text-xs font-semibold uppercase tracking-normal text-muted">
          <Zap className="size-4 text-accent" /> Status Dashboard
        </div>
        <div className="grid grid-cols-2 gap-3 sm:grid-cols-4 lg:grid-cols-6">
          {/* Operational State */}
          <div className={cn(
            "flex flex-col items-center gap-1.5 rounded-lg p-3 text-center",
            runtimeStatus.activeRunCount > 0 ? "bg-accent/15 border border-accent/30" :
            runtimeStatus.operationalState === "paused" || runtimeStatus.operationalState === "retired" 
              ? "bg-panel border border-border" : "bg-good/10 border border-good/30"
          )}>
            <div className={cn(
              "flex h-10 w-10 items-center justify-center rounded-full",
              runtimeStatus.activeRunCount > 0 ? "bg-accent/20" :
              runtimeStatus.operationalState === "paused" || runtimeStatus.operationalState === "retired" 
                ? "bg-panel/60" : "bg-good/20"
            )}>
              {runtimeStatus.activeRunCount > 0 ? (
                <ActivityIcon className="size-5 text-accent animate-pulse" />
              ) : runtimeStatus.operationalState === "paused" ? (
                <Pause className="size-5 text-muted" />
              ) : runtimeStatus.operationalState === "retired" ? (
                <Skull className="size-5 text-muted" />
              ) : (
                <Flame className="size-5 text-good" />
              )}
            </div>
            <span className={cn(
              "text-sm font-bold",
              runtimeStatus.activeRunCount > 0 ? "text-accent" :
              runtimeStatus.operationalState === "paused" || runtimeStatus.operationalState === "retired" 
                ? "text-muted" : "text-good"
            )}>
              {runtimeStatus.operationalText || "sleeping"}
            </span>
            <span className="text-xs text-muted/70">State</span>
          </div>

          {/* Live Presence */}
          <div className={cn(
            "flex flex-col items-center gap-1.5 rounded-lg p-3 text-center",
            livePresence.tone === "good" ? "bg-good/10 border border-good/30" :
            livePresence.tone === "bad" ? "bg-bad/10 border border-bad/30" : "bg-panel border border-border"
          )}>
            <div className={cn(
              "flex h-10 w-10 items-center justify-center rounded-full",
              livePresence.tone === "good" ? "bg-good/20" :
              livePresence.tone === "bad" ? "bg-bad/20" : "bg-panel/60"
            )}>
              {livePresence.tone === "good" ? (
                <CheckCheck className="size-5 text-good" />
              ) : livePresence.tone === "bad" ? (
                <AlertTriangle className="size-5 text-bad" />
              ) : (
                <Clock className="size-5 text-muted" />
              )}
            </div>
            <span className={cn(
              "text-sm font-bold",
              livePresence.tone === "good" ? "text-good" :
              livePresence.tone === "bad" ? "text-bad" : "text-muted"
            )}>
              {livePresence.value}
            </span>
            <span className="text-xs text-muted/70">Live presence</span>
          </div>

          {/* Last Activity */}
          <div className="flex flex-col items-center gap-1.5 rounded-lg bg-panel/40 p-3 text-center">
            <div className="flex h-10 w-10 items-center justify-center rounded-full bg-panel/60">
              <Clock className="size-5 text-muted" />
            </div>
            <span className="text-sm font-bold text-foreground/90">
              {runtimeStatus.lastActivityMs ? fmtAgo(runtimeStatus.lastActivityMs) : "—"}
            </span>
            <span className="text-xs text-muted/70">Last active</span>
          </div>

          {/* Wake Source */}
          <div className="flex flex-col items-center gap-1.5 rounded-lg bg-panel/40 p-3 text-center">
            <div className="flex h-10 w-10 items-center justify-center rounded-full bg-panel/60">
              <CalendarClock className="size-5 text-muted" />
            </div>
            <span className="text-sm font-bold text-foreground/90">
              {runtimeStatus.wakeText || "wake: none"}
            </span>
            <span className="text-xs text-muted/70">Wake source</span>
          </div>

          {/* Next Wake */}
          <div className="flex flex-col items-center gap-1.5 rounded-lg bg-panel/40 p-3 text-center">
            <div className="flex h-10 w-10 items-center justify-center rounded-full bg-panel/60">
              <ArrowRight className="size-5 text-muted" />
            </div>
            <span className="text-sm font-bold text-foreground/90">
              {runtimeStatus.nextWakeMs ? fmtDue(runtimeStatus.nextWakeMs) : "—"}
            </span>
            <span className="text-xs text-muted/70">Next wake</span>
          </div>

          {/* Task Progress */}
          <div className="flex flex-col items-center gap-1.5 rounded-lg bg-panel/40 p-3 text-center">
            <div className="flex h-10 w-10 items-center justify-center rounded-full bg-panel/60">
              <Waypoints className="size-5 text-muted" />
            </div>
            <span className="text-sm font-bold text-foreground/90">
              {taskProgress || "—"}
            </span>
            <span className="text-xs text-muted/70">Tasks</span>
          </div>
        </div>

        {/* Quick actions for active run */}
        {runtimeStatus.activeCorrelationId && (
          <div className="mt-3 flex items-center gap-2 rounded-lg border border-accent/30 bg-accent/10 p-2">
            <ActivityIcon className="size-4 animate-pulse text-accent" />
            <span className="flex-1 text-xs text-muted">
              Active run: <span className="font-mono text-accent">{clip(runtimeStatus.activeCorrelationId, 20)}</span>
            </span>
            <Button
              variant="ghost"
              size="sm"
              onClick={() => setShowActiveRun((v) => !v)}
              className="text-xs"
            >
              {showActiveRun ? "Hide" : "Inspect"}
            </Button>
            <Button
              variant="ghost"
              size="sm"
              onClick={() => {
                onFocusRun(runtimeStatus.activeCorrelationId);
                onView("activity");
              }}
              className="text-xs"
              title="Inspect run"
              aria-label="Inspect run"
            >
              <ListTree className="size-3.5" />
            </Button>
          </div>
        )}
        {showActiveRun && runtimeStatus.activeCorrelationId && (
          <div className="mt-2 rounded-md bg-card/40 p-2">
            <RunDetailLoader correlationId={runtimeStatus.activeCorrelationId} status="running" />
          </div>
        )}
      </div>

      <div
        className={cn(
          "rounded-lg bg-panel/40 p-2.5",
          mailboxWakeContract.tone === "good" && "bg-good/8",
          mailboxWakeContract.tone === "warn" && "bg-warn/10",
          mailboxWakeContract.tone === "bad" && "bg-bad/8",
        )}
        title={mailboxWakeContract.detail}
      >
        <div className="mb-1 flex items-center gap-1.5 text-xs font-semibold uppercase tracking-normal text-muted">
          <Mail className="size-3" /> Mailbox wake contract
        </div>
        <div
          className={cn(
            "text-xs font-medium text-foreground",
            mailboxWakeContract.tone === "good" && "text-good",
            mailboxWakeContract.tone === "warn" && "text-warn",
            mailboxWakeContract.tone === "bad" && "text-bad",
          )}
        >
          {mailboxWakeContract.value}
        </div>
        <div className="mt-1 text-xs text-muted">{mailboxWakeContract.detail}</div>
      </div>

        </div>
      </Disclosure>

      {/* How it runs */}
      <div className="rounded-lg bg-accent/8 p-2.5">
        <div className="mb-1.5 flex items-center gap-1.5 text-xs font-semibold uppercase tracking-normal text-accent">
          <ActivityIcon className="size-3" /> How does this run?
        </div>
        <div className="flex flex-wrap gap-2">
          {triggers.length === 0 ? (
            <span className="text-[11px] text-muted">
              manual / delegated only — runs when you or another agent calls it
            </span>
          ) : (
            triggers.map((t, i) => (
              <TriggerChip
                key={`${t.mode}-${i}`}
                mode={t.mode}
                label={t.label}
              />
            ))
          )}
        </div>
      </div>

      <button
        onClick={() => onView("diag")}
        className={cn(
          "flex w-full items-start gap-2 rounded-lg border p-2.5 text-left",
          permissionPassport.level === "tight"
            ? "border-good/30 bg-good/5"
            : permissionPassport.level === "open"
              ? "border-warn/40 bg-warn/10"
              : "border-border bg-panel/40",
        )}
      >
        <ShieldCheck
          className={cn(
            "mt-0.5 size-3.5 shrink-0",
            permissionPassport.level === "tight"
              ? "text-good"
              : permissionPassport.level === "open"
                ? "text-warn"
                : "text-muted",
          )}
        />
        <div className="min-w-0 flex-1">
          <div
            className={cn(
              "text-[11px] font-medium",
              permissionPassport.level === "tight"
                ? "text-good"
                : permissionPassport.level === "open"
                  ? "text-warn"
                  : "text-foreground/80",
            )}
          >
            Capability passport
          </div>
          <div className="text-[11px] text-muted">{permissionPassport.detail}</div>
          {permissionPassport.policy && (
            <div className="mt-1 flex flex-wrap gap-1.5 text-xs uppercase tracking-normal text-muted">
              {permissionPassport.policy.map((item) => (
                <span key={item}>{item}</span>
              ))}
            </div>
          )}
        </div>
        <ChevronRight className="ml-auto size-3.5 shrink-0 text-muted" />
      </button>

      <button
        onClick={() => onView("diag")}
        className={cn(
          "flex w-full items-start gap-2 rounded-lg border p-2.5 text-left",
          configContract.tone === "good"
            ? "border-good/30 bg-good/5"
            : configContract.tone === "warn"
              ? "border-warn/40 bg-warn/10"
              : configContract.tone === "bad"
                ? "border-bad/40 bg-bad/5"
                : "border-border bg-panel/40",
        )}
        title={configContract.detail}
      >
        <ShieldCheck
          className={cn(
            "mt-0.5 size-3.5 shrink-0",
            configContract.tone === "good"
              ? "text-good"
              : configContract.tone === "warn"
                ? "text-warn"
                : configContract.tone === "bad"
                  ? "text-bad"
                  : "text-muted",
          )}
        />
        <div className="min-w-0 flex-1">
          <div
            className={cn(
              "text-[11px] font-medium",
              configContract.tone === "good"
                ? "text-good"
                : configContract.tone === "warn"
                  ? "text-warn"
                  : configContract.tone === "bad"
                    ? "text-bad"
                    : "text-foreground/80",
            )}
          >
            Config authority
          </div>
          <div className="text-[11px] text-muted">{configContract.value}</div>
          <div className="mt-0.5 text-xs text-muted">{configContract.detail}</div>
        </div>
        <ChevronRight className="ml-auto size-3.5 shrink-0 text-muted" />
      </button>

      {/* Headline stats */}
      <div className="grid grid-cols-2 gap-2 sm:grid-cols-4">
        <Stat icon={Bot} label="runs" value={summary.runs} />
        <Stat
          icon={Coins}
          label="total spend"
          value={money(summary.totalSpentMc)}
        />
        <Stat
          icon={Clock}
          label="last active"
          value={
            runtimeStatus.lastActivityMs
              ? fmtAgo(runtimeStatus.lastActivityMs)
              : summary.lastStartedMs
                ? fmtAgo(summary.lastStartedMs)
                : "never"
          }
          detail={runtimeStatus.lastActivitySummary}
        />
        <Stat
          icon={
            health.state === "healthy"
              ? ShieldCheck
              : health.state === "retired"
                ? Skull
                : AlertTriangle
          }
          label="health"
          value={health.label}
          accent={health.state !== "healthy"}
        />
      </div>

      <button
        onClick={() => onView("diag")}
        className={cn(
          "flex w-full items-start gap-2 rounded-lg border p-2.5 text-left",
          health.state === "healthy"
            ? "border-good/30 bg-good/5"
            : health.state === "retired"
              ? "border-border bg-panel/40"
              : "border-bad/40 bg-bad/5",
        )}
      >
        {health.state === "healthy" ? (
          <ShieldCheck className="mt-0.5 size-3.5 shrink-0 text-good" />
        ) : health.state === "retired" ? (
          <Skull className="mt-0.5 size-3.5 shrink-0 text-muted" />
        ) : (
          <AlertTriangle className="mt-0.5 size-3.5 shrink-0 text-bad" />
        )}
        <div className="min-w-0">
          <div
            className={cn(
              "text-[11px] font-medium",
              health.state === "healthy"
                ? "text-good"
                : health.state === "retired"
                  ? "text-foreground/80"
                  : "text-bad",
            )}
          >
            Health status
          </div>
          <div className="text-[11px] text-muted">{health.detail}</div>
        </div>
        <ChevronRight className="ml-auto size-3.5 shrink-0 text-muted" />
      </button>

      <button
        onClick={() => onView("repair")}
        className="flex w-full items-start gap-2 rounded-lg border border-border bg-panel/40 p-2.5 text-left"
      >
        <ActivityIcon className="mt-0.5 size-3.5 shrink-0 text-accent" />
        <div className="min-w-0 flex-1">
          <div className="text-[11px] font-medium text-foreground/80">
            Health contract
          </div>
          {/* Health indicators - more visual */}
          <div className="mt-1 grid grid-cols-3 gap-1.5">
            {healthContract.map((entry) => (
              <div
                key={entry.label}
                title={`${entry.label}: ${entry.detail}`}
                className={cn(
                  "flex flex-col items-center gap-0.5 rounded-md border p-1.5 text-center",
                  entry.tone === "good" && "border-good/30 bg-good/10",
                  entry.tone === "warn" && "border-warn/30 bg-warn/10",
                  entry.tone === "bad" && "border-bad/30 bg-bad/10",
                  entry.tone === "muted" && "border-border bg-panel/30",
                )}
              >
                {entry.tone === "good" ? (
                  <CheckCircle className="size-3.5 text-good" />
                ) : entry.tone === "warn" ? (
                  <AlertCircle className="size-3.5 text-warn" />
                ) : entry.tone === "bad" ? (
                  <XCircle className="size-3.5 text-bad" />
                ) : (
                  <MinusCircle className="size-3.5 text-muted" />
                )}
                <div className={cn(
                  "truncate text-[9px] font-medium",
                  entry.tone === "good" && "text-good",
                  entry.tone === "warn" && "text-warn",
                  entry.tone === "bad" && "text-bad",
                  entry.tone === "muted" && "text-muted",
                )}>
                  {entry.label}
                </div>
                <div className="max-w-full truncate text-[9px] text-muted/85">
                  {entry.value}
                </div>
              </div>
            ))}
          </div>
        </div>
        <ChevronRight className="ml-auto size-3.5 shrink-0 text-muted" />
      </button>

      <button
        onClick={() => onView("soul")}
        className={cn(
          "flex w-full items-start gap-2 rounded-lg border p-2.5 text-left",
          profile.retry_policy?.max_attempts ? "border-good/30 bg-good/5" : "border-warn/40 bg-warn/10",
        )}
      >
        <Repeat className={cn("mt-0.5 size-3.5 shrink-0", profile.retry_policy?.max_attempts ? "text-good" : "text-warn")} />
        <div className="min-w-0">
          <div className={cn("text-[11px] font-medium", profile.retry_policy?.max_attempts ? "text-good" : "text-warn")}>
            Run retry policy
          </div>
          <div className="text-[11px] text-muted">{agentRetryPolicyDetail(profile)}</div>
        </div>
        <ChevronRight className="ml-auto size-3.5 shrink-0 text-muted" />
      </button>

      <button
        onClick={() => onView("files")}
        className="flex w-full items-start gap-2 rounded-lg border border-border bg-panel/40 p-2.5 text-left"
      >
        <FolderOpen className="mt-0.5 size-3.5 shrink-0 text-muted" />
        <div className="min-w-0">
          <div className="text-[11px] font-medium text-foreground/80">
            Resource passport
          </div>
          <div className="text-[11px] text-muted">{agentResourcePassportDetail(profile, slug)}</div>
        </div>
        <ChevronRight className="ml-auto size-3.5 shrink-0 text-muted" />
      </button>

      <button
        onClick={() => onView("repair")}
        className={cn(
          "flex w-full items-start gap-2 rounded-lg border p-2.5 text-left",
          repair.state === "completed"
            ? "border-good/30 bg-good/5"
            : repair.state === "failed"
              ? "border-bad/40 bg-bad/5"
              : repair.state === "queued"
                ? "border-accent/40 bg-accent/5"
                : "border-border bg-panel/40",
        )}
      >
        {repair.state === "completed" ? (
          <ShieldCheck className="mt-0.5 size-3.5 shrink-0 text-good" />
        ) : repair.state === "failed" ? (
          <AlertTriangle className="mt-0.5 size-3.5 shrink-0 text-bad" />
        ) : repair.state === "queued" ? (
          <Wrench className="mt-0.5 size-3.5 shrink-0 text-accent" />
        ) : (
          <Wrench className="mt-0.5 size-3.5 shrink-0 text-muted" />
        )}
        <div className="min-w-0">
          <div
            className={cn(
              "text-[11px] font-medium",
              repair.state === "completed"
                ? "text-good"
                : repair.state === "failed"
                  ? "text-bad"
                  : repair.state === "queued"
                    ? "text-accent"
                    : "text-foreground/80",
            )}
          >
            Auto-repair
          </div>
          <div className="text-[11px] text-muted">{repair.detail}</div>
          <div
            title={repairReadiness.detail}
            className={cn(
              "mt-1 inline-flex max-w-full items-center gap-1.5 rounded-md border border-border bg-card px-1.5 py-0.5 text-xs",
              repairReadiness.tone === "good" && "border-good/30 bg-good/10 text-good",
              repairReadiness.tone === "warn" && "border-warn/35 bg-warn/10 text-warn",
              repairReadiness.tone === "bad" && "border-bad/35 bg-bad/10 text-bad",
            )}
          >
            <span className="font-medium">{repairReadiness.value}</span>
            <span className="truncate text-muted">{repairReadiness.detail}</span>
          </div>
          {repair.mode && (
            <div className="mt-1 text-xs uppercase tracking-normal text-muted">
              {repair.mode === "degraded"
                ? "degraded doctor flow"
                : "config repair flow"}
            </div>
          )}
          {repairStatus?.latest && (
            <button
              onClick={() =>
                repairStatus.latest?.root_incident_id
                  ? openIncident(repairStatus.latest.root_incident_id)
                  : repairStatus.latest?.incident_id
                    ? openIncident(repairStatus.latest.incident_id)
                    : undefined
              }
              className="mt-1 text-xs uppercase tracking-normal text-muted transition-colors hover:text-accent"
              title="Open this repair incident"
            >
              {incidentLineageLabel(repairStatus.latest)}
            </button>
          )}
          {repair.nextEligibleMs && repair.nextEligibleMs > Date.now() && (
            <div className="mt-1 font-mono text-xs text-muted">
              cooldown until {fmtDateTime(repair.nextEligibleMs)}
            </div>
          )}
        </div>
        <div className="ml-auto flex shrink-0 items-center gap-2">
          {repairStatus?.inflight_count ? (
            <Badge variant="accent">
              {repairStatus.inflight_count} inflight
            </Badge>
          ) : null}
          <ChevronRight className="size-3.5 text-muted" />
        </div>
      </button>

      {(escalations || []).length > 0 && (
        <button
          onClick={() => onView("comms")}
          className={cn(
            "flex w-full items-start gap-2 rounded-lg border p-2.5 text-left",
            escalation.openCount > 0
              ? "border-warn/40 bg-warn/10"
              : "border-border bg-panel/40",
          )}
        >
          <LifeBuoy
            className={cn(
              "mt-0.5 size-3.5 shrink-0",
              escalation.openCount > 0 ? "text-warn" : "text-muted",
            )}
          />
          <div className="min-w-0">
            <div
              className={cn(
                "text-[11px] font-medium",
                escalation.openCount > 0 ? "text-warn" : "text-foreground/80",
              )}
            >
              Escalation queue
            </div>
            <div className="text-[11px] text-muted">
              {escalation.openCount > 0
                ? `${escalation.openCount} open escalation${escalation.openCount === 1 ? "" : "s"} waiting for this agent`
                : escalation.ackedCount > 0
                  ? `${escalation.ackedCount} escalation${escalation.ackedCount === 1 ? "" : "s"} acknowledged by this agent`
                  : "no open escalations are currently assigned here"}
            </div>
            {(escalation.doctorOpenCount > 0 ||
              escalation.delegatedOpenCount > 0) && (
              <div className="mt-1 flex flex-wrap gap-1.5 text-xs uppercase tracking-normal text-muted">
                {escalation.doctorOpenCount > 0 && (
                  <span>doctor {escalation.doctorOpenCount}</span>
                )}
                {escalation.delegatedOpenCount > 0 && (
                  <span>delegated {escalation.delegatedOpenCount}</span>
                )}
              </div>
            )}
            {escalation.latest?.source_agent && (
              <button
                onClick={() =>
                  escalation.latest?.root_incident_id
                    ? openIncident(escalation.latest.root_incident_id)
                    : escalation.latest?.incident_id
                      ? openIncident(escalation.latest.incident_id)
                      : undefined
                }
                className="mt-1 text-xs uppercase tracking-normal text-muted transition-colors hover:text-accent"
                title="Open this escalation incident"
              >
                latest for {escalation.latest.source_agent}
                {escalation.latest.mode === "degraded"
                  ? " · degraded doctor flow"
                  : " · config repair flow"}
                {(() => {
                  const chain = escalationChainLabel(escalation.latest);
                  const incident = incidentLineageLabel(escalation.latest);
                  return [chain, incident].filter(Boolean).join(" · ")
                    ? ` · ${[chain, incident].filter(Boolean).join(" · ")}`
                    : "";
                })()}
              </button>
            )}
            {escalation.latest?.resolution_summary && (
              <div className="mt-1 text-[11px] text-muted">
                {escalation.latest.resolution_summary}
              </div>
            )}
          </div>
          <div className="ml-auto flex shrink-0 items-center gap-2">
            {escalation.openCount > 0 ? (
              <Badge variant="warn">{escalation.openCount} open</Badge>
            ) : null}
            <ChevronRight className="size-3.5 text-muted" />
          </div>
        </button>
      )}

      {/* Budgets */}
      <div className="grid gap-2 sm:grid-cols-2">
        <BudgetBar
          label="today's spend"
          spentMc={todaySpent}
          capMc={profile.max_daily_mc}
        />
        <BudgetBar
          label="per-run ceiling"
          spentMc={0}
          capMc={profile.max_cost_mc}
        />
      </div>

      {/* Identity */}
      <div className="space-y-1.5 rounded-lg border border-border bg-panel/30 p-2.5">
        <Row
          label="model"
          value={
            profile.model ? (
              <span className="font-mono">{profile.model}</span>
            ) : (
              "(daemon default)"
            )
          }
        />
        {(profile.fallbacks || []).length > 0 && (
          <Row
            label="fallbacks"
            value={
              <span className="font-mono">
                {(profile.fallbacks || []).join(" → ")}
              </span>
            }
          />
        )}
        <Row label="task type" value={profile.task_type} />
        <Row
          label="lifecycle"
          value={
            <span>
              {agentLifecycleSummary(profile)} · {agentLifecycleDetail(profile)}
            </span>
          }
        />
        <Row label="call policy" value={agentHierarchySummary(profile)} />
        <Row label="trust ceiling" value={profile.trust_ceiling || "L4"} />
        <Row
          label="memory scope"
          value={
            <span className="font-mono">
              {agentScope(slug, profile.memory_scope)}
            </span>
          }
        />
        <Row
          label="workdir"
          value={
            profile.workdir ? (
              <span className="font-mono">{profile.workdir}</span>
            ) : undefined
          }
        />
        <Row label="resources" value={agentResourcePassportDetail(profile, slug)} />
      </div>

      {escalationTasks.length > 0 && (
        <div className="rounded-lg border border-warn/30 bg-warn/5 p-2.5">
          <div className="mb-1 text-xs uppercase tracking-normal text-warn">
            active responsibilities
          </div>
          <OperationalTaskList tasks={escalationTasks} compact />
        </div>
      )}

      {/* Last failure — the "ne bok yedi" headline */}
      {fail && (
        <button
          onClick={() => onView("diag")}
          className="flex w-full items-start gap-2 rounded-lg border border-bad/40 bg-bad/5 p-2.5 text-left"
        >
          <AlertTriangle className="mt-0.5 size-3.5 shrink-0 text-bad" />
          <div className="min-w-0">
            <div className="text-[11px] font-medium text-bad">
              Most recent failure
            </div>
            <div
              className="truncate text-[11px] text-muted"
              title={fail.status}
            >
              {clip(fail.correlation_id || "run", 48)} ·{" "}
              {fail.started_unix_ms ? fmtTime(fail.started_unix_ms) : "—"} — see
              Diagnostics
            </div>
          </div>
          <ChevronRight className="ml-auto size-3.5 shrink-0 text-muted" />
        </button>
      )}

      <div className="flex flex-wrap gap-2">
        <Button variant="ghost" size="sm" onClick={() => onManage("roster")}>
          Manage in Roster <ArrowUpRight className="size-3.5" />
        </Button>
      </div>
    </div>
  );
}

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

function agentDetailRemovalCascadePreset(mode: "clean_all" | "keep_all"): ResolvedAgentRemovalCascade {
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

function LifecycleInterventionPanel({
  slug,
  profile,
  schedules,
  memory,
  skills,
  mailboxMessages,
  busy,
  onChanged,
}: {
  slug: string;
  profile: AgentProfile;
  schedules: ApiSchedule[];
  memory: MemoryRecord[] | null;
  skills: SkillLite[] | null;
  mailboxMessages?: BoardMessage[];
  busy: boolean;
  onChanged: () => void;
}) {
  const ui = useUI();
  const fallbackImpact = useMemo<AgentImpactSummary>(
    () => ({
      schedules: schedules.map((s) => `${s.intent || s.id} (${s.id})`),
      memories: (memory || []).map((m) => `${m.subject || m.id} (${m.id})`),
      skills: (skills || []).map((s) => `${s.name || s.id} (${s.id})`),
      mailbox_messages: (mailboxMessages || []).map((m) => `${m.topic || "board"} ${m.id || m.ts_unix_ms || ""}`.trim()),
    }),
    [mailboxMessages, memory, schedules, skills],
  );
  const [impact, setImpact] = useState<AgentImpactSummary>(fallbackImpact);
  const [expanded, setExpanded] = useState(false);
  const [reason, setReason] = useState("");
  const [lastAction, setLastAction] = useState<AgentLifecycleActionResultSummary | null>(null);
  const [cascade, setCascade] = useState({
    standing: false,
    schedules: false,
    memory: false,
    authored_memory: false,
    skills: false,
    config: false,
    workspace: false,
    subagents: false,
  });
  const [working, setWorking] = useState(false);

  useEffect(() => {
    let alive = true;
    getJSON<AgentImpactSummary>("/api/agents/impact", { ref: slug })
      .then((next) => {
        if (!alive) return;
        setImpact({ ...fallbackImpact, ...next });
        const nextSubagents = (next.subagents || []).length > 0;
        setCascade({
          standing: (next.standing_orders || []).length > 0,
          schedules: (next.schedules || fallbackImpact.schedules || []).length > 0,
          memory: (next.memories || fallbackImpact.memories || []).length > 0 || (nextSubagents && (next.subagent_memories || []).length > 0),
          authored_memory: false,
          skills: (next.skills || fallbackImpact.skills || []).length > 0 || (nextSubagents && (next.subagent_skills || []).length > 0),
          config: (next.configs || []).length > 0 || (nextSubagents && (next.subagent_configs || []).length > 0),
          workspace: (next.workspaces || []).length > 0 || (nextSubagents && (next.subagent_workspaces || []).length > 0),
          subagents: nextSubagents,
        });
      })
      .catch(() => {
        if (!alive) return;
        setImpact(fallbackImpact);
        setCascade({
          standing: false,
          schedules: (fallbackImpact.schedules || []).length > 0,
          memory: (fallbackImpact.memories || []).length > 0,
          authored_memory: false,
          skills: (fallbackImpact.skills || []).length > 0,
          config: false,
          workspace: false,
          subagents: false,
        });
      });
    return () => {
      alive = false;
    };
  }, [fallbackImpact, slug]);

  async function applyRetire() {
    setWorking(true);
    try {
      const res = await postAction<AgentRetireResult>(
        "/api/agents/retire",
        reason.trim() ? { ref: slug, reason: reason.trim() } : { ref: slug },
      );
      ui.toast(agentRetireToast(slug, res), "success");
      setLastAction(agentLifecycleActionResultSummary("retire", slug, res));
      setReason("");
      onChanged();
    } catch (e) {
      ui.toast((e as Error).message, "error");
    } finally {
      setWorking(false);
    }
  }

  async function applyRevive() {
    setWorking(true);
    try {
      const res = await postAction<AgentReviveResult>("/api/agents/revive", { ref: slug });
      ui.toast(agentReviveToast(slug, res), "success");
      setLastAction(agentLifecycleActionResultSummary("revive", slug, res));
      onChanged();
    } catch (e) {
      ui.toast((e as Error).message, "error");
    } finally {
      setWorking(false);
    }
  }

  async function applyRemove() {
    const confirmed = await ui.confirm({
      title: `Remove agent ${slug}?`,
      message: [
        "This permanently deletes the agent identity.",
        `Risk: ${removalRisk}.`,
        cleanupPlan.length > 0 ? `Clean: ${cleanupPlan.join(", ")}.` : "No dependent cleanup selected.",
        keepPlan.length > 0 ? `Keep: ${keepPlan.join(", ")}.` : "",
      ].filter(Boolean).join(" "),
      confirmLabel: "Remove",
      danger: true,
    });
    if (!confirmed) return;
    setWorking(true);
    try {
      const res = await postJSON<AgentRemoveResult>("/api/agents/remove", { ref: slug, cascade });
      ui.toast(
        agentRemoveToast(slug, res),
        res.removed ? "success" : "info",
      );
      setLastAction(agentLifecycleActionResultSummary("remove", slug, res));
      onChanged();
    } catch (e) {
      ui.toast((e as Error).message, "error");
    } finally {
      setWorking(false);
    }
  }

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
  const hasImpact = standing.length + scheduleItems.length + memoryItems.length + authoredMemoryItems.length + skillItems.length + configItems.length + workspaceItems.length + workflowRefs.length + mailboxItems.length + subagentItems.length + subagentStanding.length + subagentSchedules.length + subagentMemories.length + subagentAuthoredMemories.length + subagentSkills.length + subagentConfigs.length + subagentWorkspaces.length + subagentWorkflowRefs.length + subagentMailboxMessages.length > 0;
  const disabled = busy || working;
  const removalPlan = agentRemovalImpactPlan(impact, cascade);
  const cleanupPlan = removalPlan.clean;
  const keepPlan = removalPlan.keep;
  const removeBlockedBySubagents = removalPlan.blockedBySubagents;
  const removalRisk = agentRemovalRiskLabel(removalPlan);
  const interventionSummary = agentLifecycleInterventionSummary(profile, removalPlan);
  const lifecycleLedger = agentLifecycleDecisionLedger(profile, removalPlan);
  const toggleItems = {
    standing: cascade.subagents ? [...standing, ...subagentStanding] : standing,
    schedules: cascade.subagents ? [...scheduleItems, ...subagentSchedules] : scheduleItems,
    memory: cascade.subagents ? [...memoryItems, ...subagentMemories] : memoryItems,
    authoredMemory: cascade.subagents ? [...authoredMemoryItems, ...subagentAuthoredMemories] : authoredMemoryItems,
    skills: cascade.subagents ? [...skillItems, ...subagentSkills] : skillItems,
    configs: cascade.subagents ? [...configItems, ...subagentConfigs] : configItems,
    workspaces: cascade.subagents ? [...workspaceItems, ...subagentWorkspaces] : workspaceItems,
  };

  return (
    <div className="rounded-lg border border-border bg-panel/35 p-2.5">
      <div className="flex flex-wrap items-start gap-2">
        <div className="min-w-0 flex-1">
          <div className="mb-1 flex items-center gap-1.5 text-xs font-semibold uppercase tracking-normal text-muted">
            <Skull className="size-3" /> Lifecycle intervention
          </div>
          <div className="text-xs text-muted">
            Retire keeps identity, logs, memory, skills and mailbox inspectable. Remove deletes the identity and can clean private/owned resources.
          </div>
        </div>
        <Button variant="ghost" size="sm" onClick={() => setExpanded((v) => !v)}>
          {expanded ? "Close" : "Manage"}
        </Button>
      </div>
      <LifecycleDecisionLedger entries={lifecycleLedger} slug={slug} />
      {lastAction && (
        <div
          className={cn(
            "mt-2 rounded-md border bg-card/60 px-2 py-1.5 text-[11px]",
            lastAction.tone === "good"
              ? "border-good/30 bg-good/5"
              : lastAction.tone === "bad"
                ? "border-bad/35 bg-bad/5"
                : lastAction.tone === "warn"
                  ? "border-warn/35 bg-warn/10"
                  : "border-border",
          )}
        >
          <div
            className={cn(
              "font-medium",
              lastAction.tone === "good"
                ? "text-good"
                : lastAction.tone === "bad"
                  ? "text-bad"
                  : lastAction.tone === "warn"
                    ? "text-warn"
                    : "text-foreground/85",
            )}
          >
            {lastAction.label}
          </div>
          <div className="mt-0.5 text-muted">{lastAction.detail}</div>
        </div>
      )}
      {expanded && (
        <div className="mt-3 space-y-3">
          <div
            className={cn(
              "rounded-lg border bg-card/60 p-2 text-xs",
              interventionSummary.tone === "bad"
                ? "border-bad/40"
                : interventionSummary.tone === "warn"
                  ? "border-warn/40"
                  : interventionSummary.tone === "good"
                    ? "border-good/35"
                    : "border-border",
            )}
          >
            <div
              className={cn(
                "mb-1 font-medium",
                interventionSummary.tone === "bad"
                  ? "text-bad"
                  : interventionSummary.tone === "warn"
                    ? "text-warn"
                    : interventionSummary.tone === "good"
                      ? "text-good"
                      : "text-muted",
              )}
            >
              {interventionSummary.disposition}
            </div>
            <div className="grid gap-1.5 text-muted md:grid-cols-2">
              <div>{interventionSummary.retire}</div>
              <div>{interventionSummary.remove}</div>
            </div>
          </div>
          {hasImpact && (
            <div className="grid gap-2 md:grid-cols-2">
              <ImpactPreview label="Standing orders" items={standing} />
              <ImpactPreview label="Schedules" items={scheduleItems} />
              <ImpactPreview label="Private memory" items={memoryItems} note="Retire keeps these; remove can forget them." />
              <ImpactPreview label="Authored shared memory" items={authoredMemoryItems} note="Shared brain records this agent wrote; remove can forget them only when selected." />
              <ImpactPreview label="Private skills" items={skillItems} note="Retire keeps these; remove can archive them." />
              <ImpactPreview label="Agent config" items={configItems} note="Retire keeps these; remove can delete owned config entries and prune this agent from shared config access lists." />
              <ImpactPreview label="Workspace" items={workspaceItems} note="Retire keeps files; remove can delete the agent's workspace-relative workdir." />
              <ImpactPreview label="Workflow references" items={workflowRefs} note="Retained; workflows are reusable chains, not agent identities." />
              <ImpactPreview label="Mailbox / audit history" items={mailboxItems} note="Remove keeps board messages and audit history inspectable; use board retention to age them out." />
              <ImpactPreview label="Dependent sub-agents" items={subagentItems} note="Remove can retire these dependents so they do not run orphaned." />
              <ImpactPreview label="Sub-agent standing orders" items={subagentStanding} note="Cleaned when standing cleanup and sub-agent cleanup are both selected." />
              <ImpactPreview label="Sub-agent schedules" items={subagentSchedules} note="Cleaned when schedule cleanup and sub-agent cleanup are both selected." />
              <ImpactPreview label="Sub-agent private memory" items={subagentMemories} note="Cleaned when private memory and sub-agent cleanup are both selected." />
              <ImpactPreview label="Sub-agent authored shared memory" items={subagentAuthoredMemories} note="Cleaned when authored shared memory and sub-agent cleanup are both selected." />
              <ImpactPreview label="Sub-agent skills" items={subagentSkills} note="Cleaned when private skills and sub-agent cleanup are both selected." />
              <ImpactPreview label="Sub-agent config" items={subagentConfigs} note="Cleaned when agent config and sub-agent cleanup are both selected." />
              <ImpactPreview label="Sub-agent workspace" items={subagentWorkspaces} note="Cleaned when workspace and sub-agent cleanup are both selected." />
              <ImpactPreview label="Sub-agent workflow references" items={subagentWorkflowRefs} note="Retained with workflow graphs; inspect before removing the identity tree." />
              <ImpactPreview label="Sub-agent mailbox / audit history" items={subagentMailboxMessages} note="Retained with the retired dependent identities." />
            </div>
          )}
          <div className="grid gap-2 lg:grid-cols-[1fr_auto]">
            <label className="flex min-w-0 flex-col gap-1 text-[11px] text-muted">
              Retirement reason
              <textarea
                value={reason}
                onChange={(e) => setReason(e.target.value)}
                rows={2}
                disabled={disabled || profile.retired}
                className="rounded-md border border-border bg-card px-2 py-1 text-sm text-foreground outline-none focus-visible:border-accent disabled:opacity-60"
              />
            </label>
            <div className="flex flex-wrap items-end gap-2">
              {profile.retired ? (
                <Button size="sm" disabled={disabled} onClick={applyRevive}>
                  <ArchiveRestore className="size-3.5" /> Revive
                </Button>
              ) : (
                <Button size="sm" variant="ghost" disabled={disabled} onClick={applyRetire}>
                  <Archive className="size-3.5" /> Retire
                </Button>
              )}
            </div>
          </div>
          {profile.system ? (
            <div className="rounded-lg border border-warn/35 bg-warn/10 p-2 text-xs text-muted">
              <div className="mb-1 font-medium text-warn">System identity protection</div>
              System agents cannot be permanently removed from this page. Retire or pause them to stop execution while keeping their identity, audit log, and diagnostics inspectable.
            </div>
          ) : (
            <div className="rounded-lg border border-bad/30 bg-bad/5 p-2">
              <div className="mb-2 text-xs font-medium text-bad">Remove identity and cleanup</div>
              <div className="mb-2 flex flex-wrap items-center gap-2 rounded-md border border-border bg-card/55 p-2 text-xs text-muted">
                <span className="mr-auto">Cleanup preset</span>
                <Button size="sm" variant="ghost" onClick={() => setCascade(agentDetailRemovalCascadePreset("clean_all"))}>
                  Clean all
                </Button>
                <Button size="sm" variant="ghost" onClick={() => setCascade(agentDetailRemovalCascadePreset("keep_all"))}>
                  Keep all
                </Button>
              </div>
              <div className="grid gap-1.5 sm:grid-cols-2">
                <CleanupToggle label="Standing orders" count={toggleItems.standing.length} checked={cascade.standing} onChange={(v) => setCascade((c) => ({ ...c, standing: v }))} />
                <CleanupToggle label="Schedules" count={toggleItems.schedules.length} checked={cascade.schedules} onChange={(v) => setCascade((c) => ({ ...c, schedules: v }))} />
                <CleanupToggle label="Private memory" count={toggleItems.memory.length} checked={cascade.memory} onChange={(v) => setCascade((c) => ({ ...c, memory: v }))} />
                <CleanupToggle label="Authored shared memory" count={toggleItems.authoredMemory.length} checked={cascade.authored_memory} onChange={(v) => setCascade((c) => ({ ...c, authored_memory: v }))} />
                <CleanupToggle label="Private skills" count={toggleItems.skills.length} checked={cascade.skills} onChange={(v) => setCascade((c) => ({ ...c, skills: v }))} />
                <CleanupToggle label="Agent config" count={toggleItems.configs.length} checked={cascade.config} onChange={(v) => setCascade((c) => ({ ...c, config: v }))} />
                <CleanupToggle label="Workspace" count={toggleItems.workspaces.length} checked={cascade.workspace} onChange={(v) => setCascade((c) => ({ ...c, workspace: v }))} />
                <CleanupToggle label="Dependent sub-agents" count={subagentItems.length} checked={cascade.subagents} onChange={(v) => setCascade((c) => ({ ...c, subagents: v }))} />
              </div>
              <div className="mt-2 rounded-md bg-card/70 px-2 py-1.5 text-[11px] text-muted">
                <div className="mb-1 grid gap-1 sm:grid-cols-2">
                  <PlanStat label="will clean" value={cleanupPlan.length} tone="bad" />
                  <PlanStat label="will keep" value={keepPlan.length} />
                </div>
                <div className={cn("mb-1 font-medium", removeBlockedBySubagents ? "text-bad" : keepPlan.length > 0 ? "text-warn" : "text-foreground/80")}>
                  {removalRisk}
                </div>
                Remove plan: delete identity
                {cleanupPlan.length > 0 ? `; clean ${cleanupPlan.join(", ")}` : "; no dependent cleanup selected"}
                {keepPlan.length > 0 ? `; keep ${keepPlan.join(", ")}` : ""}
                {removeBlockedBySubagents ? (
                  <span className="block pt-1 font-medium text-bad">
                    Dependent sub-agents must be retired with this removal before the identity can be deleted.
                  </span>
                ) : null}
              </div>
              <div className="mt-2 flex justify-end">
                <Button
                  size="sm"
                  variant="danger"
                  disabled={disabled || removeBlockedBySubagents}
                  title={removeBlockedBySubagents ? "Dependent sub-agents must be selected for cleanup first" : "Remove identity"}
                  onClick={applyRemove}
                >
                  <Trash2 className="size-3.5" /> Remove
                </Button>
              </div>
            </div>
          )}
        </div>
      )}
    </div>
  );
}

function LifecycleDecisionLedger({ entries, slug }: { entries: AgentLifecycleLedgerEntry[]; slug: string }) {
  return (
    <div className="mt-2 rounded-md border border-border/70 bg-card/45 p-1.5" aria-label={`${slug} lifecycle ledger`}>
      <div className="mb-1 text-[9px] font-semibold uppercase tracking-normal text-muted/80">Lifecycle ledger</div>
      <div className="grid gap-1 sm:grid-cols-2 xl:grid-cols-4">
        {entries.map((entry) => (
          <div
            key={entry.label}
            title={entry.detail}
            className={cn(
              "min-h-[44px] min-w-0 rounded-md border border-border/50 bg-panel/45 px-2 py-1.5",
              entry.tone === "good" && "border-good/25 bg-good/5",
              entry.tone === "bad" && "border-bad/30 bg-bad/5",
              entry.tone === "warn" && "border-warn/35 bg-warn/10",
            )}
          >
            <div className="truncate text-[9px] font-semibold uppercase tracking-normal text-muted/80">{entry.label}</div>
            <div
              className={cn(
                "mt-0.5 truncate text-[11px] font-medium text-foreground/90",
                entry.tone === "good" && "text-good",
                entry.tone === "bad" && "text-bad",
                entry.tone === "warn" && "text-warn",
                entry.tone === "muted" && "text-muted",
              )}
            >
              {entry.value}
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}

function RuntimeDoctorLedger({ entries, slug }: { entries: AgentRuntimeDoctorLedgerEntry[]; slug: string }) {
  return (
    <div className="mt-2 rounded-md border border-border/70 bg-card/45 p-1.5" aria-label={`${slug} runtime doctor ledger`}>
      <div className="mb-1 text-[9px] font-semibold uppercase tracking-normal text-muted/80">Runtime doctor ledger</div>
      <div className="grid gap-1 sm:grid-cols-2 xl:grid-cols-4">
        {entries.map((entry) => (
          <div
            key={entry.label}
            title={entry.detail}
            className={cn(
              "min-h-[44px] min-w-0 rounded-md border border-border/50 bg-panel/45 px-2 py-1.5",
              entry.tone === "good" && "border-good/25 bg-good/5",
              entry.tone === "bad" && "border-bad/30 bg-bad/5",
              entry.tone === "warn" && "border-warn/35 bg-warn/10",
              entry.tone === "accent" && "border-accent/30 bg-accent/10",
            )}
          >
            <div className="truncate text-[9px] font-semibold uppercase tracking-normal text-muted/80">{entry.label}</div>
            <div
              className={cn(
                "mt-0.5 truncate text-[11px] font-medium text-foreground/90",
                entry.tone === "good" && "text-good",
                entry.tone === "bad" && "text-bad",
                entry.tone === "warn" && "text-warn",
                entry.tone === "accent" && "text-accent",
                entry.tone === "muted" && "text-muted",
              )}
            >
              {entry.value}
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}

function AutonomyRunbook({ entries, slug }: { entries: AgentAutonomyRunbookEntry[]; slug: string }) {
  return (
    <div className="rounded-lg bg-panel/40 p-2.5" aria-label={`${slug} autonomy runbook`}>
      <div className="mb-1.5 flex items-center gap-1.5 text-xs font-semibold uppercase tracking-normal text-muted">
        <Repeat className="size-3" /> Autonomy runbook
      </div>
      <div className="grid gap-1.5 sm:grid-cols-2 xl:grid-cols-3">
        {entries.map((entry) => (
          <div
            key={entry.label}
            title={entry.detail}
            className={cn(
              "min-h-[54px] min-w-0 rounded-md border border-border/60 bg-card/50 px-2 py-1.5",
              entry.tone === "good" && "border-good/25 bg-good/5",
              entry.tone === "bad" && "border-bad/30 bg-bad/5",
              entry.tone === "warn" && "border-warn/35 bg-warn/10",
              entry.tone === "accent" && "border-accent/30 bg-accent/10",
            )}
          >
            <div className="truncate text-[9px] font-semibold uppercase tracking-normal text-muted/80">{entry.label}</div>
            <div
              className={cn(
                "mt-0.5 truncate text-[11px] font-medium text-foreground/90",
                entry.tone === "good" && "text-good",
                entry.tone === "bad" && "text-bad",
                entry.tone === "warn" && "text-warn",
                entry.tone === "accent" && "text-accent",
                entry.tone === "muted" && "text-muted",
              )}
            >
              {entry.value}
            </div>
            <div className="mt-0.5 line-clamp-2 text-xs leading-snug text-muted">{entry.detail}</div>
          </div>
        ))}
      </div>
    </div>
  );
}

function AgentEntityContract({ entries, slug }: { entries: AgentEntityContractEntry[]; slug: string }) {
  return (
    <div className="rounded-lg bg-panel/40 p-2.5" aria-label={`${slug} entity contract`}>
      <div className="mb-1.5 flex items-center gap-1.5 text-xs font-semibold uppercase tracking-normal text-muted">
        <Waypoints className="size-3" /> Agent entity contract
      </div>
      <div className="grid gap-1.5 sm:grid-cols-2 xl:grid-cols-3">
        {entries.map((entry) => (
          <div
            key={entry.label}
            title={entry.detail}
            className={cn(
              "min-h-[58px] min-w-0 rounded-md border border-border/60 bg-card/50 px-2 py-1.5",
              entry.tone === "good" && "border-good/25 bg-good/5",
              entry.tone === "bad" && "border-bad/30 bg-bad/5",
              entry.tone === "warn" && "border-warn/35 bg-warn/10",
              entry.tone === "accent" && "border-accent/30 bg-accent/10",
            )}
          >
            <div className="truncate text-[9px] font-semibold uppercase tracking-normal text-muted/80">{entry.label}</div>
            <div
              className={cn(
                "mt-0.5 truncate text-[11px] font-medium text-foreground/90",
                entry.tone === "good" && "text-good",
                entry.tone === "bad" && "text-bad",
                entry.tone === "warn" && "text-warn",
                entry.tone === "accent" && "text-accent",
                entry.tone === "muted" && "text-muted",
              )}
            >
              {entry.value}
            </div>
            <div className="mt-0.5 line-clamp-2 text-xs leading-snug text-muted">{entry.detail}</div>
          </div>
        ))}
      </div>
    </div>
  );
}

function ImpactPreview({ label, items, note }: { label: string; items: string[]; note?: string }) {
  return (
    <div className="min-w-0 rounded-lg border border-border bg-card/70 p-2 text-xs">
      <div className="flex items-center gap-2">
        <span className="font-medium text-foreground">{label}</span>
        <span className="ml-auto rounded-md bg-panel px-1.5 py-0.5 font-mono text-xs text-muted">{items.length}</span>
      </div>
      {note && <div className="mt-1 text-[11px] text-muted">{note}</div>}
      {items.length > 0 && (
        <ul className="mt-1 max-h-20 space-y-0.5 overflow-auto rounded-md bg-panel/60 px-2 py-1 text-[11px] text-muted">
          {items.slice(0, 8).map((item) => (
            <li key={item} className="truncate" title={item}>
              {item}
            </li>
          ))}
          {items.length > 8 && <li>{items.length - 8} more</li>}
        </ul>
      )}
    </div>
  );
}

function PlanStat({ label, value, tone }: { label: string; value: number; tone?: "bad" }) {
  return (
    <span className={cn("inline-flex items-center justify-between gap-2 rounded border border-border bg-panel px-2 py-1", tone === "bad" && "border-bad/30 bg-bad/10 text-bad")}>
      <span>{label}</span>
      <span className="font-mono text-xs tabular-nums">{value}</span>
    </span>
  );
}

function CleanupToggle({
  label,
  count,
  checked,
  onChange,
}: {
  label: string;
  count: number;
  checked: boolean;
  onChange: (checked: boolean) => void;
}) {
  return (
    <label className="flex items-center gap-2 rounded-md border border-border bg-card/70 px-2 py-1.5 text-xs">
      <input
        type="checkbox"
        checked={checked}
        disabled={count === 0}
        onChange={(e) => onChange(e.target.checked)}
        className="size-3.5"
      />
      <span className="min-w-0 flex-1 truncate">{label}</span>
      <span className="rounded bg-panel px-1.5 py-0.5 font-mono text-xs text-muted">{count}</span>
    </label>
  );
}

function agentLifecycleDetail(profile: AgentProfile): string {
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

function agentNoisePolicyLabel(profile: AgentProfile): string {
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

function AgentTaskList({
  tasks,
  busy,
  onAction,
  onAdd,
}: {
  tasks: NonNullable<AgentProfile["tasklist"]>;
  busy?: string | null;
  onAction?: (id: string, op: "update" | "remove", status?: string) => void;
  onAdd?: (title: string, scope: "cycle" | "total") => void;
}) {
  const cycle = tasks.filter(
    (t) => (t.scope || "total") === "cycle" && t.status !== "retired",
  );
  const total = tasks.filter(
    (t) => (t.scope || "total") === "total" && t.status !== "retired",
  );
  return (
    <div className="space-y-2">
      {onAdd && <TaskComposer busy={busy} onAdd={onAdd} />}
      <div className="grid gap-2 sm:grid-cols-2">
        <TaskGroup title="every-cycle tasks" tasks={cycle} busy={busy} onAction={onAction} />
        <TaskGroup title="total tasklist" tasks={total} busy={busy} onAction={onAction} />
      </div>
    </div>
  );
}

function TaskComposer({
  busy,
  onAdd,
}: {
  busy?: string | null;
  onAdd: (title: string, scope: "cycle" | "total") => void;
}) {
  const [title, setTitle] = useState("");
  const [scope, setScope] = useState<"cycle" | "total">("cycle");
  const adding = busy === `add:${scope}`;
  return (
    <div className="rounded-md border border-border bg-panel/45 p-2">
      <div className="mb-1 text-xs uppercase tracking-normal text-muted">
        add durable task
      </div>
      <div className="flex flex-wrap items-center gap-1.5">
        <input
          value={title}
          onChange={(e) => setTitle(e.target.value)}
          aria-label="New agent task"
          placeholder="Task title..."
          className="min-w-[14rem] flex-1 rounded-md border border-border bg-card px-2 py-1 text-xs text-foreground outline-none placeholder:text-muted/60 focus-visible:border-accent"
        />
        <div className="min-w-[14rem]">
          <DetailOptionPicker
            label="New task scope"
            value={scope}
            onChange={setScope}
            columns="grid-cols-2"
            options={[
              { value: "cycle", label: "Every cycle", detail: "repeat task", icon: <Repeat className="size-3.5" /> },
              { value: "total", label: "Total", detail: "finish once", icon: <CheckCheck className="size-3.5" /> },
            ]}
          />
        </div>
        <Button
          type="button"
          size="sm"
          disabled={adding || !title.trim()}
          onClick={() => {
            const submitted = title.trim();
            onAdd(submitted, scope);
            setTitle("");
          }}
        >
          {adding ? <Wrench className="size-3.5 animate-spin" /> : <Plus className="size-3.5" />} Add task
        </Button>
      </div>
    </div>
  );
}

function OperationalTaskList({
  tasks,
  compact = false,
}: {
  tasks: AgentOperationalTask[];
  compact?: boolean;
}) {
  return (
    <TaskGroup title="operational queue" tasks={tasks} compact={compact} />
  );
}

function TaskGroup({
  title,
  tasks,
  compact = false,
  busy,
  onAction,
}: {
  title: string;
  tasks: Array<{
    id?: string;
    title: string;
    description?: string;
    status?: string;
  }>;
  compact?: boolean;
  busy?: string | null;
  onAction?: (id: string, op: "update" | "remove", status?: string) => void;
}) {
  const summary = taskGroupSummary(tasks);
  return (
    <div>
      <div className="mb-1 flex items-center gap-2">
        <span className="text-xs uppercase tracking-normal text-muted">
          {title}
        </span>
        {summary && (
          <span className="ml-auto truncate text-xs text-muted" title={summary}>
            {summary}
          </span>
        )}
      </div>
      {tasks.length === 0 ? (
        <div className="rounded-md bg-panel p-2.5 text-xs text-muted">
          empty
        </div>
      ) : (
        <ul className="space-y-1 rounded-md bg-panel p-2.5 text-xs text-foreground/85">
          {tasks.map((t, i) => (
            <li
              key={t.id || `${t.title}-${i}`}
              className={cn(
                "flex gap-2",
                !compact && t.description && "items-start",
              )}
            >
              <span className={cn("shrink-0 rounded px-1.5 py-0.5 font-mono text-xs uppercase", taskStatusTone(t.status))}>
                {t.status || "todo"}
              </span>
              <span className="min-w-0">
                <span>{t.title}</span>
                {!compact && t.description && (
                  <span className="mt-0.5 block text-[11px] text-muted">
                    {t.description}
                  </span>
                )}
              </span>
              {!compact && onAction && t.id && (
                <span className="ml-auto flex shrink-0 items-center gap-1">
                  <TaskAction
                    title="Mark doing"
                    busy={busy === `update:${t.id}:doing`}
                    onClick={() => onAction(t.id || "", "update", "doing")}
                  >
                    <Play className="size-3" />
                  </TaskAction>
                  <TaskAction
                    title="Mark done"
                    busy={busy === `update:${t.id}:done`}
                    onClick={() => onAction(t.id || "", "update", "done")}
                  >
                    <CheckCheck className="size-3" />
                  </TaskAction>
                  <TaskAction
                    title="Mark blocked"
                    busy={busy === `update:${t.id}:blocked`}
                    onClick={() => onAction(t.id || "", "update", "blocked")}
                  >
                    <AlertTriangle className="size-3" />
                  </TaskAction>
                  <TaskAction
                    title="Retire task"
                    busy={busy === `update:${t.id}:retired`}
                    onClick={() => onAction(t.id || "", "update", "retired")}
                  >
                    <Archive className="size-3" />
                  </TaskAction>
                  <TaskAction
                    title="Remove task"
                    busy={busy === `remove:${t.id}:`}
                    danger
                    onClick={() => onAction(t.id || "", "remove")}
                  >
                    <Trash2 className="size-3" />
                  </TaskAction>
                </span>
              )}
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

function taskGroupSummary(tasks: Array<{ status?: string }>): string {
  if (tasks.length === 0) return "";
  const counts = tasks.reduce<Record<string, number>>((acc, task) => {
    const status = (task.status || "todo").trim().toLowerCase() || "todo";
    acc[status] = (acc[status] || 0) + 1;
    return acc;
  }, {});
  return ["todo", "doing", "blocked", "done"]
    .map((status) => (counts[status] ? `${counts[status]} ${status}` : ""))
    .filter(Boolean)
    .join(" · ");
}

function taskStatusTone(status?: string): string {
  switch ((status || "todo").toLowerCase()) {
    case "doing":
      return "bg-accent/10 text-accent";
    case "done":
      return "bg-good/10 text-good";
    case "blocked":
      return "bg-bad/10 text-bad";
    default:
      return "bg-card text-muted";
  }
}

function TaskAction({
  title,
  busy,
  danger,
  onClick,
  children,
}: {
  title: string;
  busy?: boolean;
  danger?: boolean;
  onClick: () => void;
  children: ReactNode;
}) {
  return (
    <button
      type="button"
      title={title}
      onClick={onClick}
      disabled={busy}
      className={cn(
        "rounded border border-border bg-card p-1 text-muted transition-colors hover:border-accent hover:text-accent disabled:opacity-50",
        danger && "hover:border-bad hover:text-bad",
      )}
    >
      {busy ? <Wrench className="size-3 animate-spin" /> : children}
    </button>
  );
}

function ToolPolicyBox({
  title,
  items,
  empty,
}: {
  title: string;
  items: string[];
  empty: string;
}) {
  return (
    <div>
      <div className="mb-1 text-xs uppercase tracking-normal text-muted">
        {title}
      </div>
      {items.length === 0 ? (
        <div className="rounded-md bg-panel p-2.5 text-xs text-muted">
          {empty}
        </div>
      ) : (
        <div className="rounded-md bg-panel p-2.5 text-xs text-foreground/85">
          {items.join(", ")}
        </div>
      )}
    </div>
  );
}

function TriggersTab({
  slug,
  profile,
  wakeAccess,
  orders,
  schedules,
  triggers,
  busy,
  onAction,
  onCreateMailboxWake,
  onManage,
}: {
  slug: string;
  profile: AgentProfile;
  wakeAccess?: AgentWakeAccess;
  orders: ApiOrder[];
  schedules: ApiSchedule[];
  triggers: FleetTrigger[];
  busy: boolean;
  onAction: (
    path: string,
    params: Record<string, string>,
    success: string,
    opts?: { confirm?: ConfirmOptions },
  ) => void;
  onCreateMailboxWake: (kind: "dm" | "help" | "broadcast", subject: string) => void;
  onManage: (view: string) => void;
}) {
  const [why, setWhy] = useState<string | null>(null);
  const mailboxArmIssue = mailboxWakeArmIssue(profile, wakeAccess);
  const mailboxArmOwner =
    wakeAccess?.manager ||
    (agentManagedSubagent(profile)
      ? profile.parent_agent || profile.owner_agent || ""
      : "");
  return (
    <div className="space-y-3">
      <div className="rounded-lg border border-border bg-panel/30 p-2.5">
        <div className="mb-1.5 text-xs uppercase tracking-normal text-muted">
          how this agent is triggered
        </div>
        <div className="flex flex-wrap gap-2">
          {triggers.length === 0 ? (
            <span className="text-[11px] text-muted">
              No automatic triggers — runs manually or via delegation.
            </span>
          ) : (
            triggers.map((t, i) => (
              <TriggerChip key={i} mode={t.mode} label={t.label} />
            ))
          )}
        </div>
      </div>

      <MailboxWakeSubjects
        slug={slug}
        orders={orders}
        busy={busy}
        armIssue={mailboxArmIssue}
        armOwner={mailboxArmOwner}
        onCreate={onCreateMailboxWake}
      />

      {/* Upcoming fires — what this agent WILL do next, from each binding schedule. */}
      {schedules.length > 0 && (
        <div>
          <div className="mb-1 flex items-center gap-1.5 text-xs uppercase tracking-normal text-muted">
            <CalendarClock className="size-3" /> upcoming runs
          </div>
          <ul className="space-y-2">
            {schedules.map((s) => (
              <li
                key={s.id}
                className="rounded-lg border border-border bg-panel/30 p-2.5"
              >
                <div className="flex flex-wrap items-center gap-2">
                  <Badge variant={s.enabled ? "good" : "default"}>
                    {s.enabled ? "armed" : "paused"}
                  </Badge>
                  <span className="text-xs font-medium">
                    {s.cadence || s.mode || s.id}
                  </span>
                  <span className="inline-flex min-w-0 items-center gap-1 rounded-md bg-card px-1.5 py-0.5 text-xs text-muted" title={agentScheduleBindingTitle(s, slug)}>
                    <span className="truncate">{agentScheduleBindingTitle(s, slug)}</span>
                  </span>
                  <Button
                    size="sm"
                    variant="ghost"
                    disabled={busy}
                    className="ml-auto"
                    aria-label={`Run schedule ${s.id}`}
                    title="Run now"
                    onClick={() =>
                      onAction("/api/schedule/run", { id: s.id }, `ran ${s.id}`)
                    }
                  >
                    <Flame className="size-3.5" />
                  </Button>
                  <Button
                    size="sm"
                    variant="ghost"
                    disabled={busy}
                    aria-label={`${s.enabled ? "Pause" : "Resume"} schedule ${s.id}`}
                    title={s.enabled ? "Pause" : "Resume"}
                    onClick={() =>
                      onAction(
                        "/api/schedule/enable",
                        { id: s.id, enabled: s.enabled ? "false" : "true" },
                        s.enabled ? "paused" : "resumed",
                      )
                    }
                  >
                    {s.enabled ? (
                      <Pause className="size-3.5" />
                    ) : (
                      <Play className="size-3.5" />
                    )}
                  </Button>
                  <Button
                    size="sm"
                    variant="ghost"
                    disabled={busy}
                    aria-label={`Remove schedule ${s.id}`}
                    title="Remove this schedule binding"
                    onClick={() =>
                      onAction(
                        "/api/schedule/remove",
                        { id: s.id },
                        `removed ${s.id}`,
                        {
                          confirm: {
                            title: "Remove this schedule binding?",
                            message: `Schedule ${s.id} will stop: ${agentScheduleBindingTitle(s, slug)}.`,
                            confirmLabel: "Remove",
                            danger: true,
                          },
                        },
                      )
                    }
                  >
                    <Trash2 className="size-3.5" />
                  </Button>
                </div>
                <ScheduleForecast id={s.id} fallbackNext={s.next_run_unix} />
              </li>
            ))}
          </ul>
        </div>
      )}

      <div className="text-xs uppercase tracking-normal text-muted">
        standing orders firing this agent
      </div>
      {orders.length === 0 ? (
        <EmptyState
          icon={Anchor}
          title="No standing orders bind this agent"
          hint="Create a standing order in the Standing page and set it to run as this agent — cron or event triggered."
        />
      ) : (
        <ul className="space-y-2">
          {orders.map((o) => (
            <li
              key={o.id}
              className="rounded-lg border border-border bg-panel/30 p-2.5"
            >
              <div className="flex flex-wrap items-center gap-2">
                <Badge variant={o.enabled ? "good" : "default"}>
                  {o.enabled ? "armed" : "paused"}
                </Badge>
                <span className="text-xs font-medium">{o.name || o.id}</span>
                {o.initiative?.mode && (
                  <span className="text-xs text-muted">
                    · {o.initiative.mode}
                  </span>
                )}
                <span className="ml-auto flex items-center gap-1">
                  <Button
                    size="sm"
                    variant="ghost"
                    disabled={busy}
                    title="Fire now"
                    onClick={() =>
                      onAction(
                        "/api/standing/fire",
                        { id: o.id },
                        `fired ${o.name || o.id}`,
                      )
                    }
                  >
                    <Flame className="size-3.5" />
                  </Button>
                  <Button
                    size="sm"
                    variant="ghost"
                    disabled={busy}
                    title={o.enabled ? "Pause" : "Resume"}
                    onClick={() =>
                      onAction(
                        "/api/standing/enable",
                        { id: o.id, enabled: o.enabled ? "false" : "true" },
                        o.enabled ? "paused" : "resumed",
                      )
                    }
                  >
                    {o.enabled ? (
                      <Pause className="size-3.5" />
                    ) : (
                      <Play className="size-3.5" />
                    )}
                  </Button>
                  <Button
                    size="sm"
                    variant="ghost"
                    disabled={busy}
                    aria-label={`Remove standing order ${o.id}`}
                    title="Remove this standing order binding"
                    onClick={() =>
                      onAction(
                        "/api/standing/remove",
                        { id: o.id },
                        `removed ${o.name || o.id}`,
                        {
                          confirm: {
                            title: "Remove this standing order binding?",
                            message: `${o.name || o.id} will stop waking ${slug}.`,
                            confirmLabel: "Remove",
                            danger: true,
                          },
                        },
                      )
                    }
                  >
                    <Trash2 className="size-3.5" />
                  </Button>
                </span>
              </div>
              <div className="mt-1.5 flex flex-wrap gap-1.5">
                {(o.triggers || []).map((t, i) => (
                  <span
                    key={i}
                    className="inline-flex items-center gap-1 rounded-md border border-border bg-card px-1.5 py-0.5 text-xs text-foreground/80"
                  >
                    {t.type === "event" ? (
                      <Zap className="size-2.5 text-muted" />
                    ) : (
                      <CalendarClock className="size-2.5 text-muted" />
                    )}
                    <span className="font-mono">
                      {t.type === "event" ? t.subject : t.schedule}
                    </span>
                  </span>
                ))}
              </div>
              {o.plan && (
                <div className="mt-1.5 text-[11px] text-muted">
                  {clip(o.plan, 200)}
                </div>
              )}
              <button
                onClick={() => setWhy(why === o.id ? null : o.id)}
                className="mt-1.5 text-xs text-accent hover:underline"
              >
                {why === o.id ? "hide history" : "firing history"}
              </button>
              {why === o.id && <WhyHistory id={o.id} />}
            </li>
          ))}
        </ul>
      )}
      <Button variant="ghost" size="sm" onClick={() => onManage("standing")}>
        Manage standing orders <ArrowUpRight className="size-3.5" />
      </Button>
    </div>
  );
}

export function agentMailboxSubjects(slug: string): {
  kind: "dm" | "help" | "broadcast";
  label: string;
  subject: string;
}[] {
  const s = slug.trim();
  if (!s) return [];
  return [
    { kind: "dm", label: "DM", subject: `board.dm.${s}` },
    { kind: "help", label: "Help", subject: `board.help.${s}` },
    { kind: "broadcast", label: "Broadcast", subject: "board.broadcast" },
  ];
}

export function mailboxSubjectBinding(
  orders: ApiOrder[],
  subject: string,
): ApiOrder | undefined {
  return orders.find((o) =>
    (o.triggers || []).some((t) => t.type === "event" && t.subject === subject),
  );
}

export function agentMailboxWakeContract(
  slug: string,
  orders: ApiOrder[],
  profile: Pick<AgentProfile, "enabled" | "retired" | "kind" | "managed" | "direct_callable" | "parent_agent" | "owner_agent">,
  access?: Pick<AgentWakeAccess, "channel_allowed" | "manager" | "reason">,
): { value: string; detail: string; tone: "good" | "warn" | "bad" | "muted" } {
  const subjects = agentMailboxSubjects(slug);
  const rows = subjects.map((subject) => ({
    ...subject,
    binding: mailboxSubjectBinding(orders, subject.subject),
  }));
  const armed = rows.filter((row) => row.binding?.enabled).map((row) => row.label);
  const paused = rows.filter((row) => row.binding && !row.binding.enabled).map((row) => row.label);
  const idle = rows.filter((row) => !row.binding).map((row) => row.label);
  const armIssue = mailboxWakeArmIssue(profile, access);
  const value = armIssue
    ? "mailbox blocked"
    : armed.length > 0
      ? `mailbox armed · ${armed.join(", ")}`
      : paused.length > 0
        ? `mailbox paused · ${paused.join(", ")}`
        : "mailbox manual";
  const detail = [
    armed.length > 0 ? `armed ${armed.join(", ")}` : "",
    paused.length > 0 ? `paused ${paused.join(", ")}` : "",
    idle.length > 0 ? `idle ${idle.join(", ")}` : "",
    armIssue ? `blocked: ${armIssue}` : "channel wake allowed",
  ].filter(Boolean).join(" · ");
  return {
    value,
    detail,
    tone: armIssue
      ? profile.retired || profile.enabled === false
        ? "bad"
        : "warn"
      : armed.length > 0
        ? "good"
        : paused.length > 0
          ? "warn"
          : "muted",
  };
}

export function mailboxWakeArmIssue(
  profile: Pick<AgentProfile, "enabled" | "retired" | "kind" | "managed" | "direct_callable" | "parent_agent" | "owner_agent">,
  access?: Pick<AgentWakeAccess, "channel_allowed" | "manager" | "reason">,
): string {
  if (profile.retired) return "revive this agent before arming mailbox wake";
  if (profile.enabled === false) return "resume this agent before arming mailbox wake";
  if (access && access.channel_allowed === false) {
    const owner = access.manager || profile.parent_agent || profile.owner_agent;
    return owner
      ? `channel wake blocked; arm mailbox wake on ${owner}`
      : access.reason || "channel wake is blocked for this agent";
  }
  if (agentManagedSubagent(profile)) {
    const owner = profile.parent_agent || profile.owner_agent;
    return owner
      ? `managed sub-agent; arm mailbox wake on ${owner}`
      : "managed sub-agent; only its parent/owner can wake it";
  }
  return "";
}

export function operatorWakeIssue(
  profile: Pick<AgentProfile, "enabled" | "retired" | "kind" | "managed" | "direct_callable" | "parent_agent" | "owner_agent">,
): string {
  if (profile.retired) return "revive this agent before waking it";
  if (profile.enabled === false) return "resume this agent before waking it";
  if (agentManagedSubagent(profile)) {
    const owner = profile.parent_agent || profile.owner_agent;
    return owner
      ? `managed sub-agent; wake ${owner} instead`
      : "managed sub-agent; only its parent/owner can wake it";
  }
  return "";
}

function MailboxWakeSubjects({
  slug,
  orders,
  busy,
  armIssue,
  armOwner,
  onCreate,
}: {
  slug: string;
  orders: ApiOrder[];
  busy: boolean;
  armIssue: string;
  armOwner: string;
  onCreate: (kind: "dm" | "help" | "broadcast", subject: string) => void;
}) {
  const subjects = agentMailboxSubjects(slug);
  if (subjects.length === 0) return null;
  const icon = { dm: Mail, help: LifeBuoy, broadcast: Megaphone };
  return (
    <div className="rounded-lg border border-border bg-panel/30 p-2.5">
      <div className="mb-1.5 flex items-center gap-1.5 text-xs uppercase tracking-normal text-muted">
        <Mail className="size-3" /> mailbox wake subjects
      </div>
      <div className="grid gap-1.5 md:grid-cols-3">
        {subjects.map((row) => {
          const bound = mailboxSubjectBinding(orders, row.subject);
          const Icon = icon[row.kind];
          return (
            <div
              key={row.subject}
              className="min-w-0 rounded-md border border-border bg-card px-2 py-1.5"
            >
              <div className="flex items-center gap-1.5">
                <Icon className="size-3.5 text-muted" />
                <span className="text-xs font-medium">{row.label}</span>
                <Badge variant={bound?.enabled ? "good" : "default"}>
                  {bound?.enabled ? "armed" : "idle"}
                </Badge>
              </div>
              <div className="mt-1 truncate font-mono text-xs text-muted">
                {row.subject}
              </div>
              {bound?.name && (
                <div className="mt-1 truncate text-xs text-foreground/75">
                  {bound.name}
                </div>
              )}
              {!bound && (
                <div className="mt-1">
                  <Button
                    size="sm"
                    variant="ghost"
                    disabled={busy || !!armIssue}
                    className="h-6 px-1.5 text-xs"
                    aria-label={`Arm ${row.label} mailbox wake for ${slug}`}
                    title={armIssue || `Create a standing order that wakes ${slug} on ${row.subject}`}
                    onClick={() => onCreate(row.kind, row.subject)}
                  >
                    <Zap className="size-3" /> Arm wake
                  </Button>
                  {armIssue && (
                    <div className="mt-1 flex flex-wrap items-center gap-1 text-xs text-muted">
                      <span>{armIssue}</span>
                      {armOwner && (
                        <button
                          type="button"
                          className="font-mono text-accent hover:underline"
                          aria-label={`Open parent agent ${armOwner}`}
                          onClick={() => openAgent(armOwner)}
                        >
                          Open {armOwner}
                        </button>
                      )}
                    </div>
                  )}
                </div>
              )}
            </div>
          );
        })}
      </div>
    </div>
  );
}

interface WhyEvent {
  seq?: number;
  kind?: string;
  ts_unix_ms?: number;
}
function WhyHistory({ id }: { id: string }) {
  const [events, setEvents] = useState<WhyEvent[] | null>(null);
  const [err, setErr] = useState<string | null>(null);
  useEffect(() => {
    let alive = true;
    getJSON<{ events?: WhyEvent[] }>("/api/standing/why", { id })
      .then((d) => alive && setEvents(d.events || []))
      .catch((e) => alive && setErr((e as Error).message));
    return () => {
      alive = false;
    };
  }, [id]);
  if (err) return <ErrorText>{err}</ErrorText>;
  if (!events) return <SkeletonList count={2} lines={1} />;
  if (events.length === 0)
    return <div className="mt-1 text-[11px] text-muted">no history yet</div>;
  return (
    <ul className="mt-1 space-y-1">
      {events.map((e, i) => (
        <li key={e.seq ?? i} className="flex items-center gap-2 text-[11px]">
          <span className="rounded bg-card px-1.5 py-0.5 font-mono text-xs text-accent">
            {(e.kind || "").replace(/^standing\./, "")}
          </span>
          <span className="ml-auto font-mono text-xs text-muted">
            {fmtTime(e.ts_unix_ms)}
          </span>
        </li>
      ))}
    </ul>
  );
}

// ScheduleForecast shows the next few fire times of a schedule (the agent's
// near future), via the read-only /api/schedule/test dry-run.
function ScheduleForecast({
  id,
  fallbackNext,
}: {
  id: string;
  fallbackNext?: number;
}) {
  const [fires, setFires] = useState<number[] | null>(null);
  useEffect(() => {
    let alive = true;
    getJSON<{ forecasts?: { unix?: number }[] }>("/api/schedule/test", {
      id,
      count: "4",
    })
      .then(
        (d) =>
          alive &&
          setFires((d.forecasts || []).map((f) => f.unix || 0).filter(Boolean)),
      )
      .catch(() => alive && setFires([]));
    return () => {
      alive = false;
    };
  }, [id]);
  const list =
    fires && fires.length ? fires : fallbackNext ? [fallbackNext] : [];
  if (fires === null)
    return <div className="mt-1.5 text-xs text-muted">forecasting…</div>;
  if (list.length === 0)
    return (
      <div className="mt-1.5 text-xs text-muted">no upcoming runs</div>
    );
  return (
    <div className="mt-1.5 flex flex-wrap gap-1.5">
      {list.map((u, i) => (
        <span
          key={i}
          className="inline-flex items-center gap-1 rounded-md border border-border bg-card px-1.5 py-0.5 text-xs text-foreground/80"
        >
          <Clock className="size-2.5 text-muted" />
          {fmtDateTime(u * 1000)}
        </span>
      ))}
    </div>
  );
}

// ModelTab answers "which provider/model does this run on, and what happens when
// it fails": the agent's primary model + fallback chain, the global per-task
// chain its task_type resolves to, and the provider/fallback events its runs
// actually produced.
function ModelTab({
  slug,
  profile,
  routing,
  provLog,
  onManage,
  onChanged,
}: {
  slug: string;
  profile: AgentProfile;
  routing: RoutingInfo | null;
  provLog: ProviderRoutingRow[] | null;
  onManage: (view: string) => void;
  onChanged: () => void;
}) {
  const { toast } = useUI();
  const taskChain = profile.task_type
    ? routing?.chains?.[profile.task_type]
    : undefined;
  const fallbacks = provLog
    ? provLog.filter((r) => r.kind === "fallback")
    : null;

  // A named fallback chain (M963): if this agent's model is "@name", expand it to
  // the chain's real models so the page shows what it will actually run, with a
  // health dot per model (M965). Best-effort fetches — absence just hides them.
  const [chains, setChains] = useState<Record<string, string[]>>({});
  const [cat, setCat] = useState<ModelCatalog | null>(null);
  useEffect(() => {
    let live = true;
    getJSON<ChainsState>("/api/chains")
      .then((c) => live && setChains(c.chains || {}))
      .catch(() => {});
    getJSON<ModelCatalog>("/api/catalog")
      .then((c) => live && setCat(c))
      .catch(() => {});
    return () => {
      live = false;
    };
  }, []);

  // Inline edit of the agent's model straight from the detail page (M970): the
  // ModelPicker surfaces the "Fallback chains" group, so you can point an agent
  // at a named chain (@name) without leaving the page. /api/agents/edit is a full
  // replace, so we send the whole profile with only the model (and, for a chain,
  // the now-redundant per-agent fallbacks cleared) changed.
  const [model, setModel] = useState(profile.model || "");
  const [saving, setSaving] = useState(false);
  useEffect(() => setModel(profile.model || ""), [profile.model]);
  const dirty = model !== (profile.model || "");
  async function saveModel() {
    setSaving(true);
    try {
      await postJSON("/api/agents/edit", {
        ref: slug,
        profile: editableAgentProfile(profile, {
          model,
          fallbacks: isChainRef(model) ? [] : profile.fallbacks || [],
        }),
      });
      toast(
        isChainRef(model)
          ? `Model set to chain @${chainName(model)}`
          : model
            ? `Model set to ${model}`
            : "Model reset to daemon default",
        "success",
      );
      onChanged();
    } catch (e) {
      toast((e as Error).message, "error");
    } finally {
      setSaving(false);
    }
  }

  const modelIsChain = isChainRef(model);
  const expandedChain = modelIsChain ? chains[chainName(model)] : undefined;
  const fallbackCount = fallbacks?.length || 0;

  return (
    <div className="space-y-3">
      <div className="space-y-2 rounded-lg border border-accent/30 bg-accent/5 p-2.5">
        <div className="flex items-center gap-1.5 text-xs font-semibold uppercase tracking-normal text-accent">
          <Cpu className="size-3" /> Change model / fallback chain
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <ModelPicker
            value={model}
            activeModel="daemon default"
            onChange={setModel}
          />
          <Button size="sm" onClick={saveModel} disabled={!dirty || saving}>
            {saving ? "Saving…" : "Save"}
          </Button>
          {dirty && <span className="text-xs text-warn">unsaved</span>}
        </div>
        <p className="text-xs text-muted">
          Pick a single model, or a{" "}
          <span className="text-accent">⛓ fallback chain</span> (defined under{" "}
          <button
            className="underline-offset-2 hover:underline"
            onClick={() => onManage("chains")}
          >
            Fallback Chains
          </button>
          ). A chain is self-contained — per-agent fallbacks are ignored when
          one is selected.
        </p>
      </div>
      <div className="space-y-1.5 rounded-lg border border-border bg-panel/30 p-2.5">
        <Row
          label={dirty ? "model (unsaved)" : "primary model"}
          value={
            modelIsChain ? (
              <span className="inline-flex items-center gap-1 font-mono text-accent">
                <Waypoints className="size-3" /> {chainName(model)}
                {expandedChain && (
                  <span className="text-muted">
                    · {expandedChain.length} model
                    {expandedChain.length === 1 ? "" : "s"}
                  </span>
                )}
              </span>
            ) : model ? (
              <span className="font-mono">{model}</span>
            ) : (
              "(daemon default)"
            )
          }
        />
        {modelIsChain && expandedChain && expandedChain.length > 0 && (
          <Row
            label="chain expands to"
            value={
              <span className="flex flex-wrap items-center gap-1">
                {expandedChain.map((m, i) => (
                  <span key={i} className="inline-flex items-center gap-1">
                    {i > 0 && <ArrowRight className="size-3 text-muted" />}
                    <ModelChip id={m} cat={cat} />
                  </span>
                ))}
              </span>
            }
          />
        )}
        {modelIsChain && expandedChain === undefined && (
          <Row
            label="chain"
            value={
              <span className="text-bad">
                @{chainName(model)} — no such chain (falls through to default)
              </span>
            }
          />
        )}
        {!modelIsChain && (
          <Row
            label="fallback chain"
            value={
              (profile.fallbacks || []).length > 0 ? (
                <span className="flex flex-wrap items-center gap-1 font-mono">
                  {(profile.fallbacks || []).map((m, i) => (
                    <span key={i} className="inline-flex items-center gap-1">
                      {i > 0 && <ArrowRight className="size-3 text-muted" />}
                      <span className="rounded bg-card px-1.5 py-0.5 text-xs">
                        {m}
                      </span>
                    </span>
                  ))}
                </span>
              ) : (
                "none — uses the per-task chain only"
              )
            }
          />
        )}
        <Row label="task type" value={profile.task_type || "—"} />
        {taskChain && taskChain.length > 0 && (
          <Row
            label="task chain (global)"
            value={<span className="font-mono">{taskChain.join(" → ")}</span>}
          />
        )}
      </div>

      <div>
        <div className="mb-1 flex items-center gap-1.5 text-xs uppercase tracking-normal text-muted">
          <Cpu className="size-3" /> routing &amp; fallbacks
        </div>
        <div className="mb-2 flex flex-wrap items-center gap-2 text-[11px]">
          <span
            className={cn(
              "rounded px-1.5 py-0.5 font-medium",
              fallbackCount > 0
                ? "bg-bad/15 text-bad"
                : "bg-good/10 text-good",
            )}
          >
            {fallbackCount > 0 ? "fallback pressure" : "stable route"}
          </span>
          <span className="text-muted">
            {fallbackCount > 0
              ? `${fallbackCount} recent fallback hop(s) recorded for this model/task`
              : "no recent fallback hop recorded for this model/task"}
          </span>
        </div>
        {!provLog ? (
          <SkeletonList count={2} lines={1} />
        ) : provLog.length === 0 ? (
          <div className="text-[11px] text-muted">
            no routing or fallback events relevant to this agent's model/task
          </div>
        ) : (
          <ul className="space-y-1">
            {provLog.slice(0, 30).map((r, i) => (
              <li key={i} className="flex items-start gap-2 text-[11px]">
                {(() => {
                  const summary = summarizeProviderRoutingRow(r);
                  return (
                    <>
                      <span
                        className={cn(
                          "rounded px-1.5 py-0.5 font-mono text-xs",
                          summary.kindTone === "bad"
                            ? "bg-bad/15 text-bad"
                            : "bg-card text-foreground/80",
                        )}
                      >
                        {summary.kindLabel}
                      </span>
                      <span
                        className={cn(
                          "rounded px-1.5 py-0.5 font-medium",
                          summary.stateTone === "bad"
                            ? "bg-bad/10 text-bad"
                            : "bg-good/10 text-good",
                        )}
                      >
                        {summary.stateLabel}
                      </span>
                      <span className="min-w-0 flex-1">
                        <span className="flex flex-wrap items-center gap-1">
                          {summary.failedModel ? (
                            <>
                              <ModelChip
                                id={summary.failedModel}
                                chains={chains}
                                cat={cat}
                              />
                              <ArrowRight className="size-3 text-muted" />
                              <ModelChip
                                id={summary.nextModel || "?"}
                                chains={chains}
                                cat={cat}
                              />
                            </>
                          ) : summary.primaryModel ? (
                            <ModelChip
                              id={summary.primaryModel}
                              chains={chains}
                              cat={cat}
                            />
                          ) : (
                            <span
                              className="block truncate text-foreground/85"
                              title={summary.primaryText}
                            >
                              {summary.primaryText}
                            </span>
                          )}
                        </span>
                        {(summary.secondaryText || summary.detail) && (
                          <span
                            className="block truncate text-muted"
                            title={summary.detail || summary.secondaryText}
                          >
                            {[
                              summary.secondaryText,
                              summary.detail ? clip(summary.detail, 80) : "",
                            ]
                              .filter(Boolean)
                              .join(" — ")}
                          </span>
                        )}
                      </span>
                      <span className="ml-auto shrink-0 font-mono text-xs text-muted opacity-70">
                        {fmtTime(r.ts_unix_ms)}
                      </span>
                    </>
                  );
                })()}
              </li>
            ))}
          </ul>
          )}
        {fallbacks && fallbacks.length > 0 && (
          <div className="mt-1.5 text-xs text-bad">
            a model in this route/chain has been failing over recently; check
            the hop reasons above before trusting the primary path
          </div>
        )}
      </div>

      <div className="flex flex-wrap gap-2">
        {modelIsChain && (
          <Button variant="ghost" size="sm" onClick={() => onManage("chains")}>
            Edit fallback chains <ArrowUpRight className="size-3.5" />
          </Button>
        )}
        <Button variant="ghost" size="sm" onClick={() => onManage("routing")}>
          Edit routing chains <ArrowUpRight className="size-3.5" />
        </Button>
      </div>
    </div>
  );
}

function editableAgentProfile(profile: AgentProfile, patch: Partial<AgentProfile> = {}) {
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
    config_overrides: profile.config_overrides || {},
    lifecycle: profile.lifecycle || {},
    tasklist: profile.tasklist || [],
    description: profile.description || "",
    ...patch,
  };
}

// CommsTab is the agent's mailbox: the board messages it sent, was addressed, or
// received as a broadcast/ack — its communication trail with the rest of the
// fleet.
export function agentBoardMessages(
  messages: BoardMessage[],
  slug: string,
): BoardMessage[] {
  const s = slug.trim().toLowerCase();
  return messages
    .filter((m) => {
      const from = (m.from || "").trim().toLowerCase();
      const to = (m.to || "").trim().toLowerCase();
      const acked = (m.acked_by || []).some(
        (a) => a.trim().toLowerCase() === s,
      );
      return from === s || to === s || m.to === "*" || acked;
    })
    .sort((a, b) => (b.ts_unix_ms || 0) - (a.ts_unix_ms || 0));
}

export function messageAckedBy(m: BoardMessage, slug: string): boolean {
  const s = slug.trim().toLowerCase();
  return (m.acked_by || []).some((a) => a.trim().toLowerCase() === s);
}

export function messageAckedByLabel(m: Pick<BoardMessage, "acked_by">): string {
  return (m.acked_by || []).map((a) => a.trim()).filter(Boolean).join(", ");
}

export function waitingForAgent(
  messages: BoardMessage[],
  slug: string,
): BoardMessage[] {
  const s = slug.trim().toLowerCase();
  const answered = new Set(
    messages.filter((m) => m.reply_to).map((m) => m.reply_to as string),
  );
  return messages.filter((m) => {
    if (!m.id || m.reply_to || answered.has(m.id) || messageAckedBy(m, slug))
      return false;
    const from = (m.from || "").trim().toLowerCase();
    const to = (m.to || "").trim().toLowerCase();
    return to === s || (m.to === "*" && from !== s);
  });
}

export interface AgentInboxPrioritySummary {
  direct: number;
  broadcast: number;
  help: number;
  replied: number;
  stale: number;
  waiting: number;
  label: string;
  detail: string;
  tone: "good" | "warn" | "muted";
}

export function agentInboxPrioritySummary(
  messages: BoardMessage[],
  slug: string,
  nowMs = Date.now(),
): AgentInboxPrioritySummary {
  const s = slug.trim().toLowerCase();
  const waiting = waitingForAgent(messages, slug);
  const direct = waiting.filter((m) => (m.to || "").trim().toLowerCase() === s).length;
  const broadcast = waiting.filter((m) => m.to === "*").length;
  const help = waiting.filter((m) => m.help).length;
  const replied = messages.filter((m) => {
    if (!m.reply_to) return false;
    const from = (m.from || "").trim().toLowerCase();
    const to = (m.to || "").trim().toLowerCase();
    return from === s || to === s;
  }).length;
  const staleCutoff = nowMs - 24 * 60 * 60 * 1000;
  const stale = waiting.filter((m) => (m.ts_unix_ms || nowMs) < staleCutoff).length;
  const parts = [
    direct ? `${direct} direct` : "",
    broadcast ? `${broadcast} broadcast` : "",
    help ? `${help} help` : "",
    replied ? `${replied} replied` : "",
    stale ? `${stale} stale` : "",
  ].filter(Boolean);
  return {
    direct,
    broadcast,
    help,
    replied,
    stale,
    waiting: waiting.length,
    label: waiting.length > 0 ? `${waiting.length} waiting` : "inbox clear",
    detail: parts.length > 0 ? parts.join(" · ") : "no waiting direct, broadcast, help, replied, or stale messages",
    tone: stale > 0 || help > 0 ? "warn" : waiting.length > 0 ? "warn" : replied > 0 ? "muted" : "good",
  };
}

export function agentMailboxPassport(
  slug: string,
  messages: BoardMessage[] = [],
  orders: ApiOrder[] = [],
): { value: string; detail: string; tone: "good" | "warn" | "muted" } {
  const waiting = waitingForAgent(messages, slug);
  const involved = agentBoardMessages(messages, slug);
  const s = slug.trim().toLowerCase();
  const sent = involved.filter((m) => (m.from || "").trim().toLowerCase() === s).length;
  const received = involved.filter((m) => {
    const to = (m.to || "").trim().toLowerCase();
    const from = (m.from || "").trim().toLowerCase();
    return to === s || (m.to === "*" && from !== s);
  }).length;
  const subjects = agentMailboxSubjects(slug);
  const armed = subjects.filter((row) => mailboxSubjectBinding(orders, row.subject)).map((row) => row.label);
  return {
    value: waiting.length > 0
      ? `inbox ${waiting.length} waiting`
      : armed.length > 0
        ? `armed ${armed.join(", ")}`
        : involved.length > 0
          ? "inbox clear"
          : "no mailbox traffic",
    detail: [
      `${waiting.length} waiting`,
      `${received} received`,
      `${sent} sent`,
      armed.length > 0 ? `wake subjects armed: ${armed.join(", ")}` : "no mailbox wake subjects armed",
    ].join(" · "),
    tone: waiting.length > 0 ? "warn" : armed.length > 0 ? "good" : "muted",
  };
}

// CommsCausalityLineage renders the delegated/doctor comms causality lineage for
// one escalation message: the wake run this agent's run that the message caused
// (deep-links into the Activity tab) and the incident chain (root → parent →
// current, each deep-linkable to the Incident page). This is the delegated/doctor
// analogue of the mailbox "woke" badge — it proves the message-to-wake causal
// link and lets the operator follow it downstream.
function CommsCausalityLineage({
  slug,
  lineage,
  onFocusRun,
}: {
  slug: string;
  lineage: EscalationCausalityLineage;
  onFocusRun: (correlationId: string | undefined) => void;
}) {
  if (!lineage.origin) return null;
  const tone = lineage.origin === "delegated" ? "text-accent" : "text-muted";
  return (
    <span className="inline-flex flex-wrap items-center gap-1 rounded bg-card px-1.5 py-0.5 text-xs">
      <span
        className={cn("inline-flex items-center gap-1", tone)}
        title={`This escalation message woke ${slug}`}
      >
        <Zap className="size-3" />
        {lineage.label}
      </span>
      {lineage.wakeCorrelationId && (
        <button
          onClick={() => onFocusRun(lineage.wakeCorrelationId)}
          className="font-mono text-accent transition-colors hover:text-accent2"
          title="Open the run this escalation woke in Activity"
        >
          run {clip(lineage.wakeCorrelationId, 24)}
        </button>
      )}
      {lineage.rootIncidentId && (
        <button
          onClick={() => openIncident(lineage.rootIncidentId!)}
          className="font-mono text-muted transition-colors hover:text-accent"
          title={
            lineage.rootAgent
              ? `Open the root incident (root ${lineage.rootAgent})`
              : "Open the root incident"
          }
        >
          root {clip(lineage.rootIncidentId, 18)}
        </button>
      )}
      {lineage.parentIncidentId &&
        lineage.parentIncidentId !== lineage.rootIncidentId &&
        lineage.parentIncidentId !== lineage.incidentId && (
          <button
            onClick={() => openIncident(lineage.parentIncidentId!)}
            className="font-mono text-muted transition-colors hover:text-accent"
            title="Open the parent (delegation hop) incident"
          >
            parent {clip(lineage.parentIncidentId, 18)}
          </button>
        )}
      {lineage.incidentId && lineage.incidentId !== lineage.rootIncidentId && (
        <button
          onClick={() => openIncident(lineage.incidentId!)}
          className="font-mono text-muted transition-colors hover:text-accent"
          title="Open this agent's incident"
        >
          incident {clip(lineage.incidentId, 18)}
        </button>
      )}
      {lineage.nextOwner && (
        <span className="font-mono text-muted">→ {lineage.nextOwner}</span>
      )}
    </span>
  );
}

function CommsTab({
  slug,
  messages,
  escalations,
  wokeMessages,
  onFocusRun,
  onManage,
  onChanged,
}: {
  slug: string;
  messages: BoardMessage[] | null;
  escalations: AgentEscalation[] | null;
  wokeMessages?: Record<string, MailboxWakeRef>;
  onFocusRun: (correlationId: string | undefined) => void;
  onManage: (view: string) => void;
  onChanged: () => void;
}) {
  const ui = useUI();
  const [topic, setTopic] = useState("dm");
  const [text, setText] = useState("");
  const [outTo, setOutTo] = useState("");
  const [outTopic, setOutTopic] = useState("dm");
  const [outText, setOutText] = useState("");
  const [replyTo, setReplyTo] = useState("");
  const [replyText, setReplyText] = useState("");
  const [busy, setBusy] = useState(false);
  const loadedMessages = messages || [];
  const waiting = waitingForAgent(loadedMessages, slug);
  const inboxPriority = agentInboxPrioritySummary(loadedMessages, slug);
  const escalationByMessage = useMemo(
    () =>
      new Map(
        (escalations || [])
          .filter((row) => row.message_id)
          .map((row) => [String(row.message_id), row] as const),
      ),
    [escalations],
  );
  if (!messages) return <SkeletonList count={3} lines={2} />;
  const sent = loadedMessages.filter((m) => m.from === slug).length;
  const received = loadedMessages.filter(
    (m) => m.to === slug || (m.to === "*" && m.from !== slug),
  ).length;
  const seen = loadedMessages.filter((m) => messageAckedBy(m, slug)).length;

  async function send() {
    const body = text.trim();
    if (!body) return;
    setBusy(true);
    try {
      await postJSON("/api/board/send", {
        from: "operator",
        to: slug,
        topic: topic.trim() || "dm",
        text: body,
      });
      setText("");
      ui.toast(`message sent to ${slug}`, "success");
      onChanged();
    } catch (e) {
      ui.toast((e as Error).message, "error");
    } finally {
      setBusy(false);
    }
  }

  async function sendAsAgent() {
    const body = outText.trim();
    const target = outTo.trim();
    if (!body || !target) return;
    setBusy(true);
    try {
      await postJSON("/api/board/send", {
        from: slug,
        to: target,
        topic: outTopic.trim() || "dm",
        text: body,
      });
      setOutText("");
      ui.toast(`message sent from ${slug}`, "success");
      onChanged();
    } catch (e) {
      ui.toast((e as Error).message, "error");
    } finally {
      setBusy(false);
    }
  }

  async function ack(id: string) {
    setBusy(true);
    try {
      await postJSON("/api/board/ack", { id, by: slug });
      ui.toast(`message acknowledged for ${slug}`, "success");
      onChanged();
    } catch (e) {
      ui.toast((e as Error).message, "error");
    } finally {
      setBusy(false);
    }
  }

  async function reply(id: string) {
    const body = replyText.trim();
    if (!body) return;
    setBusy(true);
    try {
      await postJSON("/api/board/send", {
        from: slug,
        reply_to: id,
        text: body,
      });
      setReplyTo("");
      setReplyText("");
      ui.toast(`reply sent as ${slug}`, "success");
      onChanged();
    } catch (e) {
      ui.toast((e as Error).message, "error");
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="space-y-3">
      <div className="grid grid-cols-2 gap-2 sm:grid-cols-4">
        <Stat
          icon={Mail}
          label="waiting"
          value={waiting.length}
          accent={waiting.length > 0}
        />
        <Stat icon={ArrowRight} label="received" value={received} />
        <Stat icon={Send} label="sent" value={sent} />
        <Stat icon={CheckCheck} label="seen" value={seen} accent={seen > 0} />
      </div>

      <div className="rounded-lg border border-border bg-panel/30 p-2.5">
        <div className="mb-2 flex items-center gap-1.5 text-xs font-semibold uppercase tracking-normal text-muted">
          <Send className="size-3" /> message this agent
        </div>
        <div className="flex flex-col gap-2 sm:flex-row">
          <input
            value={topic}
            onChange={(e) => setTopic(e.target.value)}
            aria-label="Mailbox topic"
            placeholder="topic"
            className="h-8 rounded-md border border-border bg-card px-2 font-mono text-xs outline-none focus-visible:border-accent sm:w-32"
          />
          <input
            value={text}
            onChange={(e) => setText(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter" && !e.shiftKey) void send();
            }}
            aria-label="Mailbox message"
            placeholder={`to ${slug}`}
            className="h-8 min-w-0 flex-1 rounded-md border border-border bg-card px-2 text-xs outline-none focus-visible:border-accent"
          />
          <Button size="sm" onClick={send} disabled={busy || !text.trim()}>
            <Send className="size-3.5" /> Send
          </Button>
        </div>
      </div>

      <div className="rounded-lg border border-accent/30 bg-accent/5 p-2.5">
        <div className="mb-2 flex items-center gap-1.5 text-xs font-semibold uppercase tracking-normal text-accent">
          <Share2 className="size-3" /> send as this agent
        </div>
        <div className="grid gap-2 sm:grid-cols-[9rem_9rem_1fr_auto]">
          <input
            value={outTo}
            onChange={(e) => setOutTo(e.target.value)}
            aria-label="Agent outbox recipient"
            placeholder="to agent or *"
            className="h-8 rounded-md border border-border bg-card px-2 font-mono text-xs outline-none focus-visible:border-accent"
          />
          <input
            value={outTopic}
            onChange={(e) => setOutTopic(e.target.value)}
            aria-label="Agent outbox topic"
            placeholder="topic"
            className="h-8 rounded-md border border-border bg-card px-2 font-mono text-xs outline-none focus-visible:border-accent"
          />
          <input
            value={outText}
            onChange={(e) => setOutText(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter" && !e.shiftKey) void sendAsAgent();
            }}
            aria-label="Agent outbox message"
            placeholder={`from ${slug}`}
            className="h-8 min-w-0 rounded-md border border-border bg-card px-2 text-xs outline-none focus-visible:border-accent"
          />
          <Button size="sm" onClick={sendAsAgent} disabled={busy || !outTo.trim() || !outText.trim()}>
            <Send className="size-3.5" /> Send as agent
          </Button>
        </div>
      </div>

      <div className="rounded-lg border border-border bg-panel/30 p-2.5">
        <div className="mb-2 flex items-center gap-1.5 text-xs font-semibold uppercase tracking-normal text-muted">
          <Mail className="size-3" /> inbox priority summary
          <Button className="ml-auto" variant="ghost" size="sm" onClick={() => onManage("board")}>
            Open board <ArrowUpRight className="size-3.5" />
          </Button>
        </div>
        <div className="flex flex-wrap items-center gap-2 text-[11px]">
          <Badge variant={inboxPriority.tone === "good" ? "good" : inboxPriority.tone === "warn" ? "warn" : "default"}>
            {inboxPriority.label}
          </Badge>
          <span className="min-w-0 flex-1 text-muted" title={inboxPriority.detail}>
            {inboxPriority.detail}
          </span>
        </div>
      </div>

      {messages.length === 0 ? (
        <EmptyState
          icon={Mail}
          title="No messages"
          hint={`Board posts ${slug} sent, was addressed (to: ${slug}), or received as a broadcast appear here.`}
        />
      ) : (
        <ul className="space-y-2">
          {messages.slice(0, 60).map((m, i) => {
            const outbound = m.from === slug;
            const waitingHere = waiting.some((w) => w.id === m.id);
            const escalation = m.id ? escalationByMessage.get(m.id) : undefined;
            const woke = mailboxWakeFor(wokeMessages, m.id);
            const seenBy = messageAckedByLabel(m);
            return (
              <li
                key={m.id || i}
                className="rounded-lg border border-border bg-panel/30 p-2.5"
              >
                <div className="flex flex-wrap items-center gap-2 text-[11px]">
                  <Badge
                    variant={
                      outbound ? "good" : waitingHere ? "bad" : "default"
                    }
                  >
                    {outbound
                      ? "sent"
                      : m.to === "*"
                        ? "broadcast"
                        : "received"}
                  </Badge>
                  {m.from && (
                    <span className="font-mono text-xs text-muted">
                      from {m.from}
                    </span>
                  )}
                  {m.to && (
                    <span className="inline-flex items-center gap-1 font-mono text-xs text-muted">
                      {m.to === "*" ? (
                        <Megaphone className="size-3" />
                      ) : (
                        <ArrowRight className="size-3" />
                      )}
                      {m.to}
                    </span>
                  )}
                  {m.topic && (
                    <span className="rounded bg-card px-1.5 py-0.5 font-mono text-xs text-accent">
                      {m.topic}
                    </span>
                  )}
                  {m.reply_to && (
                    <span className="rounded bg-card px-1.5 py-0.5 font-mono text-xs text-muted">
                      reply
                    </span>
                  )}
                  {seenBy && (
                    <span
                      className="inline-flex items-center gap-1 rounded bg-good/10 px-1.5 py-0.5 text-xs text-good"
                      title={`acknowledged by ${seenBy}`}
                    >
                      <CheckCheck className="size-3" /> seen by {seenBy}
                    </span>
                  )}
                  {m.help && (
                    <span className="inline-flex items-center gap-1 rounded bg-bad/15 px-1.5 py-0.5 text-xs text-bad">
                      <LifeBuoy className="size-3" /> help
                    </span>
                  )}
                  {woke && (
                    <span
                      className="inline-flex items-center gap-1 rounded bg-accent/15 px-1.5 py-0.5 text-xs text-accent"
                      title={`This message woke ${slug}${woke.correlation_id ? ` · run ${woke.correlation_id}` : ""}`}
                    >
                      <Zap className="size-3" /> woke {slug}
                    </span>
                  )}
                  {escalation?.origin_kind === "doctor" && (
                    <CommsCausalityLineage
                      slug={slug}
                      lineage={escalationCausalityLineage(escalation)}
                      onFocusRun={onFocusRun}
                    />
                  )}
                  {escalation?.origin_kind === "delegated" && (
                    <CommsCausalityLineage
                      slug={slug}
                      lineage={escalationCausalityLineage(escalation)}
                      onFocusRun={onFocusRun}
                    />
                  )}
                  {waitingHere && (
                    <span className="ml-auto flex items-center gap-1">
                      <Button
                        size="sm"
                        variant="ghost"
                        disabled={busy || !m.id}
                        title={`Reply as ${slug}`}
                        onClick={() => {
                          setReplyTo(replyTo === m.id ? "" : m.id || "");
                          setReplyText("");
                        }}
                      >
                        <CornerDownRight className="size-3.5" /> Reply
                      </Button>
                      <Button
                        size="sm"
                        variant="ghost"
                        disabled={busy || !m.id}
                        title={`Acknowledge for ${slug}`}
                        onClick={() => m.id && ack(m.id)}
                      >
                        <CheckCheck className="size-3.5" /> Ack
                      </Button>
                    </span>
                  )}
                  <span
                    className={cn(
                      "font-mono text-xs text-muted",
                      !waitingHere && "ml-auto",
                    )}
                  >
                    {fmtAgo(m.ts_unix_ms)}
                  </span>
                </div>
                {m.text && (
                  <div className="mt-1 whitespace-pre-wrap text-[11px] text-muted">
                    {clip(m.text, 280)}
                  </div>
                )}
                {m.id && replyTo === m.id && (
                  <div className="mt-2 flex flex-col gap-1.5 rounded-md border border-border bg-card/60 p-2 sm:flex-row">
                    <input
                      value={replyText}
                      onChange={(e) => setReplyText(e.target.value)}
                      onKeyDown={(e) => {
                        if (e.key === "Enter" && !e.shiftKey) void reply(m.id || "");
                      }}
                      aria-label={`Reply to ${m.id}`}
                      placeholder={`reply as ${slug}`}
                      className="h-8 min-w-0 flex-1 rounded-md border border-border bg-panel px-2 text-xs outline-none focus-visible:border-accent"
                    />
                    <Button size="sm" disabled={busy || !replyText.trim()} onClick={() => reply(m.id || "")}>
                      <CornerDownRight className="size-3.5" /> Send reply
                    </Button>
                  </div>
                )}
              </li>
            );
          })}
        </ul>
      )}
      <Button
        variant="ghost"
        size="sm"
        onClick={() => {
          location.hash = `board?agent=${encodeURIComponent(slug)}`;
        }}
      >
        Open Board <ArrowUpRight className="size-3.5" />
      </Button>
    </div>
  );
}

function MemoryTab({
  records,
  scope,
  busy,
  onAction,
  onManage,
}: {
  records: MemoryRecord[] | null;
  scope: string;
  busy: boolean;
  onAction: (
    path: string,
    params: Record<string, string>,
    success: string,
  ) => void;
  onManage: (view: string) => void;
}) {
  if (!records) return <SkeletonList count={4} lines={2} />;
  if (records.length === 0)
    return (
      <EmptyState
        icon={Brain}
        title="No private memory yet"
        hint={`Records this agent writes to its private scope (${scope}) appear here. Shared knowledge lives in the Memory page.`}
      />
    );
  return (
    <div className="space-y-2">
      <div className="text-xs uppercase tracking-normal text-muted">
        private to scope{" "}
        <span className="font-mono text-foreground/80">{scope}</span> ·{" "}
        {records.length} record(s)
      </div>
      <ul className="space-y-2">
        {records.map((r) => (
          <li
            key={r.id}
            className="rounded-lg border border-border bg-panel/30 p-2.5"
          >
            <div className="flex flex-wrap items-center gap-2">
              {r.type && (
                <span className="rounded bg-card px-1.5 py-0.5 font-mono text-xs text-accent">
                  {r.type}
                </span>
              )}
              {r.subject && (
                <span className="text-xs font-medium">{r.subject}</span>
              )}
              {typeof r.confidence === "number" && (
                <span className="text-xs text-muted">
                  conf {r.confidence.toFixed(2)}
                </span>
              )}
              <span className="ml-auto flex items-center gap-2">
                <span className="font-mono text-xs text-muted">
                  {fmtAgo(r.last_seen_ms || r.created_ms)}
                </span>
                <Button
                  size="sm"
                  variant="ghost"
                  disabled={busy}
                  title="Promote to shared memory"
                  onClick={() =>
                    r.id &&
                    onAction(
                      "/api/memory/promote",
                      { id: r.id },
                      "promoted to shared",
                    )
                  }
                >
                  <Share2 className="size-3.5" />
                </Button>
              </span>
            </div>
            {r.content && (
              <div className="mt-1 whitespace-pre-wrap text-[11px] text-muted">
                {clip(r.content, 280)}
              </div>
            )}
          </li>
        ))}
      </ul>
      <Button variant="ghost" size="sm" onClick={() => onManage("memory")}>
        Open Memory <ArrowUpRight className="size-3.5" />
      </Button>
    </div>
  );
}

function SkillsTab({
  skills,
  busy,
  onAction,
  onManage,
}: {
  skills: SkillLite[] | null;
  busy: boolean;
  onAction: (
    path: string,
    params: Record<string, string>,
    success: string,
  ) => void;
  onManage: (view: string) => void;
}) {
  if (!skills) return <SkeletonList count={3} lines={2} />;
  if (skills.length === 0)
    return (
      <EmptyState
        icon={Sparkles}
        title="No private skills"
        hint="Skills authored privately for this agent appear here. Share one to make it available fleet-wide."
      />
    );
  return (
    <div className="space-y-2">
      <ul className="space-y-2">
        {skills.map((s) => (
          <li
            key={s.id}
            className="rounded-lg border border-border bg-panel/30 p-2.5"
          >
            <div className="flex flex-wrap items-center gap-2">
              {s.status && (
                <Badge variant={statusVariant(s.status)}>{s.status}</Badge>
              )}
              <span className="text-xs font-medium">{s.name}</span>
              <Button
                size="sm"
                variant="ghost"
                disabled={busy}
                className="ml-auto"
                title="Share with the whole fleet"
                onClick={() =>
                  s.id &&
                  onAction("/api/skill/share", { id: s.id }, `shared ${s.name}`)
                }
              >
                <Share2 className="size-3.5" /> Share
              </Button>
            </div>
            {s.description && (
              <div className="mt-1 text-[11px] text-muted">
                {clip(s.description, 200)}
              </div>
            )}
            {(s.triggers || []).length > 0 && (
              <div className="mt-1 flex flex-wrap gap-1">
                {(s.triggers || []).map((t, i) => (
                  <span
                    key={i}
                    className="rounded bg-card px-1.5 py-0.5 font-mono text-xs text-muted"
                  >
                    {t}
                  </span>
                ))}
              </div>
            )}
          </li>
        ))}
      </ul>
      <Button variant="ghost" size="sm" onClick={() => onManage("skills")}>
        Open Skills <ArrowUpRight className="size-3.5" />
      </Button>
    </div>
  );
}

function CapabilityControlPanel({
  slug,
  profile,
  toolCatalog,
  edictLevels,
  agentPermissions,
  busy,
  onChanged,
}: {
  slug: string;
  profile: AgentProfile;
  toolCatalog: ToolCatalogRow[] | null;
  edictLevels: Record<string, string>;
  agentPermissions: AgentPermissionsSnapshot | null;
  busy: boolean;
  onChanged: () => void;
}) {
  const ui = useUI();
  const [trust, setTrust] = useState<AgentProfile["trust_ceiling"]>(profile.trust_ceiling || "L4");
  const [allow, setAllow] = useState((profile.tool_allow || []).join(", "));
  const [deny, setDeny] = useState((profile.tool_deny || []).join(", "));
  const [config, setConfig] = useState(configOverridesText(profile.config_overrides));
  const [memoryScope, setMemoryScope] = useState(profile.memory_scope || "");
  const [workdir, setWorkdir] = useState(profile.workdir || "");
  const [maxCost, setMaxCost] = useState(mcToUsdInput(profile.max_cost_mc));
  const [maxDaily, setMaxDaily] = useState(mcToUsdInput(profile.max_daily_mc));
  const [silentOnSuccess, setSilentOnSuccess] = useState(!!profile.noise_policy?.silent_on_success);
  const [disableMemoryWrites, setDisableMemoryWrites] = useState(!!profile.noise_policy?.disable_memory_writes);
  const [notifySeverity, setNotifySeverity] = useState<NonNullable<NonNullable<AgentProfile["noise_policy"]>["min_notify_severity"]>>(profile.noise_policy?.min_notify_severity || "info");
  const [notifyCooldown, setNotifyCooldown] = useState(String(profile.noise_policy?.min_notify_interval_sec || 0));
  const [saving, setSaving] = useState(false);
  const draftAllow = splitCsv(allow);
  const draftDeny = splitCsv(deny);
  const draftProfile = useMemo(
    () => ({
      ...profile,
      trust_ceiling: trust,
      tool_allow: draftAllow,
      tool_deny: draftDeny,
      memory_scope: memoryScope,
      workdir,
    }),
    [draftAllow, draftDeny, memoryScope, profile, trust, workdir],
  );
  const effective = useMemo(
    () =>
      toolCatalog && toolCatalog.length > 0
        ? effectiveToolPermissions(toolCatalog, draftProfile, edictLevels)
        : agentPermissions?.permissions
        ? agentPermissions.permissions.map(permissionRowFromSnapshot)
        : [],
    [agentPermissions, draftProfile, edictLevels, toolCatalog],
  );
  const configEntries = useMemo(
    () => [...(agentPermissions?.config_entries || [])].sort((a, b) => a.key.localeCompare(b.key)),
    [agentPermissions],
  );
  const authoritySnapshot = agentToolAuthoritySnapshot(draftProfile, effective, !!toolCatalog || !!agentPermissions);
  const configSnapshot = agentConfigAccessSummary(agentPermissions);
  const governanceSnapshot = agentGovernancePassport(agentPermissions, authoritySnapshot.detail);
  const authorityContract = agentAuthorityContractSummary(draftProfile, effective, configEntries);
  const authorityManifest = agentAuthorityManifest(draftProfile, effective, agentPermissions, slug);
  const riskPassport = agentCapabilityRiskPassport(draftProfile, effective, agentPermissions);
  const authorityLedger = agentAuthorityLedger(
    {
      ...draftProfile,
      max_cost_mc: usdToMcInput(maxCost) ?? profile.max_cost_mc,
      max_daily_mc: usdToMcInput(maxDaily) ?? profile.max_daily_mc,
      noise_policy: {
        silent_on_success: silentOnSuccess,
        disable_memory_writes: disableMemoryWrites,
        min_notify_severity: notifySeverity,
        min_notify_interval_sec: Math.max(0, Number.parseInt(notifyCooldown || "0", 10) || 0),
      },
    },
    effective,
    agentPermissions,
    slug,
  );
  const draftConfig = parseConfigOverridesText(config);
  const draftConfigCount = typeof draftConfig === "string" ? Object.keys(profile.config_overrides || {}).length : Object.keys(draftConfig).length;
  const highImpactLockdownTools = useMemo(() => {
    const names = effective.length > 0
      ? [...HIGH_IMPACT_LOCKDOWN_TOOLS, ...effective.map((row) => row.name)]
      : HIGH_IMPACT_LOCKDOWN_TOOLS;
    return highImpactToolNames(names, HIGH_IMPACT_LOCKDOWN_TOOLS.length);
  }, [effective]);

  useEffect(() => {
    setTrust(profile.trust_ceiling || "L4");
    setAllow((profile.tool_allow || []).join(", "));
    setDeny((profile.tool_deny || []).join(", "));
    setConfig(configOverridesText(profile.config_overrides));
    setMemoryScope(profile.memory_scope || "");
    setWorkdir(profile.workdir || "");
    setMaxCost(mcToUsdInput(profile.max_cost_mc));
    setMaxDaily(mcToUsdInput(profile.max_daily_mc));
    setSilentOnSuccess(!!profile.noise_policy?.silent_on_success);
    setDisableMemoryWrites(!!profile.noise_policy?.disable_memory_writes);
    setNotifySeverity(profile.noise_policy?.min_notify_severity || "info");
    setNotifyCooldown(String(profile.noise_policy?.min_notify_interval_sec || 0));
  }, [profile]);

  async function save() {
    const policyTools = normalizeNoiseToolPolicy(draftAllow, draftDeny, disableMemoryWrites);
    const overlap = toolPolicyOverlap(policyTools.allow, policyTools.deny);
    if (overlap) {
      ui.toast(`Tool ${overlap} cannot be both allowed and denied`, "error");
      return;
    }
    const parsed = parseConfigOverridesText(config);
    if (typeof parsed === "string") {
      ui.toast(parsed, "error");
      return;
    }
    const maxCostMc = usdToMcInput(maxCost);
    if (maxCostMc === null) {
      ui.toast("Max/run must be a dollar amount like 0.05", "error");
      return;
    }
    const maxDailyMc = usdToMcInput(maxDaily);
    if (maxDailyMc === null) {
      ui.toast("Max/day must be a dollar amount like 0.50", "error");
      return;
    }
    setSaving(true);
    try {
      await postJSON("/api/agents/capabilities", {
        ref: slug,
        trust_ceiling: trust,
        tool_allow: policyTools.allow,
        tool_deny: policyTools.deny,
        memory_scope: memoryScope.trim(),
        workdir: workdir.trim(),
        max_cost_mc: maxCostMc,
        max_daily_mc: maxDailyMc,
        noise_policy: {
          silent_on_success: silentOnSuccess,
          disable_memory_writes: disableMemoryWrites,
          min_notify_severity: notifySeverity,
          min_notify_interval_sec: Math.max(0, Number.parseInt(notifyCooldown || "0", 10) || 0),
        },
        config_overrides: parsed,
      });
      ui.toast(`${slug} capability policy updated`, "success");
      onChanged();
    } catch (e) {
      ui.toast((e as Error).message, "error");
    } finally {
      setSaving(false);
    }
  }

  function allowTool(name: string) {
    setAllow(addCsvItem(allow, name));
    setDeny(removeCsvItem(deny, name));
  }

  function denyTool(name: string) {
    setDeny(addCsvItem(deny, name));
    setAllow(removeCsvItem(allow, name));
  }

  function clearTool(name: string) {
    setAllow(removeCsvItem(allow, name));
    setDeny(removeCsvItem(deny, name));
  }

  function applyQuietSystemPreset() {
    setTrust((current) => (current === "L4" ? "L2" : current));
    setMemoryScope((current) => current.trim() || `system/${slug}`);
    setWorkdir((current) => current.trim() || `system/${slug}`);
    setMaxCost("0.05");
    setMaxDaily("0.05");
    setSilentOnSuccess(true);
    setDisableMemoryWrites(true);
    setNotifySeverity("warning");
    setNotifyCooldown("28800");
  }

  function applyRestrictedWorkerPreset() {
    setTrust("L2");
    setAllow("memory, notify");
    setDeny("");
    setWorkdir((current) => current.trim() || `agents/${slug}`);
    setMaxCost("0.25");
    setMaxDaily("1");
    setSilentOnSuccess(true);
    setDisableMemoryWrites(true);
    setNotifySeverity("warning");
    setNotifyCooldown("3600");
  }

  function applyHighImpactLockdownPreset() {
    setTrust((current) => (current === "L4" ? "L2" : current));
    setAllow(removeCsvItems(allow, highImpactLockdownTools));
    setDeny(addCsvItems(deny, highImpactLockdownTools));
    setSilentOnSuccess(true);
    setNotifySeverity("warning");
    setNotifyCooldown((current) => {
      const seconds = Number.parseInt(current || "0", 10) || 0;
      return String(Math.max(seconds, 3600));
    });
  }

  function applyOpenLabPreset() {
    setTrust("L4");
    setAllow("");
    setDeny("");
    setMaxCost("");
    setMaxDaily("");
    setSilentOnSuccess(false);
    setDisableMemoryWrites(false);
    setNotifySeverity("info");
    setNotifyCooldown("0");
  }

  async function updateConfigAccess(row: AgentConfigPermissionRow, mode: "allow" | "exclude" | "clear") {
    const allowed = row.allowed_agents || [];
    const excluded = row.excluded_agents || [];
    const nextAllowed =
      mode === "allow"
        ? addListItem(allowed, slug)
        : mode === "clear"
          ? removeListItem(allowed, slug)
          : removeListItem(allowed, slug);
    const nextExcluded =
      mode === "exclude"
        ? addListItem(excluded, slug)
        : mode === "clear"
          ? removeListItem(excluded, slug)
          : removeListItem(excluded, slug);
    setSaving(true);
    try {
      await postJSON("/api/configcenter/access", {
        key: row.key,
        allowed_agents: nextAllowed,
        excluded_agents: nextExcluded,
      });
      ui.toast(`${row.key} access updated for ${slug}`, "success");
      onChanged();
    } catch (e) {
      ui.toast((e as Error).message, "error");
    } finally {
      setSaving(false);
    }
  }

  return (
    <div className="rounded-lg border border-accent/30 bg-accent/5 p-2.5">
      <div className="mb-2 flex flex-wrap items-center gap-2">
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-1.5 text-xs font-semibold uppercase tracking-normal text-accent">
            <ShieldCheck className="size-3" /> Capability control
          </div>
          <div className="mt-1 text-[11px] text-muted">
            Tool access, trust ceiling, spend caps, memory scope, and runtime config for this identity.
          </div>
        </div>
        <Button size="sm" disabled={busy || saving} onClick={save}>
          <Wrench className="size-3.5" /> Save
        </Button>
      </div>
      <div
        className={cn(
          "mb-2 rounded-md border px-2 py-1.5 text-[11px]",
          authorityContract.tone === "warn"
            ? "border-warn/40 bg-warn/10"
            : authorityContract.tone === "good"
              ? "border-good/30 bg-good/5"
              : authorityContract.tone === "bad"
                ? "border-bad/35 bg-bad/5"
                : "border-border bg-card/55",
        )}
      >
        <div
          className={cn(
            "font-medium",
            authorityContract.tone === "warn" && "text-warn",
            authorityContract.tone === "good" && "text-good",
            authorityContract.tone === "bad" && "text-bad",
          )}
        >
          {authorityContract.label}
        </div>
        <div className="mt-0.5 text-muted" title={authorityContract.detail}>
          {authorityContract.detail}
        </div>
      </div>
      <div
        className={cn(
          "mb-2 rounded-md border p-2 text-[11px]",
          authorityManifest.tone === "warn"
            ? "border-warn/40 bg-warn/10"
            : authorityManifest.tone === "good"
              ? "border-good/30 bg-good/5"
              : authorityManifest.tone === "bad"
                ? "border-bad/35 bg-bad/5"
                : "border-border bg-card/55",
        )}
      >
        <div className="mb-1 flex items-center gap-1.5 text-xs font-semibold uppercase tracking-normal text-muted">
          <IdCard className="size-3" /> Authority manifest
        </div>
        <div
          className={cn(
            "font-medium",
            authorityManifest.tone === "warn" && "text-warn",
            authorityManifest.tone === "good" && "text-good",
            authorityManifest.tone === "bad" && "text-bad",
          )}
        >
          {authorityManifest.label}
        </div>
        <div className="mt-0.5 text-muted" title={authorityManifest.detail}>
          {authorityManifest.detail}
        </div>
        <div className="mt-2 grid gap-1.5 md:grid-cols-4">
          {Object.entries(authorityManifest.fields).map(([key, value]) => (
            <div key={key} className="min-w-0 rounded border border-border bg-panel/40 px-2 py-1">
              <div className="truncate text-[9px] font-semibold uppercase tracking-normal text-muted" title={key}>
                {key}
              </div>
              <div className="mt-0.5 truncate text-xs text-foreground/80" title={value}>
                {value}
              </div>
            </div>
          ))}
        </div>
      </div>
      <div className="mb-2 grid gap-2 md:grid-cols-4">
        <div
          className={cn(
            "rounded-md border px-2 py-1.5",
            riskPassport.tone === "warn"
              ? "border-warn/40 bg-warn/10"
              : riskPassport.tone === "good"
                ? "border-good/30 bg-good/5"
                : riskPassport.tone === "bad"
                  ? "border-bad/35 bg-bad/5"
                  : "border-border bg-card/55",
          )}
        >
          <div className="text-[9px] font-semibold uppercase tracking-normal text-muted">Risk passport</div>
          <div
            className={cn(
              "mt-0.5 truncate text-[11px] font-medium",
              riskPassport.tone === "warn" && "text-warn",
              riskPassport.tone === "good" && "text-good",
              riskPassport.tone === "bad" && "text-bad",
            )}
            title={riskPassport.detail}
          >
            {riskPassport.label}
          </div>
          <div className="mt-0.5 truncate text-xs text-muted" title={riskPassport.detail}>
            {riskPassport.detail}
          </div>
        </div>
        <div
          className={cn(
            "rounded-md border px-2 py-1.5",
            authoritySnapshot.tone === "warn"
              ? "border-warn/40 bg-warn/10"
              : authoritySnapshot.tone === "good"
                ? "border-good/30 bg-good/5"
                : authoritySnapshot.tone === "bad"
                  ? "border-bad/35 bg-bad/5"
                  : "border-border bg-card/55",
          )}
        >
          <div className="text-[9px] font-semibold uppercase tracking-normal text-muted">Authority snapshot</div>
          <div
            className={cn(
              "mt-0.5 truncate text-[11px] font-medium",
              authoritySnapshot.tone === "warn" && "text-warn",
              authoritySnapshot.tone === "good" && "text-good",
              authoritySnapshot.tone === "bad" && "text-bad",
            )}
            title={authoritySnapshot.detail}
          >
            {authoritySnapshot.headline}
          </div>
          <div className="mt-0.5 truncate text-xs text-muted" title={authoritySnapshot.detail}>
            {authoritySnapshot.detail}
          </div>
        </div>
        <div className="rounded-md border border-border bg-card/55 px-2 py-1.5">
          <div className="text-[9px] font-semibold uppercase tracking-normal text-muted">Config center</div>
          <div className="mt-0.5 truncate text-[11px] font-medium text-foreground/85" title={configSnapshot}>
            {configSnapshot}
          </div>
          <div className="mt-0.5 truncate text-xs text-muted">
            {draftConfigCount} runtime override{draftConfigCount === 1 ? "" : "s"}
          </div>
        </div>
        <div
          className={cn(
            "rounded-md border px-2 py-1.5",
            governanceSnapshot.tone === "warn"
              ? "border-warn/40 bg-warn/10"
              : governanceSnapshot.tone === "good"
                ? "border-good/30 bg-good/5"
                : governanceSnapshot.tone === "bad"
                  ? "border-bad/35 bg-bad/5"
                  : "border-border bg-card/55",
          )}
        >
          <div className="text-[9px] font-semibold uppercase tracking-normal text-muted">Governance</div>
          <div className="mt-0.5 truncate text-[11px] font-medium text-foreground/85" title={governanceSnapshot.detail}>
            {governanceSnapshot.detail}
          </div>
          <div className="mt-0.5 truncate text-xs text-muted">
            trust {draftProfile.trust_ceiling || "L4"}
          </div>
        </div>
      </div>
      <div className="mb-2 rounded-md border border-border bg-card/55 p-2">
        <div className="mb-1 flex items-center gap-1.5 text-xs font-semibold uppercase tracking-normal text-muted">
          <IdCard className="size-3" /> Authority ledger
        </div>
        <div className="grid gap-1.5 md:grid-cols-4">
          {authorityLedger.map((entry) => (
            <div
              key={entry.label}
              className={cn(
                "min-w-0 rounded border px-2 py-1.5",
                entry.tone === "warn"
                  ? "border-warn/35 bg-warn/10"
                  : entry.tone === "good"
                    ? "border-good/30 bg-good/5"
                    : entry.tone === "bad"
                      ? "border-bad/35 bg-bad/5"
                      : "border-border bg-panel/40",
              )}
            >
              <div className="truncate text-[9px] font-semibold uppercase tracking-normal text-muted" title={entry.label}>
                {entry.label}
              </div>
              <div
                className={cn(
                  "mt-0.5 truncate text-[11px] font-medium",
                  entry.tone === "warn" && "text-warn",
                  entry.tone === "good" && "text-good",
                  entry.tone === "bad" && "text-bad",
                )}
                title={entry.detail}
              >
                {entry.value}
              </div>
              <div className="mt-0.5 truncate text-xs text-muted" title={entry.detail}>
                {entry.detail}
              </div>
            </div>
          ))}
        </div>
      </div>
      <div className="grid gap-2 md:grid-cols-3">
        <div className="md:col-span-1">
          <DetailOptionPicker
            label="Trust ceiling"
            value={trust || "L4"}
            onChange={(level) => setTrust(level as AgentProfile["trust_ceiling"])}
            columns="grid-cols-2"
            options={[
              { value: "L4", label: "L4", detail: "allow", icon: <ShieldCheck className="size-3.5" /> },
              { value: "L3", label: "L3", detail: "ask scoped", icon: <ShieldCheck className="size-3.5" /> },
              { value: "L2", label: "L2", detail: "ask first", icon: <AlertTriangle className="size-3.5" /> },
              { value: "L1", label: "L1", detail: "ask always", icon: <AlertCircle className="size-3.5" /> },
              { value: "L0", label: "L0", detail: "deny", icon: <XCircle className="size-3.5" /> },
            ]}
          />
        </div>
        <label className="flex flex-col gap-1 text-[11px] text-muted">
          Tool allow
          <input
            value={allow}
            onChange={(e) => setAllow(e.target.value)}
            placeholder="shell, memory, mcp_fake_greet"
            className="rounded-md border border-border bg-card px-2 py-1 text-sm text-foreground outline-none focus-visible:border-accent"
          />
        </label>
        <label className="flex flex-col gap-1 text-[11px] text-muted">
          Tool deny
          <input
            value={deny}
            onChange={(e) => setDeny(e.target.value)}
            placeholder="notify, shell"
            className="rounded-md border border-border bg-card px-2 py-1 text-sm text-foreground outline-none focus-visible:border-accent"
          />
        </label>
      </div>
      <div className="mt-2 flex flex-wrap items-center gap-1.5 rounded-md border border-border bg-card/55 px-2 py-1.5">
        <span className="mr-1 text-xs font-semibold uppercase tracking-normal text-muted">Policy presets</span>
        <Button type="button" variant="ghost" size="sm" onClick={applyQuietSystemPreset} title="Apply quiet guardian defaults">
          <Megaphone className="size-3.5" /> Quiet system preset
        </Button>
        <Button type="button" variant="ghost" size="sm" onClick={applyRestrictedWorkerPreset} title="Allow only memory/notify with quiet output">
          <ShieldCheck className="size-3.5" /> Restricted worker preset
        </Button>
        <Button type="button" variant="ghost" size="sm" onClick={applyHighImpactLockdownPreset} title={`Deny high-impact tools: ${highImpactLockdownTools.join(", ")}`}>
          <ShieldCheck className="size-3.5" /> High-impact lockdown
        </Button>
        <Button type="button" variant="ghost" size="sm" onClick={applyOpenLabPreset} title="Clear agent-specific tool restrictions">
          <Wrench className="size-3.5" /> Open lab preset
        </Button>
      </div>
      <div className="mt-2 grid gap-2 md:grid-cols-[1fr_1fr_12rem_12rem]">
        <button
          type="button"
          onClick={() => setSilentOnSuccess(!silentOnSuccess)}
          className={cn(
            "flex items-center gap-2 rounded-lg border px-3 py-2 transition-all",
            silentOnSuccess
              ? "border-good/50 bg-good/15 shadow-sm shadow-good/20"
              : "border-border/50 bg-panel/30 hover:border-border"
          )}
        >
          <div className={cn(
            "flex h-8 w-8 items-center justify-center rounded-lg",
            silentOnSuccess ? "bg-good/25" : "bg-panel/60"
          )}>
            <ShieldCheck className={cn("size-4", silentOnSuccess ? "text-good" : "text-muted/50")} />
          </div>
          <div className="flex flex-col text-left">
            <span className={cn("text-xs font-medium", silentOnSuccess ? "text-foreground" : "text-muted")}>
              Silent on success
            </span>
            <span className="text-xs text-muted">suppress OK logs</span>
          </div>
          <div className={cn(
            "ml-auto flex h-5 w-10 items-center rounded-full border px-0.5 transition-colors",
            silentOnSuccess ? "border-good/50 bg-good/30" : "border-border bg-panel"
          )}>
            <div className={cn(
              "h-3.5 w-3.5 rounded-full transition-transform",
              silentOnSuccess ? "translate-x-5 bg-good shadow-sm shadow-good/50" : "translate-x-0 bg-muted/40"
            )} />
          </div>
        </button>
        <button
          type="button"
          onClick={() => setDisableMemoryWrites(!disableMemoryWrites)}
          className={cn(
            "flex items-center gap-2 rounded-lg border px-3 py-2 transition-all",
            disableMemoryWrites
              ? "border-warn/50 bg-warn/15 shadow-sm shadow-warn/20"
              : "border-border/50 bg-panel/30 hover:border-border"
          )}
        >
          <div className={cn(
            "flex h-8 w-8 items-center justify-center rounded-lg",
            disableMemoryWrites ? "bg-warn/25" : "bg-panel/60"
          )}>
            <HardDrive className={cn("size-4", disableMemoryWrites ? "text-warn" : "text-muted/50")} />
          </div>
          <div className="flex flex-col text-left">
            <span className={cn("text-xs font-medium", disableMemoryWrites ? "text-foreground" : "text-muted")}>
              Memory writes
            </span>
            <span className="text-xs text-muted">
              {disableMemoryWrites ? "blocked" : "allowed"}
            </span>
          </div>
          <div className={cn(
            "ml-auto flex h-5 w-10 items-center rounded-full border px-0.5 transition-colors",
            disableMemoryWrites ? "border-warn/50 bg-warn/30" : "border-border bg-panel"
          )}>
            <div className={cn(
              "h-3.5 w-3.5 rounded-full transition-transform",
              disableMemoryWrites ? "translate-x-5 bg-warn shadow-sm shadow-warn/50" : "translate-x-0 bg-muted/40"
            )} />
          </div>
        </button>
        <DetailOptionPicker
          label="Notify level"
          value={notifySeverity}
          onChange={setNotifySeverity}
          columns="grid-cols-1"
          options={[
            { value: "info", label: "Info", detail: "all notifications", icon: <Megaphone className="size-3.5" /> },
            { value: "warning", label: "Warning", detail: "warn and critical", icon: <AlertTriangle className="size-3.5" /> },
            { value: "critical", label: "Critical", detail: "critical only", icon: <Flame className="size-3.5" /> },
          ]}
        />
        <label className="flex flex-col gap-1 text-[11px] text-muted">
          <span className="flex items-center gap-1"><Clock className="size-3" /> Cooldown (sec)</span>
          <input
            type="number"
            min={0}
            value={notifyCooldown}
            onChange={(e) => setNotifyCooldown(e.target.value)}
            className="rounded-md border border-border bg-card px-2 py-1.5 font-mono text-sm text-foreground outline-none focus-visible:border-accent"
          />
        </label>
      </div>
      <div className="mt-2 grid gap-2 md:grid-cols-4">
        <label className="flex flex-col gap-1 text-[11px] text-muted">
          Memory scope
          <input
            value={memoryScope}
            onChange={(e) => setMemoryScope(e.target.value)}
            placeholder={`agent/${slug}`}
            className="rounded-md border border-border bg-card px-2 py-1 font-mono text-sm text-foreground outline-none focus-visible:border-accent"
          />
        </label>
        <label className="flex flex-col gap-1 text-[11px] text-muted">
          Workspace subdir
          <input
            value={workdir}
            onChange={(e) => setWorkdir(e.target.value)}
            placeholder={`agents/${slug}`}
            className="rounded-md border border-border bg-card px-2 py-1 font-mono text-sm text-foreground outline-none focus-visible:border-accent"
          />
        </label>
        <label className="flex flex-col gap-1 text-[11px] text-muted">
          Max/run ($)
          <input
            type="text"
            inputMode="decimal"
            value={maxCost}
            onChange={(e) => setMaxCost(e.target.value)}
            placeholder="0.05"
            className="rounded-md border border-border bg-card px-2 py-1 text-sm text-foreground outline-none focus-visible:border-accent"
          />
        </label>
        <label className="flex flex-col gap-1 text-[11px] text-muted">
          Max/day ($)
          <input
            type="text"
            inputMode="decimal"
            value={maxDaily}
            onChange={(e) => setMaxDaily(e.target.value)}
            placeholder="0.50"
            className="rounded-md border border-border bg-card px-2 py-1 text-sm text-foreground outline-none focus-visible:border-accent"
          />
        </label>
      </div>
      <label className="mt-2 flex flex-col gap-1 text-[11px] text-muted">
        Config overrides
        <textarea
          value={config}
          onChange={(e) => setConfig(e.target.value)}
          rows={3}
          placeholder="AGEZT_PROVIDER=openai"
          className="rounded-md border border-border bg-card px-2 py-1 font-mono text-xs text-foreground outline-none focus-visible:border-accent"
        />
      </label>
      <div className="mt-3">
        <div className="mb-1 flex items-center gap-2 text-xs font-semibold uppercase tracking-normal text-muted">
          Config center access
          <span className="rounded bg-card px-1.5 py-0.5 font-mono text-[9px]">
            {configEntries.filter((row) => row.visible).length}/{configEntries.length}
          </span>
        </div>
        {!agentPermissions ? (
          <SkeletonList count={2} lines={1} />
        ) : configEntries.length === 0 ? (
          <div className="text-[11px] text-muted">no config center entries are visible to this daemon snapshot</div>
        ) : (
          <div className="max-h-44 overflow-auto rounded-lg border border-border bg-card/55">
            <table className="w-full text-left text-[11px]">
              <thead className="sticky top-0 bg-card text-xs uppercase tracking-normal text-muted">
                <tr>
                  <th className="px-2 py-1.5 font-medium">Key</th>
                  <th className="px-2 py-1.5 font-medium">Rating</th>
                  <th className="px-2 py-1.5 font-medium">Access</th>
                  <th className="px-2 py-1.5 font-medium">Scope</th>
                  <th className="px-2 py-1.5 font-medium">Policy</th>
                </tr>
              </thead>
              <tbody>
                {configEntries.map((row) => (
                  <tr key={row.key} className="border-t border-border/70">
                    <td className="max-w-52 truncate px-2 py-1.5 font-mono text-foreground/85" title={row.description || row.key}>
                      {row.key}
                    </td>
                    <td className="px-2 py-1.5 text-muted">{row.rating || "-"}</td>
                    <td className="px-2 py-1.5">
                      <Badge variant={row.visible ? "good" : "bad"}>
                        {row.visible ? "visible" : "blocked"}
                      </Badge>
                    </td>
                    <td className="max-w-56 truncate px-2 py-1.5 text-muted" title={configAccessDetail(row)}>
                      {configAccessLabel(row)}
                    </td>
                    <td className="px-2 py-1.5">
                      <div className="flex items-center gap-1">
                        <Button
                          type="button"
                          variant="ghost"
                          size="icon"
                          className="h-6 w-6 text-good"
                          title={`Allow ${slug} for ${row.key}`}
                          aria-label={`Allow ${slug} for ${row.key}`}
                          disabled={saving}
                          onClick={() => updateConfigAccess(row, "allow")}
                        >
                          <CheckCheck className="size-3.5" />
                        </Button>
                        <Button
                          type="button"
                          variant="ghost"
                          size="icon"
                          className="h-6 w-6 text-bad"
                          title={`Exclude ${slug} from ${row.key}`}
                          aria-label={`Exclude ${slug} from ${row.key}`}
                          disabled={saving}
                          onClick={() => updateConfigAccess(row, "exclude")}
                        >
                          <X className="size-3.5" />
                        </Button>
                        <Button
                          type="button"
                          variant="ghost"
                          size="icon"
                          className="h-6 w-6 text-muted"
                          title={`Clear ${slug} config policy for ${row.key}`}
                          aria-label={`Clear ${slug} config policy for ${row.key}`}
                          disabled={saving}
                          onClick={() => updateConfigAccess(row, "clear")}
                        >
                          <Trash2 className="size-3.5" />
                        </Button>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
      <div className="mt-3">
        <div className="mb-1 flex items-center gap-2 text-xs font-semibold uppercase tracking-normal text-muted">
          Effective tool access
          <span className="rounded bg-card px-1.5 py-0.5 font-mono text-[9px]">
            {effective.filter((row) => row.allowed).length}/{effective.length}
          </span>
        </div>
        <WorkflowToolAccessCard summary={workflowToolAccessSummary(effective)} />
        {!toolCatalog && !agentPermissions ? (
          <SkeletonList count={2} lines={1} />
        ) : effective.length === 0 ? (
          <div className="text-[11px] text-muted">no registered tools are visible to this daemon</div>
        ) : (
          <div className="max-h-56 overflow-auto rounded-lg border border-border bg-card/55">
            <table className="w-full text-left text-[11px]">
              <thead className="sticky top-0 bg-card text-xs uppercase tracking-normal text-muted">
                <tr>
                  <th className="px-2 py-1.5 font-medium">Tool</th>
                  <th className="px-2 py-1.5 font-medium">Capability</th>
                  <th className="px-2 py-1.5 font-medium">Effective</th>
                  <th className="px-2 py-1.5 font-medium">Source</th>
                  <th className="px-2 py-1.5 font-medium">Policy</th>
                </tr>
              </thead>
              <tbody>
                {effective.map((row) => (
                  <tr key={row.name} className="border-t border-border/70">
                    <td className="max-w-40 truncate px-2 py-1.5 font-mono text-foreground/85" title={row.description || row.name}>
                      {row.name}
                    </td>
                    <td className="px-2 py-1.5 font-mono text-muted">{row.capability || "-"}</td>
                    <td className="px-2 py-1.5">
                      <Badge variant={row.allowed ? "good" : row.ask ? "warn" : "bad"}>
                        {row.label}
                      </Badge>
                    </td>
                    <td className="px-2 py-1.5 text-muted">{row.reason}</td>
                    <td className="px-2 py-1.5">
                      <div className="flex items-center gap-1">
                        <Button
                          type="button"
                          variant="ghost"
                          size="icon"
                          className="h-6 w-6 text-good"
                          title={`Allow ${row.name}`}
                          aria-label={`Allow ${row.name}`}
                          onClick={() => allowTool(row.name)}
                        >
                          <CheckCheck className="size-3.5" />
                        </Button>
                        <Button
                          type="button"
                          variant="ghost"
                          size="icon"
                          className="h-6 w-6 text-bad"
                          title={`Deny ${row.name}`}
                          aria-label={`Deny ${row.name}`}
                          onClick={() => denyTool(row.name)}
                        >
                          <X className="size-3.5" />
                        </Button>
                        <Button
                          type="button"
                          variant="ghost"
                          size="icon"
                          className="h-6 w-6 text-muted"
                          title={`Clear ${row.name} policy`}
                          aria-label={`Clear ${row.name} policy`}
                          onClick={() => clearTool(row.name)}
                        >
                          <Trash2 className="size-3.5" />
                        </Button>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </div>
  );
}

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

function WorkflowToolAccessCard({ summary }: { summary: WorkflowToolAccessSummary }) {
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

interface WakeAccessSummary {
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

function summarizeWakeAccess(profile: AgentProfile, access?: AgentWakeAccess): WakeAccessSummary {
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

function agentManagedSubagent(profile: Pick<AgentProfile, "kind" | "managed" | "direct_callable">): boolean {
  return profile.kind === "subagent" || !!profile.managed || profile.direct_callable === false;
}

function summarizePermissionPassport(
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

function agentToolAuthoritySnapshot(
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

function agentConfigAccessSummary(snapshot: AgentPermissionsSnapshot | null): string {
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

function agentGovernancePassport(
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

function effectiveToolPermissions(
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

function permissionRowFromSnapshot(row: AgentPermissionRow): EffectiveToolPermission {
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

function configAccessLabel(row: AgentConfigPermissionRow): string {
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

function configAccessDetail(row: AgentConfigPermissionRow): string {
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

function splitCsv(s: string): string[] {
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

const HIGH_IMPACT_LOCKDOWN_TOOLS = [
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

function mcToUsdInput(mc?: number): string {
  return mc && mc > 0 ? String(mc / 1e9) : "";
}

function usdToMcInput(value: string): number | null {
  const raw = value.trim().replace(/^\$/, "");
  if (!raw) return 0;
  const parsed = Number.parseFloat(raw);
  if (!Number.isFinite(parsed) || parsed < 0) return null;
  return Math.round(parsed * 1e9);
}

function toolPolicyOverlap(allow: string[], deny: string[]): string {
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

function addCsvItem(text: string, item: string): string {
  const clean = item.trim();
  if (!clean) return text;
  const lower = clean.toLowerCase();
  const items = splitCsv(text).filter((x) => x.toLowerCase() !== lower);
  items.push(clean);
  return csvText(items);
}

function removeCsvItem(text: string, item: string): string {
  const lower = item.trim().toLowerCase();
  if (!lower) return text;
  return csvText(splitCsv(text).filter((x) => x.toLowerCase() !== lower));
}

function addCsvItems(text: string, items: string[]): string {
  return items.reduce((acc, item) => addCsvItem(acc, item), text);
}

function removeCsvItems(text: string, items: string[]): string {
  return items.reduce((acc, item) => removeCsvItem(acc, item), text);
}

function addListItem(items: string[], item: string): string[] {
  const clean = item.trim();
  if (!clean) return items;
  const lower = clean.toLowerCase();
  return [...items.filter((x) => x.trim().toLowerCase() !== lower), clean];
}

function removeListItem(items: string[], item: string): string[] {
  const lower = item.trim().toLowerCase();
  if (!lower) return items;
  return items.filter((x) => x.trim().toLowerCase() !== lower);
}

function configOverridesText(config?: Record<string, string>): string {
  return Object.entries(config || {})
    .map(([k, v]) => `${k}=${v}`)
    .join("\n");
}

function parseConfigOverridesText(text: string): Record<string, string> | string {
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

function DiagTab({
  slug,
  profile,
  posture,
  askPolicy,
  edictLevels,
  toolCatalog,
  agentPermissions,
  wakePolicy,
  denials,
  approvals,
  toolErrors,
  fail,
  health,
  overrides,
  repair,
  repairStatus,
  busy,
  onChanged,
}: {
  slug: string;
  profile: AgentProfile;
  posture: PolicyStats | null;
  askPolicy: string | null;
  edictLevels: Record<string, string>;
  toolCatalog: ToolCatalogRow[] | null;
  agentPermissions: AgentPermissionsSnapshot | null;
  wakePolicy: WakeAccessSummary;
  denials: PolicyDecision[] | null;
  approvals: ApprovalDecision[] | null;
  toolErrors: ToolInvocation[] | null;
  fail?: RunLite;
  health: AgentHealthSnapshot;
  overrides: AgentConfigOverrideSummary;
  repair: AgentRepairSnapshot;
  repairStatus: AgentRepairStatus | null;
  busy: boolean;
  onChanged: () => void;
}) {
  const ui = useUI();
  const [repairing, setRepairing] = useState(false);
  const repairCommand = agentRepairCommandSummary(profile, repairStatus);
  const repairOperations = agentRepairOperationsSummary(profile, repairStatus);
  const repairDecision = agentRepairDecisionSummary(repairStatus);
  const repairBlocked = profile.retired
    ? "revive this agent before requesting repair"
    : agentManagedSubagent(profile)
      ? "managed sub-agent; request repair through its parent/owner"
      : "";
  async function requestRepair() {
    if (repairBlocked) return;
    setRepairing(true);
    try {
      const res = await postJSON<{ correlation_id?: string }>("/api/agents/repair", {
        ref: slug,
        reason: `operator requested repair from ${slug} identity page`,
      });
      ui.toast(res.correlation_id ? `Repair accepted (${res.correlation_id})` : "Repair accepted", "success");
      onChanged();
    } catch (e) {
      ui.toast((e as Error).message, "error");
    } finally {
      setRepairing(false);
    }
  }

  return (
    <div className="space-y-3">
      <CapabilityControlPanel
        slug={slug}
        profile={profile}
        toolCatalog={toolCatalog}
        edictLevels={edictLevels}
        agentPermissions={agentPermissions}
        busy={busy}
        onChanged={onChanged}
      />

      {/* Calm lead: one health line, always visible. The full wake-policy,
          health, repair, posture and override walls fold underneath — one click
          away, never flooding the tab by default. */}
      <div className="flex flex-wrap items-center gap-2 rounded-lg border border-border bg-panel/30 p-2.5 text-[11px]">
        <Badge variant={health.state === "healthy" ? "good" : health.state === "retired" ? "default" : "bad"}>
          {health.label}
        </Badge>
        <span className="min-w-0 flex-1 truncate text-muted" title={health.detail}>
          {health.detail}
        </span>
      </div>

      <Disclosure
        summary={<span className="text-xs uppercase tracking-normal text-muted">Health, policy &amp; repair detail</span>}
      >
      <div className="space-y-3 pt-1">

      <div className="rounded-lg border border-border bg-panel/40 p-2.5 text-[11px]">
        <div className="mb-2 flex flex-wrap items-center gap-2">
          <Badge variant={wakePolicy.tone === "good" ? "good" : wakePolicy.tone === "bad" ? "bad" : "warn"}>
            {wakePolicy.status}
          </Badge>
          <span className="text-muted">{wakePolicy.detail}</span>
        </div>
        <div className="grid gap-1.5 sm:grid-cols-4">
          <MiniPolicy label="operator" allowed={wakePolicy.operatorAllowed} />
          <MiniPolicy label="schedule" allowed={wakePolicy.scheduleAllowed} />
          <MiniPolicy label="channel" allowed={wakePolicy.channelAllowed} />
          <MiniPolicy label="delegation" allowed={wakePolicy.delegationAllowed} note={wakePolicy.delegationDetail} />
        </div>
      </div>

      <div
        className={cn(
          "rounded-lg border p-2.5 text-[11px]",
          health.state === "healthy"
            ? "border-good/30 bg-good/5"
            : health.state === "retired"
              ? "border-border bg-panel/40"
              : "border-bad/40 bg-bad/5",
        )}
      >
        <div className="flex flex-wrap items-center gap-2">
          <Badge
            variant={
              health.state === "healthy"
                ? "good"
                : health.state === "retired"
                  ? "default"
                  : "bad"
            }
          >
            {health.label}
          </Badge>
          {health.doctorAgent && (
            <span className="text-muted">
              doctor{" "}
              <span className="font-mono text-foreground/80">
                {health.doctorAgent}
              </span>
            </span>
          )}
          {health.selfRepairEnabled && (
            <span className="text-muted">self-repair on</span>
          )}
          {health.escalateTo && (
            <span className="text-muted">
              escalate{" "}
              <span className="font-mono text-foreground/80">
                {health.escalateTo}
              </span>
            </span>
          )}
          {health.lastFailureMs && (
            <span className="ml-auto font-mono text-xs text-muted">
              last fail {fmtAgo(health.lastFailureMs)}
            </span>
          )}
          {!health.lastFailureMs && health.lastActiveMs && (
            <span className="ml-auto font-mono text-xs text-muted">
              last active {fmtAgo(health.lastActiveMs)}
            </span>
          )}
        </div>
        <div className="mt-1 text-muted">{health.detail}</div>
        {(health.configIssues || []).length > 0 && (
          <ul className="mt-1 space-y-1 text-[11px] text-bad">
            {(health.configIssues || []).slice(0, 4).map((issue) => (
              <li key={issue}>{issue}</li>
            ))}
          </ul>
        )}
      </div>

      <div
        className={cn(
          "rounded-lg border p-2.5 text-[11px]",
          repair.state === "completed"
            ? "border-good/30 bg-good/5"
            : repair.state === "failed"
              ? "border-bad/40 bg-bad/5"
              : repair.state === "queued"
                ? "border-accent/40 bg-accent/5"
                : "border-border bg-panel/40",
        )}
      >
        <div className="flex flex-wrap items-center gap-2">
          <Badge
            variant={
              repair.state === "completed"
                ? "good"
                : repair.state === "failed"
                  ? "bad"
                  : repair.state === "queued"
                    ? "accent"
                    : "default"
            }
          >
            {repair.label}
          </Badge>
          <Button
            size="sm"
            variant="ghost"
            disabled={busy || repairing || !!repairBlocked}
            onClick={requestRepair}
            title={repairBlocked || "Request a governed doctor/repair run for this agent"}
          >
            <Wrench className="size-3.5" /> Repair now
          </Button>
          {repairStatus?.cooldown_sec ? (
            <span className="text-muted">
              cooldown {repairStatus.cooldown_sec}s
            </span>
          ) : null}
          {repairStatus?.inflight_count ? (
            <span className="text-muted">
              {repairStatus.inflight_count} inflight
            </span>
          ) : null}
          {repair.nextEligibleMs && (
            <span className="ml-auto font-mono text-xs text-muted">
              next eligible {fmtDateTime(repair.nextEligibleMs)}
            </span>
          )}
        </div>
        <div
          className={cn(
            "mt-2 rounded-md border px-2 py-1.5",
            repairOperations.tone === "good"
              ? "border-good/30 bg-good/5"
              : repairOperations.tone === "warn"
                ? "border-warn/35 bg-warn/10"
                : repairOperations.tone === "bad"
                  ? "border-bad/35 bg-bad/5"
                  : "border-border bg-card/55",
          )}
        >
          <div
            className={cn(
              "text-xs font-semibold uppercase tracking-normal",
              repairOperations.tone === "good"
                ? "text-good"
                : repairOperations.tone === "warn"
                  ? "text-warn"
                  : repairOperations.tone === "bad"
                    ? "text-bad"
                    : "text-muted",
            )}
          >
            Repair operations
          </div>
          <div className="mt-0.5 text-[11px] text-muted" title={repairOperations.detail}>
            <span className="font-medium text-foreground/85">{repairOperations.label}</span>
            {repairOperations.detail ? ` · ${repairOperations.detail}` : ""}
          </div>
        </div>
        <div className="mt-2 grid gap-1.5 md:grid-cols-4">
          <RepairCommandCell label="next action" value={`${repairDecision.label} · ${repairDecision.detail}`} tone={repairDecision.tone} />
          <RepairCommandCell label="contract" value={repairCommand.contract} />
          <RepairCommandCell label="latest" value={repairCommand.latest} tone={repair.state === "failed" ? "bad" : repair.state === "completed" ? "good" : "muted"} />
          <RepairCommandCell label="cooldown" value={repairCommand.cooldown} tone={repairCommand.cooldown === "eligible now" ? "good" : "warn"} />
        </div>
        <div className="mt-1 text-muted">{repair.detail}</div>
        {(repairStatus?.history || []).length > 0 && (
          <ul className="mt-2 space-y-1.5">
            {(repairStatus?.history || []).slice(0, 5).map((row, i) => (
              <li
                key={`${row.seq || row.ts_unix_ms || i}-${row.phase || "event"}`}
                className="rounded-md border border-border bg-card/40 p-2"
              >
                <div className="flex flex-wrap items-center gap-2">
                  <Badge
                    variant={
                      row.phase === "completed"
                        ? "good"
                        : row.phase === "failed"
                          ? "bad"
                          : row.phase === "queued"
                            ? "accent"
                            : "default"
                    }
                  >
                    {row.phase || "event"}
                  </Badge>
                  {row.mode ? (
                    <Badge variant="default">
                      {row.mode === "degraded" ? "doctor" : "config"}
                    </Badge>
                  ) : null}
                  {row.correlation_id ? (
                    <span className="font-mono text-xs text-muted">
                      {clip(row.correlation_id, 24)}
                    </span>
                  ) : null}
                  <span className="ml-auto font-mono text-xs text-muted">
                    {fmtTime(row.ts_unix_ms)}
                  </span>
                </div>
                {row.reason && (
                  <div className="mt-1 text-muted">{row.reason}</div>
                )}
                {incidentLineageLabel(row) && (
                  <button
                    onClick={() =>
                      row.root_incident_id
                        ? openIncident(row.root_incident_id)
                        : row.incident_id
                          ? openIncident(row.incident_id)
                          : undefined
                    }
                    className="mt-1 text-xs uppercase tracking-normal text-muted transition-colors hover:text-accent"
                    title="Open this repair incident"
                  >
                    {incidentLineageLabel(row)}
                  </button>
                )}
                {row.error && <div className="mt-1 text-bad">{row.error}</div>}
                {(row.applied || []).length > 0 && (
                  <div className="mt-1 text-muted">
                    applied {(row.applied || []).join(", ")}
                  </div>
                )}
              </li>
            ))}
          </ul>
        )}
      </div>

      {/* posture */}
      <div className="grid grid-cols-2 gap-2 sm:grid-cols-4">
        <Stat icon={ShieldCheck} label="ask policy" value={askPolicy || "—"} />
        <Stat
          icon={ShieldCheck}
          label="allowed"
          value={
            posture?.allow_rate != null
              ? `${Math.round(posture.allow_rate * 100)}%`
              : (posture?.allowed ?? "—")
          }
          accent
        />
        <Stat
          icon={AlertTriangle}
          label="denial rate"
          value={
            posture?.denial_rate != null
              ? `${Math.round(posture.denial_rate * 100)}%`
              : "—"
          }
        />
        <Stat
          icon={X}
          label="hard-denied"
          value={posture?.hard_denied ?? "—"}
        />
      </div>
      <p className="text-xs text-muted">
        Capabilities default to <span className="text-good">allow</span> — only
        the denials below were ever blocked. This agent can use any tool that
        isn't explicitly restricted.
      </p>

      {(overrides.runtime.length > 0 || overrides.generic.length > 0) && (
        <div className="rounded-lg border border-border bg-panel/30 p-2.5">
          <div className="mb-1 text-xs uppercase tracking-normal text-muted">
            runtime policy overlay
          </div>
          {overrides.runtime.length === 0 ? (
            <div className="text-[11px] text-muted">
              no known runtime knob is overridden; only generic agent config
              keys are set
            </div>
          ) : (
            <ul className="space-y-1.5">
              {overrides.runtime.map((row) => (
                <li
                  key={row.key}
                  className="rounded-md border border-border bg-card/40 p-2 text-[11px]"
                >
                  <div className="flex flex-wrap items-center gap-2">
                    <span className="font-mono text-foreground/85">
                      {row.key}
                    </span>
                    <Badge variant={row.valid ? "accent" : "bad"}>
                      {row.valid ? row.label : "invalid"}
                    </Badge>
                    <span className="font-mono text-muted">{row.value}</span>
                  </div>
                  <div
                    className={cn("mt-1 text-muted", !row.valid && "text-bad")}
                  >
                    {row.valid ? row.effect : row.issue}
                  </div>
                </li>
              ))}
            </ul>
          )}
          {overrides.generic.length > 0 && (
            <div className="mt-2 text-[11px] text-muted">
              {overrides.generic.length} generic agent config key(s) are also
              present and may affect config-aware tools/plugins.
            </div>
          )}
        </div>
      )}

      {fail && (
        <div className="rounded-lg border border-bad/40 bg-bad/5 p-2.5 text-[11px]">
          <span className="font-medium text-bad">Last failed run:</span>{" "}
          <span className="font-mono text-muted">
            {clip(fail.correlation_id || "run", 60)}
          </span>{" "}
          · {fail.started_unix_ms ? fmtTime(fail.started_unix_ms) : "—"}
        </div>
      )}

      </div>
      </Disclosure>

      {/* denied capabilities — the healthy "nothing denied" state stays plain;
          a non-empty list folds behind its count so it never floods the page. */}
      <div>
        {!denials ? (
          <>
            <div className="mb-1 text-xs uppercase tracking-normal text-muted">capability denials</div>
            <SkeletonList count={2} lines={1} />
          </>
        ) : denials.length === 0 ? (
          <div className="text-[11px] text-muted">
            no capability was denied to this agent
          </div>
        ) : (
          <Disclosure
            summary={
              <span className="text-xs uppercase tracking-normal text-muted">
                {denials.length} capability denial{denials.length === 1 ? "" : "s"}
              </span>
            }
          >
          <ul className="space-y-1">
            {denials.slice(0, 40).map((d, i) => (
              <li key={i} className="flex items-start gap-2 text-[11px]">
                <span
                  className={cn(
                    "rounded px-1.5 py-0.5 font-mono text-xs",
                    d.hard_denied
                      ? "bg-bad/15 text-bad"
                      : "bg-card text-foreground/80",
                  )}
                >
                  {d.capability || "?"}
                </span>
                <span
                  className="min-w-0 flex-1 truncate text-muted"
                  title={d.reason}
                >
                  {d.tool ? `${d.tool} — ` : ""}
                  {d.reason || (d.hard_denied ? "hard-denied" : "denied")}
                </span>
                <span className="ml-auto shrink-0 font-mono text-xs text-muted opacity-70">
                  {fmtTime(d.ts_unix_ms)}
                </span>
              </li>
            ))}
          </ul>
          </Disclosure>
        )}
      </div>

      {/* approvals — human gates are the positive/negative counterpart to policy denials. */}
      <div>
        {!approvals ? (
          <>
            <div className="mb-1 text-xs uppercase tracking-normal text-muted">human approvals</div>
            <SkeletonList count={2} lines={1} />
          </>
        ) : approvals.length === 0 ? (
          <div className="text-[11px] text-muted">
            no human approval request is attributed to this agent
          </div>
        ) : (
          <Disclosure
            summary={
              <span className="text-xs uppercase tracking-normal text-muted">
                {approvals.length} human approval{approvals.length === 1 ? "" : "s"}
              </span>
            }
          >
          <ul className="space-y-1">
            {approvals.slice(0, 40).map((a, i) => (
              <li key={a.approval_id || i} className="flex items-start gap-2 text-[11px]">
                <span
                  className={cn(
                    "rounded px-1.5 py-0.5 font-mono text-xs",
                    a.status === "granted"
                      ? "bg-good/15 text-good"
                      : a.status === "denied" || a.status === "timeout"
                        ? "bg-bad/15 text-bad"
                        : "bg-warn/15 text-warn",
                  )}
                >
                  {a.status || "pending"}
                </span>
                <span className="min-w-0 flex-1 truncate text-muted" title={a.reason}>
                  {a.capability || a.tool || "capability"}
                  {a.tool ? ` · ${a.tool}` : ""}
                  {a.reason ? ` — ${a.reason}` : ""}
                  {a.resolved_by ? ` (${a.resolved_by})` : ""}
                </span>
                <span className="ml-auto shrink-0 font-mono text-xs text-muted opacity-70">
                  {fmtTime(a.ts_unix_ms)}
                </span>
              </li>
            ))}
          </ul>
          </Disclosure>
        )}
      </div>

      {/* tool errors */}
      <div>
        {!toolErrors ? (
          <>
            <div className="mb-1 text-xs uppercase tracking-normal text-muted">tool errors</div>
            <SkeletonList count={2} lines={1} />
          </>
        ) : toolErrors.length === 0 ? (
          <div className="text-[11px] text-muted">
            no tool errors recorded for this agent
          </div>
        ) : (
          <Disclosure
            summary={
              <span className="text-xs uppercase tracking-normal text-muted">
                {toolErrors.length} tool error{toolErrors.length === 1 ? "" : "s"}
              </span>
            }
          >
          <ul className="space-y-1">
            {toolErrors.slice(0, 40).map((t, i) => (
              <li key={i} className="flex items-start gap-2 text-[11px]">
                <span className="rounded bg-bad/15 px-1.5 py-0.5 font-mono text-xs text-bad">
                  {t.tool || "?"}
                </span>
                <span
                  className="min-w-0 flex-1 truncate text-muted"
                  title={t.output}
                >
                  {clip(t.output || "error", 120)}
                </span>
                <span className="ml-auto shrink-0 font-mono text-xs text-muted opacity-70">
                  {fmtTime(t.ts_unix_ms)}
                </span>
              </li>
            ))}
          </ul>
          </Disclosure>
        )}
      </div>
    </div>
  );
}

interface SkillFile {
  path?: string;
  size?: number;
}
function FilesTab({
  workdir,
  skills,
}: {
  workdir?: string;
  skills: SkillLite[] | null;
}) {
  const [openId, setOpenId] = useState<string | null>(null);
  return (
    <div className="space-y-3">
      <Row
        label="workdir"
        value={
          workdir ? (
            <span className="font-mono">{workdir}</span>
          ) : (
            "(workspace root — no dedicated subdir)"
          )
        }
      />
      <div>
        <div className="mb-1 text-xs uppercase tracking-normal text-muted">
          skill bundle files
        </div>
        {!skills ? (
          <SkeletonList count={2} lines={1} />
        ) : skills.length === 0 ? (
          <div className="text-[11px] text-muted">
            this agent owns no private skill bundles
          </div>
        ) : (
          <ul className="space-y-1.5">
            {skills.map((s) => (
              <li
                key={s.id}
                className="rounded-md border border-border bg-panel/30 p-2"
              >
                <button
                  onClick={() =>
                    setOpenId(openId === s.id ? null : (s.id ?? null))
                  }
                  className="flex w-full items-center gap-2 text-[11px]"
                >
                  <FolderOpen className="size-3.5 text-muted" />
                  <span className="font-medium">{s.name}</span>
                  <ChevronRight
                    className={cn(
                      "ml-auto size-3.5 text-muted transition-transform",
                      openId === s.id && "rotate-90",
                    )}
                  />
                </button>
                {openId === s.id && s.id && <SkillFiles id={s.id} />}
              </li>
            ))}
          </ul>
        )}
      </div>
    </div>
  );
}

function SkillFiles({ id }: { id: string }) {
  const [files, setFiles] = useState<SkillFile[] | null>(null);
  const [err, setErr] = useState<string | null>(null);
  useEffect(() => {
    let alive = true;
    getJSON<{ files?: SkillFile[] }>("/api/skill/files", { id })
      .then((d) => alive && setFiles(d.files || []))
      .catch((e) => alive && setErr((e as Error).message));
    return () => {
      alive = false;
    };
  }, [id]);
  if (err) return <ErrorText>{err}</ErrorText>;
  if (!files) return <SkeletonList count={2} lines={1} />;
  if (files.length === 0)
    return (
      <div className="mt-1 pl-5 text-[11px] text-muted">no bundled files</div>
    );
  return (
    <ul className="mt-1 space-y-0.5 pl-5">
      {files.map((f, i) => (
        <li
          key={i}
          className="flex items-center gap-2 font-mono text-xs text-muted"
        >
          <span className="truncate">{f.path}</span>
          {typeof f.size === "number" && (
            <span className="ml-auto shrink-0">{f.size}B</span>
          )}
        </li>
      ))}
    </ul>
  );
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
    case "triggers":
      return data.orders.length + data.schedules.length;
    case "comms":
      return data.comms?.length;
    case "memory":
      return data.memory?.length;
    case "skills":
      return data.skills?.length;
    case "diag":
      return (data.denials?.length || 0) + (data.toolErrors?.length || 0);
    default:
      return undefined;
  }
}
