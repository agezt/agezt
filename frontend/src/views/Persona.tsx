import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Bot, RefreshCw, Save, Sparkles, Eraser } from "lucide-react";
import { getJSON, postJSON } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { ErrorText } from "@/components/JsonView";
import { Skeleton } from "@/components/ui/skeleton";
import { useUI } from "@/components/ui/feedback";

// Persona is the agent's standing instructions / personality — the default system
// prompt prepended to every run. Editing it here makes the daemon *yours*: tone,
// priorities, house rules. Edits apply live (the next run uses them) and persist.
// Memory / world / skill context still layer on top per run.

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
      toast(next.trim() ? "Persona saved — applies to the next run" : "Persona cleared — back to the default", "success");
    } catch (e) {
      toast((e as Error).message, "error");
    } finally {
      setSaving(false);
    }
  }

  function insertPreset(text: string) {
    setDraft(text);
    taRef.current?.focus();
  }

  const status = useMemo(() => {
    if (saved.trim()) return { label: "custom persona active", tone: "text-good" };
    return { label: "using the default (no custom persona)", tone: "text-muted" };
  }, [saved]);

  return (
    <div className="mx-auto max-w-3xl space-y-4">
      <div className="flex flex-wrap items-center gap-2">
        <h2 className="flex items-center gap-2 text-sm font-semibold">
          <Bot className="size-4 text-accent" /> Persona
        </h2>
        <span className="text-xs text-muted">the agent's standing instructions</span>
        <span className={`ml-auto text-xs ${status.tone}`}>● {status.label}</span>
      </div>

      <p className="text-xs text-muted">
        This is the default <span className="text-foreground/80">system prompt</span> prepended to every run — your
        Jarvis's personality, priorities, and house rules. Changes apply{" "}
        <span className="text-foreground/80">live</span> (the next run uses them) and persist across restarts. Per-run
        memory, world, and skill context still layer on top.
      </p>

      {err ? (
        <ErrorText>{err}</ErrorText>
      ) : loading ? (
        <Skeleton className="h-64 w-full" />
      ) : (
        <>
          <div className="rounded-lg border border-border bg-card">
            <textarea
              ref={taRef}
              value={draft}
              onChange={(e) => setDraft(e.target.value)}
              spellCheck={false}
              aria-label="Persona system prompt"
              placeholder="e.g. You are Jarvis. Be terse and proactive; take initiative on obvious next steps and state assumptions briefly…"
              className="h-64 w-full resize-y rounded-lg bg-transparent p-3 font-mono text-sm leading-relaxed text-foreground outline-none placeholder:text-muted/60"
            />
            <div className="flex items-center justify-between border-t border-border px-3 py-1.5 text-[11px] text-muted">
              <span className="tabular-nums">{chars.toLocaleString()} chars</span>
              {dirty && <span className="text-warn">unsaved changes</span>}
            </div>
          </div>

          <div className="flex flex-wrap items-center gap-2">
            <Button onClick={() => save(draft)} disabled={!dirty || saving} title="Save persona">
              {saving ? <RefreshCw className="size-3.5 animate-spin" /> : <Save className="size-3.5" />} Save
            </Button>
            <Button variant="ghost" onClick={() => setDraft(saved)} disabled={!dirty || saving} title="Discard edits">
              Discard
            </Button>
            <Button
              variant="ghost"
              onClick={() => save("")}
              disabled={saving || (!saved.trim() && !draft.trim())}
              title="Clear the custom persona"
            >
              <Eraser className="size-3.5" /> Clear
            </Button>
          </div>

          <div className="space-y-1.5">
            <div className="flex items-center gap-1.5 text-[11px] font-semibold uppercase tracking-wider text-muted">
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
