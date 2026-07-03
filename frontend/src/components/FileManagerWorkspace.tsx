import { useEffect, useMemo, useState } from "react";
import {
  ChevronRight,
  Download,
  File as FileIcon,
  FileText,
  Folder,
  FolderOpen,
  Home,
  Loader2,
  RefreshCw,
  ChevronDown,
  Search,
} from "lucide-react";
import { cn, fmtTime } from "@/lib/utils";
import { Badge } from "@/components/ui/badge";
import { EmptyState } from "@/components/ui/empty";
import { ErrorText } from "@/components/JsonView";
import { Workspace, WorkspaceColumn } from "@/components/ui/workspace";
import { MonacoView } from "@/components/MonacoView";
import {
  basename,
  fetchFileBlob,
  isPathSafe,
  parentPath,
  rawFileURL,
  useFileTree,
  type FileNode,
} from "@/lib/files";
import { textKind } from "@/views/Files";

// FileManagerWorkspace is the dedicated 3-pane manager: folder tree on the left,
// file list in the middle, file detail/preview on the right. Reached via the
// "Workspace" toggle in Files.tsx. Backend route /api/files/{tree,raw,…} lives
// in kernel/webui/files_route.go (Slice 5); until then useFileTree falls back
// to a deterministic stub so the layout renders end-to-end.

const MAX_BYTES = 2 * 1024 * 1024; // matches Files preview cap (M842)
const TEXT_LANGS = new Set(["markdown", "json", "code", "text"]);

function humanSize(n?: number): string {
  if (!n || n <= 0) return "—";
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`;
  return `${(n / (1024 * 1024)).toFixed(1)} MB`;
}

export function FileManagerWorkspace() {
  const [cwd, setCwd] = useState("");
  const [selected, setSelected] = useState<string | null>(null);
  const [query, setQuery] = useState("");
  const tree = useFileTree(cwd);

  // Breadcrumb segments — root + each folder component.
  const crumbs = useMemo(() => cwd.split("/").filter(Boolean), [cwd]);

  // Filtered list of the current directory (recursive search still kept flat
  // across the visible node list — folders + immediate files only, the user
  // navigates with the tree for deep dives).
  const listed = useMemo(() => {
    const all = tree.data?.nodes ?? [];
    if (!query) return all;
    const q = query.toLowerCase();
    return all.filter((n) => n.name.toLowerCase().includes(q));
  }, [tree.data, query]);

  // Whenever cwd changes (and the data has loaded), drop a selection that no
  // longer exists. The detail panel stays in "select a file" state.
  useEffect(() => {
    if (selected && !listed.some((n) => n.path === selected && n.type === "file")) {
      setSelected(null);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [cwd, listed]);

  return (
    <Workspace
      left={
        <WorkspaceColumn
          title="Folders"
          actions={
            <button
              onClick={tree.reload}
              className="rounded p-1 text-muted hover:text-foreground"
              title="Reload"
              aria-label="Reload tree"
            >
              <RefreshCw className={cn("size-3.5", tree.loading && "animate-spin")} />
            </button>
          }
        >
          <div className="p-1">
            {tree.error && <ErrorText>{tree.error}</ErrorText>}
            <button
              onClick={() => {
                setCwd("");
                setSelected(null);
              }}
              className={cn(
                "flex w-full items-center gap-1.5 rounded px-2 py-1 text-left text-xs transition-colors",
                cwd === "" ? "bg-accent/10 text-accent" : "text-muted hover:bg-panel hover:text-foreground",
              )}
            >
              <Home className="size-3.5" /> workspace root
            </button>
            {tree.data?.nodes.map((n) =>
              n.type === "dir" ? (
                <TreeRow key={n.path} node={n} current={cwd} onPick={(p) => setCwd(p)} />
              ) : null,
            )}
          </div>
        </WorkspaceColumn>
      }
      center={
        <WorkspaceColumn
          title={
            <Breadcrumb crumbs={crumbs} onJump={(i) => setCwd(crumbs.slice(0, i).join("/"))} />
          }
          actions={
            <div className="relative">
              <Search className="pointer-events-none absolute left-1.5 top-1/2 size-3 -translate-y-1/2 text-muted" />
              <input
                value={query}
                onChange={(e) => setQuery(e.target.value)}
                placeholder="filter"
                className="w-28 rounded-full border border-border bg-panel py-0.5 pl-6 pr-2 text-[11px]"
              />
            </div>
          }
        >
          {!tree.data?.nodes.length ? (
            <p className="px-3 py-6 text-center text-xs text-muted">
              {tree.loading ? "loading…" : "Empty folder"}
            </p>
          ) : (
            <ul className="divide-y divide-border">
              {listed.map((n) => (
                <li
                  key={n.path}
                  onClick={() => n.type === "file" && setSelected(n.path)}
                  className={cn(
                    "flex items-center gap-2 px-3 py-1.5 text-xs transition-colors",
                    n.type === "file"
                      ? "cursor-pointer hover:bg-panel"
                      : "text-muted",
                    selected === n.path && "bg-accent/10 text-accent",
                  )}
                >
                  {n.type === "dir" ? (
                    <Folder className="size-3.5 shrink-0" />
                  ) : (
                    <FileIcon className="size-3.5 shrink-0" />
                  )}
                  <span className="min-w-0 flex-1 truncate font-mono">{n.name}</span>
                  {n.type === "file" && (
                    <>
                      <span className="text-[11px] text-muted">{humanSize(n.size)}</span>
                      <span className="text-[11px] text-muted">{fmtTime(n.modified_ms)}</span>
                    </>
                  )}
                </li>
              ))}
            </ul>
          )}
        </WorkspaceColumn>
      }
      right={
        <WorkspaceColumn title="Preview">
          {selected ? (
            <FileDetail path={selected} onJump={setSelected} />
          ) : (
            <div className="flex h-full min-h-[200px] items-center justify-center p-6">
              <EmptyState
                icon={FileText}
                title="Select a file"
                hint="Pick a file from the list to preview it. Code, JSON, and Markdown open in the editor; binaries offer Download."
              />
            </div>
          )}
        </WorkspaceColumn>
      }
    />
  );
}

// TreeRow is a single collapsible folder row in the left pane. Single level of
// disclosure; the user expands into the list view if they want to go deeper.
// Cheaper than a full tree walker when most folders stay unexpanded.
function TreeRow({ node, current, onPick }: { node: FileNode; current: string; onPick: (p: string) => void }) {
  const [open, setOpen] = useState(false);
  const subtree = useFileTree(open ? node.path : "");
  const on = current === node.path || current.startsWith(node.path + "/");
  return (
    <div>
      <button
        onClick={() => {
          setOpen((v) => !v);
          onPick(node.path);
        }}
        className={cn(
          "flex w-full items-center gap-1 rounded px-2 py-1 text-left text-xs transition-colors",
          on ? "bg-accent/10 text-accent" : "text-foreground/90 hover:bg-panel",
        )}
        title={node.path}
      >
        {open ? (
          <ChevronDown className="size-3 shrink-0 text-muted" />
        ) : (
          <ChevronRight className="size-3 shrink-0 text-muted" />
        )}
        {open ? <FolderOpen className="size-3.5 shrink-0" /> : <Folder className="size-3.5 shrink-0" />}
        <span className="min-w-0 flex-1 truncate font-mono">{node.name}</span>
      </button>
      {open && subtree.data?.nodes && (
        <ul className="ml-4 border-l border-border pl-2">
          {subtree.data.nodes
            .filter((n) => n.type === "dir")
            .map((child) => (
              <li key={child.path}>
                <TreeRow node={child} current={current} onPick={onPick} />
              </li>
            ))}
        </ul>
      )}
    </div>
  );
}

// Breadcrumb renders the path segments as clickable chips — the standard
// "you are here" affordance for a tree browser.
function Breadcrumb({ crumbs, onJump }: { crumbs: string[]; onJump: (i: number) => void }) {
  return (
    <div className="flex min-w-0 items-center gap-0.5">
      <button
        onClick={() => onJump(0)}
        className="rounded px-1.5 py-0.5 font-mono text-[11px] text-muted hover:bg-panel hover:text-foreground"
        title="workspace root"
      >
        ~
      </button>
      {crumbs.map((c, i) => (
        <span key={i} className="flex min-w-0 items-center gap-0.5">
          <ChevronRight className="size-3 shrink-0 text-muted/60" />
          <button
            onClick={() => onJump(i + 1)}
            className="truncate rounded px-1.5 py-0.5 font-mono text-[11px] text-foreground/80 hover:bg-panel"
            title={crumbs.slice(0, i + 1).join("/")}
          >
            {c}
          </button>
        </span>
      ))}
    </div>
  );
}

// FileDetail renders the right-hand preview. Text-shaped files are read and
// shown via MonacoView; binary / over-cap files get a Download-only fallback
// so a giant log doesn't freeze the UI.
function FileDetail({ path, onJump: _onJump }: { path: string; onJump: (p: string) => void }) {
  void _onJump;
  const [content, setContent] = useState<string | null>(null);
  const [mime, setMime] = useState<string>("");
  const [size, setSize] = useState<number | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);

  const fakeEntry = useMemo(
    () => ({ id: path, ref: path, name: basename(path), mime: "", kind: "file", size: undefined as number | undefined }),
    [path],
  );
  const kind = textKind(fakeEntry);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setErr(null);
    setContent(null);
    setSize(null);
    setMime("");
    if (!isPathSafe(path)) {
      setErr("Unsafe path");
      return () => {
        cancelled = true;
      };
    }
    if (!TEXT_LANGS.has(kind)) {
      // Binary-ish — fall through to download-only.
      return () => {
        cancelled = true;
      };
    }
    setLoading(true);
    fetch(rawFileURL(path), { headers: {} })
      .then((r) => {
        if (!r.ok) throw new Error(`HTTP ${r.status}`);
        setSize(parseInt(r.headers.get("content-length") || "0", 10) || null);
        setMime(r.headers.get("content-type") || "");
        return r.text();
      })
      .then((t) => {
        if (!cancelled) setContent(t);
      })
      .catch((e: Error) => !cancelled && setErr(e.message))
      .finally(() => !cancelled && setLoading(false));
    return () => {
      cancelled = true;
    };
  }, [path, kind]);

  const download = async () => {
    try {
      const blob = await fetchFileBlob(path);
      const href = URL.createObjectURL(blob);
      const a = document.createElement("a");
      a.href = href;
      a.download = basename(path);
      document.body.appendChild(a);
      a.click();
      a.remove();
      URL.revokeObjectURL(href);
    } catch (e: unknown) {
      setErr(e instanceof Error ? e.message : String(e));
    }
  };

  const parent = parentPath(path);

  return (
    <div className="flex h-full min-h-0 flex-col">
      <div className="flex items-center gap-2 border-b border-border px-3 py-2">
        <Badge variant="accent">{kind || "file"}</Badge>
        <span className="min-w-0 flex-1 truncate font-mono text-xs" title={path}>
          {path}
        </span>
        {parent && (
          <span className="text-[11px] text-muted">in {parent || "~"}</span>
        )}
        <button
          onClick={download}
          className="inline-flex items-center gap-1 rounded border border-border bg-card px-2 py-0.5 text-[11px] text-muted hover:text-foreground"
          title="Download"
        >
          <Download className="size-3" /> download
        </button>
      </div>
      <div className="min-h-0 flex-1 overflow-auto p-3">
        {err && <p className="py-6 text-center text-xs text-muted">{err}</p>}
        {!err && loading && (
          <p className="flex items-center justify-center gap-2 py-6 text-xs text-muted">
            <Loader2 className="size-3.5 animate-spin" /> loading…
          </p>
        )}
        {!err && !loading && content === null && (
          <div className="flex h-full items-center justify-center py-10">
            <EmptyState
              icon={Download}
              title="Binary file"
              hint={
                size !== null && size > MAX_BYTES
                  ? `File is too large to preview inline (${humanSize(size)}); preview is disabled past ${humanSize(MAX_BYTES)}. Download to inspect.`
                  : "No inline preview for this file type — use Download."
              }
              action={
                <button
                  onClick={download}
                  className="mt-2 inline-flex items-center gap-1 rounded border border-border px-2 py-1 text-xs text-muted hover:text-foreground"
                >
                  <Download className="size-3.5" /> Download
                </button>
              }
            />
          </div>
        )}
        {!err && !loading && content !== null && (
          <MonacoView
            value={content}
            path={path}
            readOnly
            height={Math.min(560, content.split("\n").length * 18 + 60)}
          />
        )}
      </div>
    </div>
  );
}
