import { useEffect, useRef, useState } from "react";
import {
  Send,
  Square,
  Wrench,
  ShieldCheck,
  ShieldX,
  ChevronRight,
  ChevronDown,
  Loader2,
  User,
  Sparkles,
  AlertTriangle,
  Copy,
  Check,
  Brain,
  Plus,
  Trash2,
  RotateCcw,
  ArrowDown,
  X,
  Paperclip,
  Volume2,
  VolumeX,
  CornerDownRight,
} from "lucide-react";
import { cn } from "@/lib/utils";
import { money } from "@/lib/format";
import { Button } from "@/components/ui/button";
import { useUI } from "@/components/ui/feedback";
import { Badge } from "@/components/ui/badge";
import { Markdown } from "@/components/Markdown";
import { ToolOutput } from "@/components/DataView";
import { ModelPicker } from "@/components/ModelPicker";
import { AttachPicker } from "@/components/AttachPicker";
import { MicButton } from "@/components/MicButton";
import { speak, stopSpeaking, speechSupported } from "@/lib/speech";

const AUTOSPEAK_KEY = "agezt.chat.autospeak";
import { turnText, type ChatTurn, type ChatTool, type TimelineItem } from "@/lib/chat";
import { useChat } from "@/lib/chatStore";
import { buildContext, type AttachRef } from "@/lib/attach";
import type { Msg } from "@/lib/conversations";

// Chat is the humane front door to the agent: a conversational thread where you
// type an intent and watch the governed loop answer live — streaming text, the
// tool calls it made (with the policy verdict), and the final answer with its
// cost. The engine (store, streaming, model) lives in ChatProvider so a run keeps
// going when you leave the view; this component is the full-screen UI over it.
export function Chat() {
  const { store, messages, busy, model, setModel, activeModel, send, retry, stop, newChat, selectConversation, removeConversation } =
    useChat();
  const [input, setInput] = useState("");
  // pinned = the thread is stuck to the bottom (auto-scrolls on new content).
  // It flips to false when you scroll up to read scrollback, so a live stream
  // never yanks you back down mid-read; a "jump to latest" button restores it.
  const [pinned, setPinned] = useState(true);
  // Attachments staged for the next message: existing skills/memories/runs to
  // hand the agent as context. Cleared once the message is sent.
  const [attached, setAttached] = useState<AttachRef[]>([]);
  const [attachOpen, setAttachOpen] = useState(false);
  // autoSpeak: read each completed answer aloud (browser TTS). Persisted, off by
  // default so the UI is silent unless the user opts in.
  const [autoSpeak, setAutoSpeak] = useState(() => {
    try {
      return localStorage.getItem(AUTOSPEAK_KEY) === "1";
    } catch {
      return false;
    }
  });
  const scrollRef = useRef<HTMLDivElement>(null);
  const taRef = useRef<HTMLTextAreaElement>(null);
  const prevBusy = useRef(busy);
  const lastSpokeRef = useRef("");

  function toggleAutoSpeak() {
    setAutoSpeak((v) => {
      const next = !v;
      try {
        localStorage.setItem(AUTOSPEAK_KEY, next ? "1" : "0");
      } catch {
        /* ignore storage errors */
      }
      if (!next) stopSpeaking();
      return next;
    });
  }

  // Auto-speak the latest answer when a run finishes (busy true→false). Keyed on
  // the busy transition — not on `messages` content — so navigating, reloading,
  // or switching conversations never reads an old answer aloud, and each answer
  // is spoken at most once. Stop any speech when the component unmounts.
  useEffect(() => {
    if (prevBusy.current && !busy && autoSpeak) {
      const last = messages[messages.length - 1];
      if (last?.role === "assistant" && last.turn.status === "done") {
        // Speak each answer at most once: key on the run id (or the text, before
        // a correlation lands) so a follow-up state update can't read it twice.
        const key = last.turn.correlationId || turnText(last.turn);
        if (key && key !== lastSpokeRef.current) {
          lastSpokeRef.current = key;
          speak(turnText(last.turn));
        }
      }
    }
    prevBusy.current = busy;
  }, [busy, autoSpeak, messages]);
  useEffect(() => () => stopSpeaking(), []);

  // Pin the thread to the bottom as content streams in — but only while pinned,
  // so scrolling up to read history during a live stream isn't disrupted.
  useEffect(() => {
    const el = scrollRef.current;
    if (el && pinned) el.scrollTop = el.scrollHeight;
  }, [messages, pinned]);

  // Track whether the user is near the bottom; that's what keeps the thread
  // pinned. A ~80px slack means "close enough" counts as pinned.
  function onScroll() {
    const el = scrollRef.current;
    if (!el) return;
    setPinned(el.scrollHeight - el.scrollTop - el.clientHeight < 80);
  }
  function jumpToLatest() {
    const el = scrollRef.current;
    if (el) el.scrollTop = el.scrollHeight;
    setPinned(true);
  }

  // Grow the composer with its content (up to max-h-40 = 160px), then scroll.
  useEffect(() => {
    const el = taRef.current;
    if (!el) return;
    el.style.height = "auto";
    el.style.height = Math.min(el.scrollHeight, 160) + "px";
  }, [input]);

  // Submit the composer: pin to bottom, clear the box, hand the intent to the
  // engine with any attached context (skills/memories/runs) prepended.
  function doSend() {
    const t = input.trim();
    if (!t || busy) return;
    stopSpeaking(); // a new turn interrupts any answer being read aloud
    setPinned(true);
    setInput("");
    const ctx = buildContext(attached);
    setAttached([]);
    send(t, ctx);
  }

  function addAttachment(ref: AttachRef) {
    setAttached((prev) => (prev.some((r) => r.id === ref.id) ? prev : [...prev, ref]));
  }
  function removeAttachment(id: string) {
    setAttached((prev) => prev.filter((r) => r.id !== id));
  }

  function startNewChat() {
    newChat();
    setInput("");
  }

  function onKeyDown(e: React.KeyboardEvent<HTMLTextAreaElement>) {
    // Enter sends; Shift+Enter inserts a newline (ChatGPT/Claude convention).
    if (e.key === "Enter" && !e.shiftKey) {
      e.preventDefault();
      doSend();
    }
  }

  return (
    <div className="flex h-full gap-3">
      {attachOpen && (
        <AttachPicker
          selectedIds={new Set(attached.map((r) => r.id))}
          onPick={addAttachment}
          onClose={() => setAttachOpen(false)}
        />
      )}
      {/* Conversation list — past threads, ChatGPT-style (desktop). */}
      <aside className="hidden w-52 shrink-0 flex-col border-r border-border pr-3 md:flex">
        <button
          onClick={startNewChat}
          className="mb-2 inline-flex items-center justify-center gap-1.5 rounded-md border border-border px-2 py-1.5 text-xs transition-colors hover:border-accent hover:text-foreground"
        >
          <Plus className="size-3.5" /> New chat
        </button>
        <div className="min-h-0 flex-1 space-y-0.5 overflow-auto">
          {[...store.conversations]
            .sort((a, b) => b.updatedAt - a.updatedAt)
            .map((c) => (
              <div
                key={c.id}
                className={cn(
                  "group flex items-center gap-1 rounded-md px-2 py-1.5 text-xs transition-colors",
                  c.id === store.activeId ? "bg-accent/15 text-accent" : "text-muted hover:bg-panel",
                )}
              >
                <button onClick={() => selectConversation(c.id)} className="min-w-0 flex-1 truncate text-left">
                  {c.title || "New chat"}
                </button>
                <button
                  onClick={() => removeConversation(c.id)}
                  title="Delete conversation"
                  className="shrink-0 opacity-0 transition-opacity hover:text-bad group-hover:opacity-100"
                >
                  <Trash2 className="size-3.5" />
                </button>
              </div>
            ))}
        </div>
      </aside>

      {/* Active thread + composer. */}
      <div className="mx-auto flex h-full min-w-0 max-w-3xl flex-1 flex-col">
        {/* Small screens have no sidebar — keep a New chat affordance here. */}
        {messages.length > 0 && (
          <div className="flex items-center justify-between border-b border-border pb-2 md:hidden">
            <span className="text-xs text-muted">
              {messages.filter((m) => m.role === "user").length} message
              {messages.filter((m) => m.role === "user").length === 1 ? "" : "s"}
            </span>
            <button
              onClick={startNewChat}
              className="inline-flex items-center gap-1 rounded-md border border-border px-2 py-1 text-xs text-muted transition-colors hover:border-accent hover:text-foreground"
            >
              <Plus className="size-3.5" /> New chat
            </button>
          </div>
        )}
        <div className="relative min-h-0 flex-1">
          <div ref={scrollRef} onScroll={onScroll} className="h-full overflow-auto">
            {messages.length === 0 ? (
              <EmptyState onPick={setInput} />
            ) : (
              <div className="space-y-4 py-2">
                {messages.map((m, i) => {
                  const isLast = i === messages.length - 1;
                  const canRetry = isLast && !busy && m.role === "assistant" && m.turn.status === "error";
                  return (
                    <div key={i} className="msg-in">
                      <MessageRow msg={m} onRetry={canRetry ? retry : undefined} />
                    </div>
                  );
                })}
              </div>
            )}
          </div>
          {!pinned && messages.length > 0 && (
            <button
              onClick={jumpToLatest}
              title="Jump to latest"
              className="absolute bottom-3 left-1/2 inline-flex -translate-x-1/2 items-center gap-1.5 rounded-full border border-border bg-card px-3 py-1.5 text-xs shadow-lg shadow-black/20 transition-colors hover:border-accent hover:text-accent"
            >
              <ArrowDown className="size-3.5" /> Jump to latest
            </button>
          )}
        </div>

        <div className="border-t border-border pt-3">
        {/* Staged attachments — existing skills/memories/runs handed to the agent
            as context for the next message. */}
        {attached.length > 0 && (
          <div className="mb-2 flex flex-wrap gap-1.5">
            {attached.map((r) => (
              <span
                key={r.id}
                className="inline-flex items-center gap-1 rounded-full border border-accent/40 bg-accent/10 px-2 py-0.5 text-[11px] text-accent"
                title={r.content}
              >
                <span className="font-semibold uppercase tracking-wider opacity-70">{r.kind}</span>
                <span className="max-w-[14rem] truncate">{r.label}</span>
                <button onClick={() => removeAttachment(r.id)} title="Remove" className="hover:text-bad">
                  <X className="size-3" />
                </button>
              </span>
            ))}
          </div>
        )}
        <div className="flex items-end gap-2">
          <Button
            variant="ghost"
            size="icon"
            onClick={() => setAttachOpen(true)}
            title="Attach a skill, memory, or past run"
            className={cn(attached.length > 0 && "text-accent")}
          >
            <Paperclip className="size-4" />
          </Button>
          <MicButton
            onText={(t) => setInput((cur) => (cur.trim() ? cur.trimEnd() + " " : "") + t)}
            disabled={busy}
          />
          <textarea
            ref={taRef}
            value={input}
            onChange={(e) => setInput(e.target.value)}
            onKeyDown={onKeyDown}
            rows={1}
            placeholder="Ask the agent to do something…  (Enter to send, Shift+Enter for a new line)"
            className="max-h-40 min-h-[2.5rem] flex-1 resize-none overflow-y-auto rounded-lg border border-border bg-panel px-3 py-2 text-sm outline-none placeholder:text-muted focus-visible:border-accent"
          />
          {busy ? (
            <Button variant="danger" size="icon" onClick={stop} title="Stop">
              <Square className="size-4" />
            </Button>
          ) : (
            <Button variant="accent" size="icon" onClick={doSend} disabled={!input.trim()} title="Send">
              <Send className="size-4" />
            </Button>
          )}
        </div>
        <div className="mt-1.5 flex items-center gap-2 px-1 text-xs text-muted">
          <span>model</span>
          <ModelPicker value={model} onChange={setModel} activeModel={activeModel} />
          {speechSupported() && (
            <button
              onClick={toggleAutoSpeak}
              title={autoSpeak ? "Auto-speak answers: on" : "Auto-speak answers: off"}
              className={cn(
                "inline-flex items-center gap-1 rounded px-1.5 py-0.5 transition-colors hover:text-foreground",
                autoSpeak ? "text-accent" : "text-muted",
              )}
            >
              {autoSpeak ? <Volume2 className="size-3.5" /> : <VolumeX className="size-3.5" />}
              <span>speak</span>
            </button>
          )}
          <span className="ml-auto">runs through the governed loop · same as the CLI</span>
        </div>
        </div>
      </div>
    </div>
  );
}

function EmptyState({ onPick }: { onPick: (s: string) => void }) {
  const examples = [
    "What can you do?",
    "Summarize my recent runs",
    "Turn off the living room light",
    "What's the weather in Istanbul?",
  ];
  return (
    <div className="flex h-full flex-col items-center justify-center gap-4 text-center">
      <div className="flex size-12 items-center justify-center rounded-2xl bg-accent/15 text-accent">
        <Sparkles className="size-6" />
      </div>
      <div>
        <h2 className="text-lg font-semibold">Talk to your agent</h2>
        <p className="mt-1 text-sm text-muted">
          Type an intent and watch it run — streaming answer, live tool calls, real cost.
        </p>
      </div>
      <div className="flex flex-wrap justify-center gap-2">
        {examples.map((ex) => (
          <button
            key={ex}
            onClick={() => onPick(ex)}
            className="rounded-full border border-border px-3 py-1 text-xs text-muted transition-colors hover:border-accent hover:text-foreground"
          >
            {ex}
          </button>
        ))}
      </div>
    </div>
  );
}

function MessageRow({ msg, onRetry }: { msg: Msg; onRetry?: () => void }) {
  if (msg.role === "user") {
    return (
      <div className="flex justify-end gap-2">
        <div className="max-w-[85%] rounded-2xl rounded-br-sm bg-accent/15 px-3.5 py-2 text-sm text-foreground whitespace-pre-wrap break-words">
          {msg.text}
        </div>
        <div className="mt-0.5 flex size-7 shrink-0 items-center justify-center rounded-full bg-panel text-muted">
          <User className="size-4" />
        </div>
      </div>
    );
  }
  return <AssistantBubble turn={msg.turn} onRetry={onRetry} />;
}

function AssistantBubble({ turn, onRetry }: { turn: ChatTurn; onRetry?: () => void }) {
  const text = turnText(turn);
  const streaming = turn.status === "streaming";
  // Turns restored from older storage have no `timeline` — fall back to the prior
  // shape (tools first, then the final text) so old conversations still render.
  const timeline: TimelineItem[] =
    turn.timeline && turn.timeline.length > 0
      ? turn.timeline
      : [
          ...turn.tools.map((c) => ({ kind: "tool" as const, callId: c.callId })),
          ...(text ? [{ kind: "text" as const, text }] : []),
        ];
  // The trailing timeline item: if it's a tool that hasn't produced output yet,
  // that's the call in flight — drive the live "working" pulse from it.
  const lastItem = timeline[timeline.length - 1];
  const lastTool = lastItem?.kind === "tool" ? turn.tools.find((c) => c.callId === lastItem.callId) : undefined;
  const runningTool = lastTool && !lastTool.output && lastTool.allow !== false ? lastTool : undefined;
  // Show the live pulse at the very start (nothing streamed yet, and no reasoning
  // header to stand in for it) or while the trailing tool runs. Once text streams,
  // the text + cursor convey liveliness instead.
  const working = streaming && (timeline.length === 0 ? !turn.reasoning : !!runningTool);

  return (
    <div className="flex gap-2">
      <div className="mt-0.5 flex size-7 shrink-0 items-center justify-center rounded-full bg-accent/15 text-accent">
        <Sparkles className="size-4" />
      </div>
      <div className="min-w-0 flex-1 space-y-2">
        {turn.reasoning && <ReasoningBlock text={turn.reasoning} live={streaming && timeline.length === 0} />}

        {/* Chronological timeline: each text run and tool call in the order it
            happened, so the conversation reads as "said this → ran that → said
            this" and the final answer is simply the last text run. While
            streaming, text is plain (an unclosed code fence would otherwise
            swallow the rest mid-stream); once final, it renders as Markdown. */}
        {timeline.map((item, i) => {
          if (item.kind === "tool") {
            const c = turn.tools.find((x) => x.callId === item.callId);
            return c ? <ToolChip key={`tl${i}`} c={c} /> : null;
          }
          const isLast = i === timeline.length - 1;
          return streaming ? (
            <div key={`tl${i}`} className="text-sm leading-relaxed text-foreground whitespace-pre-wrap break-words">
              {item.text}
              {isLast && <span className="ml-0.5 inline-block h-4 w-1.5 animate-pulse bg-accent align-text-bottom" />}
            </div>
          ) : (
            <Markdown key={`tl${i}`} source={item.text} className="text-sm text-foreground" />
          );
        })}

        {working && <WorkingIndicator running={runningTool?.tool} />}

        {turn.fallbacks && turn.fallbacks.length > 0 && <FallbackNote hops={turn.fallbacks} />}

        {turn.status === "error" && (
          <div className="space-y-2">
            <div className="flex items-start gap-2 rounded-lg border border-bad/40 bg-bad/10 px-3 py-2 text-sm text-bad">
              <AlertTriangle className="mt-0.5 size-4 shrink-0" />
              <span className="break-words">{turn.error || "the run failed"}</span>
            </div>
            {onRetry && (
              <button
                onClick={onRetry}
                className="inline-flex items-center gap-1.5 rounded-md border border-border px-2.5 py-1 text-xs text-muted transition-colors hover:border-accent hover:text-accent"
              >
                <RotateCcw className="size-3.5" /> Retry
              </button>
            )}
          </div>
        )}

        {turn.status === "done" && (
          <div className="flex items-center gap-3">
            <TurnMeta turn={turn} />
            {text && <CopyAnswer text={text} />}
            {text && <SpeakAnswer text={text} />}
          </div>
        )}

        {turn.status === "done" && turn.correlationId && <LearnedChips corr={turn.correlationId} />}
      </div>
    </div>
  );
}

// WorkingIndicator is the live "the agent is working" pulse: a soft pulsing dot,
// what it's doing right now (running a specific tool, or thinking), and a ticking
// elapsed timer — so a run feels alive and you can see the agent at work rather
// than staring at a static spinner.
function WorkingIndicator({ running }: { running?: string }) {
  const [elapsed, setElapsed] = useState(0);
  useEffect(() => {
    const start = Date.now();
    const id = window.setInterval(() => setElapsed((Date.now() - start) / 1000), 100);
    return () => window.clearInterval(id);
  }, []);
  const label = running ? `running ${running}…` : "thinking…";
  return (
    <div className="flex items-center gap-2 text-sm text-muted">
      <span className="work-pulse size-2.5 rounded-full bg-accent" />
      <span>{label}</span>
      <span className="tabular-nums text-xs text-muted/70">{elapsed.toFixed(1)}s</span>
    </div>
  );
}

// ReasoningBlock shows a reasoning model's chain of thought: expanded and live
// while the model is still thinking (before any answer text), collapsible once
// the answer arrives — so it's visible but never in the way.
function ReasoningBlock({ text, live }: { text: string; live: boolean }) {
  const [open, setOpen] = useState(false);
  const show = live || open;
  return (
    <div className="rounded-lg border border-border bg-panel/40 text-xs">
      <button
        onClick={() => setOpen((o) => !o)}
        className="flex w-full items-center gap-1.5 px-2.5 py-1.5 text-left text-muted hover:text-foreground"
      >
        {live ? (
          <Loader2 className="size-3.5 shrink-0 animate-spin" />
        ) : show ? (
          <ChevronDown className="size-3.5 shrink-0" />
        ) : (
          <ChevronRight className="size-3.5 shrink-0" />
        )}
        <Brain className="size-3.5 shrink-0" />
        <span>{live ? "Thinking…" : "Reasoning"}</span>
      </button>
      {show && (
        <div className="max-h-48 overflow-auto whitespace-pre-wrap break-words border-t border-border px-2.5 py-1.5 font-mono text-[11px] leading-snug text-muted">
          {text}
        </div>
      )}
    </div>
  );
}

// FallbackNote shows when the per-task model chain (M703) had to fall back: the
// primary model failed and a later model in the chain answered. It makes the
// routing you configured observable right where it matters — so when a different
// model answers, you know why, not just that the model name changed.
export function FallbackNote({ hops }: { hops: { from: string; to: string }[] }) {
  // Collapse consecutive hops into one path: a→b, b→c ⇒ a → b → c.
  const path: string[] = hops.length ? [hops[0].from, ...hops.map((h) => h.to)] : [];
  return (
    <div className="flex flex-wrap items-center gap-1.5 rounded-md border border-warn/30 bg-warn/5 px-2 py-1 text-xs text-warn">
      <CornerDownRight className="size-3.5 shrink-0" />
      <span className="font-medium">{hops.length === 1 ? "fell back" : `fell back ${hops.length}×`}</span>
      <span className="font-mono text-foreground/70">{path.join(" → ")}</span>
    </div>
  );
}

function TurnMeta({ turn }: { turn: ChatTurn }) {
  const parts: string[] = [];
  if (turn.model) parts.push(turn.model);
  if (turn.iters) parts.push(`${turn.iters} iter${turn.iters === 1 ? "" : "s"}`);
  if (turn.costMicrocents) parts.push(money(turn.costMicrocents));
  if (parts.length === 0) return null;
  return <div className="text-xs text-muted">{parts.join(" · ")}</div>;
}

// CopyAnswer copies the agent's full reply — the whole answer is often what you
// want to paste elsewhere, not just a code block within it.
function CopyAnswer({ text }: { text: string }) {
  const ui = useUI();
  const [copied, setCopied] = useState(false);
  async function copy() {
    try {
      await navigator.clipboard.writeText(text);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 1200);
    } catch {
      ui.toast("Couldn't copy — clipboard unavailable", "error");
    }
  }
  return (
    <button
      onClick={copy}
      title={copied ? "Copied" : "Copy answer"}
      className="inline-flex items-center gap-1 text-xs text-muted transition-colors hover:text-foreground"
    >
      {copied ? <Check className="size-3 text-good" /> : <Copy className="size-3" />}
      {copied ? "Copied" : "Copy"}
    </button>
  );
}

// SpeakAnswer reads one answer aloud via the browser's speech synthesis — the
// per-message companion to the global auto-speak toggle. Renders nothing when the
// browser can't do TTS. Toggles between speak and stop, and stops on unmount.
function SpeakAnswer({ text }: { text: string }) {
  const [speaking, setSpeaking] = useState(false);
  useEffect(() => () => stopSpeaking(), []);
  if (!speechSupported()) return null;
  function toggle() {
    if (speaking) {
      stopSpeaking();
      setSpeaking(false);
    } else {
      setSpeaking(true);
      speak(text, () => setSpeaking(false));
    }
  }
  return (
    <button
      onClick={toggle}
      title={speaking ? "Stop" : "Read aloud"}
      className="inline-flex items-center gap-1 text-xs text-muted transition-colors hover:text-foreground"
    >
      {speaking ? <VolumeX className="size-3" /> : <Volume2 className="size-3" />}
      {speaking ? "Stop" : "Speak"}
    </button>
  );
}

// LearnedChips shows the memories the daemon recorded during this turn — what the
// agent took away from the exchange — each one forgettable in a click if it's not
// worth keeping. Renders nothing until something is actually learned.
function LearnedChips({ corr }: { corr: string }) {
  const { learnedFor, forgetLearned } = useChat();
  const ui = useUI();
  const [busy, setBusy] = useState<string | null>(null);
  const items = learnedFor(corr);
  if (items.length === 0) return null;

  async function forget(id: string, subject: string) {
    const ok = await ui.confirm({
      title: "Forget this memory?",
      message: subject ? `“${subject}” will be permanently removed.` : "This memory will be permanently removed.",
      confirmLabel: "Forget",
      danger: true,
    });
    if (!ok) return;
    setBusy(id);
    try {
      await forgetLearned(corr, id);
      ui.toast("Memory forgotten", "success");
    } catch (e) {
      ui.toast(`forget failed: ${(e as Error).message}`, "error");
    } finally {
      setBusy(null);
    }
  }

  return (
    <div className="mt-1.5 flex flex-wrap items-center gap-1.5">
      <span className="inline-flex items-center gap-1 text-[10px] text-muted">
        <Brain className="size-3 text-accent" /> learned
      </span>
      {items.map((m) => (
        <span
          key={m.id}
          className="inline-flex items-center gap-1 rounded-full border border-border bg-panel px-2 py-0.5 text-[10px]"
          title={`${m.action} · ${m.type}`}
        >
          <span className="font-semibold uppercase tracking-wider text-accent">{m.type}</span>
          <span className="max-w-[16rem] truncate">{m.subject || "(memory)"}</span>
          <button
            onClick={() => forget(m.id, m.subject)}
            disabled={busy === m.id}
            title="Forget this memory"
            className="text-muted transition-colors hover:text-bad disabled:opacity-50"
          >
            <X className="size-3" />
          </button>
        </span>
      ))}
    </div>
  );
}

function ToolChip({ c }: { c: ChatTool }) {
  const [open, setOpen] = useState(false);
  const denied = c.allow === false;
  // The chip is expandable whenever there's a trace to show — the arguments it
  // was called with and/or the result it returned.
  const hasDetail = !!c.input || !!c.output;
  return (
    <div className="rounded-lg border border-border bg-panel/60 text-xs">
      <button
        onClick={() => hasDetail && setOpen((o) => !o)}
        className={cn("flex w-full items-center gap-2 px-2.5 py-1.5 text-left", !hasDetail && "cursor-default")}
      >
        {hasDetail ? (
          open ? <ChevronDown className="size-3.5 text-muted" /> : <ChevronRight className="size-3.5 text-muted" />
        ) : (
          <Wrench className="size-3.5 text-muted" />
        )}
        <span className="font-medium">{c.tool || "tool"}</span>
        {c.capability && <Badge variant="accent">{c.capability}</Badge>}
        {denied ? (
          <Badge variant="bad">
            <ShieldX className="mr-1 size-3" />
            {c.hardDenied ? "hard-denied" : "denied"}
          </Badge>
        ) : (
          <Badge variant="good">
            <ShieldCheck className="mr-1 size-3" />
            allowed
          </Badge>
        )}
        {c.error && <Badge variant="bad">error</Badge>}
      </button>
      {open && hasDetail && (
        <div className="border-t border-border">
          {c.input && (
            <div>
              <div className="px-2.5 pt-1.5 text-[10px] font-semibold uppercase tracking-wider text-muted">Arguments</div>
              <ToolOutput text={c.input} />
            </div>
          )}
          {c.output && (
            <div>
              <div className="px-2.5 pt-1.5 text-[10px] font-semibold uppercase tracking-wider text-muted">
                {c.error ? "Error" : "Result"}
              </div>
              <ToolOutput text={c.output} />
            </div>
          )}
        </div>
      )}
    </div>
  );
}
