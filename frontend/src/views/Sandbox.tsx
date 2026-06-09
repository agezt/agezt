import { useEffect, useState } from "react";
import { FlaskConical, RefreshCw, ChevronRight, ChevronDown, FileCode, FileText, Download, Trash2 } from "lucide-react";
import { getJSON, postAction } from "@/lib/api";
import { cn, fmtDateTime } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { useUI } from "@/components/ui/feedback";
import { SkeletonList } from "@/components/ui/skeleton";
import { EmptyState } from "@/components/ui/empty";
import { ErrorText } from "@/components/JsonView";

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

function fmtBytes(n: number): string {
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`;
  return `${(n / (1024 * 1024)).toFixed(1)} MB`;
}

// Sandbox shows what the agents BUILT with the code_exec tool: each persistent
// project, its files, and (on click) a file's contents — so the work agents do
// "in the background" is visible and inspectable instead of buried on disk.
export function Sandbox() {
  const [projects, setProjects] = useState<Project[] | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);

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
    <div className="flex h-full min-h-0 flex-col gap-3">
      <div className="flex items-center gap-2">
        <h2 className="text-lg font-semibold">Sandbox</h2>
        <span className="text-xs text-muted">
          {projects ? `${projects.length} project${projects.length === 1 ? "" : "s"}` : ""}
        </span>
        <Button variant="ghost" size="sm" className="ml-auto" onClick={reload} disabled={loading}>
          <RefreshCw className={cn("size-3.5", loading && "animate-spin")} />
        </Button>
      </div>
      <p className="-mt-1 text-xs text-muted">
        Persistent projects your agents built and iterate on with the <code>code_exec</code> tool. Open a file to view or download it; remove a project you no longer need.
      </p>

      {err ? (
        <ErrorText>{err}</ErrorText>
      ) : !projects ? (
        <SkeletonList count={3} lines={2} />
      ) : projects.length === 0 ? (
        <EmptyState
          icon={FlaskConical}
          title="No sandbox projects yet"
          hint="When an agent runs code_exec with a project name, its files appear here — code, data, whatever it builds."
        />
      ) : (
        <div className="min-h-0 flex-1 overflow-auto">
          <ul className="space-y-2">
            {projects.map((p) => (
              <ProjectCard key={p.name} p={p} onChanged={reload} />
            ))}
          </ul>
        </div>
      )}
    </div>
  );
}

function ProjectCard({ p, onChanged }: { p: Project; onChanged: () => void }) {
  const [open, setOpen] = useState(false);
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

  return (
    <li className="rounded-lg border border-border bg-card">
      <div className="flex items-center gap-2 px-3 py-2.5">
        <button onClick={() => setOpen((o) => !o)} className="flex flex-1 items-center gap-2 text-left">
          {open ? <ChevronDown className="size-4 text-muted" /> : <ChevronRight className="size-4 text-muted" />}
          <FlaskConical className="size-4 text-accent" />
          <span className="font-medium">{p.name}</span>
          <span className="text-xs text-muted">
            {p.file_count} file{p.file_count === 1 ? "" : "s"} · {fmtBytes(p.total_bytes)}
          </span>
          {p.modified_unix > 0 && (
            <span className="ml-auto text-xs text-muted">{fmtDateTime(p.modified_unix * 1000)}</span>
          )}
        </button>
        <button
          onClick={remove}
          disabled={busy}
          title="Delete project"
          className="shrink-0 rounded p-1 text-muted transition-colors hover:bg-bad/10 hover:text-bad disabled:opacity-50"
        >
          <Trash2 className="size-3.5" />
        </button>
      </div>
      {open && (
        <div className="border-t border-border px-3 py-2">
          {p.files.length === 0 ? (
            <p className="text-xs text-muted">empty project</p>
          ) : (
            <ul className="space-y-0.5">
              {p.files.map((f) => (
                <FileRow key={f.name} project={p.name} file={f} />
              ))}
            </ul>
          )}
        </div>
      )}
    </li>
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
              {truncated && <p className="mb-1 text-[10px] text-warn">truncated to first 256 KiB</p>}
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
