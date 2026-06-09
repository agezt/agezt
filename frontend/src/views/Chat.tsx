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
} from "lucide-react";
import { cn } from "@/lib/utils";
import { money } from "@/lib/format";
import { getJSON } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { useUI } from "@/components/ui/feedback";
import { Badge } from "@/components/ui/badge";
import { Markdown } from "@/components/Markdown";
import { ToolOutput } from "@/components/DataView";
import {
  streamRun,
  foldChatFrame,
  newTurn,
  turnText,
  type ChatTurn,
  type ChatTool,
  type ChatHistoryTurn,
} from "@/lib/chat";
import {
  loadStore,
  saveStore,
  activeMessages,
  withActiveMessages,
  startConversation,
  deleteConversation,
  type Msg,
  type Store,
} from "@/lib/conversations";

// A unique id for a new conversation (localhost is a secure context, so
// crypto.randomUUID is available; a timestamp+random fallback covers the rest).
function genId(): string {
  if (typeof crypto !== "undefined" && crypto.randomUUID) return crypto.randomUUID();
  return "c-" + Date.now().toString(36) + Math.random().toString(36).slice(2, 8);
}

// buildHistory turns prior thread messages into the history payload sent with a
// run, so the agent has multi-turn context. Empty turns are dropped. Shared by
// send() (full thread) and retry() (thread up to the failed intent).
export function buildHistory(msgs: Msg[]): ChatHistoryTurn[] {
  return msgs
    .map((m): ChatHistoryTurn =>
      m.role === "user" ? { role: "user", text: m.text } : { role: "assistant", text: turnText(m.turn) },
    )
    .filter((t) => t.text.trim() !== "");
}

// Chat is the humane front door to the agent: a conversational thread where you
// type an intent and watch the governed loop answer live — streaming text, the
// tool calls it made (with the policy verdict), and the final answer with its
// cost. It drives the same CmdRun as the CLI, so what you see here is exactly
// what the daemon did.
export function Chat() {
  const ui = useUI();
  const [store, setStore] = useState<Store>(() => loadStore(genId, Date.now()));
  const [input, setInput] = useState("");
  const [model, setModel] = useState("");
  const [activeModel, setActiveModel] = useState("");
  const [busy, setBusy] = useState(false);
  // pinned = the thread is stuck to the bottom (auto-scrolls on new content).
  // It flips to false when you scroll up to read scrollback, so a live stream
  // never yanks you back down mid-read; a "jump to latest" button restores it.
  const [pinned, setPinned] = useState(true);
  const abortRef = useRef<AbortController | null>(null);
  const scrollRef = useRef<HTMLDivElement>(null);
  const taRef = useRef<HTMLTextAreaElement>(null);

  // The active conversation's messages, and a setMessages shim that routes any
  // update back into that conversation in the store — so every existing call
  // site (send, updateLastTurn) works unchanged.
  const messages = activeMessages(store);
  const setMessages = (updater: Msg[] | ((prev: Msg[]) => Msg[])) => {
    setStore((s) => {
      const prev = activeMessages(s);
      const next = typeof updater === "function" ? (updater as (p: Msg[]) => Msg[])(prev) : updater;
      return withActiveMessages(s, next, Date.now());
    });
  };

  // The daemon's default model — shown so you always know which model you're
  // talking to (and a typo in the override field is obvious against it).
  useEffect(() => {
    getJSON<{ model?: string }>("/api/config")
      .then((c) => setActiveModel(String(c.model || "")))
      .catch(() => {
        /* config momentarily unavailable — the field just shows "default" */
      });
  }, []);

  // Persist the store (skip while a turn is mid-stream — a half-folded turn
  // would restore as "interrupted"; the settled store is saved on completion).
  useEffect(() => {
    if (busy) return;
    saveStore(store);
  }, [store, busy]);

  function newChat() {
    if (busy) abortRef.current?.abort();
    setStore((s) => startConversation(s, genId, Date.now()));
    setInput("");
  }

  function selectConversation(id: string) {
    if (id === store.activeId) return;
    if (busy) abortRef.current?.abort();
    setStore((s) => ({ ...s, activeId: id }));
    setInput("");
  }

  function removeConversation(id: string) {
    if (busy && id === store.activeId) abortRef.current?.abort();
    setStore((s) => deleteConversation(s, id, genId, Date.now()));
  }

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

  // Replace the trailing assistant turn (the one currently streaming).
  function updateLastTurn(fn: (t: ChatTurn) => ChatTurn) {
    setMessages((prev) => {
      const next = prev.slice();
      const last = next[next.length - 1];
      if (last && last.role === "assistant") {
        next[next.length - 1] = { role: "assistant", turn: fn(last.turn) };
      }
      return next;
    });
  }

  // streamIntent runs one intent against the governed loop, folding frames into
  // the trailing assistant turn (which the caller must have just appended).
  async function streamIntent(intent: string, history: ChatHistoryTurn[]) {
    setPinned(true);
    setBusy(true);
    const ctrl = new AbortController();
    abortRef.current = ctrl;
    try {
      await streamRun(
        { intent, model: model.trim() || undefined, history: history.length ? history : undefined },
        (f) => updateLastTurn((t) => foldChatFrame(t, f)),
        ctrl.signal,
      );
    } catch (e) {
      if (ctrl.signal.aborted) {
        updateLastTurn((t) => ({ ...t, status: "error", error: "stopped" }));
      } else {
        updateLastTurn((t) => ({ ...t, status: "error", error: (e as Error).message }));
      }
    } finally {
      abortRef.current = null;
      setBusy(false);
    }
  }

  async function send() {
    const intent = input.trim();
    if (!intent || busy) return;
    const history = buildHistory(messages);
    setInput("");
    setMessages((m) => [...m, { role: "user", text: intent }, { role: "assistant", turn: newTurn() }]);
    await streamIntent(intent, history);
  }

  // retry re-runs the most recent user intent after a failed turn: it drops the
  // errored assistant turn, re-appends a fresh one, and streams again with the
  // same prior history — so a transient failure is one click to recover from.
  async function retry() {
    if (busy) return;
    let userIdx = -1;
    for (let i = messages.length - 1; i >= 0; i--) {
      if (messages[i].role === "user") {
        userIdx = i;
        break;
      }
    }
    if (userIdx < 0) return;
    const intent = (messages[userIdx] as Msg & { role: "user" }).text;
    const history = buildHistory(messages.slice(0, userIdx));
    setMessages((prev) => [...prev.slice(0, userIdx + 1), { role: "assistant", turn: newTurn() }]);
    await streamIntent(intent, history);
  }

  function stop() {
    abortRef.current?.abort();
  }

  function onKeyDown(e: React.KeyboardEvent<HTMLTextAreaElement>) {
    // Enter sends; Shift+Enter inserts a newline (ChatGPT/Claude convention).
    if (e.key === "Enter" && !e.shiftKey) {
      e.preventDefault();
      send();
    }
  }

  return (
    <div className="flex h-full gap-3">
      {/* Conversation list — past threads, ChatGPT-style (desktop). */}
      <aside className="hidden w-52 shrink-0 flex-col border-r border-border pr-3 md:flex">
        <button
          onClick={newChat}
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
              onClick={newChat}
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
        <div className="flex items-end gap-2">
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
            <Button variant="accent" size="icon" onClick={send} disabled={!input.trim()} title="Send">
              <Send className="size-4" />
            </Button>
          )}
        </div>
        <div className="mt-1.5 flex items-center gap-2 px-1 text-xs text-muted">
          <span>model</span>
          <input
            value={model}
            onChange={(e) => setModel(e.target.value)}
            placeholder={activeModel || "default"}
            title={activeModel ? `active model: ${activeModel}` : undefined}
            className="h-6 w-44 rounded border border-border bg-panel px-2 text-xs outline-none placeholder:text-muted/60 focus-visible:border-accent"
          />
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
  // The most recent tool that hasn't produced output yet is the one in flight.
  const running = streaming ? [...turn.tools].reverse().find((t) => !t.output && t.allow !== false) : undefined;
  // Show the live working indicator during the pre-answer phase whenever a tool
  // is actually running (the satisfying "watch it work" moment), or when there's
  // no reasoning stream to stand in for it. While a reasoning model is only
  // thinking (no tool yet), its own live "Thinking…" header covers it.
  const working = streaming && !text && (!!running || !turn.reasoning);

  return (
    <div className="flex gap-2">
      <div className="mt-0.5 flex size-7 shrink-0 items-center justify-center rounded-full bg-accent/15 text-accent">
        <Sparkles className="size-4" />
      </div>
      <div className="min-w-0 flex-1 space-y-2">
        {turn.reasoning && <ReasoningBlock text={turn.reasoning} live={streaming && !text} />}

        {turn.tools.length > 0 && (
          <div className="space-y-1.5">
            {turn.tools.map((c) => (
              <ToolChip key={c.callId || c.tool} c={c} />
            ))}
          </div>
        )}

        {working ? (
          <WorkingIndicator running={running?.tool} />
        ) : (
          text &&
          // While streaming, render plain text (an unclosed code fence would
          // otherwise swallow the rest mid-stream); once the answer is final,
          // render it as Markdown.
          (streaming ? (
            <div className="text-sm leading-relaxed text-foreground whitespace-pre-wrap break-words">
              {text}
              <span className="ml-0.5 inline-block h-4 w-1.5 animate-pulse bg-accent align-text-bottom" />
            </div>
          ) : (
            <Markdown source={text} className="text-sm text-foreground" />
          ))
        )}

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
          </div>
        )}
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

function ToolChip({ c }: { c: ChatTool }) {
  const [open, setOpen] = useState(false);
  const denied = c.allow === false;
  return (
    <div className="rounded-lg border border-border bg-panel/60 text-xs">
      <button
        onClick={() => setOpen((o) => !o)}
        className="flex w-full items-center gap-2 px-2.5 py-1.5 text-left"
      >
        {c.output ? (
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
      {open && c.output && <ToolOutput text={c.output} />}
    </div>
  );
}
