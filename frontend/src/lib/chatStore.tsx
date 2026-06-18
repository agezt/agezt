import { createContext, useContext, useEffect, useRef, useState } from "react";
import { getJSON, postAction, postJSON } from "@/lib/api";
import { useEvents, type AgentEvent } from "@/lib/events";
import {
  streamRun,
  foldChatFrame,
  newTurn,
  buildHistory,
  buildHistoryWithSummary,
  summaryFoldRange,
  summaryBriefingTurn,
  type ChatTurn,
  type ChatHistoryTurn,
} from "@/lib/chat";
import {
  loadStore,
  saveStore,
  activeMessages,
  withActiveMessages,
  activePersona,
  withActivePersona,
  activeConvModel,
  withActiveConvModel,
  activeConvAgent,
  withActiveConvAgent,
  activeSummary,
  withActiveSummary,
  activeQueue,
  withActiveQueue,
  startConversation,
  deleteConversation,
  renameConversation,
  togglePinned,
  type HistorySummary,
  type Msg,
  type Store,
} from "@/lib/conversations";
import { addQueued, removeQueued as removeFromQueue, moveQueued, dequeueFront, type QueuedMsg } from "@/lib/queue";

// genId: a unique conversation id (localhost is a secure context, so
// crypto.randomUUID is available; a timestamp+random fallback covers the rest).
function genId(): string {
  if (typeof crypto !== "undefined" && crypto.randomUUID) return crypto.randomUUID();
  return "c-" + Date.now().toString(36) + Math.random().toString(36).slice(2, 8);
}

// freshTurn is newTurn() stamped with the current wall-clock time, so the chat
// can show when each exchange happened. newTurn() itself stays time-free (it's
// the pure reducer's seed); only turns the store creates on a real send get a ts.
function freshTurn(): ChatTurn {
  return { ...newTurn(), ts: Date.now() };
}

// A memory the daemon recorded during a run (from a memory.written event). The
// Chat shows these under the turn so you can see what the agent learned — and
// forget any that aren't worth keeping.
export interface LearnedMem {
  id: string;
  type: string;
  subject: string;
  action: string; // write | reinforce | revive
}

// collectLearned folds one firehose event into the per-correlation learned map.
// Only memory.written events with a real id are kept; duplicates (same id under
// the same run) are ignored. Pure, so it's unit-testable without the provider.
export function collectLearned(
  prev: Record<string, LearnedMem[]>,
  e: AgentEvent,
): Record<string, LearnedMem[]> {
  if (e.kind !== "memory.written" || !e.correlation_id) return prev;
  const p = e.payload || {};
  const id = String(p.id || "");
  if (!id) return prev; // distill_failed and other id-less notes are skipped
  const corr = e.correlation_id;
  const list = prev[corr] || [];
  if (list.some((x) => x.id === id)) return prev;
  const mem: LearnedMem = {
    id,
    type: String(p.type || "FACT"),
    subject: String(p.subject || ""),
    action: String(p.action || "write"),
  };
  return { ...prev, [corr]: [...list, mem] };
}

// The chat engine, lifted above the view router so a run keeps streaming (and the
// store keeps folding + persisting) even when you navigate away from the Chat
// view — and so a global mini-chat can share the exact same conversation. The
// composer's text stays view-local; send()/retry() take the intent directly.
export interface ChatEngine {
  store: Store;
  messages: Msg[];
  busy: boolean;
  model: string; // "" = daemon default
  setModel: (m: string) => void;
  agent: string; // "" = the daemon's default identity; else a roster slug (M789)
  setAgent: (slug: string) => void;
  activeModel: string; // the daemon's configured default (a hint)
  /** Send a message. `context`, when given, is prepended to the run intent only —
   *  the conversation still shows the clean `intent` as the user's message. */
  send: (intent: string, context?: string) => void;
  retry: () => void;
  /** Resume an errored/maxed-out turn: keep the partial answer as context and ask
   *  the model to finish, instead of restarting the whole task like retry(). */
  continueRun: () => void;
  /** This conversation's legacy-named identity override, "" when it uses the
   *  daemon default identity. Set it to make a single thread act as something
   *  else without creating a durable roster agent. */
  conversationPersona: string;
  setConversationPersona: (persona: string) => void;
  /** Edit the user message at `index` and re-run from there: replace its text,
   *  drop every later message, and stream a fresh answer with the prior history. */
  editAndResend: (index: number, text: string) => void;
  stop: () => void;
  newChat: () => void;
  selectConversation: (id: string) => void;
  removeConversation: (id: string) => void;
  /** Rename a conversation; a blank name restores the message-derived title. */
  renameConversation: (id: string, title: string) => void;
  /** Toggle a conversation's pinned flag (pinned threads sort to the top). */
  togglePin: (id: string) => void;
  /** The active conversation's history briefing (M925) — older turns folded
   *  into one summary so long threads don't silently lose their start. */
  historySummary?: HistorySummary;
  /** Memories the daemon recorded during the run with this correlation id. */
  learnedFor: (corr?: string) => LearnedMem[];
  /** Forget one learned memory (tombstones it in the store + drops the chip). */
  forgetLearned: (corr: string, id: string) => Promise<void>;
  /** Correlation id of the run currently streaming, or null — the steer target. */
  activeCorr: string | null;
  /** Inject a directive into the running run: "steer" re-prioritises, "note" is a
   *  soft BTW the agent reads without abandoning its task (M962). */
  steer: (directive: string, mode: "steer" | "note") => Promise<void>;
  /** The active thread's pending message queue (M962). */
  queue: QueuedMsg[];
  /** Append a message to the queue (auto-sent when the current run finishes). */
  enqueue: (text: string) => void;
  /** Remove one queued message by id. */
  removeQueued: (id: string) => void;
  /** Move a queued message up (-1) or down (+1). */
  reorderQueued: (id: string, dir: -1 | 1) => void;
  /** Drop every queued message. */
  clearQueue: () => void;
  /** Send the front of the queue right now (when idle). */
  sendQueuedNow: () => void;
}

const ChatCtx = createContext<ChatEngine | null>(null);

export function useChat(): ChatEngine {
  const c = useContext(ChatCtx);
  if (!c) throw new Error("useChat must be used within <ChatProvider>");
  return c;
}

export function ChatProvider({ children }: { children: React.ReactNode }) {
  const { subscribe } = useEvents();
  const [store, setStore] = useState<Store>(() => loadStore(genId, Date.now()));
  // The model is a per-conversation override (M712): each thread remembers its own
  // (derived from the store, so switching threads switches the model). "" = default.
  const model = activeConvModel(store);
  const setModel = (m: string) => setStore((s) => withActiveConvModel(s, m, Date.now()));
  // The agent is a per-conversation identity (M789): the thread runs AS the
  // picked roster agent — soul/model chain/memory scope/budget (M783).
  const agent = activeConvAgent(store);
  const setAgent = (a: string) => setStore((s) => withActiveConvAgent(s, a, Date.now()));
  const [activeModel, setActiveModel] = useState("");
  const [busy, setBusy] = useState(false);
  const [learned, setLearned] = useState<Record<string, LearnedMem[]>>({});
  const abortRef = useRef<AbortController | null>(null);
  // The correlation id of the run currently streaming, captured from its frames
  // (M907). Stopping the browser stream alone leaves the daemon running the loop
  // headless (cancel-on-disconnect is off by default), so Stop must ALSO cancel
  // the run server-side by this id — otherwise it keeps burning budget unseen.
  const activeCorrRef = useRef<string | null>(null);
  // Same correlation, mirrored into state so the composer can show Steer/BTW
  // controls the moment a run is addressable (M962).
  const [activeCorr, setActiveCorr] = useState<string | null>(null);

  const messages = activeMessages(store);
  const setMessages = (updater: Msg[] | ((prev: Msg[]) => Msg[])) => {
    setStore((s) => {
      const prev = activeMessages(s);
      const next = typeof updater === "function" ? (updater as (p: Msg[]) => Msg[])(prev) : updater;
      return withActiveMessages(s, next, Date.now());
    });
  };

  // The daemon's default model — shown so you always know what you're talking to.
  useEffect(() => {
    getJSON<{ model?: string }>("/api/config")
      .then((c) => setActiveModel(String(c.model || "")))
      .catch(() => {
        /* config momentarily unavailable — the picker just shows "default" */
      });
  }, []);

  // Persist the store (skip while a turn is mid-stream — a half-folded turn would
  // restore as "interrupted"; the settled store is saved on completion).
  useEffect(() => {
    if (busy) return;
    saveStore(store);
  }, [store, busy]);

  // Auto-flush the queue (M962): when a run finishes (busy true → false) and the
  // active thread has queued messages, pop the front and send it. Gated on the
  // busy→idle transition (a prevBusy ref) so a persisted queue does NOT
  // auto-fire on reload — it waits for the next completed run, or a manual send.
  const prevBusyRef = useRef(false);
  useEffect(() => {
    const wasBusy = prevBusyRef.current;
    prevBusyRef.current = busy;
    if (!wasBusy || busy) return; // only on the busy→idle edge
    const q = activeQueue(store);
    if (q.length === 0) return;
    const { front, rest } = dequeueFront(q);
    if (!front) return;
    setStore((s) => withActiveQueue(s, rest, Date.now()));
    send(front.text);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [busy]);

  // Collect every memory the daemon records (memory.written) off the global
  // firehose, bucketed by the run's correlation id — so a chat turn can show
  // exactly what it taught the agent (distillation fires post-run, so these may
  // land after the turn is already done; the global subscription catches them
  // regardless of which view is open).
  useEffect(() => subscribe((e) => setLearned((prev) => collectLearned(prev, e))), [subscribe]);

  function learnedFor(corr?: string): LearnedMem[] {
    return (corr && learned[corr]) || [];
  }
  async function forgetLearned(corr: string, id: string) {
    await postAction("/api/memory/forget", { id });
    setLearned((prev) => ({ ...prev, [corr]: (prev[corr] || []).filter((x) => x.id !== id) }));
  }

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
  // the trailing assistant turn (which the caller must have just appended). The
  // AbortController lives here (above the router), so leaving the Chat view does
  // NOT abort the run — it keeps folding into the store and persists on finish.
  async function streamIntent(intent: string, history: ChatHistoryTurn[]) {
    setBusy(true);
    const ctrl = new AbortController();
    abortRef.current = ctrl;
    activeCorrRef.current = null;
    try {
      const persona = activePersona(store).trim();
      await streamRun(
        {
          intent,
          model: activeConvModel(store).trim() || undefined, // per-conversation model (M712)
          agent: activeConvAgent(store).trim() || undefined, // per-conversation agent (M789)
          history: history.length ? history : undefined,
          system: persona || undefined, // per-conversation identity override (M711)
        },
        (f) => {
          // Capture the run's correlation id from the first frame that carries
          // it, so Stop can cancel the run server-side (M907).
          if (f.correlation_id && !activeCorrRef.current) {
            activeCorrRef.current = f.correlation_id;
            setActiveCorr(f.correlation_id);
          }
          updateLastTurn((t) => foldChatFrame(t, f));
        },
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
      activeCorrRef.current = null;
      setActiveCorr(null);
      setBusy(false);
    }
  }

  // steer injects an operator directive into the run currently streaming (M962):
  // mode "steer" re-prioritises (the agent breaks at its next safe boundary and
  // follows it), mode "note" is a soft "BTW" the agent reads but doesn't abandon
  // its task for. No-op when nothing is running. Throws on transport error so the
  // caller can surface it.
  async function steer(directive: string, mode: "steer" | "note") {
    const corr = activeCorrRef.current;
    const d = directive.trim();
    if (!corr || !d) return;
    await postAction("/api/run/steer", { correlation: corr, directive: d, mode });
  }

  // abortActiveRun tears down the browser stream AND cancels the run on the
  // daemon (M907). Aborting the fetch alone only stops the UI folding frames;
  // because cancel-on-disconnect is off by default, the governed loop would run
  // on headless and keep spending budget. Cancelling by correlation id (the same
  // targeted cancel `agt` uses) actually stops the work. Best-effort — a failed
  // cancel must not throw into the caller.
  function abortActiveRun() {
    const corr = activeCorrRef.current;
    abortRef.current?.abort();
    if (corr) void postAction("/api/cancel_run", { correlation: corr }).catch(() => {});
  }

  // ensureSummary folds older history into one briefing when the thread has
  // outgrown the window (M925): prior briefing (if any) + the messages between
  // the old fold point and the keep-tail go through one bounded summarize call.
  // Best-effort — on failure the send just rides the windowed history as before.
  async function ensureSummary(msgs: Msg[]): Promise<HistorySummary | undefined> {
    const cur = activeSummary(store);
    const range = summaryFoldRange(msgs.length, cur?.upto || 0);
    if (!range) return cur;
    const fold = buildHistory(msgs.slice(range.from, range.to));
    if (fold.length === 0) return cur;
    const turns: ChatHistoryTurn[] = cur?.text.trim() ? [summaryBriefingTurn(cur.text), ...fold] : fold;
    try {
      const r = await postJSON<{ summary?: string }>("/api/chat/summarize", { turns });
      const text = String(r.summary || "").trim();
      if (!text) return cur;
      const next: HistorySummary = { text, upto: range.to };
      setStore((s) => withActiveSummary(s, next, Date.now()));
      return next;
    } catch {
      return cur; // summarizer unavailable — fall back to the windowed history
    }
  }

  function send(intent: string, context?: string) {
    const t = intent.trim();
    if (!t || busy) return;
    const msgs = messages;
    // The conversation records the clean intent; the run receives the attachment
    // context (if any) prepended, so the user's bubble stays uncluttered.
    setMessages((m) => [...m, { role: "user", text: t }, { role: "assistant", turn: freshTurn() }]);
    // Busy now, not when streamIntent starts — the summarize call below awaits
    // first, and a double-send must not slip through that gap.
    setBusy(true);
    void (async () => {
      const summary = await ensureSummary(msgs);
      await streamIntent(context ? `${context}\n\n---\n\n${t}` : t, buildHistoryWithSummary(msgs, summary));
    })();
  }

  // retry re-runs the most recent user intent after a failed turn: drop the
  // errored assistant turn, re-append a fresh one, stream again with the same
  // prior history — so a transient failure is one click to recover from.
  function retry() {
    if (busy) return;
    let userIdx = -1;
    for (let i = messages.length - 1; i >= 0; i--) {
      if (messages[i].role === "user") {
        userIdx = i;
        break;
      }
    }
    if (userIdx < 0) return;
    const intent = (messages[userIdx] as Extract<Msg, { role: "user" }>).text;
    const history = buildHistoryWithSummary(messages.slice(0, userIdx), activeSummary(store));
    setMessages((prev) => [...prev.slice(0, userIdx + 1), { role: "assistant", turn: freshTurn() }]);
    void streamIntent(intent, history);
  }

  // continueRun resumes the trailing assistant turn that stopped (e.g. hit the
  // max-iteration cap) WITHOUT restarting: it keeps the partial answer as history
  // and appends a fresh turn that asks the model to finish from where it left off.
  // Unlike retry (drop the failed turn, re-run the original ask), this preserves
  // the work already done.
  function continueRun() {
    if (busy) return;
    const last = messages[messages.length - 1];
    if (!last || last.role !== "assistant") return;
    const history = buildHistoryWithSummary(messages, activeSummary(store)); // includes the partial answer
    setMessages((prev) => [...prev, { role: "assistant", turn: freshTurn() }]);
    void streamIntent(
      "Continue from where you stopped and finish the task. Do not repeat work already completed.",
      history,
    );
  }

  // editAndResend rewrites a past user message and re-runs the conversation from
  // that point: drop everything after the edited message (the old answer and any
  // later turns no longer apply), then stream a fresh turn with the history that
  // preceded it. The common "fix the ask without retyping the rest" affordance.
  function editAndResend(index: number, text: string) {
    if (busy) return;
    const t = text.trim();
    if (!t) return;
    const msg = messages[index];
    if (!msg || msg.role !== "user") return;
    // Editing a message the briefing covered makes it stale (it summarizes
    // turns that no longer exist) — drop it; a later send re-folds if needed.
    const cur = activeSummary(store);
    if (cur && index < cur.upto) setStore((s) => withActiveSummary(s, undefined, Date.now()));
    const history = buildHistoryWithSummary(messages.slice(0, index), cur && index < cur.upto ? undefined : cur);
    setMessages((prev) => [...prev.slice(0, index), { role: "user", text: t }, { role: "assistant", turn: freshTurn() }]);
    void streamIntent(t, history);
  }

  function setConversationPersona(persona: string) {
    setStore((s) => withActivePersona(s, persona.trim(), Date.now()));
  }

  function stop() {
    abortActiveRun();
  }

  function newChat() {
    if (busy) abortActiveRun();
    setStore((s) => startConversation(s, genId, Date.now()));
  }

  function selectConversation(id: string) {
    if (id === store.activeId) return;
    if (busy) abortActiveRun();
    setStore((s) => ({ ...s, activeId: id }));
  }

  function removeConversation(id: string) {
    if (busy && id === store.activeId) abortActiveRun();
    setStore((s) => deleteConversation(s, id, genId, Date.now()));
  }

  function renameConv(id: string, title: string) {
    setStore((s) => renameConversation(s, id, title, Date.now()));
  }

  function togglePin(id: string) {
    setStore((s) => togglePinned(s, id));
  }

  // ── Message queue (M962) ──────────────────────────────────────────────────
  function enqueue(text: string) {
    const t = text.trim();
    if (!t) return;
    setStore((s) => withActiveQueue(s, addQueued(activeQueue(s), t, genId()), Date.now()));
  }
  function removeQueued(id: string) {
    setStore((s) => withActiveQueue(s, removeFromQueue(activeQueue(s), id), Date.now()));
  }
  function reorderQueued(id: string, dir: -1 | 1) {
    setStore((s) => withActiveQueue(s, moveQueued(activeQueue(s), id, dir), Date.now()));
  }
  function clearQueue() {
    setStore((s) => withActiveQueue(s, [], Date.now()));
  }
  // sendQueuedNow flushes the front of the queue immediately (used when idle, so
  // the operator doesn't have to wait for a run to drain it).
  function sendQueuedNow() {
    if (busy) return;
    const { front, rest } = dequeueFront(activeQueue(store));
    if (!front) return;
    setStore((s) => withActiveQueue(s, rest, Date.now()));
    send(front.text);
  }

  const engine: ChatEngine = {
    store,
    messages,
    busy,
    model,
    setModel,
    agent,
    setAgent,
    activeModel,
    send,
    retry,
    continueRun,
    editAndResend,
    conversationPersona: activePersona(store),
    setConversationPersona,
    historySummary: activeSummary(store),
    stop,
    newChat,
    selectConversation,
    removeConversation,
    renameConversation: renameConv,
    togglePin,
    learnedFor,
    forgetLearned,
    activeCorr,
    steer,
    queue: activeQueue(store),
    enqueue,
    removeQueued,
    reorderQueued,
    clearQueue,
    sendQueuedNow,
  };
  return <ChatCtx.Provider value={engine}>{children}</ChatCtx.Provider>;
}
