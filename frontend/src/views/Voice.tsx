import { useEffect, useMemo, useRef, useState } from "react";
import { Mic, Square, Radio, Loader2, Volume2, Ear, Sparkles } from "lucide-react";
import { PageHeader } from "@/components/ui/page-header";
import { Card } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { useUI } from "@/components/ui/feedback";
import { getJSON } from "@/lib/api";
import { VoiceSession, createBrowserVoiceIO, type VoiceState } from "@/lib/voiceSession";
import { VoiceSetup } from "@/views/VoiceSetup";

const WAKE_KEY = "agezt.voice.wake";
const AGENT_KEY = "agezt.voice.agent";
const WAKE_WORDS = ["agezt", "jarvis"];

type Line = { role: "you" | "agezt"; text: string };

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
          on ? (
            <Button variant="danger" onClick={stopSession}>
              <Square className="size-4" /> Stop
            </Button>
          ) : (
            <Button variant="accent" onClick={startSession}>
              <Mic className="size-4" /> Start talking
            </Button>
          )
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
            <label className="flex items-center justify-between gap-3">
              <span className="text-muted">Agent</span>
              <select
                value={agent}
                onChange={(e) => setAgent(e.target.value)}
                disabled={on}
                className="min-w-0 flex-1 rounded-md border border-border bg-surface px-2 py-1 text-right text-sm disabled:opacity-50"
                style={{ maxWidth: "60%" }}
              >
                <option value="">Default routing</option>
                {agents.map((a) => (
                  <option key={a.slug} value={a.slug}>
                    {a.name || a.slug}
                  </option>
                ))}
              </select>
            </label>
          </div>
        </Card>

        {/* Transcript */}
        <Card glass className="flex max-h-[28rem] min-h-[16rem] flex-col overflow-hidden">
          <div className="border-b border-border px-4 py-3 text-xs font-semibold uppercase tracking-wide text-muted">Conversation</div>
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
                    className={`inline-block max-w-[85%] rounded-2xl px-3 py-2 text-sm ${
                      l.role === "you" ? "bg-accent/15 text-fg" : "bg-surface ring-1 ring-inset ring-border"
                    }`}
                  >
                    <div className="mb-0.5 text-[10px] uppercase tracking-wide text-muted">{l.role === "you" ? "You" : "AGEZT"}</div>
                    {l.text}
                  </div>
                </div>
              ))
            )}
          </div>
        </Card>
      </div>

      {/* Inline setup — wire up speech servers here without leaving the cockpit. */}
      <VoiceSetup />
    </div>
  );
}
