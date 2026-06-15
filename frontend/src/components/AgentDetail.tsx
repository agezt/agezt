import { useEffect, useMemo, useState } from "react";
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
} from "lucide-react";
import { getJSON, postAction, postJSON } from "@/lib/api";
import { cn, fmtTime, fmtDateTime, fmtAgo, clip } from "@/lib/utils";
import { money } from "@/lib/format";
import { Button } from "@/components/ui/button";
import { Badge, statusVariant } from "@/components/ui/badge";
import { SkeletonList } from "@/components/ui/skeleton";
import { EmptyState } from "@/components/ui/empty";
import { ErrorText } from "@/components/JsonView";
import { useUI } from "@/components/ui/feedback";
import { useEvents } from "@/lib/events";
import { AgentAvatar } from "@/components/AgentAvatar";
import { AgentActivity } from "@/components/AgentActivity";
import { AgentRepair } from "@/components/AgentRepair";
import { ModelPicker } from "@/components/ModelPicker";
import { ModelChip } from "@/components/ModelChip";
import { TriggerChip } from "@/components/Fleet";
import { openAgent } from "@/lib/agentnav";
import type { AgentProfile } from "@/views/Roster";
import { scheduleAgentSlug, type FleetTrigger, type FleetState, type ApiOrder, type ApiSchedule } from "@/lib/fleet";
import {
  agentScope,
  agentCorrelations,
  filterByCorrelation,
  filterAgentMemory,
  filterAgentSkills,
  summarizeAgent,
  lastFailure,
  type MemoryRecord,
  type SkillLite,
  type RunLite,
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
interface PolicyStats {
  denial_rate?: number;
  denied?: number;
  hard_denied?: number;
  allow_rate?: number;
  allowed?: number;
  total?: number;
}

// Board message (mirror of /api/board rows) for the Comms tab. The read view
// emits ts_unix_ms (not ts_ms) and omits from/to when empty.
interface BoardMessage {
  id?: string;
  topic?: string;
  from?: string;
  to?: string;
  text?: string;
  ts_unix_ms?: number;
  help?: boolean;
}
// Routing snapshot (mirror of /api/routing) for the Model tab.
interface RoutingInfo {
  chains?: Record<string, string[]>;
}
// Provider-log row (mirror of /api/provider_log `events`): a global routing
// decision (kind "route") or a fallback hop (kind "fallback"). These are
// daemon-wide (no per-agent correlation), so the Model tab surfaces the ones
// relevant to THIS agent's model/task rather than attributing them by run.
interface ProviderLogRow {
  ts_unix_ms?: number;
  kind?: string; // "route" | "fallback"
  primary?: string;
  chain?: string;
  task_type?: string;
  failed?: string;
  next?: string;
  reason?: string;
  scope?: string;
}

type DetailTab = "overview" | "soul" | "triggers" | "model" | "activity" | "comms" | "memory" | "skills" | "diag" | "files" | "repair";

const TABS: { id: DetailTab; label: string; icon: typeof Bot }[] = [
  { id: "overview", label: "Overview", icon: Bot },
  { id: "soul", label: "Soul", icon: ScrollText },
  { id: "triggers", label: "Triggers", icon: Anchor },
  { id: "model", label: "Model", icon: Cpu },
  { id: "activity", label: "Activity", icon: ActivityIcon },
  { id: "comms", label: "Comms", icon: Mail },
  { id: "memory", label: "Memory", icon: Brain },
  { id: "skills", label: "Skills", icon: Sparkles },
  { id: "diag", label: "Diagnostics", icon: ShieldCheck },
  { id: "files", label: "Files", icon: FolderOpen },
  { id: "repair", label: "Repair", icon: Wrench },
];

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
}) {
  const ui = useUI();
  const { subscribe } = useEvents();
  const [tab, setTab] = useState<DetailTab>("overview");
  const [bump, setBump] = useState(0);
  const [busy, setBusy] = useState(false);

  // Aux data fetched best-effort on mount / agent change / live nudge. One
  // failing endpoint never blanks the rest (Promise.allSettled).
  const [memory, setMemory] = useState<MemoryRecord[] | null>(null);
  const [skills, setSkills] = useState<SkillLite[] | null>(null);
  const [policy, setPolicy] = useState<PolicyDecision[] | null>(null);
  const [tools, setTools] = useState<ToolInvocation[] | null>(null);
  const [posture, setPosture] = useState<PolicyStats | null>(null);
  const [askPolicy, setAskPolicy] = useState<string | null>(null);
  const [board, setBoard] = useState<BoardMessage[] | null>(null);
  const [routing, setRouting] = useState<RoutingInfo | null>(null);
  const [provLog, setProvLog] = useState<ProviderLogRow[] | null>(null);

  useEffect(() => {
    let alive = true;
    Promise.allSettled([
      getJSON<{ records?: MemoryRecord[] }>("/api/memory"),
      getJSON<{ skills?: SkillLite[] }>("/api/skills"),
      getJSON<{ decisions?: PolicyDecision[] }>("/api/policy_log", { limit: "200" }),
      getJSON<{ invocations?: ToolInvocation[] }>("/api/tool_log", { limit: "200" }),
      getJSON<PolicyStats>("/api/policy"),
      getJSON<{ ask_policy?: string }>("/api/edict_show"),
      getJSON<{ messages?: BoardMessage[] }>("/api/board"),
      getJSON<RoutingInfo>("/api/routing"),
      getJSON<{ events?: ProviderLogRow[] }>("/api/provider_log", { limit: "200" }),
    ]).then((res) => {
      if (!alive) return;
      const [m, sk, pl, tl, po, ed, bd, rt, pv] = res;
      setMemory(m.status === "fulfilled" ? m.value.records || [] : []);
      setSkills(sk.status === "fulfilled" ? sk.value.skills || [] : []);
      setPolicy(pl.status === "fulfilled" ? pl.value.decisions || [] : []);
      setTools(tl.status === "fulfilled" ? tl.value.invocations || [] : []);
      setPosture(po.status === "fulfilled" ? po.value : null);
      setAskPolicy(ed.status === "fulfilled" ? ed.value.ask_policy ?? null : null);
      setBoard(bd.status === "fulfilled" ? bd.value.messages || [] : []);
      setRouting(rt.status === "fulfilled" ? rt.value : null);
      setProvLog(pv.status === "fulfilled" ? pv.value.events || [] : []);
    });
    return () => {
      alive = false;
    };
  }, [slug, bump]);

  // Live: refetch aux on any event attributable to this agent (debounced).
  useEffect(() => {
    let t: ReturnType<typeof setTimeout> | undefined;
    const off = subscribe((e) => {
      if (e.actor !== slug) return;
      if (t) clearTimeout(t);
      t = setTimeout(() => setBump((b) => b + 1), 1500);
    });
    return () => {
      if (t) clearTimeout(t);
      off();
    };
  }, [slug, subscribe]);

  const corrs = useMemo(() => agentCorrelations(runs, slug), [runs, slug]);
  const myMemory = useMemo(() => (memory ? filterAgentMemory(memory, slug, profile.memory_scope) : null), [memory, slug, profile.memory_scope]);
  const mySkills = useMemo(() => (skills ? filterAgentSkills(skills, slug) : null), [skills, slug]);
  const myDenials = useMemo(
    () => (policy ? filterByCorrelation(policy, corrs, slug).filter((d) => d.allow === false) : null),
    [policy, corrs, slug],
  );
  const myToolErrors = useMemo(
    () => (tools ? filterByCorrelation(tools, corrs, slug).filter((t) => t.error) : null),
    [tools, corrs, slug],
  );
  const myOrders = useMemo(() => orders.filter((o) => o.agent === slug), [orders, slug]);
  const mySchedules = useMemo(() => schedules.filter((s) => scheduleAgentSlug(s.intent) === slug), [schedules, slug]);
  const summary = useMemo(() => summarizeAgent(runs, slug), [runs, slug]);
  const fail = useMemo(() => lastFailure(runs, slug), [runs, slug]);
  // Comms: board messages this agent sent, received, or broadcast.
  const myComms = useMemo(
    () => (board ? board.filter((m) => m.from === slug || m.to === slug || m.to === "*").sort((a, b) => (b.ts_unix_ms || 0) - (a.ts_unix_ms || 0)) : null),
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
        (mdl && (r.primary === mdl || r.failed === mdl || r.next === mdl || (r.chain || "").split(",").includes(mdl))) ||
        (tt && r.task_type === tt),
    );
    // If nothing is attributable but routing is happening, still show the latest
    // few fallbacks so "is my model failing over" is answerable.
    return rel.length > 0 ? rel : provLog.filter((r) => r.kind === "fallback");
  }, [provLog, profile.model, profile.task_type]);

  async function action(path: string, params: Record<string, string>, success: string) {
    setBusy(true);
    try {
      await postAction(path, params);
      ui.toast(success, "success");
      setBump((b) => b + 1);
    } catch (e) {
      ui.toast((e as Error).message, "error");
    } finally {
      setBusy(false);
    }
  }

  const running = state === "running";

  return (
    <section className="flex min-h-0 flex-col gap-3 rounded-lg border border-border bg-card p-3">
      {/* Header */}
      <div className="flex items-start gap-2">
        <AgentAvatar slug={slug} name={profile.name} size={40} status={running ? "running" : profile.retired ? "retired" : undefined} />
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-2">
            <span className="font-mono text-sm font-semibold text-foreground">{slug}</span>
            {profile.name && profile.name !== slug && <span className="text-xs text-muted">{profile.name}</span>}
            <StatePill state={state} />
          </div>
          {profile.description && <p className="mt-0.5 text-[11px] text-muted">{profile.description}</p>}
        </div>
        <div className="flex shrink-0 items-center gap-1">
          {running && onLive && (
            <Button variant="ghost" size="sm" onClick={onLive} title="View live delegation tree">
              <ActivityIcon className="size-3.5" /> Live
            </Button>
          )}
          {!profile.retired && (
            <Button
              variant="ghost"
              size="sm"
              disabled={busy}
              title={profile.enabled ? "Pause agent" : "Resume agent"}
              onClick={() =>
                action("/api/agents/enable", { ref: slug, enabled: profile.enabled ? "false" : "true" }, profile.enabled ? `${slug} paused` : `${slug} resumed`)
              }
            >
              {profile.enabled ? <Pause className="size-3.5" /> : <Play className="size-3.5" />}
            </Button>
          )}
          {!page && (
            <Button variant="ghost" size="sm" onClick={() => openAgent(slug)} title="Open this agent's full identity page">
              <ArrowUpRight className="size-3.5" /> Page
            </Button>
          )}
          {!page && (
            <button
              onClick={onClose}
              className="rounded-md border border-border p-1 text-muted hover:border-accent hover:text-foreground"
              title="Close"
            >
              <X className="size-3.5" />
            </button>
          )}
        </div>
      </div>

      {/* Tabs */}
      <div className="flex flex-wrap items-center gap-1 border-b border-border pb-1">
        {TABS.map((t) => {
          const count = tabCount(t.id, {
            memory: myMemory,
            skills: mySkills,
            orders: myOrders,
            schedules: mySchedules,
            comms: myComms,
            denials: myDenials,
            toolErrors: myToolErrors,
          });
          return (
            <button
              key={t.id}
              onClick={() => setTab(t.id)}
              className={cn(
                "flex items-center gap-1 rounded px-2 py-1 text-[11px] font-medium transition-colors",
                tab === t.id ? "bg-panel text-foreground" : "text-muted hover:text-foreground",
              )}
            >
              <t.icon className="size-3" />
              {t.label}
              {count !== undefined && count > 0 && (
                <span className="ml-0.5 rounded bg-card px-1 font-mono text-[9px] text-muted">{count}</span>
              )}
            </button>
          );
        })}
      </div>

      <div className="min-h-0 overflow-auto">
        {tab === "overview" && (
          <Overview
            slug={slug}
            profile={profile}
            triggers={triggers}
            summary={summary}
            runs={runs}
            fail={fail}
            onManage={onManage}
            onView={setTab}
          />
        )}

        {tab === "soul" && (
          <div className="space-y-2">
            <Row label="task type" value={profile.task_type || "—"} />
            <Row label="memory scope" value={<span className="font-mono">{agentScope(slug, profile.memory_scope)}</span>} />
            <Row label="workdir" value={profile.workdir ? <span className="font-mono">{profile.workdir}</span> : "—"} />
            <div>
              <div className="mb-1 text-[10px] uppercase tracking-wider text-muted">soul — its system prompt</div>
              {profile.soul ? (
                <pre className="max-h-[28rem] overflow-auto whitespace-pre-wrap rounded-md bg-panel p-2.5 font-mono text-[11px] text-foreground/85">
                  {profile.soul}
                </pre>
              ) : (
                <div className="text-xs text-muted">no soul set — this agent runs with the daemon default persona</div>
              )}
            </div>
            <Button variant="ghost" size="sm" onClick={() => onManage("roster")}>
              Edit in Roster <ArrowUpRight className="size-3.5" />
            </Button>
          </div>
        )}

        {tab === "triggers" && (
          <TriggersTab orders={myOrders} schedules={mySchedules} triggers={triggers} busy={busy} onAction={action} onManage={onManage} />
        )}

        {tab === "model" && <ModelTab slug={slug} profile={profile} routing={routing} provLog={myProvLog} onManage={onManage} />}

        {tab === "activity" && <AgentActivity slug={slug} />}

        {tab === "comms" && <CommsTab slug={slug} messages={myComms} onManage={onManage} />}

        {tab === "memory" && (
          <MemoryTab records={myMemory} scope={agentScope(slug, profile.memory_scope)} busy={busy} onAction={action} onManage={onManage} />
        )}

        {tab === "skills" && <SkillsTab skills={mySkills} busy={busy} onAction={action} onManage={onManage} />}

        {tab === "diag" && (
          <DiagTab posture={posture} askPolicy={askPolicy} denials={myDenials} toolErrors={myToolErrors} fail={fail} />
        )}

        {tab === "files" && <FilesTab workdir={profile.workdir} skills={mySkills} />}

        {tab === "repair" && (
          <AgentRepair
            slug={slug}
            profile={profile}
            fail={fail}
            denials={myDenials}
            toolErrors={myToolErrors}
            runs={summary.runs}
            onApplied={() => setBump((b) => b + 1)}
          />
        )}
      </div>
    </section>
  );
}

// ───────────────────────────── sub-views ─────────────────────────────

function StatePill({ state }: { state: FleetState }) {
  const cls =
    state === "running" ? "text-accent" : state === "armed" ? "text-good" : state === "paused" || state === "retired" ? "text-muted" : "text-foreground/70";
  return (
    <span className={cn("inline-flex items-center gap-1 text-[10px] uppercase tracking-wider", cls)}>
      {state === "running" && <span className="size-1.5 animate-pulse rounded-full bg-accent" />}
      {state}
    </span>
  );
}

function Row({ label, value }: { label: string; value: React.ReactNode }) {
  if (value == null || value === "") return null;
  return (
    <div className="flex gap-2 text-[11px]">
      <span className="w-28 shrink-0 text-muted">{label}</span>
      <span className="min-w-0 flex-1 break-words">{value}</span>
    </div>
  );
}

function Stat({ icon: Icon, label, value, accent }: { icon: typeof Bot; label: string; value: React.ReactNode; accent?: boolean }) {
  return (
    <div className={cn("rounded-lg border bg-panel/40 p-2.5", accent ? "border-accent/50" : "border-border")}>
      <div className="flex items-center gap-1.5 text-[10px] font-semibold uppercase tracking-wider text-muted">
        <Icon className={cn("size-3", accent && "text-accent")} /> {label}
      </div>
      <div className={cn("mt-0.5 text-base font-semibold tabular-nums", accent && "text-accent")}>{value}</div>
    </div>
  );
}

// BudgetBar shows spend against a ceiling (microcents). No ceiling → just the
// amount. Over budget → the bar goes red.
function BudgetBar({ label, spentMc, capMc }: { label: string; spentMc: number; capMc?: number }) {
  const pct = capMc && capMc > 0 ? Math.min(100, (spentMc / capMc) * 100) : 0;
  const over = capMc != null && capMc > 0 && spentMc > capMc;
  return (
    <div className="rounded-lg border border-border bg-panel/40 p-2.5">
      <div className="flex items-center justify-between text-[10px] uppercase tracking-wider text-muted">
        <span>{label}</span>
        <span className="font-mono normal-case text-foreground/80">
          {money(spentMc)}
          {capMc && capMc > 0 ? ` / ${money(capMc)}` : " · no cap"}
        </span>
      </div>
      {capMc != null && capMc > 0 && (
        <div className="mt-1.5 h-1.5 overflow-hidden rounded-full bg-card">
          <div className={cn("h-full rounded-full", over ? "bg-bad" : "bg-accent")} style={{ width: `${Math.max(2, pct)}%` }} />
        </div>
      )}
    </div>
  );
}

function Overview({
  slug,
  profile,
  triggers,
  summary,
  runs,
  fail,
  onManage,
  onView,
}: {
  slug: string;
  profile: AgentProfile;
  triggers: FleetTrigger[];
  summary: { runs: number; totalSpentMc: number; lastStartedMs?: number };
  runs: RunLite[];
  fail?: RunLite;
  onManage: (view: string) => void;
  onView: (t: DetailTab) => void;
}) {
  // Today's spend for this agent (client-side fold over its runs started today).
  const todayMs = useMemo(() => {
    const d = new Date();
    d.setHours(0, 0, 0, 0);
    return d.getTime();
  }, []);
  const todaySpent = useMemo(
    () => runs.filter((r) => r.agent === slug && (r.started_unix_ms || 0) >= todayMs).reduce((s, r) => s + (r.spent_mc || 0), 0),
    [runs, slug, todayMs],
  );

  return (
    <div className="space-y-3">
      {/* How it runs */}
      <div className="rounded-lg border border-accent/40 bg-accent/5 p-2.5">
        <div className="mb-1.5 flex items-center gap-1.5 text-[10px] font-semibold uppercase tracking-wider text-accent">
          <ActivityIcon className="size-3" /> How does this run?
        </div>
        <div className="flex flex-wrap gap-1.5">
          {triggers.length === 0 ? (
            <span className="text-[11px] text-muted">manual / delegated only — runs when you or another agent calls it</span>
          ) : (
            triggers.map((t, i) => <TriggerChip key={`${t.mode}-${i}`} mode={t.mode} label={t.label} />)
          )}
        </div>
      </div>

      {/* Headline stats */}
      <div className="grid grid-cols-2 gap-2 sm:grid-cols-4">
        <Stat icon={Bot} label="runs" value={summary.runs} />
        <Stat icon={Coins} label="total spend" value={money(summary.totalSpentMc)} />
        <Stat icon={Clock} label="last active" value={summary.lastStartedMs ? fmtAgo(summary.lastStartedMs) : "never"} />
        <Stat icon={Flame} label="triggers" value={triggers.length} accent={triggers.length > 0} />
      </div>

      {/* Budgets */}
      <div className="grid gap-2 sm:grid-cols-2">
        <BudgetBar label="today's spend" spentMc={todaySpent} capMc={profile.max_daily_mc} />
        <BudgetBar label="per-run ceiling" spentMc={0} capMc={profile.max_cost_mc} />
      </div>

      {/* Identity */}
      <div className="space-y-1.5 rounded-lg border border-border bg-panel/30 p-2.5">
        <Row label="model" value={profile.model ? <span className="font-mono">{profile.model}</span> : "(daemon default)"} />
        {(profile.fallbacks || []).length > 0 && (
          <Row label="fallbacks" value={<span className="font-mono">{(profile.fallbacks || []).join(" → ")}</span>} />
        )}
        <Row label="task type" value={profile.task_type} />
        <Row label="memory scope" value={<span className="font-mono">{agentScope(slug, profile.memory_scope)}</span>} />
        <Row label="workdir" value={profile.workdir ? <span className="font-mono">{profile.workdir}</span> : undefined} />
      </div>

      {/* Last failure — the "ne bok yedi" headline */}
      {fail && (
        <button
          onClick={() => onView("diag")}
          className="flex w-full items-start gap-2 rounded-lg border border-bad/40 bg-bad/5 p-2.5 text-left"
        >
          <AlertTriangle className="mt-0.5 size-3.5 shrink-0 text-bad" />
          <div className="min-w-0">
            <div className="text-[11px] font-medium text-bad">Most recent failure</div>
            <div className="truncate text-[11px] text-muted" title={fail.status}>
              {clip(fail.correlation_id || "run", 48)} · {fail.started_unix_ms ? fmtTime(fail.started_unix_ms) : "—"} — see Diagnostics
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

function TriggersTab({
  orders,
  schedules,
  triggers,
  busy,
  onAction,
  onManage,
}: {
  orders: ApiOrder[];
  schedules: ApiSchedule[];
  triggers: FleetTrigger[];
  busy: boolean;
  onAction: (path: string, params: Record<string, string>, success: string) => void;
  onManage: (view: string) => void;
}) {
  const [why, setWhy] = useState<string | null>(null);
  return (
    <div className="space-y-3">
      <div className="rounded-lg border border-border bg-panel/30 p-2.5">
        <div className="mb-1.5 text-[10px] uppercase tracking-wider text-muted">how this agent is triggered</div>
        <div className="flex flex-wrap gap-1.5">
          {triggers.length === 0 ? (
            <span className="text-[11px] text-muted">No automatic triggers — runs manually or via delegation.</span>
          ) : (
            triggers.map((t, i) => <TriggerChip key={i} mode={t.mode} label={t.label} />)
          )}
        </div>
      </div>

      {/* Upcoming fires — what this agent WILL do next, from each binding schedule. */}
      {schedules.length > 0 && (
        <div>
          <div className="mb-1 flex items-center gap-1.5 text-[10px] uppercase tracking-wider text-muted">
            <CalendarClock className="size-3" /> upcoming runs
          </div>
          <ul className="space-y-2">
            {schedules.map((s) => (
              <li key={s.id} className="rounded-lg border border-border bg-panel/30 p-2.5">
                <div className="flex flex-wrap items-center gap-2">
                  <Badge variant={s.enabled ? "good" : "default"}>{s.enabled ? "armed" : "paused"}</Badge>
                  <span className="text-xs font-medium">{s.cadence || s.mode || s.id}</span>
                  <Button size="sm" variant="ghost" className="ml-auto" disabled={busy} title="Run now" onClick={() => onAction("/api/schedule/run", { id: s.id }, `ran ${s.id}`)}>
                    <Flame className="size-3.5" />
                  </Button>
                </div>
                <ScheduleForecast id={s.id} fallbackNext={s.next_run_unix} />
              </li>
            ))}
          </ul>
        </div>
      )}

      <div className="text-[10px] uppercase tracking-wider text-muted">standing orders firing this agent</div>
      {orders.length === 0 ? (
        <EmptyState
          icon={Anchor}
          title="No standing orders bind this agent"
          hint="Create a standing order in the Standing page and set it to run as this agent — cron or event triggered."
        />
      ) : (
        <ul className="space-y-2">
          {orders.map((o) => (
            <li key={o.id} className="rounded-lg border border-border bg-panel/30 p-2.5">
              <div className="flex flex-wrap items-center gap-2">
                <Badge variant={o.enabled ? "good" : "default"}>{o.enabled ? "armed" : "paused"}</Badge>
                <span className="text-xs font-medium">{o.name || o.id}</span>
                {o.initiative?.mode && <span className="text-[10px] text-muted">· {o.initiative.mode}</span>}
                <span className="ml-auto flex items-center gap-1">
                  <Button size="sm" variant="ghost" disabled={busy} title="Fire now" onClick={() => onAction("/api/standing/fire", { id: o.id }, `fired ${o.name || o.id}`)}>
                    <Flame className="size-3.5" />
                  </Button>
                  <Button
                    size="sm"
                    variant="ghost"
                    disabled={busy}
                    title={o.enabled ? "Pause" : "Resume"}
                    onClick={() => onAction("/api/standing/enable", { id: o.id, enabled: o.enabled ? "false" : "true" }, o.enabled ? "paused" : "resumed")}
                  >
                    {o.enabled ? <Pause className="size-3.5" /> : <Play className="size-3.5" />}
                  </Button>
                </span>
              </div>
              <div className="mt-1.5 flex flex-wrap gap-1.5">
                {(o.triggers || []).map((t, i) => (
                  <span key={i} className="inline-flex items-center gap-1 rounded-md border border-border bg-card px-1.5 py-0.5 text-[10px] text-foreground/80">
                    {t.type === "event" ? <Zap className="size-2.5 text-muted" /> : <CalendarClock className="size-2.5 text-muted" />}
                    <span className="font-mono">{t.type === "event" ? t.subject : t.schedule}</span>
                  </span>
                ))}
              </div>
              {o.plan && <div className="mt-1.5 text-[11px] text-muted">{clip(o.plan, 200)}</div>}
              <button onClick={() => setWhy(why === o.id ? null : o.id)} className="mt-1.5 text-[10px] text-accent hover:underline">
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
  if (events.length === 0) return <div className="mt-1 text-[11px] text-muted">no history yet</div>;
  return (
    <ul className="mt-1 space-y-1">
      {events.map((e, i) => (
        <li key={e.seq ?? i} className="flex items-center gap-2 text-[11px]">
          <span className="rounded bg-card px-1.5 py-0.5 font-mono text-[10px] text-accent">{(e.kind || "").replace(/^standing\./, "")}</span>
          <span className="ml-auto font-mono text-[10px] text-muted">{fmtTime(e.ts_unix_ms)}</span>
        </li>
      ))}
    </ul>
  );
}

// ScheduleForecast shows the next few fire times of a schedule (the agent's
// near future), via the read-only /api/schedule/test dry-run.
function ScheduleForecast({ id, fallbackNext }: { id: string; fallbackNext?: number }) {
  const [fires, setFires] = useState<number[] | null>(null);
  useEffect(() => {
    let alive = true;
    getJSON<{ forecasts?: { unix?: number }[] }>("/api/schedule/test", { id, count: "4" })
      .then((d) => alive && setFires((d.forecasts || []).map((f) => f.unix || 0).filter(Boolean)))
      .catch(() => alive && setFires([]));
    return () => {
      alive = false;
    };
  }, [id]);
  const list = fires && fires.length ? fires : fallbackNext ? [fallbackNext] : [];
  if (fires === null) return <div className="mt-1.5 text-[10px] text-muted">forecasting…</div>;
  if (list.length === 0) return <div className="mt-1.5 text-[10px] text-muted">no upcoming runs</div>;
  return (
    <div className="mt-1.5 flex flex-wrap gap-1.5">
      {list.map((u, i) => (
        <span key={i} className="inline-flex items-center gap-1 rounded-md border border-border bg-card px-1.5 py-0.5 text-[10px] text-foreground/80">
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
}: {
  slug: string;
  profile: AgentProfile;
  routing: RoutingInfo | null;
  provLog: ProviderLogRow[] | null;
  onManage: (view: string) => void;
}) {
  const { toast } = useUI();
  const taskChain = profile.task_type ? routing?.chains?.[profile.task_type] : undefined;
  const fallbacks = provLog ? provLog.filter((r) => r.kind === "fallback") : null;

  // A named fallback chain (M963): if this agent's model is "@name", expand it to
  // the chain's real models so the page shows what it will actually run, with a
  // health dot per model (M965). Best-effort fetches — absence just hides them.
  const [chains, setChains] = useState<Record<string, string[]>>({});
  const [cat, setCat] = useState<ModelCatalog | null>(null);
  useEffect(() => {
    let live = true;
    getJSON<ChainsState>("/api/chains").then((c) => live && setChains(c.chains || {})).catch(() => {});
    getJSON<ModelCatalog>("/api/catalog").then((c) => live && setCat(c)).catch(() => {});
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
        profile: {
          name: profile.name || "",
          soul: profile.soul || "",
          model,
          fallbacks: isChainRef(model) ? [] : profile.fallbacks || [],
          task_type: profile.task_type || "",
          max_cost_mc: profile.max_cost_mc || 0,
          max_daily_mc: profile.max_daily_mc || 0,
          memory_scope: profile.memory_scope || "",
          workdir: profile.workdir || "",
          description: profile.description || "",
        },
      });
      toast(isChainRef(model) ? `Model set to chain @${chainName(model)}` : model ? `Model set to ${model}` : "Model reset to daemon default", "success");
    } catch (e) {
      toast((e as Error).message, "error");
    } finally {
      setSaving(false);
    }
  }

  const modelIsChain = isChainRef(model);
  const expandedChain = modelIsChain ? chains[chainName(model)] : undefined;

  return (
    <div className="space-y-3">
      <div className="space-y-2 rounded-lg border border-accent/30 bg-accent/5 p-2.5">
        <div className="flex items-center gap-1.5 text-[10px] font-semibold uppercase tracking-wider text-accent">
          <Cpu className="size-3" /> Change model / fallback chain
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <ModelPicker value={model} activeModel="daemon default" onChange={setModel} />
          <Button size="sm" onClick={saveModel} disabled={!dirty || saving}>
            {saving ? "Saving…" : "Save"}
          </Button>
          {dirty && <span className="text-[10px] text-warn">unsaved</span>}
        </div>
        <p className="text-[10px] text-muted">
          Pick a single model, or a <span className="text-accent">⛓ fallback chain</span> (defined under{" "}
          <button className="underline-offset-2 hover:underline" onClick={() => onManage("chains")}>
            Fallback Chains
          </button>
          ). A chain is self-contained — per-agent fallbacks are ignored when one is selected.
        </p>
      </div>
      <div className="space-y-1.5 rounded-lg border border-border bg-panel/30 p-2.5">
        <Row
          label={dirty ? "model (unsaved)" : "primary model"}
          value={
            modelIsChain ? (
              <span className="inline-flex items-center gap-1 font-mono text-accent">
                <Waypoints className="size-3" /> {chainName(model)}
                {expandedChain && <span className="text-muted">· {expandedChain.length} model{expandedChain.length === 1 ? "" : "s"}</span>}
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
          <Row label="chain" value={<span className="text-bad">@{chainName(model)} — no such chain (falls through to default)</span>} />
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
                      <span className="rounded bg-card px-1.5 py-0.5 text-[10px]">{m}</span>
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
          <Row label="task chain (global)" value={<span className="font-mono">{taskChain.join(" → ")}</span>} />
        )}
      </div>

      <div>
        <div className="mb-1 flex items-center gap-1.5 text-[10px] uppercase tracking-wider text-muted">
          <Cpu className="size-3" /> routing &amp; fallbacks
        </div>
        {!provLog ? (
          <SkeletonList count={2} lines={1} />
        ) : provLog.length === 0 ? (
          <div className="text-[11px] text-muted">no routing or fallback events relevant to this agent's model/task</div>
        ) : (
          <ul className="space-y-1">
            {provLog.slice(0, 30).map((r, i) => (
              <li key={i} className="flex items-start gap-2 text-[11px]">
                <span className={cn("rounded px-1.5 py-0.5 font-mono text-[10px]", r.kind === "fallback" ? "bg-bad/15 text-bad" : "bg-card text-foreground/80")}>
                  {r.kind || "route"}
                </span>
                <span className="min-w-0 flex-1 truncate text-muted" title={r.reason || r.chain}>
                  {r.kind === "fallback"
                    ? `${r.failed || "?"} → ${r.next || "?"}${r.reason ? ` — ${clip(r.reason, 60)}` : ""}`
                    : `${r.primary || "?"}${r.task_type ? ` · ${r.task_type}` : ""}`}
                </span>
                <span className="ml-auto shrink-0 font-mono text-[10px] text-muted opacity-70">{fmtTime(r.ts_unix_ms)}</span>
              </li>
            ))}
          </ul>
        )}
        {fallbacks && fallbacks.length > 0 && (
          <div className="mt-1.5 text-[10px] text-bad">
            {fallbacks.length} recent fallback hop(s) — a model in this chain has been failing over.
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


// CommsTab is the agent's mailbox: the board messages it sent, was addressed, or
// received as a broadcast — its communication trail with the rest of the fleet.
function CommsTab({ slug, messages, onManage }: { slug: string; messages: BoardMessage[] | null; onManage: (view: string) => void }) {
  if (!messages) return <SkeletonList count={3} lines={2} />;
  if (messages.length === 0)
    return (
      <EmptyState
        icon={Mail}
        title="No messages"
        hint={`Board posts ${slug} sent, was addressed (to: ${slug}), or received as a broadcast appear here. Agents talk on the Board.`}
      />
    );
  return (
    <div className="space-y-2">
      <ul className="space-y-2">
        {messages.slice(0, 60).map((m, i) => {
          const outbound = m.from === slug;
          return (
            <li key={m.id || i} className="rounded-lg border border-border bg-panel/30 p-2.5">
              <div className="flex flex-wrap items-center gap-2 text-[11px]">
                <Badge variant={outbound ? "good" : "default"}>{outbound ? "sent" : m.to === "*" ? "broadcast" : "received"}</Badge>
                {m.from && <span className="font-mono text-[10px] text-muted">from {m.from}</span>}
                {m.to && <span className="font-mono text-[10px] text-muted">→ {m.to}</span>}
                {m.topic && <span className="rounded bg-card px-1.5 py-0.5 font-mono text-[10px] text-accent">{m.topic}</span>}
                {m.help && <span className="rounded bg-bad/15 px-1.5 py-0.5 text-[10px] text-bad">help</span>}
                <span className="ml-auto font-mono text-[10px] text-muted">{fmtAgo(m.ts_unix_ms)}</span>
              </div>
              {m.text && <div className="mt-1 whitespace-pre-wrap text-[11px] text-muted">{clip(m.text, 280)}</div>}
            </li>
          );
        })}
      </ul>
      <Button variant="ghost" size="sm" onClick={() => onManage("board")}>
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
  onAction: (path: string, params: Record<string, string>, success: string) => void;
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
      <div className="text-[10px] uppercase tracking-wider text-muted">
        private to scope <span className="font-mono text-foreground/80">{scope}</span> · {records.length} record(s)
      </div>
      <ul className="space-y-2">
        {records.map((r) => (
          <li key={r.id} className="rounded-lg border border-border bg-panel/30 p-2.5">
            <div className="flex flex-wrap items-center gap-2">
              {r.type && <span className="rounded bg-card px-1.5 py-0.5 font-mono text-[10px] text-accent">{r.type}</span>}
              {r.subject && <span className="text-xs font-medium">{r.subject}</span>}
              {typeof r.confidence === "number" && <span className="text-[10px] text-muted">conf {r.confidence.toFixed(2)}</span>}
              <span className="ml-auto flex items-center gap-2">
                <span className="font-mono text-[10px] text-muted">{fmtAgo(r.last_seen_ms || r.created_ms)}</span>
                <Button
                  size="sm"
                  variant="ghost"
                  disabled={busy}
                  title="Promote to shared memory"
                  onClick={() => r.id && onAction("/api/memory/promote", { id: r.id }, "promoted to shared")}
                >
                  <Share2 className="size-3.5" />
                </Button>
              </span>
            </div>
            {r.content && <div className="mt-1 whitespace-pre-wrap text-[11px] text-muted">{clip(r.content, 280)}</div>}
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
  onAction: (path: string, params: Record<string, string>, success: string) => void;
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
          <li key={s.id} className="rounded-lg border border-border bg-panel/30 p-2.5">
            <div className="flex flex-wrap items-center gap-2">
              {s.status && <Badge variant={statusVariant(s.status)}>{s.status}</Badge>}
              <span className="text-xs font-medium">{s.name}</span>
              <Button
                size="sm"
                variant="ghost"
                disabled={busy}
                className="ml-auto"
                title="Share with the whole fleet"
                onClick={() => s.id && onAction("/api/skill/share", { id: s.id }, `shared ${s.name}`)}
              >
                <Share2 className="size-3.5" /> Share
              </Button>
            </div>
            {s.description && <div className="mt-1 text-[11px] text-muted">{clip(s.description, 200)}</div>}
            {(s.triggers || []).length > 0 && (
              <div className="mt-1 flex flex-wrap gap-1">
                {(s.triggers || []).map((t, i) => (
                  <span key={i} className="rounded bg-card px-1.5 py-0.5 font-mono text-[10px] text-muted">{t}</span>
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

function DiagTab({
  posture,
  askPolicy,
  denials,
  toolErrors,
  fail,
}: {
  posture: PolicyStats | null;
  askPolicy: string | null;
  denials: PolicyDecision[] | null;
  toolErrors: ToolInvocation[] | null;
  fail?: RunLite;
}) {
  return (
    <div className="space-y-3">
      {/* posture */}
      <div className="grid grid-cols-2 gap-2 sm:grid-cols-4">
        <Stat icon={ShieldCheck} label="ask policy" value={askPolicy || "—"} />
        <Stat
          icon={ShieldCheck}
          label="allowed"
          value={posture?.allow_rate != null ? `${Math.round(posture.allow_rate * 100)}%` : posture?.allowed ?? "—"}
          accent
        />
        <Stat icon={AlertTriangle} label="denial rate" value={posture?.denial_rate != null ? `${Math.round(posture.denial_rate * 100)}%` : "—"} />
        <Stat icon={X} label="hard-denied" value={posture?.hard_denied ?? "—"} />
      </div>
      <p className="text-[10px] text-muted">
        Capabilities default to <span className="text-good">allow</span> — only the denials below were ever blocked. This agent
        can use any tool that isn't explicitly restricted.
      </p>

      {fail && (
        <div className="rounded-lg border border-bad/40 bg-bad/5 p-2.5 text-[11px]">
          <span className="font-medium text-bad">Last failed run:</span>{" "}
          <span className="font-mono text-muted">{clip(fail.correlation_id || "run", 60)}</span> · {fail.started_unix_ms ? fmtTime(fail.started_unix_ms) : "—"}
        </div>
      )}

      {/* denied capabilities */}
      <div>
        <div className="mb-1 text-[10px] uppercase tracking-wider text-muted">capability denials</div>
        {!denials ? (
          <SkeletonList count={2} lines={1} />
        ) : denials.length === 0 ? (
          <div className="text-[11px] text-muted">no capability was denied to this agent</div>
        ) : (
          <ul className="space-y-1">
            {denials.slice(0, 40).map((d, i) => (
              <li key={i} className="flex items-start gap-2 text-[11px]">
                <span className={cn("rounded px-1.5 py-0.5 font-mono text-[10px]", d.hard_denied ? "bg-bad/15 text-bad" : "bg-card text-foreground/80")}>
                  {d.capability || "?"}
                </span>
                <span className="min-w-0 flex-1 truncate text-muted" title={d.reason}>
                  {d.tool ? `${d.tool} — ` : ""}{d.reason || (d.hard_denied ? "hard-denied" : "denied")}
                </span>
                <span className="ml-auto shrink-0 font-mono text-[10px] text-muted opacity-70">{fmtTime(d.ts_unix_ms)}</span>
              </li>
            ))}
          </ul>
        )}
      </div>

      {/* tool errors */}
      <div>
        <div className="mb-1 text-[10px] uppercase tracking-wider text-muted">tool errors</div>
        {!toolErrors ? (
          <SkeletonList count={2} lines={1} />
        ) : toolErrors.length === 0 ? (
          <div className="text-[11px] text-muted">no tool errors recorded for this agent</div>
        ) : (
          <ul className="space-y-1">
            {toolErrors.slice(0, 40).map((t, i) => (
              <li key={i} className="flex items-start gap-2 text-[11px]">
                <span className="rounded bg-bad/15 px-1.5 py-0.5 font-mono text-[10px] text-bad">{t.tool || "?"}</span>
                <span className="min-w-0 flex-1 truncate text-muted" title={t.output}>{clip(t.output || "error", 120)}</span>
                <span className="ml-auto shrink-0 font-mono text-[10px] text-muted opacity-70">{fmtTime(t.ts_unix_ms)}</span>
              </li>
            ))}
          </ul>
        )}
      </div>
    </div>
  );
}

interface SkillFile {
  path?: string;
  size?: number;
}
function FilesTab({ workdir, skills }: { workdir?: string; skills: SkillLite[] | null }) {
  const [openId, setOpenId] = useState<string | null>(null);
  return (
    <div className="space-y-3">
      <Row label="workdir" value={workdir ? <span className="font-mono">{workdir}</span> : "(workspace root — no dedicated subdir)"} />
      <div>
        <div className="mb-1 text-[10px] uppercase tracking-wider text-muted">skill bundle files</div>
        {!skills ? (
          <SkeletonList count={2} lines={1} />
        ) : skills.length === 0 ? (
          <div className="text-[11px] text-muted">this agent owns no private skill bundles</div>
        ) : (
          <ul className="space-y-1.5">
            {skills.map((s) => (
              <li key={s.id} className="rounded-md border border-border bg-panel/30 p-2">
                <button onClick={() => setOpenId(openId === s.id ? null : (s.id ?? null))} className="flex w-full items-center gap-2 text-[11px]">
                  <FolderOpen className="size-3.5 text-muted" />
                  <span className="font-medium">{s.name}</span>
                  <ChevronRight className={cn("ml-auto size-3.5 text-muted transition-transform", openId === s.id && "rotate-90")} />
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
  if (files.length === 0) return <div className="mt-1 pl-5 text-[11px] text-muted">no bundled files</div>;
  return (
    <ul className="mt-1 space-y-0.5 pl-5">
      {files.map((f, i) => (
        <li key={i} className="flex items-center gap-2 font-mono text-[10px] text-muted">
          <span className="truncate">{f.path}</span>
          {typeof f.size === "number" && <span className="ml-auto shrink-0">{f.size}B</span>}
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
