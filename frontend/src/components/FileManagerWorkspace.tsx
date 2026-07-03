import { useEffect, useMemo, useRef, useState } from "react";
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

/**
 * tooLargeReason returns the sentinel `too_large:<bytes>` if the file exceeds
 * the preview cap, otherwise null. Extracted as a pure function so the rule
 * can be unit-tested without rendering the whole detail panel.
 */
export function tooLargeReason(contentLength: number | null, actualBytes?: number): string | null {
  // Prefer the Content-Length header when present (cheap; bails before the
  // round-trip completes). Fall back to the actual byte count we observed in
  // the body, which catches servers that lie or omit the header.
  const probe = actualBytes ?? contentLength;
  if (probe === null || probe === undefined) return null;
  return probe > MAX_BYTES ? `too_large:${probe}` : null;
}

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
            {/* Workspace folders tree. role="tree" + the inner treeitems use
                roving tabindex (ArrowDown/Up cycle items, Tab moves between
                regions). Only directory nodes appear here — files are in the
                middle pane where you click to open them. */}
            <div role="tree" aria-label="Workspace folders">
              {/* The workspace root is a synthetic level-1 item so screen-reader
                  users land inside the tree on the same first row a sighted
                  user sees. Disclosure control is implicit: the root is always
                  "expanded" (showing its children directly in this view), so
                  aria-expanded isn't meaningful here. */}
              <div
                role="treeitem"
                aria-level={1}
                aria-selected={cwd === ""}
                tabIndex={cwd === "" ? 0 : -1}
                onClick={() => {
                  setCwd("");
                  setSelected(null);
                }}
                onKeyDown={(e) => {
                  if (e.key === "Enter" || e.key === " ") {
                    e.preventDefault();
                    setCwd("");
                    setSelected(null);
                  }
                }}
                className={cn(
                  "flex w-full cursor-pointer items-center gap-1.5 rounded px-2 py-1 text-left text-xs transition-colors outline-none focus-visible:ring-2 focus-visible:ring-accent",
                  cwd === "" ? "bg-accent/10 text-accent" : "text-muted hover:bg-panel hover:text-foreground",
                )}
              >
                <Home className="size-3.5 shrink-0" aria-hidden="true" /> workspace root
              </div>
              {tree.data?.nodes
                .filter((n) => n.type === "dir")
                .map((n) => (
                  <TreeRow key={n.path} node={n} current={cwd} onPick={(p) => setCwd(p)} level={1} />
                ))}
            </div>
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
          ) : listed.length === 0 ? (
            // The user typed a filter but it matched nothing in the current
            // directory. Don't render an empty <ul> — explain why and steer
            // them toward the tree (search is scoped to cwd, not recursive).
            <p
              className="px-3 py-6 text-center text-xs text-muted"
              role="status"
              aria-live="polite"
            >
              No matches for{" "}
              <span className="font-mono text-foreground/80">“{query}”</span>
              {cwd ? (
                <>
                  {" "}in <span className="font-mono">{cwd || "~"}</span>
                </>
              ) : (
                " in the workspace root"
              )}
              . The filter only scans the current folder — expand a directory
              in the tree on the left to look inside it.
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
//
// A11y: implements the WAI-ARIA tree pattern
// (https://www.w3.org/WAI/ARIA/apg/patterns/treeview/). Roaming tabindex —
// only the focused item is in the tab order; siblings carry tabIndex={-1} so
// Tab/Shift+Tab move BETWEEN regions (Folders → file list → preview pane),
// not within a region.
function TreeRow({
  node,
  current,
  onPick,
  level,
}: {
  node: FileNode;
  current: string;
  onPick: (p: string) => void;
  level: number;
}) {
  const [open, setOpen] = useState(false);
  const subtree = useFileTree(open ? node.path : "");
  const on = current === node.path || current.startsWith(node.path + "/");
  // The expanded-children group lives at level + 1. Only render once the
  // subtree has resolved (or returned an error) — rendering an empty group
  // would expose a confusing "no children" disclosure to screen readers.
  const childDirs = open && subtree.data?.nodes ? subtree.data.nodes.filter((n) => n.type === "dir") : [];
  const rowRef = useRef<HTMLDivElement | null>(null);
  // Move keyboard focus onto this row's DOM node. Used by the parent key
  // handlers (parent calls focusRow() on the appropriate child) and by the
  // arrow handlers below to walk the tree.
  const focusRow = () => rowRef.current?.focus();

  // Find all visible treeitems in DOM order, starting from this row's
  // ancestor tree. Walking the row's own DOM is tricky because each row
  // is its own component instance, so we reach up to the role="tree"
  // root, then do a single pre-order traversal that includes both siblings
  // (via the immediate parent) and expanded children (via the row's
  // role="group" container).
  //
  // We avoid `:scope >` here because jsdom's CSS engine doesn't honour it
  // reliably; instead we iterate children and filter to the role we need.
  // The structure under the tree is fixed so a small set of direct child
  // checks is enough.
  const collectVisibleRows = (): HTMLElement[] => {
    const start = rowRef.current;
    if (!start) return [];
    const treeRoot = start.closest('[role="tree"]') as HTMLElement | null;
    if (!treeRoot) return [start];
    const out: HTMLElement[] = [];
    // jsdom-friendly walk: scan all descendants once, sort by document
    // position, and emit only those whose DOM tree holds a role="treeitem".
    // `querySelectorAll` returns nodes in document order, so no extra sort
    // is required — and we avoid `:scope >` because jsdom's CSS engine
    // doesn't honour it reliably.
    const allTreeitems = Array.from(treeRoot.querySelectorAll('[role="treeitem"]'));
    allTreeitems.forEach((el) => out.push(el as HTMLElement));
    return out;
  };

  const handleKeyDown = (e: React.KeyboardEvent<HTMLDivElement>) => {
    const rows = collectVisibleRows();
    const idx = rows.findIndex((r) => r === rowRef.current);
    if (idx < 0) return;

    switch (e.key) {
      case "ArrowDown": {
        e.preventDefault();
        const next = rows[idx + 1];
        if (next) next.focus();
        break;
      }
      case "ArrowUp": {
        e.preventDefault();
        const prev = rows[idx - 1];
        if (prev) prev.focus();
        break;
      }
      case "Home": {
        e.preventDefault();
        rows[0]?.focus();
        break;
      }
      case "End": {
        e.preventDefault();
        rows[rows.length - 1]?.focus();
        break;
      }
      case "ArrowRight": {
        e.preventDefault();
        if (!open) {
          setOpen(true);
          onPick(node.path);
        }
        // If we already had children, focus the first one.
        else if (rows[idx + 1]) {
          rows[idx + 1].focus();
        }
        break;
      }
      case "ArrowLeft": {
        e.preventDefault();
        if (open) {
          setOpen(false);
        } else {
          // Collapsed + ArrowLeft: focus the parent. Walk up to the enclosing
          // treeitem (skipping the row container so the parent's rowRef matches
          // a treeitem).
          const parent = rowRef.current?.parentElement?.closest('[role="treeitem"]') as HTMLElement | null;
          parent?.focus();
        }
        break;
      }
      case "Enter":
      case " ": {
        e.preventDefault();
        // Enter/Space toggle disclosure, matching the click affordance.
        setOpen((v) => !v);
        onPick(node.path);
        break;
      }
    }
  };

  return (
    <>
    <div
      ref={rowRef}
      role="treeitem"
      aria-level={level + 1}
      // aria-expanded applies only when the disclosure has effect. A folder
      // that hasn't been opened yet still has potential children — until we
      // know it doesn't, we declare it expandable.
      aria-expanded={open || subtree.data === null ? true : childDirs.length > 0}
      aria-selected={on}
      tabIndex={on ? 0 : -1}
      onClick={() => {
        setOpen((v) => !v);
        onPick(node.path);
      }}
      onKeyDown={handleKeyDown}
      className={cn(
        "flex w-full cursor-pointer items-center gap-1 rounded px-2 py-1 text-left text-xs transition-colors outline-none focus-visible:ring-2 focus-visible:ring-accent",
        on ? "bg-accent/10 text-accent" : "text-foreground/90 hover:bg-panel",
      )}
      title={node.path}
    >
      {open ? (
        <ChevronDown className="size-3 shrink-0 text-muted" aria-hidden="true" />
      ) : (
        <ChevronRight className="size-3 shrink-0 text-muted" aria-hidden="true" />
      )}
      {open ? (
        <FolderOpen className="size-3.5 shrink-0" aria-hidden="true" />
      ) : (
        <Folder className="size-3.5 shrink-0" aria-hidden="true" />
      )}
      <span className="min-w-0 flex-1 truncate font-mono">{node.name}</span>
    </div>
    {open && (
      <div role="group" aria-label={`${node.name} contents`} className="ml-4 border-l border-border pl-2">
        {/* Subtree-loading state: an open folder whose child fetch hasn't
            resolved yet. Without it a screen-reader expanding a folder hears
            silence until the network call lands. */}
        {subtree.loading && subtree.data === null && (
          <p className="flex items-center gap-1.5 px-2 py-1 text-[11px] text-muted" role="status">
            <Loader2 className="size-3 animate-spin" /> loading…
          </p>
        )}
        {/* Subtree error state (S1.4): previously the only feedback was
            silent absence of children. Show a one-liner + a Retry so the
            operator can recover without reloading the whole panel. */}
        {subtree.error && (
          <div className="flex items-center justify-between gap-2 px-2 py-1 text-[11px] text-muted">
            <span className="truncate">couldn't list: {subtree.error}</span>
            <button
              onClick={subtree.reload}
              className="rounded border border-border px-1.5 py-0.5 text-[11px] hover:text-foreground"
              title={`Retry listing ${node.path}`}
            >
              retry
            </button>
          </div>
        )}
        {/* Tree's disclosure state once the fetch resolves. When a folder
            turns out to be empty, render an explicit "empty" line so the
            row above stays meaningful as a disclosure anchor. */}
        {!subtree.loading && !subtree.error && subtree.data !== null && childDirs.length === 0 && (
          <p className="px-2 py-1 text-[11px] text-muted" role="note">
            empty
          </p>
        )}
        {childDirs.map((child) => (
          // Key on the path so React can keep subtree state stable across
          // re-renders. The child's level is current + 1.
          <TreeRow key={child.path} node={child} current={current} onPick={onPick} level={level + 1} />
        ))}
      </div>
    )}
  </>
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
    // Reset every piece of detail state when the path switches, so a brief
    // mismatched render of the old file's content never flashes on screen.
    setErr(null);
    setContent(null);
    setSize(null);
    setMime("");
    setLoading(false);

    if (!isPathSafe(path)) {
      // Belt-and-braces: the UI uses a stub `entry` synthesised from the path,
      // but the route also rejects unsafe paths. Surface the rejection here
      // so the operator sees WHY the preview is empty instead of a stale one.
      setErr("Unsafe path");
      return () => {
        cancelled = true;
      };
    }
    if (!TEXT_LANGS.has(kind)) {
      // Binary/over-cap kinds already render an explicit "use Download" CTA via
      // the JSX below; there's nothing to fetch, so leave every detail state
      // cleared and return.
      return () => {
        cancelled = true;
      };
    }

    setLoading(true);
    fetch(rawFileURL(path), { headers: {} })
      .then((r) => {
        if (!r.ok) throw new Error(`HTTP ${r.status}`);
        const lenHeader = parseInt(r.headers.get("content-length") || "0", 10) || null;
        // Pre-flight cap check. If we already know the body is too big to
        // preview, refuse to call r.text() — saving a 50 MB round-trip on
        // huge log files. The "too_large:<n>" sentinel is how the render
        // block recognises this and shows the Download CTA.
        const pre = tooLargeReason(lenHeader);
        if (pre) {
          setSize(lenHeader);
          setErr(pre);
          return null;
        }
        setSize(lenHeader);
        setMime(r.headers.get("content-type") || "");
        return r.text();
      })
      .then((t) => {
        if (cancelled || t === null) return;
        // Post-body check: Content-Length lied, was missing, or the server
        // truncated it differently than we expected. Catches streaming
        // responses that didn't carry a header.
        const post = tooLargeReason(null, t.length);
        if (post) {
          setSize(t.length);
          setErr(post);
          return;
        }
        setContent(t);
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
        {/* "too_large:<bytes>" is a sentinel the useEffect sets after either a
            Content-Length pre-check or a post-body check fires; both indicate
            the file is past the inline preview cap and we should show the
            Download-CTA EmptyState instead of a raw error string. */}
        {err && !err.startsWith("too_large:") && <p className="py-6 text-center text-xs text-muted">{err}</p>}
        {!err && loading && (
          <p className="flex items-center justify-center gap-2 py-6 text-xs text-muted">
            <Loader2 className="size-3.5 animate-spin" /> loading…
          </p>
        )}
        {((err && err.startsWith("too_large:")) || (!err && !loading && content === null)) && (
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
