import { useEffect, useMemo, useState } from "react";
import { Skull, Archive, ArchiveRestore, Trash2 } from "lucide-react";
import { getJSON, postAction, postJSON } from "@/lib/api";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { useUI } from "@/components/ui/feedback";
import { agentRemoveToast, agentRetireToast, agentReviveToast, type AgentProfile, type AgentRemoveResult, type AgentRetireResult, type AgentReviveResult } from "@/views/Roster";
import { type ApiSchedule } from "@/lib/fleet";
import { type MemoryRecord, type SkillLite } from "@/lib/agentdetail";
import { AgentImpactSummary, AgentLifecycleActionResultSummary, AgentLifecycleLedgerEntry, agentDetailRemovalCascadePreset, agentLifecycleActionResultSummary, agentLifecycleDecisionLedger, agentLifecycleInterventionSummary, agentRemovalImpactPlan, agentRemovalRiskLabel } from "@/components/agentdetail/lifecycle";
import { BoardMessage } from "@/components/agentdetail/shared";

export function LifecycleInterventionPanel({
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

