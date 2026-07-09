import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { MessageSquarePlus, RefreshCw, Save, Plus, Trash2, ArrowUp, ArrowDown, Download, Upload, ListChecks, Pencil, X } from "lucide-react";
import { getJSON, postJSON } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { ErrorText } from "@/components/JsonView";
import { SkeletonList } from "@/components/ui/skeleton";
import { useUI } from "@/components/ui/feedback";
import { Page } from "@/components/ui/page";
import { EmptyState } from "@/components/ui/empty";
import { downloadText } from "@/lib/export";

// parsePromptsJSON normalises an imported prompt file into clean {title,text} rows,
// tolerating either a bare array or a {prompts:[…]} wrapper. Throws on bad JSON or
// a shape that yields no usable prompts, so the caller can surface a clear error.
export function parsePromptsJSON(text: string): Prompt[] {
  const data = JSON.parse(text);
  const arr = Array.isArray(data) ? data : Array.isArray(data?.prompts) ? data.prompts : null;
  if (!arr) throw new Error("expected an array of {title, text} (or a {prompts:[…]} object)");
  const out: Prompt[] = [];
  for (const e of arr) {
    const title = typeof e?.title === "string" ? e.title.trim() : "";
    const body = typeof e?.text === "string" ? e.text.trim() : "";
    if (title && body) out.push({ title, text: body });
  }
  if (out.length === 0) throw new Error("no valid prompts found in the file");
  return out;
}

// Prompts is the owner's library of saved chat prompts — reusable workflows
// ("draft my standup", "review the diff") defined once and launched from the Chat
// view's empty state. Stored daemon-side so they're the same from any browser.

export interface Prompt {
  title: string;
  text: string;
}

interface PromptsResp {
  prompts?: Prompt[];
}

export function Prompts() {
  const { toast } = useUI();
  const [items, setItems] = useState<Prompt[]>([]);
  const [saved, setSaved] = useState<Prompt[]>([]);
  const [err, setErr] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [editor, setEditor] = useState<{ index: number | null; prompt: Prompt } | null>(null);
  const fileRef = useRef<HTMLInputElement>(null);

  const reload = useCallback(async () => {
    setLoading(true);
    try {
      const r = await getJSON<PromptsResp>("/api/prompts");
      setItems(r.prompts || []);
      setSaved(r.prompts || []);
      setErr(null);
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    reload();
  }, [reload]);

  const dirty = useMemo(() => JSON.stringify(items) !== JSON.stringify(saved), [items, saved]);

  function add() {
    setEditor({ index: null, prompt: { title: "", text: "" } });
  }
  function edit(i: number) {
    setEditor({ index: i, prompt: items[i] || { title: "", text: "" } });
  }
  function applyEditor() {
    if (!editor) return;
    const next = { title: editor.prompt.title, text: editor.prompt.text };
    setItems((xs) => editor.index == null ? [...xs, next] : xs.map((p, j) => (j === editor.index ? next : p)));
    setEditor(null);
  }
  function remove(i: number) {
    setItems((xs) => xs.filter((_, j) => j !== i));
  }
  function move(i: number, dir: -1 | 1) {
    setItems((xs) => {
      const next = [...xs];
      const j = i + dir;
      if (j < 0 || j >= next.length) return xs;
      [next[i], next[j]] = [next[j], next[i]];
      return next;
    });
  }

  async function save() {
    // Drop blank rows before saving (the daemon does too, but keep the UI honest).
    const clean = items.map((p) => ({ title: p.title.trim(), text: p.text.trim() })).filter((p) => p.title && p.text);
    setSaving(true);
    try {
      const r = await postJSON<{ count?: number }>("/api/prompts/set", { prompts: clean });
      setItems(clean);
      setSaved(clean);
      toast(`Saved ${r.count ?? clean.length} prompt${(r.count ?? clean.length) === 1 ? "" : "s"}`, "success");
    } catch (e) {
      toast((e as Error).message, "error");
    } finally {
      setSaving(false);
    }
  }

  function exportPrompts() {
    const clean = items.map((p) => ({ title: p.title.trim(), text: p.text.trim() })).filter((p) => p.title && p.text);
    downloadText("agezt-prompts.json", JSON.stringify(clean, null, 2), "application/json");
  }

  async function onImportFile(file: File) {
    try {
      const parsed = parsePromptsJSON(await file.text());
      // Merge into the editor (skip exact title+text dupes); the owner reviews then Saves.
      setItems((cur) => {
        const seen = new Set(cur.map((p) => `${p.title}\u0000${p.text}`));
        const merged = [...cur];
        for (const p of parsed) {
          const k = `${p.title}\u0000${p.text}`;
          if (!seen.has(k)) {
            seen.add(k);
            merged.push(p);
          }
        }
        return merged;
      });
      toast(`Imported ${parsed.length} prompt${parsed.length === 1 ? "" : "s"} — review and Save`, "success");
    } catch (e) {
      toast(`Import failed: ${(e as Error).message}`, "error");
    }
  }

  return (
    <Page
      icon={MessageSquarePlus}
      title="Prompts"
      description="Reusable chat prompts, saved daemon-side and launched from Chat's empty state."
      width="readable"
      mode="scroll"
      className="max-w-3xl"
      actions={
        <>
          <Button variant="ghost" size="sm" onClick={() => fileRef.current?.click()} title="Import prompts from a JSON file">
            <Upload className="size-3.5" /> Import
          </Button>
          <Button variant="ghost" size="sm" onClick={exportPrompts} disabled={items.length === 0} title="Export prompts to a JSON file">
            <Download className="size-3.5" /> Export
          </Button>
          <Button size="sm" onClick={save} disabled={!dirty || saving} title="Save prompts">
            {saving ? <RefreshCw className="size-3.5 animate-spin" /> : <Save className="size-3.5" />} Save
          </Button>
          <Button variant="ghost" size="sm" onClick={reload} disabled={loading} title="Reload">
            <RefreshCw className={loading ? "size-3.5 animate-spin" : "size-3.5"} />
          </Button>
        </>
      }
    >
      <input
        ref={fileRef}
        type="file"
        accept="application/json,.json"
        className="hidden"
        aria-hidden="true"
        onChange={(e) => {
          const f = e.target.files?.[0];
          if (f) void onImportFile(f);
          e.target.value = ""; // allow re-importing the same file
        }}
      />

      {err ? (
        <ErrorText>{err}</ErrorText>
      ) : loading && items.length === 0 ? (
        <SkeletonList count={3} lines={3} />
      ) : (
        <PromptLibraryPanel title="Prompt library" count={items.length}>
          <div className="space-y-2">
            {items.length === 0 && (
              <EmptyState
                icon={MessageSquarePlus}
                title="No prompts yet"
                hint="Add a reusable prompt below, or import a JSON library — then Save."
              />
            )}
            {items.map((p, i) => (
              <div key={i} className="glass rounded-xl p-3">
                <div className="flex items-start gap-2">
                  <div className="min-w-0 flex-1">
                    <div className="truncate text-sm font-semibold text-foreground">{p.title || "Untitled prompt"}</div>
                    <div className="mt-1 line-clamp-2 text-xs text-muted">{p.text || "No prompt text yet"}</div>
                  </div>
                  <button onClick={() => move(i, -1)} disabled={i === 0} className="text-muted hover:text-foreground disabled:opacity-30" title="Move up">
                    <ArrowUp className="size-3.5" />
                  </button>
                  <button onClick={() => move(i, 1)} disabled={i === items.length - 1} className="text-muted hover:text-foreground disabled:opacity-30" title="Move down">
                    <ArrowDown className="size-3.5" />
                  </button>
                  <button onClick={() => edit(i)} className="text-muted hover:text-accent" title="Edit prompt" aria-label={`Edit prompt ${p.title || i + 1}`}>
                    <Pencil className="size-3.5" />
                  </button>
                  <button onClick={() => remove(i)} className="text-muted hover:text-bad" title="Delete prompt">
                    <Trash2 className="size-3.5" />
                  </button>
                </div>
              </div>
            ))}
            <Button variant="ghost" size="sm" onClick={add} title="Add a prompt">
              <Plus className="size-3.5" /> Add prompt
            </Button>
          </div>
        </PromptLibraryPanel>
      )}
      {editor && (
        <PromptModal title={editor.index == null ? "Add prompt" : `Edit ${editor.prompt.title || "prompt"}`} onClose={() => setEditor(null)}>
          <label className="flex flex-col gap-1 text-[11px] text-muted">
            Title
            <input
              value={editor.prompt.title}
              onChange={(e) => setEditor((cur) => cur && { ...cur, prompt: { ...cur.prompt, title: e.target.value } })}
              placeholder="Daily standup"
              aria-label="Prompt title"
              className="rounded-md border border-border bg-panel px-2 py-1 text-sm text-foreground outline-none focus-visible:border-accent"
            />
          </label>
          <label className="mt-3 flex flex-col gap-1 text-[11px] text-muted">
            Prompt text
            <textarea
              value={editor.prompt.text}
              onChange={(e) => setEditor((cur) => cur && { ...cur, prompt: { ...cur.prompt, text: e.target.value } })}
              placeholder="The prompt text sent to the agent..."
              aria-label="Prompt text"
              className="h-44 w-full resize-y rounded-md border border-border bg-panel p-2 text-sm text-foreground outline-none placeholder:text-muted/60 focus-visible:border-accent"
            />
          </label>
          <div className="mt-3 flex justify-end gap-2">
            <Button size="sm" variant="ghost" onClick={() => setEditor(null)}>Cancel</Button>
            <Button size="sm" onClick={applyEditor} disabled={!editor.prompt.title.trim() || !editor.prompt.text.trim()}>
              <Save className="size-3.5" /> Apply
            </Button>
          </div>
        </PromptModal>
      )}
    </Page>
  );
}

function PromptModal({ title, onClose, children }: { title: string; onClose: () => void; children: React.ReactNode }) {
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
        className="modal-in mt-10 w-full max-w-xl rounded-lg border border-border bg-card p-4 shadow-xl shadow-black/30"
        onClick={(e) => e.stopPropagation()}
        role="dialog"
        aria-modal="true"
        aria-label={title}
      >
        <div className="mb-3 flex items-center gap-2">
          <span className="grid size-8 place-items-center rounded-lg bg-accent/12 text-accent ring-1 ring-inset ring-accent/25">
            <MessageSquarePlus className="size-4" />
          </span>
          <h3 className="text-sm font-semibold text-foreground">{title}</h3>
          <button className="ml-auto rounded-md p-1 text-muted transition-colors hover:bg-panel hover:text-foreground" onClick={onClose} aria-label="Close prompt modal">
            <X className="size-4" />
          </button>
        </div>
        {children}
      </div>
    </div>
  );
}

function PromptLibraryPanel({ title, count, children }: { title: string; count: number; children: React.ReactNode }) {
  return (
    <section className="rounded-xl border border-border bg-card/70 p-3 shadow-e1">
      <div className="mb-2 flex items-center gap-2">
        <span className="grid size-8 shrink-0 place-items-center rounded-lg border border-accent/35 bg-accent/5 text-accent">
          <ListChecks className="size-4" />
        </span>
        <div className="min-w-0 flex-1">
          <h3 className="text-sm font-semibold">{title}</h3>
          <div className="truncate text-xs text-muted">{count} prompt{count === 1 ? "" : "s"}</div>
        </div>
      </div>
      {children}
    </section>
  );
}
