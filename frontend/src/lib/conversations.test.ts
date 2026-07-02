// @vitest-environment jsdom
// conversations.ts imports chat.ts (→ api.ts touches location); jsdom provides it.
import { describe, it, expect, beforeEach } from "vitest";
import {
  deriveTitle,
  normalizeMessages,
  loadStore,
  withActiveMessages,
  startConversation,
  deleteConversation,
  renameConversation,
  activeMessages,
  activePersona,
  withActivePersona,
  activeConvModel,
  withActiveConvModel,
  activeConvAgent,
  withActiveConvAgent,
  activeConvExecutionProfile,
  withActiveConvExecutionProfile,
  togglePinned,
  sortConversations,
  filterConversations,
  conversationText,
  type Store,
  type Conversation,
  type Msg,
} from "@/lib/conversations";

let n = 0;
const genId = () => `id-${++n}`;

beforeEach(() => {
  n = 0;
  localStorage.clear();
});

const userMsg = (text: string): Msg => ({ role: "user", text });

describe("deriveTitle", () => {
  it("uses the first user message, capped and single-line", () => {
    expect(deriveTitle([userMsg("  hello   world  ")])).toBe("hello world");
    expect(deriveTitle([userMsg("x".repeat(50))])).toBe("x".repeat(40) + "…");
    expect(deriveTitle([])).toBe("New chat");
  });
});

describe("normalizeMessages", () => {
  it("marks a restored mid-stream turn as interrupted", () => {
    const msgs: Msg[] = [
      { role: "user", text: "hi" },
      { role: "assistant", turn: { status: "streaming", streamedText: "half", reasoning: "", tools: [], iters: 1, costMicrocents: 0 } },
    ];
    const out = normalizeMessages(msgs);
    const bot = out[1] as Extract<Msg, { role: "assistant" }>;
    expect(bot.turn.status).toBe("error");
    expect(bot.turn.error).toBe("interrupted");
  });
});

describe("loadStore", () => {
  it("seeds a fresh store when storage is empty", () => {
    const s = loadStore(genId, 100);
    expect(s.conversations).toHaveLength(1);
    expect(s.activeId).toBe(s.conversations[0].id);
    expect(s.conversations[0].messages).toEqual([]);
  });

  it("migrates a legacy single thread into one conversation", () => {
    localStorage.setItem("agezt.chat.thread.v1", JSON.stringify([userMsg("legacy question")]));
    const s = loadStore(genId, 100);
    expect(s.conversations).toHaveLength(1);
    expect(s.conversations[0].title).toBe("legacy question");
    expect(activeMessages(s)).toHaveLength(1);
  });

  it("repairs an activeId that doesn't exist", () => {
    const stored: Store = {
      conversations: [{ id: "a", title: "A", messages: [], updatedAt: 1 }],
      activeId: "missing",
    };
    localStorage.setItem("agezt.chat.store.v2", JSON.stringify(stored));
    expect(loadStore(genId, 100).activeId).toBe("a");
  });
});

describe("store mutations", () => {
  it("withActiveMessages updates the active conversation and titles it", () => {
    let s = loadStore(genId, 1); // one empty "New chat"
    s = withActiveMessages(s, [userMsg("first prompt")], 2);
    const active = s.conversations.find((c) => c.id === s.activeId)!;
    expect(active.messages).toHaveLength(1);
    expect(active.title).toBe("first prompt"); // titled from first user msg
  });

  it("startConversation reuses an empty active rather than piling up blanks", () => {
    const s0 = loadStore(genId, 1); // empty active
    expect(startConversation(s0, genId, 2)).toBe(s0); // unchanged (reused)

    const s1 = withActiveMessages(s0, [userMsg("hi")], 2);
    const s2 = startConversation(s1, genId, 3);
    expect(s2.conversations).toHaveLength(2);
    expect(s2.activeId).not.toBe(s1.activeId);
    expect(activeMessages(s2)).toEqual([]); // new one is empty
  });

  it("withActivePersona sets and clears the active conversation's persona", () => {
    let s = loadStore(genId, 1);
    expect(activePersona(s)).toBe(""); // none by default
    s = withActivePersona(s, "  act as a reviewer  ", 2);
    expect(activePersona(s)).toBe("act as a reviewer"); // trimmed
    // Clearing drops the field entirely (no empty-string persisted).
    s = withActivePersona(s, "", 3);
    expect(activePersona(s)).toBe("");
    expect(s.conversations.find((c) => c.id === s.activeId)!.persona).toBeUndefined();
  });

  it("persona is per-conversation, not global", () => {
    let s = withActiveMessages(loadStore(genId, 1), [userMsg("a")], 2);
    s = withActivePersona(s, "persona A", 3);
    const firstId = s.activeId;
    s = startConversation(s, genId, 4); // new active thread
    expect(activePersona(s)).toBe(""); // the new thread has no override
    // The first thread still holds its own.
    expect(s.conversations.find((c) => c.id === firstId)!.persona).toBe("persona A");
  });

  it("withActiveConvModel sets and clears the active conversation's model, per thread", () => {
    let s = loadStore(genId, 1);
    expect(activeConvModel(s)).toBe(""); // daemon default by default
    s = withActiveConvModel(s, " deepseek-chat ", 2);
    expect(activeConvModel(s)).toBe("deepseek-chat"); // trimmed
    const firstId = s.activeId;

    s = startConversation(withActiveMessages(s, [userMsg("hi")], 3), genId, 4);
    expect(activeConvModel(s)).toBe(""); // new thread defaults
    expect(s.conversations.find((c) => c.id === firstId)!.model).toBe("deepseek-chat"); // first keeps its own

    s = withActiveConvModel(s, "", 5);
    expect(activeConvModel(s)).toBe("");
    expect(s.conversations.find((c) => c.id === s.activeId)!.model).toBeUndefined();
  });

  it("withActiveConvAgent sets and clears the active conversation's agent, per thread (M789)", () => {
    let s = loadStore(genId, 1);
    expect(activeConvAgent(s)).toBe(""); // default identity by default
    s = withActiveConvAgent(s, " researcher ", 2);
    expect(activeConvAgent(s)).toBe("researcher"); // trimmed
    const firstId = s.activeId;

    s = startConversation(withActiveMessages(s, [userMsg("hi")], 3), genId, 4);
    expect(activeConvAgent(s)).toBe(""); // new thread defaults
    expect(s.conversations.find((c) => c.id === firstId)!.agent).toBe("researcher"); // first keeps its own

    s = withActiveConvAgent(s, "", 5);
    expect(activeConvAgent(s)).toBe("");
    expect(s.conversations.find((c) => c.id === s.activeId)!.agent).toBeUndefined();
  });

  it("withActiveConvExecutionProfile sets and clears the active conversation's execution profile", () => {
    let s = loadStore(genId, 1);
    expect(activeConvExecutionProfile(s)).toBe("");
    s = withActiveConvExecutionProfile(s, " local ", 2);
    expect(activeConvExecutionProfile(s)).toBe("local");
    const firstId = s.activeId;

    s = startConversation(withActiveMessages(s, [userMsg("hi")], 3), genId, 4);
    expect(activeConvExecutionProfile(s)).toBe("");
    expect(s.conversations.find((c) => c.id === firstId)!.executionProfile).toBe("local");

    s = withActiveConvExecutionProfile(s, "docker", 5);
    expect(activeConvExecutionProfile(s)).toBe("docker");
    expect(s.conversations.find((c) => c.id === s.activeId)!.executionProfile).toBe("docker");

    s = withActiveConvExecutionProfile(s, "ssh", 6);
    expect(activeConvExecutionProfile(s)).toBe("ssh");
    s = withActiveConvExecutionProfile(s, "warden", 7);
    expect(activeConvExecutionProfile(s)).toBe("warden");
    s = withActiveConvExecutionProfile(s, "remote-agezt", 8);
    expect(activeConvExecutionProfile(s)).toBe("remote-agezt");
    s = withActiveConvExecutionProfile(s, "", 9);
    expect(activeConvExecutionProfile(s)).toBe("");
  });

  it("renameConversation sets a manual title; blank restores the derived one", () => {
    let s = withActiveMessages(loadStore(genId, 1), [userMsg("how do I deploy the thing")], 2);
    const id = s.activeId;
    s = renameConversation(s, id, "  Deploy notes  ", 3);
    expect(s.conversations.find((c) => c.id === id)!.title).toBe("Deploy notes"); // trimmed
    // A custom title sticks across new messages (no longer "New chat").
    s = withActiveMessages(s, [...activeMessages(s), userMsg("more")], 4);
    expect(s.conversations.find((c) => c.id === id)!.title).toBe("Deploy notes");
    // Clearing restores the message-derived title.
    s = renameConversation(s, id, "   ", 5);
    expect(s.conversations.find((c) => c.id === id)!.title).toBe("how do I deploy the thing");
  });

  it("togglePinned flips the flag and sortConversations floats pinned to the top", () => {
    let s = withActiveMessages(loadStore(genId, 1), [userMsg("a")], 10);
    const a = s.activeId;
    s = startConversation(s, genId, 20);
    s = withActiveMessages(s, [userMsg("b")], 20);
    const b = s.activeId; // b is newer than a

    // By recency, b is first.
    expect(sortConversations(s.conversations)[0].id).toBe(b);
    // Pin the older one (a) → it floats to the top despite being less recent.
    s = togglePinned(s, a);
    expect(s.conversations.find((c) => c.id === a)!.pinned).toBe(true);
    expect(sortConversations(s.conversations)[0].id).toBe(a);
    // Unpin → back to recency order.
    s = togglePinned(s, a);
    expect(s.conversations.find((c) => c.id === a)!.pinned).toBe(false);
    expect(sortConversations(s.conversations)[0].id).toBe(b);
  });

  it("filterConversations matches title and message text; empty query is a passthrough", () => {
    const botTurn = (text: string): Msg => ({
      role: "assistant",
      turn: { status: "done", streamedText: text, reasoning: "", tools: [], iters: 1, costMicrocents: 0 },
    });
    const convs: Conversation[] = [
      { id: "a", title: "Deploy notes", messages: [userMsg("how do I ship")], updatedAt: 3 },
      { id: "b", title: "Random chat", messages: [userMsg("hi"), botTurn("the kubernetes rollout is green")], updatedAt: 2 },
      { id: "c", title: "Groceries", messages: [userMsg("milk and eggs")], updatedAt: 1 },
    ];
    // Empty query → unchanged (same array contents).
    expect(filterConversations(convs, "  ").map((c) => c.id)).toEqual(["a", "b", "c"]);
    // Title match.
    expect(filterConversations(convs, "deploy").map((c) => c.id)).toEqual(["a"]);
    // User-message match.
    expect(filterConversations(convs, "EGGS").map((c) => c.id)).toEqual(["c"]); // case-insensitive
    // Assistant-reply match (searches the streamed text, not just titles).
    expect(filterConversations(convs, "kubernetes").map((c) => c.id)).toEqual(["b"]);
    // No match → empty.
    expect(filterConversations(convs, "zzz")).toEqual([]);
  });

  it("conversationText folds title + user + assistant text, lower-cased", () => {
    const c: Conversation = {
      id: "x",
      title: "My Title",
      messages: [
        { role: "user", text: "Hello THERE" },
        { role: "assistant", turn: { status: "done", streamedText: "General Kenobi", reasoning: "", tools: [], iters: 1, costMicrocents: 0 } },
      ],
      updatedAt: 1,
    };
    const t = conversationText(c);
    expect(t).toContain("my title");
    expect(t).toContain("hello there");
    expect(t).toContain("general kenobi");
  });

  it("deleteConversation activates a remaining one, or seeds fresh when last", () => {
    let s = withActiveMessages(loadStore(genId, 1), [userMsg("a")], 2);
    s = startConversation(s, genId, 3);
    s = withActiveMessages(s, [userMsg("b")], 4);
    const firstId = s.conversations[1].id;
    const del = deleteConversation(s, s.activeId, genId, 5);
    expect(del.conversations).toHaveLength(1);
    expect(del.activeId).toBe(firstId);

    const empty = deleteConversation(del, del.activeId, genId, 6);
    expect(empty.conversations).toHaveLength(1); // seeded fresh
    expect(activeMessages(empty)).toEqual([]);
  });
});
