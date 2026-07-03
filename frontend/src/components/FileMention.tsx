import { useEffect, useState } from "react";
import { Download, FileText, Loader2, X } from "lucide-react";
import { fetchFileBlob, isPathSafe, rawFileURL } from "@/lib/files";
import { Badge } from "@/components/ui/badge";
import { MonacoView } from "@/components/MonacoView";

// FileMention is a clickable chip used by Chat's Markdown for any token the
// parser flagged as `t: "file"`. Click → opens the FileDetail modal so the
// operator can read the file in a single click, without leaving the chat
// surface. Behaviour mirrors a small subset of the full File Manager: read
// bytes, show a fallback when the backend (Slice 5) isn't answering yet,
// and offer an "Open in File Manager" escape hatch for bigger files.

// FileMention keeps it cheap: a single render of the chip, no modal logic
// inline. The Modal is its own component so tests can mount either in
// isolation.
export function FileMention({ path }: { path: string }) {
  const [open, setOpen] = useState(false);
  if (!isPathSafe(path)) {
    // Unsafe paths render as inert, dimmed text — the parser already filtered
    // them but defence in depth: the chat input must never be an attack path.
    return (
      <span className="cursor-not-allowed font-mono text-xs text-muted line-through" title={`unsafe path: ${path}`}>
        {path}
      </span>
    );
  }
  const leaf = path.slice(path.lastIndexOf("/") + 1) || path;
  return (
    <>
      <button
        onClick={(e) => {
          e.stopPropagation();
          setOpen(true);
        }}
        className="inline-flex items-center gap-1 rounded border border-border bg-panel/60 px-1.5 py-0.5 font-mono text-xs text-accent transition-colors hover:border-accent hover:text-foreground"
        title={`Open ${path} in the file viewer`}
      >
        <FileText className="size-3" /> {leaf}
      </button>
      {open && <FileDetail path={path} onClose={() => setOpen(false)} />}
    </>
  );
}

// FileDetail fetches the file via /api/files/raw and renders a small inline
// preview. Slice 5's Go route is the real source; before it lands /api/files/raw
// 404s and we show an "Open in File Manager" fallback that hops to the
// workspace route.
const TEXT_CAP = 256 * 1024; // 256 KB inline, anything bigger → download only

export function FileDetail({ path, onClose }: { path: string; onClose: () => void }) {
  const [content, setContent] = useState<string | null>(null);
  const [size, setSize] = useState<number | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setErr(null);
    setContent(null);
    setSize(null);
    (async () => {
      try {
        const res = await fetch(rawFileURL(path), { headers: {} });
        if (!res.ok) {
          if (res.status === 404) {
            // Slice 5 not present yet — stay quiet and offer the WS route.
            throw new Error("not_found");
          }
          throw new Error(`HTTP ${res.status}`);
        }
        setSize(parseInt(res.headers.get("content-length") || "0", 10) || null);
        if ((parseInt(res.headers.get("content-length") || "0", 10) || 0) > TEXT_CAP) {
          // Too big to preview inline — keep `loading` off and stay download-only.
          if (!cancelled) setLoading(false);
          return;
        }
        const t = await res.text();
        if (!cancelled) setContent(t);
      } catch (e: unknown) {
        if (cancelled) return;
        setErr(e instanceof Error ? e.message : String(e));
      } finally {
        if (!cancelled) setLoading(false);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [path]);

  const openInFileManager = () => {
    window.location.hash = `files?path=${encodeURIComponent(path)}`;
    onClose();
  };

  const download = async () => {
    try {
      const blob = await fetchFileBlob(path);
      const href = URL.createObjectURL(blob);
      const a = document.createElement("a");
      a.href = href;
      a.download = path.slice(path.lastIndexOf("/") + 1) || path;
      document.body.appendChild(a);
      a.click();
      a.remove();
      URL.revokeObjectURL(href);
    } catch (e: unknown) {
      setErr(e instanceof Error ? e.message : String(e));
    }
  };

  const tooLarge = size !== null && size > TEXT_CAP;
  const leaf = path.slice(path.lastIndexOf("/") + 1) || path;

  return (
    <div
      className="fixed inset-0 z-[200] flex items-center justify-center bg-black/70 p-4 backdrop-blur-sm"
      onClick={onClose}
      role="dialog"
      aria-modal="true"
      aria-label={`File detail: ${path}`}
    >
      <div
        className="glass flex max-h-[85vh] w-full max-w-3xl flex-col overflow-hidden rounded-xl shadow-2xl"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-center gap-2 border-b border-border px-4 py-2">
          <Badge variant="accent">file</Badge>
          <span className="min-w-0 flex-1 truncate font-mono text-xs" title={path}>
            {path}
          </span>
          <button
            onClick={openInFileManager}
            className="inline-flex items-center gap-1 rounded border border-border px-2 py-0.5 text-xs text-muted hover:text-foreground"
            title="Open in the File Manager workspace"
          >
            file manager
          </button>
          <button
            onClick={download}
            className="text-muted hover:text-accent"
            title="Download"
            aria-label="Download file"
          >
            <Download className="size-4" />
          </button>
          <button onClick={onClose} className="text-muted hover:text-foreground" aria-label="Close detail">
            <X className="size-4" />
          </button>
        </div>
        <div className="min-h-0 flex-1 overflow-auto bg-panel/40 p-3">
          {loading && (
            <p className="flex items-center justify-center gap-2 py-6 text-xs text-muted">
              <Loader2 className="size-3.5 animate-spin" /> loading {leaf}…
            </p>
          )}
          {!loading && err === "not_found" && (
            <p className="py-6 text-center text-xs text-muted">
              <span className="block">
                The file API isn't responding yet (route lands in a later release).
              </span>
              <button
                onClick={openInFileManager}
                className="mt-2 inline-flex items-center gap-1 rounded border border-border px-2 py-0.5 text-xs text-accent hover:border-accent"
              >
                open the File Manager
              </button>
            </p>
          )}
          {!loading && err && err !== "not_found" && (
            <p className="py-6 text-center text-xs text-muted">{err}</p>
          )}
          {!loading && !err && tooLarge && (
            <p className="py-6 text-center text-xs text-muted">
              File is too large to preview inline — use Download.
            </p>
          )}
          {!loading && !err && !tooLarge && content !== null && (
            <MonacoView
              value={content}
              path={path}
              readOnly
              height={Math.min(420, content.split("\n").length * 18 + 60)}
            />
          )}
        </div>
      </div>
    </div>
  );
}
