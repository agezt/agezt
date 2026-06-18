import { useEffect, useMemo, useState } from "react";
import {
  ChevronLeft,
  LifeBuoy,
  ShieldAlert,
  Bot,
  Archive,
  ArchiveRestore,
  Pause,
  Play,
  Send,
  Wrench,
} from "lucide-react";
import { getJSON, postAction, postJSON } from "@/lib/api";
import { useEvents, type AgentEvent } from "@/lib/events";
import { classifyAlert, type RankedAlert } from "@/lib/alerts";
import {
  autonomyEventMatches,
  doctorIncidentTreeOpsSummary,
  doctorIncidentTrees,
  type AutonomyItem,
} from "@/lib/autonomy";
import {
  incidentActionContext,
  incidentDelegateCandidates,
  incidentForceChainPresets,
  previewIncidentForceChain,
  incidentResolutionDelegateDraft,
  incidentResolutionForceDraft,
  incidentResolutionHistory,
  incidentResolutionPresets,
  incidentMatches,
  incidentMetaFromAutonomy,
  incidentMetaFromEvent,
  incidentRootId,
  validateIncidentDelegateTarget,
} from "@/lib/incidents";
import { incidentEventSummary } from "@/lib/incidentevents";
import { openAgent } from "@/lib/agentnav";
import {
  summarizeAgentRuntimeStatus,
  summarizeEscalations,
  type AgentEscalation,
} from "@/lib/agentdetail";
import type { AgentProfile } from "@/views/Roster";
import { Button } from "@/components/ui/button";
import { SkeletonList } from "@/components/ui/skeleton";
import { EmptyState } from "@/components/ui/empty";
import { ErrorText } from "@/components/JsonView";
import { DoctorIncidentTrees } from "@/components/DoctorIncidentTrees";
import { IncidentBadges, incidentPhaseBadgeClass } from "@/components/IncidentBadges";
import { PageHeader } from "@/components/ui/page-header";
import { AgentRepair } from "@/components/AgentRepair";
import { useUI } from "@/components/ui/feedback";
import { fmtTime } from "@/lib/utils";

interface IncidentAlert extends RankedAlert {}

export function IncidentPage({
  incidentId,
  onNavigate,
}: {
  incidentId: string;
  onNavigate: (view: string) => void;
}) {
  const ui = useUI();
  const { subscribe } = useEvents();
  const [items, setItems] = useState<AutonomyItem[] | null>(null);
  const [events, setEvents] = useState<AgentEvent[] | null>(null);
  const [profiles, setProfiles] = useState<AgentProfile[] | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [busy, setBusy] = useState<string | null>(null);
  const [noteText, setNoteText] = useState("");
  const [delegateTo, setDelegateTo] = useState("");
  const [delegateEscalations, setDelegateEscalations] = useState<AgentEscalation[] | null>(null);
  const [forceTaskType, setForceTaskType] = useState("");
  const [forceChainText, setForceChainText] = useState("");
  const [showRepair, setShowRepair] = useState(false);

  async function load() {
    const [a, j, p] = await Promise.allSettled([
      getJSON<{ items?: AutonomyItem[] }>("/api/autonomy", { limit: "250" }),
      getJSON<{ events?: AgentEvent[] }>("/api/journal", { kind: "info", limit: "800" }),
      getJSON<{ profiles?: AgentProfile[] }>("/api/agents"),
    ]);
    if (a.status === "fulfilled") setItems(a.value.items || []);
    else setErr((a.reason as Error)?.message || "failed to load autonomy feed");
    if (j.status === "fulfilled") setEvents(j.value.events || []);
    if (p.status === "fulfilled") setProfiles(p.value.profiles || []);
  }

  useEffect(() => {
    void load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [incidentId]);

  useEffect(() => {
    let t: ReturnType<typeof setTimeout> | undefined;
    const off = subscribe((e) => {
      if (!autonomyEventMatches(e)) return;
      if (!incidentMatches(incidentMetaFromEvent(e), incidentId)) return;
      if (t) clearTimeout(t);
      t = setTimeout(() => void load(), 700);
    });
    return () => {
      if (t) clearTimeout(t);
      off();
    };
  }, [incidentId, subscribe]);

  const matchedItems = useMemo(
    () => (items || []).filter((row) => row.category === "doctor" && incidentMatches(incidentMetaFromAutonomy(row), incidentId)),
    [items, incidentId],
  );
  const rootId = useMemo(() => {
    const first = matchedItems[0];
    return first ? incidentRootId(incidentMetaFromAutonomy(first)) : incidentId;
  }, [matchedItems, incidentId]);
  const treeItems = useMemo(
    () =>
      (items || []).filter(
        (row) =>
          row.category === "doctor" &&
          incidentRootId(incidentMetaFromAutonomy(row)) === rootId,
      ),
    [items, rootId],
  );
  const trees = useMemo(() => doctorIncidentTrees(treeItems, 12), [treeItems]);
  const treeOps = useMemo(
    () => (trees[0] ? doctorIncidentTreeOpsSummary(trees[0]) : null),
    [trees],
  );
  const focus = useMemo(
    () =>
      treeItems.find(
        (row) =>
          incidentMetaFromAutonomy(row).incidentId === incidentId ||
          incidentMetaFromAutonomy(row).rootIncidentId === incidentId,
      ),
    [treeItems, incidentId],
  );
  const alerts = useMemo(() => {
    const out: IncidentAlert[] = [];
    for (const e of events || []) {
      if (e.subject !== "doctor.auto_repair") continue;
      if (!incidentMatches(incidentMetaFromEvent(e), rootId)) continue;
      const a = classifyAlert(e);
      if (!a) continue;
      out.push({
        ...a,
        id: e.id || `${e.kind}-${e.seq || out.length}`,
        tsMs: e.ts_unix_ms,
        correlationId: e.correlation_id,
        incidentId: incidentMetaFromEvent(e).incidentId,
        rootIncidentId: incidentMetaFromEvent(e).rootIncidentId,
      });
    }
    out.sort((a, b) => (b.tsMs || 0) - (a.tsMs || 0));
    return out.slice(0, 8);
  }, [events, rootId]);
  const resolutionRows = useMemo(
    () => incidentResolutionHistory(events, rootId).slice(0, 6),
    [events, rootId],
  );
  const agents = useMemo(() => {
    const slugs = new Set<string>();
    for (const row of treeItems) {
      for (const slug of [row.agent, row.target_agent, row.delegate_to, row.delegated_by, row.root_agent]) {
        if (slug) slugs.add(String(slug));
      }
    }
    return Array.from(slugs).map((slug) => {
      const found = (profiles || []).find((p) => p.slug === slug);
      return { slug, name: found?.name };
    });
  }, [profiles, treeItems]);
  const ops = useMemo(
    () => incidentActionContext(treeItems, profiles, events),
    [treeItems, profiles, events],
  );
  const resolutionPresets = useMemo(() => incidentResolutionPresets(ops), [ops]);
  const rootProfile = ops.rootProfile
    ? ((profiles || []).find((row) => row.slug === ops.rootProfile?.slug) as
        | AgentProfile
        | undefined)
    : undefined;
  const noteTarget = ops.ownerSlug || rootProfile?.parent_agent || rootProfile?.owner_agent || "";
  const noteTopic = `incident:${rootId}`;
  const incidentLineage = {
    incident_id: focus?.incident_id || incidentId,
    root_incident_id: rootId,
    parent_incident_id: focus?.parent_incident_id || "",
  };
  const canWakeRoot =
    !rootProfile?.retired &&
    !!rootProfile?.enabled &&
    !ops.preferOwnerWake;
  const canWakeOwner =
    !rootProfile?.retired &&
    !!ops.ownerSlug &&
    !!ops.ownerProfile?.enabled &&
    !ops.ownerProfile?.retired &&
    ops.ownerProfile?.direct_callable !== false;
  const repairReason = [
    `operator requested doctor rerun for incident ${rootId}`,
    focus?.detail || "",
    ops.configIssues[0] || "",
  ]
    .filter(Boolean)
    .join(" · ");
  const effectiveForceTaskType = forceTaskType.trim() || ops.routingTaskType || rootProfile?.task_type || "";
  const exhaustedChain = ops.routingTaskModelChain.join(" → ");
  const delegateValidation = useMemo(
    () =>
      validateIncidentDelegateTarget(delegateTo, {
        rootSlug: ops.rootSlug,
        ownerSlug: ops.ownerSlug,
        profiles,
      }),
    [delegateTo, ops.ownerSlug, ops.rootSlug, profiles],
  );
  const delegateCandidates = useMemo(
    () =>
      incidentDelegateCandidates(profiles, {
        rootSlug: ops.rootSlug,
        ownerSlug: ops.ownerSlug,
        preferredSlugs: treeItems.flatMap((row) =>
          [row.target_agent, row.delegate_to, row.delegated_by].filter(Boolean) as string[],
        ),
      }),
    [profiles, ops.ownerSlug, ops.rootSlug, treeItems],
  );
  const validDelegateCandidates = delegateCandidates.filter((row) => row.valid).slice(0, 6);
  const invalidDelegateCandidates = delegateCandidates.filter((row) => !row.valid).slice(0, 4);
  const selectedDelegateCandidate = useMemo(
    () => delegateCandidates.find((row) => row.slug === delegateValidation.normalizedTarget),
    [delegateCandidates, delegateValidation.normalizedTarget],
  );
  const selectedDelegateProfile = useMemo(
    () => (profiles || []).find((row) => row.slug === delegateValidation.normalizedTarget),
    [profiles, delegateValidation.normalizedTarget],
  );
  const selectedDelegateStatus = useMemo(
    () => summarizeAgentRuntimeStatus(selectedDelegateProfile?.status),
    [selectedDelegateProfile],
  );
  const selectedDelegateEscalationSummary = useMemo(
    () => summarizeEscalations(delegateEscalations),
    [delegateEscalations],
  );
  const forcePresets = useMemo(
    () => incidentForceChainPresets(resolutionRows).slice(0, 4),
    [resolutionRows],
  );
  const forcePresetViews = useMemo(
    () =>
      forcePresets.map((preset) => ({
        ...preset,
        preview: previewIncidentForceChain(
          preset.taskType,
          ops.routingTaskModelChain,
          preset.chainText,
        ),
      })),
    [forcePresets, ops.routingTaskModelChain],
  );
  const forcePreview = useMemo(
    () =>
      previewIncidentForceChain(
        effectiveForceTaskType,
        ops.routingTaskModelChain,
        forceChainText,
      ),
    [effectiveForceTaskType, forceChainText, ops.routingTaskModelChain],
  );

  useEffect(() => {
    setDelegateTo("");
    setDelegateEscalations(null);
    setForceTaskType("");
    setForceChainText("");
  }, [rootId]);

  useEffect(() => {
    let alive = true;
    const target = delegateValidation.valid
      ? delegateValidation.normalizedTarget
      : "";
    if (!target) {
      setDelegateEscalations(null);
      return;
    }
    getJSON<{ escalations?: AgentEscalation[] }>("/api/agents/escalations", {
      ref: target,
      limit: "12",
    })
      .then((res) => {
        if (alive) setDelegateEscalations(res.escalations || []);
      })
      .catch(() => {
        if (alive) setDelegateEscalations([]);
      });
    return () => {
      alive = false;
    };
  }, [delegateValidation.normalizedTarget, delegateValidation.valid]);

  async function mutate(
    key: string,
    success: string,
    fn: () => Promise<unknown>,
  ) {
    setBusy(key);
    setErr(null);
    try {
      await fn();
      ui.toast(success, "success");
      await load();
    } catch (e) {
      const message = (e as Error)?.message || "action failed";
      setErr(message);
      ui.toast(message, "error");
    } finally {
      setBusy(null);
    }
  }

  function wakeIntent(target: string): string {
    const bits = [
      "Manual incident wake.",
      `You are ${target}.`,
      `Incident root ${rootId}.`,
      ops.rootSlug ? `Broken/root agent: ${ops.rootSlug}.` : "",
      focus?.detail ? `Current incident state: ${focus.detail}.` : "",
      ops.configIssues.length > 0
        ? `Known config issues: ${ops.configIssues.slice(0, 3).join("; ")}.`
        : "",
      "Inspect your mailbox, standing instructions, open tasklist, and current health context. Take the next concrete recovery step and then stop.",
    ];
    return bits.filter(Boolean).join(" ");
  }

  async function toggleEnabled(enabled: boolean) {
    if (!ops.rootSlug || !rootProfile || rootProfile.retired) return;
    await mutate(
      `enable:${ops.rootSlug}`,
      enabled ? `${ops.rootSlug} resumed` : `${ops.rootSlug} paused`,
      () =>
        postAction("/api/agents/enable", {
          ref: ops.rootSlug!,
          enabled: enabled ? "true" : "false",
        }),
    );
  }

  async function toggleRetired() {
    if (!ops.rootSlug || !rootProfile) return;
    if (rootProfile.retired) {
      await mutate(
        `revive:${ops.rootSlug}`,
        `${ops.rootSlug} revived`,
        () => postAction("/api/agents/revive", { ref: ops.rootSlug! }),
      );
      return;
    }
    const ok = await ui.confirm({
      title: `Retire ${ops.rootSlug}?`,
      message:
        "This moves the root agent to the graveyard. Incident lineage stays readable, but the agent will stop waking until revived.",
      confirmLabel: "Retire",
      danger: true,
    });
    if (!ok) return;
    await mutate(
      `retire:${ops.rootSlug}`,
      `${ops.rootSlug} retired`,
      () =>
        ops.humanRequired
          ? postJSON("/api/agents/resolve", {
              ref: ops.rootSlug,
              resolution: "retired",
              summary:
                ops.policySummary ||
                `operator retired ${ops.rootSlug} from exhausted routing incident ${rootId}`,
              ...incidentLineage,
            })
          : postAction("/api/agents/retire", {
              ref: ops.rootSlug!,
              reason: `operator retired from ${noteTopic}`,
            }),
    );
  }

  async function sendOperatorNote() {
    const text = noteText.trim();
    if (!noteTarget || !text) return;
    await mutate(
      `note:${noteTarget}`,
      `note sent to ${noteTarget}`,
      () =>
        postJSON("/api/board/send", {
          from: "operator",
          to: noteTarget,
          topic: noteTopic,
          text,
          help: true,
        }),
    );
    setNoteText("");
  }

  function applyResolutionPreset(key: string) {
    const preset = resolutionPresets.find((row) => row.key === key);
    if (!preset) return;
    setNoteText(preset.text);
  }

  function branchForceChain(row: (typeof resolutionRows)[number]) {
    const draft = incidentResolutionForceDraft(row);
    if (!draft) return;
    setForceTaskType(draft.taskType);
    setForceChainText(draft.chainText);
    setNoteText(draft.summary);
  }

  function reuseDelegate(row: (typeof resolutionRows)[number]) {
    const draft = incidentResolutionDelegateDraft(row);
    if (!draft) return;
    setDelegateTo(draft.delegateTo);
    setNoteText(draft.summary);
  }

  function useForcePreset(taskType: string, chainText: string, summary: string) {
    setForceTaskType(taskType);
    setForceChainText(chainText);
    setNoteText(summary);
  }

  async function triggerRepair() {
    if (!ops.rootSlug || !rootProfile || rootProfile.retired) return;
    await mutate(
      `repair:${ops.rootSlug}`,
      `doctor rerun accepted for ${ops.rootSlug}`,
      () =>
        postJSON("/api/agents/repair", {
          ref: ops.rootSlug,
          reason: repairReason,
          ...incidentLineage,
        }),
    );
  }

  async function resolveIncident(resolution: "paused" | "retired") {
    if (!ops.rootSlug || !rootProfile || rootProfile.retired) return;
    const success =
      resolution === "paused"
        ? `${ops.rootSlug} paused via incident resolution`
        : `${ops.rootSlug} retired via incident resolution`;
    await mutate(`resolve:${resolution}:${ops.rootSlug}`, success, () =>
      postJSON("/api/agents/resolve", {
        ref: ops.rootSlug,
        resolution,
        summary:
          ops.policySummary ||
          `operator applied ${resolution} to ${ops.rootSlug} from incident ${rootId}`,
        ...incidentLineage,
      }),
    );
  }

  async function resolveDelegated() {
    if (!ops.rootSlug || !rootProfile || rootProfile.retired) return;
    if (!delegateValidation.valid) {
      setErr(delegateValidation.reason || "delegate target required");
      return;
    }
    const target = delegateValidation.normalizedTarget;
    await mutate(`resolve:delegated:${ops.rootSlug}`, `delegated ${ops.rootSlug} to ${target}`, () =>
      postJSON("/api/agents/resolve", {
        ref: ops.rootSlug,
        resolution: "delegated",
        delegate_to: target,
        summary:
          noteText.trim() ||
          `operator delegated exhausted routing incident for ${ops.rootSlug} to ${target}`,
        ...incidentLineage,
      }),
    );
  }

  async function resolveForceChain() {
    if (!ops.rootSlug || !rootProfile || rootProfile.retired) return;
    if (!forcePreview.valid) {
      setErr(forcePreview.reason || "force_chain model chain required");
      return;
    }
    const taskType = forcePreview.taskType;
    const taskModelChain = forcePreview.proposedChain;
    await mutate(`resolve:force_chain:${ops.rootSlug}`, `forced new ${taskType} chain for ${ops.rootSlug}`, () =>
      postJSON("/api/agents/resolve", {
        ref: ops.rootSlug,
        resolution: "force_chain",
        task_type: taskType,
        task_model_chain: taskModelChain,
        summary:
          noteText.trim() ||
          `operator forced a new ${taskType} chain for ${ops.rootSlug} from incident ${rootId}`,
        ...incidentLineage,
      }),
    );
  }

  async function wakeAgent(target: string) {
    if (!target) return;
    await mutate(
      `wake:${target}`,
      `wake accepted for ${target}`,
      () =>
        postJSON("/api/agents/wake", {
          ref: target,
          reason: `incident ${rootId} needs active ownership`,
          intent: wakeIntent(target),
          ...incidentLineage,
        }),
    );
  }

  const back = (
    <Button variant="ghost" size="sm" onClick={() => onNavigate("autonomy")}>
      <ChevronLeft className="size-3.5" /> Autonomy
    </Button>
  );

  if (!items && !err) {
    return (
      <div className="space-y-3">
        {back}
        <SkeletonList count={5} lines={2} />
      </div>
    );
  }
  if (err && !items) {
    return (
      <div className="space-y-3">
        {back}
        <ErrorText>{err}</ErrorText>
      </div>
    );
  }
  if (treeItems.length === 0) {
    return (
      <div className="space-y-3">
        {back}
        <EmptyState
          icon={LifeBuoy}
          title={`No incident “${incidentId}”`}
          hint="This incident may have aged out of the current feed window."
        />
      </div>
    );
  }

  return (
    <div className="flex min-h-0 flex-col gap-3">
      {back}
      <PageHeader
        icon={LifeBuoy}
        title={focus?.title || "Doctor incident"}
        description={
          <span className="text-xs text-muted">
            {focus?.detail || `incident ${incidentId}`}
          </span>
        }
        actions={
          <span className="font-mono text-[11px] text-muted">
            {rootId}
          </span>
        }
      />

      {treeOps && (
        <div className="glass rounded-xl p-3">
          <div className="flex flex-wrap items-center gap-2 text-xs">
            <span className="font-semibold uppercase tracking-wider text-muted">
              repair ops
            </span>
            <span className={incidentPhaseBadgeClass(treeOps.tone, true)}>
              {treeOps.label}
            </span>
            <span className="text-muted">{treeOps.detail}</span>
            <span className="ml-auto font-mono text-[10px] text-muted opacity-70">
              {rootId}
            </span>
          </div>
        </div>
      )}

      <div className="grid gap-3 lg:grid-cols-[minmax(0,2fr)_minmax(280px,1fr)]">
        <div className="space-y-3">
          <div className="glass rounded-xl p-3">
            <div className="mb-2 text-xs font-semibold uppercase tracking-wider text-muted">
              Repair incident tree
            </div>
            <DoctorIncidentTrees trees={trees} />
          </div>
          <div className="glass rounded-xl p-3">
            <div className="mb-2 flex items-center gap-2 text-xs font-semibold uppercase tracking-wider text-muted">
              <ShieldAlert className="size-3.5" /> Related alerts
            </div>
            {alerts.length === 0 ? (
              <div className="text-xs text-muted">no alert-level doctor failure in this incident</div>
            ) : (
              <ul className="space-y-1.5">
                {alerts.map((row) => (
                  <li key={row.id} className="rounded-lg border border-border bg-panel/40 px-2.5 py-2">
                    <div className="flex items-center gap-2 text-xs">
                      <span className="font-medium">{row.title}</span>
                      <span className="ml-auto font-mono text-[10px] text-muted opacity-70">
                        {fmtTime(row.tsMs)}
                      </span>
                    </div>
                    {row.detail && (
                      <div className="mt-1 text-[11px] text-muted">{row.detail}</div>
                    )}
                  </li>
                ))}
              </ul>
            )}
          </div>
          <div className="glass rounded-xl p-3">
            <div className="mb-2 flex items-center gap-2 text-xs font-semibold uppercase tracking-wider text-muted">
              <Wrench className="size-3.5" /> Resolution history
            </div>
            {resolutionRows.length === 0 ? (
              <div className="text-xs text-muted">no operator resolution recorded for this incident yet</div>
            ) : (
              <ul className="space-y-1.5">
                {resolutionRows.map((row) => (
                  <li key={row.id} className="rounded-lg border border-border bg-panel/40 px-2.5 py-2">
                    <div className="flex items-center gap-2">
                      <div className="flex flex-wrap items-center gap-1.5">
                        <IncidentBadges item={row} mono />
                      </div>
                      <span className="ml-auto font-mono text-[10px] text-muted opacity-70">
                        {fmtTime(row.tsMs)}
                      </span>
                    </div>
                    <div className="mt-1 text-[11px] text-foreground/90">
                      {incidentEventSummary({
                        subject: row.subject,
                        payload: row.payload,
                      })}
                    </div>
                    {(row.resolutionSummary || row.delegateTo) && (
                      <div className="mt-1 text-[11px] text-muted">
                        {[row.resolutionSummary, row.delegateTo ? `delegate ${row.delegateTo}` : ""]
                          .filter(Boolean)
                          .join(" · ")}
                      </div>
                    )}
                    <div className="mt-2 flex flex-wrap gap-2">
                      {ops.rootSlug && (
                        <Button size="sm" variant="ghost" onClick={() => openAgent(ops.rootSlug!)}>
                          <Bot className="size-3.5" /> Root
                        </Button>
                      )}
                      {row.delegateTo && (
                        <>
                          <Button size="sm" variant="ghost" onClick={() => openAgent(row.delegateTo!)}>
                            <Bot className="size-3.5" /> Delegate
                          </Button>
                          {ops.humanRequired && !rootProfile?.retired && (
                            <Button size="sm" variant="ghost" onClick={() => reuseDelegate(row)}>
                              <Send className="size-3.5" /> Use target
                            </Button>
                          )}
                        </>
                      )}
                      {row.resolution === "force_chain" &&
                        row.routingTaskType &&
                        (row.routingTaskModelChain?.length || 0) > 0 &&
                        ops.humanRequired &&
                        !rootProfile?.retired && (
                          <Button size="sm" variant="ghost" onClick={() => branchForceChain(row)}>
                            <Wrench className="size-3.5" /> Branch chain
                          </Button>
                        )}
                    </div>
                  </li>
                ))}
              </ul>
            )}
          </div>
        </div>

        <div className="space-y-3">
          <div className="glass rounded-xl p-3">
            <div className="mb-2 flex items-center gap-2 text-xs font-semibold uppercase tracking-wider text-muted">
              <Wrench className="size-3.5" /> Operations
            </div>
            {!ops.rootSlug || !rootProfile ? (
              <div className="text-xs text-muted">
                Root agent profile is no longer present. The incident tree is still readable, but operator actions are unavailable.
              </div>
            ) : (
              <div className="space-y-3">
                <div className="rounded-lg border border-border bg-panel/35 p-2.5 text-[11px]">
                  <div className="flex flex-wrap items-center gap-2">
                    <button
                      onClick={() => openAgent(rootProfile.slug)}
                      className="font-mono font-semibold text-foreground hover:text-accent"
                    >
                      {rootProfile.slug}
                    </button>
                    <span className="text-muted">
                      {rootProfile.retired
                        ? "graveyard"
                        : rootProfile.enabled
                          ? "enabled"
                          : "paused"}
                    </span>
                    {ops.ownerSlug && (
                      <span className="text-muted">
                        owner <span className="font-mono text-foreground/85">{ops.ownerSlug}</span>
                      </span>
                    )}
                  </div>
                  <div className="mt-1 text-muted">
                    {rootProfile.name && rootProfile.name !== rootProfile.slug
                      ? rootProfile.name
                      : "incident root agent"}
                    {ops.configIssues.length > 0
                      ? ` · ${ops.configIssues.length} current config issue(s)`
                      : ""}
                  </div>
                </div>

                {ops.humanRequired && (
                  <div className="rounded-lg border border-amber-500/35 bg-amber-500/10 p-2.5 text-[11px]">
                    <div className="flex flex-wrap items-center gap-2 font-medium text-amber-200">
                      <ShieldAlert className="size-3.5" />
                      {ops.policyLabel || "Human-required routing incident"}
                    </div>
                    <div className="mt-1 text-amber-100/85">
                      {ops.policySummary ||
                        "This incident should be handled as an ownership decision, not a routine doctor retry."}
                    </div>
                    {ops.allowedResolutions.length > 0 && (
                      <div className="mt-2 flex flex-wrap gap-1.5">
                        {ops.allowedResolutions.map((row) => (
                          <span
                            key={row}
                            className="rounded-full border border-amber-400/25 bg-amber-400/10 px-2 py-0.5 font-mono text-[10px] text-amber-100/90"
                          >
                            {row}
                          </span>
                        ))}
                      </div>
                    )}
                  </div>
                )}

                <div className="flex flex-wrap gap-2">
                  <Button size="sm" onClick={() => openAgent(rootProfile.slug)}>
                    <Bot className="size-3.5" /> Root agent
                  </Button>
                  {ops.ownerSlug && ops.ownerSlug !== rootProfile.slug && (
                    <Button size="sm" variant="ghost" onClick={() => openAgent(ops.ownerSlug!)}>
                      <Bot className="size-3.5" /> Owner
                    </Button>
                  )}
                  {!rootProfile.retired && !ops.suppressDoctorRerun && (
                    <Button
                      size="sm"
                      variant="ghost"
                      disabled={busy === `repair:${rootProfile.slug}`}
                      onClick={triggerRepair}
                    >
                      <Wrench className="size-3.5" /> Doctor rerun
                    </Button>
                  )}
                  {!rootProfile.retired && !ops.humanRequired && (
                    <Button
                      size="sm"
                      variant="ghost"
                      disabled={busy === `enable:${rootProfile.slug}`}
                      onClick={() => void toggleEnabled(!rootProfile.enabled)}
                    >
                      {rootProfile.enabled ? (
                        <>
                          <Pause className="size-3.5" /> Pause
                        </>
                      ) : (
                        <>
                          <Play className="size-3.5" /> Resume
                        </>
                      )}
                    </Button>
                  )}
                  {!rootProfile.retired && ops.humanRequired && rootProfile.enabled && (
                    <Button
                      size="sm"
                      variant="ghost"
                      disabled={busy === `resolve:paused:${rootProfile.slug}`}
                      onClick={() => void resolveIncident("paused")}
                    >
                      <Pause className="size-3.5" /> Pause root now
                    </Button>
                  )}
                  <Button
                    size="sm"
                    variant={rootProfile.retired ? "ghost" : "danger"}
                    disabled={
                      busy === `${rootProfile.retired ? "revive" : "retire"}:${rootProfile.slug}`
                    }
                    onClick={toggleRetired}
                  >
                    {rootProfile.retired ? (
                      <>
                        <ArchiveRestore className="size-3.5" /> Revive
                      </>
                    ) : (
                      <>
                        <Archive className="size-3.5" /> Retire
                      </>
                    )}
                  </Button>
                  {canWakeRoot && (
                    <Button
                      size="sm"
                      variant="ghost"
                      disabled={busy === `wake:${rootProfile.slug}`}
                      onClick={() => void wakeAgent(rootProfile.slug)}
                    >
                      <Play className="size-3.5" /> Wake root
                    </Button>
                  )}
                  {ops.ownerSlug && (
                    <Button
                      size="sm"
                      variant={ops.preferOwnerWake ? "accent" : "ghost"}
                      disabled={busy === `wake:${ops.ownerSlug}` || !canWakeOwner}
                      onClick={() => void wakeAgent(ops.ownerSlug!)}
                    >
                      <Play className="size-3.5" /> Wake owner
                    </Button>
                  )}
                  {!rootProfile.retired && (
                    <Button
                      size="sm"
                      variant={showRepair ? "accent" : "ghost"}
                      onClick={() => setShowRepair((v) => !v)}
                    >
                      <Wrench className="size-3.5" /> {showRepair ? "Hide repair" : ops.policyKind === "routing_force_exhausted" ? "Routing console" : "Repair console"}
                    </Button>
                  )}
                </div>

                {ops.humanRequired && resolutionPresets.length > 0 && (
                  <div className="space-y-2 rounded-lg border border-amber-500/20 bg-amber-500/5 p-2.5">
                    <div className="text-[11px] font-medium text-amber-100">
                      Owner resolution shortcuts
                    </div>
                    <div className="text-[11px] text-amber-100/80">
                      These presets keep the mailbox thread aligned to the only allowed outcomes for this exhausted routing policy.
                    </div>
                    <div className="flex flex-wrap gap-2">
                      {resolutionPresets.map((preset) => (
                        <Button
                          key={preset.key}
                          size="sm"
                          variant="ghost"
                          onClick={() => applyResolutionPreset(preset.key)}
                        >
                          {preset.label}
                        </Button>
                      ))}
                    </div>
                  </div>
                )}

                {ops.humanRequired && !rootProfile.retired && (
                  <div className="space-y-3 rounded-lg border border-amber-500/20 bg-amber-500/5 p-2.5">
                    <div className="text-[11px] font-medium text-amber-100">
                      Direct incident resolution
                    </div>
                    <div className="space-y-2">
                      <div className="text-[11px] text-amber-100/80">
                        Delegate this exhausted routing incident to a concrete owner.
                      </div>
                      <div className="flex flex-wrap gap-2">
                        <input
                          list="incident-delegate-candidates"
                          value={delegateTo}
                          onChange={(e) => setDelegateTo(e.target.value)}
                          placeholder="delegate target slug"
                          className="min-w-[180px] flex-1 rounded-md border border-border bg-panel px-2 py-1.5 text-xs text-foreground outline-none focus-visible:border-accent"
                        />
                        <datalist id="incident-delegate-candidates">
                          {validDelegateCandidates.map((row) => (
                            <option key={row.slug} value={row.slug}>
                              {row.name && row.name !== row.slug ? `${row.name} (${row.slug})` : row.slug}
                            </option>
                          ))}
                        </datalist>
                        <Button
                          size="sm"
                          variant="ghost"
                          disabled={
                            busy === `resolve:delegated:${rootProfile.slug}` ||
                            !delegateValidation.valid
                          }
                          onClick={() => void resolveDelegated()}
                        >
                          <Send className="size-3.5" /> Delegate now
                        </Button>
                      </div>
                      {validDelegateCandidates.length > 0 && (
                        <div className="flex flex-wrap gap-1.5">
                          {validDelegateCandidates.map((row) => (
                            <button
                              key={row.slug}
                              type="button"
                              onClick={() => setDelegateTo(row.slug)}
                              className="rounded-full border border-amber-400/20 bg-amber-400/10 px-2 py-0.5 text-[10px] text-amber-100/90 transition-colors hover:border-amber-300/35 hover:bg-amber-400/15"
                              title={row.name && row.name !== row.slug ? row.name : row.slug}
                            >
                              {row.slug}
                              {(row.escalationOpenCount || 0) > 0
                                ? ` · esc ${row.escalationOpenCount}`
                                : row.healthState && row.healthState !== "healthy"
                                  ? ` · ${row.healthState}`
                                  : ""}
                            </button>
                          ))}
                        </div>
                      )}
                      {delegateTo.trim() && (
                        <div
                          className={
                            delegateValidation.valid
                              ? "text-[11px] text-amber-100/75"
                              : "text-[11px] text-rose-200"
                          }
                        >
                          {delegateValidation.valid
                            ? `delegates incident ownership to ${delegateValidation.normalizedTarget}`
                            : delegateValidation.reason}
                        </div>
                      )}
                      {selectedDelegateCandidate?.valid && (
                        <div className="space-y-1">
                          <div className="text-[11px] text-amber-100/65">
                            target{" "}
                            <span className="font-mono text-amber-50/90">
                              {selectedDelegateCandidate.slug}
                            </span>
                            {" · "}
                            {selectedDelegateCandidate.enabled ? "enabled" : "paused"}
                            {selectedDelegateCandidate.preferred ? " · incident-adjacent" : ""}
                            {selectedDelegateCandidate.parentAgent
                              ? ` · parent ${selectedDelegateCandidate.parentAgent}`
                              : selectedDelegateCandidate.ownerAgent
                                ? ` · owner ${selectedDelegateCandidate.ownerAgent}`
                                : ""}
                          </div>
                          {(selectedDelegateStatus.healthText ||
                            selectedDelegateStatus.repairKindText ||
                            selectedDelegateStatus.repairText ||
                            selectedDelegateStatus.routingText ||
                            selectedDelegateEscalationSummary.openCount > 0 ||
                            selectedDelegateEscalationSummary.ackedCount > 0) && (
                            <div className="flex flex-wrap gap-1.5">
                              {selectedDelegateStatus.healthText && (
                                <span className="rounded-full border border-border/70 bg-panel/50 px-2 py-0.5 text-[10px] text-amber-50/90">
                                  {selectedDelegateStatus.healthText}
                                </span>
                              )}
                              {selectedDelegateStatus.repairKindText && (
                                <span className="rounded-full border border-border/70 bg-panel/50 px-2 py-0.5 text-[10px] text-amber-50/90">
                                  {selectedDelegateStatus.repairKindText}
                                </span>
                              )}
                              {selectedDelegateStatus.repairText && (
                                <span className="rounded-full border border-border/70 bg-panel/50 px-2 py-0.5 text-[10px] text-amber-50/90">
                                  {selectedDelegateStatus.repairText}
                                </span>
                              )}
                              {selectedDelegateStatus.routingText && (
                                <span
                                  className="rounded-full border border-border/70 bg-panel/50 px-2 py-0.5 text-[10px] text-amber-50/90"
                                  title={selectedDelegateStatus.routingDetail}
                                >
                                  {selectedDelegateStatus.routingText}
                                </span>
                              )}
                              {selectedDelegateEscalationSummary.openCount > 0 && (
                                <span
                                  className="rounded-full border border-border/70 bg-panel/50 px-2 py-0.5 text-[10px] text-amber-50/90"
                                  title={`${selectedDelegateEscalationSummary.doctorOpenCount} doctor, ${selectedDelegateEscalationSummary.delegatedOpenCount} delegated`}
                                >
                                  open esc {selectedDelegateEscalationSummary.openCount}
                                </span>
                              )}
                              {selectedDelegateEscalationSummary.ackedCount > 0 && (
                                <span className="rounded-full border border-border/70 bg-panel/50 px-2 py-0.5 text-[10px] text-amber-50/90">
                                  acked {selectedDelegateEscalationSummary.ackedCount}
                                </span>
                              )}
                            </div>
                          )}
                          {selectedDelegateEscalationSummary.latest && (
                            <div className="text-[11px] text-amber-100/55">
                              latest escalation{" "}
                              <span className="font-mono text-amber-50/80">
                                {selectedDelegateEscalationSummary.latest.source_agent || "unknown"}
                              </span>
                              {selectedDelegateEscalationSummary.latest.status
                                ? ` · ${selectedDelegateEscalationSummary.latest.status}`
                                : ""}
                            </div>
                          )}
                        </div>
                      )}
                      {invalidDelegateCandidates.length > 0 && (
                        <div className="text-[11px] text-amber-100/60">
                          blocked:{" "}
                          {invalidDelegateCandidates
                            .map((row) => `${row.slug}${row.reason ? ` (${row.reason.replace(/^delegate target /, "")})` : ""}`)
                            .join(" · ")}
                        </div>
                      )}
                    </div>
                    <div className="space-y-2">
                      <div className="text-[11px] text-amber-100/80">
                        Force a new chain only when you have a concrete replacement. Current exhausted chain:{" "}
                        <span className="font-mono text-amber-50/90">{exhaustedChain || "(unknown)"}</span>
                      </div>
                      <input
                        value={forceTaskType}
                        onChange={(e) => setForceTaskType(e.target.value)}
                        placeholder={ops.routingTaskType || rootProfile.task_type || "task type"}
                        className="w-full rounded-md border border-border bg-panel px-2 py-1.5 text-xs text-foreground outline-none focus-visible:border-accent"
                      />
                      <textarea
                        value={forceChainText}
                        onChange={(e) => setForceChainText(e.target.value)}
                        placeholder="new chain, e.g. gpt-4.1, deepseek-chat"
                        rows={2}
                        className="w-full rounded-md border border-border bg-panel px-2 py-1.5 text-xs text-foreground outline-none focus-visible:border-accent"
                      />
                      {forcePresetViews.length > 0 && (
                        <div className="space-y-1">
                          <div className="text-[11px] text-amber-100/70">
                            Prior force-chain presets
                          </div>
                          <div className="flex flex-wrap gap-1.5">
                            {forcePresetViews.map((preset) => (
                              <button
                                key={preset.key}
                                type="button"
                                disabled={!preset.preview.valid}
                                onClick={() => useForcePreset(preset.taskType, preset.chainText, preset.summary)}
                                className="rounded-full border border-amber-400/20 bg-amber-400/10 px-2 py-0.5 text-[10px] text-amber-100/90 transition-colors hover:border-amber-300/35 hover:bg-amber-400/15 disabled:cursor-not-allowed disabled:opacity-45"
                                title={preset.summary}
                              >
                                {preset.taskType}: {preset.chainText}
                                {preset.generation && preset.generation > 1 ? ` · gen ${preset.generation}` : ""}
                                {preset.preview.sameAsCurrent
                                  ? " · same current"
                                  : preset.preview.added.length > 0 || preset.preview.removed.length > 0
                                    ? ` · +${preset.preview.added.length}/-${preset.preview.removed.length}`
                                    : ""}
                              </button>
                            ))}
                          </div>
                        </div>
                      )}
                      {(forceChainText.trim() || exhaustedChain) && (
                        <div className="rounded-md border border-amber-500/15 bg-black/10 p-2 text-[11px] text-amber-100/80">
                          <div className="font-medium text-amber-100/90">
                            Chain diff preview
                          </div>
                          <div className="mt-1">
                            current{" "}
                            <span className="font-mono text-amber-50/90">
                              {forcePreview.currentChain.length > 0
                                ? forcePreview.currentChain.join(" → ")
                                : "(unknown)"}
                            </span>
                          </div>
                          <div className="mt-1">
                            proposed{" "}
                            <span className="font-mono text-amber-50/90">
                              {forcePreview.proposedChain.length > 0
                                ? forcePreview.proposedChain.join(" → ")
                                : "(empty)"}
                            </span>
                          </div>
                          {(forcePreview.added.length > 0 ||
                            forcePreview.removed.length > 0) && (
                            <div className="mt-1 flex flex-wrap gap-2">
                              {forcePreview.added.length > 0 && (
                                <span>
                                  add{" "}
                                  <span className="font-mono text-amber-50/90">
                                    {forcePreview.added.join(", ")}
                                  </span>
                                </span>
                              )}
                              {forcePreview.removed.length > 0 && (
                                <span>
                                  drop{" "}
                                  <span className="font-mono text-amber-50/90">
                                    {forcePreview.removed.join(", ")}
                                  </span>
                                </span>
                              )}
                            </div>
                          )}
                          {!forcePreview.valid && forceChainText.trim() && (
                            <div className="mt-1 text-rose-200">
                              {forcePreview.reason}
                            </div>
                          )}
                        </div>
                      )}
                      <div className="flex justify-end">
                        <Button
                          size="sm"
                          variant="ghost"
                          disabled={
                            busy === `resolve:force_chain:${rootProfile.slug}` ||
                            !forcePreview.valid
                          }
                          onClick={() => void resolveForceChain()}
                        >
                          <Wrench className="size-3.5" /> Force new chain
                        </Button>
                      </div>
                    </div>
                  </div>
                )}

                <div className="space-y-2">
                  <div className="text-[11px] text-muted">
                    {ops.humanRequired
                      ? "Send an in-band operator note to the current owner chain. Exhausted routing incidents should stay inside mailbox/escalation rather than drifting into ad hoc chat."
                      : "Send a help-thread note to the current owner chain. This keeps the incident inside the mailbox/escalation path instead of becoming an out-of-band comment."}
                  </div>
                  <textarea
                    value={noteText}
                    onChange={(e) => setNoteText(e.target.value)}
                    placeholder={
                      noteTarget
                        ? `message ${noteTarget} about ${rootProfile.slug}`
                        : "no owner/parent target found for this incident"
                    }
                    rows={3}
                    className="w-full rounded-md border border-border bg-panel px-2 py-1.5 text-xs text-foreground outline-none focus-visible:border-accent"
                  />
                  <div className="flex flex-wrap items-center gap-2 text-[11px] text-muted">
                    <span>
                      target{" "}
                      <span className="font-mono text-foreground/85">
                        {noteTarget || "(none)"}
                      </span>
                    </span>
                    <Button
                      size="sm"
                      variant="ghost"
                      disabled={!noteTarget || !noteText.trim() || busy === `note:${noteTarget}`}
                      onClick={sendOperatorNote}
                    >
                      <Send className="size-3.5" /> Send note
                    </Button>
                  </div>
                </div>

                {showRepair && (
                  <div className="rounded-lg border border-border bg-panel/20 p-2.5">
                    <AgentRepair
                      slug={rootProfile.slug}
                      profile={rootProfile}
                      denials={null}
                      toolErrors={null}
                      runs={0}
                      configIssues={ops.configIssues}
                      onApplied={() => void load()}
                    />
                  </div>
                )}
              </div>
            )}
          </div>
          <div className="glass rounded-xl p-3">
            <div className="mb-2 text-xs font-semibold uppercase tracking-wider text-muted">
              Related agents
            </div>
            <div className="flex flex-wrap gap-2">
              {agents.map((agent) => (
                <button
                  key={agent.slug}
                  onClick={() => openAgent(agent.slug)}
                  className="inline-flex items-center gap-1.5 rounded-lg border border-border bg-panel/40 px-2.5 py-1.5 text-xs transition-colors hover:border-accent hover:text-accent"
                >
                  <Bot className="size-3.5" />
                  {agent.name && agent.name !== agent.slug
                    ? `${agent.name} (${agent.slug})`
                    : agent.slug}
                </button>
              ))}
            </div>
          </div>
          <div className="glass rounded-xl p-3">
            <div className="mb-2 text-xs font-semibold uppercase tracking-wider text-muted">
              Selected node
            </div>
            <div className="space-y-1.5 text-[11px]">
              <div className="text-foreground/90">{focus?.title}</div>
              <div className="flex flex-wrap items-center gap-1.5">
                <IncidentBadges item={focus} mono />
              </div>
              {focus?.detail && <div className="text-muted">{focus.detail}</div>}
              <div className="font-mono text-muted">
                incident {focus ? incidentMetaFromAutonomy(focus).incidentId || rootId : rootId}
              </div>
              {focus?.root_agent && (
                <div className="text-muted">root {focus.root_agent}</div>
              )}
              {typeof focus?.chain_depth === "number" && (
                <div className="text-muted">hop {focus.chain_depth}</div>
              )}
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
