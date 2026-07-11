import { useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { Users, RefreshCw, Pause, Play, Trash2, Plus, Pencil, Bot, Archive, ArchiveRestore, Skull, Activity, Sparkles, IdCard, ShieldCheck, Zap, Wrench, Megaphone, Mail, CalendarClock, GitBranch, AlertTriangle, Radio, Network } from "lucide-react";
import { getJSON, postAction, postJSON } from "@/lib/api";
import { openAgent } from "@/lib/agentnav";
import { openIncident } from "@/lib/incidentnav";
import { cn, fmtDateTime } from "@/lib/utils";
import { money } from "@/lib/format";
import { Button } from "@/components/ui/button";
import { useUI } from "@/components/ui/feedback";
import { SkeletonList } from "@/components/ui/skeleton";
import { EmptyState } from "@/components/ui/empty";
import { Badge } from "@/components/ui/badge";
import { LoadMoreFooter } from "@/components/ui/load-more-footer";
import { Page } from "@/components/ui/page";
import { TabNav } from "@/components/ui/tab-nav";
import { MetricWidget, MetricGrid } from "@/components/ui/metric-widget";
import { ErrorText, KeyValue } from "@/components/JsonView";
import { Disclosure } from "@/components/ui/disclosure";
import { AgentAvatar } from "@/components/AgentAvatar";
import { AgentActivity } from "@/components/AgentActivity";
import { summarizeConfigOverrides, summarizeAgentRuntimeStatus } from "@/lib/agentdetail";
import { useEvents } from "@/lib/events";
import { applyAgentLivePatches, reduceAgentLivePatchMap, shouldReloadAgentCatalog, type AgentLivePatchMap } from "@/lib/agentlive";
import {
  agentEnableToast,
  agentIdentityKind,
  agentRemoveToast,
  agentRetireToast,
  agentReviveToast,
  formatWakeDue,
  type AgentEnableResult,
  type AgentProfile,
  type AgentRemoveResult,
  type AgentRetireResult,
  type AgentReviveResult,
  type RosterBoardMessage,
  type RosterSchedule,
} from "./roster/shared";
import { agentNeedsAttention, agentNeedsRepair, filterAgentRoster, sortAgentRoster, type RosterFilter } from "./roster/filters";
import {
  agentNoiseBudgetPassport,
  agentNoisePolicySummary,
  agentSchedulePressurePassport,
  guardianQuietingSummary,
  guardianQuietPolicyPayload,
  noisySystemGuardians,
  systemGuardianNoiseContract,
  systemGuardianRiskSummary,
  systemGuardianSafetySummary,
} from "./roster/guardians";
import {
  agentGraveyardCleanupPassport,
  agentGraveyardStats,
  agentHealthIssueSummary,
  agentHierarchySummary,
  agentLifecycleSummary,
  agentTaskContractSummary,
  agentTaskProgressSummary,
  rosterRepairIssue,
  rosterWaitingMailboxCounts,
  rosterWakeIssue,
} from "./roster/passports";
import {
  agentRemovalCascadePreset,
  agentRemovalDecisionSummary,
  agentRemovalImpactSummary,
  agentRemovalPlan,
  type AgentRemovalPlanInput,
} from "./roster/removal";
import { EditAgentForm, NewAgentForm, RosterModal } from "./roster/form";
import {
  AgentKindBadge,
  CascadeOption,
  IdentityPill,
  ImpactList,
  RosterSignalPanel,
} from "./roster/cards";

// agentHue maps a slug to a stable hue (0–359) so every agent gets a consistent
// colored identity avatar across the UI. The deterministic hue + monogram now
// live in @/lib/agent (M948) so the avatar can be shared; re-exported here for
// existing importers.
import { agentHue, initials } from "@/lib/agent";
export { agentHue, initials };

// Re-exports: Roster.tsx remains the public module for the roster domain —
// the implementation now lives in src/views/roster/* (mechanical split).
export {
  agentEnableToast,
  agentIdentityKind,
  agentRemoveToast,
  agentRetireToast,
  agentReviveToast,
  formatWakeDue,
  agentNeedsAttention,
  agentNeedsRepair,
  filterAgentRoster,
  sortAgentRoster,
  agentNoiseBudgetPassport,
  agentNoisePolicySummary,
  agentSchedulePressurePassport,
  guardianQuietingSummary,
  guardianQuietPolicyPayload,
  noisySystemGuardians,
  systemGuardianNoiseContract,
  systemGuardianRiskSummary,
  systemGuardianSafetySummary,
  agentGraveyardCleanupPassport,
  agentGraveyardStats,
  agentHealthIssueSummary,
  agentHierarchySummary,
  agentLifecycleSummary,
  agentTaskContractSummary,
  agentTaskProgressSummary,
  rosterRepairIssue,
  rosterWaitingMailboxCounts,
  rosterWakeIssue,
  agentRemovalCascadePreset,
  agentRemovalDecisionSummary,
  agentRemovalImpactSummary,
  agentRemovalPlan,
  EditAgentForm,
  NewAgentForm,
};
export type { AgentEnableResult, AgentProfile, AgentRemoveResult, AgentRetireResult, AgentReviveResult };
export { slugOk, usdToMc } from "./roster/shared";
export { systemGuardianSafetyIssues } from "./roster/guardians";
export { profileFields } from "./roster/form";

// ROSTER_CARD_WINDOW is how many fleet cards render at once. /api/agents has no
// cursor, so the whole roster arrives in one fetch — the window keeps a big
// fleet from ballooning the DOM; a Load-more footer grows it client-side.
// Cross-agent rollups (census metrics, mailbox/schedule pressure) always use
// the FULL list, never the window.
const ROSTER_CARD_WINDOW = 60;

// Roster is the agent-identity console (M785): the durable, named agents
// (M783) — each with its own soul, model, cost ceiling, and memory scope —
// with create/edit/pause/resume/remove governance. Run one from chat or the
// CLI with `agt run --agent <slug>`; the lead delegates to them by name.
export function Roster() {
  const ui = useUI();
  const { subscribe } = useEvents();
  const [profiles, setProfiles] = useState<AgentProfile[] | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [busy, setBusy] = useState<string | null>(null);
  const [showForm, setShowForm] = useState(false);
  const [editing, setEditing] = useState<string | null>(null);
  const [activityFor, setActivityFor] = useState<string | null>(null);
  const [activityFocus, setActivityFocus] = useState<Record<string, string>>({});
  const [rosterFilter, setRosterFilter] = useState<RosterFilter>("all");
  const [cardWin, setCardWin] = useState(ROSTER_CARD_WINDOW);

  // Reset the render window whenever the roster tab changes so a new filter
  // always starts from the top of its result set.
  useEffect(() => {
    setCardWin(ROSTER_CARD_WINDOW);
  }, [rosterFilter]);
  const [retiring, setRetiring] = useState<{
    slug: string;
    reason: string;
    standing: string[];
    schedules: string[];
    memories: string[];
    authoredMemories: string[];
    skills: string[];
    configs: string[];
    workspaces: string[];
    workflowRefs: string[];
    mailboxMessages: string[];
    subagents: string[];
    subagentStanding: string[];
    subagentSchedules: string[];
    subagentMemories: string[];
    subagentAuthoredMemories: string[];
    subagentSkills: string[];
    subagentConfigs: string[];
    subagentWorkspaces: string[];
    subagentWorkflowRefs: string[];
    subagentMailboxMessages: string[];
  } | null>(null);
  const [removing, setRemoving] = useState<{
    slug: string;
    standing: string[];
    schedules: string[];
    memories: string[];
    authoredMemories: string[];
    skills: string[];
    configs: string[];
    workspaces: string[];
    workflowRefs: string[];
    mailboxMessages: string[];
    subagents: string[];
    subagentStanding: string[];
    subagentSchedules: string[];
    subagentMemories: string[];
    subagentAuthoredMemories: string[];
    subagentSkills: string[];
    subagentConfigs: string[];
    subagentWorkspaces: string[];
    subagentWorkflowRefs: string[];
    subagentMailboxMessages: string[];
    cascade: AgentRemovalPlanInput["cascade"];
  } | null>(null);
  // Per-agent private-skill counts (M943): how many skills each agent owns
  // (Skill.Agent == slug), so an operator sees who has learned what before
  // sharing/reassigning (M942) or exporting (`agt skill export --all --agent`).
  const [skillCounts, setSkillCounts] = useState<Record<string, number>>({});
  const [mailboxCounts, setMailboxCounts] = useState<Record<string, number>>({});
  const [schedulePressure, setSchedulePressure] = useState<Record<string, ReturnType<typeof agentSchedulePressurePassport>>>({});
  const [livePatches, setLivePatches] = useState<AgentLivePatchMap>({});
  // Keep the interval handle so a refresh-on-event nudge can coexist with the
  // poll without leaking timers (preserves the original useRef import).
  const pollRef = useRef<ReturnType<typeof setInterval> | null>(null);
  // Per-poll AbortController. We cancel any in-flight reload the next time
  // reload runs so an over-budget request can't accumulate (the original
  // 8-second setInterval would stack up aborted-but-unmounted fetches if
  // the backend ever took longer than the poll period; on a large journal
  // the 11× journal.Range walk in `agentStatusViews` could realistically
  // trip the kernel's 10-minute per-connection read deadline, leading to a
  // hung reload that prevented the next poll from doing anything useful).
  const reloadAbortRef = useRef<AbortController | null>(null);

  async function reload() {
    // Cancel any in-flight previous reload before starting the next one.
    if (reloadAbortRef.current) {
      try { reloadAbortRef.current.abort(); } catch { /* noop */ }
    }
    const ac = new AbortController();
    reloadAbortRef.current = ac;
    // Soft client-side cap so a stuck `/api/agents` (which itself can drive
    // up to O(11N) journal work) can't freeze the page. We treat any abort
    // as "this tick had no fresh data", keeping the previous rendered list
    // and merely bumping `err` so the operator sees a non-fatal warning.
    // Five seconds covers a worst-case healthy journal; the polling cadence
    // (8s) means we don't even visibly flicker.
    const timeoutMs = 5000;
    setLoading(true);
    try {
      const [d, sk, bd, sch] = await Promise.all([
        getJSON<{ profiles?: AgentProfile[] }>("/api/agents", undefined, { signal: ac.signal, timeoutMs }),
        getJSON<{ skills?: { agent?: string }[] }>("/api/skills", undefined, { signal: ac.signal }).catch(() => ({ skills: [] })),
        getJSON<{ messages?: RosterBoardMessage[] }>("/api/board", undefined, { signal: ac.signal }).catch(() => ({ messages: [] })),
        getJSON<{ schedules?: RosterSchedule[] }>("/api/schedules", undefined, { signal: ac.signal }).catch(() => ({ schedules: [] })),
      ]);
      // If the user navigated or another reload fired, drop this tick's data.
      if (reloadAbortRef.current !== ac) return;
      const nextProfiles = d.profiles || [];
      setProfiles(nextProfiles);
      const counts: Record<string, number> = {};
      for (const s of sk.skills || []) {
        if (s.agent) counts[s.agent] = (counts[s.agent] || 0) + 1;
      }
      setSkillCounts(counts);
      setMailboxCounts(rosterWaitingMailboxCounts(bd?.messages || [], nextProfiles.map((p) => p.slug)));
      setSchedulePressure(Object.fromEntries(nextProfiles.map((p) => [p.slug, agentSchedulePressurePassport(p, sch?.schedules || [])])));
      setErr(null);
    } catch (e) {
      // Aborted (timeout or superseded poll) is a soft "no fresh data" — the
      // previous list stays on screen and we don't surface it as a hard error.
      const err2 = e as (Error & { status?: number });
      const aborted = err2?.name === "AbortError" || ac.signal.aborted || err2?.status === 0;
      if (aborted) return;
      if (reloadAbortRef.current !== ac) return;
      setErr(err2.message);
    } finally {
      if (reloadAbortRef.current === ac) {
        reloadAbortRef.current = null;
        setLoading(false);
      }
    }
  }

  useEffect(() => {
    reload();
    pollRef.current = setInterval(reload, 8000);
    return () => {
      if (pollRef.current) clearInterval(pollRef.current);
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    let t: ReturnType<typeof setTimeout> | undefined;
    const off = subscribe((ev) => {
      setLivePatches((prev) => reduceAgentLivePatchMap(prev, ev));
      if (!shouldReloadAgentCatalog(ev)) return;
      if (t) clearTimeout(t);
      t = setTimeout(() => void reload(), 1200);
    });
    return () => {
      if (t) clearTimeout(t);
      off();
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [subscribe]);

  // retire moves an agent to the graveyard (M846): fetch the impact first (which
  // standing orders fire it) and show it in the confirm, so the effects are
  // explicit before the agent is retired. Recoverable via Revive.
  async function retire(slug: string) {
    let impact: {
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
    } = {};
    try {
      impact = await getJSON("/api/agents/impact", { ref: slug });
    } catch {
      // Impact is advisory; proceed without it if the lookup fails.
    }
    setRetiring({
      slug,
      reason: "",
      standing: impact.standing_orders || [],
      schedules: impact.schedules || [],
      memories: impact.memories || [],
      authoredMemories: impact.authored_shared_memories || [],
      skills: impact.skills || [],
      configs: impact.configs || [],
      workspaces: impact.workspaces || [],
      workflowRefs: impact.workflow_refs || [],
      mailboxMessages: impact.mailbox_messages || [],
      subagents: impact.subagents || [],
      subagentStanding: impact.subagent_standing_orders || [],
      subagentSchedules: impact.subagent_schedules || [],
      subagentMemories: impact.subagent_memories || [],
      subagentAuthoredMemories: impact.subagent_authored_shared_memories || [],
      subagentSkills: impact.subagent_skills || [],
      subagentConfigs: impact.subagent_configs || [],
      subagentWorkspaces: impact.subagent_workspaces || [],
      subagentWorkflowRefs: impact.subagent_workflow_refs || [],
      subagentMailboxMessages: impact.subagent_mailbox_messages || [],
    });
  }

  async function confirmRetire() {
    if (!retiring) return;
    const { slug } = retiring;
    const reason = retiring.reason.trim();
    setRetiring(null);
    setBusy(slug);
    try {
      const res = await postAction<AgentRetireResult>(
        "/api/agents/retire",
        reason ? { ref: slug, reason } : { ref: slug },
      );
      ui.toast(agentRetireToast(slug, res), "success");
      await reload();
    } catch (e) {
      ui.toast((e as Error).message, "error");
    } finally {
      setBusy(null);
    }
  }

  async function revive(slug: string) {
    setBusy(slug);
    try {
      const res = await postAction<AgentReviveResult>("/api/agents/revive", { ref: slug });
      ui.toast(agentReviveToast(slug, res), "success");
      await reload();
    } catch (e) {
      ui.toast((e as Error).message, "error");
    } finally {
      setBusy(null);
    }
  }

  async function setAgentEnabled(slug: string, enabled: boolean) {
    setBusy(slug);
    try {
      const res = await postAction<AgentEnableResult>("/api/agents/enable", {
        ref: slug,
        enabled: enabled ? "true" : "false",
      });
      ui.toast(agentEnableToast(slug, enabled, res), "success");
      await reload();
    } catch (e) {
      ui.toast((e as Error).message, "error");
    } finally {
      setBusy(null);
    }
  }

  async function wakeAgent(slug: string) {
    setBusy(slug);
    try {
      const res = await postAction<{ correlation_id?: string }>("/api/agents/wake", { ref: slug, reason: "manual operator wake" });
      ui.toast(`${slug} wake queued`, "success");
      if (res?.correlation_id) setActivityFocus((prev) => ({ ...prev, [slug]: res.correlation_id || "" }));
      setActivityFor(slug);
      await reload();
    } catch (e) {
      ui.toast((e as Error).message, "error");
    } finally {
      setBusy(null);
    }
  }

  async function repairAgent(slug: string) {
    setBusy(slug);
    try {
      const res = await postJSON<{ correlation_id?: string }>("/api/agents/repair", {
        ref: slug,
        reason: `operator requested repair from ${slug} roster card`,
      });
      ui.toast(res?.correlation_id ? `${slug} repair accepted (${res.correlation_id})` : `${slug} repair accepted`, "success");
      if (res?.correlation_id) setActivityFocus((prev) => ({ ...prev, [slug]: res.correlation_id || "" }));
      setActivityFor(slug);
      await reload();
    } catch (e) {
      ui.toast((e as Error).message, "error");
    } finally {
      setBusy(null);
    }
  }

  async function removeAgent(slug: string) {
    let impact: {
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
    } = {};
    try {
      impact = await getJSON("/api/agents/impact", { ref: slug });
    } catch {
      // Impact is advisory; the remove call itself remains authoritative.
    }
    const standing = impact.standing_orders || [];
    const schedules = impact.schedules || [];
    const memories = impact.memories || [];
    const authoredMemories = impact.authored_shared_memories || [];
    const skills = impact.skills || [];
    const configs = impact.configs || [];
    const workspaces = impact.workspaces || [];
    const workflowRefs = impact.workflow_refs || [];
    const mailboxMessages = impact.mailbox_messages || [];
    const subagents = impact.subagents || [];
    const subagentStanding = impact.subagent_standing_orders || [];
    const subagentSchedules = impact.subagent_schedules || [];
    const subagentMemories = impact.subagent_memories || [];
    const subagentAuthoredMemories = impact.subagent_authored_shared_memories || [];
    const subagentSkills = impact.subagent_skills || [];
    const subagentConfigs = impact.subagent_configs || [];
    const subagentWorkspaces = impact.subagent_workspaces || [];
    const subagentWorkflowRefs = impact.subagent_workflow_refs || [];
    const subagentMailboxMessages = impact.subagent_mailbox_messages || [];
    const hasSubagents = subagents.length > 0;
    setRemoving({
      slug,
      standing,
      schedules,
      memories,
      authoredMemories,
      skills,
      configs,
      workspaces,
      workflowRefs,
      mailboxMessages,
      subagents,
      subagentStanding,
      subagentSchedules,
      subagentMemories,
      subagentAuthoredMemories,
      subagentSkills,
      subagentConfigs,
      subagentWorkspaces,
      subagentWorkflowRefs,
      subagentMailboxMessages,
      cascade: {
        standing: standing.length > 0,
        schedules: schedules.length > 0,
        memory: memories.length > 0 || (hasSubagents && subagentMemories.length > 0),
        authored_memory: false,
        skills: skills.length > 0 || (hasSubagents && subagentSkills.length > 0),
        config: configs.length > 0 || (hasSubagents && subagentConfigs.length > 0),
        workspace: workspaces.length > 0 || (hasSubagents && subagentWorkspaces.length > 0),
        subagents: hasSubagents,
      },
    });
  }

  async function confirmRemove() {
    if (!removing) return;
    const target = removing;
    setBusy(target.slug);
    try {
      const res = await postJSON<AgentRemoveResult>("/api/agents/remove", { ref: target.slug, cascade: target.cascade });
      setRemoving(null);
      ui.toast(
        agentRemoveToast(target.slug, res),
        res.removed ? "success" : "info",
      );
      await reload();
    } catch (e) {
      ui.toast((e as Error).message, "error");
    } finally {
      setBusy(null);
    }
  }

  async function quietNoisyGuardians() {
    const targets = noisySystemGuardians(list);
    const activeGuardians = list.filter((profile) => profile.system && !profile.retired);
    const frequentScheduleIds = Array.from(new Set(
      activeGuardians.flatMap((profile) => schedulePressure[profile.slug]?.frequentIds || []),
    ));
    if (targets.length === 0 && frequentScheduleIds.length === 0) return;
    setBusy("guardians");
    try {
      await Promise.all(
        targets.map((profile) =>
          postJSON("/api/agents/capabilities", guardianQuietPolicyPayload(profile)),
        ).concat(frequentScheduleIds.map((id) => postAction("/api/schedule/enable", { id, enabled: "false" }))),
      );
      ui.toast(
        `${targets.length} system guardian${targets.length === 1 ? "" : "s"} quieted${frequentScheduleIds.length > 0 ? `; ${frequentScheduleIds.length} frequent schedule${frequentScheduleIds.length === 1 ? "" : "s"} paused` : ""}`,
        "success",
      );
      await reload();
    } catch (e) {
      ui.toast((e as Error).message, "error");
    } finally {
      setBusy(null);
    }
  }

  async function pauseFrequentSchedules(slug: string, scheduleIds: string[]) {
    const ids = Array.from(new Set(scheduleIds.filter(Boolean)));
    if (ids.length === 0) return;
    if (!(await ui.confirm({
      title: `Pause frequent schedules for ${slug}?`,
      message: `${ids.length} schedule${ids.length === 1 ? "" : "s"} will stop waking this agent automatically. Manual wake, mailbox wake, and delegation remain available.`,
      confirmLabel: "Pause schedules",
      danger: false,
    }))) return;
    setBusy(`schedule:${slug}`);
    try {
      await Promise.all(ids.map((id) => postAction("/api/schedule/enable", { id, enabled: "false" })));
      ui.toast(`${ids.length} frequent schedule${ids.length === 1 ? "" : "s"} paused for ${slug}`, "success");
      await reload();
    } catch (e) {
      ui.toast((e as Error).message, "error");
    } finally {
      setBusy(null);
    }
  }

  const list = useMemo(() => sortAgentRoster(applyAgentLivePatches(profiles || [], livePatches)), [profiles, livePatches]);
  const shownList = useMemo(() => filterAgentRoster(list, rosterFilter, mailboxCounts, schedulePressure), [list, mailboxCounts, rosterFilter, schedulePressure]);
  const visibleList = useMemo(() => shownList.slice(0, cardWin), [shownList, cardWin]);
  const enabled = list.filter((p) => p.enabled && !p.retired).length;
  const paused = list.filter((p) => !p.enabled && !p.retired).length;
  const graveyard = list.filter((p) => p.retired).length;
  const direct = list.filter((p) => !p.retired && agentIdentityKind(p) === "custom").length;
  const subagents = list.filter((p) => !p.retired && agentIdentityKind(p) === "subagent").length;
  const system = list.filter((p) => !p.retired && agentIdentityKind(p) === "system").length;
  const repair = list.filter((p) => !p.retired && agentNeedsRepair(p)).length;
  const mailboxAgents = list.filter((p) => !p.retired && (mailboxCounts[p.slug.toLowerCase()] || mailboxCounts[p.slug] || 0) > 0).length;
  const mailboxBacklog = list.reduce((sum, p) => sum + (p.retired ? 0 : mailboxCounts[p.slug.toLowerCase()] || mailboxCounts[p.slug] || 0), 0);
  const attention = list.filter((p) => agentNeedsAttention(p, mailboxCounts, schedulePressure)).length;
  const guardianRisk = systemGuardianRiskSummary(list);
  const noisyGuardians = noisySystemGuardians(list);
  const guardianQuieting = guardianQuietingSummary(list, schedulePressure);
  const removalPlan = removing ? agentRemovalPlan(removing) : null;
  const removalDecision = removalPlan ? agentRemovalDecisionSummary(removalPlan) : null;
  const removalIncludesSubagents = !!removing?.cascade.subagents;
  const removalToggleItems = removing
    ? {
        standing: removalIncludesSubagents ? [...removing.standing, ...removing.subagentStanding] : removing.standing,
        schedules: removalIncludesSubagents ? [...removing.schedules, ...removing.subagentSchedules] : removing.schedules,
        memory: removalIncludesSubagents ? [...removing.memories, ...removing.subagentMemories] : removing.memories,
        authoredMemory: removalIncludesSubagents ? [...removing.authoredMemories, ...removing.subagentAuthoredMemories] : removing.authoredMemories,
        skills: removalIncludesSubagents ? [...removing.skills, ...removing.subagentSkills] : removing.skills,
        configs: removalIncludesSubagents ? [...removing.configs, ...removing.subagentConfigs] : removing.configs,
        workspaces: removalIncludesSubagents ? [...removing.workspaces, ...removing.subagentWorkspaces] : removing.workspaces,
      }
    : null;
  const graveyardStats = agentGraveyardStats(list);
  const graveyardCleanup = agentGraveyardCleanupPassport(list);
  const graveyardAgents = useMemo(
    () =>
      list
        .filter((p) => p.retired)
        .sort((a, b) => {
          const bm = b.retired_ms || 0;
          const am = a.retired_ms || 0;
          if (bm !== am) return bm - am;
          return a.slug.localeCompare(b.slug);
        })
        .slice(0, 6),
    [list],
  );

  return (
    <Page
      icon={Users}
      title="Agent roster"
      width="wide"
      actions={
        <>
          <Button size="sm" variant="ghost" onClick={reload} disabled={loading} aria-label="Refresh">
            <RefreshCw className={cn("h-3.5 w-3.5", loading && "animate-spin")} />
          </Button>
          {guardianQuieting.tone === "warn" && (
            <Button
              size="sm"
              variant="ghost"
              onClick={quietNoisyGuardians}
              disabled={busy === "guardians"}
              title="Apply quiet policy and pause frequent system guardian schedules"
              aria-label="Quiet noisy guardians"
            >
              <Megaphone className="h-3.5 w-3.5" />
              Quiet guardians
            </Button>
          )}
          <Button size="sm" onClick={() => setShowForm(true)}>
            <Plus className="h-3.5 w-3.5" />
            New agent
          </Button>
        </>
      }
    >

      {showForm && (
        <RosterModal title="New agent" icon={Plus} onClose={() => setShowForm(false)}>
          <NewAgentForm
            onCreated={(slug) => {
              setShowForm(false);
              ui.toast(`agent ${slug} created`, "success");
              reload();
            }}
            onError={(msg) => ui.toast(msg, "error")}
          />
        </RosterModal>
      )}

      {profiles && profiles.length > 0 && (
        <MetricGrid>
          <MetricWidget icon={Bot} label="Agents" value={list.length} tone="muted" />
          <MetricWidget icon={Radio} label="Enabled" value={enabled} tone={enabled > 0 ? "good" : "muted"} />
          <MetricWidget icon={Pause} label="Paused" value={paused} tone={paused > 0 ? "warn" : "muted"} />
          <MetricWidget icon={GitBranch} label="Sub-agents" value={subagents} tone="muted" />
          <MetricWidget icon={AlertTriangle} label="Attention" value={attention} tone={attention > 0 ? "warn" : "muted"} />
          <MetricWidget icon={Wrench} label="Repair" value={repair} tone={repair > 0 ? "bad" : "muted"} />
          <MetricWidget icon={Mail} label="Inbox" value={mailboxBacklog} tone={mailboxBacklog > 0 ? "warn" : "muted"} />
          <MetricWidget icon={Skull} label="Graveyard" value={graveyard} />
        </MetricGrid>
      )}

      {guardianRisk && (
        <RosterSignalPanel
          icon={ShieldCheck}
          title="Guardian noise"
          status={noisyGuardians.length > 0 ? `${noisyGuardians.length} noisy` : guardianQuieting.label}
          tone={guardianQuieting.tone === "warn" ? "warn" : guardianQuieting.tone === "good" ? "good" : "muted"}
        >
          <p className="text-xs text-muted">{guardianQuieting.detail}</p>
          {guardianQuieting.tone === "warn" && (
            <div className="mt-2 flex flex-wrap gap-1.5">
              <Badge variant="warn">{guardianQuieting.quietTargets} quiet target{guardianQuieting.quietTargets === 1 ? "" : "s"}</Badge>
              <Badge variant={guardianQuieting.frequentSchedules > 0 ? "warn" : "default"}>
                {guardianQuieting.frequentSchedules} frequent schedule{guardianQuieting.frequentSchedules === 1 ? "" : "s"}
              </Badge>
              {noisyGuardians.slice(0, 4).map((profile) => (
                <Badge key={profile.slug} variant="default">{profile.slug}</Badge>
              ))}
              {noisyGuardians.length > 4 && <Badge variant="default">+{noisyGuardians.length - 4}</Badge>}
            </div>
          )}
        </RosterSignalPanel>
      )}

      {profiles && profiles.length > 0 && (
        <TabNav
          tabs={([
            { id: "all" as RosterFilter, label: "All", icon: Network, count: list.length, content: null as React.ReactNode },
            { id: "attention" as RosterFilter, label: "Attention", icon: AlertTriangle, count: attention, content: null as React.ReactNode },
            { id: "direct" as RosterFilter, label: "Direct", icon: Bot, count: direct, content: null as React.ReactNode },
            { id: "subagents" as RosterFilter, label: "Sub-agents", icon: GitBranch, count: subagents, content: null as React.ReactNode },
            { id: "system" as RosterFilter, label: "System", icon: ShieldCheck, count: system, content: null as React.ReactNode },
            { id: "repair" as RosterFilter, label: "Repair", icon: Wrench, count: repair, content: null as React.ReactNode },
            { id: "mailbox" as RosterFilter, label: "Inbox", icon: Mail, count: mailboxAgents, content: null as React.ReactNode },
            { id: "paused" as RosterFilter, label: "Paused", icon: Pause, count: paused, content: null as React.ReactNode },
            { id: "graveyard" as RosterFilter, label: "Graveyard", icon: Skull, count: graveyard, content: null as React.ReactNode },
          ] as { id: RosterFilter; label: string; icon: typeof Network; count: number; content: React.ReactNode }[]).filter(t => t.count > 0 || t.id === "all")}
          value={rosterFilter}
          onValueChange={(v) => setRosterFilter(v as RosterFilter)}
        />
      )}

      {graveyardStats.count > 0 && (
        <section className="rounded-xl border border-border bg-card/45 p-3" aria-label="Agent graveyard">
          <div className="flex flex-wrap items-start gap-3">
            <div className="min-w-0 flex-1">
              <div className="flex items-center gap-1.5 text-xs font-semibold uppercase tracking-normal text-muted">
                <Skull className="h-3 w-3" /> Agent graveyard
              </div>
              <div className="mt-1 text-sm font-medium text-foreground">
                {graveyardStats.count} retired identit{graveyardStats.count === 1 ? "y" : "ies"}
              </div>
              <div className="mt-1 flex flex-wrap gap-x-3 gap-y-1 text-[11px] text-muted">
                <span>{graveyardStats.custom} custom</span>
                <span>{graveyardStats.subagents} sub-agent</span>
                <span>{graveyardStats.system} system</span>
                <span>{graveyardStats.withReason} with reason</span>
                <span>{graveyardCleanup.detail}</span>
                {graveyardStats.oldest?.retired_ms && (
                  <span>oldest: {graveyardStats.oldest.slug} · {fmtDateTime(graveyardStats.oldest.retired_ms)}</span>
                )}
              </div>
            </div>
            <div
              className={cn(
                "rounded-lg border px-2 py-1.5 text-xs",
                graveyardCleanup.tone === "warn"
                  ? "border-warn/40 bg-warn/10"
                  : graveyardCleanup.tone === "good"
                    ? "border-good/30 bg-good/5"
                    : "border-border bg-panel/50",
              )}
              title={graveyardCleanup.detail}
            >
              <div className={cn("text-xs font-semibold uppercase tracking-normal", graveyardCleanup.tone === "warn" ? "text-warn" : graveyardCleanup.tone === "good" ? "text-good" : "text-muted")}>
                Cleanup
              </div>
              <div className="mt-0.5 font-medium text-foreground/85">{graveyardCleanup.label}</div>
            </div>
            <Button size="sm" variant="ghost" onClick={() => setRosterFilter("graveyard")}>
              <Skull className="h-3.5 w-3.5" /> View graveyard
            </Button>
          </div>
          <div className="mt-2 grid gap-1.5 md:grid-cols-2 xl:grid-cols-3">
            {graveyardAgents.map((p) => (
              <div key={p.slug} className="min-w-0 rounded-lg border border-border bg-panel/45 p-2">
                <div className="flex min-w-0 items-start gap-2">
                  <AgentAvatar slug={p.slug} name={p.name} size={28} status="retired" />
                  <div className="min-w-0 flex-1">
                    <button
                      type="button"
                      onClick={() => openAgent(p.slug)}
                      className="truncate font-mono text-xs font-semibold text-foreground hover:underline"
                    >
                      {p.slug}
                    </button>
                    {p.retired_reason?.trim() && (
                      <div className="mt-0.5 truncate text-[11px] text-muted" title={p.retired_reason.trim()}>
                        {p.retired_reason.trim()}
                      </div>
                    )}
                    <div className="mt-1 text-xs text-muted">
                      {agentIdentityKind(p)}{p.retired_ms ? ` · ${fmtDateTime(p.retired_ms)}` : ""}
                    </div>
                  </div>
                </div>
                <div className="mt-2 flex flex-wrap items-center gap-1">
                  <Button
                    size="sm"
                    variant="ghost"
                    aria-label={`Open graveyard identity ${p.slug}`}
                    onClick={() => openAgent(p.slug)}
                  >
                    <IdCard className="h-3.5 w-3.5" /> Open
                  </Button>
                  <Button
                    size="sm"
                    variant="ghost"
                    aria-label={`Revive from graveyard ${p.slug}`}
                    disabled={busy === p.slug}
                    onClick={() => revive(p.slug)}
                  >
                    <ArchiveRestore className="h-3.5 w-3.5" /> Revive
                  </Button>
                  {!p.system && (
                    <Button
                      size="sm"
                      variant="danger"
                      aria-label={`Remove from graveyard ${p.slug}`}
                      disabled={busy === p.slug}
                      onClick={() => removeAgent(p.slug)}
                    >
                      <Trash2 className="h-3.5 w-3.5" /> Remove
                    </Button>
                  )}
                </div>
              </div>
            ))}
          </div>
        </section>
      )}

      {retiring && (
        <div
          role="dialog"
          aria-labelledby="retire-agent-title"
          className="glass rounded-xl border-bad/40 p-3 shadow-e2"
        >
          <div className="flex flex-wrap items-start gap-3">
            <div className="flex min-w-0 flex-1 flex-col gap-1">
              <div id="retire-agent-title" className="text-sm font-semibold text-foreground">
                Retire {retiring.slug} to the graveyard
              </div>
              <p className="text-xs text-muted">
                The profile stays recoverable, but the agent is paused and excluded from delegation until revived.
              </p>
              {retiring.standing.length + retiring.schedules.length + retiring.memories.length + retiring.authoredMemories.length + retiring.skills.length + retiring.configs.length + retiring.workspaces.length + retiring.workflowRefs.length + retiring.mailboxMessages.length + retiring.subagents.length + retiring.subagentStanding.length + retiring.subagentSchedules.length + retiring.subagentMemories.length + retiring.subagentAuthoredMemories.length + retiring.subagentSkills.length + retiring.subagentConfigs.length + retiring.subagentWorkspaces.length + retiring.subagentWorkflowRefs.length + retiring.subagentMailboxMessages.length > 0 && (
                <div className="mt-2 rounded-lg border border-bad/30 bg-bad/5 p-2">
                  <div className="text-xs font-medium text-bad">Retirement impact</div>
                  <div className="mt-2 grid gap-2 md:grid-cols-2">
                    <ImpactList label="Standing orders" count={retiring.standing.length} items={retiring.standing} />
                    <ImpactList label="Schedules" count={retiring.schedules.length} items={retiring.schedules} />
                    <ImpactList label="Private memory" count={retiring.memories.length} items={retiring.memories} note="Kept inspectable; not deleted." />
                    <ImpactList label="Authored shared memory" count={retiring.authoredMemories.length} items={retiring.authoredMemories} note="Shared brain records are kept unless explicitly removed." />
                    <ImpactList label="Private skills" count={retiring.skills.length} items={retiring.skills} note="Kept inspectable; not archived." />
                    <ImpactList label="Agent config" count={retiring.configs.length} items={retiring.configs} note="Kept inspectable; remove can delete owned config entries." />
                    <ImpactList label="Workspace" count={retiring.workspaces.length} items={retiring.workspaces} note="Kept inspectable; remove can delete the agent workdir." />
                    <ImpactList label="Workflow references" count={retiring.workflowRefs.length} items={retiring.workflowRefs} note="Kept inspectable; workflows are reusable chains, not agent identities." />
                    <ImpactList label="Mailbox / audit history" count={retiring.mailboxMessages.length} items={retiring.mailboxMessages} note="Kept inspectable with the retired identity." />
                    <ImpactList label="Dependent sub-agent tree" count={retiring.subagents.length} items={retiring.subagents} note="Parent/owner links remain; remove can retire the full descendant tree." />
                    <ImpactList label="Sub-agent standing orders" count={retiring.subagentStanding.length} items={retiring.subagentStanding} note="Remove can clean these with standing + sub-agent cleanup." />
                    <ImpactList label="Sub-agent schedules" count={retiring.subagentSchedules.length} items={retiring.subagentSchedules} note="Remove can clean these with schedule + sub-agent cleanup." />
                    <ImpactList label="Sub-agent private memory" count={retiring.subagentMemories.length} items={retiring.subagentMemories} note="Remove can clean these with private memory + sub-agent cleanup." />
                    <ImpactList label="Sub-agent authored shared memory" count={retiring.subagentAuthoredMemories.length} items={retiring.subagentAuthoredMemories} note="Remove can clean these with authored shared memory + sub-agent cleanup." />
                    <ImpactList label="Sub-agent skills" count={retiring.subagentSkills.length} items={retiring.subagentSkills} note="Remove can clean these with private skills + sub-agent cleanup." />
                    <ImpactList label="Sub-agent config" count={retiring.subagentConfigs.length} items={retiring.subagentConfigs} note="Remove can clean these with agent config + sub-agent cleanup." />
                    <ImpactList label="Sub-agent workspace" count={retiring.subagentWorkspaces.length} items={retiring.subagentWorkspaces} note="Remove can clean these with workspace + sub-agent cleanup." />
                    <ImpactList label="Sub-agent workflow references" count={retiring.subagentWorkflowRefs.length} items={retiring.subagentWorkflowRefs} note="Kept with workflow graphs; inspect before removing the identity tree." />
                    <ImpactList label="Sub-agent mailbox / audit history" count={retiring.subagentMailboxMessages.length} items={retiring.subagentMailboxMessages} note="Kept inspectable with the dependent retired identities." />
                  </div>
                </div>
              )}
              <label className="mt-2 flex flex-col gap-1 text-[11px] text-muted">
                Retirement reason
                <textarea
                  aria-label="Retirement reason"
                  value={retiring.reason}
                  onChange={(e) => setRetiring((r) => (r ? { ...r, reason: e.target.value } : r))}
                  rows={2}
                  placeholder="why this identity is being retired"
                  className="rounded-md border border-border bg-panel px-2 py-1 text-sm text-foreground outline-none focus-visible:border-accent"
                />
              </label>
            </div>
            <div className="flex shrink-0 gap-2">
              <Button size="sm" variant="ghost" onClick={() => setRetiring(null)}>
                Cancel
              </Button>
              <Button size="sm" variant="danger" disabled={busy === retiring.slug} onClick={confirmRetire}>
                <Archive className="h-3.5 w-3.5" /> Retire
              </Button>
            </div>
          </div>
        </div>
      )}

      {removing && (
        <div role="dialog" aria-labelledby="remove-agent-title" className="glass rounded-xl border-bad/40 p-3 shadow-e2">
          <div className="flex flex-wrap items-start gap-3">
            <div className="min-w-0 flex-1">
              <div id="remove-agent-title" className="text-sm font-semibold text-foreground">
                Remove {removing.slug}
              </div>
              <p className="mt-1 text-xs text-muted">
                This deletes the identity profile. Select which private/owned resources should be cleaned up with it.
              </p>
              {removalDecision && (
                <div
                  className={cn(
                    "mt-3 rounded-lg border px-2 py-1.5 text-xs",
                    removalDecision.tone === "bad"
                      ? "border-bad/40 bg-bad/10"
                      : removalDecision.tone === "warn"
                        ? "border-warn/40 bg-warn/10"
                        : removalDecision.tone === "good"
                          ? "border-good/35 bg-good/5"
                          : "border-border bg-card/55",
                  )}
                >
                  <div
                    className={cn(
                      "font-medium",
                      removalDecision.tone === "bad"
                        ? "text-bad"
                        : removalDecision.tone === "warn"
                          ? "text-warn"
                          : removalDecision.tone === "good"
                            ? "text-good"
                            : "text-muted",
                    )}
                  >
                    {removalDecision.label}
                  </div>
                  <div className="mt-0.5 text-muted">{removalDecision.detail}</div>
                </div>
              )}
              <div className="mt-3 grid gap-2 md:grid-cols-2">
                <div className="md:col-span-2 flex flex-wrap items-center gap-2 rounded-lg border border-border bg-card/50 p-2 text-xs text-muted">
                  <span className="mr-auto">Cleanup preset</span>
                  <Button
                    size="sm"
                    variant="ghost"
                    onClick={() => setRemoving((r) => (r ? { ...r, cascade: agentRemovalCascadePreset("clean_all") } : r))}
                  >
                    Clean all
                  </Button>
                  <Button
                    size="sm"
                    variant="ghost"
                    onClick={() => setRemoving((r) => (r ? { ...r, cascade: agentRemovalCascadePreset("keep_all") } : r))}
                  >
                    Keep all
                  </Button>
                </div>
                <CascadeOption
                  label="Standing orders"
                  count={removalToggleItems?.standing.length || 0}
                  checked={removing.cascade.standing}
                  onChange={(v) => setRemoving((r) => (r ? { ...r, cascade: { ...r.cascade, standing: v } } : r))}
                  items={removalToggleItems?.standing || []}
                  note={removalIncludesSubagents ? "Includes dependent sub-agent tree standing orders." : undefined}
                />
                <CascadeOption
                  label="Schedules"
                  count={removalToggleItems?.schedules.length || 0}
                  checked={removing.cascade.schedules}
                  onChange={(v) => setRemoving((r) => (r ? { ...r, cascade: { ...r.cascade, schedules: v } } : r))}
                  items={removalToggleItems?.schedules || []}
                  note={removalIncludesSubagents ? "Includes dependent sub-agent tree schedules." : undefined}
                />
                <CascadeOption
                  label="Private memory"
                  count={removalToggleItems?.memory.length || 0}
                  checked={removing.cascade.memory}
                  onChange={(v) => setRemoving((r) => (r ? { ...r, cascade: { ...r.cascade, memory: v } } : r))}
                  items={removalToggleItems?.memory || []}
                  note={removalIncludesSubagents ? "Includes dependent sub-agent tree private memory." : "Only this agent's private scope."}
                />
                <CascadeOption
                  label="Authored shared memory"
                  count={removalToggleItems?.authoredMemory.length || 0}
                  checked={removing.cascade.authored_memory}
                  onChange={(v) => setRemoving((r) => (r ? { ...r, cascade: { ...r.cascade, authored_memory: v } } : r))}
                  items={removalToggleItems?.authoredMemory || []}
                  note={removalIncludesSubagents ? "Includes dependent sub-agent tree authored shared memory; off by default." : "Shared brain records this agent wrote; off by default."}
                />
                <CascadeOption
                  label="Private skills"
                  count={removalToggleItems?.skills.length || 0}
                  checked={removing.cascade.skills}
                  onChange={(v) => setRemoving((r) => (r ? { ...r, cascade: { ...r.cascade, skills: v } } : r))}
                  items={removalToggleItems?.skills || []}
                  note={removalIncludesSubagents ? "Includes dependent sub-agent tree private skills; shared skills are kept." : "Shared skills are kept."}
                />
                <CascadeOption
                  label="Agent config"
                  count={removalToggleItems?.configs.length || 0}
                  checked={removing.cascade.config}
                  onChange={(v) => setRemoving((r) => (r ? { ...r, cascade: { ...r.cascade, config: v } } : r))}
                  items={removalToggleItems?.configs || []}
                  note={removalIncludesSubagents ? "Includes dependent sub-agent tree config; shared config entries stay with removed agents pruned from access lists." : "Owned config entries are deleted; shared config entries stay, with this agent pruned from access lists."}
                />
                <CascadeOption
                  label="Workspace"
                  count={removalToggleItems?.workspaces.length || 0}
                  checked={!!removing.cascade.workspace}
                  onChange={(v) => setRemoving((r) => (r ? { ...r, cascade: { ...r.cascade, workspace: v } } : r))}
                  items={removalToggleItems?.workspaces || []}
                  note={removalIncludesSubagents ? "Includes dependent sub-agent tree workdirs under the workspace root." : "Deletes only this agent's workspace-relative workdir."}
                />
                <CascadeOption
                  label="Dependent sub-agent tree"
                  count={removing.subagents.length}
                  checked={removing.cascade.subagents}
                  onChange={(v) => setRemoving((r) => (r ? { ...r, cascade: { ...r.cascade, subagents: v } } : r))}
                  items={removing.subagents}
                  note="Retired, not deleted, so identity and logs remain inspectable."
                />
                <ImpactList label="Sub-agent standing orders" count={removing.subagentStanding.length} items={removing.subagentStanding} note="Cleaned when Standing orders and Dependent sub-agent tree are both selected." />
                <ImpactList label="Sub-agent schedules" count={removing.subagentSchedules.length} items={removing.subagentSchedules} note="Cleaned when Schedules and Dependent sub-agent tree are both selected." />
                <ImpactList label="Sub-agent private memory" count={removing.subagentMemories.length} items={removing.subagentMemories} note="Cleaned when Private memory and Dependent sub-agent tree are both selected." />
                <ImpactList label="Sub-agent authored shared memory" count={removing.subagentAuthoredMemories.length} items={removing.subagentAuthoredMemories} note="Cleaned when Authored shared memory and Dependent sub-agent tree are both selected." />
                <ImpactList label="Sub-agent skills" count={removing.subagentSkills.length} items={removing.subagentSkills} note="Cleaned when Private skills and Dependent sub-agent tree are both selected." />
                <ImpactList label="Sub-agent config" count={removing.subagentConfigs.length} items={removing.subagentConfigs} note="Cleaned when Agent config and Dependent sub-agent tree are both selected." />
                <ImpactList label="Sub-agent workspace" count={removing.subagentWorkspaces.length} items={removing.subagentWorkspaces} note="Cleaned when Workspace and Dependent sub-agent tree are both selected." />
                <ImpactList label="Workflow references" count={(removing.workflowRefs || []).length} items={removing.workflowRefs || []} note="Retained; workflows are reusable operator-owned chains, not agent identities." />
                <ImpactList label="Sub-agent workflow references" count={(removing.subagentWorkflowRefs || []).length} items={removing.subagentWorkflowRefs || []} note="Retained with the workflow graph; inspect before deleting this identity tree." />
                <ImpactList label="Mailbox / audit history" count={(removing.mailboxMessages || []).length} items={removing.mailboxMessages || []} note="Retained for inspection; board retention controls aging." />
                <ImpactList label="Sub-agent mailbox / audit history" count={(removing.subagentMailboxMessages || []).length} items={removing.subagentMailboxMessages || []} note="Retained with the retired dependent identities." />
              </div>
              {removalPlan && (
                <div className="mt-3 rounded-md bg-card/70 px-2 py-1.5 text-[11px] text-muted">
                  <div className={cn("mb-1 font-medium", removalPlan.blockedBySubagents ? "text-bad" : "text-foreground")}>
                    {agentRemovalImpactSummary(removalPlan)}
                  </div>
                  Remove plan: delete identity
                  {removalPlan.cleanupPlan.length > 0 ? `; clean ${removalPlan.cleanupPlan.join(", ")}` : "; no dependent cleanup selected"}
                  {removalPlan.keepPlan.length > 0 ? `; keep ${removalPlan.keepPlan.join(", ")}` : ""}
                  {removalPlan.blockedBySubagents ? (
                    <span className="block pt-1 font-medium text-bad">
                      Dependent sub-agent tree must be retired with this removal before the identity can be deleted.
                    </span>
                  ) : null}
                </div>
              )}
            </div>
            <div className="flex shrink-0 gap-2">
              <Button size="sm" variant="ghost" onClick={() => setRemoving(null)}>
                Cancel
              </Button>
              <Button
                size="sm"
                variant="danger"
                disabled={busy === removing.slug || !!removalPlan?.blockedBySubagents}
                title={removalPlan?.blockedBySubagents ? "Dependent sub-agent tree must be selected for cleanup first" : "Remove identity"}
                onClick={confirmRemove}
              >
                <Trash2 className="h-3.5 w-3.5" /> Remove
              </Button>
            </div>
          </div>
        </div>
      )}

      {err && <ErrorText>{err}</ErrorText>}
      {!profiles && !err && <SkeletonList count={3} />}
      {profiles && profiles.length === 0 && !showForm && (
        <EmptyState
          icon={Bot}
          title="No agents yet"
          hint='Create a named agent — its soul, model, and budget — then run it with "agt run --agent <slug>" or delegate to it by name.'
        />
      )}

      {profiles && profiles.length > 0 && shownList.length === 0 && (
        <EmptyState icon={Users} title="No matching agents" hint="Try a different roster filter." />
      )}

      <ul className="grid gap-2.5 sm:grid-cols-2 xl:grid-cols-3">
        {visibleList.map((p) => {
          const open = editing === p.slug || activityFor === p.slug;
          const lifecycle = p.lifecycle?.mode || (p.lifecycle?.retire_on_complete ? "retire_on_complete" : "persistent");
          const cfgSummary = summarizeConfigOverrides(p.config_overrides);
          const invalidRuntimeOverrides = cfgSummary.runtime.filter((r) => !r.valid).length;
          const runtimeStatus = summarizeAgentRuntimeStatus(p.status);
          const healthIssueSummary = agentHealthIssueSummary(runtimeStatus);
          const noiseSummary = agentNoisePolicySummary(p);
          const guardianSafety = systemGuardianSafetySummary(p);
          const lifecycleSummary = agentLifecycleSummary(p);
          const privateSkillCount = skillCounts[p.slug] || 0;
          const schedulePassport = schedulePressure[p.slug] || agentSchedulePressurePassport(p, []);
          const guardianNoiseContract = systemGuardianNoiseContract(p, schedulePassport);
          const wakeIssue = rosterWakeIssue(p);
          const repairIssue = rosterRepairIssue(p);
          const avatarStatus = p.retired
            ? "retired"
            : runtimeStatus.activeRunCount > 0
              ? "running"
              : runtimeStatus.operationalState === "paused" || !p.enabled
                ? "paused"
                : "sleeping";
          const waitingMailboxCount = mailboxCounts[p.slug.toLowerCase()] || mailboxCounts[p.slug] || 0;
          const fallbacks = (p.fallbacks || []).map((m) => m.trim()).filter(Boolean);
          const toolAllowCount = (p.tool_allow || []).filter((x) => x.trim()).length;
          const toolDenyCount = (p.tool_deny || []).filter((x) => x.trim()).length;
          const configOverrideCount = Object.keys(p.config_overrides || {}).length;
          return (
          <li
            key={p.id}
            className={cn(
              "glass flex min-h-[420px] flex-col overflow-hidden rounded-xl shadow-e1 transition-[box-shadow,border-color] hover:shadow-e2",
              open && "sm:col-span-2 xl:col-span-3",
            )}
          >
            <div className="border-b border-border/70 bg-card/35 p-3">
              <div className="flex min-w-0 items-start gap-2.5">
                <button onClick={() => openAgent(p.slug)} title="Open identity page" className="shrink-0">
                  <AgentAvatar slug={p.slug} name={p.name} size={38} status={avatarStatus} />
                </button>
                <div className="min-w-0 flex-1">
                  <div className="flex min-w-0 items-center gap-2">
                    <button
                      onClick={() => openAgent(p.slug)}
                      title="Open identity page"
                      className={cn("truncate font-mono text-sm font-semibold hover:underline", p.retired ? "text-muted line-through" : "text-foreground")}
                    >
                      {p.slug}
                    </button>
                    {p.retired ? (
                      <Badge variant="default" className="inline-flex shrink-0 items-center gap-1 text-muted">
                        <Skull className="h-3 w-3" /> graveyard
                      </Badge>
                    ) : (
                      <Badge variant={p.enabled ? "good" : "default"} className="shrink-0">
                        {p.enabled ? "enabled" : "paused"}
                      </Badge>
                    )}
                  </div>
                  <div className="mt-0.5 min-h-[1rem] truncate text-xs text-muted">
                    {p.name && p.name !== p.slug ? p.name : p.description || "identity profile"}
                  </div>
                </div>
              </div>
              {p.system && (
                <div
                  className={cn(
                    "mt-2 flex min-w-0 items-start gap-1.5 rounded-md border px-2 py-1.5 text-[11px]",
                    guardianNoiseContract.tone === "warn"
                      ? "border-warn/35 bg-warn/10 text-warn"
                      : guardianNoiseContract.tone === "good"
                        ? "border-good/30 bg-good/5 text-good"
                        : "border-border bg-card/45 text-muted",
                  )}
                  title={guardianNoiseContract.detail}
                >
                  <Megaphone className="mt-0.5 h-3 w-3 shrink-0" />
                  <span className="shrink-0 font-medium">{guardianNoiseContract.label}</span>
                  <span className="min-w-0 truncate text-muted">{guardianNoiseContract.detail}</span>
                </div>
              )}
              <div className="mt-2 flex flex-wrap items-center gap-1.5">
                <AgentKindBadge profile={p} />
                {privateSkillCount > 0 && (
                  <IdentityPill className="bg-accent/10 text-accent" title={`${privateSkillCount} skill(s) private to this agent`}>
                    <Sparkles className="h-2.5 w-2.5" /> {privateSkillCount} skill{privateSkillCount === 1 ? "" : "s"}
                  </IdentityPill>
                )}
                {noiseSummary && <IdentityPill title={noiseSummary}>quiet policy</IdentityPill>}
                {guardianSafety && (
                  <IdentityPill className={cn(guardianSafety.startsWith("review:") ? "bg-warn/10 text-warn" : "bg-good/10 text-good")} title={guardianSafety}>
                    system safety
                  </IdentityPill>
                )}
                {p.self_repair?.enabled && <IdentityPill>self-repair</IdentityPill>}
                {p.trust_ceiling && p.trust_ceiling !== "L4" && <IdentityPill>ceiling {p.trust_ceiling}</IdentityPill>}
                {runtimeStatus.healthText && (
                  <IdentityPill
                    className={cn(runtimeStatus.healthTone === "bad" && "bg-bad/10 text-bad", runtimeStatus.healthTone === "muted" && "bg-panel text-muted")}
                    title={healthIssueSummary || undefined}
                  >
                    {runtimeStatus.healthText}
                  </IdentityPill>
                )}
                {runtimeStatus.repairText && (
                  <IdentityPill
                    className={cn(
                      runtimeStatus.repairTone === "accent" && "bg-accent/10 text-accent",
                      runtimeStatus.repairTone === "good" && "bg-good/10 text-good",
                      runtimeStatus.repairTone === "bad" && "bg-bad/10 text-bad",
                    )}
                    title={runtimeStatus.repairDetail || (runtimeStatus.nextRepairEligibleMs ? `next eligible ${new Date(runtimeStatus.nextRepairEligibleMs).toLocaleString()}` : "autonomous self-repair state")}
                  >
                    {runtimeStatus.repairText}
                  </IdentityPill>
                )}
                {runtimeStatus.repairKindText && (
                  <IdentityPill
                    className={cn(
                      runtimeStatus.repairKindTone === "accent" && "bg-accent/10 text-accent",
                      runtimeStatus.repairKindTone === "warn" && "bg-warn/10 text-warn",
                    )}
                    title="repair family"
                  >
                    {runtimeStatus.repairKindText}
                  </IdentityPill>
                )}
                {runtimeStatus.repairIncidentText && runtimeStatus.repairIncidentId && (
                  <button
                    type="button"
                    onClick={() => openIncident(runtimeStatus.repairIncidentId!)}
                    className="inline-flex items-center gap-1 rounded-md border border-border bg-warn/10 px-1.5 py-0.5 text-xs font-medium text-warn transition-colors hover:border-warn/50 hover:bg-warn/15"
                    title={runtimeStatus.repairIncidentDetail || "Open repair incident"}
                  >
                    {runtimeStatus.repairIncidentText}
                  </button>
                )}
                {runtimeStatus.routingText && (
                  <IdentityPill
                    className={cn(runtimeStatus.routingTone === "bad" && "bg-bad/10 text-bad")}
                    title={runtimeStatus.routingDetail || "recent model-chain fallback pressure"}
                  >
                    {runtimeStatus.routingText}
                  </IdentityPill>
                )}
                {runtimeStatus.retryText && (
                  <IdentityPill
                    className={cn(runtimeStatus.retryTone === "bad" && "bg-bad/10 text-bad")}
                    title={runtimeStatus.retryDetail || "recent whole-run retry pressure"}
                  >
                    {runtimeStatus.retryText}
                  </IdentityPill>
                )}
                {runtimeStatus.escalationText && (
                  <IdentityPill
                    className={cn(
                      runtimeStatus.escalationTone === "bad" && "bg-bad/10 text-bad",
                      runtimeStatus.escalationTone === "accent" && "bg-accent/10 text-accent",
                    )}
                    title="open owner/escalation workload"
                  >
                    {runtimeStatus.escalationText}
                  </IdentityPill>
                )}
                {waitingMailboxCount > 0 && (
                  <IdentityPill className="bg-accent/10 text-accent" title={`${waitingMailboxCount} mailbox message${waitingMailboxCount === 1 ? "" : "s"} waiting for this agent`}>
                    <Mail className="h-2.5 w-2.5" /> inbox {waitingMailboxCount}
                  </IdentityPill>
                )}
                {runtimeStatus.liveText && (
                  <IdentityPill
                    className={cn(runtimeStatus.liveTone === "accent" && "bg-accent/10 text-accent")}
                    title={[
                      runtimeStatus.liveDetail || "active run",
                      runtimeStatus.activeStartedMs ? `since ${new Date(runtimeStatus.activeStartedMs).toLocaleString()}` : "",
                      runtimeStatus.activeLastEventMs ? `last ${new Date(runtimeStatus.activeLastEventMs).toLocaleString()}` : "",
                    ].filter(Boolean).join(" · ")}
                  >
                    {runtimeStatus.liveText}
                  </IdentityPill>
                )}
                {!runtimeStatus.liveText && runtimeStatus.operationalText && (
                  <IdentityPill
                    className={cn(
                      runtimeStatus.operationalState === "paused" && "bg-warn/10 text-warn",
                      runtimeStatus.operationalState === "sleeping" && "bg-panel text-muted",
                    )}
                    title={runtimeStatus.lastActivitySummary || "agent operational state"}
                  >
                    {runtimeStatus.operationalText}
                  </IdentityPill>
                )}
                {runtimeStatus.wakeText && (
                  <IdentityPill
                    className={cn(runtimeStatus.wakeTone === "accent" && "bg-accent/10 text-accent")}
                    title={runtimeStatus.nextWakeMs ? `${runtimeStatus.wakeDetail || "wake bindings"} · next ${new Date(runtimeStatus.nextWakeMs).toLocaleString()}` : runtimeStatus.wakeDetail || "wake bindings"}
                  >
                    {runtimeStatus.wakeText}
                  </IdentityPill>
                )}
                {(p.config_overrides && Object.keys(p.config_overrides).length > 0) && (
                  <IdentityPill className={cn(invalidRuntimeOverrides > 0 && "bg-bad/10 text-bad")} title={invalidRuntimeOverrides > 0 ? `${invalidRuntimeOverrides} invalid runtime override(s)` : "agent config overrides"}>
                    cfg {Object.keys(p.config_overrides).length}{invalidRuntimeOverrides > 0 ? ` !${invalidRuntimeOverrides}` : ""}
                  </IdentityPill>
                )}
                {lifecycle !== "persistent" && <IdentityPill>{lifecycleSummary}</IdentityPill>}
              </div>
            </div>

            <div className="flex flex-1 flex-col p-3">
              {p.description && p.name && p.name !== p.slug && <div className="text-xs text-muted">{p.description}</div>}
              {p.soul && (
                <div className={cn("mt-2 rounded-md bg-panel px-2 py-1.5 text-xs text-muted whitespace-pre-wrap", !open && "line-clamp-3")}>
                  {p.soul}
                </div>
              )}
              <Disclosure className="mt-2" summary={<span className="text-xs text-muted">details</span>}>
                <div className="rounded-md border border-border/60 bg-panel/40 p-2 text-xs">
                  <KeyValue
                    pairs={([
                      p.model ? ["model", <span key="model" className="font-mono">{p.model}</span>] : null,
                      fallbacks.length > 0
                        ? ["fallbacks", <span key="fallbacks" className="font-mono">{fallbacks.join(" → ")}</span>]
                        : null,
                      p.task_type?.trim() ? ["task type", p.task_type.trim()] : null,
                      (p.max_cost_mc || 0) > 0 ? ["per-run cap", money(p.max_cost_mc)] : null,
                      (p.max_daily_mc || 0) > 0 ? ["daily cap", money(p.max_daily_mc)] : null,
                      p.owner_agent?.trim() ? ["owner", <span key="owner" className="font-mono">{p.owner_agent.trim()}</span>] : null,
                      p.parent_agent?.trim() ? ["parent", <span key="parent" className="font-mono">{p.parent_agent.trim()}</span>] : null,
                      p.memory_scope?.trim() ? ["memory scope", <span key="scope" className="font-mono">{p.memory_scope.trim()}</span>] : null,
                      p.workdir?.trim() ? ["workdir", <span key="workdir" className="font-mono">{p.workdir.trim()}</span>] : null,
                      toolAllowCount > 0 || toolDenyCount > 0
                        ? ["tools", [
                            toolAllowCount > 0 ? `${toolAllowCount} allow` : "",
                            toolDenyCount > 0 ? `${toolDenyCount} deny` : "",
                          ].filter(Boolean).join(" · ")]
                        : null,
                      configOverrideCount > 0 ? ["config overrides", String(configOverrideCount)] : null,
                      p.trust_ceiling?.trim() ? ["trust ceiling", p.trust_ceiling.trim()] : null,
                      p.retired && p.retired_reason?.trim() ? ["retired reason", p.retired_reason.trim()] : null,
                    ] as ([string, ReactNode] | null)[]).filter((pair): pair is [string, ReactNode] => pair !== null)}
                  />
                </div>
              </Disclosure>
            </div>

            <div className="mt-auto flex items-center justify-between gap-2 border-t border-border/70 bg-panel/30 px-3 py-2">
              <span className="truncate font-mono text-xs text-muted">{p.id}</span>
              <span className="flex shrink-0 items-center gap-1">
                <Button
                  size="sm"
                  variant="ghost"
                  aria-label={`Identity page for ${p.slug}`}
                  title="Open the full identity page (everything about this agent)"
                  onClick={() => openAgent(p.slug)}
                >
                  <IdCard className="h-3.5 w-3.5" />
                </Button>
                <Button
                  size="sm"
                  variant="ghost"
                  aria-label={`Activity for ${p.slug}`}
                  title="What this agent did (runs, consults, memory, messages)"
                  onClick={() => setActivityFor(activityFor === p.slug ? null : p.slug)}
                >
                  <Activity className="h-3.5 w-3.5" />
                </Button>
                <Button
                  size="sm"
                  variant="ghost"
                  disabled={busy === p.slug || !!wakeIssue}
                  aria-label={`Wake ${p.slug}`}
                  title={wakeIssue || "Wake this agent now"}
                  onClick={() => wakeAgent(p.slug)}
                >
                  <Zap className="h-3.5 w-3.5" />
                </Button>
                <Button
                  size="sm"
                  variant="ghost"
                  disabled={busy === `schedule:${p.slug}` || schedulePassport.frequentIds.length === 0}
                  aria-label={`Pause frequent schedules for ${p.slug}`}
                  title={schedulePassport.frequentIds.length > 0 ? `Pause ${schedulePassport.frequentIds.length} frequent schedule${schedulePassport.frequentIds.length === 1 ? "" : "s"}` : "No frequent schedules"}
                  onClick={() => pauseFrequentSchedules(p.slug, schedulePassport.frequentIds)}
                >
                  <CalendarClock className="h-3.5 w-3.5" />
                </Button>
                <Button
                  size="sm"
                  variant="ghost"
                  disabled={busy === p.slug || !!repairIssue}
                  aria-label={`Repair ${p.slug}`}
                  title={repairIssue || "Request a governed doctor/repair run for this agent"}
                  onClick={() => repairAgent(p.slug)}
                >
                  <Wrench className="h-3.5 w-3.5" />
                </Button>
                <Button
                  size="sm"
                  variant="ghost"
                  aria-label={`Edit ${p.slug}`}
                  onClick={() => setEditing(p.slug)}
                >
                  <Pencil className="h-3.5 w-3.5" />
                </Button>
                {!p.retired && (
                  <Button
                    size="sm"
                    variant="ghost"
                    disabled={busy === p.slug}
                    aria-label={p.enabled ? `Pause ${p.slug}` : `Resume ${p.slug}`}
                    onClick={() => setAgentEnabled(p.slug, !p.enabled)}
                  >
                    {p.enabled ? <Pause className="h-3.5 w-3.5" /> : <Play className="h-3.5 w-3.5" />}
                  </Button>
                )}
                {p.retired ? (
                  <Button
                    size="sm"
                    variant="ghost"
                    disabled={busy === p.slug}
                    aria-label={`Revive ${p.slug}`}
                    title="Revive from the graveyard"
                    onClick={() => revive(p.slug)}
                  >
                    <ArchiveRestore className="h-3.5 w-3.5" />
                  </Button>
                ) : (
                  <Button
                    size="sm"
                    variant="ghost"
                    disabled={busy === p.slug}
                    aria-label={`Retire ${p.slug}`}
                    title="Retire to the graveyard"
                    onClick={() => retire(p.slug)}
                  >
                    <Archive className="h-3.5 w-3.5" />
                  </Button>
                )}
                {!p.system && (
                  <Button
                    size="sm"
                    variant="ghost"
                    disabled={busy === p.slug}
                    aria-label={`Remove ${p.slug}`}
                    onClick={() => removeAgent(p.slug)}
                  >
                    <Trash2 className="h-3.5 w-3.5" />
                  </Button>
                )}
              </span>
            </div>
            {editing === p.slug && (
              <RosterModal title={`Edit ${p.slug}`} icon={Pencil} onClose={() => setEditing(null)}>
                <EditAgentForm
                  profile={p}
                  onSaved={(slug) => {
                    setEditing(null);
                    ui.toast(`agent ${slug} updated`, "success");
                    reload();
                  }}
                  onError={(msg) => ui.toast(msg, "error")}
                />
              </RosterModal>
            )}
            {activityFor === p.slug && (
              <div className="border-t border-border/70 p-3">
                <AgentActivity slug={p.slug} initialOpenRun={activityFocus[p.slug]} initialTab={activityFocus[p.slug] ? "runs" : "activity"} />
              </div>
            )}
          </li>
          );
        })}
      </ul>
      {shownList.length > ROSTER_CARD_WINDOW && (
        <LoadMoreFooter
          hasMore={cardWin < shownList.length}
          loadingMore={false}
          onLoadMore={() => setCardWin((w) => w + ROSTER_CARD_WINDOW)}
          pageSize={Math.min(ROSTER_CARD_WINDOW, Math.max(1, shownList.length - cardWin))}
          label="agents"
        />
      )}
    </Page>
  );
}
