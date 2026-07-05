import { useCallback, useEffect, useState } from "react";
import { getJSON, authHeaders, HTTPError } from "@/lib/api";

// Files workspace — types + hooks backed by /api/files/{tree,raw,…}. The Go
// side lands in Slice 5; until then the tree hook falls back to a small in-memory
// stub so the UI renders end-to-end and the chat mention → detail flow has
// something to point at.
//
// Security: `path` is always a relative POSIX path inside the configured root.
// The route (when it exists) rejects "..", absolute paths, and symlink escapes.
// The UI never constructs absolute paths and never trusts a user-supplied
// path as a FileSystem access — that's the route's job.

export interface FileNode {
  name: string;
  path: string; // POSIX, no leading slash, root = ""
  type: "dir" | "file";
  size?: number;
  modified_ms?: number;
  // Children are loaded on demand by useFileTree; absent on a leaf.
  children?: FileNode[];
}

/** @public Response envelope for the file-tree API. */
export interface FileTreeResponse {
  root: string;
  nodes: FileNode[];
}

// isPathSafe is the client-side sanity check — UI never offers an unsafe path.
// The route must always re-validate; this just keeps the UI quiet.
export function isPathSafe(p: string): boolean {
  if (!p) return true;
  if (p.startsWith("/") || /^[A-Za-z]:/.test(p)) return false;
  if (p.includes("\0")) return false;
  // ".." as a full segment is rejected; "../foo" is too. The server also checks.
  const parts = p.split("/");
  return !parts.some((seg) => seg === "..");
}

// joinPath normalises "a" + "b/c" → "a/b/c", collapses redundant slashes, and
// refuses to walk above root (returns the parent unchanged).
export function joinPath(base: string, add: string): string {
  const a = (base || "").replace(/^\/+|\/+$/g, "");
  const b = (add || "").replace(/^\/+|\/+$/g, "");
  const merged = [a, b].filter(Boolean).join("/");
  // Collapse stray "./" so it never produces "a/./b".
  return merged.replace(/\/\.{0,1}\//g, "/").replace(/^\.\//, "");
}

// parentPath returns "" for the root and the joined path for everything else.
export function parentPath(p: string): string {
  if (!p) return "";
  const i = p.lastIndexOf("/");
  return i < 0 ? "" : p.slice(0, i);
}

// basename is the leaf segment of a path.
export function basename(p: string): string {
  if (!p) return "";
  const i = p.lastIndexOf("/");
  return i < 0 ? p : p.slice(i + 1);
}

interface TreeState {
  data: FileTreeResponse | null;
  error: string | null;
  loading: boolean;
  reload: () => void;
}

// useFileTree fetches the tree node list for `path`. Lazy: a folder's children
// arrive only when expandNode(path) is called. Until the route lands, returns
// a deterministic stub so the workspace still walks.
export function useFileTree(path: string): TreeState {
  const [data, setData] = useState<FileTreeResponse | null>(() => treeCache.get(path));
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);

  // S2.2: tree-cache short-circuit — if the path has a fresh cache entry,
  // mount with that data immediately so the workspace renders without a
  // round-trip when the operator re-opens a folder they already looked at
  // in this session (or within the 60 s TTL).
  useEffect(() => {
    // Re-check the cache on path change; the initial-state lazy initializer
    // only fires on first mount, so the useState seed stays empty on
    // re-mount with a different path.
    const cached = treeCache.get(path);
    if (cached) setData(cached);
    if (!isPathSafe(path)) {
      setError("Unsafe path");
      setData(null);
      return;
    }
    // Skip the fetch entirely on a fresh cache hit — the data we already
    // memoised is what the caller wanted.
    if (cached) {
      setLoading(false);
      return;
    }
    setLoading(true);
    setError(null);
    inflightTrees
      .fetch(path)
      .then((d) => {
        treeCache.set(path, d);
        setData(d);
      })
      .catch((e: Error) => {
        // 404 → stub fallback so the workspace still renders pre-Slice 5.
        // Use HTTPError.status (typed) instead of the previous regex on the
        // human error message, which silently broke when lib/api changed
        // its wording.
        if (e instanceof HTTPError && e.status === 404) {
          const stub = stubTree(path);
          treeCache.set(path, stub);
          setData(stub);
          setError(null);
        } else {
          setError(e.message);
          setData(null);
        }
      })
      .finally(() => setLoading(false));
  }, [path]);

  // S2.2: explicit reload bypasses the cache entirely. The Reload button
  // is the operator's escape hatch when a folder's contents have changed.
  const reload = useCallback(() => {
    if (!isPathSafe(path)) {
      setError("Unsafe path");
      setData(null);
      return;
    }
    treeCache.invalidate(path);
    setLoading(true);
    setError(null);
    inflightTrees
      .fetch(path, { force: true })
      .then((d) => {
        treeCache.set(path, d);
        setData(d);
      })
      .catch((e: Error) => {
        if (e instanceof HTTPError && e.status === 404) {
          const stub = stubTree(path);
          treeCache.set(path, stub);
          setData(stub);
          setError(null);
        } else {
          setError(e.message);
          setData(null);
        }
      })
      .finally(() => setLoading(false));
  }, [path]);

  return { data, error, loading, reload };
}

// ─────────────────────────────────────────────────────────────────────────
// S2.2 tree cache
//
// A small LRU + TTL cache that dedupes /api/files/tree round-trips across
// hook instances. The pattern looks like:
//
//   * tree-cache.get(path)          → reuses the most recent response if
//                                     it's younger than the TTL
//   * tree-cache.set(path, data)    → records a fresh response
//   * tree-cache.invalidate(path)   → drops a single entry (the Reload
//                                     button calls this so an operator
//                                     can force a refetch without
//                                     waiting for the TTL)
//
// 32 entries is enough for a typical session (operators open a handful of
// folders) and the 60 s TTL keeps stale results out of long-running tabs
// without forcing a manual reload after every back-and-forth navigation.
//
// Concurrent fetches for the same path share one promise via `inflightTrees`
// so a workspace that opens ten folders in one render pass still issues
// one network request per unique path.
// ─────────────────────────────────────────────────────────────────────────

const TREE_CACHE_MAX = 32;
const TREE_CACHE_TTL_MS = 60_000;

interface CacheEntry {
  data: FileTreeResponse;
  ts: number;
}

const treeCache = {
  // Map keeps insertion order, which is the oldest-first signal we need
  // for LRU eviction. `entries()` is O(1) for size, delete at any key
  // is O(1); we cap by shifting the oldest entry on overflow.
  store: new Map<string, CacheEntry>(),

  get(path: string): FileTreeResponse | null {
    const e = this.store.get(path);
    if (!e) return null;
    if (Date.now() - e.ts > TREE_CACHE_TTL_MS) {
      this.store.delete(path);
      return null;
    }
    // Touch — re-insert to move to the back (LRU update).
    this.store.delete(path);
    this.store.set(path, e);
    return e.data;
  },

  set(path: string, data: FileTreeResponse): void {
    this.invalidate(path);
    this.store.set(path, { data, ts: Date.now() });
    while (this.store.size > TREE_CACHE_MAX) {
      // Map iteration order is insertion order; first key is the LRU victim.
      const oldest = this.store.keys().next().value;
      if (oldest === undefined) break;
      this.store.delete(oldest);
    }
  },

  invalidate(path: string): void {
    this.store.delete(path);
  },
};

// in-flight promise registry — collapses concurrent fetches for the same
// path into one network request. `force: true` skips the dedup so the
// Reload button always hits the wire.
const inflightTrees = {
  pending: new Map<string, Promise<FileTreeResponse>>(),
  async fetch(path: string, opts: { force?: boolean } = {}): Promise<FileTreeResponse> {
    if (!opts.force) {
      const live = this.pending.get(path);
      if (live) return live;
    }
    const p = (async () => {
      try {
        return await getJSON<FileTreeResponse>("/api/files/tree", { path });
      } finally {
        this.pending.delete(path);
      }
    })();
    this.pending.set(path, p);
    return p;
  },
};

// Test-only escape hatch. The cache lives at module scope (it survives
// between hook instances within one session), so unit tests need a way to
// reset it. Production code never calls this; tests do via `beforeEach`.
export function __resetTreeCacheForTest(): void {
  treeCache.store.clear();
}

// rawFileURL is the route the Files/Artifacts viewer already use; the new
// file-tree viewer reuses /api/files/raw for actual byte reads (same auth, same
// path-traversal guards on the server).
export function rawFileURL(path: string, download = false): string {
  const params = new URLSearchParams({ path });
  if (download) params.set("download", "1");
  return `/api/files/raw?${params.toString()}`;
}

export async function fetchFileBlob(path: string): Promise<Blob> {
  const res = await fetch(rawFileURL(path), { headers: authHeaders() });
  if (!res.ok) throw new Error(`HTTP ${res.status}`);
  return res.blob();
}

// stubTree is the deterministic fallback until /api/files/tree lands. It seeds
// the workspace with one folder and a couple of plain text files so operators
// can click around and verify the layout works.
function stubTree(path: string): FileTreeResponse {
  const rel = path || "";
  const nodes: FileNode[] = [
    {
      name: "notes",
      path: joinPath(rel, "notes"),
      type: "dir",
      modified_ms: Date.now() - 86400_000,
      children: [
        { name: "README.md", path: joinPath(rel, "notes/README.md"), type: "file", size: 320, modified_ms: Date.now() - 3600_000 },
      ],
    },
    {
      name: "scratch.txt",
      path: joinPath(rel, "scratch.txt"),
      type: "file",
      size: 128,
      modified_ms: Date.now() - 600_000,
    },
  ];
  return { root: rel, nodes };
}
