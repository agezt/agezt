import { useEffect, useMemo, useState } from "react";
import { X, Activity as ActivityIcon, ArrowUpRight, Play, Pause, Bot, CalendarClock, Coins, Cpu, HeartPulse, Megaphone, Wrench, Zap, Archive, ArchiveRestore } from "lucide-react";
import { getJSON, postAction, postJSON } from "@/lib/api";
import { cn, fmtAgo, fmtDue } from "@/lib/utils";
import { money } from "@/lib/format";
import { Button } from "@/components/ui/button";
import { MetricGrid, MetricWidget } from "@/components/ui/metric-widget";
import { useUI, type ConfirmOptions } from "@/components/ui/feedback";
import { useEvents } from "@/lib/events";
import { AgentAvatar } from "@/components/AgentAvatar";
import { AgentActivity } from "@/components/AgentActivity";
import { AgentRepair } from "@/components/AgentRepair";
import { ModelChip } from "@/components/ModelChip";
import { ModelPicker } from "@/components/ModelPicker";
import { openAgent } from "@/lib/agentnav";
import { agentEnableToast, agentRetireToast, agentReviveToast, agentSchedulePressurePassport, guardianQuietPolicyPayload, type AgentEnableResult, type AgentProfile, type AgentRetireResult, type AgentReviveResult } from "@/views/Roster";
import { scheduleAgentSlug, type FleetTrigger, type FleetState, type ApiOrder, type ApiSchedule } from "@/lib/fleet";
import { agentCorrelations, agentRunTrend, filterByCorrelation, filterAgentMemory, filterAgentSkills, summarizeAgent, lastFailure, healthSnapshot, summarizeConfigOverrides, summarizeAutoRepair, summarizeAgentRuntimeStatus, summarizeEscalations, escalationOperationalTasks, type ReaperReport, type AgentEscalation, type AgentRepairStatus, type MemoryRecord, type SkillLite, type RunLite, type ProviderRoutingRow } from "@/lib/agentdetail";
import { AgentDetailTabButton, AgentNowPanel, AgentPermissionsSnapshot, AgentWakeResult, ApprovalDecision, BoardMessage, DetailTab, EMPTY_BOARD_MESSAGES, PRIMARY_TABS, PolicyDecision, PolicyStats, RoutingInfo, StatePill, ToolCatalogRow, ToolInvocation, editableAgentProfile } from "@/components/agentdetail/shared";
import { CommsTab, agentBoardMessages, operatorWakeIssue } from "@/components/agentdetail/comms";
import { DiagTab } from "@/components/agentdetail/DiagTab";
import { MindTab } from "@/components/agentdetail/MindTab";
import { ModelTab } from "@/components/agentdetail/ModelTab";
import { Overview } from "@/components/agentdetail/Overview";
import { TriggersTab } from "@/components/agentdetail/TriggersTab";
import { agentManagedSubagent, summarizeWakeAccess } from "@/components/agentdetail/capability";

// AgentDetail (M953) — the per-agent Command Center: one screen that answers
// "what is this agent, how is it triggered, what has it done, what does it know,
// what is it allowed to do, and what went wrong". Declutter law: a glance layer
// of MetricWidgets in the header, actions inline, six grouped tabs, and the
// Diagnostics tab as the single raw escape hatch.
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
  const runtimeStatus = useMemo(
    () => summarizeAgentRuntimeStatus(profile.status),
    [profile.status],
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
  const runTrend = useMemo(() => agentRunTrend(runs, slug), [runs, slug]);
  // Today's spend for this agent (client-side fold over its runs started today).
  const todaySpentMc = useMemo(() => {
    const d = new Date();
    d.setHours(0, 0, 0, 0);
    const todayMs = d.getTime();
    return runs
      .filter((r) => r.agent === slug && (r.started_unix_ms || 0) >= todayMs)
      .reduce((s, r) => s + (r.spent_mc || 0), 0);
  }, [runs, slug]);
  const schedulePassport = useMemo(
    () => agentSchedulePressurePassport(profile, mySchedules),
    [profile, mySchedules],
  );
  const wakeAccess = agentPermissions?.wake_access;
  const wakePolicy = useMemo(
    () => summarizeWakeAccess(profile, wakeAccess),
    [profile, wakeAccess],
  );
  const wakeOwner =
    wakeAccess?.manager ||
    (agentManagedSubagent(profile)
      ? profile.parent_agent || profile.owner_agent || ""
      : "");

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
  const isSystemGuardian = profile.system || profile.kind === "system";
  const presenceTone = running
    ? "accent"
    : profile.retired || runtimeStatus.operationalState === "paused" || !profile.enabled
      ? "muted"
      : "good";
  const healthTone =
    health.state === "healthy" ? "good" : health.state === "retired" ? "muted" : "bad";
  const spendOver =
    !!profile.max_daily_mc && profile.max_daily_mc > 0 && todaySpentMc > profile.max_daily_mc;

  return (
    <section className="flex min-h-0 flex-col gap-3 rounded-lg border border-border bg-card p-3">
      {/* Header — identity + actions on one band. */}
      <div className="rounded-xl border border-border bg-panel/35 p-3">
        <div className="flex min-w-0 flex-wrap items-start gap-3">
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
              <p className="mt-1 max-w-3xl text-sm leading-6 text-foreground/80 line-clamp-2">
                {profile.description}
              </p>
            )}
            {wakeIssue && (
              <p className="mt-1 flex flex-wrap items-center gap-1 text-[11px] text-muted">
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
          {/* Action rail — flat, no folds. */}
          <div className="flex flex-wrap items-center justify-end gap-1">
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
            {schedulePassport.frequentIds.length > 0 && (
              <Button
                variant="ghost"
                size="sm"
                disabled={busy}
                title={`Pause ${schedulePassport.frequentIds.length} frequent schedule${schedulePassport.frequentIds.length === 1 ? "" : "s"}`}
                aria-label={`Pause frequent schedules for ${slug}`}
                onClick={() => pauseFrequentSchedules(schedulePassport.frequentIds)}
              >
                <CalendarClock className="size-3.5" /> Pause wakes
              </Button>
            )}
            {isSystemGuardian && (
              <Button
                variant="ghost"
                size="sm"
                disabled={busy}
                title="Apply quiet system guardian policy to this agent"
                aria-label={`Quiet guardian ${slug}`}
                onClick={quietSystemGuardian}
              >
                <Megaphone className="size-3.5" /> Quiet guardian
              </Button>
            )}
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

        {/* Glance layer — one metric per fact, numbers and color, no sentences. */}
        <MetricGrid className="mt-3" cols="repeat(auto-fill, minmax(150px, 1fr))">
          <MetricWidget
            icon={ActivityIcon}
            label="Presence"
            value={runtimeStatus.operationalText || state}
            subvalue={
              runtimeStatus.lastActivityMs
                ? `active ${fmtAgo(runtimeStatus.lastActivityMs)}`
                : undefined
            }
            tone={presenceTone}
            pulse={running}
          />
          <MetricWidget
            icon={CalendarClock}
            label="Next wake"
            value={
              runtimeStatus.nextWakeMs
                ? fmtDue(runtimeStatus.nextWakeMs)
                : runtimeStatus.wakeText || "manual"
            }
            subvalue={runtimeStatus.wakeDetail}
            tone={runtimeStatus.nextWakeMs ? "accent" : "muted"}
          />
          <MetricWidget
            icon={Bot}
            label="Runs"
            value={summary.runs}
            subvalue={`${money(summary.totalSpentMc)} total`}
            tone="accent"
            trend={summary.runs > 0 ? runTrend : undefined}
          />
          <MetricWidget
            icon={Coins}
            label="Spend today"
            value={money(todaySpentMc)}
            subvalue={
              profile.max_daily_mc
                ? `cap ${money(profile.max_daily_mc)}`
                : "uncapped"
            }
            tone={spendOver ? "bad" : "good"}
          />
          <MetricWidget
            icon={HeartPulse}
            label="Health"
            value={health.label}
            subvalue={
              fail?.started_unix_ms
                ? `last failure ${fmtAgo(fail.started_unix_ms)}`
                : undefined
            }
            tone={healthTone}
          />
          {/* Model & fallback — same altitude as the metrics, editable in place. */}
          <div className="flex flex-col gap-2 rounded-xl border border-border bg-card p-4 shadow-e1">
            <div className="flex items-center justify-between gap-2">
              <div className="inline-flex items-center gap-1.5 rounded-md bg-panel px-1.5 py-0.5 text-xs font-medium text-foreground">
                <Cpu className="size-3" aria-hidden /> Model
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
            <div className="min-w-0">
              {profile.model ? (
                <ModelChip id={profile.model} chains={routing?.chains} />
              ) : (
                <span className="text-sm text-muted">routing default</span>
              )}
              {(profile.fallbacks || []).length > 0 && (
                <div
                  className="mt-1 truncate font-mono text-[11px] text-muted"
                  title={(profile.fallbacks || []).join(" → ")}
                >
                  → {(profile.fallbacks || []).join(" → ")}
                </div>
              )}
            </div>
          </div>
        </MetricGrid>
      </div>

      {modelPickerOpen && (
        <ModelPicker
          value={profile.model || ""}
          onChange={saveModel}
        />
      )}

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

      {/* Tabs — six grouped sections in one clean row */}
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
            fail={fail}
            health={health}
            repair={repair}
            repairStatus={repairStatus}
            escalation={escalation}
            escalationTasks={activeEscalationTasks}
            todaySpentMc={todaySpentMc}
            memory={myMemory}
            skills={mySkills}
            schedules={mySchedules}
            mailboxMessages={myComms || EMPTY_BOARD_MESSAGES}
            busy={busy}
            onLifecycleChanged={() => {
              setBump((b) => b + 1);
              onChanged?.();
            }}
            onManage={onManage}
            onView={setTab}
            onFocusRun={setActivityFocusRun}
          />
        )}

        {tab === "activity" && (
          <AgentActivity
            slug={slug}
            initialOpenRun={activityFocusRun}
            initialTab="activity"
          />
        )}

        {tab === "wiring" && (
          <div className="space-y-3">
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
          </div>
        )}

        {tab === "mind" && (
          <MindTab
            slug={slug}
            profile={profile}
            overrides={overrides}
            memory={myMemory}
            skills={mySkills}
            busy={busy}
            taskBusy={taskBusy}
            onSaveExecProfile={saveExecProfile}
            onMutateTask={mutateTask}
            onAddTask={addTask}
            onAction={action}
            onChanged={() => {
              setBump((b) => b + 1);
              onChanged?.();
            }}
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

        {tab === "diag" && (
          <div className="space-y-3">
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
          </div>
        )}
      </div>
    </section>
  );
}
