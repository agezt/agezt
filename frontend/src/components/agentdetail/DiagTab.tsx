import { useState } from "react";
import { X, ShieldCheck, AlertTriangle, Wrench } from "lucide-react";
import { postJSON } from "@/lib/api";
import { cn, fmtTime, fmtDateTime, fmtAgo, clip } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { SkeletonList } from "@/components/ui/skeleton";
import { Disclosure } from "@/components/ui/disclosure";
import { useUI } from "@/components/ui/feedback";
import { openIncident } from "@/lib/incidentnav";
import { type AgentProfile } from "@/views/Roster";
import { incidentLineageLabel, type AgentHealthSnapshot, type AgentConfigOverrideSummary, type AgentRepairStatus, type AgentRepairSnapshot, type RunLite } from "@/lib/agentdetail";
import { AgentPermissionsSnapshot, ApprovalDecision, MiniPolicy, PolicyDecision, PolicyStats, RepairCommandCell, Stat, ToolCatalogRow, ToolInvocation } from "@/components/agentdetail/shared";
import { CapabilityControlPanel } from "@/components/agentdetail/CapabilityPanel";
import { WakeAccessSummary, agentManagedSubagent } from "@/components/agentdetail/capability";
import { agentRepairCommandSummary, agentRepairDecisionSummary, agentRepairOperationsSummary } from "@/components/agentdetail/lifecycle";

export function DiagTab({
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

