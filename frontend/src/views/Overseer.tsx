import { useEffect, useMemo, useRef, useState } from "react";
import {
  Eye,
  RefreshCw,
  Activity as ActivityIcon,
  Users,
  LifeBuoy,
  ArrowRight,
  CircleDot,
  Megaphone,
  CheckCircle2,
  XCircle,
  GitBranch,
  Scale,
  Radio,
  Stethoscope,
  Pause,
  Play,
  Archive,
  ArchiveRestore,
} from "lucide-react";
import { getJSON, postAction } from "@/lib/api";
import { focusRun } from "@/lib/runfocus";
import { cn, fmtTime } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { ErrorText } from "@/components/JsonView";
import { SkeletonList } from "@/components/ui/skeleton";
import { useEvents, type AgentEvent } from "@/lib/events";
import { buildLiveRunContexts, liveWakeLabel } from "@/lib/liveruncontext";
import { AgentAvatar } from "@/components/AgentAvatar";
import { Page } from "@/components/ui/page";
import { Advanced } from "@/components/ui/disclosure";
import { useUI } from "@/components/ui/feedback";
import { LoadMoreFooter } from "@/components/ui/load-more-footer";
import { incidentEventSummary, isIncidentFamilyEvent } from "@/lib/incidentevents";

// Shapes mirror the read routes this view aggregates — kept loose (all optional)
// so a field the backend drops never crashes the dashboard.
interface Run {
  correlation_id?: string;
  status?: string;
  intent?: string;
  model?: string;
  iters?: number;
  duration_ms?: number;
  started_unix_ms?: number;
  parent_correlation?: string;
}
interface AgentProfile {
  slug: string;
  name?: string;
  model?: string;
  task_type?: string;
  enabled?: boolean;
  retired?: boolean;
}
interface HelpMsg {
  id?: string;
  topic?: string;
  from?: string;
  to?: string;
  text?: string;
  ts_unix_ms?: number;
  help?: boolean;
}

interface OverseerData {
  runs: Run[];
  agents: AgentProfile[];
  help: HelpMsg[];
}

const EMPTY: OverseerData = { runs: [], agents: [], help: [] };

// FLEET_WINDOW is how many fleet rows render at once inside the Advanced
// disclosure. The /api/agents fetch is bounded at 200; the window keeps the
// disclosure from ballooning the DOM on a big roster — a Load-more footer
// grows it client-side. The roster counters up top stay computed over the
// FULL fetched set.
const FLEET_WINDOW = 60;

// Event kinds that change what the dashboard shows — an arrival of any of these
// triggers a debounced refetch so the panels reflect reality within ~1s instead
// of waiting out the fallback poll.
const REFRESH_KINDS = new Set([
  "task.received",
  "task.completed",
  "task.failed",
  "task.continued",
  "subagent.spawned",
  "council.consensus",
  "board.posted",
  "agent.retry",
  "agent.repair",
  "agent.resolve",
  "doctor.auto_repair",
]);

// Overseer is the supervisory dashboard — the "brain that watches" surface
// (M850/M862, live in M867). It folds three existing read routes into one screen
// — what is running now, who is on the roster, who has raised an unanswered call
// for help — and rides the live event stream so it updates as things happen.
// Read-only and conflict-free: it watches, it mutates nothing.
export function Overseer() {
  const [data, setData] = useState<OverseerData>(EMPTY);
  const [err, setErr] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [firstLoad, setFirstLoad] = useState(true);
  const { connected, events, subscribe } = useEvents();
  const debounce = useRef<ReturnType<typeof setTimeout> | null>(null);

  const [busySet, setBusy] = useState<Set<string>>(new Set());
  const [fleetWin, setFleetWin] = useState(FLEET_WINDOW);
  const ui = useUI();

  async function toggleEnabled(ref: string, enabled: boolean) {
    setBusy((prev) => new Set(prev).add(ref));
    try {
      await postAction("/api/agents/enable", { ref, enabled: String(enabled) });
      await reload();
    } catch (e) {
      ui.toast(`Failed to ${enabled ? "resume" : "pause"} ${ref}: ${(e as Error).message}`);
    } finally {
      setBusy((prev) => { const next = new Set(prev); next.delete(ref); return next; });
    }
  }

  async function toggleRetired(ref: string, retired: boolean) {
    setBusy((prev) => new Set(prev).add(ref));
    try {
      if (retired) {
        await postAction("/api/agents/retire", { ref });
      } else {
        await postAction("/api/agents/revive", { ref });
      }
      await reload();
    } catch (e) {
      ui.toast(`Failed to ${retired ? "retire" : "revive"} ${ref}: ${(e as Error).message}`);
    } finally {
      setBusy((prev) => { const next = new Set(prev); next.delete(ref); return next; });
    }
  }

  async function reload() {
    setLoading(true);
    try {
      const [runsRes, agentsRes, helpRes] = await Promise.all([
        getJSON<{ runs?: Run[] }>("/api/runs", { limit: "200" }).catch(() => ({ runs: [] })),
        getJSON<{ profiles?: AgentProfile[] }>("/api/agents", { limit: "200" }).catch(() => ({ profiles: [] })),
        getJSON<{ open_help?: HelpMsg[] }>("/api/board/help").catch(() => ({ open_help: [] })),
      ]);
      setData({
        runs: runsRes.runs || [],
        agents: agentsRes.profiles || [],
        help: helpRes.open_help || [],
      });
      setErr(null);
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setLoading(false);
      setFirstLoad(false);
    }
  }

  // Live: refetch (debounced) on any state-changing event, plus a slow fallback
  // poll so the view still self-heals if the stream drops or an event is missed.
  useEffect(() => {
    reload();
    const poll = setInterval(reload, 15000);
    const unsub = subscribe((e: AgentEvent) => {
      if (!isSignificant(e)) return;
      if (debounce.current) clearTimeout(debounce.current);
      debounce.current = setTimeout(reload, 700);
    });
    return () => {
      clearInterval(poll);
      unsub();
      if (debounce.current) clearTimeout(debounce.current);
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [subscribe]);

  const active = useMemo(
    () => data.runs.filter((r) => (r.status || "running") === "running"),
    [data.runs],
  );
  // corr → responsible agent, learned from the live stream: task.received carries
  // the agent slug in `actor`. Lets each active-run card name WHO is running it.
  // Runs that started before the page loaded have no actor in the buffer → the
  // card just omits the agent chip (graceful, like the rest of the live UI).
  const liveRunContext = useMemo(() => buildLiveRunContexts(events), [events]);
  const live = useMemo(() => {
    const roster = data.agents.filter((a) => !a.retired);
    return { total: roster.length, enabled: roster.filter((a) => a.enabled !== false).length };
  }, [data.agents]);
  // Recent significant events for the "what just happened" ticker, newest first.
  const recent = useMemo(() => events.filter(isSignificant).slice(0, 10), [events]);

  return (
    <Page
      icon={Eye}
      title="Overseer"
      description="The supervisory dashboard — what is running, who is on the roster, who needs help."
      width="wide"
      actions={
        <>
          <span
            className={cn(
              "flex items-center gap-1 rounded-full px-2 py-0.5 text-xs font-medium",
              connected ? "bg-good/10 text-good" : "bg-muted/20 text-muted",
            )}
            title={connected ? "Live event stream connected" : "Stream disconnected — polling"}
          >
            <CircleDot className={cn("size-3", connected && "animate-pulse")} />
            {connected ? "live" : "offline"}
          </span>
          <Button
            variant="ghost"
            size="sm"
            onClick={reload}
            disabled={loading}
            title="Refresh now"
          >
            <RefreshCw className={cn("size-4", loading && "animate-spin")} />
          </Button>
        </>
      }
    >
      {err && <ErrorText>{err}</ErrorText>}

      <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
        <Stat
          icon={<ActivityIcon className="size-4" />}
          label="Active runs"
          value={active.length}
          tone={active.length > 0 ? "accent" : "muted"}
        />
        <Stat
          icon={<Users className="size-4" />}
          label="Agents (enabled / total)"
          value={`${live.enabled} / ${live.total}`}
          tone="muted"
        />
        <Stat
          icon={<LifeBuoy className="size-4" />}
          label="Open help requests"
          value={data.help.length}
          tone={data.help.length > 0 ? "warn" : "muted"}
        />
      </div>

      {firstLoad ? (
        <SkeletonList count={4} />
      ) : (
        <>
        <div className="grid grid-cols-1 gap-3 lg:grid-cols-2">
            <Panel title="Active runs" icon={<ActivityIcon className="size-4 text-accent" />}>
              {active.length === 0 ? (
                <span className="text-xs text-muted">Nothing running right now.</span>
              ) : (
                <ul className="flex flex-col gap-1.5">
                  {active.map((r) => {
                    const corr = r.correlation_id || "";
                    const ctx = corr ? liveRunContext[corr] : undefined;
                    return (
                    <li key={r.correlation_id}>
                      <button
                        type="button"
                        onClick={() => {
                          if (corr) focusRun(corr);
                          location.hash = "runs";
                        }}
                        className="w-full rounded-md border border-border bg-panel/50 px-2.5 py-1.5 text-left shadow-e1 transition-[background-color,border-color,box-shadow] hover:border-accent/50 hover:bg-panel hover:shadow-e2"
                        title="Open this run in Runs"
                      >
                        <div className="flex items-center gap-2">
                          <CircleDot className="size-3.5 shrink-0 animate-pulse text-accent" />
                          <span className="truncate text-xs font-medium" title={r.intent}>
                            {r.intent || r.correlation_id}
                          </span>
                          {r.parent_correlation && (
                            <span className="shrink-0 rounded bg-panel px-1 text-xs text-muted">
                              sub-agent
                            </span>
                          )}
                        </div>
                        <div className="mt-0.5 flex items-center gap-2 text-xs text-muted">
                          {ctx?.agent && (
                            <span className="inline-flex items-center gap-1 rounded bg-accent/10 py-0.5 pl-0.5 pr-1.5 text-accent">
                              <AgentAvatar slug={ctx.agent} size={14} status="running" />
                              {ctx.agent}
                            </span>
                          )}
                          {ctx?.phase && (
                            <span className="rounded bg-panel px-1 text-xs text-foreground/85">
                              {ctx.phase}
                            </span>
                          )}
                          {ctx?.tool && (
                            <span className="font-mono">tool {ctx.tool}</span>
                          )}
                          {ctx?.wakeSource && (
                            <span>
                              · {liveWakeLabel(ctx)}
                            </span>
                          )}
                          {(ctx?.model || r.model) && (
                            <span className="truncate">
                              {ctx?.model || r.model}
                            </span>
                          )}
                          {typeof r.iters === "number" && <span>· {r.iters} iters</span>}
                          {ctx?.lastEventMs ? (
                            <span>· event {fmtTime(ctx.lastEventMs)}</span>
                          ) : null}
                          {r.started_unix_ms ? <span>· started {fmtTime(r.started_unix_ms)}</span> : null}
                        </div>
                      </button>
                    </li>
                    );
                  })}
                </ul>
              )}
            </Panel>

            <Panel title="Needs attention" icon={<LifeBuoy className="size-4 text-amber-400" />}>
              {data.help.length === 0 ? (
                <span className="text-xs text-muted">No open help requests.</span>
              ) : (
                <ul className="flex flex-col gap-1.5">
                  {data.help.map((m, i) => (
                    <li
                      key={m.id || i}
                      className="rounded-md border border-amber-500/30 bg-amber-500/5 px-2.5 py-1.5"
                    >
                      <div className="flex items-center gap-1.5 text-xs text-muted">
                        {m.to === "*" ? (
                          <Megaphone className="size-3 text-amber-400" />
                        ) : (
                          <LifeBuoy className="size-3 text-amber-400" />
                        )}
                        <span className="font-medium text-foreground">{m.from || "agent"}</span>
                        {m.to && m.to !== "*" && (
                          <>
                            <ArrowRight className="size-3" />
                            <span>{m.to}</span>
                          </>
                        )}
                        {m.topic && <span className="ml-auto">#{m.topic}</span>}
                      </div>
                      <p className="mt-0.5 line-clamp-2 text-xs">{m.text}</p>
                    </li>
                  ))}
                </ul>
              )}
            </Panel>
        </div>

        <Advanced label="Agent fleet & quick actions">
          <Panel title="Agent fleet" icon={<Users className="size-4 text-accent" />}>
            {data.agents.length === 0 ? (
              <span className="text-xs text-muted">No agents configured.</span>
            ) : (
              <ul className="flex flex-col gap-1">
                {data.agents.slice(0, fleetWin).map((a) => {
                  const slug = a.slug || "";
                  const retired = a.retired === true;
                  const enabled = a.enabled !== false;
                  const busy = busySet.has(slug);
                  return (
                    <li key={slug} className="flex items-center gap-2 rounded-md border border-border/50 bg-panel/30 px-2.5 py-1.5">
                      <AgentAvatar slug={slug} size={20} status={retired ? "retired" : enabled ? "running" : "paused"} />
                      <div className="min-w-0 flex-1 leading-tight">
                        <span className="text-xs font-medium">{slug}</span>
                        {a.model && <span className="ml-1.5 text-xs text-muted">{a.model}</span>}
                        <span className={"ml-1.5 inline-flex items-center rounded px-1 text-[10px] font-medium " +
                          (retired ? "bg-muted/20 text-muted" : enabled ? "bg-good/10 text-good" : "bg-amber-400/10 text-amber-400")}>
                          {retired ? "retired" : enabled ? "enabled" : "paused"}
                        </span>
                      </div>
                      <div className="flex shrink-0 gap-0.5">
                        {!retired && (
                          <button
                            type="button"
                            disabled={busy}
                            onClick={() => toggleEnabled(slug, !enabled)}
                            className="rounded p-1 text-muted transition-colors hover:bg-panel hover:text-foreground disabled:opacity-40"
                            title={enabled ? "Pause agent" : "Resume agent"}
                          >
                            {enabled ? <Pause className="size-3.5" /> : <Play className="size-3.5" />}
                          </button>
                        )}
                        {retired ? (
                          <button
                            type="button"
                            disabled={busy}
                            onClick={() => toggleRetired(slug, false)}
                            className="rounded p-1 text-muted transition-colors hover:bg-panel hover:text-foreground disabled:opacity-40"
                            title="Revive agent"
                          >
                            <ArchiveRestore className="size-3.5" />
                          </button>
                        ) : (
                          <button
                            type="button"
                            disabled={busy || slug === "owner"}
                            onClick={() => toggleRetired(slug, true)}
                            className="rounded p-1 text-muted transition-colors hover:bg-panel hover:text-foreground disabled:opacity-40"
                            title="Retire agent"
                          >
                            <Archive className="size-3.5" />
                          </button>
                        )}
                      </div>
                    </li>
                  );
                })}
                {data.agents.length > FLEET_WINDOW && (
                  <li>
                    <LoadMoreFooter
                      hasMore={fleetWin < data.agents.length}
                      loadingMore={false}
                      onLoadMore={() => setFleetWin((w) => w + FLEET_WINDOW)}
                      pageSize={Math.min(FLEET_WINDOW, Math.max(1, data.agents.length - fleetWin))}
                      label="fleet"
                    />
                  </li>
                )}
              </ul>
            )}
          </Panel>
        </Advanced>

        <Advanced label="Recent activity feed">
          <Panel title="Recent activity" icon={<Radio className="size-4 text-accent" />}>
            {recent.length === 0 ? (
              <span className="text-xs text-muted">Waiting for events…</span>
            ) : (
              <ul className="flex flex-col">
                {recent.map((e, i) => {
                  const d = describe(e);
                  return (
                    <li
                      key={e.id || `${e.seq}-${i}`}
                      className="flex items-start gap-2 border-b border-border/40 py-1.5 last:border-0"
                    >
                      <span className={cn("mt-0.5 shrink-0", d.tone)}>{d.icon}</span>
                      <div className="min-w-0 flex-1">
                        <div className="flex items-baseline gap-2">
                          <span className="truncate text-xs">{d.label}</span>
                          <span className="ml-auto shrink-0 text-xs text-muted">
                            {fmtTime(e.ts_unix_ms)}
                          </span>
                        </div>
                        {e.actor && <div className="truncate text-xs text-muted">{e.actor}</div>}
                      </div>
                    </li>
                  );
                })}
              </ul>
            )}
          </Panel>
        </Advanced>
        </>
      )}
    </Page>
  );
}

// isSignificant keeps the supervisory-relevant events out of the raw feed noise.
export function overseerShouldRefresh(e: AgentEvent): boolean {
  return (!!e.kind && REFRESH_KINDS.has(e.kind)) || isIncidentFamilyEvent(e);
}

// isSignificant keeps the supervisory-relevant events out of the raw feed noise.
function isSignificant(e: AgentEvent): boolean {
  return overseerShouldRefresh(e);
}

// describe maps an event to a one-line supervisory label + an icon/tone.
function describe(e: AgentEvent): { label: string; icon: React.ReactNode; tone: string } {
  const p = e.payload || {};
  if (isIncidentFamilyEvent(e)) {
    return {
      label: incidentEventSummary(e),
      icon: <Stethoscope className="size-3.5" />,
      tone: e.subject === "doctor.auto_repair" ? "text-amber-400" : "text-sky-400",
    };
  }
  switch (e.kind) {
    case "task.received":
      return { label: `Run started: ${clip(p.intent) || e.correlation_id || "task"}`, icon: <CircleDot className="size-3.5" />, tone: "text-accent" };
    case "task.completed":
      return { label: `Run completed${p.iters ? ` (${p.iters} iters)` : ""}`, icon: <CheckCircle2 className="size-3.5" />, tone: "text-emerald-400" };
    case "task.failed":
      return { label: `Run failed${p.error ? `: ${clip(p.error)}` : ""}`, icon: <XCircle className="size-3.5" />, tone: "text-rose-400" };
    case "task.continued":
      return { label: "Run auto-continued past the iteration limit", icon: <ArrowRight className="size-3.5" />, tone: "text-muted" };
    case "subagent.spawned":
      return { label: `Sub-agent spawned: ${clip(p.task) || p.child_correlation || "delegated"}`, icon: <GitBranch className="size-3.5" />, tone: "text-sky-400" };
    case "council.consensus":
      return { label: "Council reached consensus", icon: <Scale className="size-3.5" />, tone: "text-violet-400" };
    case "board.posted":
      return {
        label: p.help ? `Help requested by ${p.from || "agent"}` : `Board post by ${p.from || "agent"}`,
        icon: p.help ? <LifeBuoy className="size-3.5" /> : <Megaphone className="size-3.5" />,
        tone: p.help ? "text-amber-400" : "text-muted",
      };
    case "agent.retry":
      return {
        label: `Retrying ${p.agent || e.actor || "agent"}${p.next_attempt && p.max_attempts ? ` ${p.next_attempt}/${p.max_attempts}` : ""}${p.reason ? `: ${clip(p.reason)}` : ""}`,
        icon: <RefreshCw className="size-3.5" />,
        tone: "text-amber-400",
      };
    default:
      return { label: e.kind || "event", icon: <CircleDot className="size-3.5" />, tone: "text-muted" };
  }
}

function clip(s: unknown, n = 64): string {
  if (typeof s !== "string") return "";
  return s.length > n ? s.slice(0, n).trimEnd() + "…" : s;
}

function Stat({
  icon,
  label,
  value,
  tone,
}: {
  icon: React.ReactNode;
  label: string;
  value: React.ReactNode;
  tone: "accent" | "warn" | "muted";
}) {
  return (
    <div
      className={cn(
        "glass flex items-center gap-3 rounded-xl px-3 py-2.5",
        tone === "accent" && "border-accent/40",
        tone === "warn" && "border-amber-500/40",
      )}
    >
      <span
        className={cn(
          "grid size-8 place-items-center rounded-md bg-panel",
          tone === "accent" && "text-accent",
          tone === "warn" && "text-amber-400",
          tone === "muted" && "text-muted",
        )}
      >
        {icon}
      </span>
      <div className="min-w-0">
        <div className="text-lg font-semibold leading-none">{value}</div>
        <div className="mt-1 truncate text-xs uppercase tracking-normal text-muted">{label}</div>
      </div>
    </div>
  );
}

function Panel({
  title,
  icon,
  children,
}: {
  title: string;
  icon: React.ReactNode;
  children: React.ReactNode;
}) {
  return (
    <div className="glass flex min-h-0 flex-col gap-2 overflow-hidden rounded-xl p-3">
      <h3 className="flex items-center gap-2 text-xs font-semibold">
        {icon} {title}
      </h3>
      <div className="min-h-0 flex-1 overflow-auto">{children}</div>
    </div>
  );
}
