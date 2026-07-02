import { useEffect, useMemo, useRef, useState } from "react";
import {
  Sparkles, Mic, Zap, UserRound, RefreshCw, ArrowRight, Activity, Ear, Volume2, HeartPulse, Check, X, Power,
} from "lucide-react";
import { getJSON, postAction, authHeaders } from "@/lib/api";
import type { AgentEvent } from "@/lib/events";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { Page } from "@/components/ui/page";
import { AnimatedNumber } from "@/components/AnimatedNumber";
import { useUI } from "@/components/ui/feedback";

// Jarvis — the "presence" surface. The three pillars that turn AGEZT from a tool
// into a companion, shown together as one live status: it HEARS you (voice, M998),
// it ACTS for you (Pulse initiative, M999), and it KNOWS you (operator profile,
// M1000). Every number on this page is live; nothing here is decorative-only.

const PROFILE_PREFIX = "operator profile: ";

interface PulseStatus {
  running?: boolean;
  paused?: boolean;
  beats?: number;
  observers?: string[];
  cadence_ms?: number;
  dial?: string;
  initiative?: string; // off | ask | act (M999)
}
interface MemRecord {
  id?: string;
  subject?: string;
  content?: string;
  type?: string;
}

interface PulseAsk {
  issue_key: string;
  source?: string;
  summary?: string;
  reason?: string;
}

interface StandingOrder {
  id?: string;
  slug?: string;
  name?: string;
  enabled?: boolean;
}

// The seeded responder that turns initiative events (act, or an approved ask) into
// a governed run. It ships DISABLED, so the whole act/ask loop is inert until armed.
function isInitiativeResponder(o: StandingOrder): boolean {
  return o.slug === "guardian-initiative" || /initiative/i.test(o.name || "");
}

type VoiceState = "probing" | "natural" | "browser";
type HearState = "probing" | "server" | "browser";

// navigate to another view by hash (the app routes on location.hash).
function go(id: string) {
  window.location.hash = id;
}

// Compact relative time for the recent-initiative feed ("3m", "2h", "just now").
function agoLabel(ms?: number): string {
  if (!ms) return "";
  const s = Math.max(0, Math.floor((Date.now() - ms) / 1000));
  if (s < 45) return "just now";
  if (s < 3600) return `${Math.round(s / 60)}m`;
  if (s < 86400) return `${Math.round(s / 3600)}h`;
  return `${Math.round(s / 86400)}d`;
}

export function Jarvis() {
  const ui = useUI();
  const [pulse, setPulse] = useState<PulseStatus | null>(null);
  const [records, setRecords] = useState<MemRecord[] | null>(null);
  const [voice, setVoice] = useState<VoiceState>("probing");
  const [hear, setHear] = useState<HearState>("probing");
  const [asks, setAsks] = useState<PulseAsk[]>([]);
  const [responder, setResponder] = useState<StandingOrder | null>(null);
  const [rebuilding, setRebuilding] = useState(false);
  const [resolving, setResolving] = useState<string | null>(null);
  const [arming, setArming] = useState(false);
  const [beating, setBeating] = useState(false);
  const [recent, setRecent] = useState<AgentEvent[]>([]);
  const probed = useRef(false);

  async function reload() {
    try {
      const [p, m, a, s, j] = await Promise.all([
        getJSON<PulseStatus>("/api/pulse").catch(() => null),
        getJSON<{ records?: MemRecord[] }>("/api/memory").catch(() => null),
        getJSON<{ asks?: PulseAsk[] }>("/api/pulse/asks").catch(() => null),
        getJSON<{ orders?: StandingOrder[] }>("/api/standing").catch(() => null),
        getJSON<{ events?: AgentEvent[] }>("/api/journal", { kind: "initiative.act", limit: "6" }).catch(() => null),
      ]);
      if (p) setPulse(p);
      if (m) setRecords(m.records || []);
      if (a) setAsks(a.asks || []);
      if (s) setResponder((s.orders || []).find(isInitiativeResponder) || null);
      if (j) setRecent((j.events || []).slice().reverse());
    } catch {
      /* transient; keep last good */
    }
  }

  // Arm the initiative responder (M999/M1001): without it, act-mode emits and
  // approved asks promote to act, but nothing actually runs. One click here saves
  // a trip to the Standing view.
  async function armResponder() {
    if (!responder?.id) return;
    setArming(true);
    try {
      await postAction("/api/standing/enable", { id: responder.id, enabled: "true" });
      ui.toast("autonomous action armed — the responder will now act on initiative", "success");
      reload();
    } catch (e) {
      ui.toast((e as Error).message, "error");
    } finally {
      setArming(false);
    }
  }

  // Approve (re-emit onto the act path) or reject one pending ask (M1001).
  async function resolveAsk(ask: PulseAsk, approve: boolean) {
    setResolving(ask.issue_key);
    try {
      const r = await postAction<{ acted?: boolean }>("/api/pulse/asks/resolve", {
        issue_key: ask.issue_key,
        approve: approve ? "true" : "false",
      });
      ui.toast(
        approve
          ? r?.acted
            ? "approved — handed to the initiative responder"
            : "approved — enable the Initiative responder in Standing to act on it"
          : "dismissed",
        "success",
      );
      setAsks((prev) => prev.filter((a) => a.issue_key !== ask.issue_key));
      reload();
    } catch (e) {
      ui.toast((e as Error).message, "error");
    } finally {
      setResolving(null);
    }
  }

  // Trigger one on-demand heartbeat ("think now", M756) — poke Pulse to sweep its
  // observers right now; any actionable finding shows up as a new pending ask.
  async function thinkNow() {
    setBeating(true);
    try {
      await postAction("/api/pulse/beat", {});
      ui.toast("thinking now — checking for anything that needs you", "info");
      // The beat runs async on the daemon; give it a beat, then refresh.
      window.setTimeout(reload, 1000);
    } catch (e) {
      ui.toast((e as Error).message, "error");
    } finally {
      setBeating(false);
    }
  }

  // One-shot speech probe: both halves report 501 when unconfigured, so a tiny
  // request to each tells us whether a real provider is wired (natural voice /
  // server transcription) or we fall back to the browser's built-in speech.
  // /api/transcribe checks "configured?" before parsing, so an empty POST is a
  // safe, cheap probe (501 = no STT; anything else = a provider is wired).
  async function probeSpeech() {
    try {
      const res = await fetch("/api/tts", {
        method: "POST",
        headers: authHeaders({ "Content-Type": "application/json" }),
        body: JSON.stringify({ text: " " }),
      });
      setVoice(res.status === 501 ? "browser" : res.ok ? "natural" : "browser");
    } catch {
      setVoice("browser");
    }
    try {
      const res = await fetch("/api/transcribe", { method: "POST", headers: authHeaders() });
      setHear(res.status === 501 ? "browser" : "server");
    } catch {
      setHear("browser");
    }
  }

  useEffect(() => {
    reload();
    if (!probed.current) {
      probed.current = true;
      probeSpeech();
    }
    const id = setInterval(reload, 6000);
    return () => clearInterval(id);
  }, []);

  const profile = useMemo(
    () => (records || []).filter((r) => (r.subject || "").startsWith(PROFILE_PREFIX)),
    [records],
  );

  // --- derive each pillar's live state --------------------------------------
  const init = (pulse?.initiative || "").toLowerCase();
  const pulseRunning = !!pulse?.running && !pulse?.paused;
  const willLive = pulseRunning && init !== "off" && init !== "";
  const profileLive = profile.length > 0;
  // Voice can always speak (browser fallback), so the pillar is always "on";
  // "natural" just means the server TTS is wired for a richer voice.
  const voiceLive = voice !== "probing";
  const liveCount = (voiceLive ? 1 : 0) + (willLive ? 1 : 0) + (profileLive ? 1 : 0);

  async function rebuildProfile() {
    setRebuilding(true);
    try {
      const r = await postAction<{ facets_written?: number; input_records?: number }>(
        "/api/profile/rebuild",
        {},
      );
      const n = r?.facets_written ?? 0;
      ui.toast(
        n > 0
          ? `profile rebuilt: ${n} facet${n === 1 ? "" : "s"} learned`
          : "nothing to learn from yet — give it some memory first",
        n > 0 ? "success" : "info",
      );
      reload();
    } catch (e) {
      ui.toast((e as Error).message, "error");
    } finally {
      setRebuilding(false);
    }
  }

  return (
    <Page
      icon={Sparkles}
      title="Jarvis"
      description="Your AI's presence — it hears you, acts for you, and knows you."
      width="wide"
      actions={
        <Button variant="ghost" size="sm" onClick={reload} title="Refresh">
          <RefreshCw className="size-3.5" />
        </Button>
      }
    >
      {/* Presence hero — an aurora band with the live pillar count. */}
      <div className="glass glow-accent relative overflow-hidden rounded-lg p-5">
        <div
          aria-hidden
          className="pointer-events-none absolute -inset-x-10 -top-24 h-48 opacity-60 blur-3xl"
          style={{
            background:
              "radial-gradient(40% 80% at 20% 50%, var(--accent), transparent), radial-gradient(40% 80% at 70% 50%, var(--accent-2), transparent)",
          }}
        />
        <div className="relative flex flex-wrap items-end justify-between gap-3">
          <div>
            <div className="text-xs uppercase tracking-normal text-muted-foreground">Presence</div>
            <div className="mt-1 text-2xl font-semibold sm:text-3xl">
              <span className="text-gradient">{liveCount} of 3</span>{" "}
              <span className="text-foreground/80">pillars live</span>
            </div>
            <p className="mt-1 max-w-xl text-sm text-muted-foreground">
              {liveCount === 3
                ? "Fully present: listening, acting on its own, and learning who you are."
                : "Light up the rest below — each pillar turns a tool into a companion."}
            </p>
          </div>
          <div className="flex items-center gap-2 text-xs text-muted-foreground">
            <span className={cn("inline-flex items-center gap-1.5", voiceLive && "text-foreground")}>
              <Ear className="size-3.5" /> hear
            </span>
            <span className="opacity-40">·</span>
            <span className={cn("inline-flex items-center gap-1.5", willLive && "text-foreground")}>
              <Zap className="size-3.5" /> act
            </span>
            <span className="opacity-40">·</span>
            <span className={cn("inline-flex items-center gap-1.5", profileLive && "text-foreground")}>
              <UserRound className="size-3.5" /> know
            </span>
          </div>
        </div>
      </div>

      {/* The three pillars. */}
      <div className="grid gap-3 md:grid-cols-3">
        {/* VOICE — it hears you */}
        <Pillar
          icon={Mic}
          eyebrow="It hears you"
          live={voiceLive}
          title={
            voice === "probing"
              ? "Checking voice…"
              : voice === "natural"
                ? "Natural voice ready"
                : "Browser voice ready"
          }
          tone="converse"
        >
          <div className="space-y-1 text-sm">
            <div className="flex items-center gap-1.5 text-muted-foreground">
              <Ear className="size-3.5 shrink-0" /> Hearing:{" "}
              <span className="text-foreground">{hear === "probing" ? "…" : hear === "server" ? "a speech provider" : "your browser"}</span>
            </div>
            <div className="flex items-center gap-1.5 text-muted-foreground">
              <Volume2 className="size-3.5 shrink-0" /> Speaking:{" "}
              <span className="text-foreground">{voice === "probing" ? "…" : voice === "natural" ? "a natural voice" : "your browser"}</span>
            </div>
          </div>
          <p className="text-sm text-muted-foreground">
            {voice === "natural" && hear === "server"
              ? "Fully wired — it listens and replies in a natural voice, with barge-in."
              : "Hands-free conversation works through your browser. Pick a provider — OpenAI, ElevenLabs, Groq, Deepgram or a local server — on the Voice page for natural speech."}
          </p>
          <PillarCTA label={voice === "natural" && hear === "server" ? "Start talking" : "Set up voice"} onClick={() => go("voice")} />
        </Pillar>

        {/* INITIATIVE — it acts for you */}
        <Pillar
          icon={Zap}
          eyebrow="It acts for you"
          live={willLive}
          title={
            !pulse
              ? "…"
              : !pulseRunning
                ? "Heartbeat paused"
                : init === "act"
                  ? "Acting on its own"
                  : init === "ask"
                    ? "Asks before acting"
                    : "Observing only"
          }
          tone="monitor"
        >
          <div className="flex items-center gap-4 text-sm">
            <div className="flex items-center gap-1.5 text-muted-foreground" title="heartbeats since boot">
              <HeartPulse className={cn("size-4", pulseRunning && "text-[var(--good)]")} />
              <AnimatedNumber value={pulse?.beats || 0} className="font-semibold text-foreground" />
              <span>beats</span>
            </div>
            <div className="flex items-center gap-1.5 text-muted-foreground" title="active observers">
              <Activity className="size-4" />
              <span className="font-semibold text-foreground">{pulse?.observers?.length ?? 0}</span>
              <span>watching</span>
            </div>
          </div>
          <p className="text-sm text-muted-foreground">
            Dial <span className="text-foreground">{pulse?.dial || "—"}</span>
            {pulse?.cadence_ms ? (
              <> · every <span className="text-foreground">{Math.round(pulse.cadence_ms / 1000)}s</span></>
            ) : null}
            {init === "act" ? " · actionable signals become governed actions." : init === "ask" ? " · actionable signals ask you first." : ""}
          </p>

          {/* Responder arming (M999/M1001): the act/ask loop is inert until this is on. */}
          {responder &&
            (responder.enabled ? (
              <div className="flex items-center gap-1.5 text-xs text-[var(--good)]">
                <span className="inline-block size-1.5 rounded-full bg-[var(--good)]" />
                Responder armed — initiative becomes governed action
              </div>
            ) : (
              <div className="flex items-center justify-between gap-2 rounded-lg border border-amber-500/30 bg-amber-500/5 p-2 text-sm">
                <span className="min-w-0 text-muted-foreground">
                  Responder <span className="font-medium text-amber-500">disarmed</span> — nothing acts yet
                </span>
                <Button size="sm" variant="accent" onClick={armResponder} disabled={arming}>
                  <Power className={cn("size-3.5", arming && "animate-pulse")} /> Arm
                </Button>
              </div>
            ))}

          {/* Pending asks (M1001): the operator's verdict on what it wants to act on. */}
          {asks.length > 0 && (
            <div className="space-y-1.5 rounded-lg border border-[var(--accent)]/30 bg-[var(--accent)]/5 p-2">
              <div className="text-[11px] font-medium uppercase tracking-normal text-[var(--accent)]">
                {asks.length} waiting on you
              </div>
              {asks.slice(0, 3).map((a) => (
                <div key={a.issue_key} className="flex items-center gap-2 text-sm">
                  <span className="min-w-0 flex-1 truncate" title={a.reason || a.summary}>
                    {a.summary || a.source || a.issue_key}
                  </span>
                  <button
                    onClick={() => resolveAsk(a, true)}
                    disabled={resolving === a.issue_key}
                    title="Approve — hand to the act path"
                    className="grid size-6 place-items-center rounded-md text-[var(--good)] hover:bg-[var(--good)]/15 disabled:opacity-40"
                  >
                    <Check className="size-4" />
                  </button>
                  <button
                    onClick={() => resolveAsk(a, false)}
                    disabled={resolving === a.issue_key}
                    title="Dismiss"
                    className="grid size-6 place-items-center rounded-md text-muted-foreground hover:bg-muted/40 disabled:opacity-40"
                  >
                    <X className="size-4" />
                  </button>
                </div>
              ))}
            </div>
          )}

          <div className="flex items-center gap-3">
            <PillarCTA label="Tune autonomy" onClick={() => go("autonomy")} />
            <button
              onClick={thinkNow}
              disabled={beating}
              title="Run one heartbeat now"
              className="inline-flex items-center gap-1 text-sm text-muted-foreground hover:text-foreground disabled:opacity-40"
            >
              <Zap className={cn("size-3.5", beating && "animate-pulse")} /> Think now
            </button>
          </div>
        </Pillar>

        {/* PROFILE — it knows you */}
        <Pillar
          icon={UserRound}
          eyebrow="It knows you"
          live={profileLive}
          title={
            !records
              ? "…"
              : profileLive
                ? `Knows ${profile.length} thing${profile.length === 1 ? "" : "s"} about you`
                : "Still learning you"
          }
          tone="knowledge"
        >
          {profileLive ? (
            <ul className="space-y-1 text-sm">
              {profile.slice(0, 4).map((r) => (
                <li key={r.id} className="flex gap-1.5">
                  <span className="font-medium capitalize text-foreground">
                    {(r.subject || "").slice(PROFILE_PREFIX.length)}
                  </span>
                  <span className="truncate text-muted-foreground">— {r.content}</span>
                </li>
              ))}
            </ul>
          ) : (
            <p className="text-sm text-muted-foreground">
              As you work together, AGEZT distills who you are from memory and folds it into every run.
            </p>
          )}
          <div className="flex items-center gap-2">
            <PillarCTA label="Manage profile" onClick={() => go("memory")} />
            <Button variant="ghost" size="sm" onClick={rebuildProfile} disabled={rebuilding}>
              <RefreshCw className={cn("size-3.5", rebuilding && "animate-spin")} />
              Rebuild
            </Button>
          </div>
        </Pillar>
      </div>

      {/* Recent initiative — what Jarvis flagged/acted on lately (M1003), so armed
          autonomy isn't a black box. Hidden until there's something to show. */}
      {recent.length > 0 && (
        <div className="glass rounded-lg p-4">
          <div className="mb-2 flex items-center gap-2 text-[11px] font-medium uppercase tracking-normal text-muted-foreground">
            <Zap className="size-3.5" /> Recent initiative
          </div>
          <ul className="divide-y divide-border/60">
            {recent.map((e, i) => {
              const asked = (e.subject || "").endsWith(".ask");
              const p = e.payload || {};
              return (
                <li key={e.id || i} className="flex items-center gap-2 py-1.5 text-sm">
                  <span
                    className={cn(
                      "shrink-0 rounded px-1.5 py-0.5 text-[10px] font-semibold uppercase",
                      asked ? "bg-amber-500/15 text-amber-500" : "bg-[var(--good)]/15 text-[var(--good)]",
                    )}
                  >
                    {asked ? "asked" : "acted"}
                  </span>
                  <span className="min-w-0 flex-1 truncate" title={p.reason || p.summary}>
                    {p.summary || p.source || p.issue_key || "an observation"}
                  </span>
                  {p.source && <span className="shrink-0 text-xs text-muted-foreground">{p.source}</span>}
                  <span className="shrink-0 text-xs tabular-nums text-muted-foreground">{agoLabel(e.ts_unix_ms)}</span>
                </li>
              );
            })}
          </ul>
        </div>
      )}
    </Page>
  );
}

// Pillar — one glass card. Glows and shows a live dot when its pillar is active.
function Pillar({
  icon: Icon,
  eyebrow,
  title,
  live,
  tone,
  children,
}: {
  icon: typeof Mic;
  eyebrow: string;
  title: string;
  live: boolean;
  tone: "converse" | "monitor" | "knowledge";
  children: React.ReactNode;
}) {
  const hue = tone === "converse" ? 255 : tone === "monitor" ? 150 : 195;
  return (
    <div
      className={cn(
        "glass flex flex-col gap-3 rounded-lg p-4 transition-shadow",
        live && "glow-accent",
      )}
    >
      <div className="flex items-center gap-3">
        <div
          className="grid size-10 place-items-center rounded-xl"
          style={{
            background: `linear-gradient(135deg, oklch(0.7 0.14 ${hue} / 0.25), oklch(0.7 0.14 ${hue} / 0.05))`,
            boxShadow: live ? `0 0 18px -4px oklch(0.7 0.18 ${hue} / 0.6)` : undefined,
          }}
        >
          <Icon className="size-5" style={{ color: `oklch(0.72 0.16 ${hue})` }} />
        </div>
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-1.5 text-[11px] uppercase tracking-normal text-muted-foreground">
            {eyebrow}
            <span
              className={cn(
                "inline-block size-1.5 rounded-full",
                live ? "bg-[var(--good)] animate-pulse" : "bg-muted-foreground/40",
              )}
              title={live ? "live" : "dormant"}
            />
          </div>
          <div className="truncate font-semibold">{title}</div>
        </div>
      </div>
      {children}
    </div>
  );
}

function PillarCTA({ label, onClick }: { label: string; onClick: () => void }) {
  return (
    <button
      onClick={onClick}
      className="inline-flex w-fit items-center gap-1 text-sm font-medium text-[var(--accent)] hover:underline"
    >
      {label} <ArrowRight className="size-3.5" />
    </button>
  );
}
