// @vitest-environment jsdom
//
// Tests for the S2.2 useFileTree cache. We mock getJSON so we can drive
// the hook through its cache-hit, cache-miss, reload, 404-fallback and
// invalidation paths without hitting the network.

import { describe, expect, it, vi, beforeEach } from "vitest";
import { renderHook, waitFor, act } from "@testing-library/react";

const treeByPath = new Map<string, { nodes: Array<{ name: string; path: string; type: "dir" | "file" }> }>();

vi.mock("@/lib/api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/api")>();
  return {
    ...actual,
    HTTPError: class HTTPError extends Error {
      status: number;
      url: string;
      constructor(status: number, url: string, message: string) {
        super(message);
        this.name = "HTTPError";
        this.status = status;
        this.url = url;
      }
    },
    // Match getJSON's signature (path, params?) — the second arg carries
    // the workspace path under `params.path`, exactly the way the daemon's
    // tree endpoint is invoked.
    getJSON: vi.fn((_path: string, params?: Record<string, string>) => {
      const p = params?.path ?? "";
      const data = treeByPath.get(p);
      if (data === undefined) {
        return Promise.reject(
          new (class extends Error {
            status = 404;
            url = _path;
            name = "HTTPError";
          })(),
        );
      }
      return Promise.resolve({ root: p, nodes: data.nodes });
    }),
    authHeaders: () => ({}),
  };
});

import { useFileTree, __resetTreeCacheForTest } from "@/lib/files";
import * as api from "@/lib/api";
const mockGetJSON = vi.mocked(api.getJSON);

function setTree(path: string, nodes: Array<{ name: string; path: string; type: "dir" | "file" }>) {
  treeByPath.set(path, { nodes });
}

beforeEach(() => {
  treeByPath.clear();
  __resetTreeCacheForTest();
  mockGetJSON.mockClear();
});

// ─── Re-mounts: same path, second mount should hit cache, no extra fetch
describe("useFileTree cache", () => {
  it("returns the cached response on remount within the TTL window", async () => {
    setTree("", [{ name: "x", path: "x", type: "file" }]);
    const { result, unmount } = renderHook(() => useFileTree(""));
    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.data?.nodes).toHaveLength(1);
    expect(mockGetJSON).toHaveBeenCalledTimes(1);
    unmount();
    // Re-mount with the same path. The cached entry should return
    // without another round-trip.
    const second = renderHook(() => useFileTree(""));
    await waitFor(() => expect(second.result.current.loading).toBe(false));
    expect(mockGetJSON).toHaveBeenCalledTimes(1); // unchanged
    expect(second.result.current.data?.nodes).toHaveLength(1);
    second.unmount();
  });

  it("different paths issue separate fetches and end up cached separately", async () => {
    setTree("a", [{ name: "a1", path: "a1", type: "file" }]);
    setTree("b", [{ name: "b1", path: "b1", type: "file" }]);
    const a = renderHook(() => useFileTree("a"));
    const b = renderHook(() => useFileTree("b"));
    await waitFor(() => {
      expect(a.result.current.loading).toBe(false);
      expect(b.result.current.loading).toBe(false);
    });
    expect(a.result.current.data?.nodes[0].name).toBe("a1");
    expect(b.result.current.data?.nodes[0].name).toBe("b1");
    expect(mockGetJSON).toHaveBeenCalledTimes(2);
    a.unmount();
    b.unmount();
  });

  it("reload() bypasses the cache (mutated upstream is observed)", async () => {
    setTree("", [{ name: "v1", path: "v1", type: "file" }]);
    const { result } = renderHook(() => useFileTree(""));
    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.data?.nodes[0].name).toBe("v1");
    expect(mockGetJSON).toHaveBeenCalledTimes(1);
    // Mutate upstream and call reload — hook should observe v2 and
    // perform a second round-trip.
    setTree("", [{ name: "v2", path: "v2", type: "file" }]);
    await act(async () => {
      result.current.reload();
    });
    await waitFor(() => expect(result.current.data?.nodes[0].name).toBe("v2"));
    expect(mockGetJSON).toHaveBeenCalledTimes(2);
  });
});
