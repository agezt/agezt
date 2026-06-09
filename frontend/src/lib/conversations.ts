import type { ChatTurn } from "@/lib/chat";

// A chat message: a user prompt, or an assistant turn (the folded ChatTurn).
export interface UserMsg {
  role: "user";
  text: string;
}
export interface BotMsg {
  role: "assistant";
  turn: ChatTurn;
}
export type Msg = UserMsg | BotMsg;

// One named conversation thread. Multiple are kept so the Chat view can list
// past conversations (ChatGPT-style) and switch between them.
export interface Conversation {
  id: string;
  title: string;
  messages: Msg[];
  updatedAt: number;
  // Optional per-conversation persona (system-prompt override, M711): when set,
  // every run in THIS thread uses it instead of the daemon's global persona.
  persona?: string;
  // Optional per-conversation model (M712): the model id this thread runs on
  // ("" / undefined = the daemon default). Each thread remembers its own.
  model?: string;
}

export interface Store {
  conversations: Conversation[];
  activeId: string;
}

const STORE_KEY = "agezt.chat.store.v2";
const LEGACY_THREAD_KEY = "agezt.chat.thread.v1"; // single-thread persistence (M600)

// deriveTitle names a conversation from its first user message (trimmed, single
// line, capped) — or "New chat" when empty.
export function deriveTitle(messages: Msg[]): string {
  const firstUser = messages.find((m): m is UserMsg => m.role === "user");
  const t = firstUser ? firstUser.text.trim().replace(/\s+/g, " ") : "";
  if (!t) return "New chat";
  return t.length > 40 ? t.slice(0, 40) + "…" : t;
}

// normalizeMessages repairs a thread restored from storage: a turn that was
// mid-stream when the page closed can't resume, so it's marked interrupted
// rather than left as a spinner that never resolves.
export function normalizeMessages(messages: Msg[]): Msg[] {
  if (!Array.isArray(messages)) return [];
  return messages.map((m) =>
    m.role === "assistant" && m.turn?.status === "streaming"
      ? { role: "assistant", turn: { ...m.turn, status: "error", error: m.turn.error || "interrupted" } }
      : m,
  );
}

export function newConversation(id: string, now: number): Conversation {
  return { id, title: "New chat", messages: [], updatedAt: now };
}

// loadStore reads the multi-conversation store, migrating a legacy single-thread
// (M600) into one conversation if found, and always returns a valid store with
// at least one conversation and an activeId that exists. genId/now are injected
// so the loader stays deterministic under test.
export function loadStore(genId: () => string, now: number): Store {
  try {
    const raw = localStorage.getItem(STORE_KEY);
    if (raw) {
      const s = JSON.parse(raw) as Store;
      if (s && Array.isArray(s.conversations) && s.conversations.length > 0) {
        for (const c of s.conversations) c.messages = normalizeMessages(c.messages);
        if (!s.conversations.some((c) => c.id === s.activeId)) s.activeId = s.conversations[0].id;
        return s;
      }
    }
    const legacy = localStorage.getItem(LEGACY_THREAD_KEY);
    if (legacy) {
      const messages = normalizeMessages(JSON.parse(legacy) as Msg[]);
      const id = genId();
      return { conversations: [{ id, title: deriveTitle(messages), messages, updatedAt: now }], activeId: id };
    }
  } catch {
    /* corrupt storage — fall through to a fresh store */
  }
  const id = genId();
  return { conversations: [newConversation(id, now)], activeId: id };
}

export function saveStore(store: Store): void {
  try {
    localStorage.setItem(STORE_KEY, JSON.stringify(store));
    localStorage.removeItem(LEGACY_THREAD_KEY); // superseded by the v2 store
  } catch {
    /* storage full/unavailable — history is best-effort */
  }
}

// activeMessages returns the active conversation's messages (empty if none).
export function activeMessages(store: Store): Msg[] {
  return store.conversations.find((c) => c.id === store.activeId)?.messages ?? [];
}

// activePersona returns the active conversation's persona override ("" if none).
export function activePersona(store: Store): string {
  return store.conversations.find((c) => c.id === store.activeId)?.persona ?? "";
}

// withActivePersona returns a new store with the active conversation's persona
// override set (empty string clears it). Pure — safe for React state.
export function withActivePersona(store: Store, persona: string, now: number): Store {
  const p = persona.trim();
  return {
    ...store,
    conversations: store.conversations.map((c) =>
      c.id === store.activeId ? { ...c, persona: p || undefined, updatedAt: now } : c,
    ),
  };
}

// activeConvModel returns the active conversation's model override ("" if none).
export function activeConvModel(store: Store): string {
  return store.conversations.find((c) => c.id === store.activeId)?.model ?? "";
}

// withActiveConvModel returns a new store with the active conversation's model
// override set (empty string clears it → daemon default). Pure.
export function withActiveConvModel(store: Store, model: string, now: number): Store {
  const m = model.trim();
  return {
    ...store,
    conversations: store.conversations.map((c) =>
      c.id === store.activeId ? { ...c, model: m || undefined, updatedAt: now } : c,
    ),
  };
}

// withActiveMessages returns a new store with the active conversation's messages
// replaced (and its title/updatedAt refreshed). Pure — safe for React state.
export function withActiveMessages(store: Store, messages: Msg[], now: number): Store {
  return {
    ...store,
    conversations: store.conversations.map((c) =>
      c.id === store.activeId
        ? { ...c, messages, title: c.title === "New chat" ? deriveTitle(messages) : c.title, updatedAt: now }
        : c,
    ),
  };
}

// renameConversation sets a conversation's title to a manual value (trimmed). A
// blank title falls back to one derived from the messages, so clearing a name
// restores the auto-title rather than leaving it empty. Because the result is no
// longer "New chat", later messages won't auto-rename it.
export function renameConversation(store: Store, id: string, title: string, now: number): Store {
  return {
    ...store,
    conversations: store.conversations.map((c) =>
      c.id === id ? { ...c, title: title.trim() || deriveTitle(c.messages), updatedAt: now } : c,
    ),
  };
}

// startConversation adds a fresh empty conversation and makes it active. If the
// current active conversation is already empty, it's reused (no pile-up of blank
// "New chat" entries).
export function startConversation(store: Store, genId: () => string, now: number): Store {
  const active = store.conversations.find((c) => c.id === store.activeId);
  if (active && active.messages.length === 0) return store;
  const id = genId();
  return { conversations: [newConversation(id, now), ...store.conversations], activeId: id };
}

// deleteConversation removes one; if it was active, activates the most recent
// remaining; if none remain, seeds a fresh empty conversation.
export function deleteConversation(store: Store, id: string, genId: () => string, now: number): Store {
  const remaining = store.conversations.filter((c) => c.id !== id);
  if (remaining.length === 0) {
    const nid = genId();
    return { conversations: [newConversation(nid, now)], activeId: nid };
  }
  const activeId = store.activeId === id ? remaining[0].id : store.activeId;
  return { conversations: remaining, activeId };
}
