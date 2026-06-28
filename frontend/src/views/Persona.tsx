import { useCallback, useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { Bot, RefreshCw, Save, Sparkles, Eraser, Pencil, X } from "lucide-react";
import { getJSON, postJSON } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { ErrorText } from "@/components/JsonView";
import { Skeleton } from "@/components/ui/skeleton";
import { useUI } from "@/components/ui/feedback";
import { PageHeader } from "@/components/ui/page-header";

// Persona is the legacy API name for the daemon's default identity instructions.
// They apply only to runs that are not bound to a roster agent. Editing here
// changes tone, priorities, and house rules for that default identity; roster
// agent souls remain owned by their profiles.

interface PersonaResp {
  system?: string;
  set?: boolean;
}

// A few starting points the owner can drop in and adapt — not prescriptive.
const PRESETS: { label: string; text: string }[] = [
  {
    label: "Terse & proactive",
    text:
      "You are the user's personal agent. Be terse and direct — no filler, no preamble. " +
      "Take initiative: when a task has an obvious next step, do it and report what you did. " +
      "State assumptions briefly rather than asking for confirmation on small things.",
  },
  {
    label: "Careful & explicit",
    text:
      "You are a careful, methodical assistant. Think step by step, surface trade-offs, and " +
      "confirm before any irreversible or outward-facing action. Prefer correctness over speed, " +
      "and always explain your reasoning concisely.",
  },
  {
    label: "Friendly concierge",
    text:
      "You are a warm, helpful concierge. Speak naturally and conversationally, anticipate needs, " +
      "and keep the user informed of progress. Be encouraging, but never hide problems — flag them early.",
  },
];

export function Persona() {
  const { toast } = useUI();
  const [saved, setSaved] = useState("");
  const [draft, setDraft] = useState("");
  const [err, setErr] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [editorOpen, setEditorOpen] = useState(false);
  const taRef = useRef<HTMLTextAreaElement>(null);

  const reload = useCallback(async () => {
    setLoading(true);
    try {
      const r = await getJSON<PersonaResp>("/api/persona");
      setSaved(r.system || "");
      setDraft(r.system || "");
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

  const dirty = draft !== saved;
  const chars = draft.length;

  async function save(next: string) {
    setSaving(true);
    try {
      await postJSON("/api/persona/set", { system: next });
      setSaved(next);
      setDraft(next);
      setEditorOpen(false);
      toast(next.trim() ? "Default identity saved — applies to the next default run" : "Default identity cleared", "success");
    } catch (e) {
      toast((e as Error).message, "error");
    } finally {
      setSaving(false);
    }
  }

  function insertPreset(text: string) {
    setDraft(text);
    setEditorOpen(true);
    setTimeout(() => taRef.current?.focus(), 0);
  }

  const status = useMemo(() => {
    if (saved.trim()) return { label: "custom default identity active", tone: "text-good" };
    return { label: "using built-in default identity", tone: "text-muted" };
  }, [saved]);

  return (
    <div className="mx-auto max-w-3xl space-y-4">
      <PageHeader
        icon={Bot}
        title="Default Identity"
        description="daemon fallback instructions for runs without a roster agent"
        actions={<span className={`text-xs ${status.tone}`}>● {status.label}</span>}
      />

      <p className="text-xs text-muted">
        These are the daemon's default <span className="text-foreground/80">identity instructions</span> for runs
        that are not bound to a roster agent. Changes apply <span className="text-foreground/80">live</span> to the
        next default-identity run and persist across restarts. Roster agents keep their own soul, model, memory,
        skills, and budget.
      </p>

      {err ? (
        <ErrorText>{err}</ErrorText>
      ) : loading ? (
        <Skeleton className="h-64 w-full" />
      ) : (
        <>
          <div className="glass flex flex-wrap items-center gap-3 rounded-xl p-3">
            <span className="grid size-10 place-items-center rounded-lg bg-accent/12 text-accent">
              <Bot className="size-5" />
            </span>
            <div className="min-w-0 flex-1">
              <div className="flex flex-wrap items-center gap-2">
                <span className="text-sm font-semibold">Default identity</span>
                <span className={`text-xs ${status.tone}`}>● {status.label}</span>
              </div>
              <p className="mt-0.5 text-xs text-muted">
                {saved.trim() ? `${saved.length.toLocaleString()} chars saved` : "Built-in identity is active"}
                {dirty ? " · unsaved draft" : ""}
              </p>
            </div>
            <Button onClick={() => setEditorOpen(true)} title="Edit default identity">
              <Pencil className="size-3.5" /> Edit
            </Button>
            <Button
              variant="ghost"
              onClick={() => save("")}
              disabled={saving || (!saved.trim() && !draft.trim())}
              title="Clear the custom default identity"
            >
              <Eraser className="size-3.5" /> Clear
            </Button>
          </div>

          {editorOpen && (
            <PersonaModal title="Edit default identity" onClose={() => setEditorOpen(false)}>
              <textarea
                ref={taRef}
                value={draft}
                onChange={(e) => setDraft(e.target.value)}
                spellCheck={false}
                aria-label="Default identity instructions"
                placeholder="e.g. You are Jarvis. Be terse and proactive; take initiative on obvious next steps and state assumptions briefly…"
                className="h-64 w-full resize-y rounded-lg border border-border bg-panel p-3 font-mono text-sm leading-relaxed text-foreground outline-none placeholder:text-muted/60 focus-visible:border-accent"
              />
              <div className="flex items-center justify-between px-1 text-[11px] text-muted">
                <span className="tabular-nums">{chars.toLocaleString()} chars</span>
                {dirty && <span className="text-warn">unsaved changes</span>}
              </div>
              <div className="flex flex-wrap items-center justify-end gap-2">
                <Button variant="ghost" onClick={() => setDraft(saved)} disabled={!dirty || saving} title="Discard edits">
                  Discard
                </Button>
                <Button onClick={() => save(draft)} disabled={!dirty || saving} title="Save default identity">
                  {saving ? <RefreshCw className="size-3.5 animate-spin" /> : <Save className="size-3.5" />} Save
                </Button>
              </div>
            </PersonaModal>
          )}

          <div className="space-y-1.5">
            <div className="flex items-center gap-1.5 text-[11px] font-semibold uppercase tracking-normal text-muted">
              <Sparkles className="size-3.5" /> Starting points
            </div>
            <div className="flex flex-wrap gap-1.5">
              {PRESETS.map((p) => (
                <button
                  key={p.label}
                  onClick={() => insertPreset(p.text)}
                  className="rounded-full border border-border px-2.5 py-1 text-xs text-muted transition-colors hover:border-accent hover:text-foreground"
                  title="Replace the editor with this template"
                >
                  {p.label}
                </button>
              ))}
            </div>
          </div>
        </>
      )}
    </div>
  );
}

function PersonaModal({ title, children, onClose }: { title: string; children: ReactNode; onClose: () => void }) {
  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-bg/75 p-4 backdrop-blur-sm" role="dialog" aria-modal="true" aria-label={title}>
      <div className="glass flex max-h-[86vh] w-full max-w-3xl flex-col overflow-hidden rounded-xl border border-accent/25 shadow-e3">
        <div className="flex items-center gap-2 border-b border-border/70 px-4 py-3">
          <span className="grid size-8 place-items-center rounded-lg bg-accent/12 text-accent">
            <Bot className="size-4" />
          </span>
          <div className="min-w-0">
            <h2 className="truncate text-sm font-semibold text-foreground">{title}</h2>
            <p className="text-xs text-muted">Only default runs use this; roster agents keep their own soul.</p>
          </div>
          <Button variant="ghost" size="icon" onClick={onClose} className="ml-auto" aria-label="Close persona modal">
            <X className="size-4" />
          </Button>
        </div>
        <div className="flex min-h-0 flex-col gap-3 overflow-auto p-4">{children}</div>
      </div>
    </div>
  );
}
