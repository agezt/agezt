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
// agents are running which tasks. It reads only the rolling SSE buffer (no
// fetch): a run is "live" when the most recent lifecycle event for its
// correlation is task.received (not yet completed/failed).
//
// Two shapes (M973): COLLAPSED shows a compact overlapping stack of agent
// avatars (3 shown + "+N"), so many concurrent agents stay a glance, not a wall
// of text. EXPANDED turns the strip into a single horizontal row of taller
// "running agent" cards you can slide left/right and click through to each
// agent's identity page.

interface LiveRun {
  corr: string;
  agent?: string;
  intent?: string;
  ts?: number;
}

// PayloadIntent is the subset of agent-event payloads the Now strip reads: the
// task intent and the agent SLUG. The slug must come from the payload, not the
// event actor — for ad-hoc/chat runs the actor is the run correlation id
// ("agent-run-…"), which is not a navigable agent (M980).
interface PayloadIntent {
  intent?: string;
  agent?: string;
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

export function FleetNowBar({ onNavigate }: { onNavigate?: (id: string) => void }) {
  const { events, connected } = useEvents();
  const [expanded, setExpanded] = useState(false);

  // Replay the buffer (newest-first): the first lifecycle event per correlation
  // decides its state. Most-recent == task.received → still running.
  const live = useMemo<LiveRun[]>(() => {
    const seen = new Set<string>();
    const runs: LiveRun[] = [];
    for (const e of events) {
      const corr = e.correlation_id;
      if (!corr || seen.has(corr)) continue;
      if (e.kind === "task.received" || e.kind === "task.completed" || e.kind === "task.failed") {
        seen.add(corr);
        if (e.kind === "task.received") {
          {
            const p = e.payload as PayloadIntent | null;
            // Agent slug comes from the payload; fall back to the actor only when
            // it's a real slug (not a run id). Ad-hoc/chat runs stay agentless.
            const agent = isRealAgent(p?.agent) ? p?.agent : isRealAgent(e.actor) ? e.actor : undefined;
            runs.push({ corr, agent, intent: p?.intent, ts: e.ts_unix_ms });
          }
        }
      }
    }
    return runs;
  }, [events]);

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
                <div className="flex items-center gap-1 text-[10px] text-good">
                  <span className="work-pulse size-1.5 rounded-full bg-good" /> running
                  {r.ts ? <span className="text-muted">· {fmtAgo(r.ts)}</span> : null}
                </div>
              </div>
              <ChevronRight className="size-3.5 shrink-0 text-muted opacity-0 transition-opacity group-hover:opacity-100" />
            </div>
            <div className="line-clamp-3 min-h-[3.2em] text-[11px] leading-snug text-muted" title={r.intent || ""}>
              {r.intent ? clip(r.intent, 160) : <span className="italic text-muted/60">no intent recorded</span>}
            </div>
          </button>
        ))}
      </div>
    </div>
  );
}
