import { useMemo, useState } from "react";
import { Activity as ActivityIcon, ShieldCheck, FolderOpen, Bot, Coins, Clock, ArrowUpRight, Pause, Flame, AlertTriangle, CalendarClock, Zap, ChevronRight, Mail, Wrench, ArrowRight, Waypoints, CheckCheck, LifeBuoy, Megaphone, Skull, ListTree, Repeat, IdCard, CheckCircle, AlertCircle, XCircle, MinusCircle } from "lucide-react";
import { cn, fmtTime, fmtDateTime, fmtAgo, fmtDue, clip } from "@/lib/utils";
import { money } from "@/lib/format";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Disclosure } from "@/components/ui/disclosure";
import { RunDetailLoader } from "@/components/RunDetail";
import { TriggerChip } from "@/components/Fleet";
import { openIncident } from "@/lib/incidentnav";
import { agentHierarchySummary, agentLifecycleSummary, agentTaskProgressSummary, type AgentCommandStripItem, type AgentControlCenterEntry, type AgentProfile } from "@/views/Roster";
import { type FleetTrigger, type ApiOrder, type ApiSchedule } from "@/lib/fleet";
import { agentScope, escalationChainLabel, incidentLineageLabel, wakeLineage, type AgentHealthSnapshot, type AgentEscalation, type AgentOperationalTask, type AgentRepairStatus, type AgentRepairSnapshot, type AgentCardRuntimeSummary, type MemoryRecord, type SkillLite, type RunLite } from "@/lib/agentdetail";
import { AgentDetailCommandStrip, AgentPermissionsSnapshot, BoardMessage, BudgetBar, DetailTab, Row, Stat, ToolCatalogRow } from "@/components/agentdetail/shared";
import { AgentEntityContract, AutonomyRunbook, LifecycleInterventionPanel, RuntimeDoctorLedger } from "@/components/agentdetail/LifecyclePanel";
import { OperationalTaskList } from "@/components/agentdetail/tasks";
import { agentAutonomyRunbook, agentEntityContractLedger, agentLifecycleDetail, agentOperationsPassport, agentRepairOperationsSummary, agentResourcePassportDetail, agentRetryPolicyDetail, agentRuntimeDoctorLedger, agentSystemGuardianContract } from "@/components/agentdetail/lifecycle";
import { agentConfigAuthorityContract, effectiveToolPermissions, permissionRowFromSnapshot, summarizePermissionPassport, summarizeWakeAccess } from "@/components/agentdetail/capability";
import { agentMailboxWakeContract } from "@/components/agentdetail/comms";

export function Overview({
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

