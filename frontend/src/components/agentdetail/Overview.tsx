import { Activity as ActivityIcon, AlertTriangle, ArrowUpRight, ChevronRight, LifeBuoy, Skull, Wrench, ShieldCheck } from "lucide-react";
import { cn, fmtTime, fmtDateTime, clip } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Advanced } from "@/components/ui/disclosure";
import { KeyValue } from "@/components/JsonView";
import { TriggerChip } from "@/components/Fleet";
import { openIncident } from "@/lib/incidentnav";
import { agentHierarchySummary, agentLifecycleSummary, type AgentProfile } from "@/views/Roster";
import { type FleetTrigger, type ApiSchedule } from "@/lib/fleet";
import { agentScope, incidentLineageLabel, type AgentHealthSnapshot, type AgentEscalation, type AgentOperationalTask, type AgentRepairStatus, type AgentRepairSnapshot, type MemoryRecord, type SkillLite, type RunLite } from "@/lib/agentdetail";
import { BoardMessage, BudgetBar, DetailTab } from "@/components/agentdetail/shared";
import { LifecycleInterventionPanel } from "@/components/agentdetail/LifecyclePanel";
import { OperationalTaskList } from "@/components/agentdetail/tasks";
import { agentLifecycleDetail } from "@/components/agentdetail/lifecycle";

// Overview — the operator's one-screen answer to "is this agent okay and what
// should I look at next". Declutter law: the header already carries the glance
// metrics, so this tab holds only what you'd ACT on — the last failure, how it
// runs, budgets, open attention items, one identity card, and the lifecycle
// intervention panel folded under Advanced. Everything else lives in Diagnostics.
export function Overview({
  slug,
  profile,
  triggers,
  fail,
  health,
  repair,
  repairStatus,
  escalation,
  escalationTasks,
  todaySpentMc,
  memory,
  skills,
  schedules,
  mailboxMessages,
  busy,
  onLifecycleChanged,
  onManage,
  onView,
  onFocusRun,
}: {
  slug: string;
  profile: AgentProfile;
  triggers: FleetTrigger[];
  fail?: RunLite;
  health: AgentHealthSnapshot;
  repair: AgentRepairSnapshot;
  repairStatus: AgentRepairStatus | null;
  escalation: {
    openCount: number;
    ackedCount: number;
    doctorOpenCount: number;
    delegatedOpenCount: number;
    latest?: AgentEscalation;
  };
  escalationTasks: AgentOperationalTask[];
  todaySpentMc: number;
  memory: MemoryRecord[] | null;
  skills: SkillLite[] | null;
  schedules: ApiSchedule[];
  mailboxMessages: BoardMessage[];
  busy: boolean;
  onLifecycleChanged: () => void;
  onManage: (view: string) => void;
  onView: (t: DetailTab) => void;
  onFocusRun: (correlationId: string | undefined) => void;
}) {
  const repairNoteworthy =
    repair.state === "failed" || repair.state === "queued" || !!repairStatus?.inflight_count;
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

      {/* Last failure — the "ne bok yedi" headline */}
      {fail && (
        <button
          onClick={() => {
            onFocusRun(fail.correlation_id);
            onView("activity");
          }}
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
              {fail.started_unix_ms ? fmtTime(fail.started_unix_ms) : "—"}
            </div>
          </div>
          <ChevronRight className="ml-auto size-3.5 shrink-0 text-muted" />
        </button>
      )}

      {/* Unhealthy but no failed run to jump to — still surface it. */}
      {!fail && health.state !== "healthy" && health.state !== "retired" && (
        <button
          onClick={() => onView("diag")}
          className="flex w-full items-start gap-2 rounded-lg border border-bad/40 bg-bad/5 p-2.5 text-left"
        >
          <AlertTriangle className="mt-0.5 size-3.5 shrink-0 text-bad" />
          <div className="min-w-0">
            <div className="text-[11px] font-medium text-bad">{health.label}</div>
            <div className="truncate text-[11px] text-muted">{health.detail}</div>
          </div>
          <ChevronRight className="ml-auto size-3.5 shrink-0 text-muted" />
        </button>
      )}

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

      {/* Budgets */}
      <div className="grid gap-2 sm:grid-cols-2">
        <BudgetBar
          label="today's spend"
          spentMc={todaySpentMc}
          capMc={profile.max_daily_mc}
        />
        <BudgetBar
          label="per-run ceiling"
          spentMc={0}
          capMc={profile.max_cost_mc}
        />
      </div>

      {/* Attention strip — rendered only when something actually needs the operator. */}
      {escalation.openCount > 0 && (
        <button
          onClick={() => onView("wiring")}
          className="flex w-full items-start gap-2 rounded-lg border border-warn/40 bg-warn/10 p-2.5 text-left"
        >
          <LifeBuoy className="mt-0.5 size-3.5 shrink-0 text-warn" />
          <div className="min-w-0">
            <div className="text-[11px] font-medium text-warn">
              Escalation queue
            </div>
            <div className="text-[11px] text-muted">
              {escalation.openCount} open escalation
              {escalation.openCount === 1 ? "" : "s"} waiting for this agent
            </div>
            {escalation.latest?.source_agent && (
              <span
                role="button"
                tabIndex={0}
                onClick={(e) => {
                  e.stopPropagation();
                  if (escalation.latest?.root_incident_id)
                    openIncident(escalation.latest.root_incident_id);
                  else if (escalation.latest?.incident_id)
                    openIncident(escalation.latest.incident_id);
                }}
                onKeyDown={(e) => {
                  if (e.key !== "Enter" && e.key !== " ") return;
                  e.stopPropagation();
                  if (escalation.latest?.root_incident_id)
                    openIncident(escalation.latest.root_incident_id);
                  else if (escalation.latest?.incident_id)
                    openIncident(escalation.latest.incident_id);
                }}
                className="mt-1 inline-block text-xs uppercase tracking-normal text-muted transition-colors hover:text-accent"
                title="Open this escalation incident"
              >
                latest for {escalation.latest.source_agent}
              </span>
            )}
          </div>
          <div className="ml-auto flex shrink-0 items-center gap-2">
            <Badge variant="warn">{escalation.openCount} open</Badge>
            <ChevronRight className="size-3.5 text-muted" />
          </div>
        </button>
      )}

      {repairNoteworthy && (
        <button
          onClick={() => onView("diag")}
          className={cn(
            "flex w-full items-start gap-2 rounded-lg border p-2.5 text-left",
            repair.state === "failed"
              ? "border-bad/40 bg-bad/5"
              : "border-accent/40 bg-accent/5",
          )}
        >
          {repair.state === "failed" ? (
            <AlertTriangle className="mt-0.5 size-3.5 shrink-0 text-bad" />
          ) : (
            <Wrench className="mt-0.5 size-3.5 shrink-0 text-accent" />
          )}
          <div className="min-w-0">
            <div
              className={cn(
                "text-[11px] font-medium",
                repair.state === "failed" ? "text-bad" : "text-accent",
              )}
            >
              Auto-repair
            </div>
            <div className="text-[11px] text-muted">{repair.detail}</div>
            {repairStatus?.latest && (
              <span
                role="button"
                tabIndex={0}
                onClick={(e) => {
                  e.stopPropagation();
                  if (repairStatus.latest?.root_incident_id)
                    openIncident(repairStatus.latest.root_incident_id);
                  else if (repairStatus.latest?.incident_id)
                    openIncident(repairStatus.latest.incident_id);
                }}
                onKeyDown={(e) => {
                  if (e.key !== "Enter" && e.key !== " ") return;
                  e.stopPropagation();
                  if (repairStatus.latest?.root_incident_id)
                    openIncident(repairStatus.latest.root_incident_id);
                  else if (repairStatus.latest?.incident_id)
                    openIncident(repairStatus.latest.incident_id);
                }}
                className="mt-1 inline-block text-xs uppercase tracking-normal text-muted transition-colors hover:text-accent"
                title="Open this repair incident"
              >
                {incidentLineageLabel(repairStatus.latest)}
              </span>
            )}
          </div>
          <div className="ml-auto flex shrink-0 items-center gap-2">
            {repairStatus?.inflight_count ? (
              <Badge variant="accent">{repairStatus.inflight_count} inflight</Badge>
            ) : null}
            <ChevronRight className="size-3.5 text-muted" />
          </div>
        </button>
      )}

      {escalationTasks.length > 0 && (
        <div className="rounded-lg border border-warn/30 bg-warn/5 p-2.5">
          <div className="mb-1 text-xs uppercase tracking-normal text-warn">
            active responsibilities
          </div>
          <OperationalTaskList tasks={escalationTasks} compact />
        </div>
      )}

      {/* Identity facts — every stored field exactly once. */}
      <div className="rounded-lg border border-border bg-panel/30 p-2.5">
        <div className="mb-1.5 flex items-center gap-1.5 text-xs font-semibold uppercase tracking-normal text-muted">
          <ShieldCheck className="size-3" /> identity
        </div>
        <KeyValue
          pairs={(
            [
              [
                "model",
                profile.model ? (
                  <span className="font-mono">{profile.model}</span>
                ) : (
                  "(routing default)"
                ),
              ],
              (profile.fallbacks || []).length > 0
                ? [
                    "fallbacks",
                    <span key="fb" className="font-mono">
                      {(profile.fallbacks || []).join(" → ")}
                    </span>,
                  ]
                : null,
              profile.task_type ? ["task type", profile.task_type] : null,
              [
                "lifecycle",
                `${agentLifecycleSummary(profile)} · ${agentLifecycleDetail(profile)}`,
              ],
              ["call policy", agentHierarchySummary(profile)],
              ["trust ceiling", profile.trust_ceiling || "L4"],
              [
                "memory scope",
                <span key="scope" className="font-mono">
                  {agentScope(slug, profile.memory_scope)}
                </span>,
              ],
              profile.workdir
                ? [
                    "workdir",
                    <span key="wd" className="font-mono">{profile.workdir}</span>,
                  ]
                : null,
            ] as ([string, React.ReactNode] | null)[]
          ).filter((p): p is [string, React.ReactNode] => p !== null)}
        />
      </div>

      <Advanced label="Lifecycle intervention">
        <LifecycleInterventionPanel
          slug={slug}
          profile={profile}
          schedules={schedules}
          memory={memory}
          skills={skills}
          mailboxMessages={mailboxMessages}
          busy={busy}
          onChanged={onLifecycleChanged}
        />
      </Advanced>

      <div className="flex flex-wrap gap-2">
        <Button variant="ghost" size="sm" onClick={() => onManage("roster")}>
          Manage in Roster <ArrowUpRight className="size-3.5" />
        </Button>
      </div>
    </div>
  );
}
