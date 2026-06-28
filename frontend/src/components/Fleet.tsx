// Fleet.tsx (M952) — the visual layer for the unified agent census (see
// lib/fleet.ts). FleetCard is one entity at a glance; FleetDetail is the drill-in
// that answers, loudly, "how does this thing actually run?". Every kind of agent
// — roster identity, standing order, schedule, workflow, system engine — wears
// the same card shape so the page reads as one army roster, not five disjoint
// lists. Trigger info is the hero on both surfaces.
import {
  Users,
  Anchor,
  CalendarClock,
  GitFork,
  Cpu,
  Zap,
  Timer,
  Webhook,
  Infinity as InfinityIcon,
  MousePointerClick,
  Share2,
  Clock,
  Coins,
  X,
  ArrowUpRight,
  Activity,
  AlertTriangle,
  Flame,
  ShieldCheck,
  IdCard,
  ListChecks,
  Wrench,
  Moon,
  Play,
  PauseCircle,
  Skull,
  type LucideIcon,
} from "lucide-react";
import { useState, type MouseEvent as ReactMouseEvent, type KeyboardEvent as ReactKeyboardEvent } from "react";
import {
  fleetAgentCapabilityLabel,
  fleetAgentAuthorityLabel,
  fleetAgentHierarchyLabel,
  fleetAgentIdentityCardSummary,
  fleetAgentResilienceLabel,
  fleetAgentTaskContractLabel,
  type ApiProfile,
  type FleetEntity,
  type FleetKind,
  type FleetState,
  type TriggerMode,
} from "@/lib/fleet";
import { AgentAvatar } from "@/components/AgentAvatar";
import { Button } from "@/components/ui/button";
import { cn, clip, fmtDateTime, fmtAgo } from "@/lib/utils";
import { money } from "@/lib/format";
import { summarizeAgentRuntimeStatus, fleetCardIssueSummary } from "@/lib/agentdetail";
import { useUI } from "@/components/ui/feedback";
import { postAction } from "@/lib/api";

export function fleetAgentRepairOpsSummary(
  profile: Pick<ApiProfile, "retired" | "retry_policy" | "health_policy" | "self_repair">,
  runtime: ReturnType<typeof summarizeAgentRuntimeStatus> | null,
): { value: string; detail: string; tone: "good" | "warn" | "bad" | "accent" | "muted" } {
  if (profile.retired) {
    return { value: "graveyard", detail: "repair blocked until revived", tone: "muted" };
  }
  if ((runtime?.repairInflight || 0) > 0) {
    return {
      value: runtime?.repairText || `repair ${runtime?.repairInflight || 1}`,
      detail: [runtime?.repairKindText || "repair", runtime?.repairDetail, runtime?.repairIncidentDetail].filter(Boolean).join(" · "),
      tone: "accent",
    };
  }
  if (runtime?.repairText) {
    return {
      value: runtime.repairText,
      detail: [runtime.repairKindText || "repair", runtime.repairDetail, runtime.repairIncidentDetail].filter(Boolean).join(" · "),
      tone: runtime.repairTone === "bad" ? "bad" : runtime.repairTone === "good" ? "good" : runtime.repairTone === "accent" ? "accent" : "muted",
    };
  }
  if (runtime?.retryText) {
    return { value: runtime.retryText, detail: runtime.retryDetail || "whole-run retry pressure is active", tone: "bad" };
  }
  const parts = [
    profile.retry_policy?.max_attempts ? `retry ${profile.retry_policy.max_attempts}x` : "",
    profile.health_policy?.doctor_agent ? `doctor ${profile.health_policy.doctor_agent}` : "",
    profile.self_repair?.enabled ? `self-repair${profile.self_repair.max_attempts ? ` ${profile.self_repair.max_attempts}x` : ""}` : "",
    profile.self_repair?.escalate_to ? `escalate ${profile.self_repair.escalate_to}` : "",
  ].filter(Boolean);
  if (parts.length > 0) {
    return { value: "repair guarded", detail: parts.join(" · "), tone: "good" };
  }
  return { value: "manual repair", detail: "no retry, doctor, or self-repair policy configured", tone: "warn" };
}

function fleetAgentHealthOpsSummary(
  e: Pick<FleetEntity, "running" | "state" | "retired">,
  runtime: ReturnType<typeof summarizeAgentRuntimeStatus> | null,
): { value: string; detail: string; tone: "good" | "warn" | "bad" | "accent" | "muted" } {
  if (e.retired || e.state === "retired") return { value: "graveyard", detail: "retired identity is inspectable but asleep", tone: "muted" };
  if (e.running) return { value: runtime?.activePhase || runtime?.liveText || "awake", detail: runtime?.liveDetail || "live execution", tone: "accent" };
  if (runtime?.healthText) {
    return {
      value: runtime.healthText,
      detail: runtime.configIssues?.length ? runtime.configIssues.join(" · ") : runtime.lastActivitySummary || runtime.healthText,
      tone: runtime.healthTone === "bad" ? "bad" : runtime.healthTone === "muted" ? "muted" : "good",
    };
  }
  if (e.state === "paused") return { value: "paused", detail: "wake disabled", tone: "warn" };
  return { value: "nominal", detail: runtime?.lastActivitySummary || "no health pressure reported", tone: "good" };
}

export function fleetAgentLiveOpsSummary(
  e: Pick<FleetEntity, "kind" | "state" | "running" | "retired" | "nextRunMs">,
  runtime: ReturnType<typeof summarizeAgentRuntimeStatus> | null,
  trigger?: { mode: TriggerMode; label: string },
  repair?: { value: string; detail: string; tone: "good" | "warn" | "bad" | "accent" | "muted" } | null,
): { value: string; detail: string; tone: "good" | "warn" | "bad" | "accent" | "muted" } {
  const wake = wakeStateDescriptor(e, trigger);
  const health = fleetAgentHealthOpsSummary(e, runtime);
  const next = runtime?.nextWakeMs || e.nextRunMs || 0;
  const nextLine = next ? `next ${fmtDateTime(next)}` : "";
  const repairLine = repair ? `${repair.value}: ${repair.detail}` : "";
  const detail = [
    wake.detail,
    health.detail,
    repairLine,
    nextLine,
    runtime?.activeModel ? `model ${runtime.activeModel}` : "",
    runtime?.activeTool ? `tool ${runtime.activeTool}` : "",
  ].filter(Boolean).join(" · ");
  const value = e.running
    ? `awake · ${runtime?.activePhase || runtime?.liveText || "running"}`
    : e.retired || e.state === "retired"
      ? "graveyard · asleep"
      : e.state === "paused"
        ? "paused · asleep"
        : wake.mode === "armed"
          ? "sleeping · armed"
          : "sleeping · manual";
  const tone =
    e.running ? "accent" :
      health.tone === "bad" || repair?.tone === "bad" ? "bad" :
        health.tone === "warn" || repair?.tone === "warn" ? "warn" :
          e.retired || e.state === "retired" || e.state === "paused" ? "muted" :
            wake.tone === "good" ? "good" : "muted";
  return { value, detail, tone };
}

// Per-archetype identity: a label, an icon, and a hue so each kind has its own
// colour (the owner wants the console colourful, not a grid of grey).
const KIND_META: Record<FleetKind, { label: string; icon: LucideIcon; hue: number | null }> = {
  roster: { label: "Roster agent", icon: Users, hue: 210 },
  standing: { label: "Standing order", icon: Anchor, hue: 150 },
  schedule: { label: "Schedule", icon: CalendarClock, hue: 35 },
  workflow: { label: "Workflow", icon: GitFork, hue: 280 },
  system: { label: "System engine", icon: Cpu, hue: null }, // neutral
};

const TRIGGER_ICON: Record<TriggerMode, LucideIcon> = {
  cron: CalendarClock,
  event: Zap,
  webhook: Webhook,
  cadence: Timer,
  continuous: InfinityIcon,
  manual: MousePointerClick,
  delegation: Share2,
};

const STATE_LABEL: Record<FleetState, string> = {
  running: "running",
  armed: "armed",
  manual: "manual",
  paused: "paused",
  retired: "retired",
};

function kindColor(kind: FleetKind): string | undefined {
  const hue = KIND_META[kind].hue;
  return hue == null ? undefined : `hsl(${hue} 60% 55%)`;
}

function StatePill({ state }: { state: FleetState }) {
  const cls =
    state === "running"
      ? "text-accent"
      : state === "armed"
        ? "text-good"
        : state === "paused" || state === "retired"
          ? "text-muted"
          : "text-foreground/70";
  return (
    <span className={cn("inline-flex items-center gap-1 text-xs uppercase tracking-normal", cls)}>
      {state === "running" && <span className="size-1.5 animate-pulse rounded-full bg-accent" />}
      {STATE_LABEL[state]}
    </span>
  );
}

export function wakeStateDescriptor(e: Pick<FleetEntity, "kind" | "state" | "running" | "retired">, trigger?: { mode: TriggerMode; label: string }) {
  if (e.retired || e.state === "retired") {
    return { label: "graveyard", detail: "retired identity", tone: "muted" as const, mode: "retired" as const };
  }
  if (e.state === "paused") {
    return { label: "paused", detail: "wake disabled", tone: "muted" as const, mode: "paused" as const };
  }
  if (e.running) {
    return {
      label: e.kind === "roster" ? "awake now" : "running now",
      detail: trigger ? `${trigger.mode}: ${trigger.label}` : "live execution",
      tone: "live" as const,
      mode: "running" as const,
    };
  }
  if (e.state === "armed") {
    return {
      label: e.kind === "roster" ? "sleeping until trigger" : "armed",
      detail: trigger ? `${trigger.mode}: ${trigger.label}` : "automatic trigger",
      tone: "good" as const,
      mode: "armed" as const,
    };
  }
  return {
    label: e.kind === "roster" ? "sleeping" : "manual",
    detail: trigger ? `${trigger.mode}: ${trigger.label}` : "manual / delegated",
    tone: "plain" as const,
    mode: "manual" as const,
  };
}

function WakeStateBand({ e, trigger }: { e: FleetEntity; trigger?: { mode: TriggerMode; label: string } }) {
  const state = wakeStateDescriptor(e, trigger);
  const Icon =
    state.mode === "running" ? Activity :
      state.mode === "armed" ? Clock :
        state.mode === "manual" ? Moon :
          state.mode === "paused" ? PauseCircle :
            Skull;
  return (
    <div
      className={cn(
        "flex min-h-[3rem] items-center gap-2 rounded-lg px-2 py-1.5",
        state.tone === "live" && "bg-accent/10 text-accent",
        state.tone === "good" && "bg-good/10 text-good",
        state.tone === "muted" && "bg-panel/25 text-muted",
        state.tone === "plain" && "bg-panel/30 text-foreground/75",
      )}
    >
      <Icon className={cn("size-4 shrink-0", state.mode === "running" && "work-pulse")} />
      <div className="min-w-0 flex-1">
        <div className="truncate text-sm font-semibold uppercase tracking-normal">{state.label}</div>
        <div className="truncate font-mono text-xs opacity-80" title={state.detail}>
          {state.detail}
        </div>
      </div>
      <span className="shrink-0 rounded-md bg-card/60 px-1.5 py-0.5 text-xs uppercase tracking-normal opacity-80">
        {STATE_LABEL[e.state]}
      </span>
    </div>
  );
}

// TriggerChip — the answer to "what makes this run", compact for cards.
export function TriggerChip({ mode, label }: { mode: TriggerMode; label: string }) {
  const Icon = TRIGGER_ICON[mode];
  return (
    <span
      title={`${mode} · ${label}`}
      className="inline-flex max-w-full items-center gap-1 rounded-md bg-panel/50 px-1.5 py-0.5 text-xs text-foreground/80"
    >
      <Icon className="size-2.5 shrink-0 text-muted" />
      <span className="truncate font-mono">{label}</span>
    </span>
  );
}

// FleetCard — one agent/automation, whatever its kind, at a glance: identity,
// type, state, and how it gets triggered.
export function FleetCard({
  e,
  onOpen,
  onAction,
}: {
  e: FleetEntity;
  onOpen: () => void;
  /** Called after a quick action (wake / pause / resume) succeeds, so the
   *  parent can refetch. Optional — live SSE refetch covers it otherwise. */
  onAction?: () => void;
}) {
  const ui = useUI();
  const [acting, setActing] = useState(false);
  const Kind = KIND_META[e.kind].icon;
  const color = kindColor(e.kind);
  const primaryTrigger = e.triggers[0];
  const runtimeStatus = e.kind === "roster" && e.raw && typeof e.raw === "object"
    ? summarizeAgentRuntimeStatus((e.raw as { status?: Parameters<typeof summarizeAgentRuntimeStatus>[0] }).status)
    : null;
  // The calm signal card surfaces only the essentials (avatar · name · state ·
  // model · last/next) plus ONE collapsed "issues" chip. The full operational
  // ledger (identity, tasks, tools, authority, health, repair) lives on the
  // detail page / Fleet drill-in, reached by clicking the card.
  const issues = fleetCardIssueSummary(runtimeStatus);
  const metric = e.nextRunMs
    ? { label: "next wake", value: fmtDateTime(e.nextRunMs) }
    : { label: "last run", value: e.lastRunMs ? fmtAgo(e.lastRunMs) : "never" };

  // Per-card quick actions for roster agents — wake now, pause/resume — so the
  // operator never has to open the detail page for the common moves.
  const canAct = e.kind === "roster" && !e.retired;
  async function runAction(ev: ReactMouseEvent, fn: () => Promise<unknown>, okMsg: string) {
    ev.stopPropagation();
    if (acting) return;
    setActing(true);
    try {
      await fn();
      ui.toast(okMsg, "success");
      onAction?.();
    } catch (err) {
      ui.toast((err as Error).message, "error");
    } finally {
      setActing(false);
    }
  }
  const wake = (ev: ReactMouseEvent) =>
    runAction(ev, () => postAction("/api/agents/wake", { ref: e.slug, reason: "manual operator wake" }), `${e.name} wake queued`);
  const toggleEnabled = (ev: ReactMouseEvent) => {
    const next = !e.enabled;
    return runAction(
      ev,
      () => postAction("/api/agents/enable", { ref: e.slug, enabled: next ? "true" : "false" }),
      `${e.name} ${next ? "resumed" : "paused"}`,
    );
  };
  const onCardKey = (ev: ReactKeyboardEvent) => {
    if (ev.key === "Enter" || ev.key === " ") {
      ev.preventDefault();
      onOpen();
    }
  };

  return (
    <div
      role="button"
      tabIndex={0}
      onClick={onOpen}
      onKeyDown={onCardKey}
      title={e.kind === "roster" ? `Open ${e.name}'s identity page` : `Open ${e.name}`}
      className={cn(
        "card-lift group flex min-h-[230px] cursor-pointer flex-col overflow-hidden rounded-xl bg-card text-left shadow-e1",
        e.running ? "border border-accent/60" : "border border-border",
        (e.state === "paused" || e.state === "retired") && "opacity-60",
      )}
    >
      <div className="flex items-start gap-2 border-b border-border/50 bg-panel/35 p-3">
        <AgentAvatar
          slug={e.slug}
          name={e.name}
          size={34}
          status={e.running ? "running" : e.retired ? "retired" : undefined}
        />
        <div className="min-w-0 flex-1">
          <div className="truncate text-sm font-semibold leading-tight" title={e.name}>
            {e.name}
          </div>
          <div className="mt-1 flex flex-wrap items-center gap-1">
            <span
              className="inline-flex max-w-full items-center gap-1 rounded-md bg-card/70 px-1.5 py-0.5 text-xs font-medium"
              style={color ? { color } : undefined}
            >
              <Kind className="size-2.5 shrink-0" /> <span className="truncate">{KIND_META[e.kind].label}</span>
            </span>
            {e.system && (
              <span className="inline-flex items-center gap-0.5 rounded-md bg-accent/15 px-1.5 py-0.5 text-xs font-medium text-accent" title="Shipped internal guardian — part of the daemon's self-healing fleet">
                <ShieldCheck className="size-2.5" /> guardian
              </span>
            )}
            {issues.count > 0 && (
              <span
                className={cn(
                  "inline-flex items-center gap-0.5 rounded-md px-1.5 py-0.5 text-xs font-medium",
                  issues.tone === "bad" && "bg-bad/10 text-bad",
                  issues.tone === "warn" && "bg-warn/10 text-warn",
                  issues.tone === "accent" && "bg-accent/10 text-accent",
                )}
                title={issues.detail}
              >
                {issues.tone === "accent" ? <Activity className="size-2.5 work-pulse" /> : <AlertTriangle className="size-2.5" />}
                {issues.count}
              </span>
            )}
          </div>
        </div>
        <div className="shrink-0 pt-0.5">
          <StatePill state={e.state} />
        </div>
        {e.kind === "roster" && (
          <ArrowUpRight
            className="mt-0.5 size-3.5 shrink-0 text-muted opacity-0 transition-opacity group-hover:opacity-100"
            aria-hidden="true"
          />
        )}
      </div>

      <div className="flex flex-1 flex-col gap-2 p-3">
        <WakeStateBand e={e} trigger={primaryTrigger} />

        <div className="grid grid-cols-2 gap-1.5">
          <FleetInfo label="model" value={e.model || "default"} mono />
          <FleetInfo label={metric.label} value={metric.value} />
        </div>

        {e.description ? (
          <div className="mt-auto line-clamp-2 rounded-lg bg-panel/40 px-2 py-1.5 text-sm text-muted" title={e.description}>
            {clip(e.description, 140)}
          </div>
        ) : (
          <div className="mt-auto rounded-lg border border-dashed border-border/50 bg-panel/20 px-2 py-1.5 text-sm text-muted">
            no description
          </div>
        )}

        {canAct && (
          <div className="flex gap-1.5">
            <button
              type="button"
              onClick={wake}
              disabled={acting}
              title={`Wake ${e.name} now`}
              className="inline-flex flex-1 items-center justify-center gap-1 rounded-md border border-accent/40 bg-accent/10 px-2 py-1 text-sm font-medium text-accent transition-colors hover:bg-accent/20 disabled:opacity-50"
            >
              <Zap className="size-3" /> Wake
            </button>
            <button
              type="button"
              onClick={toggleEnabled}
              disabled={acting}
              title={e.enabled ? "Pause automatic wakes" : "Resume automatic wakes"}
              className="inline-flex items-center justify-center gap-1 rounded-md border border-border bg-panel/50 px-2 py-1 text-sm font-medium text-muted transition-colors hover:text-foreground disabled:opacity-50"
            >
              {e.enabled ? (
                <>
                  <PauseCircle className="size-3" /> Pause
                </>
              ) : (
                <>
                  <Play className="size-3" /> Resume
                </>
              )}
            </button>
          </div>
        )}
      </div>

      <div className="flex items-center gap-2 border-t border-border px-3 py-2 text-xs text-muted">
        <span className="truncate font-mono" title={e.slug}>
          {e.slug}
        </span>
        {typeof e.fires === "number" && e.fires > 0 && (
          <span className="inline-flex shrink-0 items-center gap-1 rounded-md border border-border bg-panel/40 px-1.5 py-0.5">
            <Flame className="size-2.5" /> {e.fires}
          </span>
        )}
        {typeof e.raw === "object" && e.raw && typeof (e.raw as Record<string, unknown>)["max_cost_mc"] === "number" && (
          <span className="ml-auto inline-flex shrink-0 items-center gap-1 rounded-md border border-border bg-panel/40 px-1.5 py-0.5">
            <Coins className="size-2.5" /> {money((e.raw as Record<string, unknown>)["max_cost_mc"] as number)}
          </span>
        )}
      </div>
    </div>
  );
}

function FleetInfo({ label, value, mono }: { label: string; value?: string; mono?: boolean }) {
  return (
    <div className="min-w-0 rounded-lg bg-panel/35 px-2 py-1.5">
      <div className="text-xs font-semibold uppercase tracking-normal text-muted">{label}</div>
      <div className={cn("truncate text-sm text-foreground/85", mono && "font-mono")} title={value || ""}>
        {value || "-"}
      </div>
    </div>
  );
}

function Row({ label, value }: { label: string; value: React.ReactNode }) {
  if (value == null || value === "") return null;
  return (
    <div className="flex gap-2 text-sm">
      <span className="w-24 shrink-0 text-muted">{label}</span>
      <span className="min-w-0 flex-1 break-words">{value}</span>
    </div>
  );
}

// FleetDetail — the drill-in. Leads with a plain-language "How does this run?"
// block (the whole point of the page), then kind-specific config, then a deep
// link to the page that manages this kind.
export function FleetDetail({
  e,
  onClose,
  onManage,
  onLive,
}: {
  e: FleetEntity;
  onClose: () => void;
  onManage: (view: string) => void;
  onLive?: () => void;
}) {
  const Kind = KIND_META[e.kind].icon;
  const color = kindColor(e.kind);
  const raw = e.raw as Record<string, unknown> | null;
  const profile = e.kind === "roster" && e.raw && typeof e.raw === "object" ? (e.raw as ApiProfile) : null;
  const runtimeStatus = profile
    ? summarizeAgentRuntimeStatus((profile as { status?: Parameters<typeof summarizeAgentRuntimeStatus>[0] }).status)
    : null;
  const repairOps = profile ? fleetAgentRepairOpsSummary(profile, runtimeStatus) : null;
  const healthOps = profile ? fleetAgentHealthOpsSummary(e, runtimeStatus) : null;
  const liveOps = profile ? fleetAgentLiveOpsSummary(e, runtimeStatus, e.triggers[0], repairOps) : null;
  const str = (k: string) => (raw && typeof raw[k] === "string" ? (raw[k] as string) : undefined);
  const num = (k: string) => (raw && typeof raw[k] === "number" ? (raw[k] as number) : undefined);

  return (
    <aside className="flex min-h-0 flex-col gap-3 overflow-auto rounded-lg bg-card p-3 shadow-e1 lg:w-96 lg:shrink-0">
      <div className="flex items-start gap-2">
        <AgentAvatar slug={e.slug} name={e.name} size={36} status={e.running ? "running" : e.retired ? "retired" : undefined} />
        <div className="min-w-0 flex-1">
          <div className="truncate text-sm font-semibold" title={e.name}>
            {e.name}
          </div>
          <div className="flex items-center gap-1.5 text-xs" style={color ? { color } : undefined}>
            <Kind className="size-3" /> {KIND_META[e.kind].label}
            <StatePill state={e.state} />
          </div>
        </div>
        <button
          onClick={onClose}
          className="shrink-0 rounded-md p-1 text-muted hover:bg-panel hover:text-foreground"
          title="Close"
        >
          <X className="size-3.5" />
        </button>
      </div>

      {e.description && <p className="text-sm text-muted">{e.description}</p>}

      {/* The hero: how this comes alive. */}
      <div className="rounded-lg bg-accent/8 p-2.5">
        <div className="mb-1.5 flex items-center gap-1.5 text-xs font-semibold uppercase tracking-normal text-accent">
          <Activity className="size-3" /> How does this run?
        </div>
        <div className="space-y-2">
          {e.triggers.map((t, i) => {
            const Icon = TRIGGER_ICON[t.mode];
            return (
              <div key={`${t.mode}-${i}`} className="flex gap-2">
                <Icon className="mt-0.5 size-3.5 shrink-0 text-accent" />
                <div className="min-w-0">
                  <div className="text-sm font-medium">
                    {t.mode}
                    {t.label && t.label !== t.mode ? <span className="ml-1 font-mono text-muted">· {t.label}</span> : null}
                    {t.via ? <span className="ml-1 text-muted">via {t.via}</span> : null}
                  </div>
                  <div className="text-sm text-muted">{t.needs}</div>
                </div>
              </div>
            );
          })}
          {e.nextRunMs && (
            <div className="text-sm text-muted">
              Next fire: <span className="font-mono text-foreground/80">{fmtDateTime(e.nextRunMs)}</span>
            </div>
          )}
          {e.kind === "workflow" && (e.raw as Record<string, unknown>)?.["trigger_kind"] === "webhook" && (
            <div className="text-sm text-muted">
              Endpoint: <span className="font-mono text-foreground/80">POST /api/workflows/webhook</span> — exact URL &amp; secret
              live in Flow Studio.
            </div>
          )}
        </div>
      </div>

      {/* Kind-specific config. */}
      <div className="space-y-1.5 rounded-lg bg-panel/30 p-2.5">
        {e.kind === "roster" && (
          <>
            {profile && (
              <>
                <Row
                  label="live ops"
                  value={
                    <span className="inline-flex items-center gap-1">
                      <Activity className="size-3 text-muted" /> {liveOps?.value}
                    </span>
                  }
                />
                <Row
                  label="identity"
                  value={
                    <span className="inline-flex items-center gap-1">
                      <IdCard className="size-3 text-muted" /> {fleetAgentHierarchyLabel(profile)}
                    </span>
                  }
                />
                <Row
                  label="tasks"
                  value={
                    <span className="inline-flex items-center gap-1">
                      <ListChecks className="size-3 text-muted" /> {fleetAgentTaskContractLabel(profile)}
                    </span>
                  }
                />
                <Row
                  label="tools"
                  value={
                    <span className="inline-flex items-center gap-1 font-mono">
                      <ShieldCheck className="size-3 text-muted" /> {fleetAgentCapabilityLabel(profile)}
                    </span>
                  }
                />
                <Row
                  label="authority"
                  value={
                    <span className="inline-flex items-center gap-1">
                      <ShieldCheck className="size-3 text-muted" /> {fleetAgentAuthorityLabel(profile)}
                    </span>
                  }
                />
                <Row
                  label="repair"
                  value={
                    <span className="inline-flex items-center gap-1">
                      <Wrench className="size-3 text-muted" /> {fleetAgentResilienceLabel(profile)}
                    </span>
                  }
                />
                <Row
                  label="health ops"
                  value={
                    healthOps && (
                      <span className="inline-flex items-center gap-1" title={healthOps.detail}>
                        <Activity className="size-3 text-muted" /> {healthOps.value}
                      </span>
                    )
                  }
                />
                <Row
                  label="repair ops"
                  value={
                    repairOps && (
                      <span
                        className={cn(
                          "inline-flex items-center gap-1",
                          repairOps.tone === "bad" && "text-bad",
                          repairOps.tone === "accent" && "text-accent",
                          repairOps.tone === "good" && "text-good",
                          repairOps.tone === "warn" && "text-warn",
                        )}
                        title={repairOps.detail}
                      >
                        <Wrench className="size-3" /> {repairOps.value}
                      </span>
                    )
                  }
                />
              </>
            )}
            <Row label="model" value={e.model && <span className="font-mono">{e.model}</span>} />
            <Row label="memory" value={str("memory_scope")} />
            <Row label="workdir" value={str("workdir") && <span className="font-mono">{str("workdir")}</span>} />
            <Row label="max / run" value={num("max_cost_mc") ? money(num("max_cost_mc")) : undefined} />
            <Row label="last run" value={e.lastRunMs ? fmtAgo(e.lastRunMs) : "never"} />
            {str("soul") && (
              <div>
                <div className="mb-1 text-xs uppercase tracking-normal text-muted">soul</div>
                <pre className="max-h-40 overflow-auto whitespace-pre-wrap rounded-md bg-card p-2 font-mono text-xs text-foreground/80">
                  {str("soul")}
                </pre>
              </div>
            )}
          </>
        )}
        {e.kind === "standing" && (
          <>
            <Row label="runs as" value={str("agent") && <span className="font-mono">{str("agent")}</span>} />
            <Row
              label="initiative"
              value={
                raw && raw["initiative"] && typeof (raw["initiative"] as Record<string, unknown>)["mode"] === "string"
                  ? ((raw["initiative"] as Record<string, unknown>)["mode"] as string)
                  : undefined
              }
            />
            <Row label="plan" value={str("plan")} />
          </>
        )}
        {e.kind === "schedule" && (
          <>
            <Row label="cadence" value={str("cadence")} />
            <Row label="mode" value={str("mode") || "interval"} />
            <Row label="source" value={str("source")} />
            <Row label="next fire" value={e.nextRunMs ? fmtDateTime(e.nextRunMs) : "—"} />
            <Row label="fires" value={typeof e.fires === "number" ? String(e.fires) : undefined} />
            <Row label="last" value={str("last_status")} />
            <Row label="intent" value={str("intent")} />
          </>
        )}
        {e.kind === "workflow" && (
          <>
            <Row label="trigger" value={`${str("trigger_kind") || "manual"}${str("trigger_detail") ? ` (${str("trigger_detail")})` : ""}`} />
            <Row label="nodes" value={typeof num("node_count") === "number" ? String(num("node_count")) : undefined} />
          </>
        )}
        {e.kind === "system" && (
          <Row label="status" value={e.running ? "running" : e.state} />
        )}
      </div>

      <div className="flex flex-wrap gap-2">
        {e.running && onLive && (
          <Button variant="ghost" size="sm" onClick={onLive}>
            <Activity className="size-3.5" /> View live
          </Button>
        )}
        {e.manageView && (
          <Button variant="ghost" size="sm" onClick={() => onManage(e.manageView)} className="ml-auto">
            Manage <ArrowUpRight className="size-3.5" />
          </Button>
        )}
      </div>
    </aside>
  );
}
