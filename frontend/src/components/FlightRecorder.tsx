import { useEffect, useRef, useState } from "react";
import {
  Play,
  Pause,
  SkipBack,
  SkipForward,
  Flag,
  ArrowUp,
  Sparkles,
  Wrench,
  CornerDownLeft,
  Shield,
  DollarSign,
  Navigation,
  CheckCircle2,
  XCircle,
  Scissors,
  Circle,
  Gauge,
} from "lucide-react";
import { cn, fmtTime } from "@/lib/utils";
import { money } from "@/lib/format";
import type { ReplayStep, StepTone } from "@/lib/replay";
import { IncidentBadges } from "@/components/IncidentBadges";

const TONE: Record<StepTone, { color: string; ring: string; Icon: typeof Circle }> = {
  received: { color: "text-accent", ring: "bg-accent", Icon: Flag },
  request: { color: "text-muted", ring: "bg-muted", Icon: ArrowUp },
  response: { color: "text-accent", ring: "bg-accent", Icon: Sparkles },
  tool: { color: "text-accent", ring: "bg-accent", Icon: Wrench },
  result: { color: "text-good", ring: "bg-good", Icon: CornerDownLeft },
  policy: { color: "text-foreground", ring: "bg-foreground", Icon: Shield },
  spend: { color: "text-accent", ring: "bg-accent", Icon: DollarSign },
  steer: { color: "text-accent", ring: "bg-accent", Icon: Navigation },
  done: { color: "text-good", ring: "bg-good", Icon: CheckCircle2 },
  fail: { color: "text-bad", ring: "bg-bad", Icon: XCircle },
  context: { color: "text-muted", ring: "bg-muted", Icon: Scissors },
  other: { color: "text-muted", ring: "bg-muted", Icon: Circle },
};

const SPEEDS = [1, 2, 4];

// FlightRecorder replays a run step by step: a scrubber + play/pause moves a
// cursor through the journaled arc, the timeline highlights the current moment,
// and the header shows the cumulative state (tokens/cost/tools) the agent had
// reached by that point — a flight recorder for an autonomous run.
export function FlightRecorder({ steps, live }: { steps: ReplayStep[]; live?: boolean }) {
  const [cursor, setCursor] = useState(0);
  const [playing, setPlaying] = useState(false);
  const [speed, setSpeed] = useState(1);
  const listRef = useRef<HTMLDivElement>(null);
  const last = Math.max(0, steps.length - 1);

  // Clamp the cursor when the step list shrinks/grows (e.g. live updates).
  useEffect(() => {
    setCursor((c) => Math.min(c, last));
  }, [last]);

  // Playback engine: advance one step per tick; stop at the end.
  useEffect(() => {
    if (!playing) return;
    if (cursor >= last) {
      setPlaying(false);
      return;
    }
    const id = setTimeout(() => setCursor((c) => Math.min(c + 1, last)), 900 / speed);
    return () => clearTimeout(id);
  }, [playing, cursor, last, speed]);

  // Keep the active step in view as the cursor moves.
  useEffect(() => {
    const el = listRef.current?.querySelector<HTMLElement>(`[data-step="${cursor}"]`);
    el?.scrollIntoView({ block: "nearest", behavior: "smooth" });
  }, [cursor]);

  if (steps.length === 0) {
    return <div className="text-xs text-muted">no replayable events yet</div>;
  }

  const cur = steps[Math.min(cursor, last)];
  const pct = last === 0 ? 100 : (cursor / last) * 100;

  return (
    <div className="flex h-full min-h-0 flex-col gap-3">
      {/* Transport + cumulative state */}
      <div className="rounded-lg border border-border bg-card p-3">
        <div className="flex flex-wrap items-center gap-2">
          <button
            onClick={() => setCursor(0)}
            className="inline-flex size-8 items-center justify-center rounded-md border border-border hover:border-accent"
            title="Restart"
          >
            <SkipBack className="size-4" />
          </button>
          <button
            onClick={() => setPlaying((p) => !p)}
            disabled={cursor >= last}
            className="inline-flex size-8 items-center justify-center rounded-md border border-accent text-accent hover:bg-accent hover:text-white disabled:opacity-40"
            title={playing ? "Pause" : "Play"}
          >
            {playing ? <Pause className="size-4" /> : <Play className="size-4" />}
          </button>
          <button
            onClick={() => setCursor((c) => Math.min(c + 1, last))}
            className="inline-flex size-8 items-center justify-center rounded-md border border-border hover:border-accent"
            title="Step forward"
          >
            <SkipForward className="size-4" />
          </button>
          <div className="flex items-center gap-1 text-xs text-muted">
            <Gauge className="size-3.5" />
            {SPEEDS.map((s) => (
              <button
                key={s}
                onClick={() => setSpeed(s)}
                className={cn("rounded px-1.5 py-0.5", speed === s ? "bg-accent/20 text-accent" : "hover:text-foreground")}
              >
                {s}×
              </button>
            ))}
          </div>
          <div className="ml-auto text-xs tabular-nums text-muted">
            step {cursor + 1} / {steps.length}
            {live && <span className="ml-2 text-good">● live</span>}
          </div>
        </div>

        {/* Scrubber */}
        <input
          type="range"
          min={0}
          max={last}
          value={cursor}
          onChange={(e) => {
            setPlaying(false);
            setCursor(Number(e.target.value));
          }}
          className="mt-3 h-1.5 w-full cursor-pointer appearance-none rounded-full bg-panel accent-accent"
        />
        <div className="mt-1 h-1 overflow-hidden rounded-full bg-panel">
          <div className="h-full rounded-full bg-accent/60 transition-all" style={{ width: `${pct}%` }} />
        </div>

        {/* Cumulative state at the cursor */}
        <div className="mt-3 grid grid-cols-2 gap-2 sm:grid-cols-4">
          <Metric label="iteration" value={cur.iter != null ? cur.iter + 1 : "—"} />
          <Metric label="tokens" value={`${cur.cumIn.toLocaleString()} / ${cur.cumOut.toLocaleString()}`} />
          <Metric label="spend" value={money(cur.cumCostMc)} />
          <Metric label="tool calls" value={cur.cumTools} />
        </div>
      </div>

      {/* Timeline */}
      <div ref={listRef} className="min-h-0 flex-1 overflow-auto rounded-lg border border-border bg-card">
        <ul className="relative py-1">
          {steps.map((s, i) => {
            const t = TONE[s.tone];
            const active = i === cursor;
            const past = i < cursor;
            return (
              <li
                key={`${s.seq}-${i}`}
                data-step={i}
                onClick={() => {
                  setPlaying(false);
                  setCursor(i);
                }}
                className={cn(
                  "flex cursor-pointer items-start gap-2.5 px-3 py-1.5 transition-colors",
                  active ? "bg-accent/10" : "hover:bg-panel/60",
                  !active && !past && "opacity-55",
                )}
              >
                <div className="flex w-12 shrink-0 flex-col items-end pt-0.5">
                  <span className="text-xs tabular-nums text-muted">{fmtTime(s.ts)}</span>
                </div>
                <div className="relative flex flex-col items-center self-stretch">
                  <span className={cn("mt-1 inline-flex size-4 items-center justify-center rounded-full", active ? t.ring : "bg-border")}>
                    <t.Icon className={cn("size-2.5", active ? "text-white" : t.color)} />
                  </span>
                  {i < steps.length - 1 && <span className="w-px flex-1 bg-border/70" />}
                </div>
                <div className="min-w-0 flex-1 pb-0.5">
                  <div className="flex items-center gap-1.5">
                    {s.incident && <IncidentBadges item={s.incident} mono />}
                    <div className={cn("truncate text-xs font-medium", active ? "text-foreground" : t.color)}>
                      {s.title}
                    </div>
                  </div>
                  {s.detail && (
                    <div
                      className={cn(
                        "mt-0.5 break-words font-mono text-[11px] text-muted",
                        active ? "line-clamp-none" : "line-clamp-1",
                      )}
                    >
                      {s.detail}
                    </div>
                  )}
                </div>
              </li>
            );
          })}
        </ul>
      </div>
    </div>
  );
}

function Metric({ label, value }: { label: string; value: number | string }) {
  return (
    <div className="rounded-md border border-border/70 bg-panel/40 px-2.5 py-1.5">
      <div className="text-xs uppercase tracking-normal text-muted">{label}</div>
      <div className="mt-0.5 text-sm font-semibold tabular-nums">{value}</div>
    </div>
  );
}
