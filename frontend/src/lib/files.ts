import { useCallback, useEffect, useState } from "react";
import { getJSON, authHeaders } from "@/lib/api";

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
  const [data, setData] = useState<FileTreeResponse | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);

  const reload = useCallback(() => {
    if (!isPathSafe(path)) {
      setError("Unsafe path");
      setData(null);
      return;
    }
    setLoading(true);
    setError(null);
    getJSON<FileTreeResponse>("/api/files/tree", { path })
      .then((d) => {
        setData(d);
      })
      .catch((e: Error) => {
        // 404 → stub fallback so the workspace still renders pre-Slice 5.
        if (/\b404\b/.test(e.message) || /not found/i.test(e.message)) {
          setData(stubTree(path));
          setError(null);
        } else {
          setError(e.message);
          setData(null);
        }
      })
      .finally(() => setLoading(false));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [path]);

  useEffect(() => {
    reload();
  }, [reload]);

  return { data, error, loading, reload };
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
