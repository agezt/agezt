// Jarvis.tsx — a presence / companion surface. When the operator lands here
// they're asking one question: "what's my agent doing right now, and can I
// nudge it?". Day 25 brought this back after Day 23 deleted @/views/Jarvis.
//
// Jarvis is NOT a chat surface (Talk › Chat owns that). It answers four
// narrower questions:
//   1. who's online?            — roster of agents with their readiness chip
//   2. what's running?          — the active run + a stream of recent runs
//   3. what's the agent heard? — the last few memory writes (M920: heartbeat)
//   4. what can I say quickly?  — a row of "ask / steer / stand-by" shortcuts

import { useEffect, useState } from "react";
import { Activity, ArrowRight, Brain, Check, Ear, History, MessageCircle, Mic, Sparkles } from "lucide-react";
import { getJSON } from "@/app/api";
import { useChat } from "@/lib/chatStore";
import { Button } from "@/components/ui/button";
import { cn } from "@/app/utils";
import { goToView } from "@/lib/nav";

// AgentRow is one entry in the "who's online" list. The daemon exposes a roster
// at /api/agents; we render the readiness + last-seen for each.
interface AgentRow {
  slug: string;
  name?: string;
  ready?: boolean;
  lastSeenMs?: number;
  status?: string;
}

interface ServerVoiceStatus {
  stt?: { configured?: boolean; provider?: string };
  tts?: { configured?: boolean; provider?: string };
}

function useAgents(): AgentRow[] {
  const [rows, setRows] = useState<AgentRow[]>([]);
  useEffect(() => {
    let stop = false;
    async function load() {
      try {
        const r = await getJSON<{ agents?: AgentRow[] }>("/api/agents", {});
        if (!stop) setRows(r.agents || []);
      } catch {
        // daemon offline / first run — leave the list empty rather than noisy-error
      }
    }
    void load();
    const t = setInterval(load, 30_000);
    return () => {
      stop = true;
      clearInterval(t);
    };
  }, []);
  return rows;
}

function useRecentRuns() {
  const [runs, setRuns] = useState<{ id: string; status: string; intent?: string; correlation_id?: string; ts?: number }[]>([]);
  useEffect(() => {
    let stop = false;
    async function load() {
      try {
        const r = await getJSON<{ runs?: { id: string; status: string; intent?: string; correlation_id?: string; ts?: number }[] }>(
          "/api/runs",
          { limit: "5" },
        );
        if (!stop) setRuns(r.runs || []);
      } catch {
        /* same — empty is fine */
      }
    }
    void load();
    const t = setInterval(load, 10_000);
    return () => {
      stop = true;
      clearInterval(t);
    };
  }, []);
  return runs;
}

function useVoiceStatus(): ServerVoiceStatus | null | undefined {
  // undefined = still loading, null = no daemon response, object = the daemon's
  // honest report. We render the "Voice needs setup" card the moment the
  // status arrives with both STT and TTS unconfigured — that is the truthful
  // signal the e2e/webui.spec.ts regression guard pins down.
  const [status, setStatus] = useState<ServerVoiceStatus | null | undefined>(undefined);
  useEffect(() => {
    let stop = false;
    async function load() {
      try {
        const r = await getJSON<ServerVoiceStatus>("/api/voice/status");
        if (!stop) setStatus(r || {});
      } catch {
        if (!stop) setStatus(null);
      }
    }
    void load();
    return () => {
      stop = true;
    };
  }, []);
  return status;
}

export default function Jarvis() {
  const agents = useAgents();
  const runs = useRecentRuns();
  const voice = useVoiceStatus();
  const chat = useChat();

  const onlineCount = agents.filter((a) => a.ready).length;
  const runningCount = runs.filter((r) => r.status === "running").length;
  const sttReady = !!voice?.stt?.configured;
  const ttsReady = !!voice?.tts?.configured;
  const voiceReady = sttReady && ttsReady;

  return (
    <div className="mx-auto grid w-full max-w-6xl gap-4 p-4 lg:grid-cols-[minmax(0,1fr)_18rem]">
      <main className="space-y-4">
        {/* Status strip */}
        <section className="glass rounded-2xl p-4">
          <header className="flex flex-wrap items-baseline gap-3">
            <h2 className="flex items-center gap-2 text-lg font-semibold text-foreground">
              <Sparkles className="size-5 text-accent" />
              <span>Jarvis</span>
            </h2>
            <p className="text-xs text-muted">your operator-side companion — what's online, what's running, what to nudge</p>
          </header>
          <div className="mt-3 grid gap-3 sm:grid-cols-3">
            <Stat icon={Ear} label="Agents ready" value={onlineCount} hint={`of ${agents.length} roster`} />
            <Stat icon={Activity} label="Runs in flight" value={runningCount} hint="right now" />
            <Stat icon={Brain} label="Recent memories" value={"…"} hint="(stub)" />
          </div>
        </section>

        {!voiceReady && voice !== undefined && (
          <section
            data-testid="voice-needs-setup"
            className="glass rounded-2xl border border-warning/30 p-4"
          >
            <header className="flex items-center gap-2">
              <Mic className="size-4 text-warning" />
              <h3 className="text-sm font-semibold text-foreground">Voice needs setup</h3>
            </header>
            <p className="mt-1 text-xs text-muted">
              {voice === null
                ? "Voice status endpoint unreachable — provider not configured."
                : "Voice status endpoint reachable — provider not configured."}
            </p>
            <ul className="mt-2 space-y-1 text-xs text-muted">
              <li>
                Hearing (STT):{" "}
                <span className={cn("font-medium", sttReady ? "text-good" : "text-warning")}>
                  {sttReady ? "ready" : "provider not configured"}
                </span>
                {voice?.stt?.provider && (
                  <span className="ml-1 text-muted">({voice.stt.provider})</span>
                )}
              </li>
              <li>
                Speaking (TTS):{" "}
                <span className={cn("font-medium", ttsReady ? "text-good" : "text-warning")}>
                  {ttsReady ? "ready" : "provider not configured"}
                </span>
                {voice?.tts?.provider && (
                  <span className="ml-1 text-muted">({voice.tts.provider})</span>
                )}
              </li>
            </ul>
            <Button
              className="mt-3"
              variant="ghost"
              onClick={() => goToView("voice")}
            >
              Open Talk › Voice
            </Button>
          </section>
        )}

        {/* Quick prompts — the operator wants to nudge, not compose */}
        <section className="glass rounded-2xl p-4">
          <h3 className="text-xs font-semibold uppercase tracking-wider text-muted">Quick prompts</h3>
          <p className="mt-0.5 text-[11px] text-muted/80">One click → agent gets the same intent. Steer / note arrive on the running run; "ask" starts a new one.</p>
          <div className="mt-3 grid gap-2 sm:grid-cols-2">
            <QuickPrompt label="Summarize what changed in the last 6 hours" icon={History} />
            <QuickPrompt label="Diagnose the longest-running run" icon={Activity} />
            <QuickPrompt label="What's blocking the active runs?" icon={Sparkles} />
            <QuickPrompt label="Draft a stand-up note from the day's events" icon={MessageCircle} />
          </div>
        </section>

        {/* Recent runs — last 5 in-flight / recent decisions */}
        <section className="glass rounded-2xl p-4">
          <h3 className="text-xs font-semibold uppercase tracking-wider text-muted">Recent runs</h3>
          {runs.length === 0 ? (
            <p className="mt-2 text-sm text-muted">No recent runs. Start one with a quick prompt above, or hit Chat.</p>
          ) : (
            <ul className="mt-3 divide-y divide-border/40">
              {runs.map((r) => (
                <li key={r.id} className="flex items-center gap-2 py-1.5 text-sm">
                  <span className={cn("inline-flex h-1.5 w-1.5 shrink-0 rounded-full", r.status === "running" ? "bg-accent" : r.status === "completed" ? "bg-good" : r.status === "failed" ? "bg-bad" : "bg-muted")} />
                  <span className="min-w-0 flex-1 truncate text-foreground/90">{r.intent || "(no intent)"}</span>
                  <span className="font-mono text-[10px] text-muted">{r.correlation_id?.slice(0, 8)}</span>
                  <span className="text-[11px] uppercase tracking-wide text-muted">{r.status}</span>
                </li>
              ))}
            </ul>
          )}
        </section>
      </main>

      {/* Right rail — agents online */}
      <aside className="space-y-3">
        <section className="glass rounded-2xl p-4">
          <h3 className="flex items-center justify-between text-xs font-semibold uppercase tracking-wider text-muted">
            <span>Agents</span>
            <span>{onlineCount}/{agents.length}</span>
          </h3>
          {agents.length === 0 ? (
            <p className="mt-2 text-sm text-muted">No roster yet. Start one from Roster.</p>
          ) : (
            <ul className="mt-3 space-y-1.5">
              {agents.slice(0, 12).map((a) => (
                <li key={a.slug} className="flex items-center gap-2 rounded-md px-2 py-1 text-sm hover:bg-panel/60">
                  <span className={cn("inline-flex h-1.5 w-1.5 rounded-full", a.ready ? "bg-good" : "bg-muted")} />
                  <span className="min-w-0 flex-1 truncate font-medium text-foreground/90">{a.name || a.slug}</span>
                  <span className="text-[10px] uppercase tracking-wide text-muted">{a.ready ? "ready" : (a.status || "offline")}</span>
                </li>
              ))}
            </ul>
          )}
        </section>

        <section className="glass rounded-2xl p-4">
          <h3 className="text-xs font-semibold uppercase tracking-wider text-muted">Open chat</h3>
          <p className="mt-1 text-[11px] text-muted">Need to ask something longer? Chat has the full composer.</p>
          <Button
            className="mt-3 w-full"
            variant="accent"
            onClick={() => {
              // Use the same engine — send is a no-op if the chat has an active thread.
              chat.newChat();
              goToView("chat");
            }}
            aria-label="Open chat"
          >
            <MessageCircle className="size-4" />
            Open chat
            <ArrowRight className="size-3.5" />
          </Button>
        </section>
      </aside>
    </div>
  );
}

function Stat({ icon: Icon, label, value, hint }: { icon: typeof Activity; label: string; value: number | string; hint?: string }) {
  return (
    <div className="rounded-xl border border-border/60 bg-card/30 p-3">
      <div className="flex items-center gap-2 text-[11px] uppercase tracking-wider text-muted">
        <Icon className="size-3.5" />
        <span>{label}</span>
      </div>
      <div className="mt-1 text-2xl font-semibold tabular-nums text-foreground">{value}</div>
      {hint && <div className="text-[10px] text-muted">{hint}</div>}
    </div>
  );
}

function QuickPrompt({ label, icon: Icon }: { label: string; icon: typeof Activity }) {
  const chat = useChat();
  return (
    <button
      onClick={() => {
        chat.send(label);
        goToView("chat");
      }}
      className="flex items-center gap-2 rounded-lg border border-border bg-card/30 px-3 py-2 text-left text-sm text-foreground/90 transition-colors hover:bg-accent/10 hover:border-accent/40"
    >
      <Icon className="size-3.5 shrink-0 text-accent" />
      <span className="flex-1 truncate">{label}</span>
      <Check className="size-3 text-muted" />
    </button>
  );
}
