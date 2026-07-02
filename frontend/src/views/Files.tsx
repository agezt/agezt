import { useEffect, useMemo, useState } from "react";
import { FolderOpen, RefreshCw, Trash2, Download, ImageIcon, FileText, X, Loader2 } from "lucide-react";
import { usePanel } from "@/lib/usePanel";
import { authHeaders, postAction } from "@/lib/api";
import { cn, fmtTime } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { SkeletonGrid } from "@/components/ui/skeleton";
import { EmptyState } from "@/components/ui/empty";
import { ErrorText } from "@/components/JsonView";
import { Markdown } from "@/components/Markdown";
import { useUI } from "@/components/ui/feedback";
import { Page } from "@/components/ui/page";

// File manager (M823): browse/preview/download/delete the stored artifacts the
// daemon indexed (M822) — inbound channel images as a gallery, everything else
// as a list. Bytes come from the binary /api/artifact/raw route; metadata from
// /api/artifacts. Read + delete only; artifacts are content-addressed.

export interface ArtifactEntry {
  id: string;
  ref: string;
  name?: string;
  mime?: string;
  kind?: string;
  source?: string;
  sender?: string;
  corr?: string;
  size?: number;
  created_ms?: number;
  caption?: string;
}

interface ArtifactList {
  count: number;
  entries: ArtifactEntry[];
}

// isImage decides whether an entry renders as a gallery thumbnail.
export function isImage(e: ArtifactEntry): boolean {
  return e.kind === "image" || (e.mime ?? "").toLowerCase().startsWith("image/");
}

// isRunInternal flags an entry that is a RUN BYPRODUCT, not something a human or
// agent deliberately produced or uploaded: the agent loop offloads any large tool
// output (code-exec / introspect / shell / skill stdout > the artifact threshold)
// to the blob store and auto-indexes it as kind="tool-output", source="run". These
// pile up fast and drown the real files/artifacts. The galleries hide them by
// default (a toggle reveals them); the bytes stay retrievable by raw_ref from the
// run, so nothing is lost — they just don't clutter the human-facing view.
export function isRunInternal(e: ArtifactEntry): boolean {
  return e.kind === "tool-output" || e.source === "run";
}

// isPdf / textKind classify an entry for inline preview (M842). textKind returns
// "markdown" | "json" | "code" | "text" for text-like artifacts, or "" otherwise —
// driving how the preview pane renders the fetched bytes.
export function isPdf(e: ArtifactEntry): boolean {
  return (e.mime ?? "").toLowerCase().includes("pdf") || (e.name ?? "").toLowerCase().endsWith(".pdf");
}

export function textKind(e: ArtifactEntry): "markdown" | "json" | "code" | "text" | "" {
  const mime = (e.mime ?? "").toLowerCase();
  const name = (e.name ?? "").toLowerCase();
  const ext = name.includes(".") ? name.slice(name.lastIndexOf(".") + 1) : "";
  if (mime === "text/markdown" || ext === "md" || ext === "markdown") return "markdown";
  if (mime.includes("json") || ext === "json") return "json";
  if (
    ["js", "ts", "tsx", "jsx", "go", "py", "rs", "java", "c", "cpp", "h", "sh", "yaml", "yml", "toml", "css", "html", "xml", "sql"].includes(ext) ||
    mime.includes("javascript") ||
    mime.includes("yaml") ||
    mime.includes("xml")
  )
    return "code";
  if (mime.startsWith("text/") || ["txt", "log", "csv", "tsv", "ini", "env", "conf"].includes(ext)) return "text";
  // Unknown mime with a known-text-ish nothing → not previewable as text.
  return "";
}

// categoryOf buckets an entry for the Artifacts gallery (M931): the file types
// agents actually produce, each with its own preview treatment. Checked before
// textKind so html/svg get their dedicated buckets rather than "code"/"image".
export type ArtifactCategory = "image" | "svg" | "html" | "pdf" | "markdown" | "json" | "code" | "text" | "other";

export function categoryOf(e: ArtifactEntry): ArtifactCategory {
  const mime = (e.mime ?? "").toLowerCase();
  const name = (e.name ?? "").toLowerCase();
  const ext = name.includes(".") ? name.slice(name.lastIndexOf(".") + 1) : "";
  if (mime.includes("svg") || ext === "svg") return "svg";
  if (isImage(e)) return "image";
  if (mime === "text/html" || ext === "html" || ext === "htm") return "html";
  if (isPdf(e)) return "pdf";
  const t = textKind(e);
  return t === "" ? "other" : t;
}

// previewMaxBytes caps inline text fetches so a giant artifact can't hang the UI.
export const previewMaxBytes = 2 * 1024 * 1024;

// rawURL builds the binary URL for an entry. Fetch callers attach bearer auth;
// browser navigations must use a Blob URL produced by fetchRawBlobURL instead.
export function rawURL(e: ArtifactEntry, download = false): string {
  const params = new URLSearchParams({ ref: e.ref });
  if (e.mime) params.set("mime", e.mime);
  if (download) {
    params.set("download", "1");
    params.set("name", e.name || `${e.kind || "artifact"}-${e.id}`);
  }
  return `/api/artifact/raw?${params.toString()}`;
}

async function fetchRawBlob(e: ArtifactEntry, download = false): Promise<Blob> {
  const res = await fetch(rawURL(e, download), { headers: authHeaders() });
  if (!res.ok) throw new Error(`HTTP ${res.status}`);
  return res.blob();
}

export async function downloadArtifact(e: ArtifactEntry): Promise<void> {
  const blob = await fetchRawBlob(e, true);
  const href = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = href;
  a.download = e.name || `${e.kind || "artifact"}-${e.id}`;
  document.body.appendChild(a);
  a.click();
  a.remove();
  URL.revokeObjectURL(href);
}

export function BlobArtifact({ entry, kind, alt, title, className }: { entry: ArtifactEntry; kind: "image" | "pdf"; alt?: string; title?: string; className?: string }) {
  const [href, setHref] = useState<string | null>(null);
  const [err, setErr] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    let objectURL = "";
    setHref(null);
    setErr(null);
    fetchRawBlob(entry)
      .then((blob) => {
        if (cancelled) return;
        objectURL = URL.createObjectURL(blob);
        setHref(objectURL);
      })
      .catch((e) => !cancelled && setErr((e as Error).message));
    return () => {
      cancelled = true;
      if (objectURL) URL.revokeObjectURL(objectURL);
    };
  }, [entry]);

  if (err) return <p className="py-6 text-center text-sm text-muted">{err}</p>;
  if (!href) {
    return (
      <p className="flex items-center justify-center gap-2 py-6 text-sm text-muted">
        <Loader2 className="size-4 animate-spin" /> loading preview…
      </p>
    );
  }
  if (kind === "pdf") return <iframe src={href} title={title || entry.name || "pdf"} className={className} />;
  return <img src={href} alt={alt || entry.caption || entry.name || "image"} className={className} loading="lazy" />;
}

function humanSize(n?: number): string {
  if (!n || n <= 0) return "—";
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`;
  return `${(n / (1024 * 1024)).toFixed(1)} MB`;
}

export function Files() {
  const ui = useUI();
  const { data, error, loading, reload } = usePanel<ArtifactList>("/api/artifacts");
  const [filter, setFilter] = useState<"all" | "images" | "files">("all");
  const [showRuns, setShowRuns] = useState(false);
  const [preview, setPreview] = useState<ArtifactEntry | null>(null);

  const allEntries = useMemo(() => data?.entries ?? [], [data]);
  const runCount = useMemo(() => allEntries.filter(isRunInternal).length, [allEntries]);
  // Hide run byproducts (offloaded tool outputs) unless explicitly revealed, so
  // the manager shows real files/uploads — not the run-internal txt flood.
  const entries = useMemo(() => (showRuns ? allEntries : allEntries.filter((e) => !isRunInternal(e))), [allEntries, showRuns]);
  const shown = useMemo(() => {
    if (filter === "images") return entries.filter(isImage);
    if (filter === "files") return entries.filter((e) => !isImage(e));
    return entries;
  }, [entries, filter]);

  const images = shown.filter(isImage);
  const files = shown.filter((e) => !isImage(e));

  // collect reaps stale artifacts (M845): a dry-run reports the candidates, then a
  // confirm actually deletes them — the operator's "approved" path.
  const COLLECT_DAYS = 30;
  async function collect() {
    try {
      const dry = await postAction<{ count: number; bytes: number }>("/api/artifact/collect", {
        older_than_days: String(COLLECT_DAYS),
        dry_run: "true",
      });
      if (!dry.count) {
        ui.toast(`Nothing to collect — no files older than ${COLLECT_DAYS} days.`, "success");
        return;
      }
      const ok = await ui.confirm({
        title: `Collect ${dry.count} stale file${dry.count === 1 ? "" : "s"}?`,
        message: `Permanently delete artifacts older than ${COLLECT_DAYS} days (~${humanSize(dry.bytes)}). The most recent files are kept.`,
        confirmLabel: "Collect",
        danger: true,
      });
      if (!ok) return;
      const res = await postAction<{ count: number; bytes: number }>("/api/artifact/collect", {
        older_than_days: String(COLLECT_DAYS),
        dry_run: "false",
      });
      ui.toast(`Collected ${res.count} file${res.count === 1 ? "" : "s"} (~${humanSize(res.bytes)}).`, "success");
      reload();
    } catch (e) {
      ui.toast((e as Error).message, "error");
    }
  }

  async function del(e: ArtifactEntry) {
    const ok = await ui.confirm({
      title: "Delete artifact?",
      message: e.name || e.id,
      confirmLabel: "Delete",
      danger: true,
    });
    if (!ok) return;
    try {
      await postAction("/api/artifact/delete", { id: e.id });
      ui.toast("artifact deleted", "success");
      if (preview?.id === e.id) setPreview(null);
      reload();
    } catch (err) {
      ui.toast((err as Error).message, "error");
    }
  }

  return (
    <Page
      icon={FolderOpen}
      title="Files"
      width="full"
      description={`${entries.length} stored`}
      actions={
        <>
            <div className="flex items-center gap-1">
              {(["all", "images", "files"] as const).map((f) => (
                <button
                  key={f}
                  onClick={() => setFilter(f)}
                  className={cn(
                    "rounded-full border px-2.5 py-0.5 text-[11px] capitalize transition-colors",
                    filter === f ? "border-accent bg-accent/10 text-accent" : "border-border text-muted hover:text-foreground",
                  )}
                >
                  {f}
                </button>
              ))}
            </div>
            {runCount > 0 && (
              <button
                onClick={() => setShowRuns((v) => !v)}
                className={cn(
                  "rounded-full border px-2.5 py-0.5 text-[11px] transition-colors",
                  showRuns ? "border-accent bg-accent/10 text-accent" : "border-border text-muted hover:text-foreground",
                )}
                title="Offloaded tool/run outputs are hidden by default — they're recoverable from each run"
              >
                {showRuns ? "Hide" : "Show"} run outputs ({runCount})
              </button>
            )}
            <Button variant="ghost" size="sm" onClick={collect} disabled={loading || entries.length === 0} title={`Collect stale files (older than ${COLLECT_DAYS} days)`}>
              <Trash2 className="size-3.5" /> Collect
            </Button>
            <Button variant="ghost" size="sm" onClick={reload} disabled={loading} title="Reload">
              <RefreshCw className={cn("size-3.5", loading && "animate-spin")} />
            </Button>
          </>
        }
    >

      {error && <ErrorText>{error}</ErrorText>}
      {loading && entries.length === 0 && <SkeletonGrid count={8} />}

      {!loading && entries.length === 0 && !error && (
        <EmptyState
          icon={FolderOpen}
          title={runCount > 0 ? "No files — only run outputs" : "No stored files yet"}
          hint={
            runCount > 0
              ? `${runCount} offloaded run/tool output${runCount === 1 ? " is" : "s are"} hidden. Use “Show run outputs” above to browse them.`
              : "Images sent to the bot over a channel (Telegram, Slack, Discord) are saved here automatically."
          }
        />
      )}

      <div className="space-y-4">
        {images.length > 0 && (
          <div>
            <div className="mb-1.5 flex items-center gap-1.5 text-[11px] font-semibold uppercase tracking-normal text-muted">
              <ImageIcon className="size-3" /> Images ({images.length})
            </div>
            <div className="grid grid-cols-[repeat(auto-fill,minmax(120px,1fr))] gap-2">
              {images.map((e) => (
                <button
                  key={e.id}
                  onClick={() => setPreview(e)}
                  className="group relative aspect-square overflow-hidden rounded-lg border border-border bg-panel"
                  title={e.caption || e.name || e.id}
                >
                  <BlobArtifact entry={e} kind="image" alt={e.caption || e.name || "image"} className="size-full object-cover" />
                  <span className="absolute inset-x-0 bottom-0 truncate bg-black/55 px-1.5 py-0.5 text-left text-xs text-white/90">
                    {e.source || "file"} · {fmtTime(e.created_ms)}
                  </span>
                </button>
              ))}
            </div>
          </div>
        )}

        {files.length > 0 && (
          <div>
            <div className="mb-1.5 flex items-center gap-1.5 text-[11px] font-semibold uppercase tracking-normal text-muted">
              <FileText className="size-3" /> Files ({files.length})
            </div>
            <ul className="space-y-1">
              {files.map((e) => (
                <li
                  key={e.id}
                  onClick={() => setPreview(e)}
                  className="glass flex cursor-pointer items-center gap-2 rounded-xl px-3 py-2 text-sm transition-colors hover:border-accent/50"
                  title="Click to preview"
                >
                  <FileText className="size-4 shrink-0 text-muted" />
                  <span className="min-w-0 flex-1 truncate" title={e.name || e.id}>
                    {e.name || e.id}
                  </span>
                  <Badge variant="accent">{categoryOf(e)}</Badge>
                  {e.kind && e.kind !== "file" && <Badge variant="default">{e.kind}</Badge>}
                  <span className="text-[11px] text-muted">{humanSize(e.size)}</span>
                  <span className="text-[11px] text-muted">{fmtTime(e.created_ms)}</span>
                  <button
                    onClick={(ev) => {
                      ev.stopPropagation();
                      downloadArtifact(e).catch((err) => ui.toast((err as Error).message, "error"));
                    }}
                    className="text-muted hover:text-accent"
                    title="Download"
                  >
                    <Download className="size-3.5" />
                  </button>
                  <button
                    onClick={(ev) => {
                      ev.stopPropagation();
                      del(e);
                    }}
                    className="text-muted hover:text-bad"
                    title="Delete"
                  >
                    <Trash2 className="size-3.5" />
                  </button>
                </li>
              ))}
            </ul>
          </div>
        )}
      </div>

      {preview && <PreviewModal entry={preview} onClose={() => setPreview(null)} onDelete={() => del(preview)} />}
    </Page>
  );
}

function PreviewModal({
  entry,
  onClose,
  onDelete,
}: {
  entry: ArtifactEntry;
  onClose: () => void;
  onDelete: () => void;
}) {
  return (
    <div
      className="fixed inset-0 z-[200] flex items-center justify-center bg-black/70 p-4 backdrop-blur-sm"
      onClick={onClose}
    >
      <div
        className="glass flex max-h-[90vh] w-full max-w-2xl flex-col overflow-hidden rounded-xl shadow-2xl"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-center gap-2 border-b border-border px-4 py-2">
          <span className="min-w-0 flex-1 truncate text-sm font-semibold">{entry.name || entry.id}</span>
          <button onClick={() => downloadArtifact(entry).catch(console.error)} className="text-muted hover:text-accent" title="Download">
            <Download className="size-4" />
          </button>
          <button onClick={onDelete} className="text-muted hover:text-bad" title="Delete">
            <Trash2 className="size-4" />
          </button>
          <button onClick={onClose} className="text-muted hover:text-foreground" aria-label="Close preview">
            <X className="size-4" />
          </button>
        </div>
        <div className="min-h-0 flex-1 overflow-auto bg-panel/40 p-3">
          <PreviewBody entry={entry} />
        </div>
        <div className="space-y-1 border-t border-border px-4 py-2 text-[11px] text-muted">
          <div className="flex flex-wrap gap-x-4 gap-y-0.5">
            {entry.source && <span>source: {entry.source}</span>}
            {entry.sender && <span>from: {entry.sender}</span>}
            {entry.mime && <span>{entry.mime}</span>}
            <span>{humanSize(entry.size)}</span>
            <span>{fmtTime(entry.created_ms)}</span>
          </div>
          {entry.caption && <div className="text-foreground/80">“{entry.caption}”</div>}
        </div>
      </div>
    </div>
  );
}

// PreviewBody renders an artifact inline (M842): images and SVG as a picture, PDFs
// in an embedded frame, and text/markdown/code/json/csv by fetching the bytes and
// rendering them — falling back to a download prompt only for true binaries.
function PreviewBody({ entry }: { entry: ArtifactEntry }) {
  const kind = textKind(entry);
  const [text, setText] = useState<string | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    if (isImage(entry) || isPdf(entry) || kind === "") return;
    if ((entry.size ?? 0) > previewMaxBytes) {
      setErr(`File is too large to preview inline (${humanSize(entry.size)}) — download it.`);
      return;
    }
    let cancelled = false;
    setLoading(true);
    fetch(rawURL(entry), { headers: authHeaders() })
      .then((r) => (r.ok ? r.text() : Promise.reject(new Error(`HTTP ${r.status}`))))
      .then((t) => {
        if (cancelled) return;
        setText(kind === "json" ? prettyJSON(t) : t);
      })
      .catch((e) => !cancelled && setErr((e as Error).message))
      .finally(() => !cancelled && setLoading(false));
    return () => {
      cancelled = true;
    };
  }, [entry, kind]);

  if (isImage(entry)) {
    return <BlobArtifact entry={entry} kind="image" alt={entry.caption || entry.name || "image"} className="mx-auto max-h-[68vh] rounded-md" />;
  }
  if (isPdf(entry)) {
    return <BlobArtifact entry={entry} kind="pdf" title={entry.name || "pdf"} className="h-[68vh] w-full rounded-md border border-border bg-white" />;
  }
  if (kind === "") {
    return <p className="py-12 text-center text-sm text-muted">No inline preview for this file type — use Download.</p>;
  }
  if (err) return <p className="py-12 text-center text-sm text-muted">{err}</p>;
  if (loading || text === null) {
    return (
      <p className="flex items-center justify-center gap-2 py-12 text-sm text-muted">
        <Loader2 className="size-4 animate-spin" /> loading preview…
      </p>
    );
  }
  if (kind === "markdown") {
    return <Markdown source={text} className="prose-sm max-w-none text-sm text-foreground/90" />;
  }
  return (
    <pre className="overflow-auto whitespace-pre-wrap break-words rounded-md bg-card p-3 text-left font-mono text-xs leading-relaxed text-foreground/90">
      {text}
    </pre>
  );
}

// prettyJSON re-indents valid JSON; returns the original text if it doesn't parse.
function prettyJSON(s: string): string {
  try {
    return JSON.stringify(JSON.parse(s), null, 2);
  } catch {
    return s;
  }
}
