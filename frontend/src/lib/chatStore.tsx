import { createContext, useContext, useEffect, useRef, useState } from "react";
import { getJSON, postAction } from "@/lib/api";
import { useEvents, type AgentEvent } from "@/lib/events";
import {
  streamRun,
  foldChatFrame,
  newTurn,
  buildHistory,
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
  startConversation,
  deleteConversation,
  renameConversation,
  togglePinned,
  type Msg,
  type Store,
} from "@/lib/conversations";

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
  /** This conversation's persona override (system prompt), "" when it uses the
   *  daemon's global persona. Set it to make a single thread act as something else. */
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
  /** Memories the daemon recorded during the run with this correlation id. */
  learnedFor: (corr?: string) => LearnedMem[];
  /** Forget one learned memory (tombstones it in the store + drops the chip). */
  forgetLearned: (corr: string, id: string) => Promise<void>;
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
    try {
      const persona = activePersona(store).trim();
      await streamRun(
        {
          intent,
          model: activeConvModel(store).trim() || undefined, // per-conversation model (M712)
          agent: activeConvAgent(store).trim() || undefined, // per-conversation agent (M789)
          history: history.length ? history : undefined,
          system: persona || undefined, // per-conversation persona override (M711)
        },
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

  function send(intent: string, context?: string) {
    const t = intent.trim();
    if (!t || busy) return;
    const history = buildHistory(messages);
    // The conversation records the clean intent; the run receives the attachment
    // context (if any) prepended, so the user's bubble stays uncluttered.
    setMessages((m) => [...m, { role: "user", text: t }, { role: "assistant", turn: freshTurn() }]);
    void streamIntent(context ? `${context}\n\n---\n\n${t}` : t, history);
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
    const history = buildHistory(messages.slice(0, userIdx));
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
    const history = buildHistory(messages); // includes the partial answer
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
    const history = buildHistory(messages.slice(0, index));
    setMessages((prev) => [...prev.slice(0, index), { role: "user", text: t }, { role: "assistant", turn: freshTurn() }]);
    void streamIntent(t, history);
  }

  function setConversationPersona(persona: string) {
    setStore((s) => withActivePersona(s, persona.trim(), Date.now()));
  }

  function stop() {
    abortRef.current?.abort();
  }

  function newChat() {
    if (busy) abortRef.current?.abort();
    setStore((s) => startConversation(s, genId, Date.now()));
  }

  function selectConversation(id: string) {
    if (id === store.activeId) return;
    if (busy) abortRef.current?.abort();
    setStore((s) => ({ ...s, activeId: id }));
  }

  function removeConversation(id: string) {
    if (busy && id === store.activeId) abortRef.current?.abort();
    setStore((s) => deleteConversation(s, id, genId, Date.now()));
  }

  function renameConv(id: string, title: string) {
    setStore((s) => renameConversation(s, id, title, Date.now()));
  }

  function togglePin(id: string) {
    setStore((s) => togglePinned(s, id));
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
    stop,
    newChat,
    selectConversation,
    removeConversation,
    renameConversation: renameConv,
    togglePin,
    learnedFor,
    forgetLearned,
  };
  return <ChatCtx.Provider value={engine}>{children}</ChatCtx.Provider>;
}
