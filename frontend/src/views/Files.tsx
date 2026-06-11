import { useMemo, useState } from "react";
import { FolderOpen, RefreshCw, Trash2, Download, ImageIcon, FileText, X } from "lucide-react";
import { usePanel } from "@/lib/usePanel";
import { withToken, postAction } from "@/lib/api";
import { cn, fmtTime } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { SkeletonGrid } from "@/components/ui/skeleton";
import { EmptyState } from "@/components/ui/empty";
import { ErrorText } from "@/components/JsonView";
import { useUI } from "@/components/ui/feedback";

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

// rawURL builds the tokenized binary URL for an entry (optionally a download).
export function rawURL(e: ArtifactEntry, download = false): string {
  const params: Record<string, string> = { ref: e.ref };
  if (e.mime) params.mime = e.mime;
  if (download) {
    params.download = "1";
    params.name = e.name || `${e.kind || "artifact"}-${e.id}`;
  }
  return withToken("/api/artifact/raw", params);
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
  const [preview, setPreview] = useState<ArtifactEntry | null>(null);

  const entries = useMemo(() => data?.entries ?? [], [data]);
  const shown = useMemo(() => {
    if (filter === "images") return entries.filter(isImage);
    if (filter === "files") return entries.filter((e) => !isImage(e));
    return entries;
  }, [entries, filter]);

  const images = shown.filter(isImage);
  const files = shown.filter((e) => !isImage(e));

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
    <div className="flex h-full min-h-0 flex-col gap-3">
      <div className="flex items-center gap-2">
        <h2 className="flex items-center gap-2 text-sm font-semibold">
          <FolderOpen className="size-4 text-accent" /> Files
        </h2>
        <span className="text-xs text-muted">{entries.length} stored</span>
        <div className="ml-2 flex items-center gap-1">
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
        <Button variant="ghost" size="sm" className="ml-auto" onClick={reload} disabled={loading} title="Reload">
          <RefreshCw className={cn("size-3.5", loading && "animate-spin")} />
        </Button>
      </div>

      {error && <ErrorText>{error}</ErrorText>}
      {loading && entries.length === 0 && <SkeletonGrid count={8} />}

      {!loading && entries.length === 0 && !error && (
        <EmptyState
          icon={FolderOpen}
          title="No stored files yet"
          hint="Images sent to the bot over a channel (Telegram, Slack, Discord) are saved here automatically."
        />
      )}

      <div className="min-h-0 flex-1 space-y-4 overflow-auto">
        {images.length > 0 && (
          <div>
            <div className="mb-1.5 flex items-center gap-1.5 text-[11px] font-semibold uppercase tracking-wide text-muted">
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
                  <img src={rawURL(e)} alt={e.caption || e.name || "image"} className="size-full object-cover" loading="lazy" />
                  <span className="absolute inset-x-0 bottom-0 truncate bg-black/55 px-1.5 py-0.5 text-left text-[10px] text-white/90">
                    {e.source || "file"} · {fmtTime(e.created_ms)}
                  </span>
                </button>
              ))}
            </div>
          </div>
        )}

        {files.length > 0 && (
          <div>
            <div className="mb-1.5 flex items-center gap-1.5 text-[11px] font-semibold uppercase tracking-wide text-muted">
              <FileText className="size-3" /> Files ({files.length})
            </div>
            <ul className="space-y-1">
              {files.map((e) => (
                <li key={e.id} className="flex items-center gap-2 rounded-lg border border-border bg-card px-3 py-2 text-sm">
                  <FileText className="size-4 shrink-0 text-muted" />
                  <span className="min-w-0 flex-1 truncate" title={e.name || e.id}>
                    {e.name || e.id}
                  </span>
                  {e.kind && <Badge variant="default">{e.kind}</Badge>}
                  <span className="text-[11px] text-muted">{humanSize(e.size)}</span>
                  <span className="text-[11px] text-muted">{fmtTime(e.created_ms)}</span>
                  <a href={rawURL(e, true)} download className="text-muted hover:text-accent" title="Download">
                    <Download className="size-3.5" />
                  </a>
                  <button onClick={() => del(e)} className="text-muted hover:text-bad" title="Delete">
                    <Trash2 className="size-3.5" />
                  </button>
                </li>
              ))}
            </ul>
          </div>
        )}
      </div>

      {preview && <PreviewModal entry={preview} onClose={() => setPreview(null)} onDelete={() => del(preview)} />}
    </div>
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
        className="flex max-h-[90vh] w-full max-w-2xl flex-col overflow-hidden rounded-xl border border-border bg-card shadow-2xl"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-center gap-2 border-b border-border px-4 py-2">
          <span className="min-w-0 flex-1 truncate text-sm font-semibold">{entry.name || entry.id}</span>
          <a href={rawURL(entry, true)} download className="text-muted hover:text-accent" title="Download">
            <Download className="size-4" />
          </a>
          <button onClick={onDelete} className="text-muted hover:text-bad" title="Delete">
            <Trash2 className="size-4" />
          </button>
          <button onClick={onClose} className="text-muted hover:text-foreground" aria-label="Close preview">
            <X className="size-4" />
          </button>
        </div>
        <div className="min-h-0 flex-1 overflow-auto bg-panel/40 p-3 text-center">
          {isImage(entry) ? (
            <img src={rawURL(entry)} alt={entry.caption || entry.name || "image"} className="mx-auto max-h-[60vh] rounded-md" />
          ) : (
            <p className="py-12 text-sm text-muted">No inline preview — use Download.</p>
          )}
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
