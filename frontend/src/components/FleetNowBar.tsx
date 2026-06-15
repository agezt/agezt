import { useMemo } from "react";
import { Radio, CircleDot, Activity } from "lucide-react";
import { useEvents, type AgentEvent } from "@/lib/events";
import { cn, clip } from "@/lib/utils";
import { AgentAvatar } from "@/components/AgentAvatar";

// FleetNowBar (M945) is the live "now playing" strip: a qualitative, event-
// driven view of WHAT the fleet is doing right this second — which agents are
// running which tasks — to complement Vitals' numeric pulse. It reads only the
// rolling SSE buffer (no fetch): a run is "live" when the most recent lifecycle
// event for its correlation is task.received (not yet completed/failed). Each
// running agent shows as a breathing chip; idle shows a calm listening state.

interface LiveRun {
  corr: string;
  agent?: string;
  intent?: string;
  ts?: number;
}

// payloadIntent is the subset of agent-event payloads that carry an intent string.
interface PayloadIntent {
  intent?: string;
}

// tickerLabel summarizes the newest event for the right-hand ticker.
function tickerLabel(e: AgentEvent): string {
  const who = e.actor ? `${e.actor} · ` : "";
  const p = e.payload as PayloadIntent | null | undefined;
  const subj = clip(e.subject || p?.intent || "", 48);
  return `${who}${e.kind || "event"}${subj ? " — " + subj : ""}`;
}

export function FleetNowBar({ onNavigate }: { onNavigate?: (id: string) => void }) {
  const { events, connected } = useEvents();

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
          runs.push({ corr, agent: e.actor, intent: (e.payload as any)?.intent, ts: e.ts_unix_ms });
        }
      }
    }
    return runs;
  }, [events]);

  const latest = events[0];
  const shown = live.slice(0, 4);
  const overflow = live.length - shown.length;
  const go = () => onNavigate?.("overseer");

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
        <div className="flex shrink-0 items-center gap-1.5">
          {shown.map((r) => (
            <button
              key={r.corr}
              onClick={go}
              title={r.intent || r.corr}
              className="now-in flex max-w-[15rem] shrink-0 items-center gap-1.5 rounded-full border border-border bg-card py-0.5 pl-0.5 pr-2 transition-colors hover:border-accent"
            >
              {r.agent ? (
                <AgentAvatar slug={r.agent} size={18} status="running" />
              ) : (
                <span className="work-pulse ml-1 size-2 shrink-0 rounded-full bg-accent" />
              )}
              <span className="shrink-0 font-medium text-foreground">{r.agent || "run"}</span>
              {r.intent && <span className="truncate text-muted">{clip(r.intent, 40)}</span>}
            </button>
          ))}
          {overflow > 0 && (
            <button onClick={go} className="shrink-0 rounded-full bg-card px-2 py-0.5 text-muted hover:text-foreground">
              +{overflow} more
            </button>
          )}
        </div>
      )}

      {/* Live event ticker — keyed by seq so each new event eases in. */}
      {latest && (
        <div className="ml-auto flex min-w-0 shrink items-center gap-1.5 text-muted/70">
          <Activity className="size-3 shrink-0" />
          <span key={latest.seq ?? latest.id} className="now-in truncate font-mono text-[11px]">
            {tickerLabel(latest)}
          </span>
        </div>
      )}
    </div>
  );
}
