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
} from "lucide-react";
import { cn } from "@/lib/utils";
import { money } from "@/lib/format";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Markdown } from "@/components/Markdown";
import {
  streamRun,
  foldChatFrame,
  newTurn,
  turnText,
  type ChatTurn,
  type ChatTool,
  type ChatHistoryTurn,
} from "@/lib/chat";

interface UserMsg {
  role: "user";
  text: string;
}
interface BotMsg {
  role: "assistant";
  turn: ChatTurn;
}
type Msg = UserMsg | BotMsg;

// Chat is the humane front door to the agent: a conversational thread where you
// type an intent and watch the governed loop answer live — streaming text, the
// tool calls it made (with the policy verdict), and the final answer with its
// cost. It drives the same CmdRun as the CLI, so what you see here is exactly
// what the daemon did.
export function Chat() {
  const [messages, setMessages] = useState<Msg[]>([]);
  const [input, setInput] = useState("");
  const [model, setModel] = useState("");
  const [busy, setBusy] = useState(false);
  const abortRef = useRef<AbortController | null>(null);
  const scrollRef = useRef<HTMLDivElement>(null);
  const taRef = useRef<HTMLTextAreaElement>(null);

  // Pin the thread to the bottom as content streams in.
  useEffect(() => {
    const el = scrollRef.current;
    if (el) el.scrollTop = el.scrollHeight;
  }, [messages]);

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

  async function send() {
    const intent = input.trim();
    if (!intent || busy) return;
    // Prior turns become the conversation history sent with this run, so the
    // agent has multi-turn context (the server folds them into the intent).
    const history: ChatHistoryTurn[] = messages
      .map((m): ChatHistoryTurn =>
        m.role === "user" ? { role: "user", text: m.text } : { role: "assistant", text: turnText(m.turn) },
      )
      .filter((t) => t.text.trim() !== "");
    setInput("");
    setBusy(true);
    setMessages((m) => [...m, { role: "user", text: intent }, { role: "assistant", turn: newTurn() }]);

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
    <div className="mx-auto flex h-full max-w-3xl flex-col">
      <div ref={scrollRef} className="min-h-0 flex-1 overflow-auto">
        {messages.length === 0 ? <EmptyState onPick={setInput} /> : (
          <div className="space-y-4 py-2">
            {messages.map((m, i) => (
              <MessageRow key={i} msg={m} />
            ))}
          </div>
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
            placeholder="default"
            className="h-6 w-40 rounded border border-border bg-panel px-2 text-xs outline-none placeholder:text-muted/60 focus-visible:border-accent"
          />
          <span className="ml-auto">runs through the governed loop · same as the CLI</span>
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

function MessageRow({ msg }: { msg: Msg }) {
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
  return <AssistantBubble turn={msg.turn} />;
}

function AssistantBubble({ turn }: { turn: ChatTurn }) {
  const text = turnText(turn);
  const streaming = turn.status === "streaming";
  const thinking = streaming && !text && turn.tools.length === 0 && !turn.reasoning;

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

        {thinking ? (
          <div className="flex items-center gap-2 text-sm text-muted">
            <Loader2 className="size-4 animate-spin" /> thinking…
          </div>
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
          <div className="flex items-start gap-2 rounded-lg border border-bad/40 bg-bad/10 px-3 py-2 text-sm text-bad">
            <AlertTriangle className="mt-0.5 size-4 shrink-0" />
            <span className="break-words">{turn.error || "the run failed"}</span>
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
  const [copied, setCopied] = useState(false);
  async function copy() {
    try {
      await navigator.clipboard.writeText(text);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 1200);
    } catch {
      /* clipboard unavailable — silently no-op */
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
      {open && c.output && (
        <pre className="overflow-auto border-t border-border px-2.5 py-1.5 font-mono text-[11px] leading-snug text-muted">
          {c.output}
        </pre>
      )}
    </div>
  );
}
