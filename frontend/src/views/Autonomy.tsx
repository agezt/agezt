import { useEffect, useMemo, useState, type ReactNode } from "react";
import {
  Waves,
  RefreshCw,
  CalendarClock,
  Anchor,
  ShieldCheck,
  Sparkles,
  Radio,
  MessagesSquare,
  Play,
  Pause,
  Heart,
  Zap,
  Eye,
  Plus,
  Activity,
  X,
  Moon,
  LifeBuoy,
  GitBranch,
  Bell,
  Gauge,
} from "lucide-react";
import type { LucideIcon } from "lucide-react";
import { getJSON, postAction } from "@/lib/api";
import { useEvents } from "@/lib/events";
import { useUI } from "@/components/ui/feedback";
import { cn, fmtTime } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { Muted, ErrorText } from "@/components/JsonView";
import { SkeletonList } from "@/components/ui/skeleton";
import { Page } from "@/components/ui/page";
import { DoctorIncidentTrees } from "@/components/DoctorIncidentTrees";
import { IncidentBadges } from "@/components/IncidentBadges";
import { openIncident } from "@/lib/incidentnav";
import { Disclosure } from "@/components/ui/disclosure";
import {
  autonomyEventMatches,
  doctorIncidentTrees,
  type AutonomyItem,
} from "@/lib/autonomy";

interface Feed {
  items?: AutonomyItem[];
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
  doctor: { icon: LifeBuoy, tone: "text-warn" },
  delegation: { icon: GitBranch, tone: "text-accent2" },
};

// Autonomy is the "living organism" pane: a curated, newest-first timeline of
// everything the daemon did ON ITS OWN - schedules and standing orders firing,
// skills learned/promoted, do-it-for-sure completion checks, pulse briefings.
// Unlike the raw Live Stream it keeps only self-directed milestones, so the
// operator can see their Jarvis acting unprompted. Read-only; polls live.
export function Autonomy() {
  const { subscribe } = useEvents();
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
  useEffect(() => {
    let t: ReturnType<typeof setTimeout> | undefined;
    const off = subscribe((e) => {
      if (!autonomyEventMatches(e)) return;
      if (t) clearTimeout(t);
      t = setTimeout(() => void reload(), 700);
    });
    return () => {
      if (t) clearTimeout(t);
      off();
    };
  }, [subscribe]);

  const cats = useMemo(() => {
    const c: Record<string, number> = {};
    for (const it of feed?.items || [])
      c[it.category] = (c[it.category] || 0) + 1;
    return Object.entries(c).sort((a, b) => b[1] - a[1]);
  }, [feed]);
  const items = useMemo(
    () => (feed?.items || []).filter((it) => !cat || it.category === cat),
    [feed, cat],
  );
  const doctorIncidents = useMemo(
    () => doctorIncidentTrees(feed?.items, 8),
    [feed],
  );

  return (
    <Page
      icon={Waves}
      title="Autonomy"
      description="A live timeline of everything the daemon did on its own - schedules, standing orders, learned skills, pulse briefings."
      actions={
        <>
          <span className="text-xs text-muted">
            {feed
              ? `${feed.count ?? 0} self-directed event${feed.count === 1 ? "" : "s"}`
              : ""}
          </span>
          <Button
            variant="ghost"
            size="sm"
            onClick={reload}
            disabled={loading}
            title="Reload"
          >
            <RefreshCw
              className={cn("size-3.5", loading && "animate-spin")}
            />
          </Button>
        </>
      }
    >
      <PulseControl />

      {cats.length > 0 && (
        <div className="flex flex-wrap items-center gap-1.5">
          <button
            onClick={() => setCat(null)}
            className={cn(
              "rounded-full border px-2.5 py-0.5 text-[11px] transition-colors",
              cat === null
                ? "border-accent bg-accent/10 text-accent"
                : "border-border bg-panel text-muted hover:text-foreground",
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
                cat === name
                  ? "border-accent bg-accent/10 text-accent"
                  : "border-border bg-panel text-muted hover:text-foreground",
              )}
            >
              {name}
              <span className="opacity-60">{n}</span>
            </button>
          ))}
        </div>
      )}

      {doctorIncidents.length > 0 && (cat === null || cat === "doctor") && (
        <div className="glass rounded-xl p-2.5">
          <div className="mb-2 flex items-center gap-2 text-sm font-medium">
            <LifeBuoy className="size-4 text-warn" />
            Repair incident trees
          </div>
          <DoctorIncidentTrees
            trees={doctorIncidents}
            compact
            onOpenIncident={openIncident}
          />
        </div>
      )}

      {err ? (
        <ErrorText>{err}</ErrorText>
      ) : !feed ? (
        <SkeletonList count={4} lines={2} />
      ) : items.length === 0 ? (
        <Muted>
          nothing autonomous yet - when a schedule or standing order fires, a
          skill is learned, or a do-it-for-sure check runs, it shows here. The
          system is quiet, not asleep.
        </Muted>
      ) : (
        <ul className="space-y-1.5">
          {items.map((it) => {
              const meta = catMeta[it.category] || {
                icon: Waves,
                tone: "text-muted",
              };
              const Icon = meta.icon;
              return (
                <li
                  key={it.seq}
                  className="flex items-start gap-2.5 glass rounded-xl p-2.5"
                >
                  <Icon className={cn("mt-0.5 size-4 shrink-0", meta.tone)} />
                  <div className="min-w-0 flex-1">
                    <div className="flex items-center gap-2">
                      <span className="text-sm font-medium">{it.title}</span>
                      <span className="rounded bg-panel px-1.5 py-0.5 text-xs uppercase tracking-normal text-muted">
                        {it.category}
                      </span>
                      {it.category === "doctor" && <IncidentBadges item={it} />}
                      <span className="ml-auto font-mono text-xs text-muted opacity-70">
                        {fmtTime(it.ts_unix_ms)}
                      </span>
                    </div>
                    {it.detail && (
                      <p className="mt-0.5 truncate text-xs text-foreground/80">
                        {it.detail}
                      </p>
                    )}
                  </div>
                </li>
              );
            })}
        </ul>
      )}
    </Page>
  );
}

interface PulseStatus {
  enabled?: boolean;
  running?: boolean;
  paused?: boolean;
  beats?: number;
  observers?: string[];
  removable?: string[];
  cadence_ms?: number;
  last_tick_ms?: number;
  dial?: string;
  quiet?: { enabled?: boolean; start?: number; end?: number; spec?: string };
  digest_pending?: number;
}

// The proactivity dial (M758): how much reaches you - quiet (alerts only), balanced
// (notify and up), chatty (digests too).
const DIALS = ["quiet", "balanced", "chatty"];

// Cadence presets for live retuning (M757), in seconds - how often the agent checks in.
const CADENCE_PRESETS = [10, 30, 60, 300, 900, 3600];

// cadenceLabel formats a second count as a compact human interval (10s, 5m, 1h).
export function cadenceLabel(sec: number): string {
  if (sec < 60) return `${sec}s`;
  if (sec < 3600) return `${Math.round(sec / 60)}m`;
  return `${Math.round(sec / 3600)}h`;
}

// PulseControl surfaces the proactive heartbeat - the engine that drives the daemon's
// self-directed work - with its live status and a pause/resume master switch (M743),
// an on-demand "beat now" (M756), and a live cadence selector (M757). Pausing
// suppresses new beats (in-flight work finishes); the daemon goes reactive-only until
// resumed. Previously this was reachable only via `agt pulse` on the CLI.
export function PulseControl() {
  const ui = useUI();
  const [st, setSt] = useState<PulseStatus | null>(null);
  const [busy, setBusy] = useState(false);
  const [beating, setBeating] = useState(false);
  const [watchKind, setWatchKind] = useState<"" | "disk" | "probe">("");
  const [watchPath, setWatchPath] = useState("");
  const [watchPct, setWatchPct] = useState("10");
  const [probeName, setProbeName] = useState("");
  const [probeCmd, setProbeCmd] = useState("");
  const [quietHours, setQuietHours] = useState("");
  const [quietOpen, setQuietOpen] = useState(false);

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
      await postAction(
        st.paused ? "/api/pulse/resume" : "/api/pulse/pause",
        {},
      );
      ui.toast(
        st.paused
          ? "Pulse resumed - proactivity is back on"
          : "Pulse paused - the daemon is reactive-only",
        "success",
      );
      await load();
    } catch (e) {
      ui.toast((e as Error).message, "error");
    } finally {
      setBusy(false);
    }
  }

  // Beat now (M756): fire one heartbeat on demand - the agent checks its observers and
  // may raise an initiative immediately, without waiting for the cadence. Works even
  // while paused (an explicit one-off override). Results surface in the feed.
  async function beatNow() {
    setBeating(true);
    try {
      await postAction("/api/pulse/beat", {});
      ui.toast("Heartbeat triggered - the agent is checking in now", "success");
      setTimeout(load, 1500);
    } catch (e) {
      ui.toast((e as Error).message, "error");
    } finally {
      setBeating(false);
    }
  }

  // Retune (M757): change how often the agent checks in, live. Runtime-only - resets
  // to the configured default on restart.
  async function setCadence(seconds: string) {
    try {
      await postAction("/api/pulse/cadence", { seconds });
      ui.toast(
        `Heartbeat now every ${cadenceLabel(Number(seconds))}`,
        "success",
      );
      await load();
    } catch (e) {
      ui.toast((e as Error).message, "error");
    }
  }

  // Dial (M758): change how proactive/chatty the agent is, live (quiet/balanced/chatty).
  async function setDial(dial: string) {
    try {
      await postAction("/api/pulse/dial", { dial });
      ui.toast(`Proactivity dial → ${dial}`, "success");
      await load();
    } catch (e) {
      ui.toast((e as Error).message, "error");
    }
  }

  // Flush digest (M761): deliver the briefs the pulse is holding now, instead of
  // waiting for the periodic flush.
  async function flushDigest() {
    try {
      const r = await postAction<{ flushed?: number }>("/api/pulse/flush", {});
      const n = r?.flushed ?? 0;
      ui.toast(
        n > 0
          ? `Flushed ${n} held brief${n === 1 ? "" : "s"}`
          : "Nothing held in the digest",
        n > 0 ? "success" : "info",
      );
      await load();
    } catch (e) {
      ui.toast((e as Error).message, "error");
    }
  }

  // Add a disk watch (M767): the agent will alert when free space on a path drops
  // below the threshold. Takes effect on the next beat.
  async function addWatch() {
    if (!watchPath.trim()) return;
    try {
      const r = await postAction<{ observer?: string }>("/api/pulse/watch", {
        path: watchPath.trim(),
        min_pct: watchPct,
      });
      ui.toast(
        `Now watching ${r?.observer || watchPath.trim()} - alerts under ${watchPct}% free`,
        "success",
      );
      setWatchPath("");
      setWatchKind("");
      setWatchPct("10");
      await load();
    } catch (e) {
      ui.toast((e as Error).message, "error");
    }
  }

  // Add a command-probe watch (M768): the agent runs the command each beat and alerts
  // when its pass/fail flips - e.g. watch CI or a build.
  async function addProbe() {
    if (!probeName.trim() || !probeCmd.trim()) return;
    try {
      const r = await postAction<{ observer?: string }>("/api/pulse/probe", {
        name: probeName.trim(),
        command: probeCmd.trim(),
      });
      ui.toast(
        `Now watching ${r?.observer || probeName.trim()} - alerts when it flips`,
        "success",
      );
      setProbeName("");
      setProbeCmd("");
      setWatchKind("");
      await load();
    } catch (e) {
      ui.toast((e as Error).message, "error");
    }
  }

  // Set or clear quiet hours (M770): during the window only alert/act briefs break
  // through, regardless of the dial - so the agent won't ping you overnight. `spec` is
  // "START-END" 24h (e.g. "22-7"); an empty spec clears it.
  async function setQuiet(spec: string) {
    try {
      const r = await postAction<{ quiet?: string }>("/api/pulse/quiet", {
        hours: spec,
      });
      ui.toast(
        r?.quiet ? `Quiet hours set to ${r.quiet}` : "Quiet hours cleared",
        "success",
      );
      setQuietHours("");
      setQuietOpen(false);
      await load();
    } catch (e) {
      ui.toast((e as Error).message, "error");
    }
  }

  // Remove a runtime-added watch (M769): stop watching a disk or command without a
  // restart. Only runtime-added observers (reported in `removable`) offer this; the
  // built-in self:health observer can't be removed.
  async function removeObserver(name: string) {
    if (
      !(await ui.confirm({
        title: `Stop watching ${name}?`,
        message: "The agent will no longer check this on each beat.",
        confirmLabel: "Stop watching",
        danger: true,
      }))
    )
      return;
    try {
      await postAction("/api/pulse/unwatch", { name });
      ui.toast(`Stopped watching ${name}`, "success");
      await load();
    } catch (e) {
      ui.toast((e as Error).message, "error");
    }
  }

  if (!st) return null;
  if (!st.enabled) {
    return (
      <div className="flex items-center gap-2 glass rounded-xl px-3 py-2 text-xs text-muted">
        <Radio className="size-3.5" /> Pulse is disabled on this daemon (set{" "}
        <code className="rounded bg-panel px-1">AGEZT_PULSE</code> to enable the
        proactive heartbeat).
      </div>
    );
  }

  const paused = !!st.paused;
  const curSec = st.cadence_ms ? Math.round(st.cadence_ms / 1000) : 0;
  return (
    <div className="space-y-2 glass rounded-xl px-3 py-2">
      <div className="flex flex-wrap items-center gap-2">
        <Heart
          className={cn(
            "size-4",
            paused ? "text-muted" : "animate-pulse fill-current text-bad",
          )}
        />
        <span className="text-sm font-semibold">Proactive heartbeat</span>
        <span
          className={cn(
            "rounded-full px-1.5 py-0.5 text-xs font-semibold uppercase tracking-normal",
            paused ? "bg-panel text-muted" : "bg-good/15 text-good",
          )}
        >
          {paused ? "paused" : "running"}
        </span>
        <span className="text-[11px] text-muted">
          {st.beats ?? 0} beat{st.beats === 1 ? "" : "s"}
          {st.observers != null
            ? ` · ${st.observers.length} observer${st.observers.length === 1 ? "" : "s"}`
            : ""}
          {st.last_tick_ms ? ` · last ${fmtTime(st.last_tick_ms)}` : ""}
        </span>
        <div
          className="ml-auto inline-flex items-center gap-1 rounded-lg border border-border bg-panel p-1"
          aria-label="Proactivity dial"
          title="How proactive the agent is (quiet=alerts only, chatty=digests too)"
        >
          {DIALS.map((d) => {
            const active = (DIALS.includes(st.dial || "") ? st.dial : "balanced") === d;
            return (
              <button
                key={d}
                type="button"
                onClick={() => setDial(d)}
                className={cn(
                  "inline-flex h-6 items-center gap-1 rounded-md px-2 text-xs font-medium transition-colors",
                  active ? "bg-accent text-accent-foreground" : "text-muted hover:bg-card hover:text-foreground",
                )}
                aria-pressed={active}
              >
                <Bell className="size-3" /> {d}
              </button>
            );
          })}
        </div>
        <div
          className="inline-flex items-center gap-1 rounded-lg border border-border bg-panel p-1"
          aria-label="Heartbeat cadence"
          title="How often the agent checks in (live; resets to the default on restart)"
        >
          {!CADENCE_PRESETS.includes(curSec) && curSec > 0 && (
            <span className="inline-flex h-6 items-center gap-1 rounded-md bg-card px-2 text-xs text-muted">
              <Gauge className="size-3" /> {cadenceLabel(curSec)}
            </span>
          )}
          {CADENCE_PRESETS.map((s) => {
            const active = curSec === s;
            return (
              <button
                key={s}
                type="button"
                onClick={() => setCadence(String(s))}
                className={cn(
                  "h-6 min-w-9 rounded-md px-2 text-xs font-medium transition-colors",
                  active ? "bg-accent text-accent-foreground" : "text-muted hover:bg-card hover:text-foreground",
                )}
                aria-pressed={active}
              >
                {cadenceLabel(s)}
              </button>
            );
          })}
        </div>
        {(st.digest_pending ?? 0) > 0 && (
          <Button
            size="sm"
            variant="ghost"
            onClick={flushDigest}
            title="Deliver the briefs the agent is holding in its digest now"
          >
            <MessagesSquare className="size-3.5" /> Flush digest (
            {st.digest_pending})
          </Button>
        )}
        <Button
          size="sm"
          variant="ghost"
          onClick={beatNow}
          disabled={beating}
          title="Trigger one heartbeat now (think now)"
        >
          {beating ? (
            <RefreshCw className="size-3.5 animate-spin" />
          ) : (
            <Zap className="size-3.5" />
          )}
          Beat now
        </Button>
        <Button
          size="sm"
          variant={paused ? "default" : "ghost"}
          onClick={toggle}
          disabled={busy}
          title={paused ? "Resume the heartbeat" : "Pause the heartbeat"}
        >
          {busy ? (
            <RefreshCw className="size-3.5 animate-spin" />
          ) : paused ? (
            <Play className="size-3.5" />
          ) : (
            <Pause className="size-3.5" />
          )}
          {paused ? "Resume" : "Pause"}
        </Button>
      </div>

      {/* The live observer set - each is polled every beat. Runtime-added watches
          (M767/M768) carry a remove control (M769); built-ins like self:health don't. */}
      {(st.observers?.length ?? 0) > 0 && (
        <div className="flex flex-wrap items-center gap-1.5 border-t border-border/60 pt-2 text-[11px] text-muted">
          <span className="text-muted">observers:</span>
          {st.observers!.map((name, i) => {
            const canRemove = (st.removable ?? []).includes(name);
            return (
              <span
                key={`${name}-${i}`}
                className="inline-flex items-center gap-1 rounded-full border border-border bg-panel px-2 py-0.5 font-mono"
              >
                {name}
                {canRemove && (
                  <button
                    onClick={() => removeObserver(name)}
                    className="text-muted transition-colors hover:text-bad"
                    title={`Stop watching ${name}`}
                    aria-label={`Stop watching ${name}`}
                  >
                    <X className="size-3" />
                  </button>
                )}
              </span>
            );
          })}
        </div>
      )}

      {/* Add a watch - disk (M767) or command-probe (M768) */}
      <div className="flex flex-wrap items-center gap-1.5 border-t border-border/60 pt-2 text-[11px] text-muted">
        <span className="text-muted">watch:</span>
        <button
          onClick={() => setWatchKind("disk")}
          className={cn(
            "inline-flex items-center gap-1 transition-colors",
            watchKind === "disk"
              ? "text-accent"
              : "text-accent/80 hover:text-accent",
          )}
          title="Have the agent watch a disk and alert when it's low on space"
        >
          <Eye className="size-3" /> a disk
        </button>
        <button
          onClick={() => setWatchKind("probe")}
          className={cn(
            "inline-flex items-center gap-1 transition-colors",
            watchKind === "probe"
              ? "text-accent"
              : "text-accent/80 hover:text-accent",
          )}
          title="Have the agent run a command each beat and alert when its pass/fail flips (e.g. CI, a build)"
        >
          <Activity className="size-3" /> a command
        </button>
        {watchKind === "disk" && (
          <PulseModal title="Watch disk" icon={Eye} onClose={() => setWatchKind("")}>
            <div className="space-y-2">
              <PulseFormBlock icon={Eye} title="Target" meta={watchPath.trim() || "disk path required"} defaultOpen>
                <label className="flex flex-col gap-1 text-[11px] text-muted">
                  Path
                  <input
                    value={watchPath}
                    onChange={(e) => setWatchPath(e.target.value)}
                    onKeyDown={(e) => {
                      if (e.key === "Enter") addWatch();
                    }}
                    placeholder="path (e.g. / or C:\\)"
                    aria-label="Watch disk path"
                    className="h-9 rounded-md border border-border bg-panel px-2 font-mono text-xs text-foreground outline-none focus-visible:border-accent"
                  />
                </label>
              </PulseFormBlock>
              <PulseFormBlock icon={Gauge} title="Guard" meta={`alert under ${watchPct || "?"}% free`}>
                <label className="flex flex-col gap-1 text-[11px] text-muted">
                  Alert under %
                  <input
                    type="number"
                    min={1}
                    max={99}
                    value={watchPct}
                    onChange={(e) => setWatchPct(e.target.value)}
                    aria-label="Watch min percent free"
                    className="h-9 rounded-md border border-border bg-panel px-2 text-xs text-foreground outline-none focus-visible:border-accent"
                  />
                </label>
              </PulseFormBlock>
            </div>
            <div className="mt-4 flex justify-end">
              <Button size="sm" onClick={addWatch} disabled={!watchPath.trim()}>
              <Plus className="size-3.5" /> Watch
            </Button>
            </div>
          </PulseModal>
        )}
        {watchKind === "probe" && (
          <PulseModal title="Watch command" icon={Activity} onClose={() => setWatchKind("")}>
            <div className="space-y-2">
              <PulseFormBlock icon={Activity} title="Signal" meta={probeName.trim() || "probe name required"} defaultOpen>
                <label className="flex flex-col gap-1 text-[11px] text-muted">
                  Name
                  <input
                    value={probeName}
                    onChange={(e) => setProbeName(e.target.value)}
                    placeholder="name (e.g. ci)"
                    aria-label="Probe name"
                    className="h-9 rounded-md border border-border bg-panel px-2 text-xs text-foreground outline-none focus-visible:border-accent"
                  />
                </label>
              </PulseFormBlock>
              <PulseFormBlock icon={Play} title="Command" meta={probeCmd.trim() || "command required"} defaultOpen>
                <label className="flex flex-col gap-1 text-[11px] text-muted">
                  Command
                  <input
                    value={probeCmd}
                    onChange={(e) => setProbeCmd(e.target.value)}
                    onKeyDown={(e) => {
                      if (e.key === "Enter") addProbe();
                    }}
                    placeholder="command (e.g. make test)"
                    aria-label="Probe command"
                    className="h-9 rounded-md border border-border bg-panel px-2 font-mono text-xs text-foreground outline-none focus-visible:border-accent"
                  />
                </label>
              </PulseFormBlock>
            </div>
            <div className="mt-4 flex justify-end">
            <Button
              size="sm"
              onClick={addProbe}
              disabled={!probeName.trim() || !probeCmd.trim()}
            >
              <Plus className="size-3.5" /> Watch
            </Button>
            </div>
          </PulseModal>
        )}
      </div>

      {/* Quiet hours (M770): during the window only alert/act briefs break through,
          regardless of the dial - so the agent won't ping you overnight. */}
      <div className="flex flex-wrap items-center gap-1.5 border-t border-border/60 pt-2 text-[11px] text-muted">
        <span className="inline-flex items-center gap-1 text-muted">
          <Moon className="size-3" /> quiet hours:
        </span>
        {st.quiet?.enabled ? (
          <>
            <span className="inline-flex items-center gap-1 rounded-full border border-border bg-panel px-2 py-0.5 font-mono">
              {String(st.quiet.start ?? 0).padStart(2, "0")}:00–
              {String(st.quiet.end ?? 0).padStart(2, "0")}:00
              <button
                onClick={() => setQuiet("")}
                className="text-muted transition-colors hover:text-bad"
                title="Turn off quiet hours"
                aria-label="Clear quiet hours"
              >
                <X className="size-3" />
              </button>
            </span>
            <span>only alerts break through</span>
          </>
        ) : (
          <span>off - the agent may notify any hour</span>
        )}
        <Button size="sm" variant="ghost" className="ml-auto" onClick={() => setQuietOpen(true)}>
          <Moon className="size-3.5" /> Set
        </Button>
        {quietOpen && (
          <PulseModal title="Quiet hours" icon={Moon} onClose={() => setQuietOpen(false)}>
            <PulseFormBlock icon={Moon} title="Window" meta={quietHours.trim() || "22-7 format"} defaultOpen>
              <label className="flex flex-col gap-1 text-[11px] text-muted">
                Window
                <input
                  value={quietHours}
                  onChange={(e) => setQuietHours(e.target.value)}
                  onKeyDown={(e) => {
                    if (e.key === "Enter" && quietHours.trim())
                      setQuiet(quietHours.trim());
                  }}
                  placeholder="22-7"
                  aria-label="Quiet hours window"
                  className="h-9 rounded-md border border-border bg-panel px-2 font-mono text-xs text-foreground outline-none focus-visible:border-accent"
                />
              </label>
            </PulseFormBlock>
            <div className="mt-4 flex justify-end">
              <Button
                size="sm"
                onClick={() => setQuiet(quietHours.trim())}
                disabled={!quietHours.trim()}
              >
                Set
              </Button>
            </div>
          </PulseModal>
        )}
      </div>
    </div>
  );
}

function PulseModal({
  title,
  icon: Icon,
  children,
  onClose,
}: {
  title: string;
  icon: LucideIcon;
  children: ReactNode;
  onClose: () => void;
}) {
  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-bg/75 p-4 backdrop-blur-sm" role="dialog" aria-modal="true" aria-label={title}>
      <div className="glass flex max-h-[86vh] w-full max-w-xl flex-col overflow-hidden rounded-xl border border-accent/25 shadow-e3">
        <div className="flex items-center gap-2 border-b border-border/70 px-4 py-3">
          <span className="grid size-8 place-items-center rounded-lg bg-accent/12 text-accent">
            <Icon className="size-4" />
          </span>
          <h2 className="min-w-0 flex-1 truncate text-sm font-semibold text-foreground">{title}</h2>
          <Button variant="ghost" size="icon" onClick={onClose} aria-label="Close pulse modal">
            <X className="size-4" />
          </Button>
        </div>
        <div className="min-h-0 overflow-auto p-4">{children}</div>
      </div>
    </div>
  );
}

function PulseFormBlock({
  icon: Icon,
  title,
  meta,
  children,
  defaultOpen = false,
}: {
  icon: LucideIcon;
  title: string;
  meta: string;
  children: ReactNode;
  defaultOpen?: boolean;
}) {
  return (
    <Disclosure
      defaultOpen={defaultOpen}
      className="rounded-lg border border-border bg-panel/45"
      summaryClassName="px-2.5 py-2"
      contentClassName="px-2.5 pb-2"
      summary={
        <span className="flex min-w-0 items-center gap-2">
          <span className="grid size-7 shrink-0 place-items-center rounded-md border border-border bg-background/70 text-accent">
            <Icon className="size-3.5" />
          </span>
          <span className="min-w-0 flex-1">
            <span className="block truncate text-sm font-semibold text-foreground">{title}</span>
            <span className="block truncate text-[11px] font-normal text-muted">{meta}</span>
          </span>
        </span>
      }
    >
      {children}
    </Disclosure>
  );
}
