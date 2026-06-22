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
  ArrowUp,
  ArrowRight,
  Forward,
  StickyNote,
  ListPlus,
  X,
  Paperclip,
  Volume2,
  VolumeX,
  CornerDownRight,
  Pencil,
  Bot,
  Download,
  Pin,
  Search,
  Gauge,
  Scissors,
} from "lucide-react";
import { cn, fmtTime } from "@/lib/utils";
import { money, fmtCount } from "@/lib/format";
import { findModelContext, fmtContext, type ModelCatalog } from "@/lib/models";
import { getJSON } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { useUI } from "@/components/ui/feedback";
import { Badge } from "@/components/ui/badge";
import { Markdown } from "@/components/Markdown";
import { ToolOutput } from "@/components/DataView";
import { ModelPicker } from "@/components/ModelPicker";
import { AgentPicker } from "@/components/AgentPicker";
import { AttachPicker } from "@/components/AttachPicker";
import { AgentAvatar } from "@/components/AgentAvatar";
import { MicButton } from "@/components/MicButton";
import { speak, stopSpeaking, speechSupported } from "@/lib/speech";

const AUTOSPEAK_KEY = "agezt.chat.autospeak";
import {
  turnText,
  contextTokensUsed,
  CHARS_PER_TOKEN,
  type ChatTurn,
  type ChatTool,
  type TimelineItem,
  type TurnCompaction,
} from "@/lib/chat";
import { useChat } from "@/lib/chatStore";
import { buildContext, type AttachRef } from "@/lib/attach";
import { sortConversations, filterConversations, type HistorySummary, type Msg } from "@/lib/conversations";
import type { QueuedMsg } from "@/lib/queue";
import { conversationToMarkdown, slugify, downloadText } from "@/lib/export";
import { ChannelSessions } from "@/views/ChannelSessions";

// Chat is the humane front door to the agent: a conversational thread where you
// type an intent and watch the governed loop answer live — streaming text, the
// tool calls it made (with the policy verdict), and the final answer with its
// cost. The engine (store, streaming, model) lives in ChatProvider so a run keeps
// going when you leave the view; this component is the full-screen UI over it.
export function Chat() {
  const { store, messages, busy, model, setModel, agent, setAgent, activeModel, send, retry, continueRun, editAndResend, conversationPersona, setConversationPersona, autoApproveForge, setAutoApproveForge, historySummary, stop, newChat, selectConversation, removeConversation, renameConversation, togglePin, activeCorr, steer, queue, enqueue, removeQueued, reorderQueued, clearQueue, sendQueuedNow } =
    useChat();
  const ui = useUI();
  const [input, setInput] = useState("");
  // convFilter filters the conversation sidebar by title/message text (M732).
  const [convFilter, setConvFilter] = useState("");
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
  // The chat task's routing chain (M931): pinned to the top of the model picker
  // so the models this conversation will actually fall back through lead the
  // list — pick from your configured fallbacks first, keyed providers after.
  const [chatChain, setChatChain] = useState<string[]>([]);
  useEffect(() => {
    let live = true;
    getJSON<{ chains?: Record<string, string[]> }>("/api/routing")
      .then((r) => {
        if (live) setChatChain(r.chains?.chat || []);
      })
      .catch(() => {});
    return () => {
      live = false;
    };
  }, []);

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
  // engine with any attached context (skills/memories/runs) prepended. While a
  // run is in flight, Enter instead QUEUES the message (M962) — it auto-sends
  // when the current run finishes; Steer/BTW are the explicit interrupt buttons.
  function doSend() {
    const t = input.trim();
    if (!t) return;
    if (busy) {
      enqueue(t);
      setInput("");
      return;
    }
    stopSpeaking(); // a new turn interrupts any answer being read aloud
    setPinned(true);
    setInput("");
    const ctx = buildContext(attached);
    setAttached([]);
    send(t, ctx);
  }

  // doSteer injects the composer text into the running run (M962): mode "steer"
  // re-prioritises, "note" is a soft BTW. Clears the box on success.
  async function doSteer(mode: "steer" | "note") {
    const t = input.trim();
    if (!t || !activeCorr) return;
    try {
      await steer(t, mode);
      setInput("");
      ui.toast(mode === "note" ? "BTW sent — the agent will read it and keep going" : "Steered — the agent will break at its next safe point", "success");
    } catch (e) {
      ui.toast((e as Error).message, "error");
    }
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
          className="mb-2 inline-flex items-center justify-center gap-1.5 rounded-md border border-accent/40 bg-accent/10 px-2 py-1.5 text-xs font-medium text-accent transition-colors hover:bg-accent hover:text-white"
        >
          <Plus className="size-3.5" /> New chat
        </button>
        {store.conversations.length > 1 && (
          <div className="relative mb-2">
            <Search className="pointer-events-none absolute left-2 top-1/2 size-3 -translate-y-1/2 text-muted" />
            <input
              value={convFilter}
              onChange={(e) => setConvFilter(e.target.value)}
              placeholder="Search chats…"
              aria-label="Search conversations"
              className="h-7 w-full rounded-md border border-border bg-panel pl-7 pr-6 text-xs outline-none focus:border-accent"
            />
            {convFilter && (
              <button
                onClick={() => setConvFilter("")}
                aria-label="Clear conversation search"
                className="absolute right-1.5 top-1/2 -translate-y-1/2 text-muted transition-colors hover:text-foreground"
              >
                <X className="size-3" />
              </button>
            )}
          </div>
        )}
        <div className="min-h-0 flex-1 space-y-0.5 overflow-auto">
          {(() => {
            const shown = sortConversations(filterConversations(store.conversations, convFilter));
            if (shown.length === 0) {
              return <p className="px-2 py-3 text-center text-[11px] text-muted">No chats match “{convFilter.trim()}”</p>;
            }
            return shown.map((c) => (
              <ConversationItem
                key={c.id}
                title={c.title || "New chat"}
                active={c.id === store.activeId}
                pinned={!!c.pinned}
                onSelect={() => selectConversation(c.id)}
                onRemove={() => removeConversation(c.id)}
                onRename={(t) => renameConversation(c.id, t)}
                onTogglePin={() => togglePin(c.id)}
              />
            ));
          })()}
        </div>
        {/* Channel-originated sessions (Telegram/Slack/…) — follow them live (M841). */}
        <ChannelSessions />
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
                  // Regenerate a completed answer (re-run the same intent, replacing
                  // this turn) — the staple chat affordance, reusing retry's logic.
                  const canRegenerate = isLast && !busy && m.role === "assistant" && m.turn.status === "done";
                  return (
                    <div key={i} className="msg-in">
                      {historySummary && historySummary.upto === i && i > 0 && (
                        <SummaryDivider summary={historySummary} />
                      )}
                      <MessageRow
                        msg={m}
                        agent={agent}
                        onRetry={canRetry ? retry : undefined}
                        onContinue={canRetry ? continueRun : undefined}
                        onRegenerate={canRegenerate ? retry : undefined}
                        onEdit={!busy && m.role === "user" ? (text) => editAndResend(i, text) : undefined}
                      />
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
        {/* Queued follow-ups (M962): lined up while a run streams; the next one
            auto-sends when the current run finishes. Reorder / delete / clear. */}
        {queue.length > 0 && (
          <QueuePanel
            queue={queue}
            busy={busy}
            onUp={(id) => reorderQueued(id, -1)}
            onDown={(id) => reorderQueued(id, 1)}
            onRemove={removeQueued}
            onClear={clearQueue}
            onSendNow={sendQueuedNow}
          />
        )}
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
        {/* Composer surface (M995): input + controls in one elevated card that
            lights an accent ring on focus, like a modern chat composer. */}
        <div className="rounded-xl border border-border bg-panel/40 px-2 py-1.5 shadow-e1 transition-[border-color,box-shadow] focus-within:border-accent focus-within:ring-2 focus-within:ring-accent/20">
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
            placeholder={
              busy
                ? "Run in flight — Enter queues a follow-up; Steer or BTW the running agent →"
                : "Ask the agent to do something…  (Enter to send, Shift+Enter for a new line)"
            }
            className="max-h-40 min-h-[2.5rem] flex-1 resize-none overflow-y-auto bg-transparent px-1.5 py-1.5 text-sm outline-none placeholder:text-muted"
          />
          {busy ? (
            <div className="flex items-end gap-1">
              {activeCorr && (
                <>
                  <Button variant="ghost" size="icon" onClick={() => doSteer("note")} disabled={!input.trim()} title="BTW — the agent reads this and keeps going (doesn't break its task)">
                    <StickyNote className="size-4" />
                  </Button>
                  <Button variant="accent" size="icon" onClick={() => doSteer("steer")} disabled={!input.trim()} title="Steer — interrupt at the next safe point and follow this">
                    <Forward className="size-4" />
                  </Button>
                </>
              )}
              <Button variant="ghost" size="icon" onClick={doSend} disabled={!input.trim()} title="Queue — send after the current run finishes">
                <ListPlus className="size-4" />
              </Button>
              <Button variant="danger" size="icon" onClick={stop} title="Stop">
                <Square className="size-4" />
              </Button>
            </div>
          ) : (
            <Button variant="accent" size="icon" onClick={doSend} disabled={!input.trim()} title="Send">
              <Send className="size-4" />
            </Button>
          )}
        </div>
        <div className="mt-1.5 flex flex-wrap items-center gap-2 border-t border-border/40 px-1 pt-1.5 text-xs text-muted">
          <span>model</span>
          <ModelPicker
            value={model}
            onChange={setModel}
            // With no explicit pick the chat ROUTING CHAIN serves the run, not the
            // kernel default — show the chain's primary so the label tells the truth.
            activeModel={chatChain.length ? `${chatChain[0]} (routing)` : activeModel}
            pinned={chatChain.length ? { label: "chat routing chain", ids: chatChain } : undefined}
          />
          <AgentPicker value={agent} onChange={setAgent} />
          <ConversationPersona value={conversationPersona} onChange={setConversationPersona} />
          <button
            onClick={() => setAutoApproveForge(!autoApproveForge)}
            role="switch"
            aria-checked={autoApproveForge}
            title={
              autoApproveForge
                ? "Tool Forge actions are auto-approved for this session — click to require approval again"
                : "Auto-approve Tool Forge actions for this session (no approval prompt while building an agent army)"
            }
            className={
              "inline-flex items-center gap-1 rounded px-1.5 py-0.5 text-xs transition-colors " +
              (autoApproveForge
                ? "bg-accent/15 text-accent"
                : "text-muted hover:text-foreground")
            }
          >
            <span aria-hidden>{autoApproveForge ? "🔓" : "🔒"}</span>
            Forge auto-approve
          </button>
          <PromptLauncher onPick={(text) => setInput((cur) => (cur.trim() ? cur.trimEnd() + "\n" : "") + text)} />
          {messages.length > 0 && (
            <button
              onClick={() => {
                const title =
                  store.conversations.find((c) => c.id === store.activeId)?.title || "Conversation";
                downloadText(`${slugify(title)}.md`, conversationToMarkdown(title, messages));
              }}
              title="Export this conversation as Markdown"
              className="inline-flex items-center gap-1 rounded px-1.5 py-0.5 text-muted transition-colors hover:text-foreground"
            >
              <Download className="size-3.5" />
              <span>export</span>
            </button>
          )}
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
    </div>
  );
}

// ConversationItem is one row in the thread sidebar: click to switch, hover to
// reveal rename (pencil → inline input) and delete. Renaming a thread (M720) lets
// you organise the sidebar instead of living with auto-derived titles.
export function ConversationItem({
  title,
  active,
  pinned,
  onSelect,
  onRemove,
  onRename,
  onTogglePin,
}: {
  title: string;
  active: boolean;
  pinned?: boolean;
  onSelect: () => void;
  onRemove: () => void;
  onRename: (title: string) => void;
  onTogglePin?: () => void;
}) {
  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState(title);
  const ref = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (editing) {
      ref.current?.focus();
      ref.current?.select();
    }
  }, [editing]);

  function begin() {
    setDraft(title);
    setEditing(true);
  }
  function commit() {
    setEditing(false);
    if (draft !== title) onRename(draft);
  }

  return (
    <div
      className={cn(
        "group relative flex items-center gap-1 rounded-md px-2 py-1.5 text-xs transition-colors",
        active
          ? "bg-gradient-to-r from-accent/20 to-transparent font-medium text-accent before:absolute before:left-0 before:top-1/2 before:h-4 before:w-[3px] before:-translate-y-1/2 before:rounded-r-full before:bg-accent before:content-['']"
          : "text-muted hover:bg-panel",
      )}
    >
      {editing ? (
        <input
          ref={ref}
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          onBlur={commit}
          onKeyDown={(e) => {
            if (e.key === "Enter") commit();
            else if (e.key === "Escape") {
              setDraft(title);
              setEditing(false);
            }
          }}
          aria-label="Conversation title"
          className="min-w-0 flex-1 rounded border border-accent/40 bg-panel px-1 text-xs outline-none"
        />
      ) : (
        <>
          {onTogglePin && (
            <button
              onClick={onTogglePin}
              title={pinned ? "Unpin conversation" : "Pin conversation"}
              aria-label={pinned ? "Unpin conversation" : "Pin conversation"}
              className={cn(
                "shrink-0 transition-opacity hover:text-foreground",
                pinned ? "text-accent opacity-100" : "opacity-0 group-hover:opacity-100",
              )}
            >
              <Pin className={cn("size-3", pinned && "fill-current")} />
            </button>
          )}
          <button onClick={onSelect} onDoubleClick={begin} className="min-w-0 flex-1 truncate text-left">
            {title}
          </button>
          <button
            onClick={begin}
            title="Rename conversation"
            aria-label="Rename conversation"
            className="shrink-0 opacity-0 transition-opacity hover:text-foreground group-hover:opacity-100"
          >
            <Pencil className="size-3" />
          </button>
          <button
            onClick={onRemove}
            title="Delete conversation"
            aria-label="Delete conversation"
            className="shrink-0 opacity-0 transition-opacity hover:text-bad group-hover:opacity-100"
          >
            <Trash2 className="size-3.5" />
          </button>
        </>
      )}
    </div>
  );
}

const DEFAULT_EXAMPLES = [
  "What can you do?",
  "Summarize my recent runs",
  "Turn off the living room light",
  "What's the weather in Istanbul?",
];

function EmptyState({ onPick }: { onPick: (s: string) => void }) {
  // Saved prompts (M713) replace the generic examples once the owner defines any —
  // their own reusable workflows, one click to launch. Falls back to examples.
  const [prompts, setPrompts] = useState<{ title: string; text: string }[]>([]);
  useEffect(() => {
    getJSON<{ prompts?: { title: string; text: string }[] }>("/api/prompts")
      .then((r) => setPrompts(r.prompts || []))
      .catch(() => {
        /* prompts are optional — fall back to the examples */
      });
  }, []);
  const hasSaved = prompts.length > 0;
  const chips = hasSaved ? prompts : DEFAULT_EXAMPLES.map((text) => ({ title: text, text }));

  return (
    <div className="flex h-full flex-col items-center justify-center gap-4 text-center">
      <div className="flex size-12 items-center justify-center rounded-2xl bg-gradient-to-br from-accent/30 to-accent/5 text-accent shadow-e2 ring-1 ring-inset ring-accent/20">
        <Sparkles className="size-6" />
      </div>
      <div>
        <h2 className="text-lg font-semibold">Talk to your agent</h2>
        <p className="mt-1 text-sm text-muted">
          Type an intent and watch it run — streaming answer, live tool calls, real cost.
        </p>
      </div>
      <div className="flex max-w-2xl flex-wrap justify-center gap-2">
        {chips.map((p, i) => (
          <button
            key={`${p.title}-${i}`}
            onClick={() => onPick(p.text)}
            title={hasSaved ? p.text : undefined}
            className="rounded-full border border-border bg-panel/60 px-3 py-1 text-xs text-muted shadow-e1 transition-all hover:-translate-y-0.5 hover:border-accent hover:bg-accent/10 hover:text-accent"
          >
            {p.title}
          </button>
        ))}
      </div>
      {hasSaved && <p className="text-[11px] text-muted/70">your saved prompts · manage in System → Prompts</p>}
    </div>
  );
}

// QueuePanel (M962) shows the pending follow-up messages lined up while a run
// streams. The next one auto-sends when the run finishes; here the operator can
// reorder (up/down), delete one, clear all, or — when idle — send the front now.
function QueuePanel({
  queue,
  busy,
  onUp,
  onDown,
  onRemove,
  onClear,
  onSendNow,
}: {
  queue: QueuedMsg[];
  busy: boolean;
  onUp: (id: string) => void;
  onDown: (id: string) => void;
  onRemove: (id: string) => void;
  onClear: () => void;
  onSendNow: () => void;
}) {
  return (
    <div className="mb-2 rounded-lg border border-border bg-panel/40 p-2">
      <div className="mb-1.5 flex items-center gap-2 px-0.5 text-[11px] text-muted">
        <ListPlus className="size-3.5" />
        <span className="font-medium text-foreground/80">Queue</span>
        <span className="rounded-full bg-card px-1.5 tabular-nums">{queue.length}</span>
        <span className="text-xs">{busy ? "next sends when this run finishes" : "idle — send the next now"}</span>
        <button onClick={onClear} className="ml-auto inline-flex items-center gap-1 text-xs hover:text-bad" title="Clear the whole queue">
          <Trash2 className="size-3" /> Clear
        </button>
      </div>
      <ol className="space-y-1">
        {queue.map((m, i) => (
          <li key={m.id} className="flex items-center gap-1.5 rounded-md border border-border bg-card px-2 py-1 text-xs">
            <span className="w-4 shrink-0 text-center font-mono text-xs text-muted">{i + 1}</span>
            <span className="min-w-0 flex-1 truncate" title={m.text}>{m.text}</span>
            {!busy && i === 0 && (
              <button onClick={onSendNow} className="shrink-0 text-accent hover:underline" title="Send this now">
                <Send className="size-3.5" />
              </button>
            )}
            <button onClick={() => onUp(m.id)} disabled={i === 0} className="shrink-0 text-muted enabled:hover:text-foreground disabled:opacity-30" title="Move up">
              <ArrowUp className="size-3.5" />
            </button>
            <button onClick={() => onDown(m.id)} disabled={i === queue.length - 1} className="shrink-0 text-muted enabled:hover:text-foreground disabled:opacity-30" title="Move down">
              <ArrowDown className="size-3.5" />
            </button>
            <button onClick={() => onRemove(m.id)} className="shrink-0 text-muted hover:text-bad" title="Remove">
              <X className="size-3.5" />
            </button>
          </li>
        ))}
      </ol>
    </div>
  );
}

function MessageRow({
  msg,
  agent,
  onRetry,
  onContinue,
  onRegenerate,
  onEdit,
}: {
  msg: Msg;
  agent?: string;
  onRetry?: () => void;
  onContinue?: () => void;
  onRegenerate?: () => void;
  onEdit?: (text: string) => void;
}) {
  if (msg.role === "user") {
    return <UserBubble text={msg.text} onEdit={onEdit} />;
  }
  return <AssistantBubble turn={msg.turn} agent={agent} onRetry={onRetry} onContinue={onContinue} onRegenerate={onRegenerate} />;
}

// UserBubble renders one user message, with an inline edit affordance: a pencil
// (shown on hover) switches the bubble to a textarea so you can refine the ask
// and re-run from that point — Enter / Save submits, Esc / Cancel restores. The
// edit handler is absent while a run is in flight, so the pencil simply hides.
export function UserBubble({ text, onEdit }: { text: string; onEdit?: (text: string) => void }) {
  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState(text);
  const ref = useRef<HTMLTextAreaElement>(null);

  useEffect(() => {
    if (editing) {
      const el = ref.current;
      if (el) {
        el.focus();
        el.setSelectionRange(el.value.length, el.value.length);
        el.style.height = "auto";
        el.style.height = `${el.scrollHeight}px`;
      }
    }
  }, [editing]);

  function begin() {
    setDraft(text);
    setEditing(true);
  }
  function cancel() {
    setEditing(false);
    setDraft(text);
  }
  function save() {
    const t = draft.trim();
    if (!t || !onEdit) return cancel();
    setEditing(false);
    if (t !== text) onEdit(t);
  }
  function onKeyDown(e: React.KeyboardEvent<HTMLTextAreaElement>) {
    if (e.key === "Enter" && !e.shiftKey) {
      e.preventDefault();
      save();
    } else if (e.key === "Escape") {
      e.preventDefault();
      cancel();
    }
  }

  if (editing) {
    return (
      <div className="flex justify-end gap-2">
        <div className="w-full max-w-[85%] rounded-2xl rounded-br-sm border border-accent/40 bg-accent/10 p-2">
          <textarea
            ref={ref}
            value={draft}
            onChange={(e) => {
              setDraft(e.target.value);
              e.target.style.height = "auto";
              e.target.style.height = `${e.target.scrollHeight}px`;
            }}
            onKeyDown={onKeyDown}
            aria-label="Edit message"
            className="max-h-60 w-full resize-none bg-transparent text-sm text-foreground outline-none"
          />
          <div className="mt-1.5 flex items-center justify-end gap-1.5">
            <button
              onClick={cancel}
              className="rounded-md px-2 py-1 text-xs text-muted transition-colors hover:text-foreground"
            >
              Cancel
            </button>
            <button
              onClick={save}
              className="rounded-md bg-accent px-2 py-1 text-xs font-medium text-white transition-opacity hover:opacity-90"
            >
              Save & run
            </button>
          </div>
        </div>
        <div className="mt-0.5 flex size-7 shrink-0 items-center justify-center rounded-full bg-panel text-muted">
          <User className="size-4" />
        </div>
      </div>
    );
  }

  return (
    <div className="group flex items-start justify-end gap-2">
      {onEdit && (
        <button
          onClick={begin}
          title="Edit & re-run"
          className="mt-1 shrink-0 self-center text-muted opacity-0 transition-opacity hover:text-foreground group-hover:opacity-100"
        >
          <Pencil className="size-3.5" />
        </button>
      )}
      <div className="max-w-[85%] rounded-2xl rounded-br-sm bg-accent/15 px-3.5 py-2 text-sm text-foreground whitespace-pre-wrap break-words">
        {text}
      </div>
      <div className="mt-0.5 flex size-7 shrink-0 items-center justify-center rounded-full bg-panel text-muted">
        <User className="size-4" />
      </div>
    </div>
  );
}

export function AssistantBubble({
  turn,
  agent,
  onRetry,
  onContinue,
  onRegenerate,
}: {
  turn: ChatTurn;
  agent?: string;
  onRetry?: () => void;
  onContinue?: () => void;
  onRegenerate?: () => void;
}) {
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
      {/* Who's answering: a specific roster agent shows its gradient monogram
          (breathing while it streams) so the thread reflects the agent at work;
          the default house assistant keeps the spark mark. */}
      {agent ? (
        <AgentAvatar slug={agent} size={28} status={streaming ? "running" : undefined} className="mt-0.5" />
      ) : (
        <div className="mt-0.5 flex size-7 shrink-0 items-center justify-center rounded-full bg-accent/15 text-accent">
          <Sparkles className="size-4" />
        </div>
      )}
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

        {turn.steers && turn.steers.length > 0 && <SteerNote steers={turn.steers} />}

        {turn.context && turn.context.compactions.length > 0 && <CompactionNote events={turn.context.compactions} />}

        {turn.status === "error" && (
          <div className="space-y-2">
            <div className="flex items-start gap-2 rounded-lg border border-bad/40 bg-bad/10 px-3 py-2 text-sm text-bad">
              <AlertTriangle className="mt-0.5 size-4 shrink-0" />
              <span className="break-words">{turn.error || "the run failed"}</span>
            </div>
            <div className="flex items-center gap-2">
              {onContinue && (
                <button
                  onClick={onContinue}
                  title="Resume from where it stopped, keeping the work so far (use this if it hit the iteration limit)"
                  className="inline-flex items-center gap-1.5 rounded-md border border-accent/50 px-2.5 py-1 text-xs text-accent transition-colors hover:bg-accent/10"
                >
                  <ArrowRight className="size-3.5" /> Continue
                </button>
              )}
              {onRetry && (
                <button
                  onClick={onRetry}
                  title="Start this task over from scratch"
                  className="inline-flex items-center gap-1.5 rounded-md border border-border px-2.5 py-1 text-xs text-muted transition-colors hover:border-accent hover:text-accent"
                >
                  <RotateCcw className="size-3.5" /> Retry
                </button>
              )}
            </div>
          </div>
        )}

        {turn.status === "done" && (
          <div className="flex items-center gap-3">
            <TurnMeta turn={turn} />
            <ContextChip turn={turn} />
            {text && <CopyAnswer text={text} />}
            {text && <SpeakAnswer text={text} />}
            {onRegenerate && (
              <button
                onClick={onRegenerate}
                title="Regenerate this answer"
                className="inline-flex items-center gap-1 text-xs text-muted transition-colors hover:text-foreground"
              >
                <RotateCcw className="size-3" /> Regenerate
              </button>
            )}
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

// ConversationPersona is a legacy-named per-thread identity override (M711): a
// small composer control that, when set, makes runs in THIS conversation use the
// supplied identity text instead of the daemon default identity. It is not a
// durable roster agent.
export function ConversationPersona({ value, onChange }: { value: string; onChange: (v: string) => void }) {
  const [open, setOpen] = useState(false);
  const [draft, setDraft] = useState(value);
  const active = value.trim().length > 0;

  function openEditor() {
    setDraft(value);
    setOpen(true);
  }
  function save() {
    onChange(draft.trim());
    setOpen(false);
  }
  function clear() {
    onChange("");
    setDraft("");
    setOpen(false);
  }

  return (
    <div className="relative">
      <button
        onClick={open ? () => setOpen(false) : openEditor}
        title={active ? "This conversation has a custom identity override" : "Set an identity override for this conversation"}
        className={cn(
          "inline-flex items-center gap-1 rounded px-1.5 py-0.5 transition-colors hover:text-foreground",
          active ? "text-accent" : "text-muted",
        )}
      >
        <Bot className="size-3.5" />
        <span>identity{active ? "" : ": default"}</span>
        {active && <span className="size-1.5 rounded-full bg-accent" />}
      </button>
      {open && (
        <>
          <div className="fixed inset-0 z-20" onClick={() => setOpen(false)} />
          <div className="absolute bottom-full left-0 z-30 mb-1.5 w-80 rounded-lg border border-border bg-card p-2 shadow-xl shadow-black/30">
            <div className="mb-1.5 text-[11px] text-muted">
              Identity for <span className="text-foreground/80">this conversation</span> — overrides the daemon
              default identity for every run here. Leave empty to use the default identity.
            </div>
            <textarea
              value={draft}
              onChange={(e) => setDraft(e.target.value)}
              autoFocus
              spellCheck={false}
              aria-label="Conversation identity"
              placeholder="e.g. You are a senior Go reviewer. Be blunt and specific…"
              className="h-28 w-full resize-none rounded-md border border-border bg-panel p-2 font-mono text-xs text-foreground outline-none placeholder:text-muted/60 focus-visible:border-accent"
            />
            <div className="mt-1.5 flex items-center justify-end gap-1.5">
              <button
                onClick={clear}
                disabled={!active && !draft.trim()}
                className="rounded-md px-2 py-1 text-xs text-muted transition-colors hover:text-foreground disabled:opacity-40"
              >
                Clear
              </button>
              <button
                onClick={save}
                className="rounded-md bg-accent px-2 py-1 text-xs font-medium text-white transition-opacity hover:opacity-90"
              >
                Save
              </button>
            </div>
          </div>
        </>
      )}
    </div>
  );
}

// PromptLauncher makes the saved prompt library (M713) reachable anywhere in a
// chat, not just the empty state (M725): a button that opens a menu of your saved
// prompts; picking one drops its text into the composer. Hidden when you have none.
export function PromptLauncher({ onPick }: { onPick: (text: string) => void }) {
  const [prompts, setPrompts] = useState<{ title: string; text: string }[]>([]);
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);

  useEffect(() => {
    getJSON<{ prompts?: { title: string; text: string }[] }>("/api/prompts")
      .then((r) => setPrompts(r.prompts || []))
      .catch(() => {
        /* prompts are optional */
      });
  }, []);

  useEffect(() => {
    if (!open) return;
    function onDown(e: MouseEvent) {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false);
    }
    window.addEventListener("mousedown", onDown);
    return () => window.removeEventListener("mousedown", onDown);
  }, [open]);

  if (prompts.length === 0) return null;

  return (
    <div ref={ref} className="relative">
      <button
        onClick={() => setOpen((o) => !o)}
        title="Insert a saved prompt"
        aria-label="Insert a saved prompt"
        className="inline-flex items-center gap-1 rounded px-1.5 py-0.5 text-muted transition-colors hover:text-foreground"
      >
        <Sparkles className="size-3.5" />
        <span>prompts</span>
      </button>
      {open && (
        <div className="absolute bottom-full left-0 z-30 mb-1.5 max-h-64 w-64 overflow-auto rounded-lg border border-border bg-card p-1 shadow-xl shadow-black/30">
          {prompts.map((p, i) => (
            <button
              key={`${p.title}-${i}`}
              onClick={() => {
                onPick(p.text);
                setOpen(false);
              }}
              title={p.text}
              className="block w-full truncate rounded px-2 py-1.5 text-left text-xs text-foreground/90 transition-colors hover:bg-panel"
            >
              {p.title}
            </button>
          ))}
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

// SteerNote (M962) shows the operator injections this turn received mid-run — a
// forceful steer (re-prioritise) or a soft BTW note — so the human sees their
// guidance landed and how the agent was nudged.
function SteerNote({ steers }: { steers: { text: string; note: boolean }[] }) {
  return (
    <div className="space-y-1">
      {steers.map((s, i) => (
        <div
          key={i}
          className={cn(
            "flex items-start gap-1.5 rounded-md border px-2 py-1 text-xs",
            s.note ? "border-border bg-panel/50 text-muted" : "border-accent/30 bg-accent/5 text-accent",
          )}
        >
          {s.note ? <StickyNote className="mt-0.5 size-3.5 shrink-0" /> : <Forward className="mt-0.5 size-3.5 shrink-0" />}
          <span className="font-medium">{s.note ? "BTW" : "steered"}</span>
          <span className="min-w-0 flex-1 break-words text-foreground/80">{s.text}</span>
        </div>
      ))}
    </div>
  );
}

function TurnMeta({ turn }: { turn: ChatTurn }) {
  const parts: string[] = [];
  if (turn.agent) parts.push("as " + turn.agent); // who answered (M789)
  if (turn.model) parts.push(turn.model);
  if (turn.iters) parts.push(`${turn.iters} iter${turn.iters === 1 ? "" : "s"}`);
  if (turn.costMicrocents) parts.push(money(turn.costMicrocents));
  // When this exchange happened (M877) — shown for turns the store stamped; older
  // persisted turns lack `ts` and just omit it.
  if (turn.ts) parts.push(fmtTime(turn.ts));
  if (parts.length === 0) return null;
  return <div className="text-xs text-muted">{parts.join(" · ")}</div>;
}

// loadCatalog caches the /api/catalog fetch for the context chips: every turn
// needs the same model→window lookup, so one fetch serves the whole thread.
let catalogPromise: Promise<ModelCatalog> | null = null;
function loadCatalog(): Promise<ModelCatalog> {
  if (!catalogPromise) {
    catalogPromise = getJSON<ModelCatalog>("/api/catalog").catch(() => {
      catalogPromise = null; // transient failure — let a later chip retry
      return {} as ModelCatalog;
    });
  }
  return catalogPromise;
}

// barTone maps window usage to the traffic-light color the bar fills with:
// comfortable (<50%), getting full (<90%), near the limit (≥90%).
export function barTone(pctOfWindow: number): "good" | "warn" | "bad" {
  if (pctOfWindow >= 90) return "bad";
  if (pctOfWindow >= 50) return "warn";
  return "good";
}

// ContextChip is the per-turn context-window gauge (M925): a mini fill bar with
// the usage percentage (when the model's window is known from the catalog, else
// the absolute token count), plus a scissors marker when the loop had to compact.
// Clicking opens the full breakdown modal.
export function ContextChip({ turn }: { turn: ChatTurn }) {
  const [open, setOpen] = useState(false);
  const [windowTokens, setWindowTokens] = useState(0);
  const model = turn.model;
  useEffect(() => {
    if (!model) return;
    let live = true;
    loadCatalog().then((cat) => {
      if (live) setWindowTokens(findModelContext(cat, model));
    });
    return () => {
      live = false;
    };
  }, [model]);
  const c = turn.context;
  if (!c) return null;
  const used = contextTokensUsed(c);
  if (used <= 0) return null;
  const pctOfWindow = windowTokens > 0 ? Math.min(100, (used / windowTokens) * 100) : 0;
  const tone = barTone(pctOfWindow);
  return (
    <>
      <button
        onClick={() => setOpen(true)}
        title="Context window usage — click for the breakdown"
        className="inline-flex items-center gap-1.5 text-xs text-muted transition-colors hover:text-foreground"
      >
        <Gauge className="size-3" />
        {windowTokens > 0 ? (
          <>
            <span className="h-1.5 w-12 overflow-hidden rounded-full bg-panel">
              <span
                className={cn(
                  "block h-full rounded-full",
                  tone === "bad" ? "bg-bad" : tone === "warn" ? "bg-warn" : "bg-good",
                )}
                style={{ width: `${Math.max(2, pctOfWindow)}%` }}
              />
            </span>
            <span className="tabular-nums">{Math.round(pctOfWindow)}%</span>
          </>
        ) : (
          <span className="tabular-nums">{fmtCount(used)} tok</span>
        )}
        {c.compactions.length > 0 && <Scissors className="size-3 text-warn" aria-label="context was compacted" />}
      </button>
      {open && <ContextModal turn={turn} windowTokens={windowTokens} onClose={() => setOpen(false)} />}
    </>
  );
}

// The role split renders in a fixed, meaningful order (what the model reads
// first → last), each with its own shade of the accent so the stacked bar and
// the legend match without leaning on the semantic good/warn/bad colors.
const ROLE_ORDER = ["system", "user", "assistant", "tool"];
const ROLE_FILL: Record<string, string> = {
  system: "bg-accent",
  user: "bg-accent/70",
  assistant: "bg-accent/45",
  tool: "bg-accent/25",
};

// ContextModal is the context breakdown (M925): how full the window got, where
// the context came from (role split from the last llm.request), what the
// provider actually billed (token usage incl. cache hits), and what compaction
// elided. Everything it shows is already folded into the turn — no extra fetch.
export function ContextModal({
  turn,
  windowTokens,
  onClose,
}: {
  turn: ChatTurn;
  windowTokens: number;
  onClose: () => void;
}) {
  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if (e.key === "Escape") onClose();
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose]);

  const c = turn.context;
  if (!c) return null;
  const used = contextTokensUsed(c);
  const pctOfWindow = windowTokens > 0 ? Math.min(100, (used / windowTokens) * 100) : 0;
  const tone = barTone(pctOfWindow);
  const byRole = c.byRole || {};
  // Known roles first (reading order), then anything unexpected the loop reported.
  const roles = [
    ...ROLE_ORDER.filter((r) => (byRole[r] || 0) > 0),
    ...Object.keys(byRole).filter((r) => !ROLE_ORDER.includes(r) && byRole[r] > 0),
  ];
  const totalChars = roles.reduce((a, r) => a + byRole[r], 0) || c.chars;
  const section = "px-3 pt-2.5 pb-0.5 text-xs font-semibold uppercase tracking-wider text-muted";

  return (
    <div
      className="modal-overlay fixed inset-0 z-[110] flex items-start justify-center bg-black/50 p-4 pt-[10vh]"
      onClick={onClose}
    >
      <div
        className="modal-in flex max-h-[75vh] w-full max-w-md flex-col overflow-hidden rounded-xl border border-border bg-card shadow-xl shadow-black/30"
        onClick={(e) => e.stopPropagation()}
        role="dialog"
        aria-modal="true"
        aria-label="Context window breakdown"
      >
        <div className="flex items-center gap-2 border-b border-border px-3 py-2.5">
          <Gauge className="size-4 shrink-0 text-accent" />
          <span className="text-sm font-semibold">Context window</span>
          {turn.model && <span className="truncate font-mono text-xs text-muted">{turn.model}</span>}
          <button onClick={onClose} className="ml-auto shrink-0 text-muted hover:text-foreground" title="Close">
            <X className="size-4" />
          </button>
        </div>

        <div className="min-h-0 flex-1 overflow-auto pb-3">
          {/* Headline: how full the window got on the last call. */}
          <div className="px-3 pt-3">
            <div className="flex items-baseline justify-between text-sm">
              <span className="font-medium tabular-nums">
                {fmtCount(used)} tokens
                {windowTokens > 0 && <span className="text-muted"> of {fmtContext(windowTokens)} window</span>}
              </span>
              {windowTokens > 0 && (
                <span
                  className={cn(
                    "tabular-nums text-xs font-semibold",
                    tone === "bad" ? "text-bad" : tone === "warn" ? "text-warn" : "text-good",
                  )}
                >
                  {Math.round(pctOfWindow)}%
                </span>
              )}
            </div>
            {windowTokens > 0 ? (
              <div className="mt-1.5 h-2 overflow-hidden rounded-full bg-panel">
                <div
                  className={cn(
                    "h-full rounded-full",
                    tone === "bad" ? "bg-bad" : tone === "warn" ? "bg-warn" : "bg-good",
                  )}
                  style={{ width: `${Math.max(2, pctOfWindow)}%` }}
                />
              </div>
            ) : (
              <p className="mt-1 text-xs text-muted">window size unknown — model not in the catalog</p>
            )}
          </div>

          {/* Composition: where the last request's context came from. */}
          {roles.length > 0 && (
            <>
              <div className={section}>Composition</div>
              <div className="px-3">
                <div className="flex h-2 overflow-hidden rounded-full bg-panel">
                  {roles.map((r) => (
                    <div
                      key={r}
                      className={ROLE_FILL[r] || "bg-accent/25"}
                      style={{ width: `${(byRole[r] / totalChars) * 100}%` }}
                      title={r}
                    />
                  ))}
                </div>
                <div className="mt-1.5 space-y-1">
                  {roles.map((r) => (
                    <div key={r} className="flex items-center gap-2 text-xs">
                      <span className={cn("size-2 shrink-0 rounded-full", ROLE_FILL[r] || "bg-accent/25")} />
                      <span className="w-20 capitalize">{r}</span>
                      <span className="tabular-nums text-muted">
                        ≈{fmtCount(byRole[r] / CHARS_PER_TOKEN)} tok · {fmtCount(byRole[r])} chars
                      </span>
                      <span className="ml-auto tabular-nums text-muted">{Math.round((byRole[r] / totalChars) * 100)}%</span>
                    </div>
                  ))}
                </div>
              </div>
            </>
          )}

          {/* Provider-billed tokens, across every iteration of the run. */}
          {(c.inputTokens > 0 || c.outputTokens > 0) && (
            <>
              <div className={section}>
                Tokens billed{turn.iters > 1 ? ` · ${turn.iters} iterations` : ""}
              </div>
              <div className="space-y-1 px-3 text-xs">
                <div className="flex justify-between">
                  <span>Input</span>
                  <span className="tabular-nums text-muted">
                    {fmtCount(c.inputTokens)}
                    {c.cachedTokens > 0 && <span className="text-good"> · {fmtCount(c.cachedTokens)} cached</span>}
                  </span>
                </div>
                <div className="flex justify-between">
                  <span>Output</span>
                  <span className="tabular-nums text-muted">{fmtCount(c.outputTokens)}</span>
                </div>
                {c.cacheWriteTokens > 0 && (
                  <div className="flex justify-between">
                    <span>Cache write</span>
                    <span className="tabular-nums text-muted">{fmtCount(c.cacheWriteTokens)}</span>
                  </div>
                )}
              </div>
            </>
          )}

          {/* Compaction: what the loop elided to stay inside the budget. */}
          <div className={section}>Compaction</div>
          <div className="px-3 text-xs">
            {c.compactions.length === 0 ? (
              <p className="text-muted">None needed — the context fit the budget.</p>
            ) : (
              <div className="space-y-1">
                {c.compactions.map((e, i) => (
                  <div key={i} className="flex items-center gap-1.5">
                    <Scissors className="size-3 shrink-0 text-warn" />
                    <span>
                      {e.elided} tool output{e.elided === 1 ? "" : "s"} elided · reclaimed {fmtCount(e.reclaimedChars)}{" "}
                      chars
                      {e.beforeChars > 0 && (
                        <span className="text-muted">
                          {" "}
                          ({fmtCount(e.beforeChars)} → {fmtCount(e.afterChars)})
                        </span>
                      )}
                    </span>
                  </div>
                ))}
              </div>
            )}
          </div>

          <p className="px-3 pt-3 text-xs leading-snug text-muted/70">
            Composition is measured in characters (≈{CHARS_PER_TOKEN} chars/token estimate) and covers the system
            prompt + messages — tool definitions add to the billed input on top. Token totals are provider-reported;
            the gauge uses the last call's real prompt tokens when available.
          </p>
        </div>
      </div>
    </div>
  );
}

// SummaryDivider marks the fold point in a long thread (M925): everything above
// it has been condensed into one briefing that rides with each run instead of
// the raw turns (which would otherwise fall off the history window). Click to
// read exactly what the agent now knows about the older conversation.
export function SummaryDivider({ summary }: { summary: HistorySummary }) {
  const [open, setOpen] = useState(false);
  return (
    <div className="mb-4">
      <button
        onClick={() => setOpen((o) => !o)}
        title="Older messages were condensed into a briefing the agent carries — click to read it"
        className="flex w-full items-center gap-2 text-[11px] text-muted transition-colors hover:text-foreground"
      >
        <span className="h-px flex-1 bg-border" />
        <Scissors className="size-3 shrink-0" />
        <span>
          {summary.upto} older message{summary.upto === 1 ? "" : "s"} summarized for the agent
        </span>
        {open ? <ChevronDown className="size-3 shrink-0" /> : <ChevronRight className="size-3 shrink-0" />}
        <span className="h-px flex-1 bg-border" />
      </button>
      {open && (
        <div className="mx-6 mt-2 whitespace-pre-wrap break-words rounded-lg border border-border bg-panel/40 px-3 py-2 text-xs leading-relaxed text-muted">
          {summary.text}
        </div>
      )}
    </div>
  );
}

// CompactionNote surfaces that the loop trimmed its own context mid-run (M925) —
// the same visibility rule as FallbackNote: when the run quietly did something
// that changes what the model saw, say so right in the thread.
export function CompactionNote({ events }: { events: TurnCompaction[] }) {
  const elided = events.reduce((a, e) => a + e.elided, 0);
  const reclaimed = events.reduce((a, e) => a + e.reclaimedChars, 0);
  return (
    <div className="flex flex-wrap items-center gap-1.5 rounded-md border border-border bg-panel/40 px-2 py-1 text-xs text-muted">
      <Scissors className="size-3.5 shrink-0" />
      <span className="font-medium text-foreground/80">context compacted</span>
      <span>
        {elided} old tool output{elided === 1 ? "" : "s"} elided · {fmtCount(reclaimed)} chars reclaimed
      </span>
    </div>
  );
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
      <span className="inline-flex items-center gap-1 text-xs text-muted">
        <Brain className="size-3 text-accent" /> learned
      </span>
      {items.map((m) => (
        <span
          key={m.id}
          className="inline-flex items-center gap-1 rounded-full border border-border bg-panel px-2 py-0.5 text-xs"
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
              <div className="px-2.5 pt-1.5 text-xs font-semibold uppercase tracking-wider text-muted">Arguments</div>
              <ToolOutput text={c.input} />
            </div>
          )}
          {c.output && (
            <div>
              <div className="px-2.5 pt-1.5 text-xs font-semibold uppercase tracking-wider text-muted">
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
