import { useEffect, useRef, useState } from "react";
import { MessageSquare, X, Maximize2, Send, Square, Sparkles, User } from "lucide-react";
import { cn } from "@/lib/utils";
import { useChat } from "@/lib/chatStore";
import { turnText } from "@/lib/chat";
import type { Msg } from "@/lib/conversations";

// MiniChat is the agent, one click away from any screen. A floating launcher
// expands into a compact thread bound to the SAME active conversation as the
// full Chat view (they share the ChatProvider engine), so you can ask something
// without leaving what you're doing — and pop out to the full view for detail.
// It's hidden on the Chat view itself (where the full UI already lives).
export function MiniChat({ hidden, onExpand }: { hidden: boolean; onExpand: () => void }) {
  const { messages, busy, send, stop, enqueue, queue } = useChat();
  const [open, setOpen] = useState(false);
  const [input, setInput] = useState("");
  const scrollRef = useRef<HTMLDivElement>(null);

  // Keep the mini thread pinned to the latest as it streams.
  useEffect(() => {
    const el = scrollRef.current;
    if (el) el.scrollTop = el.scrollHeight;
  }, [messages, open]);

  // If the parent hides us (we're on the full Chat view), also collapse so we
  // don't pop back open when navigating away.
  useEffect(() => {
    if (hidden) setOpen(false);
  }, [hidden]);

  if (hidden) return null;

  function doSend() {
    const t = input.trim();
    if (!t) return;
    // While a run streams, Enter queues the follow-up (M962) — it auto-sends when
    // the current run finishes (manage the queue from the full Chat view).
    if (busy) {
      enqueue(t);
      setInput("");
      return;
    }
    setInput("");
    send(t);
  }

  // Closed: a floating launcher. A soft pulse marks an in-flight run so you know
  // the agent is working even with the panel closed.
  if (!open) {
    return (
      <button
        onClick={() => setOpen(true)}
        title="Ask the agent"
        className="fixed bottom-5 right-5 z-[90] flex size-12 items-center justify-center rounded-full bg-accent text-white shadow-lg shadow-black/30 transition-transform hover:scale-105"
      >
        <MessageSquare className="size-5" />
        {busy && <span className="work-pulse absolute -right-0.5 -top-0.5 size-3 rounded-full bg-good ring-2 ring-background" />}
      </button>
    );
  }

  return (
    <div className="fixed bottom-5 right-5 z-[90] flex h-[28rem] w-[min(92vw,22rem)] flex-col overflow-hidden rounded-xl border border-border bg-card shadow-xl shadow-black/30">
      {/* Header */}
      <div className="flex items-center gap-2 border-b border-border px-3 py-2">
        <Sparkles className="size-4 text-accent" />
        <span className="text-sm font-semibold">Agent</span>
        {busy && <span className="work-pulse ml-0.5 size-2 rounded-full bg-good" />}
        <div className="ml-auto flex items-center gap-1">
          <button
            onClick={onExpand}
            title="Open full chat"
            className="rounded p-1 text-muted transition-colors hover:bg-panel hover:text-foreground"
          >
            <Maximize2 className="size-3.5" />
          </button>
          <button
            onClick={() => setOpen(false)}
            title="Close"
            className="rounded p-1 text-muted transition-colors hover:bg-panel hover:text-foreground"
          >
            <X className="size-3.5" />
          </button>
        </div>
      </div>

      {/* Thread */}
      <div ref={scrollRef} className="min-h-0 flex-1 space-y-3 overflow-auto p-3">
        {messages.length === 0 ? (
          <div className="flex h-full flex-col items-center justify-center gap-2 text-center text-xs text-muted">
            <Sparkles className="size-6 text-accent/70" />
            <p>Ask me anything — I run through the same governed loop as the full Chat.</p>
          </div>
        ) : (
          messages.map((m, i) => <MiniRow key={i} msg={m} live={busy && i === messages.length - 1} />)
        )}
      </div>

      {/* Queue hint (M962): how many follow-ups are lined up; manage them in the
          full Chat view. */}
      {queue.length > 0 && (
        <div className="flex items-center gap-1.5 border-t border-border px-2 pt-1.5 text-[11px] text-muted">
          <MessageSquare className="size-3" />
          {queue.length} queued — {busy ? "sends after this run" : "idle"} · open Chat to manage
        </div>
      )}

      {/* Composer */}
      <div className="flex items-end gap-2 border-t border-border p-2">
        <textarea
          value={input}
          onChange={(e) => setInput(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter" && !e.shiftKey) {
              e.preventDefault();
              doSend();
            }
          }}
          rows={1}
          placeholder={busy ? "Queue a follow-up…" : "Ask the agent…"}
          className="max-h-24 min-h-[2.25rem] flex-1 resize-none rounded-lg border border-border bg-panel px-2.5 py-2 text-sm outline-none placeholder:text-muted focus-visible:border-accent"
        />
        {busy ? (
          <button
            onClick={stop}
            title="Stop"
            className="flex size-9 shrink-0 items-center justify-center rounded-lg border border-bad text-bad transition-colors hover:bg-bad hover:text-white"
          >
            <Square className="size-4" />
          </button>
        ) : (
          <button
            onClick={doSend}
            disabled={!input.trim()}
            title="Send"
            className="flex size-9 shrink-0 items-center justify-center rounded-lg bg-accent text-white transition-opacity hover:opacity-90 disabled:opacity-50"
          >
            <Send className="size-4" />
          </button>
        )}
      </div>
    </div>
  );
}

// MiniRow is the condensed message renderer — user bubbles right, assistant text
// left, a "working…" line while the trailing turn is still empty. The full tool
// trace / reasoning lives in the expanded Chat view.
function MiniRow({ msg, live }: { msg: Msg; live: boolean }) {
  if (msg.role === "user") {
    return (
      <div className="flex justify-end gap-1.5">
        <div className="max-w-[85%] whitespace-pre-wrap break-words rounded-2xl rounded-br-sm bg-accent/15 px-2.5 py-1.5 text-xs text-foreground">
          {msg.text}
        </div>
        <div className="mt-0.5 flex size-5 shrink-0 items-center justify-center rounded-full bg-panel text-muted">
          <User className="size-3" />
        </div>
      </div>
    );
  }
  const text = turnText(msg.turn);
  const working = live && !text;
  const failed = msg.turn.status === "error";
  return (
    <div className="flex gap-1.5">
      <div className="mt-0.5 flex size-5 shrink-0 items-center justify-center rounded-full bg-accent/15 text-accent">
        <Sparkles className="size-3" />
      </div>
      <div className="min-w-0 flex-1 text-xs">
        {working ? (
          <span className="inline-flex items-center gap-1.5 text-muted">
            <span className="work-pulse size-2 rounded-full bg-accent" /> working…
          </span>
        ) : failed && !text ? (
          <span className="text-bad">{msg.turn.error || "the run failed"}</span>
        ) : (
          <div className="whitespace-pre-wrap break-words text-foreground/90">{text}</div>
        )}
      </div>
    </div>
  );
}
