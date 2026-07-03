// @vitest-environment jsdom
//
// Tests for the generic useCursorPager helper. Mirrors Runs.pager.test.tsx
// but covers the underlying helper so any view that uses it inherits the
// same guarantees: terminal first page, cursor chain, dedup, server-error
// surface, three-page chain.

import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, act, waitFor } from "@testing-library/react";

interface Row {
  id: string;
  [k: string]: unknown;
}

let nextResponse: ((cursor: string | null) => unknown) | "throw" = () => ({
  items: [],
  next_cursor: null,
});

const messages: string[] = [];

vi.mock("@/lib/api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/api")>();
  return {
    ...actual,
    HTTPError: actual.HTTPError,
    getJSON: vi.fn(async (path: string, params?: Record<string, string>) => {
      const cursor = params?.cursor;
      // Mirror the real getJSON's URL-encoding so the test exercises the
      // exact query string the hook sends. The real implementation uses
      // URLSearchParams under the hood; for assertions we just stringify
      // the raw record — params are already URL-safe in our test inputs.
      const allParams = Object.entries(params || {})
        .map(([k, v]) => `${k}=${v}`)
        .join("&");
      messages.push(`${path}?${allParams}`);
      if (nextResponse === "throw") throw new Error("boom");
      return nextResponse(cursor ?? null);
    }),
    authHeaders: () => ({}),
  };
});

import { useCursorPager } from "@/lib/cursorPager";
import {
  useAgentsPager,
  useInboxPager,
  useBoardPager,
  useMemoryPager,
  useAgentActivityPager,
  useAgentEscalationsPager,
} from "@/lib/cursorPager";

function row(id: string): Row {
  return { id };
}

beforeEach(() => {
  nextResponse = () => ({ items: [], next_cursor: null });
  messages.length = 0;
});

describe("useCursorPager", () => {
  it("first page terminal: cursor stays null, hasMore=false", async () => {
    nextResponse = () => ({ items: [row("a"), row("b")], next_cursor: null });
    const { result } = renderHook(() =>
      useCursorPager<Row>("/api/x", "items", "id", 50),
    );
    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.paged.map((r) => r.id)).toEqual(["a", "b"]);
    expect(result.current.hasMore).toBe(false);
  });

  it("chains three pages via repeated loadMore and merges dedup", async () => {
    const cursorChain = ["100:1", "200:2", null];
    nextResponse = (cursor) => {
      const i = ["", ...cursorChain].indexOf(cursor ?? "");
      const rows =
        i === 0 ? [row("a"), row("b"), row("c")]
        : i === 1 ? [row("d"), row("e"), row("f")]
        : i === 2 ? [row("g")]
        : [];
      const next = cursorChain[Math.min(i, cursorChain.length - 1)];
      return { items: rows, next_cursor: next };
    };
    const { result } = renderHook(() =>
      useCursorPager<Row>("/api/x", "items", "id", 50),
    );
    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.hasMore).toBe(true);
    await act(async () => {
      await result.current.loadMore();
    });
    expect(result.current.hasMore).toBe(true);
    await act(async () => {
      await result.current.loadMore();
    });
    expect(result.current.hasMore).toBe(false);
    expect(result.current.paged.map((r) => r.id)).toEqual([
      "a", "b", "c", "d", "e", "f", "g",
    ]);
  });

  it("dedups when the same row appears on a later page", async () => {
    nextResponse = (cursor) => {
      if (!cursor) return { items: [row("a"), row("b")], next_cursor: "100:1" };
      return { items: [row("a"), row("c")], next_cursor: null };
    };
    const { result } = renderHook(() =>
      useCursorPager<Row>("/api/x", "items", "id", 50),
    );
    await waitFor(() => expect(result.current.loading).toBe(false));
    await act(async () => {
      await result.current.loadMore();
    });
    expect(result.current.paged.map((r) => r.id)).toEqual(["a", "b", "c"]);
  });

  it("surfaces server error in `moreError` and keeps `hasMore`", async () => {
    nextResponse = (cursor) => {
      if (!cursor) return { items: [row("a")], next_cursor: "100:1" };
      // Returning a different nextResponse function won't trigger — we
      // flip the module-level `nextResponse` to "throw" to force an error.
      throw new Error("boom");
    };
    const { result } = renderHook(() =>
      useCursorPager<Row>("/api/x", "items", "id", 50),
    );
    await waitFor(() => expect(result.current.loading).toBe(false));
    // Override the global to make page 2 throw.
    nextResponse = "throw";
    await act(async () => {
      await result.current.loadMore();
    });
    expect(result.current.moreError).toBe("boom");
    expect(result.current.hasMore).toBe(true);
  });

  it("forwards extra params (channel, topic) on every request", async () => {
    nextResponse = () => ({ items: [row("a")], next_cursor: "100:1" });
    const { result } = renderHook(() =>
      useCursorPager<Row>("/api/x", "items", "id", 50, { channel: "telegram", topic: "ops" }),
    );
    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(messages[0]).toContain("channel=telegram");
    expect(messages[0]).toContain("topic=ops");
    await act(async () => {
      await result.current.loadMore();
    });
    expect(messages[1]).toContain("channel=telegram");
    expect(messages[1]).toContain("topic=ops");
    expect(messages[1]).toContain("cursor=100:1");
  });
});

// ────────── Endpoint-specific wrappers ──────────
//
// Smoke tests for the per-view hooks. They exercise the same
// state machine as useCursorPager (which is the bulk of the logic);
// the wrappers only pin the (itemsKey, idKey, path, params) tuple
// for each endpoint. One test per wrapper is enough to lock that
// contract.

interface AgentsResp {
  profiles: { slug: string }[];
  next_cursor: string | null;
}
interface InboxResp {
  threads: { correlation_id: string }[];
  next_cursor: string | null;
}
interface BoardResp {
  messages: { id: string }[];
  next_cursor: string | null;
}
interface MemoryResp {
  records: { id: string }[];
  next_cursor: string | null;
}
interface AgentActivityResp {
  activity: { seq: number }[];
  next_cursor: string | null;
}
interface AgentEscalationsResp {
  escalations: { message_id: string }[];
  next_cursor: string | null;
}

describe("useAgentsPager", () => {
  it("hits /api/agents with cursor + slug-dedup", async () => {
    let cursorChain = ["100:1", null];
    nextResponse = (cursor) => {
      const i = ["", ...cursorChain].indexOf(cursor ?? "");
      return {
        profiles: i === 0 ? [{ slug: "alpha" }, { slug: "bravo" }] : [{ slug: "charlie" }],
        next_cursor: cursorChain[Math.min(i, cursorChain.length - 1)],
      } as AgentsResp;
    };
    const { result } = renderHook(() => useAgentsPager());
    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(messages[0]).toMatch(/^\/api\/agents\?/);
    expect(messages[0]).toContain("limit=100");
    expect(result.current.paged.map((p) => p.slug)).toEqual(["alpha", "bravo"]);
    expect(result.current.hasMore).toBe(true);
    await act(async () => {
      await result.current.loadMore();
    });
    expect(result.current.paged.map((p) => p.slug)).toEqual(["alpha", "bravo", "charlie"]);
    expect(result.current.hasMore).toBe(false);
  });
});

describe("useInboxPager", () => {
  it("forwards channel filter and uses correlation_id as idKey", async () => {
    nextResponse = () => ({
      threads: [{ correlation_id: "c1" }],
      next_cursor: null,
    } as InboxResp);
    const { result } = renderHook(() => useInboxPager("telegram"));
    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(messages[0]).toContain("channel=telegram");
    expect(result.current.paged.map((t) => t.correlation_id)).toEqual(["c1"]);
  });
});

describe("useBoardPager", () => {
  it("forwards topic filter and uses message id as idKey", async () => {
    nextResponse = () => ({
      messages: [{ id: "m1" }],
      next_cursor: null,
    } as BoardResp);
    const { result } = renderHook(() => useBoardPager("ops"));
    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(messages[0]).toContain("topic=ops");
    expect(result.current.paged.map((m) => m.id)).toEqual(["m1"]);
  });
});

describe("useMemoryPager", () => {
  it("hits /api/memory with id as idKey", async () => {
    nextResponse = () => ({
      records: [{ id: "rec-1" }, { id: "rec-2" }],
      next_cursor: null,
    } as MemoryResp);
    const { result } = renderHook(() => useMemoryPager());
    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(messages[0]).toMatch(/^\/api\/memory\?/);
    expect(result.current.paged.map((r) => r.id)).toEqual(["rec-1", "rec-2"]);
  });
});

describe("useAgentActivityPager", () => {
  it("forwards ref and uses seq as idKey", async () => {
    nextResponse = () => ({
      activity: [{ seq: 100 }, { seq: 99 }],
      next_cursor: "98",
    } as AgentActivityResp);
    const { result } = renderHook(() => useAgentActivityPager("audited"));
    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(messages[0]).toContain("ref=audited");
    expect(result.current.paged.map((e) => String(e.seq))).toEqual(["100", "99"]);
    expect(result.current.hasMore).toBe(true);
  });
});

describe("useAgentEscalationsPager", () => {
  it("forwards ref and uses message_id as idKey", async () => {
    nextResponse = () => ({
      escalations: [{ message_id: "esc-1" }],
      next_cursor: null,
    } as AgentEscalationsResp);
    const { result } = renderHook(() => useAgentEscalationsPager("audited"));
    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(messages[0]).toContain("ref=audited");
    expect(result.current.paged.map((e) => e.message_id)).toEqual(["esc-1"]);
  });
});