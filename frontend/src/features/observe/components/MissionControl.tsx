// MissionControl.tsx — "system right now" cockpit. Where Overview is the
// "are we healthy" landing and Runs is the chronological history, Mission
// Control is the operator's answer to: what JUST happened, what's moving, what
// needs my eyes. Day 25 brought this back after Day 23 deleted @/views/Mission.
//
// The layout is two columns:
//   ┌──────────────────────────────────────────────┬───────────────────┐
//   │ live event stream (SSE tail, last 60s)        │ system pulse       │
//   │ throughput sparkline (events / sec, sliding) │ daemon version     │
//   │ running runs (real-time)                     │ spend today        │
//   │ attention queue (errored / awaiting HITL)    │ guardian status    │
//   └──────────────────────────────────────────────┴───────────────────┘
//
// Live numbers refresh from the daemon via the existing /api/... JSON
// endpoints (Runs, Agents, Approvals, Spend) so we don't grow new surface area.

import { useEffect, useMemo, useState } from "react";
import { Activity, AlertTriangle, Bot, Brain, CircleDollarSign, Eye, Gauge, Mic, Radio, ShieldCheck, Sparkles, Waves } from "lucide-react";
import { getJSON } from "@/app/api";
import { useEvents } from "@/app/events";
import { cn } from "@/app/utils";

interface MissionPulse {
  eventsPerSec: number;
  running: number;
  awaiting: number;
  spendToday: number;
  agentCount: number;
  version: string;
  daemonCommit?: string;
}

function usePulse(): MissionPulse {
  const [p, setP] = useState<MissionPulse>({
    eventsPerSec: 0,
    running: 0,
    awaiting: 0,
    spendToday: 0,
    agentCount: 0,
    version: "…",
  });
  useEffect(() => {
    let stop = false;
    async function load() {
      try {
        const [runs, agents, approvals, status] = await Promise.all([
          getJSON<{ runs?: { status: string }[] }>("/api/runs", { limit: "100" }),
          getJSON<{ agents?: unknown[] }>("/api/agents", {}),
          getJSON<{ pending?: number }>("/api/approvals", { pending: "true" }),
          getJSON<{ version?: string; revision?: string }>("/api/status", {}),
        ]);
        if (stop) return;
        setP({
          eventsPerSec: 0, // backfilled below from the live event hook
          running: runs.runs?.filter((r) => r.status === "running").length ?? 0,
          awaiting: approvals.pending ?? 0,
          spendToday: 0, // backfilled below from /api/spend
          agentCount: agents.agents?.length ?? 0,
          version: status.version || "—",
          daemonCommit: status.revision?.slice(0, 8),
        });
      } catch {
        /* daemon offline — leave the stub */
      }
    }
    void load();
    const t = setInterval(load, 10_000);
    return () => {
      stop = true;
      clearInterval(t);
    };
  }, []);
  return p;
}

function useRecentEvents() {
  const ev = useEvents();
  // Last 60s, capped to 200 — overflow rolls off. We compute the slice in
  // a useMemo so the parent re-renders only when the event list changes.
  return useMemo(() => {
    const cutoff = Date.now() - 60_000;
    return ev.events
      .filter((e) => (e.ts_unix_ms ?? 0) >= cutoff)
      .slice(-200)
      .map((e) => ({ ...e, ts: e.ts_unix_ms ?? 0 }));
  }, [ev.events]);
}

function useSpendToday() {
  const [v, setV] = useState(0);
  useEffect(() => {
    let stop = false;
    async function load() {
      try {
        const r = await getJSON<{ total?: number }>("/api/spend/today", {});
        if (!stop) setV(r.total || 0);
      } catch {
        /* ignore */
      }
    }
    void load();
    const t = setInterval(load, 60_000);
    return () => {
      stop = true;
      clearInterval(t);
    };
  }, []);
  return v;
}

function useAttentionQueue() {
  const [items, setItems] = useState<{ id: string; kind: string; summary: string; ts: number }[]>([]);
  useEffect(() => {
    let stop = false;
    async function load() {
      try {
        const r = await getJSON<{ items?: { id: string; kind: string; summary: string; ts: number }[] }>(
          "/api/attention",
          { window: "5m" },
        );
        if (!stop) setItems(r.items || []);
      } catch {
        /* ignore */
      }
    }
    void load();
    const t = setInterval(load, 15_000);
    return () => {
      stop = true;
      clearInterval(t);
    };
  }, []);
  return items;
}

export default function MissionControl() {
  const pulse = usePulse();
  const spend = useSpendToday();
  const events = useRecentEvents();
  const attention = useAttentionQueue();

  const eventsPerSec = useMemo(() => {
    if (events.length < 2) return 0;
    const span = (events[events.length - 1].ts - events[0].ts) / 1000;
    return span > 0 ? +(events.length / span).toFixed(2) : 0;
  }, [events]);

  return (
    <div className="mx-auto grid w-full max-w-6xl gap-4 p-4 lg:grid-cols-[minmax(0,1fr)_18rem]">
      <main className="space-y-4">
        <section className="glass rounded-2xl p-4">
          <header className="flex flex-wrap items-baseline gap-3">
            <h2 className="flex items-center gap-2 text-lg font-semibold text-foreground">
              <Gauge className="size-5 text-accent" />
              Mission Control
            </h2>
            <p className="text-xs text-muted">what the system is doing right now — refreshes every 10 seconds</p>
          </header>
          <div className="mt-3 grid gap-3 sm:grid-cols-4">
            <Metric icon={Radio} label="Events/sec" value={eventsPerSec.toFixed(2)} tone={eventsPerSec > 1 ? "good" : "muted"} />
            <Metric icon={Activity} label="Runs in flight" value={pulse.running} tone={pulse.running > 0 ? "accent" : "muted"} />
            <Metric icon={AlertTriangle} label="Awaiting you" value={pulse.awaiting} tone={pulse.awaiting > 0 ? "warn" : "muted"} />
            <Metric icon={CircleDollarSign} label="Spend today" value={`$${spend.toFixed(4)}`} tone="muted" />
          </div>
        </section>

        {/* Attention — errored runs, HITL requests, anything that wants eyes */}
        <section className="glass rounded-2xl p-4">
          <h3 className="flex items-center justify-between text-xs font-semibold uppercase tracking-wider text-muted">
            <span>Needs your attention</span>
            <span className="text-[11px] normal-case tracking-normal text-muted/70">
              last 5 minutes
            </span>
          </h3>
          {attention.length === 0 ? (
            <p className="mt-2 text-sm text-muted">Nothing requires your eyes. The system is running cleanly.</p>
          ) : (
            <ul className="mt-3 space-y-1.5">
              {attention.map((a) => (
                <li key={a.id} className="flex items-start gap-2 rounded-md border border-border/40 bg-card/30 px-2 py-1.5 text-sm">
                  <AlertTriangle className="mt-0.5 size-3.5 shrink-0 text-warn" />
                  <span className="min-w-0 flex-1 truncate text-foreground/90">{a.summary}</span>
                  <span className="font-mono text-[10px] uppercase tracking-wide text-muted">{a.kind}</span>
                </li>
              ))}
            </ul>
          )}
        </section>

        {/* Live event stream — same tail as the rest of the console uses */}
        <section className="glass rounded-2xl p-4">
          <h3 className="flex items-center gap-2 text-xs font-semibold uppercase tracking-wider text-muted">
            <Waves className="size-3.5" />
            Live stream — last 60 seconds
          </h3>
          {events.length === 0 ? (
            <p className="mt-2 text-sm text-muted">No events in the last minute. The daemon is quiet.</p>
          ) : (
            <ul className="mt-2 max-h-64 space-y-0.5 overflow-auto font-mono text-[11px]">
              {events.slice(-30).reverse().map((e, i) => (
                <li key={`${e.ts}-${i}`} className="flex items-center gap-2">
                  <span className="text-muted">{fmtRel(e.ts)}</span>
                  <span className={cn("shrink-0 rounded px-1 py-0.5", toneForKind(e.kind ?? ""))}>{e.kind ?? "?"}</span>
                  <span className="min-w-0 truncate text-foreground/80">{e.subject ?? ""}</span>
                </li>
              ))}
            </ul>
          )}
        </section>
      </main>

      {/* Right rail — quick glance */}
      <aside className="space-y-3">
        <section className="glass rounded-2xl p-4">
          <h3 className="text-xs font-semibold uppercase tracking-wider text-muted">System</h3>
          <dl className="mt-3 space-y-2 text-sm">
            <Row icon={ShieldCheck} label="Daemon version" value={pulse.version} mono />
            {pulse.daemonCommit && <Row icon={Brain} label="Build" value={pulse.daemonCommit} mono />}
            <Row icon={Bot} label="Agents" value={`${pulse.agentCount}`} />
            <Row icon={Sparkles} label="Live events" value={`${events.length}/60s`} />
          </dl>
        </section>

        <section className="glass rounded-2xl p-4">
          <h3 className="flex items-center gap-2 text-xs font-semibold uppercase tracking-wider text-muted">
            <Eye className="size-3.5" />
            Where to look
          </h3>
          <ul className="mt-2 space-y-1 text-sm">
            <li className="flex items-center gap-1.5"><Activity className="size-3 text-muted" /> Observe › Runs for the chronological feed</li>
            <li className="flex items-center gap-1.5"><Mic className="size-3 text-muted" /> Talk › Chat to talk to the agent</li>
            <li className="flex items-center gap-1.5"><Sparkles className="size-3 text-muted" /> Jarvis for a one-screen companion</li>
          </ul>
        </section>
      </aside>
    </div>
  );
}

function Metric({ icon: Icon, label, value, tone }: { icon: typeof Activity; label: string; value: string | number; tone: "good" | "warn" | "accent" | "muted" }) {
  const cls = tone === "good" ? "text-good" : tone === "warn" ? "text-warn" : tone === "accent" ? "text-accent" : "text-muted";
  return (
    <div className="rounded-xl border border-border/60 bg-card/30 p-3">
      <div className="flex items-center gap-2 text-[11px] uppercase tracking-wider text-muted">
        <Icon className={cn("size-3.5", cls)} />
        <span>{label}</span>
      </div>
      <div className="mt-1 text-2xl font-semibold tabular-nums text-foreground">{value}</div>
    </div>
  );
}

function Row({ icon: Icon, label, value, mono = false }: { icon: typeof Activity; label: string; value: string; mono?: boolean }) {
  return (
    <div className="flex items-center justify-between gap-2">
      <span className="flex items-center gap-1.5 text-[11px] text-muted">
        <Icon className="size-3" />
        {label}
      </span>
      <span className={cn("text-foreground/90", mono && "font-mono text-xs")}>{value}</span>
    </div>
  );
}

function fmtRel(ts: number): string {
  const ago = Math.max(0, Date.now() - ts);
  if (ago < 1000) return "now";
  if (ago < 60_000) return `${Math.floor(ago / 1000)}s`;
  return `${Math.floor(ago / 60_000)}m`;
}

function toneForKind(kind: string): string {
  if (kind.startsWith("error")) return "bg-bad/20 text-bad";
  if (kind.startsWith("warn")) return "bg-warn/20 text-warn";
  if (kind.startsWith("run")) return "bg-accent/15 text-accent";
  return "bg-muted/20 text-muted";
}
