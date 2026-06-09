import { useCallback, useEffect, useMemo, useState } from "react";
import { MessageSquarePlus, RefreshCw, Save, Plus, Trash2, ArrowUp, ArrowDown } from "lucide-react";
import { getJSON, postJSON } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { ErrorText } from "@/components/JsonView";
import { SkeletonList } from "@/components/ui/skeleton";
import { useUI } from "@/components/ui/feedback";

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

  function update(i: number, patch: Partial<Prompt>) {
    setItems((xs) => xs.map((p, j) => (j === i ? { ...p, ...patch } : p)));
  }
  function add() {
    setItems((xs) => [...xs, { title: "", text: "" }]);
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

  return (
    <div className="mx-auto max-w-3xl space-y-4">
      <div className="flex flex-wrap items-center gap-2">
        <h2 className="flex items-center gap-2 text-sm font-semibold">
          <MessageSquarePlus className="size-4 text-accent" /> Prompts
        </h2>
        <span className="text-xs text-muted">your saved chat shortcuts</span>
        <div className="ml-auto flex items-center gap-2">
          <Button size="sm" onClick={save} disabled={!dirty || saving} title="Save prompts">
            {saving ? <RefreshCw className="size-3.5 animate-spin" /> : <Save className="size-3.5" />} Save
          </Button>
          <Button variant="ghost" size="sm" onClick={reload} disabled={loading} title="Reload">
            <RefreshCw className={loading ? "size-3.5 animate-spin" : "size-3.5"} />
          </Button>
        </div>
      </div>

      <p className="text-xs text-muted">
        Reusable prompts you can launch from the Chat's empty state — define a workflow once and run it with a click.
        Saved on the daemon, so they follow you across browsers.
      </p>

      {err ? (
        <ErrorText>{err}</ErrorText>
      ) : loading && items.length === 0 ? (
        <SkeletonList count={3} lines={3} />
      ) : (
        <div className="space-y-2">
          {items.length === 0 && (
            <div className="rounded-lg border border-dashed border-border p-6 text-center text-sm text-muted">
              No saved prompts yet. Add one below — it'll appear as a launch chip in a new chat.
            </div>
          )}
          {items.map((p, i) => (
            <div key={i} className="rounded-lg border border-border bg-card p-3">
              <div className="mb-2 flex items-center gap-2">
                <input
                  value={p.title}
                  onChange={(e) => update(i, { title: e.target.value })}
                  placeholder="Title (e.g. Daily standup)"
                  aria-label={`Prompt ${i + 1} title`}
                  className="min-w-0 flex-1 rounded-md border border-border bg-panel px-2 py-1 text-sm font-medium outline-none focus-visible:border-accent"
                />
                <button onClick={() => move(i, -1)} disabled={i === 0} className="text-muted hover:text-foreground disabled:opacity-30" title="Move up">
                  <ArrowUp className="size-3.5" />
                </button>
                <button onClick={() => move(i, 1)} disabled={i === items.length - 1} className="text-muted hover:text-foreground disabled:opacity-30" title="Move down">
                  <ArrowDown className="size-3.5" />
                </button>
                <button onClick={() => remove(i)} className="text-muted hover:text-bad" title="Delete prompt">
                  <Trash2 className="size-3.5" />
                </button>
              </div>
              <textarea
                value={p.text}
                onChange={(e) => update(i, { text: e.target.value })}
                placeholder="The prompt text sent to the agent…"
                aria-label={`Prompt ${i + 1} text`}
                className="h-20 w-full resize-y rounded-md border border-border bg-panel p-2 text-sm outline-none placeholder:text-muted/60 focus-visible:border-accent"
              />
            </div>
          ))}
          <Button variant="ghost" size="sm" onClick={add} title="Add a prompt">
            <Plus className="size-3.5" /> Add prompt
          </Button>
        </div>
      )}
    </div>
  );
}
