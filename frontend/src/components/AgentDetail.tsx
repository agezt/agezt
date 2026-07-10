import { useEffect, useMemo, useState } from "react";
import { X, Activity as ActivityIcon, ArrowUpRight, Play, Pause, CalendarClock, Zap, Cpu, Wrench, Skull, Archive, ArchiveRestore, IdCard } from "lucide-react";
import { getJSON, postAction, postJSON } from "@/lib/api";
import { cn, fmtAgo, fmtDue } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { Disclosure } from "@/components/ui/disclosure";
import { useUI, type ConfirmOptions } from "@/components/ui/feedback";
import { useEvents } from "@/lib/events";
import { AgentAvatar } from "@/components/AgentAvatar";
import { AgentActivity } from "@/components/AgentActivity";
import { AgentRepair, repairReadinessPassport } from "@/components/AgentRepair";
import { ModelPicker } from "@/components/ModelPicker";
import { openAgent } from "@/lib/agentnav";
import { agentCommandStrip, agentControlCenterLedger, agentEnableToast, agentHierarchySummary, agentIdentityCardSummary, agentLifecycleDispositionPassport, agentLifecycleSummary, agentLivePresencePassport, agentModelPassportSummary, agentModelRoutePassport, agentNoiseBudgetPassport, agentRetireToast, agentReviveToast, agentSchedulePressurePassport, agentTaskContractSummary, guardianQuietPolicyPayload, type AgentEnableResult, type AgentProfile, type AgentRetireResult, type AgentReviveResult } from "@/views/Roster";
import { scheduleAgentSlug, type FleetTrigger, type FleetState, type ApiOrder, type ApiSchedule } from "@/lib/fleet";
import { agentScope, agentCorrelations, filterByCorrelation, filterAgentMemory, filterAgentSkills, summarizeAgent, lastFailure, healthSnapshot, summarizeConfigOverrides, summarizeAutoRepair, summarizeAgentRuntimeStatus, summarizeEscalations, escalationOperationalTasks, summarizeAgentPolicyDenials, type ReaperReport, type AgentEscalation, type AgentRepairStatus, type MemoryRecord, type SkillLite, type RunLite, type ProviderRoutingRow } from "@/lib/agentdetail";
import { AgentDetailHeroFact, AgentDetailTabButton, AgentNowPanel, AgentPermissionsSnapshot, AgentWakeResult, ApprovalDecision, BoardMessage, CompactChip, ConfigOverrideBox, DetailTab, EMPTY_BOARD_MESSAGES, LifecycleConfigEditor, PRIMARY_TABS, PolicyDecision, PolicyStats, RoutingInfo, Row, StatePill, ToolCatalogRow, ToolInvocation, agentDetailSkillPassport, editableAgentProfile } from "@/components/agentdetail/shared";
import { AgentTaskList, OperationalTaskList, ToolPolicyBox } from "@/components/agentdetail/tasks";
import { CommsTab, agentBoardMessages, agentMailboxPassport, operatorWakeIssue, waitingForAgent } from "@/components/agentdetail/comms";
import { DiagTab } from "@/components/agentdetail/DiagTab";
import { FilesTab } from "@/components/agentdetail/FilesTab";
import { MemoryTab } from "@/components/agentdetail/MemoryTab";
import { ModelTab } from "@/components/agentdetail/ModelTab";
import { Overview } from "@/components/agentdetail/Overview";
import { SkillsTab } from "@/components/agentdetail/SkillsTab";
import { TriggersTab } from "@/components/agentdetail/TriggersTab";
import { agentConfigAccessSummary, agentControlInterventionSummary, agentDelegationPassportDetail, agentGovernancePassport, agentManagedSubagent, effectiveToolPermissions, permissionRowFromSnapshot, summarizePermissionPassport, summarizeWakeAccess } from "@/components/agentdetail/capability";
import { agentHealthContractLedger, agentLifecycleDetail, agentNoisePolicyLabel, agentResourcePassportDetail, agentRetryPolicyDetail } from "@/components/agentdetail/lifecycle";

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
      getJSON<{ records?: MemoryRecord[] }>("/api/memory", { limit: "200" }),
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
      getJSON<{ messages?: BoardMessage[] }>("/api/board", { limit: "200" }),
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

  async function saveExecProfile(value: string) {
    setBusy(true);
    try {
      await postJSON("/api/agents/edit", {
        ref: slug,
        profile: editableAgentProfile(profile, { execution_profile: value }),
      });
      ui.toast(value ? `${slug} runs on "${value}" isolation` : `${slug} isolation cleared`, "success");
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
                  label="isolation"
                  value={
                    <select
                      aria-label="Execution isolation profile"
                      value={profile.execution_profile || ""}
                      disabled={busy}
                      onChange={(e) => saveExecProfile(e.target.value)}
                      className="h-7 rounded-md border border-border bg-panel px-2 text-xs outline-none focus-visible:border-accent"
                    >
                      <option value="">tool defaults</option>
                      <option value="local">local</option>
                      <option value="warden">warden</option>
                      <option value="container">container</option>
                    </select>
                  }
                />
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


export { agentRemovalRiskLabel, agentLifecycleActionResultSummary, agentLifecycleInterventionSummary, agentLifecycleDecisionLedger, agentScheduleBindingTitle, agentRemovalImpactPlan, agentRetryPolicyDetail, agentRepairCommandSummary, agentRepairOperationsSummary, agentRepairDecisionSummary, agentHealthContractLedger, agentOperationsPassport, agentEntityContractLedger, agentAutonomyRunbook, agentRuntimeDoctorLedger, agentSystemGuardianContract, agentResourcePassportDetail } from "@/components/agentdetail/lifecycle";
export { agentMailboxSubjects, mailboxSubjectBinding, agentMailboxWakeContract, mailboxWakeArmIssue, operatorWakeIssue, agentBoardMessages, messageAckedBy, messageAckedByLabel, waitingForAgent, agentInboxPrioritySummary, agentMailboxPassport } from "@/components/agentdetail/comms";
export { agentControlInterventionSummary, agentAuthorityContractSummary, agentAuthorityManifest, agentAuthorityLedger, agentCapabilityRiskPassport, workflowToolAccessSummary, agentDelegationPassportDetail, agentConfigAuthorityContract, normalizeNoiseToolPolicy } from "@/components/agentdetail/capability";
