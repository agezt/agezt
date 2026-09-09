import { useEffect, useState } from "react";
import { FlaskConical, RefreshCw, ChevronRight, ChevronDown, FileCode, FileText, Download, Trash2, ShieldAlert } from "lucide-react";
import { getJSON, postAction } from "@/app/api";
import { cn, fmtDateTime } from "@/app/utils";
import { Button } from "@/components/ui/button";
import { useUI } from "@/components/ui/feedback";
import { SkeletonList } from "@/components/ui/skeleton";
import { EmptyState } from "@/components/ui/empty";
import { ErrorText } from "@/components/JsonView";
import { MetricWidget, MetricGrid } from "@/components/ui/metric-widget";
import { Badge } from "@/components/ui/badge";
import { Page } from "@/components/ui/page";
import { useWardenLogPager, useNetguardLogPager } from "@/lib/cursorPager";
import { LogHistoryPanel } from "@/components/LogHistoryPanel";
import { LoadMoreFooter } from "@/components/ui/load-more-footer";
import { Segmented, ToggleChip } from "@/components/ui/segmented";
import { bytes as fmtBytes } from "@/lib/format";
import { SectionPanel } from "@/components/ui/section-panel";

// PROJECT_WINDOW is how many project cards render at once. /api/sandbox has no
// cursor, so every project arrives in one fetch — the window keeps a big
// sandbox from ballooning the DOM; a Load-more footer grows it client-side.
// The header count stays computed over the FULL list.
const PROJECT_WINDOW = 60;

// FILE_WINDOW is the same guard for a project's FILE list. Every project's files
// arrive inside the /api/sandbox payload, and the cards used to render all of
// them, for every project, side by side: a checked-out repo brought 500 rows of
// `.git/hooks/*.sample`, five projects at once, and the page became an
// unreadable wall. No list renders unbounded.
const FILE_WINDOW = 60;

// NOISE_DIR / NOISE_EXT describe files that exist because a tool put them there,
// not because an agent wrote them: VCS internals, vendored dependencies, caches
// and build output. They are hidden by default — this page's job is to show what
// the agents BUILT, and a pip install or a git clone otherwise buries it. The
// toggle reveals them; nothing is deleted or unreachable.
const NOISE_DIR = [
  ".git",
  ".hg",
  ".svn",
  "node_modules",
  "__pycache__",
  ".deps",
  ".venv",
  "venv",
  "site-packages",
  ".mypy_cache",
  ".pytest_cache",
  ".ruff_cache",
  ".cache",
  "dist-info",
  "egg-info",
];
const NOISE_EXT = [".pyc", ".pyo", ".pyd", ".class", ".o", ".so.tmp"];

// isBuildNoise reports whether a project-relative path is tool-generated rather
// than agent-authored. Pure + unit-tested.
export function isBuildNoise(path: string): boolean {
  const segments = path.split("/");
  // The final segment is the file name; every earlier one is a directory.
  for (const seg of segments.slice(0, -1)) {
    if (NOISE_DIR.some((d) => seg === d || seg.endsWith(d))) return true;
  }
  const name = segments[segments.length - 1] || "";
  return NOISE_EXT.some((e) => name.endsWith(e));
}

// downloadText saves text content to a file via a transient object URL — lets the
// operator grab an artifact an agent built without leaving the browser.
function downloadText(name: string, text: string) {
  const blob = new Blob([text], { type: "text/plain;charset=utf-8" });
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = name;
  document.body.appendChild(a);
  a.click();
  a.remove();
  URL.revokeObjectURL(url);
}

// One file inside a project (name relative to the project root, byte size).
interface SbFile {
  name: string;
  bytes: number;
}

// One persistent code_exec project the agent built under <baseDir>/sandbox/projects.
interface Project {
  name: string;
  files: SbFile[];
  file_count: number;
  total_bytes: number;
  modified_unix: number;
}


// Sandbox shows what the agents BUILT with the code_exec tool: each persistent
// project, its files, and (on click) a file's contents — so the work agents do
// "in the background" is visible and inspectable instead of buried on disk.
export function Sandbox() {
  const [projects, setProjects] = useState<Project[] | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [win, setWin] = useState(PROJECT_WINDOW);
  // Which project's files are open. Cards are a glanceable census; exactly one
  // project's files render at a time, full width, below them.
  const [openProject, setOpenProject] = useState<string | null>(null);

  // Cursor-paginated sandbox-execution log (the journal-backed /api/warden_log).
  const {
    paged: wardenRows,
    loading: wardenLoading,
    loadMore: loadMoreWarden,
    loadingMore: loadingMoreWarden,
    moreError: wardenError,
    hasMore: hasMoreWarden,
  } = useWardenLogPager(50);

  // Cursor-paginated blocked-egress log (the journal-backed /api/netguard_log).
  const {
    paged: netguardRows,
    loading: netguardLoading,
    loadMore: loadMoreNetguard,
    loadingMore: loadingMoreNetguard,
    moreError: netguardError,
    hasMore: hasMoreNetguard,
  } = useNetguardLogPager(50);

  async function reload() {
    setLoading(true);
    try {
      const d = await getJSON<{ projects?: Project[] }>("/api/sandbox");
      setProjects(d.projects || []);
      setErr(null);
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    reload();
    const id = setInterval(reload, 8000);
    return () => clearInterval(id);
  }, []);

  return (
    <Page
      mode="scroll"
      width="wide"
      icon={FlaskConical}
      title="Sandbox"
      description={projects ? `${projects.length} project${projects.length === 1 ? "" : "s"}` : undefined}
      actions={
        <Button variant="ghost" size="sm" onClick={reload} disabled={loading} title="Reload">
          <RefreshCw className={cn("size-3.5", loading && "animate-spin")} />
        </Button>
      }
    >

      {err ? (
        <ErrorText>{err}</ErrorText>
      ) : !projects ? (
        <SkeletonList count={3} lines={2} />
      ) : projects.length === 0 ? (
        <EmptyState
          icon={FlaskConical}
          title="No sandbox projects yet"
          hint="When an agent runs code_exec with a project name, its files appear here."
        />
      ) : (
        <>
          <MetricGrid cols="repeat(auto-fill, minmax(220px, 1fr))">
            {projects.slice(0, win).map((p) => (
              <ProjectCard
                key={p.name}
                p={p}
                open={openProject === p.name}
                onOpen={() => setOpenProject((cur) => (cur === p.name ? null : p.name))}
                onChanged={() => {
                  if (openProject === p.name) setOpenProject(null);
                  reload();
                }}
              />
            ))}
          </MetricGrid>
          {projects.length > PROJECT_WINDOW && (
            <LoadMoreFooter
              hasMore={win < projects.length}
              loadingMore={false}
              onLoadMore={() => setWin((w) => w + PROJECT_WINDOW)}
              pageSize={Math.min(PROJECT_WINDOW, Math.max(1, projects.length - win))}
              label="projects"
            />
          )}
          {openProject && projects.some((p) => p.name === openProject) && (
            <FileBrowser key={openProject} p={projects.find((p) => p.name === openProject)!} />
          )}
        </>
      )}

      <LogHistoryPanel
        icon={FlaskConical}
        title="Sandbox executions"
        rows={wardenRows}
        loading={wardenLoading}
        loadMore={loadMoreWarden}
        loadingMore={loadingMoreWarden}
        moreError={wardenError}
        hasMore={hasMoreWarden}
        pageSize={50}
        renderRow={(r) => {
          const code = typeof r.exit_code === "number" ? r.exit_code : Number(r.exit_code);
          const ok = !Number.isNaN(code) && code === 0;
          return (
            <>
              <span className="font-mono text-foreground">{String(r.agent || "")}</span>
              <span className="shrink-0 text-muted">{String(r.tool || "")}</span>
              <Badge variant="default">{String(r.lang || "")}</Badge>
              <Badge variant={ok ? "good" : "bad"}>exit {String(r.exit_code ?? "")}</Badge>
            </>
          );
        }}
      />

      <LogHistoryPanel
        icon={ShieldAlert}
        title="Blocked egress"
        rows={netguardRows}
        loading={netguardLoading}
        loadMore={loadMoreNetguard}
        loadingMore={loadingMoreNetguard}
        moreError={netguardError}
        hasMore={hasMoreNetguard}
        pageSize={50}
        renderRow={(r) => (
          <>
            <span className="font-mono text-foreground">{String(r.agent || "")}</span>
            <span className="shrink-0 text-muted">
              {String(r.dest_host || "")}:{String(r.dest_port || "")}
            </span>
            <Badge variant="default">{String(r.protocol || "")}</Badge>
            <Badge variant="bad">{String(r.action || "")}</Badge>
          </>
        )}
      />
    </Page>
  );
}

function ProjectCard({
  p,
  open,
  onOpen,
  onChanged,
}: {
  p: Project;
  open: boolean;
  onOpen: () => void;
  onChanged: () => void;
}) {
  const [busy, setBusy] = useState(false);
  const ui = useUI();

  async function remove() {
    const ok = await ui.confirm({
      title: `Delete project "${p.name}"?`,
      message: `This permanently removes the project and its ${p.file_count} file${p.file_count === 1 ? "" : "s"} from the sandbox. This cannot be undone.`,
      confirmLabel: "Delete",
      danger: true,
    });
    if (!ok) return;
    setBusy(true);
    try {
      await postAction("/api/sandbox/delete", { project: p.name });
      ui.toast(`Deleted "${p.name}"`, "success");
      onChanged();
    } catch (e) {
      ui.toast((e as Error).message, "error");
    } finally {
      setBusy(false);
    }
  }

  // What the agent actually authored, versus what a clone or an install left
  // behind. The card leads with the authored count because that is the number
  // the operator is here for.
  const authored = p.files.filter((f) => !isBuildNoise(f.name)).length;

  return (
    <SectionPanel
      className={cn("transition-colors", open && "border-accent/60")}
      icon={FlaskConical}
      tone="accent"
      title={
        <button onClick={onOpen} className="block w-full truncate text-left" title={`Browse ${p.name}`}>
          {p.name}
        </button>
      }
      status={fmtDateTime(p.modified_unix)}
      actions={
        <button
          onClick={remove}
          disabled={busy}
          title="Delete project"
          className="shrink-0 rounded p-1 text-muted transition-colors hover:bg-bad/10 hover:text-bad disabled:opacity-50"
        >
          <Trash2 className="size-3.5" />
        </button>
      }
    >
      <button
        onClick={onOpen}
        className="mt-2 flex w-full items-center gap-3 text-left text-xs text-muted transition-colors hover:text-foreground"
      >
        {open ? <ChevronDown className="size-3 shrink-0" /> : <ChevronRight className="size-3 shrink-0" />}
        <span className="tabular-nums">
          <span className="font-semibold text-foreground">{authored}</span> file{authored === 1 ? "" : "s"}
          {p.file_count > authored && <span className="text-muted"> · {p.file_count - authored} generated</span>}
        </span>
        <span className="ml-auto shrink-0 tabular-nums">{fmtBytes(p.total_bytes)}</span>
      </button>
    </SectionPanel>
  );
}

// FileBrowser is the one full-width place a project's files render. Cards above
// stay a census; this is the drill-in — filtered, windowed, and with the
// tool-generated noise folded away by default.
function FileBrowser({ p }: { p: Project }) {
  const [query, setQuery] = useState("");
  const [showNoise, setShowNoise] = useState(false);
  const [win, setWin] = useState(FILE_WINDOW);

  const noiseCount = p.files.filter((f) => isBuildNoise(f.name)).length;
  const visible = p.files.filter((f) => {
    if (!showNoise && isBuildNoise(f.name)) return false;
    return !query || f.name.toLowerCase().includes(query.toLowerCase());
  });

  // Reset the window whenever the filter changes, so each result set starts at
  // the top instead of inheriting a scrolled-open window from the last one.
  useEffect(() => {
    setWin(FILE_WINDOW);
  }, [query, showNoise]);

  return (
    <section className="rounded-xl border border-border bg-card/70 p-3 shadow-e1">
      <div className="mb-2 flex flex-wrap items-center gap-2">
        <FileCode className="size-4 shrink-0 text-accent" />
        <h3 className="text-sm font-semibold">{p.name}</h3>
        <span className="text-xs text-muted">
          {visible.length} of {p.file_count} file{p.file_count === 1 ? "" : "s"}
        </span>
        <input
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          placeholder="filter files…"
          aria-label="Filter files"
          className="ml-auto w-48 rounded-full border border-border bg-panel px-3 py-1 text-xs text-foreground placeholder:text-muted"
        />
        {noiseCount > 0 && (
          <ToggleChip
            on={showNoise}
            onToggle={() => setShowNoise((v) => !v)}
            title="Git internals, vendored dependencies, caches and build output — present on disk, just not what the agent wrote"
          >
            {showNoise ? "Hide" : "Show"} generated files ({noiseCount})
          </ToggleChip>
        )}
      </div>
      {visible.length === 0 ? (
        <p className="text-xs text-muted">
          {p.file_count === 0
            ? "empty project"
            : query
              ? "No file matches that filter."
              : "Every file here is tool-generated — use “Show generated files” to browse them."}
        </p>
      ) : (
        <>
          <ul className="space-y-0.5">
            {visible.slice(0, win).map((f) => (
              <FileRow key={f.name} project={p.name} file={f} />
            ))}
          </ul>
          {visible.length > FILE_WINDOW && (
            <LoadMoreFooter
              hasMore={win < visible.length}
              loadingMore={false}
              onLoadMore={() => setWin((w) => w + FILE_WINDOW)}
              pageSize={Math.min(FILE_WINDOW, Math.max(1, visible.length - win))}
              label="files"
            />
          )}
        </>
      )}
    </section>
  );
}

function FileRow({ project, file }: { project: string; file: SbFile }) {
  const [open, setOpen] = useState(false);
  const [content, setContent] = useState<string | null>(null);
  const [truncated, setTruncated] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);

  // ensureContent fetches (once) and returns the file's content — shared by the
  // expand toggle and the download button.
  async function ensureContent(): Promise<string | null> {
    if (content !== null) return content;
    setLoading(true);
    try {
      const d = await getJSON<{ content?: string; truncated?: boolean }>("/api/sandbox_file", {
        project,
        file: file.name,
      });
      const text = d.content ?? "";
      setContent(text);
      setTruncated(!!d.truncated);
      setErr(null);
      return text;
    } catch (e) {
      setErr((e as Error).message);
      return null;
    } finally {
      setLoading(false);
    }
  }

  async function toggle() {
    const next = !open;
    setOpen(next);
    if (next) await ensureContent();
  }

  async function download() {
    const text = await ensureContent();
    if (text !== null) downloadText(file.name.split("/").pop() || "file.txt", text);
  }

  const isCode = /\.(py|js|ts|tsx|jsx|json|sh|go|rs|rb|java|c|cpp|h|css|html|toml|yaml|yml)$/i.test(file.name);
  const Icon = isCode ? FileCode : FileText;

  return (
    <li>
      <div className="flex items-center gap-2 py-0.5 text-xs">
        <button onClick={toggle} className="flex flex-1 items-center gap-2 text-left">
          {open ? <ChevronDown className="size-3 text-muted" /> : <ChevronRight className="size-3 text-muted" />}
          <Icon className="size-3.5 text-muted" />
          <span className="font-mono">{file.name}</span>
          <span className="ml-auto text-muted">{fmtBytes(file.bytes)}</span>
        </button>
        <button
          onClick={download}
          disabled={loading}
          title="Download file"
          className="shrink-0 rounded p-1 text-muted transition-colors hover:bg-accent/10 hover:text-accent disabled:opacity-50"
        >
          <Download className="size-3.5" />
        </button>
      </div>
      {open && (
        <div className="ml-5 mt-1 mb-1.5">
          {err ? (
            <ErrorText>{err}</ErrorText>
          ) : loading ? (
            <p className="text-xs text-muted">loading…</p>
          ) : (
            <>
              {truncated && <p className="mb-1 text-xs text-warn">truncated to first 256 KiB</p>}
              <pre className="max-h-96 overflow-auto rounded-md border border-border bg-panel p-2 text-[11px] leading-relaxed">
                <code>{content}</code>
              </pre>
            </>
          )}
        </div>
      )}
    </li>
  );
}
