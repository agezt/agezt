// @vitest-environment jsdom
//
// Tests for the cursor pagination on the Runs view. We mock the
// network layer to drive useRunsPager through:
//   - first-page with no next_cursor (terminal),
//   - first-page with next_cursor → loadMore fetches the next page,
//   - next_cursor chain until terminal,
//   - server error on loadMore surfaces in `moreError`,
//   - dedup when the upstream double-emits the same run id.
//
// The Runs view uses `usePanel` which calls `getJSON`, so spying on the
// module-level `getJSON` is sufficient — the React hook tree runs for real
// and the only thing we control is the response.

import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { renderHook, act, waitFor } from "@testing-library/react";

// Minimal Run shape used by the Runs view's render path. We inline it
// here rather than importing from the component because the row type is
// wide and we only need a couple of fields populated for the pager
// behaviour we exercise.
interface Run {
  correlation_id?: string;
  intent?: string;
  status?: string;
  started_unix_ms?: number;
  completed_unix_ms?: number;
  duration_ms?: number;
  iters?: number;
  spent_mc?: number;
  model?: string;
  agent?: string;
  [k: string]: unknown;
}

// Mutable holder so the test can drive one-shot responses per call.
interface Response {
  runs: Run[];
  next_cursor: string | null;
}
let nextResponse: Response | ((cursor: string | null) => Response) | "throw" = {
  runs: [],
  next_cursor: null,
};

const messages: string[] = [];
let throwOn: string | null = null;

vi.mock("@/lib/api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/api")>();
  return {
    ...actual,
    HTTPError: actual.HTTPError,
    getJSON: vi.fn(async (path: string, params?: Record<string, string>) => {
      const cursor = params?.cursor;
      const limit = params?.limit;
      messages.push(`${path}?limit=${limit ?? ""}&cursor=${cursor ?? ""}`);
      if (throwOn && messages[messages.length - 1].includes(throwOn)) {
        throw new Error("boom");
      }
      if (typeof nextResponse === "function") {
        return (nextResponse as (c: string | null) => Response)(cursor ?? null);
      }
      return nextResponse as Response;
    }),
    authHeaders: () => ({}),
  };
});

// Import AFTER the mock so React.hooks etc resolve against the
// mocked module graph.
import { useRunsPager } from "@/views/Runs";

function runRow(id: string): Run {
  return {
    correlation_id: id,
    intent: id,
    status: "running",
    started_unix_ms: 1_700_000_000_000,
    completed_unix_ms: 0,
    duration_ms: 0,
    iters: 0,
    spent_mc: 0,
    model: "",
    agent: "",
  } as Run;
}

beforeEach(() => {
  nextResponse = { runs: [], next_cursor: null };
  throwOn = null;
  messages.length = 0;
});

afterEach(() => {
  vi.useRealTimers();
});

describe("useRunsPager", () => {
  it("first page: terminal — cursor stays null, hasMore=false", async () => {
    nextResponse = { runs: [runRow("a"), runRow("b")], next_cursor: null };
    const { result } = renderHook(() => useRunsPager());
    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.paged.map((r) => r.correlation_id)).toEqual(["a", "b"]);
    expect(result.current.hasMore).toBe(false);
  });

  it("first page with next_cursor sets hasMore; loadMore pulls page 2 and clears the cursor on terminal", async () => {
    nextResponse = (cursor) => {
      if (cursor) {
        return { runs: [runRow("c"), runRow("d")], next_cursor: null };
      }
      return { runs: [runRow("a"), runRow("b")], next_cursor: "100:1" };
    };
    const { result } = renderHook(() => useRunsPager());
    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.hasMore).toBe(true);
    await act(async () => {
      await result.current.loadMore();
    });
    await waitFor(() => expect(result.current.loadingMore).toBe(false));
    expect(messages).toContainEqual(expect.stringContaining("limit=50&cursor="));
    expect(messages).toContainEqual(expect.stringContaining("limit=50&cursor=100:1"));
    expect(result.current.paged.map((r) => r.correlation_id)).toEqual(["a", "b", "c", "d"]);
    expect(result.current.hasMore).toBe(false);
  });

  it("loadMore surfaces a server error in `moreError` and keeps `hasMore`", async () => {
    // First page returns a cursor. The next call (with that cursor)
    // throws — that's the `loadMore` failure path. The catch sets
    // `moreError`. Cursor remains set so the operator can retry.
    nextResponse = (cursor) => {
      if (!cursor) return { runs: [runRow("a")], next_cursor: "100:1" };
      // Throw rather than return — the loadMore catch will set moreError.
      throw new Error("boom");
    };
    const { result } = renderHook(() => useRunsPager());
    await waitFor(() => expect(result.current.loading).toBe(false));
    await act(async () => {
      await result.current.loadMore();
    });
    expect(result.current.moreError).toBe("boom");
    expect(result.current.hasMore).toBe(true);
  });

  it("dedups rows by correlation_id when a duplicate appears on a later page", async () => {
    nextResponse = (cursor) => {
      if (!cursor) return { runs: [runRow("a"), runRow("b")], next_cursor: "100:1" };
      return { runs: [runRow("a"), runRow("c")], next_cursor: null };
    };
    const { result } = renderHook(() => useRunsPager());
    await waitFor(() => expect(result.current.loading).toBe(false));
    await act(async () => {
      await result.current.loadMore();
    });
    const ids = result.current.paged.map((r) => r.correlation_id);
    expect(ids).toEqual(["a", "b", "c"]);
  });

  it("chains three pages correctly via repeated loadMore calls", async () => {
    const cursorChain = ["100:1", "200:2", null];
    nextResponse = (cursor) => {
      const i = ["", ...cursorChain].indexOf(cursor ?? "");
      const rows =
        i === 0 ? [runRow("a"), runRow("b"), runRow("c")]
        : i === 1 ? [runRow("d"), runRow("e"), runRow("f")]
        : i === 2 ? [runRow("g")]
        : [];
      const next = cursorChain[Math.min(i, cursorChain.length - 1)];
      return { runs: rows, next_cursor: next };
    };
    const { result } = renderHook(() => useRunsPager());
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
    expect(result.current.paged.map((r) => r.correlation_id)).toEqual(["a", "b", "c", "d", "e", "f", "g"]);
  });
});
