import { useMemo, useRef, useState } from "react";
import { Radio, CircleDot, Activity, ChevronLeft, ChevronRight, ChevronDown, ChevronUp } from "lucide-react";
import { useEvents, type AgentEvent } from "@/lib/events";
import { cn, clip, fmtAgo } from "@/lib/utils";
import { AgentAvatar } from "@/components/AgentAvatar";
import { Sparkline } from "@/components/Sparkline";
import { openAgent } from "@/lib/agentnav";

// activitySeries buckets the recent event buffer into a small per-interval count
// series for the live sparkline (M977) — the shape of fleet chatter over the
// last ~2 minutes, newest on the right.
function activitySeries(events: AgentEvent[], buckets = 24, windowMs = 120_000, now = Date.now()): number[] {
  const out = new Array(buckets).fill(0);
  const step = windowMs / buckets;
  for (const e of events) {
    const ts = e.ts_unix_ms;
    if (!ts) continue;
    const age = now - ts;
    if (age < 0 || age >= windowMs) continue;
    const idx = buckets - 1 - Math.floor(age / step);
    if (idx >= 0 && idx < buckets) out[idx]++;
  }
  return out;
}

// FleetNowBar (M945, reworked M973) is the live "now playing" strip: a
// qualitative, event-driven view of WHAT the fleet is doing right now — which
// agents are running which tasks or autonomous repair jobs. It reads only the
// rolling SSE buffer (no fetch): a correlation is "live" until its latest
// lifecycle/repair event is terminal.
//
// Two shapes (M973): COLLAPSED shows a compact overlapping stack of agent
// avatars (3 shown + "+N"), so many concurrent agents stay a glance, not a wall
// of text. EXPANDED turns the strip into a single horizontal row of taller
// "running agent" cards you can slide left/right and click through to each
// agent's identity page.

export interface LiveRun {
  corr: string;
  agent?: string;
  intent?: string;
  ts?: number;
  lastTs?: number;
  phase?: string;
  detail?: string;
  tool?: string;
  model?: string;
}

export interface FleetNowSummary {
  awake: number;
  repairing: number;
  toolUsers: number;
  modelThinkers: number;
  agentless: number;
  value: string;
  detail: string;
  tone: "good" | "warn" | "accent" | "muted";
}

// PayloadIntent is the subset of agent-event payloads the Now strip reads: the
// task intent and the agent SLUG. The slug must come from the payload, not the
// event actor — for ad-hoc/chat runs the actor is the run correlation id
// ("agent-run-…"), which is not a navigable agent (M980).
interface PayloadIntent {
  intent?: string;
  agent?: string;
  target_agent?: string;
  iter?: number;
  tool?: string;
  name?: string;
  capability?: string;
  model?: string;
  mode?: string;
  phase?: string;
  reason?: string;
  error?: string | boolean;
  resolution?: string;
  allow?: boolean;
}

// isRealAgent rejects run-correlation ids ("agent-run-…" / "run-…") so we never
// deep-link to #agent/<run-id>, which renders a "no such agent" page.
function isRealAgent(a: string | undefined): a is string {
  return !!a && !a.startsWith("agent-run-") && !a.startsWith("run-");
}

// tickerLabel summarizes the newest event for the right-hand ticker.
function tickerLabel(e: AgentEvent): string {
  const who = e.actor ? `${e.actor} · ` : "";
  const p = e.payload as PayloadIntent | null | undefined;
  const subj = clip(e.subject || p?.intent || "", 48);
  return `${who}${e.kind || "event"}${subj ? " — " + subj : ""}`;
}

const STACK_SHOWN = 3; // collapsed avatars before the "+N" overflow

export function fleetNowSummary(live: LiveRun[]): FleetNowSummary {
  const awake = live.length;
  const repairing = live.filter((r) => {
    const phase = (r.phase || "").toLowerCase();
    return phase.includes("repair");
  }).length;
  const toolUsers = live.filter((r) => !!r.tool || (r.phase || "").toLowerCase().includes("tool")).length;
  const modelThinkers = live.filter((r) => !!r.model || (r.phase || "").toLowerCase().includes("thinking")).length;
  const agentless = live.filter((r) => !isRealAgent(r.agent)).length;
  const parts = [`${awake} agents awake`];
  if (repairing) parts.push(`${repairing} repair`);
  if (toolUsers) parts.push(`${toolUsers} tools`);
  if (modelThinkers) parts.push(`${modelThinkers} model`);
  if (agentless) parts.push(`${agentless} agentless`);
  return {
    awake,
    repairing,
    toolUsers,
    modelThinkers,
    agentless,
    value: awake === 0 ? "fleet sleeping" : repairing > 0 ? `${awake} awake · ${repairing} repair` : `${awake} awake`,
    detail: parts.join(" · "),
    tone: awake === 0 ? "muted" : repairing > 0 ? "warn" : toolUsers > 0 || modelThinkers > 0 ? "accent" : "good",
  };
}

function eventPhase(e: AgentEvent): Pick<LiveRun, "phase" | "detail" | "tool" | "model" | "lastTs"> {
  const p = (e.payload || {}) as PayloadIntent;
  const iter = typeof p.iter === "number" ? `iter ${p.iter}` : "";
  const tool = p.tool || p.name || p.capability || "";
  const detail = [iter, tool].filter(Boolean).join(" · ");
  if (e.subject === "doctor.auto_repair") {
    const phase = p.phase || "repair";
    const mode = p.mode ? `${p.mode} repair` : "repair";
    const target = p.agent || p.target_agent || "";
    const issue = typeof p.error === "string" ? p.error : p.reason || p.resolution || "";
    return {
      phase: repairPhaseLabel(phase),
      detail: [mode, target ? `agent ${target}` : "", issue].filter(Boolean).join(" · "),
      lastTs: e.ts_unix_ms,
    };
  }
  switch (e.kind) {
    case "task.received":
      return { phase: "starting", detail: p.intent, model: p.model, lastTs: e.ts_unix_ms };
    case "llm.request":
      return { phase: "thinking", detail: iter, model: p.model, lastTs: e.ts_unix_ms };
    case "llm.response":
      return { phase: p.tool ? "planned tools" : "answered draft", detail, model: p.model, lastTs: e.ts_unix_ms };
    case "policy.decision":
      return { phase: p.allow === false ? "blocked tool" : "checking policy", detail, tool, lastTs: e.ts_unix_ms };
    case "tool.invoked":
      return { phase: "using tool", detail, tool, lastTs: e.ts_unix_ms };
    case "tool.result":
      return { phase: p.error ? "tool error" : "observed tool", detail, tool, lastTs: e.ts_unix_ms };
    default:
      return { phase: e.kind || "working", detail: e.subject, lastTs: e.ts_unix_ms };
  }
}

function repairPhaseLabel(phase: string): string {
  switch (phase) {
    case "queued":
      return "repair queued";
    case "routing_rollback_queued":
      return "rollback queued";
    case "routing_rollback_completed":
      return "rollback completed";
    case "routing_rollback_failed":
      return "rollback failed";
    case "attempts_exhausted":
      return "repair exhausted";
    case "resolution_applied":
      return "manager applied";
    case "completed":
      return "repair completed";
    case "failed":
    case "resolution_failed":
      return "repair failed";
    default:
      return phase || "repair";
  }
}

function isTerminalEvent(e: AgentEvent): boolean {
  if (e.kind === "task.completed" || e.kind === "task.failed") return true;
  if (e.subject !== "doctor.auto_repair") return false;
  const phase = String((e.payload as PayloadIntent | undefined)?.phase || "");
  return phase === "completed" || phase === "failed" || phase === "resolution_applied" || phase === "resolution_failed" || phase === "routing_rollback_completed" || phase === "routing_rollback_failed";
}

function isStandaloneRepairEvent(e: AgentEvent): boolean {
  if (e.subject !== "doctor.auto_repair") return false;
  const phase = String((e.payload as PayloadIntent | undefined)?.phase || "");
  return !isTerminalEvent(e) && phase !== "";
}

export function liveRunsFromEvents(events: AgentEvent[]): LiveRun[] {
  const byCorr = new Map<string, LiveRun & { terminal?: boolean; seenStart?: boolean }>();
  for (const e of events) {
    const corr = e.correlation_id;
    if (!corr) continue;
    const existing = byCorr.get(corr);
    if (existing?.terminal) continue;
    if (!existing) {
      const terminal = isTerminalEvent(e);
      const phase = eventPhase(e);
      byCorr.set(corr, { corr, terminal, ...phase });
      if (terminal) continue;
    }
    const row = byCorr.get(corr)!;
    if (isStandaloneRepairEvent(e)) {
      const p = e.payload as PayloadIntent | null;
      const agent = isRealAgent(p?.agent) ? p?.agent : isRealAgent(p?.target_agent) ? p?.target_agent : undefined;
      row.seenStart = true;
      row.agent = agent || row.agent;
      row.intent = row.intent || [p?.mode || "doctor", "repair", agent || ""].filter(Boolean).join(" ");
      row.ts = e.ts_unix_ms || row.ts;
      continue;
    }
    if (e.kind === "task.received") {
      const p = e.payload as PayloadIntent | null;
      row.seenStart = true;
      row.agent = isRealAgent(p?.agent) ? p?.agent : isRealAgent(e.actor) ? e.actor : row.agent;
      row.intent = p?.intent || row.intent;
      row.ts = e.ts_unix_ms || row.ts;
      if (!row.phase) Object.assign(row, eventPhase(e));
    }
  }
  return [...byCorr.values()]
    .filter((r) => !r.terminal && r.seenStart)
    .map(({ terminal: _terminal, seenStart: _seenStart, ...r }) => r);
}

export function FleetNowBar({ onNavigate }: { onNavigate?: (id: string) => void }) {
  const { events, connected } = useEvents();
  const [expanded, setExpanded] = useState(false);

  // Replay the buffer (newest-first): terminal task events close a correlation;
  // otherwise the newest non-terminal event becomes the visible phase while the
  // older task.received supplies the agent slug and intent.
  const live = useMemo<LiveRun[]>(() => liveRunsFromEvents(events), [events]);
  const summary = useMemo(() => fleetNowSummary(live), [live]);

  const latest = events[0];
  const spark = useMemo(() => activitySeries(events), [events]);
  const go = () => onNavigate?.("overseer");
  const open = (r: LiveRun) => (isRealAgent(r.agent) ? openAgent(r.agent) : go());

  // Expanded view: a horizontal slider of running-agent cards.
  if (expanded && connected && live.length > 0) {
    return <NowSlider live={live} onCollapse={() => setExpanded(false)} onOpen={open} onOverseer={go} />;
  }

  const stack = live.slice(0, STACK_SHOWN);
  const overflow = live.length - stack.length;

  return (
    <div className="flex shrink-0 items-center gap-2 overflow-x-auto border-b border-border bg-panel/40 px-3 py-1.5 text-xs">
      {/* Status lamp + label */}
      <button
        onClick={go}
        className="flex shrink-0 items-center gap-1.5 font-semibold text-muted transition-colors hover:text-foreground"
        title={connected ? "Live fleet — click for the Overseer" : "Reconnecting to the live stream…"}
      >
        <Radio className={cn("size-3.5", connected ? "text-good" : "text-muted")} />
        <span className="hidden sm:inline">Now</span>
      </button>

      {!connected ? (
        <span className="text-muted/70">reconnecting…</span>
      ) : live.length === 0 ? (
        <span className="flex items-center gap-1.5 text-muted/80">
          <CircleDot className="size-3 work-pulse text-good" />
          fleet idle · listening
        </span>
      ) : (
        <div className="glow-accent group flex shrink-0 items-center gap-2 rounded-full bg-card py-0.5 pl-1 pr-2 transition-shadow hover:shadow-e2">
          {/* Overlapping avatar stack — each running roster agent is a DIRECT
              link to its identity page (M991); agentless ad-hoc runs and the
              "+N" overflow expand the slider instead. */}
          <span className="flex items-center -space-x-2">
            {stack.map((r) =>
              isRealAgent(r.agent) ? (
                <button
                  key={r.corr}
                  onClick={() => openAgent(r.agent!)}
                  title={`Open ${r.agent}'s page`}
                  className="now-in relative rounded-full ring-2 ring-card transition-transform hover:z-10 hover:scale-110"
                >
                  <AgentAvatar slug={r.agent} size={20} status="running" />
                </button>
              ) : (
                <button
                  key={r.corr}
                  onClick={() => setExpanded(true)}
                  title={r.intent || "running task — expand for details"}
                  className="now-in work-pulse relative flex size-5 items-center justify-center rounded-full bg-accent text-[8px] font-bold text-white ring-2 ring-card"
                >
                  ?
                </button>
              ),
            )}
            {overflow > 0 && (
              <button
                onClick={() => setExpanded(true)}
                title="Expand all running agents"
                className="relative flex size-5 items-center justify-center rounded-full bg-accent/20 text-[9px] font-bold text-accent ring-2 ring-card transition-transform hover:z-10 hover:scale-110"
              >
                +{overflow}
              </button>
            )}
          </span>
          <button
            onClick={() => setExpanded(true)}
            title={`${live.length} agent${live.length === 1 ? "" : "s"} running — expand into cards`}
            className="flex items-center gap-1 font-medium text-foreground transition-colors hover:text-accent"
          >
            {live.length} running
            <ChevronDown className="size-3.5 text-muted transition-colors group-hover:text-accent" />
          </button>
          <NowMiniLedger summary={summary} />
        </div>
      )}

      {/* Live event ticker — keyed by seq so each new event eases in. */}
      {latest && (
        <div className="ml-auto flex min-w-0 shrink items-center gap-2 text-muted/70">
          <Sparkline points={spark} width={56} height={16} className="hidden shrink-0 md:block" />
          <Activity className="size-3 shrink-0" />
          <span key={latest.seq ?? latest.id} className="now-in truncate font-mono text-[11px]">
            {tickerLabel(latest)}
          </span>
        </div>
      )}
    </div>
  );
}

function NowMiniLedger({ summary }: { summary: FleetNowSummary }) {
  const cells = [
    { label: "awake", value: summary.awake, show: summary.awake > 0 },
    { label: "repair", value: summary.repairing, show: summary.repairing > 0 },
    { label: "tools", value: summary.toolUsers, show: summary.toolUsers > 0 },
    { label: "model", value: summary.modelThinkers, show: summary.modelThinkers > 0 },
    { label: "agentless", value: summary.agentless, show: summary.agentless > 0 },
  ].filter((cell) => cell.show);

  if (cells.length === 0) return null;
  return (
    <span
      aria-label="Now ledger"
      title={summary.detail}
      className={cn(
        "hidden items-center gap-1 border-l border-border/70 pl-2 text-xs sm:flex",
        summary.tone === "warn" ? "text-warn" : summary.tone === "accent" ? "text-accent" : "text-muted",
      )}
    >
      {cells.map((cell) => (
        <span key={cell.label} className="rounded-md border border-border bg-panel/50 px-1.5 py-0.5">
          {cell.label} {cell.value}
        </span>
      ))}
    </span>
  );
}

// NowSlider is the expanded strip: a single horizontal row of running-agent
// cards (no second row), scrollable left/right via the trackpad or the arrow
// buttons, each card a doorway to that agent's identity page.
function NowSlider({
  live,
  onCollapse,
  onOpen,
  onOverseer,
}: {
  live: LiveRun[];
  onCollapse: () => void;
  onOpen: (r: LiveRun) => void;
  onOverseer: () => void;
}) {
  const scroller = useRef<HTMLDivElement>(null);
  const scrollBy = (dir: -1 | 1) => scroller.current?.scrollBy({ left: dir * 260, behavior: "smooth" });
  const summary = fleetNowSummary(live);

  return (
    <div className="flex shrink-0 flex-col gap-1.5 border-b border-border bg-panel/40 px-3 py-2">
      <div className="flex items-center gap-2 text-xs">
        <button
          onClick={onOverseer}
          className="flex shrink-0 items-center gap-1.5 font-semibold text-muted transition-colors hover:text-foreground"
          title="Live fleet — click for the Overseer"
        >
          <Radio className="size-3.5 text-good" />
          <span>Now</span>
          <span className="text-muted/70">· {live.length} running</span>
        </button>
        <NowMiniLedger summary={summary} />
        <div className="ml-auto flex items-center gap-1">
          <button onClick={() => scrollBy(-1)} title="Scroll left" className="rounded-md border border-border p-1 text-muted transition-colors hover:border-accent hover:text-foreground">
            <ChevronLeft className="size-3.5" />
          </button>
          <button onClick={() => scrollBy(1)} title="Scroll right" className="rounded-md border border-border p-1 text-muted transition-colors hover:border-accent hover:text-foreground">
            <ChevronRight className="size-3.5" />
          </button>
          <button onClick={onCollapse} title="Collapse" className="ml-1 flex items-center gap-1 rounded-md border border-border px-1.5 py-1 text-[11px] text-muted transition-colors hover:border-accent hover:text-foreground">
            <ChevronUp className="size-3.5" /> Collapse
          </button>
        </div>
      </div>

      <div ref={scroller} className="flex snap-x gap-2 overflow-x-auto pb-1 [scrollbar-width:thin]">
        {live.map((r) => (
          <button
            key={r.corr}
            onClick={() => onOpen(r)}
            title={r.agent ? `Open ${r.agent}'s identity page` : r.intent || r.corr}
            className="now-in group flex w-60 shrink-0 snap-start flex-col gap-2 rounded-lg border border-border bg-card p-3 text-left transition-colors hover:border-accent"
          >
            <div className="flex items-center gap-2">
              {r.agent ? (
                <AgentAvatar slug={r.agent} size={32} status="running" />
              ) : (
                <span className="work-pulse flex size-8 items-center justify-center rounded-full bg-accent text-xs font-bold text-white">?</span>
              )}
              <div className="min-w-0 flex-1">
                <div className="truncate text-sm font-semibold text-foreground">{r.agent || "run"}</div>
                <div className="flex items-center gap-1 text-xs text-good">
                  <span className="work-pulse size-1.5 rounded-full bg-good" /> running
                  {r.ts ? <span className="text-muted">· {fmtAgo(r.ts)}</span> : null}
                </div>
              </div>
              <ChevronRight className="size-3.5 shrink-0 text-muted opacity-0 transition-opacity group-hover:opacity-100" />
            </div>
            <div className="flex flex-wrap items-center gap-1.5">
              {r.phase && (
                <span className="rounded-md border border-accent/30 bg-accent/10 px-1.5 py-0.5 text-xs font-medium text-accent">
                  {r.phase}
                </span>
              )}
              {r.tool && (
                <span className="rounded-md border border-border bg-panel/50 px-1.5 py-0.5 font-mono text-xs text-muted">
                  {r.tool}
                </span>
              )}
              {r.model && (
                <span className="rounded-md border border-border bg-panel/50 px-1.5 py-0.5 font-mono text-xs text-muted">
                  {r.model}
                </span>
              )}
              {r.lastTs && r.lastTs !== r.ts ? (
                <span className="text-xs text-muted">last event {fmtAgo(r.lastTs)}</span>
              ) : null}
            </div>
            {r.detail && r.detail !== r.intent && (
              <div className="truncate font-mono text-xs text-muted" title={r.detail}>
                {r.detail}
              </div>
            )}
            <div className="line-clamp-3 min-h-[3.2em] text-[11px] leading-snug text-muted" title={r.intent || ""}>
              {r.intent ? clip(r.intent, 160) : <span className="italic text-muted/60">no intent recorded</span>}
            </div>
          </button>
        ))}
      </div>
    </div>
  );
}
