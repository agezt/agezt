import { useEffect, useMemo, useReducer, useRef, useState } from "react";
import { Radar, Zap, Coins, Wrench, Brain, Activity, Waypoints, ListTree, Bell } from "lucide-react";
import { useEvents, type AgentEvent } from "@/lib/events";
import { getJSON } from "@/lib/api";
import { money } from "@/lib/format";
import { cn, fmtWhen } from "@/lib/utils";
import { humanizeIntent } from "@/lib/intent";
import { focusRun } from "@/lib/runfocus";
import { SpendArea } from "@/components/Charts";
import { Page } from "@/components/ui/page";
import { MetricWidget } from "@/components/ui/metric-widget";
import { emptyBucket, addEvent, summarize, type Bucket } from "@/lib/telemetry";

const WINDOW = 60; // seconds of rolling history

interface ActiveRun {
  correlation_id?: string;
  intent?: string;
  started_unix_ms?: number;
  model?: string;
}

// NOTABLE maps the event kinds worth a line in the "recent activity" panel to
// a humane label. The rolling rates above are great under load but read as a
// wall of zeros when the daemon idles — this panel keeps the page informative
// at rest ("what happened lately") without duplicating the Live Stream firehose.
const NOTABLE: Record<string, string> = {
  "task.received": "run started",
  "task.completed": "run completed",
  "task.failed": "run failed",
  "schedule.fired": "schedule fired",
  "standing.fired": "standing order fired",
  "skill.created": "skill learned",
  "skill.promoted": "skill promoted",
  "briefing.sent": "briefing sent",
  "provider.fallback": "provider fallback",
  halt: "daemon halted",
  resume: "daemon resumed",
};

export function notableEvents(events: AgentEvent[], limit = 8): AgentEvent[] {
  const seen = new Set<string>();
  const out: AgentEvent[] = [];
  for (const e of events) {
    if (!NOTABLE[String(e.kind || "")]) continue;
    const id = e.id || `${e.kind}-${e.seq ?? ""}`;
    if (seen.has(id)) continue;
    seen.add(id);
    out.push(e);
  }
  out.sort((a, b) => (b.ts_unix_ms ?? 0) - (a.ts_unix_ms ?? 0));
  return out.slice(0, limit);
}

function eventDetail(e: AgentEvent): string {
  const p: any = e.payload || {};
  return humanizeIntent(String(p.intent || "")) || String(p.reason || p.name || p.title || p.error || "");
}

// Mission is the real-time operations terminal: it folds the live event firehose
// into per-second buckets over a rolling 60s window and renders the daemon's
// pulse as live rates + animated sparklines — events/sec, LLM activity, tokens,
// spend, tool calls — updating every second as the system works. Below the
// rates, the active-runs and recent-activity panels keep the page meaningful
// when the fleet is idle (all-zero rates used to be ALL it showed at rest).
export function Mission() {
  const { events, subscribe, connected } = useEvents();
  const buckets = useRef<Bucket[]>(Array.from({ length: WINDOW }, emptyBucket));
  const [, tick] = useReducer((x) => x + 1, 0);
  const [active, setActive] = useState<ActiveRun[] | null>(null);

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

  // Poll the running-runs strip (cheap: status-filtered, capped at 10).
  useEffect(() => {
    let alive = true;
    const load = () =>
      getJSON<{ runs?: ActiveRun[] }>("/api/runs", { status: "running", limit: "10" })
        .then((d) => {
          if (alive) setActive(d.runs || []);
        })
        .catch(() => {});
    load();
    const id = setInterval(load, 5000);
    return () => {
      alive = false;
      clearInterval(id);
    };
  }, []);

  const recent = useMemo(() => notableEvents(events), [events]);

  const arr = buckets.current;
  const t = summarize(arr);
  // "Now" = the last fully-elapsed second (the newest bucket is still filling).
  const now = arr[arr.length - 2] || emptyBucket();

  return (
    <Page
      icon={Radar}
      title="Mission control"
      description={`rolling ${WINDOW}s · ${t.totalEvents} events`}
      width="wide"
      className="gap-4"
      actions={
        <span className={cn("inline-flex items-center gap-1 text-xs", connected ? "text-good" : "text-bad")}>
          ● {connected ? "live" : "offline"}
        </span>
      }
    >

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
        <MetricWidget
          icon={Brain}
          label="LLM calls/s"
          value={t.llmPerSec.toFixed(1)}
          subvalue={`now ${now.llm}`}
          tone="accent"
          pulse={connected}
          trend={arr.map((b) => b.llm)}
        />
        <MetricWidget
          icon={Zap}
          label="tokens/s"
          value={Math.round(t.tokensPerSec).toLocaleString()}
          subvalue={`now ${(now.tokensIn + now.tokensOut).toLocaleString()}`}
          tone="muted"
          trend={arr.map((b) => b.tokensIn + b.tokensOut)}
        />
        <MetricWidget
          icon={Coins}
          label="spend/s"
          value={money(t.costPerSecMc)}
          subvalue={`now ${money(now.costMc)}`}
          tone="warn"
          trend={arr.map((b) => b.costMc)}
        />
        <MetricWidget
          icon={Wrench}
          label="tool calls/s"
          value={t.toolsPerSec.toFixed(2)}
          subvalue={`now ${now.tools}`}
          tone="accent"
          trend={arr.map((b) => b.tools)}
        />
        <MetricWidget
          icon={Waypoints}
          label={`delegations/${WINDOW}s`}
          value={t.subagentsTotal.toLocaleString()}
          subvalue={`now ${now.subagents}`}
          tone="accent"
          trend={arr.map((b) => b.subagents)}
        />
      </div>

      {/* At-rest content: what's running + what happened lately. */}
      <div className="grid gap-3 lg:grid-cols-2">
        <div className="glass rounded-xl p-3">
          <div className="mb-2 inline-flex items-center gap-1.5 text-xs font-semibold uppercase tracking-normal text-muted">
            <ListTree className="size-3.5" /> active runs
          </div>
          {!active || active.length === 0 ? (
            <p className="text-xs text-muted">nothing running right now</p>
          ) : (
            <ul className="space-y-1">
              {active.map((r) => (
                <li key={r.correlation_id} className="flex items-center gap-2 text-xs">
                  <span className="size-1.5 shrink-0 animate-pulse rounded-full bg-accent" />
                  <button
                    onClick={() => {
                      if (r.correlation_id) {
                        focusRun(r.correlation_id);
                        location.hash = "runs";
                      }
                    }}
                    className="min-w-0 flex-1 truncate text-left transition-colors hover:text-accent"
                    title={r.intent || r.correlation_id}
                  >
                    {humanizeIntent(r.intent) || r.correlation_id}
                  </button>
                  <span className="ml-auto shrink-0 tabular-nums text-muted">{fmtWhen(r.started_unix_ms)}</span>
                </li>
              ))}
            </ul>
          )}
        </div>
        <div className="glass rounded-xl p-3">
          <div className="mb-2 inline-flex items-center gap-1.5 text-xs font-semibold uppercase tracking-normal text-muted">
            <Bell className="size-3.5" /> recent activity
          </div>
          {recent.length === 0 ? (
            <p className="text-xs text-muted">no notable events since this page opened — runs, schedules, and skill changes land here live</p>
          ) : (
            <ul className="space-y-1">
              {recent.map((e) => (
                <li key={e.id || `${e.kind}-${e.seq}`} className="flex items-center gap-2 text-xs">
                  <span
                    className={cn(
                      "shrink-0 rounded bg-panel px-1.5 py-0.5 font-medium",
                      e.kind === "task.failed" || e.kind === "halt" ? "text-bad" : "text-muted",
                    )}
                  >
                    {NOTABLE[String(e.kind)]}
                  </span>
                  <span className="min-w-0 flex-1 truncate text-foreground/80">{eventDetail(e)}</span>
                  <span className="ml-auto shrink-0 tabular-nums text-muted">{fmtWhen(e.ts_unix_ms)}</span>
                </li>
              ))}
            </ul>
          )}
        </div>
      </div>
    </Page>
  );
}
