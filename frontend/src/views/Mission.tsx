import { useEffect, useReducer, useRef } from "react";
import { Radar, Zap, Coins, Wrench, Brain, Activity, Waypoints } from "lucide-react";
import { useEvents } from "@/lib/events";
import { money } from "@/lib/format";
import { cn } from "@/lib/utils";
import { SpendArea } from "@/components/Charts";
import { PageHeader } from "@/components/ui/page-header";
import { emptyBucket, addEvent, summarize, type Bucket } from "@/lib/telemetry";

const WINDOW = 60; // seconds of rolling history

// Mission is the real-time operations terminal: it folds the live event firehose
// into per-second buckets over a rolling 60s window and renders the daemon's
// pulse as live rates + animated sparklines — events/sec, LLM activity, tokens,
// spend, tool calls — updating every second as the system works.
export function Mission() {
  const { subscribe, connected } = useEvents();
  const buckets = useRef<Bucket[]>(Array.from({ length: WINDOW }, emptyBucket));
  const [, tick] = useReducer((x) => x + 1, 0);

  // Fold every live event into the current (newest) second's bucket.
  useEffect(
    () =>
      subscribe((e) => {
        const arr = buckets.current;
        arr[arr.length - 1] = addEvent(arr[arr.length - 1], e);
      }),
    [subscribe],
  );

  // Roll the window once a second and re-render.
  useEffect(() => {
    const id = setInterval(() => {
      const arr = buckets.current;
      arr.push(emptyBucket());
      while (arr.length > WINDOW) arr.shift();
      tick();
    }, 1000);
    return () => clearInterval(id);
  }, []);

  const arr = buckets.current;
  const t = summarize(arr);
  // "Now" = the last fully-elapsed second (the newest bucket is still filling).
  const now = arr[arr.length - 2] || emptyBucket();

  return (
    <div className="space-y-4">
      <PageHeader
        icon={Radar}
        title="Mission control"
        description={`rolling ${WINDOW}s · ${t.totalEvents} events`}
        actions={
          <span className={cn("inline-flex items-center gap-1 text-xs", connected ? "text-good" : "text-bad")}>
            ● {connected ? "live" : "offline"}
          </span>
        }
      />

      {/* Activity hero waveform */}
      <div className="glass rounded-xl p-3">
        <div className="mb-1 flex items-center justify-between text-xs">
          <span className="inline-flex items-center gap-1.5 font-semibold uppercase tracking-normal text-muted">
            <Activity className="size-3.5" /> activity
          </span>
          <span className="tabular-nums text-muted">
            now <span className="text-accent">{now.events}</span> ev/s · peak {t.peakEvents} · avg{" "}
            {t.eventsPerSec.toFixed(1)}
          </span>
        </div>
        <SpendArea values={arr.map((b) => b.events)} className="h-28" />
      </div>

      {/* Live metric cards */}
      <div className="grid grid-cols-2 gap-3 lg:grid-cols-5">
        <Metric
          icon={Brain}
          label="LLM calls/s"
          value={t.llmPerSec.toFixed(1)}
          now={now.llm}
          series={arr.map((b) => b.llm)}
        />
        <Metric
          icon={Zap}
          label="tokens/s"
          value={Math.round(t.tokensPerSec).toLocaleString()}
          now={now.tokensIn + now.tokensOut}
          series={arr.map((b) => b.tokensIn + b.tokensOut)}
        />
        <Metric
          icon={Coins}
          label="spend/s"
          value={money(t.costPerSecMc)}
          now={now.costMc}
          nowFmt={(v) => money(v)}
          series={arr.map((b) => b.costMc)}
        />
        <Metric
          icon={Wrench}
          label="tool calls/s"
          value={t.toolsPerSec.toFixed(2)}
          now={now.tools}
          series={arr.map((b) => b.tools)}
        />
        <Metric
          icon={Waypoints}
          label={`delegations/${WINDOW}s`}
          value={t.subagentsTotal.toLocaleString()}
          now={now.subagents}
          series={arr.map((b) => b.subagents)}
        />
      </div>
    </div>
  );
}

function Metric({
  icon: Icon,
  label,
  value,
  now,
  nowFmt,
  series,
}: {
  icon: typeof Zap;
  label: string;
  value: string;
  now: number;
  nowFmt?: (v: number) => string;
  series: number[];
}) {
  return (
    <div className="glass rounded-xl p-3">
      <div className="flex items-center gap-1.5 text-xs text-muted">
        <Icon className="size-3.5" /> {label}
      </div>
      <div className="mt-0.5 flex items-baseline gap-2">
        <span className="text-2xl font-semibold tabular-nums">{value}</span>
        <span className="text-xs text-muted">now {nowFmt ? nowFmt(now) : now}</span>
      </div>
      <SpendArea values={series} className="mt-1 h-12" />
    </div>
  );
}
