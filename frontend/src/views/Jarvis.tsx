import { useEffect, useMemo, useRef, useState } from "react";
import {
  Sparkles, Mic, Zap, UserRound, RefreshCw, ArrowRight, Activity, Ear, HeartPulse, Check, X,
} from "lucide-react";
import { getJSON, postAction, authHeaders } from "@/lib/api";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { PageHeader } from "@/components/ui/page-header";
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

type VoiceState = "probing" | "natural" | "browser";

// navigate to another view by hash (the app routes on location.hash).
function go(id: string) {
  window.location.hash = id;
}

export function Jarvis() {
  const ui = useUI();
  const [pulse, setPulse] = useState<PulseStatus | null>(null);
  const [records, setRecords] = useState<MemRecord[] | null>(null);
  const [voice, setVoice] = useState<VoiceState>("probing");
  const [asks, setAsks] = useState<PulseAsk[]>([]);
  const [rebuilding, setRebuilding] = useState(false);
  const [resolving, setResolving] = useState<string | null>(null);
  const probed = useRef(false);

  async function reload() {
    try {
      const [p, m, a] = await Promise.all([
        getJSON<PulseStatus>("/api/pulse").catch(() => null),
        getJSON<{ records?: MemRecord[] }>("/api/memory").catch(() => null),
        getJSON<{ asks?: PulseAsk[] }>("/api/pulse/asks").catch(() => null),
      ]);
      if (p) setPulse(p);
      if (m) setRecords(m.records || []);
      if (a) setAsks(a.asks || []);
    } catch {
      /* transient; keep last good */
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

  // One-shot voice probe: ask the server TTS for a single space. 501 = not
  // configured (we fall back to the browser voice, which is always available).
  async function probeVoice() {
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
  }

  useEffect(() => {
    reload();
    if (!probed.current) {
      probed.current = true;
      probeVoice();
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
    <div className="flex h-full min-h-0 flex-col gap-3 overflow-y-auto">
      <PageHeader
        icon={Sparkles}
        title="Jarvis"
        description="Your AI's presence — it hears you, acts for you, and knows you."
        actions={
          <Button variant="ghost" size="sm" onClick={reload} title="Refresh">
            <RefreshCw className="size-3.5" />
          </Button>
        }
      />

      {/* Presence hero — an aurora band with the live pillar count. */}
      <div className="glass glow-accent relative overflow-hidden rounded-2xl p-5">
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
            <div className="text-xs uppercase tracking-wider text-muted-foreground">Presence</div>
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
          <p className="text-sm text-muted-foreground">
            {voice === "natural"
              ? "Server text-to-speech is wired — replies stream back in a natural voice with barge-in."
              : "Hands-free conversation works through your browser's built-in voice. Set AGEZT_TTS_* for a richer one."}
          </p>
          <PillarCTA label="Start talking" onClick={() => go("voice")} />
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

          {/* Pending asks (M1001): the operator's verdict on what it wants to act on. */}
          {asks.length > 0 && (
            <div className="space-y-1.5 rounded-lg border border-[var(--accent)]/30 bg-[var(--accent)]/5 p-2">
              <div className="text-[11px] font-medium uppercase tracking-wider text-[var(--accent)]">
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

          <PillarCTA label="Tune autonomy" onClick={() => go("autonomy")} />
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
    </div>
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
        "glass flex flex-col gap-3 rounded-2xl p-4 transition-shadow",
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
          <div className="flex items-center gap-1.5 text-[11px] uppercase tracking-wider text-muted-foreground">
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
