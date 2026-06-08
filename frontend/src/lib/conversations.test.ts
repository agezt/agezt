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
  activeMessages,
  type Store,
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
