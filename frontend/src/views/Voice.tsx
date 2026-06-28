import { useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { Mic, Square, Radio, Loader2, Volume2, Ear, Sparkles, Settings2, X, type LucideIcon } from "lucide-react";
import { PageHeader } from "@/components/ui/page-header";
import { Card } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { useUI } from "@/components/ui/feedback";
import { getJSON } from "@/lib/api";
import { cn } from "@/lib/utils";
import { VoiceSession, createBrowserVoiceIO, type VoiceState } from "@/lib/voiceSession";
import { VoiceSetup } from "@/views/VoiceSetup";

const WAKE_KEY = "agezt.voice.wake";
const AGENT_KEY = "agezt.voice.agent";
const WAKE_WORDS = ["agezt", "jarvis"];

type Line = { role: "you" | "agezt"; text: string };

function VoiceModal({
  title,
  icon: Icon,
  onClose,
  children,
}: {
  title: string;
  icon: LucideIcon;
  onClose: () => void;
  children: ReactNode;
}) {
  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/55 p-4 backdrop-blur-sm">
      <div className="w-full max-w-5xl rounded-xl border border-border bg-panel shadow-2xl">
        <div className="flex items-center gap-2 border-b border-border px-4 py-3">
          <Icon className="size-4 text-accent" />
          <h2 className="text-sm font-semibold text-foreground">{title}</h2>
          <Button className="ml-auto" size="icon" variant="ghost" onClick={onClose} aria-label={`Close ${title}`}>
            <X className="size-4" />
          </Button>
        </div>
        <div className="max-h-[78vh] overflow-y-auto p-4">{children}</div>
      </div>
    </div>
  );
}

// stateLabel + stateHue describe the orb for each phase of the loop.
const STATE_META: Record<VoiceState, { label: string; hue: string; pulse: boolean }> = {
  idle: { label: "Idle", hue: "var(--muted)", pulse: false },
  waking: { label: `Listening for "${WAKE_WORDS[0]}"…`, hue: "#f59e0b", pulse: true },
  listening: { label: "Listening…", hue: "var(--accent)", pulse: true },
  thinking: { label: "Thinking…", hue: "var(--accent2)", pulse: true },
  speaking: { label: "Speaking…", hue: "#22d3ee", pulse: true },
};

// Voice is the hands-free conversational mode — the "talk to Jarvis" surface. You
// start it, speak, and it listens (VAD), runs the agent, and speaks the answer
// back sentence-by-sentence, stopping the moment you talk over it (barge-in). The
// whole loop lives in lib/voiceSession; this view is its cockpit: an animated orb
// that reflects the current phase + mic level, the running transcript, and the
// controls (wake word, agent, start/stop).
export function Voice() {
  const ui = useUI();
  const [on, setOn] = useState(false);
  const [state, setState] = useState<VoiceState>("idle");
  const [level, setLevel] = useState(0);
  const [lines, setLines] = useState<Line[]>([]);
  const [wake, setWake] = useState<boolean>(() => localStorage.getItem(WAKE_KEY) === "1");
  const [agents, setAgents] = useState<{ slug: string; name?: string }[]>([]);
  const [agent, setAgent] = useState<string>(() => localStorage.getItem(AGENT_KEY) || "");
  const [setupOpen, setSetupOpen] = useState(false);
  const sessionRef = useRef<VoiceSession | null>(null);
  const scrollRef = useRef<HTMLDivElement | null>(null);

  // Load the roster for the optional agent picker (degrade silently — Voice works
  // with the daemon's default routing when none is chosen).
  useEffect(() => {
    getJSON<{ profiles?: { slug: string; name?: string; enabled?: boolean }[] }>("/api/agents")
      .then((r) => setAgents((r.profiles ?? []).filter((p) => p.enabled !== false).map((p) => ({ slug: p.slug, name: p.name }))))
      .catch(() => {});
  }, []);

  useEffect(() => localStorage.setItem(WAKE_KEY, wake ? "1" : "0"), [wake]);
  useEffect(() => localStorage.setItem(AGENT_KEY, agent), [agent]);

  // Auto-scroll the transcript as it grows.
  useEffect(() => {
    const el = scrollRef.current;
    el?.scrollTo?.({ top: el.scrollHeight, behavior: "smooth" });
  }, [lines]);

  // Tear the session down on unmount so the mic is released.
  useEffect(() => () => sessionRef.current?.stop(), []);

  function startSession() {
    const io = createBrowserVoiceIO({ agent: agent || undefined });
    const session = new VoiceSession(
      io,
      {
        onState: setState,
        onLevel: setLevel,
        onUserText: (text) => setLines((ls) => [...ls, { role: "you", text }]),
        onAnswerDelta: (text) =>
          setLines((ls) => {
            const last = ls[ls.length - 1];
            if (last && last.role === "agezt") return [...ls.slice(0, -1), { ...last, text: last.text + text }];
            return [...ls, { role: "agezt", text }];
          }),
        onError: (msg) => ui.toast(msg, "error"),
      },
      { wakeWords: wake ? WAKE_WORDS : [], agent: agent || undefined },
    );
    sessionRef.current = session;
    session.start();
    setOn(true);
  }

  function stopSession() {
    sessionRef.current?.stop();
    sessionRef.current = null;
    setOn(false);
    setState("idle");
    setLevel(0);
  }

  const meta = STATE_META[state];
  // The orb breathes with the mic level while active; a gentle idle size otherwise.
  const scale = on ? 1 + Math.min(level * 2.5, 0.6) : 1;

  const StateIcon = useMemo(() => {
    switch (state) {
      case "waking":
        return Ear;
      case "listening":
        return Mic;
      case "thinking":
        return Loader2;
      case "speaking":
        return Volume2;
      default:
        return Radio;
    }
  }, [state]);

  return (
    <div className="space-y-5">
      <PageHeader
        icon={Radio}
        title="Voice"
        description="Hands-free conversation — speak, and AGEZT listens, thinks, and talks back. Interrupt any time."
        actions={
          <>
            <Button variant="ghost" onClick={() => setSetupOpen(true)}>
              <Settings2 className="size-4" /> Setup
            </Button>
            {on ? (
              <Button variant="danger" onClick={stopSession}>
                <Square className="size-4" /> Stop
              </Button>
            ) : (
              <Button variant="accent" onClick={startSession}>
                <Mic className="size-4" /> Start talking
              </Button>
            )}
          </>
        }
      />

      <div className="grid gap-5 lg:grid-cols-[minmax(0,1fr)_minmax(0,1.4fr)]">
        {/* Orb + controls */}
        <Card glass className="flex flex-col items-center gap-6 p-8">
          <div className="grid h-56 w-full place-items-center">
            <div
              className="grid place-items-center rounded-full transition-transform duration-100 ease-out"
              style={{
                width: 160,
                height: 160,
                transform: `scale(${scale})`,
                background: `radial-gradient(circle at 50% 40%, ${meta.hue}, transparent 70%)`,
                boxShadow: on ? `0 0 60px -10px ${meta.hue}` : "none",
              }}
            >
              <span
                className={`grid size-20 place-items-center rounded-full bg-surface/70 ring-1 ring-inset ring-border ${
                  meta.pulse ? "animate-pulse" : ""
                }`}
                style={{ color: meta.hue }}
              >
                <StateIcon className={`size-8 ${state === "thinking" ? "animate-spin" : ""}`} />
              </span>
            </div>
          </div>
          <div className="text-center">
            <div className="text-sm font-semibold" style={{ color: on ? meta.hue : undefined }}>
              {on ? meta.label : "Tap start, then just talk"}
            </div>
            <div className="mt-1 text-xs text-muted">
              {wake ? `Say "${WAKE_WORDS[0]}" to begin a turn · interrupt to barge in` : "Speak after Start · interrupt to barge in"}
            </div>
          </div>

          <div className="flex w-full flex-col gap-3 border-t border-border pt-4 text-sm">
            <label className="flex items-center justify-between gap-3">
              <span className="flex items-center gap-2 text-muted">
                <Sparkles className="size-4" /> Wake word
              </span>
              <button
                type="button"
                role="switch"
                aria-checked={wake}
                onClick={() => setWake((w) => !w)}
                className={`relative h-5 w-9 rounded-full transition-colors ${wake ? "bg-accent" : "bg-border"}`}
              >
                <span className={`absolute top-0.5 size-4 rounded-full bg-white transition-all ${wake ? "left-[18px]" : "left-0.5"}`} />
              </button>
            </label>
            <div className="flex flex-col gap-1.5">
              <span className="text-muted">Agent</span>
              <div className="flex flex-wrap gap-1.5" role="group" aria-label="Voice agent">
                {[{ slug: "", name: "Default routing" }, ...agents].map((a) => {
                  const selected = agent === a.slug;
                  return (
                    <button
                      key={a.slug || "default"}
                      type="button"
                      aria-pressed={selected}
                      disabled={on}
                      onClick={() => setAgent(a.slug)}
                      className={cn(
                        "inline-flex h-7 items-center rounded-md border px-2 text-xs font-medium transition-colors disabled:opacity-50",
                        selected
                          ? "border-accent bg-accent/15 text-accent"
                          : "border-border bg-surface text-muted hover:border-accent/60 hover:text-foreground",
                      )}
                    >
                      {a.name || a.slug}
                    </button>
                  );
                })}
              </div>
            </div>
          </div>
        </Card>

        {/* Transcript */}
        <Card glass className="flex max-h-[28rem] min-h-[16rem] flex-col overflow-hidden">
          <div className="border-b border-border px-4 py-3 text-xs font-semibold uppercase tracking-normal text-muted">Conversation</div>
          <div ref={scrollRef} className="flex-1 space-y-3 overflow-y-auto p-4">
            {lines.length === 0 ? (
              <div className="grid h-full place-items-center text-center text-sm text-muted">
                <div>
                  <Mic className="mx-auto mb-2 size-6 opacity-50" />
                  Your conversation will appear here.
                </div>
              </div>
            ) : (
              lines.map((l, i) => (
                <div key={i} className={l.role === "you" ? "text-right" : "text-left"}>
                  <div
                    className={`inline-block max-w-[85%] rounded-xl px-3 py-2 text-sm ${
                      l.role === "you" ? "bg-accent/15 text-fg" : "bg-surface ring-1 ring-inset ring-border"
                    }`}
                  >
                    <div className="mb-0.5 text-[10px] uppercase tracking-normal text-muted">{l.role === "you" ? "You" : "AGEZT"}</div>
                    {l.text}
                  </div>
                </div>
              ))
            )}
          </div>
        </Card>
      </div>

      {setupOpen && (
        <VoiceModal title="Voice setup" icon={Settings2} onClose={() => setSetupOpen(false)}>
          <VoiceSetup />
        </VoiceModal>
      )}
    </div>
  );
}
