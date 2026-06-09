import { useEffect, useMemo, useState } from "react";
import { Waves, RefreshCw, CalendarClock, Anchor, ShieldCheck, Sparkles, Radio, MessagesSquare, Play, Pause, Heart } from "lucide-react";
import type { LucideIcon } from "lucide-react";
import { getJSON, postAction } from "@/lib/api";
import { useUI } from "@/components/ui/feedback";
import { cn, fmtTime } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { Muted, ErrorText } from "@/components/JsonView";
import { SkeletonList } from "@/components/ui/skeleton";

interface Item {
  seq: number;
  ts_unix_ms?: number;
  kind: string;
  category: string;
  title: string;
  correlation_id?: string;
  detail?: string;
}
interface Feed {
  items?: Item[];
  count?: number;
}

// catMeta colours + icons each self-directed category so the feed reads at a
// glance: timers, standing orders, completion checks, skill lifecycle, pulse.
const catMeta: Record<string, { icon: LucideIcon; tone: string }> = {
  schedule: { icon: CalendarClock, tone: "text-accent" },
  standing: { icon: Anchor, tone: "text-accent" },
  assure: { icon: ShieldCheck, tone: "text-good" },
  skill: { icon: Sparkles, tone: "text-accent" },
  pulse: { icon: Radio, tone: "text-muted" },
  board: { icon: MessagesSquare, tone: "text-accent" },
};

// Autonomy is the "living organism" pane: a curated, newest-first timeline of
// everything the daemon did ON ITS OWN — schedules and standing orders firing,
// skills learned/promoted, do-it-for-sure completion checks, pulse briefings.
// Unlike the raw Live Stream it keeps only self-directed milestones, so the
// operator can see their Jarvis acting unprompted. Read-only; polls live.
export function Autonomy() {
  const [feed, setFeed] = useState<Feed | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [cat, setCat] = useState<string | null>(null);

  async function reload() {
    setLoading(true);
    try {
      const d = await getJSON<Feed>("/api/autonomy", { limit: "150" });
      setFeed(d);
      setErr(null);
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setLoading(false);
    }
  }
  useEffect(() => {
    reload();
    const id = setInterval(reload, 6000);
    return () => clearInterval(id);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const cats = useMemo(() => {
    const c: Record<string, number> = {};
    for (const it of feed?.items || []) c[it.category] = (c[it.category] || 0) + 1;
    return Object.entries(c).sort((a, b) => b[1] - a[1]);
  }, [feed]);
  const items = useMemo(
    () => (feed?.items || []).filter((it) => !cat || it.category === cat),
    [feed, cat],
  );

  return (
    <div className="flex h-full min-h-0 flex-col gap-3">
      <div className="flex items-center gap-2">
        <h2 className="flex items-center gap-2 text-sm font-semibold">
          <Waves className="size-4 text-accent" /> Autonomy
        </h2>
        <span className="text-xs text-muted">
          {feed ? `${feed.count ?? 0} self-directed event${feed.count === 1 ? "" : "s"}` : ""}
        </span>
        <Button variant="ghost" size="sm" className="ml-auto" onClick={reload} disabled={loading} title="Reload">
          <RefreshCw className={cn("size-3.5", loading && "animate-spin")} />
        </Button>
      </div>

      <PulseControl />

      {cats.length > 0 && (
        <div className="flex flex-wrap items-center gap-1.5">
          <button
            onClick={() => setCat(null)}
            className={cn(
              "rounded-full border px-2.5 py-0.5 text-[11px] transition-colors",
              cat === null ? "border-accent bg-accent/10 text-accent" : "border-border bg-panel text-muted hover:text-foreground",
            )}
          >
            all
          </button>
          {cats.map(([name, n]) => (
            <button
              key={name}
              onClick={() => setCat(name === cat ? null : name)}
              className={cn(
                "inline-flex items-center gap-1 rounded-full border px-2.5 py-0.5 text-[11px] transition-colors",
                cat === name ? "border-accent bg-accent/10 text-accent" : "border-border bg-panel text-muted hover:text-foreground",
              )}
            >
              {name}
              <span className="opacity-60">{n}</span>
            </button>
          ))}
        </div>
      )}

      {err ? (
        <ErrorText>{err}</ErrorText>
      ) : !feed ? (
        <SkeletonList count={4} lines={2} />
      ) : items.length === 0 ? (
        <Muted>
          nothing autonomous yet — when a schedule or standing order fires, a skill is learned, or a
          do-it-for-sure check runs, it shows here. The system is quiet, not asleep.
        </Muted>
      ) : (
        <div className="min-h-0 flex-1 overflow-auto">
          <ul className="space-y-1.5">
            {items.map((it) => {
              const meta = catMeta[it.category] || { icon: Waves, tone: "text-muted" };
              const Icon = meta.icon;
              return (
                <li key={it.seq} className="flex items-start gap-2.5 rounded-lg border border-border bg-card p-2.5">
                  <Icon className={cn("mt-0.5 size-4 shrink-0", meta.tone)} />
                  <div className="min-w-0 flex-1">
                    <div className="flex items-center gap-2">
                      <span className="text-sm font-medium">{it.title}</span>
                      <span className="rounded bg-panel px-1.5 py-0.5 text-[10px] uppercase tracking-wider text-muted">{it.category}</span>
                      <span className="ml-auto font-mono text-[10px] text-muted opacity-70">{fmtTime(it.ts_unix_ms)}</span>
                    </div>
                    {it.detail && <p className="mt-0.5 truncate text-xs text-foreground/80">{it.detail}</p>}
                  </div>
                </li>
              );
            })}
          </ul>
        </div>
      )}
    </div>
  );
}

interface PulseStatus {
  enabled?: boolean;
  running?: boolean;
  paused?: boolean;
  beats?: number;
  observers?: number;
  cadence_ms?: number;
  last_tick_ms?: number;
}

// PulseControl surfaces the proactive heartbeat — the engine that drives the daemon's
// self-directed work — with its live status and a pause/resume master switch (M743).
// Pausing suppresses new beats (in-flight work finishes); the daemon goes reactive-only
// until resumed. Previously this was reachable only via `agt pulse` on the CLI.
export function PulseControl() {
  const ui = useUI();
  const [st, setSt] = useState<PulseStatus | null>(null);
  const [busy, setBusy] = useState(false);

  async function load() {
    try {
      setSt(await getJSON<PulseStatus>("/api/pulse"));
    } catch {
      setSt(null);
    }
  }
  useEffect(() => {
    load();
    const id = setInterval(load, 6000);
    return () => clearInterval(id);
  }, []);

  async function toggle() {
    if (!st) return;
    setBusy(true);
    try {
      await postAction(st.paused ? "/api/pulse/resume" : "/api/pulse/pause", {});
      ui.toast(st.paused ? "Pulse resumed — proactivity is back on" : "Pulse paused — the daemon is reactive-only", "success");
      await load();
    } catch (e) {
      ui.toast((e as Error).message, "error");
    } finally {
      setBusy(false);
    }
  }

  if (!st) return null;
  if (!st.enabled) {
    return (
      <div className="flex items-center gap-2 rounded-lg border border-border bg-card px-3 py-2 text-xs text-muted">
        <Radio className="size-3.5" /> Pulse is disabled on this daemon (set <code className="rounded bg-panel px-1">AGEZT_PULSE</code> to enable the proactive heartbeat).
      </div>
    );
  }

  const paused = !!st.paused;
  const cadence = st.cadence_ms ? `${Math.round(st.cadence_ms / 1000)}s` : "—";
  return (
    <div className="flex flex-wrap items-center gap-2 rounded-lg border border-border bg-card px-3 py-2">
      <Heart className={cn("size-4", paused ? "text-muted" : "animate-pulse fill-current text-bad")} />
      <span className="text-sm font-semibold">Proactive heartbeat</span>
      <span
        className={cn(
          "rounded-full px-1.5 py-0.5 text-[10px] font-semibold uppercase tracking-wider",
          paused ? "bg-panel text-muted" : "bg-good/15 text-good",
        )}
      >
        {paused ? "paused" : "running"}
      </span>
      <span className="text-[11px] text-muted">
        {st.beats ?? 0} beat{st.beats === 1 ? "" : "s"} · every {cadence}
        {st.observers != null ? ` · ${st.observers} observer${st.observers === 1 ? "" : "s"}` : ""}
        {st.last_tick_ms ? ` · last ${fmtTime(st.last_tick_ms)}` : ""}
      </span>
      <Button size="sm" variant={paused ? "default" : "ghost"} className="ml-auto" onClick={toggle} disabled={busy} title={paused ? "Resume the heartbeat" : "Pause the heartbeat"}>
        {busy ? <RefreshCw className="size-3.5 animate-spin" /> : paused ? <Play className="size-3.5" /> : <Pause className="size-3.5" />}
        {paused ? "Resume" : "Pause"}
      </Button>
    </div>
  );
}
