import { useEffect, useMemo, useState } from "react";
import {
  Shapes,
  RefreshCw,
  Download,
  Trash2,
  X,
  Maximize2,
  Minimize2,
  Loader2,
  ImageIcon,
  FileCode2,
  FileJson2,
  FileText,
  FileType2,
  Globe,
  PenLine,
  File as FileIcon,
  Search,
  FolderTree,
  type LucideIcon,
} from "lucide-react";
import { usePanel } from "@/lib/usePanel";
import { authHeaders, postAction } from "@/lib/api";
import { cn, fmtTime } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { SkeletonGrid } from "@/components/ui/skeleton";
import { EmptyState } from "@/components/ui/empty";
import { Page } from "@/components/ui/page";
import { LoadMoreFooter } from "@/components/ui/load-more-footer";
import { ErrorText } from "@/components/JsonView";
import { Markdown } from "@/components/Markdown";
import { Modal } from "@/components/ui/Modal";
import { MonacoView } from "@/components/MonacoView";
import { useUI } from "@/components/ui/feedback";
import { goToView } from "@/lib/nav";
import {
  type ArtifactEntry,
  type ArtifactCategory,
  categoryOf,
  rawURL,
  previewMaxBytes,
  isRunInternal,
  BlobArtifact,
  downloadArtifact,
} from "./Files";

// Artifacts gallery (M931): every kind of agent output, bucketed by what it IS
// (image / svg / html / markdown / json / code / pdf / text), each with its own
// preview treatment — HTML renders live in a sandboxed frame, markdown renders
// formatted, images as pictures — and a fullscreen viewer for the big-screen
// look. Files (M823) stays the flat manager; this is the showroom.

interface ArtifactList {
  count: number;
  entries: ArtifactEntry[];
}

export const CATEGORY_META: { key: ArtifactCategory; label: string; icon: LucideIcon }[] = [
  { key: "image", label: "Images", icon: ImageIcon },
  { key: "svg", label: "SVG", icon: Shapes },
  { key: "html", label: "HTML", icon: Globe },
  { key: "markdown", label: "Markdown", icon: PenLine },
  { key: "json", label: "JSON", icon: FileJson2 },
  { key: "code", label: "Code", icon: FileCode2 },
  { key: "pdf", label: "PDF", icon: FileType2 },
  { key: "text", label: "Text", icon: FileText },
  { key: "other", label: "Other", icon: FileIcon },
];

// groupByCategory buckets entries in CATEGORY_META order, dropping empty buckets.
export function groupByCategory(entries: ArtifactEntry[]): { key: ArtifactCategory; entries: ArtifactEntry[] }[] {
  const buckets = new Map<ArtifactCategory, ArtifactEntry[]>();
  for (const e of entries) {
    const c = categoryOf(e);
    const list = buckets.get(c);
    if (list) list.push(e);
    else buckets.set(c, [e]);
  }
  return CATEGORY_META.filter((m) => buckets.has(m.key)).map((m) => ({ key: m.key, entries: buckets.get(m.key)! }));
}

// matchesQuery does a cheap case-insensitive match over the fields a human
// would search by: name, caption, source, sender.
export function matchesQuery(e: ArtifactEntry, q: string): boolean {
  if (!q) return true;
  const needle = q.toLowerCase();
  return [e.name, e.caption, e.source, e.sender].some((f) => (f ?? "").toLowerCase().includes(needle));
}

// ARTIFACT_CARD_WINDOW caps how many artifact tiles render at once.
// /api/artifacts has no cursor (kind/source/corr filters only), so the list
// arrives whole — the window keeps a big gallery from ballooning the DOM and
// grows client-side via the Load-more footer. Section headers and category
// chips keep the FULL counts; only the rendered tiles are windowed.
const ARTIFACT_CARD_WINDOW = 60;

// windowGroups slices grouped entries down to a total budget of `limit` cards,
// preserving group order, and drops groups that end up empty.
function windowGroups(
  groups: { key: ArtifactCategory; entries: ArtifactEntry[] }[],
  limit: number,
): { key: ArtifactCategory; entries: ArtifactEntry[]; total: number }[] {
  let budget = limit;
  const out: { key: ArtifactCategory; entries: ArtifactEntry[]; total: number }[] = [];
  for (const g of groups) {
    if (budget <= 0) break;
    const take = Math.min(g.entries.length, budget);
    budget -= take;
    out.push({ key: g.key, entries: g.entries.slice(0, take), total: g.entries.length });
  }
  return out;
}

function humanSize(n?: number): string {
  if (!n || n <= 0) return "—";
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`;
  return `${(n / (1024 * 1024)).toFixed(1)} MB`;
}

export function Artifacts() {
  const ui = useUI();
  const { data, error, loading, reload } = usePanel<ArtifactList>("/api/artifacts");
  const [cat, setCat] = useState<ArtifactCategory | "all">("all");
  const [query, setQuery] = useState("");
  const [showRuns, setShowRuns] = useState(false);
  const [viewing, setViewing] = useState<ArtifactEntry | null>(null);
  const [win, setWin] = useState(ARTIFACT_CARD_WINDOW);

  // Reset the render window whenever a filter changes so the gallery starts
  // from the top of the newly-filtered set.
  useEffect(() => {
    setWin(ARTIFACT_CARD_WINDOW);
  }, [cat, query, showRuns]);

  const allEntries = useMemo(() => data?.entries ?? [], [data]);
  const runCount = useMemo(() => allEntries.filter(isRunInternal).length, [allEntries]);
  // The gallery is the showroom of deliberate products — hide offloaded run/tool
  // outputs by default so they don't bury the real artifacts (toggle to reveal).
  const entries = useMemo(() => (showRuns ? allEntries : allEntries.filter((e) => !isRunInternal(e))), [allEntries, showRuns]);
  const searched = useMemo(() => entries.filter((e) => matchesQuery(e, query)), [entries, query]);
  const groups = useMemo(() => groupByCategory(searched), [searched]);
  const shownGroups = cat === "all" ? groups : groups.filter((g) => g.key === cat);

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
      if (viewing?.id === e.id) setViewing(null);
      reload();
    } catch (err) {
      ui.toast((err as Error).message, "error");
    }
  }

  return (
    <Page
      icon={Shapes}
      title="Artifacts"
      width="full"
      description={`${entries.length} produced`}
      actions={
        <>
            <div className="relative">
              <Search className="pointer-events-none absolute left-2 top-1/2 size-3.5 -translate-y-1/2 text-muted" />
              <input
                value={query}
                onChange={(e) => setQuery(e.target.value)}
                placeholder="search name, caption, source…"
                className="w-56 rounded-full border border-border bg-panel py-1 pl-7 pr-3 text-xs text-foreground placeholder:text-muted"
              />
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
            <Button variant="ghost" size="sm" onClick={reload} disabled={loading} title="Reload">
              <RefreshCw className={cn("size-3.5", loading && "animate-spin")} />
            </Button>
          </>
        }
    >

      {/* Category chips with live counts */}
      <div className="flex flex-wrap items-center gap-1">
        <CategoryChip active={cat === "all"} label="All" count={searched.length} onClick={() => setCat("all")} />
        {groups.map((g) => {
          const meta = CATEGORY_META.find((m) => m.key === g.key)!;
          return (
            <CategoryChip
              key={g.key}
              active={cat === g.key}
              label={meta.label}
              icon={meta.icon}
              count={g.entries.length}
              onClick={() => setCat(cat === g.key ? "all" : g.key)}
            />
          );
        })}
      </div>

      {error && <ErrorText>{error}</ErrorText>}
      {loading && entries.length === 0 && <SkeletonGrid count={8} />}

      {!loading && entries.length === 0 && !error && (
        <EmptyState
          icon={Shapes}
          title={runCount > 0 ? "No artifacts — only run outputs" : "No artifacts yet"}
          hint={
            runCount > 0
              ? `${runCount} offloaded run/tool output${runCount === 1 ? " is" : "s are"} hidden. Use “Show run outputs” above to browse them.`
              : "Everything your agents produce — reports, charts, pages, code — lands here, bucketed by type."
          }
        />
      )}

      <div className="space-y-5 pr-1">
        {windowGroups(shownGroups, win).map((g) => {
          const meta = CATEGORY_META.find((m) => m.key === g.key)!;
          const Icon = meta.icon;
          return (
            <section key={g.key}>
              <div className="mb-1.5 flex items-center gap-1.5 text-[11px] font-semibold uppercase tracking-normal text-muted">
                <Icon className="size-3" /> {meta.label} ({g.total})
              </div>
              <div className="grid grid-cols-[repeat(auto-fill,minmax(150px,1fr))] gap-2">
                {g.entries.map((e) => (
                  <ArtifactCard key={e.id} entry={e} category={g.key} onOpen={() => setViewing(e)} />
                ))}
              </div>
            </section>
          );
        })}
        {(() => {
          const totalCards = shownGroups.reduce((s, g) => s + g.entries.length, 0);
          return totalCards > ARTIFACT_CARD_WINDOW ? (
            <LoadMoreFooter
              hasMore={win < totalCards}
              loadingMore={false}
              onLoadMore={() => setWin((w) => w + ARTIFACT_CARD_WINDOW)}
              pageSize={Math.min(ARTIFACT_CARD_WINDOW, Math.max(1, totalCards - win))}
              label="artifacts"
            />
          ) : null;
        })()}
        {!loading && entries.length > 0 && shownGroups.length === 0 && (
          <p className="py-10 text-center text-sm text-muted">Nothing matches the search.</p>
        )}
      </div>

      {viewing && <Viewer entry={viewing} onClose={() => setViewing(null)} onDelete={() => del(viewing)} />}
    </Page>
  );
}

function CategoryChip({
  active,
  label,
  count,
  icon: Icon,
  onClick,
}: {
  active: boolean;
  label: string;
  count: number;
  icon?: LucideIcon;
  onClick: () => void;
}) {
  return (
    <button
      onClick={onClick}
      className={cn(
        "flex items-center gap-1 rounded-full border px-2.5 py-0.5 text-[11px] transition-colors",
        active ? "border-accent bg-accent/10 text-accent" : "border-border text-muted hover:text-foreground",
      )}
    >
      {Icon && <Icon className="size-3" />}
      {label}
      <span className={cn("rounded-full px-1 text-xs", active ? "bg-accent/20" : "bg-panel")}>{count}</span>
    </button>
  );
}

// ArtifactCard — visual tile: pictures show themselves; everything else shows
// its type icon over the name, so the wall reads at a glance.
function ArtifactCard({ entry, category, onOpen }: { entry: ArtifactEntry; category: ArtifactCategory; onOpen: () => void }) {
  const meta = CATEGORY_META.find((m) => m.key === category)!;
  const Icon = meta.icon;
  const pictorial = category === "image" || category === "svg";
  return (
    <button
      onClick={onOpen}
      className="group relative flex aspect-square flex-col overflow-hidden rounded-lg border border-border bg-panel text-left transition-colors hover:border-accent/60"
      title={entry.caption || entry.name || entry.id}
    >
      {pictorial ? (
        <BlobArtifact
          entry={entry}
          kind="image"
          alt={entry.caption || entry.name || meta.label}
          className={cn("size-full", category === "svg" ? "bg-white object-contain p-2" : "object-cover")}
        />
      ) : (
        <div className="flex flex-1 flex-col items-center justify-center gap-2 p-3">
          <Icon className="size-8 text-muted transition-colors group-hover:text-accent" />
          <span className="line-clamp-2 break-all text-center text-[11px] text-foreground/90">{entry.name || entry.id}</span>
        </div>
      )}
      <span className="absolute inset-x-0 bottom-0 flex items-center gap-1 truncate bg-black/55 px-1.5 py-0.5 text-xs text-white/90">
        {entry.source && <span className="truncate">{entry.source}</span>}
        <span className="ml-auto shrink-0">{humanSize(entry.size)}</span>
      </span>
    </button>
  );
}

// Viewer — the preview modal with a fullscreen toggle ("fullscreen"): inset-2
// when expanded, so an HTML report or a chart fills the monitor.
function Viewer({ entry, onClose, onDelete }: { entry: ArtifactEntry; onClose: () => void; onDelete: () => void }) {
  const [full, setFull] = useState(false);
  const category = categoryOf(entry);

  // "Open in File Manager" only makes sense when the artifact's `ref` is a
  // path-shaped string (e.g. "notes/README.md" written by the coding/file
  // tools). Raw blob hashes and inbound channel refs are content addresses,
  // not FS paths, so the button stays hidden for those.
  const filePath = looksLikePath(entry.ref);
  const openInFileManager = () => {
    if (!filePath) return;
    goToView("files", `path=${encodeURIComponent(filePath)}`);
    onClose();
  };

  // When `full` is true we want the panel itself to fill the viewport, so
  // use a different class set (the Modal overlay still centres, but the panel
  // uses inset-2 instead of max-w-4xl / max-h-90vh).
  return (
    <Modal
      open
      onClose={onClose}
      ariaLabel={`Artifact preview: ${entry.name || entry.id}`}
      panelClassName={cn(full ? "fixed inset-2" : "max-h-[90vh] w-full max-w-4xl")}
    >
      <div className="flex items-center gap-2 border-b border-border px-4 py-2">
        <Badge variant="accent">{category}</Badge>
        <span className="min-w-0 flex-1 truncate text-sm font-semibold">{entry.name || entry.id}</span>
        {filePath && (
          <button
            onClick={openInFileManager}
            className="inline-flex items-center gap-1 rounded border border-border px-2 py-0.5 text-xs text-muted hover:text-foreground"
            title={`Open ${filePath} in the File Manager workspace`}
          >
            <FolderTree className="size-3.5" /> file manager
          </button>
        )}
        <button onClick={() => setFull(!full)} className="text-muted hover:text-accent" title={full ? "Exit fullscreen" : "Fullscreen"} aria-label={full ? "Exit fullscreen" : "Fullscreen"}>
          {full ? <Minimize2 className="size-4" /> : <Maximize2 className="size-4" />}
        </button>
        <button onClick={() => downloadArtifact(entry).catch(console.error)} className="text-muted hover:text-accent" title="Download">
          <Download className="size-4" />
        </button>
        <button onClick={onDelete} className="text-muted hover:text-bad" title="Delete">
          <Trash2 className="size-4" />
        </button>
        <button onClick={onClose} className="text-muted hover:text-foreground" aria-label="Close viewer">
          <X className="size-4" />
        </button>
      </div>
      <div className="flex min-h-0 flex-1 overflow-hidden">
        {/* Persistent metadata column on the left — full-height, narrow,
            readable. The body region keeps the centred preview. */}
        <aside className="hidden w-52 shrink-0 flex-col gap-1 overflow-auto border-r border-border bg-panel/40 p-3 text-[11px] text-muted md:flex">
          <div className="text-xs font-semibold uppercase tracking-normal text-foreground/80">Metadata</div>
          {entry.name && <Field label="name" value={entry.name} />}
          {entry.ref && <Field label="ref" value={entry.ref} mono />}
          {entry.mime && <Field label="mime" value={entry.mime} mono />}
          {entry.kind && entry.kind !== "file" && <Field label="kind" value={entry.kind} />}
          {entry.source && <Field label="source" value={entry.source} />}
          {entry.sender && <Field label="from" value={entry.sender} />}
          {entry.corr && <Field label="corr" value={entry.corr} mono />}
          <Field label="size" value={humanSize(entry.size)} />
          <Field label="created" value={fmtTime(entry.created_ms)} />
          {entry.caption && (
            <div className="mt-2">
              <div className="text-foreground/80">caption</div>
              <div className="text-foreground/90">“{entry.caption}”</div>
            </div>
          )}
        </aside>
        {/* Preview body. md+ works with the metadata column; below md the
            column collapses and the preview fills width — handled in CSS. */}
        <div className="min-h-0 flex-1 overflow-auto bg-panel/40 p-3">
          <ViewerBody entry={entry} category={category} full={full} />
        </div>
      </div>
      <div className="flex flex-wrap gap-x-4 gap-y-0.5 border-t border-border px-4 py-2 text-[11px] text-muted md:hidden">
        {entry.source && <span>source: {entry.source}</span>}
        {entry.sender && <span>from: {entry.sender}</span>}
        {entry.mime && <span>{entry.mime}</span>}
        <span>{humanSize(entry.size)}</span>
        <span>{fmtTime(entry.created_ms)}</span>
        {entry.caption && <span className="text-foreground/80">“{entry.caption}”</span>}
      </div>
    </Modal>
  );
}

// Field is one metadata row inside the persistent column. Mono-styled values
// render longer refs/IDs/paths on one line without wrapping the layout.
function Field({ label, value, mono }: { label: string; value: string; mono?: boolean }) {
  return (
    <div className="leading-tight">
      <div className="text-[10px] uppercase tracking-normal text-muted/80">{label}</div>
      <div
        className={cn("break-all text-foreground/90", mono && "font-mono text-[11px]")}
        title={value}
      >
        {value}
      </div>
    </div>
  );
}

// looksLikePath is a cheap heuristic — does the ref look like a POSIX path?
// Returns null when it doesn't (so the File Manager button stays hidden for
// content-addressed blobs / channel refs).
export function looksLikePath(ref?: string): string | null {
  if (!ref) return null;
  if (ref.startsWith("/") || /^[A-Za-z]:/.test(ref)) return null;
  if (ref.includes("\0")) return null;
  // must contain at least one path separator, OR end in a known file extension
  if (ref.includes("/")) return ref;
  if (/\.(md|txt|json|ya?ml|toml|sh|ts|tsx|js|jsx|go|py|rs|html|css|sql)$/i.test(ref)) return ref;
  return null;
}

// ViewerBody renders the artifact by category. HTML artifacts render inside a
// fully-sandboxed iframe via srcdoc. Security (VULN-008): the sandbox grants NO
// tokens — in particular no `allow-scripts` — so artifact markup (which is
// attacker-influenceable via channel/agent output) is shown as STATIC HTML/CSS
// and cannot execute JavaScript. Combined with the absent `allow-same-origin`,
// the frame can neither run scripts nor reach the console's token/API/cookies.
// `referrerpolicy=no-referrer` keeps the parent URL out of any markup-triggered
// request. Everything else mirrors the Files preview (M842).
function ViewerBody({ entry, category, full }: { entry: ArtifactEntry; category: ArtifactCategory; full: boolean }) {
  const fetchText = category === "html" || category === "markdown" || category === "json" || category === "code" || category === "text";
  const [text, setText] = useState<string | null>(null);
  const [err, setErr] = useState<string | null>(null);

  useEffect(() => {
    if (!fetchText) return;
    if ((entry.size ?? 0) > previewMaxBytes) {
      setErr(`File is too large to preview inline (${humanSize(entry.size)}) — download it.`);
      return;
    }
    let cancelled = false;
    setText(null);
    setErr(null);
    fetch(rawURL(entry), { headers: authHeaders() })
      .then((r) => (r.ok ? r.text() : Promise.reject(new Error(`HTTP ${r.status}`))))
      .then((t) => !cancelled && setText(category === "json" ? prettyJSON(t) : t))
      .catch((e) => !cancelled && setErr((e as Error).message));
    return () => {
      cancelled = true;
    };
  }, [entry, category, fetchText]);

  const frameH = full ? "h-full min-h-[60vh]" : "h-[68vh]";

  if (category === "image" || category === "svg") {
    return (
      <BlobArtifact
        entry={entry}
        kind="image"
        alt={entry.caption || entry.name || "artifact"}
        className={cn("mx-auto rounded-md", full ? "max-h-full" : "max-h-[68vh]", category === "svg" && "bg-white p-3")}
      />
    );
  }
  if (category === "pdf") {
    return <BlobArtifact entry={entry} kind="pdf" title={entry.name || "pdf"} className={cn("w-full rounded-md border border-border bg-white", frameH)} />;
  }
  if (!fetchText) {
    return <p className="py-12 text-center text-sm text-muted">No inline preview for this file type — use Download.</p>;
  }
  if (err) return <p className="py-12 text-center text-sm text-muted">{err}</p>;
  if (text === null) {
    return (
      <p className="flex items-center justify-center gap-2 py-12 text-sm text-muted">
        <Loader2 className="size-4 animate-spin" /> loading…
      </p>
    );
  }
  if (category === "html") {
    return (
      <iframe
        srcDoc={text}
        sandbox=""
        referrerPolicy="no-referrer"
        title={entry.name || "html"}
        className={cn("w-full rounded-md border border-border bg-white", frameH)}
      />
    );
  }
  if (category === "markdown") {
    return <Markdown source={text} className="prose-sm max-w-none text-sm text-foreground/90" />;
  }
  // code / json / text → Monaco. Monaco's built-in JSON formatter handles the
  // json case out of the box (its language id is "json"). The path picker
  // keeps syntax highlighting reasonable for every text-shaped artifact.
  return (
    <MonacoView
      value={text}
      path={entry.name || `artifact.${category}`}
      readOnly
      height={Math.min(560, text.split("\n").length * 18 + 60)}
    />
  );
}

function prettyJSON(s: string): string {
  try {
    return JSON.stringify(JSON.parse(s), null, 2);
  } catch {
    return s;
  }
}
