import { useEffect, useState } from "react";
import {
  Hammer,
  RefreshCw,
  Plus,
  X,
  Pencil,
  Trash2,
  FlaskConical,
  Rocket,
  ShieldOff,
  List,
  Code2,
  type LucideIcon,
} from "lucide-react";
import { getJSON, postAction, postJSON } from "@/lib/api";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { Page } from "@/components/ui/page";
import { useUI, type ConfirmOptions } from "@/components/ui/feedback";
import { SkeletonList } from "@/components/ui/skeleton";
import { EmptyState } from "@/components/ui/empty";
import { Badge } from "@/components/ui/badge";
import { ErrorText } from "@/components/JsonView";
import { Disclosure } from "@/components/ui/disclosure";

export interface ScriptTool {
  id: string;
  name: string;
  description?: string;
  language?: string;
  code?: string;
  input_schema?: string;
  status?: string;
  tested_ok?: boolean;
  callable_as?: string;
}

// toolNameOk mirrors the kernel's toolforge name rule (lowercase start, then
// letters/digits/underscore, ≤40) so the form validates before the round-trip.
// Pure + unit-tested.
export function toolNameOk(s: string): boolean {
  return /^[a-z][a-z0-9_]{0,39}$/.test(s);
}

// statusBadge maps a script tool's lifecycle state to a badge color: live is
// good, the kill switch is bad, a draft is neutral. Pure + unit-tested.
export function statusBadge(status?: string): "good" | "bad" | "default" {
  if (status === "active") return "good";
  if (status === "quarantined") return "bad";
  return "default";
}

const inputCls =
  "rounded-md border border-border bg-panel px-2 py-1 text-sm text-foreground outline-none focus-visible:border-accent";
const codeCls = cn(inputCls, "resize-y font-mono text-xs");

const LANGS = ["python", "node", "deno"];

function LanguagePicker({ value, onChange }: { value: string; onChange: (value: string) => void }) {
  return (
    <div className="flex flex-wrap items-center gap-1.5" role="group" aria-label="Tool language">
      {LANGS.map((lang) => (
        <button
          key={lang}
          type="button"
          aria-pressed={(value || "python") === lang}
          onClick={() => onChange(lang)}
          className={cn(
            "inline-flex h-8 items-center gap-1.5 rounded-md border px-2 text-xs font-medium transition-colors",
            (value || "python") === lang
              ? "border-accent bg-accent/15 text-accent"
              : "border-border bg-panel text-muted hover:border-accent/60 hover:text-foreground",
          )}
        >
          <Code2 className="size-3.5" />
          {lang}
        </button>
      ))}
    </div>
  );
}

// ToolFormFields renders the shared editable fields for New/Edit. The script
// contract is stated right where the code is typed.
function ToolFormFields(props: { state: Record<string, string>; set: (k: string, v: string) => void }) {
  const { state, set } = props;
  const codeLines = (state.code || "").trim() ? (state.code || "").split(/\r?\n/).length : 0;
  const schemaSet = Boolean((state.schema || "").trim());
  return (
    <>
      <div className="grid gap-2 sm:grid-cols-[auto_1fr]">
        <div className="flex flex-col gap-1 text-[11px] text-muted">
          Language
          <LanguagePicker value={state.language || "python"} onChange={(value) => set("language", value)} />
        </div>
        <label className="flex flex-col gap-1 text-[11px] text-muted">
          Description — what the MODEL reads to decide when to call it
          <input
            value={state.description || ""}
            onChange={(e) => set("description", e.target.value)}
            placeholder="e.g. Look up the weather for a city"
            aria-label="Tool description"
            className={inputCls}
          />
        </label>
      </div>
      <div className="mt-2 grid gap-2">
        <Disclosure
          className="rounded-lg border border-border bg-panel/45"
          summaryClassName="px-2.5 py-2 hover:bg-card/60"
          contentClassName="px-2.5 pb-2"
          defaultOpen={Boolean((state.code || "").trim())}
          summary={
            <span className="flex min-w-0 items-center gap-2">
              <Code2 className="size-3.5 shrink-0 text-accent" />
              <span className="truncate text-xs font-semibold text-foreground">Implementation</span>
              <span className="ml-auto text-[11px] text-muted">{codeLines ? `${codeLines} lines` : "code required"}</span>
            </span>
          }
        >
          <label className="flex flex-col gap-1 text-[11px] text-muted">
            Code — JSON input arrives as ./stdin.txt; print result to stdout
            <textarea
              value={state.code || ""}
              onChange={(e) => set("code", e.target.value)}
              placeholder={'import json\nd = json.load(open("stdin.txt"))\nprint("hello " + d.get("name", "?"))'}
              aria-label="Tool code"
              rows={8}
              className={codeCls}
            />
          </label>
        </Disclosure>
        <Disclosure
          className="rounded-lg border border-border bg-panel/45"
          summaryClassName="px-2.5 py-2 hover:bg-card/60"
          contentClassName="px-2.5 pb-2"
          defaultOpen={schemaSet}
          summary={
            <span className="flex min-w-0 items-center gap-2">
              <ShieldOff className="size-3.5 shrink-0 text-accent" />
              <span className="truncate text-xs font-semibold text-foreground">Input contract</span>
              <span className="ml-auto text-[11px] text-muted">{schemaSet ? "schema set" : "optional schema"}</span>
            </span>
          }
        >
          <label className="flex flex-col gap-1 text-[11px] text-muted">
            Input schema (optional JSON-Schema object)
            <textarea
              value={state.schema || ""}
              onChange={(e) => set("schema", e.target.value)}
              placeholder='{"type":"object","required":["name"],"properties":{"name":{"type":"string"}}}'
              aria-label="Tool input schema"
              rows={2}
              className={codeCls}
            />
          </label>
        </Disclosure>
      </div>
    </>
  );
}

// NewToolForm drafts a script tool (M795). Exported for tests and reuse (the
// M714 "creatable from UI" recipe).
export function NewToolForm({
  onCreated,
  onError,
}: {
  onCreated: (name: string) => void;
  onError: (msg: string) => void;
}) {
  const [state, setState] = useState<Record<string, string>>({});
  const [submitting, setSubmitting] = useState(false);
  const set = (k: string, v: string) => setState((s) => ({ ...s, [k]: v }));
  const name = (state.name || "").trim();
  const valid = toolNameOk(name) && (state.code || "").trim() !== "" && (state.description || "").trim() !== "";

  async function create() {
    if (!valid) return;
    setSubmitting(true);
    try {
      await postJSON("/api/toolforge/draft", {
        tool: {
          name,
          description: (state.description || "").trim(),
          language: state.language || "python",
          code: state.code || "",
          input_schema: (state.schema || "").trim(),
        },
      });
      onCreated(name);
    } catch (e) {
      onError((e as Error).message);
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div className="rounded-lg border border-accent/30 bg-card p-3">
      <label className="flex flex-col gap-1 text-[11px] text-muted">
        Name — the tool's permanent handle (lowercase/digits/underscore); callable as forge_&lt;name&gt; once promoted
        <input
          value={state.name || ""}
          onChange={(e) => set("name", e.target.value)}
          placeholder="e.g. fetch_weather"
          aria-label="Tool name"
          className={cn(inputCls, name !== "" && !toolNameOk(name) && "border-bad")}
        />
      </label>
      <div className="mt-2">
        <ToolFormFields state={state} set={set} />
      </div>
      <div className="mt-3 flex justify-end">
        <Button size="sm" onClick={create} disabled={!valid || submitting}>
          <Plus className="h-3.5 w-3.5" /> Draft tool
        </Button>
      </div>
    </div>
  );
}

// EditToolForm edits a tool's mutable fields. The kernel demotes on a code
// change — the form says so up front.
export function EditToolForm({
  tool,
  onSaved,
  onError,
}: {
  tool: ScriptTool;
  onSaved: (name: string) => void;
  onError: (msg: string) => void;
}) {
  const [state, setState] = useState<Record<string, string>>({
    description: tool.description || "",
    language: tool.language || "python",
    code: tool.code || "",
    schema: tool.input_schema || "",
  });
  const [submitting, setSubmitting] = useState(false);
  const set = (k: string, v: string) => setState((s) => ({ ...s, [k]: v }));

  async function save() {
    setSubmitting(true);
    try {
      await postJSON("/api/toolforge/edit", {
        ref: tool.name,
        tool: {
          description: state.description.trim(),
          language: state.language,
          code: state.code,
          input_schema: state.schema.trim(),
        },
      });
      onSaved(tool.name);
    } catch (e) {
      onError((e as Error).message);
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div className="rounded-lg border border-accent/30 bg-card p-3">
      <div className="text-[11px] text-muted">
        Editing <span className="font-mono text-foreground">{tool.name}</span> — a code change demotes it to draft
        (re-test before promoting)
      </div>
      <div className="mt-2">
        <ToolFormFields state={state} set={set} />
      </div>
      <div className="mt-3 flex justify-end">
        <Button size="sm" onClick={save} disabled={submitting}>
          <Pencil className="h-3.5 w-3.5" /> Save
        </Button>
      </div>
    </div>
  );
}

// TestPanel runs a tool's current code once in the sandbox with a sample JSON
// input and shows the verdict + output. A pass arms the promote button.
function TestPanel({ tool, onTested }: { tool: ScriptTool; onTested: () => void }) {
  const [input, setInput] = useState("{}");
  const [running, setRunning] = useState(false);
  const [result, setResult] = useState<{ ok: boolean; output: string } | null>(null);
  const ui = useUI();

  async function run() {
    setRunning(true);
    setResult(null);
    try {
      const r = await postAction<{ ok?: boolean; output?: string }>("/api/toolforge/test", {
        ref: tool.name,
        input,
      });
      setResult({ ok: !!r.ok, output: r.output || "" });
      onTested();
    } catch (e) {
      ui.toast((e as Error).message, "error");
    } finally {
      setRunning(false);
    }
  }

  return (
    <div className="mt-2 rounded-lg border border-border bg-panel p-2">
      <div className="flex items-end gap-2">
        <label className="flex flex-1 flex-col gap-1 text-[11px] text-muted">
          Sample JSON input (the script reads it from ./stdin.txt)
          <input
            value={input}
            onChange={(e) => setInput(e.target.value)}
            aria-label={`Test input for ${tool.name}`}
            className={cn(inputCls, "font-mono text-xs")}
          />
        </label>
        <Button size="sm" onClick={run} disabled={running} aria-label={`Run test for ${tool.name}`}>
          <FlaskConical className={cn("h-3.5 w-3.5", running && "animate-pulse")} />
          {running ? "Running…" : "Run test"}
        </Button>
      </div>
      {result && (
        <div className="mt-2">
          <Badge variant={result.ok ? "good" : "bad"}>{result.ok ? "PASS" : "FAIL"}</Badge>
          <pre className="mt-1.5 max-h-48 overflow-auto rounded-md bg-card px-2 py-1.5 text-xs whitespace-pre-wrap text-muted">
            {result.output || "(no output)"}
          </pre>
        </div>
      )}
    </div>
  );
}

// Toolforge is the script-tool forge console (M795): the governed pipeline by
// which agent-written code becomes a callable forge_<name> tool — draft, test
// in the sandbox, promote (the operator's sign-off), quarantine, remove.
export function Toolforge() {
  const ui = useUI();
  const [tools, setTools] = useState<ScriptTool[] | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [busy, setBusy] = useState<string | null>(null);
  const [showForm, setShowForm] = useState(false);
  const [editing, setEditing] = useState<ScriptTool | null>(null);
  const [testing, setTesting] = useState<string | null>(null);

  async function reload() {
    setLoading(true);
    try {
      const d = await getJSON<{ tools?: ScriptTool[] }>("/api/toolforge");
      setTools(d.tools || []);
      setErr(null);
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    reload();
    const t = setInterval(reload, 8000);
    return () => clearInterval(t);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  async function act(
    ref: string,
    path: string,
    params?: Record<string, string>,
    opts?: { confirm?: ConfirmOptions; success?: string },
  ) {
    if (opts?.confirm && !(await ui.confirm(opts.confirm))) return;
    setBusy(ref);
    try {
      await postAction(path, { ref, ...params });
      if (opts?.success) ui.toast(opts.success, "success");
      await reload();
    } catch (e) {
      ui.toast((e as Error).message, "error");
    } finally {
      setBusy(null);
    }
  }

  // The edit form needs the code body, which the list deliberately strips —
  // fetch the full record on demand.
  async function openEditor(name: string) {
    if (editing?.name === name) {
      setEditing(null);
      return;
    }
    try {
      const d = await getJSON<{ tool?: ScriptTool }>("/api/toolforge/show", { ref: name });
      if (d.tool) setEditing(d.tool);
    } catch (e) {
      ui.toast((e as Error).message, "error");
    }
  }

  const live = (tools || []).filter((t) => t.status === "active").length;

  return (
    <Page
      icon={Hammer}
      title="Tool Forge"
      description="Draft, sandbox-test, and promote script tools into callable forge_<name> tools."
      width="readable"
      mode="scroll"
      actions={
        <>
          <Button size="sm" variant="ghost" onClick={reload} disabled={loading} aria-label="Refresh">
            <RefreshCw className={cn("h-3.5 w-3.5", loading && "animate-spin")} />
          </Button>
          <Button size="sm" onClick={() => setShowForm((v) => !v)}>
            <Plus className="h-3.5 w-3.5" />
            New tool
          </Button>
        </>
      }
    >

      {showForm && (
        <ForgeModal title="Draft script tool" icon={Hammer} onClose={() => setShowForm(false)}>
          <NewToolForm
            onCreated={(name) => {
              setShowForm(false);
              ui.toast(`tool ${name} drafted — test it, then promote`, "success");
              reload();
            }}
            onError={(msg) => ui.toast(msg, "error")}
          />
        </ForgeModal>
      )}

      {err && <ErrorText>{err}</ErrorText>}
      {!tools && !err && <SkeletonList count={3} />}
      {tools && tools.length === 0 && !showForm && (
        <EmptyState
          icon={Hammer}
          title="No script tools yet"
          hint='Draft one here, or let an agent build its own with the tool_forge tool — then test and promote it to make it callable.'
        />
      )}

      {tools && tools.length > 0 && (
        <ForgePanel icon={List} title="Script tools" status={`${tools.length} tool${tools.length === 1 ? "" : "s"} · ${live} live`}>
          <ul className="space-y-2">
            {tools.map((t) => (
              <li key={t.id} className="rounded-lg border border-border bg-card p-3">
                <div className="flex flex-wrap items-center gap-2">
                  <span className="font-mono text-sm text-foreground">{t.name}</span>
                  <Badge variant={statusBadge(t.status)}>{t.status || "draft"}</Badge>
                  <Badge variant={t.tested_ok ? "good" : "warn"}>{t.tested_ok ? "tested" : "untested"}</Badge>
                  {t.callable_as && <span className="font-mono text-xs text-accent">{t.callable_as}</span>}
                  <span className="ml-auto flex items-center gap-1">
                    <Button
                      size="sm"
                      variant="ghost"
                      aria-label={`Test ${t.name}`}
                      onClick={() => setTesting(testing === t.name ? null : t.name)}
                    >
                      <FlaskConical className="h-3.5 w-3.5" />
                    </Button>
                    {t.status !== "active" && (
                      <Button
                        size="sm"
                        variant="ghost"
                        disabled={busy === t.name || !t.tested_ok}
                        aria-label={`Promote ${t.name}`}
                        title={t.tested_ok ? "Promote — make it callable" : "Test must pass before promotion"}
                        onClick={() =>
                          act(t.name, "/api/toolforge/promote", undefined, {
                            confirm: {
                              title: `Promote ${t.name}?`,
                              message: `Every run (and sub-agent) will be offered it as forge_${t.name}. Its code runs in the sandbox under the code.exec policy.`,
                              confirmLabel: "Promote",
                            },
                            success: `${t.name} is live as forge_${t.name}`,
                          })
                        }
                      >
                        <Rocket className="h-3.5 w-3.5" />
                      </Button>
                    )}
                    {t.status === "active" && (
                      <Button
                        size="sm"
                        variant="ghost"
                        disabled={busy === t.name}
                        aria-label={`Quarantine ${t.name}`}
                        onClick={() =>
                          act(t.name, "/api/toolforge/quarantine", { reason: "pulled from the console" }, {
                            confirm: {
                              title: `Quarantine ${t.name}?`,
                              message: "It stops being offered to runs immediately. You can re-promote it later.",
                              confirmLabel: "Quarantine",
                              danger: true,
                            },
                            success: `${t.name} quarantined`,
                          })
                        }
                      >
                        <ShieldOff className="h-3.5 w-3.5" />
                      </Button>
                    )}
                    <Button size="sm" variant="ghost" aria-label={`Edit ${t.name}`} onClick={() => openEditor(t.name)}>
                      <Pencil className="h-3.5 w-3.5" />
                    </Button>
                    <Button
                      size="sm"
                      variant="ghost"
                      disabled={busy === t.name}
                      aria-label={`Remove ${t.name}`}
                      onClick={() =>
                        act(t.name, "/api/toolforge/remove", undefined, {
                          confirm: {
                            title: `Remove tool ${t.name}?`,
                            message: "Its code and test record are deleted. Past uses stay in the journal.",
                            confirmLabel: "Remove",
                            danger: true,
                          },
                          success: `${t.name} removed`,
                        })
                      }
                    >
                      <Trash2 className="h-3.5 w-3.5" />
                    </Button>
                  </span>
                </div>
                <div className="mt-1.5 flex flex-wrap gap-x-4 gap-y-1 text-xs text-muted">
                  <span>lang: {t.language || "?"}</span>
                  {t.description && <span>{t.description}</span>}
                </div>
                {testing === t.name && <TestPanel tool={t} onTested={reload} />}
              </li>
            ))}
          </ul>
        </ForgePanel>
      )}

      {editing && (
        <ForgeModal title={`Edit ${editing.name}`} icon={Code2} onClose={() => setEditing(null)}>
          <EditToolForm
            tool={editing}
            onSaved={(name) => {
              setEditing(null);
              ui.toast(`tool ${name} updated`, "success");
              reload();
            }}
            onError={(msg) => ui.toast(msg, "error")}
          />
        </ForgeModal>
      )}
    </Page>
  );
}

function ForgePanel({
  icon: Icon,
  title,
  status,
  children,
}: {
  icon: LucideIcon;
  title: string;
  status: string;
  children: React.ReactNode;
}) {
  return (
    <section className="rounded-xl border border-border bg-card/70 p-3 shadow-e1">
      <div className="mb-2 flex items-center gap-2">
        <span className="grid size-8 shrink-0 place-items-center rounded-lg border border-accent/35 bg-accent/5 text-accent">
          <Icon className="size-4" />
        </span>
        <div className="min-w-0 flex-1">
          <h3 className="text-sm font-semibold">{title}</h3>
          <div className="truncate text-xs text-muted">{status}</div>
        </div>
      </div>
      {children}
    </section>
  );
}

function ForgeModal({
  title,
  icon: Icon,
  onClose,
  children,
}: {
  title: string;
  icon: typeof Hammer;
  onClose: () => void;
  children: React.ReactNode;
}) {
  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if (e.key === "Escape") onClose();
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose]);
  return (
    <div className="modal-overlay fixed inset-0 z-[160] flex items-start justify-center overflow-y-auto bg-black/55 p-4 backdrop-blur-sm" onClick={onClose}>
      <div
        className="modal-in mt-10 w-full max-w-3xl rounded-lg border border-border bg-card p-4 shadow-xl shadow-black/30"
        onClick={(e) => e.stopPropagation()}
        role="dialog"
        aria-modal="true"
        aria-label={title}
      >
        <div className="mb-3 flex items-center gap-2">
          <span className="grid size-8 place-items-center rounded-lg bg-accent/12 text-accent ring-1 ring-inset ring-accent/25">
            <Icon className="size-4" />
          </span>
          <h3 className="text-sm font-semibold text-foreground">{title}</h3>
          <button className="ml-auto rounded-md p-1 text-muted transition-colors hover:bg-panel hover:text-foreground" onClick={onClose} aria-label="Close forge modal">
            <X className="size-4" />
          </button>
        </div>
        {children}
      </div>
    </div>
  );
}
