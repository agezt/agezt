import { useEffect, useState } from "react";
import { FlaskConical, RefreshCw, ChevronRight, ChevronDown, FileCode, FileText } from "lucide-react";
import { getJSON } from "@/lib/api";
import { cn, fmtDateTime } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { SkeletonList } from "@/components/ui/skeleton";
import { EmptyState } from "@/components/ui/empty";
import { ErrorText } from "@/components/JsonView";

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
        Persistent projects your agents built and iterate on with the <code>code_exec</code> tool. Read-only.
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
              <ProjectCard key={p.name} p={p} />
            ))}
          </ul>
        </div>
      )}
    </div>
  );
}

function ProjectCard({ p }: { p: Project }) {
  const [open, setOpen] = useState(false);
  return (
    <li className="rounded-lg border border-border bg-card">
      <button
        onClick={() => setOpen((o) => !o)}
        className="flex w-full items-center gap-2 px-3 py-2.5 text-left"
      >
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

  async function toggle() {
    const next = !open;
    setOpen(next);
    if (next && content === null && !loading) {
      setLoading(true);
      try {
        const d = await getJSON<{ content?: string; truncated?: boolean }>("/api/sandbox_file", {
          project,
          file: file.name,
        });
        setContent(d.content ?? "");
        setTruncated(!!d.truncated);
        setErr(null);
      } catch (e) {
        setErr((e as Error).message);
      } finally {
        setLoading(false);
      }
    }
  }

  const isCode = /\.(py|js|ts|tsx|jsx|json|sh|go|rs|rb|java|c|cpp|h|css|html|toml|yaml|yml)$/i.test(file.name);
  const Icon = isCode ? FileCode : FileText;

  return (
    <li>
      <button onClick={toggle} className="flex w-full items-center gap-2 py-0.5 text-left text-xs">
        {open ? <ChevronDown className="size-3 text-muted" /> : <ChevronRight className="size-3 text-muted" />}
        <Icon className="size-3.5 text-muted" />
        <span className="font-mono">{file.name}</span>
        <span className="ml-auto text-muted">{fmtBytes(file.bytes)}</span>
      </button>
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
