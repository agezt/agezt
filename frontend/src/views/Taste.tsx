import { useEffect, useMemo, useState } from "react";
import { Sparkles, Plus, RefreshCw, Trash2, Globe, Target } from "lucide-react";
import { getJSON, postJSON } from "@/lib/api";
import { cn } from "@/lib/utils";
import { Page } from "@/components/ui/page";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { ErrorText } from "@/components/JsonView";
import { SkeletonList } from "@/components/ui/skeleton";
import { EmptyState } from "@/components/ui/empty";
import { LoadMoreFooter } from "@/components/ui/load-more-footer";

// TASTE_WINDOW is how many exemplar cards render at once. /api/taste has no
// cursor, so the whole list arrives in one fetch — the window keeps a large
// exemplar library from ballooning the DOM; a Load-more footer grows it
// client-side. The metrics up top stay computed over the FULL list.
const TASTE_WINDOW = 60;

export interface TasteExemplar {
  id: string;
  title?: string;
  body?: string;
  scope?: string;
  tags?: string[];
  created_ms?: number;
  updated_ms?: number;
}

interface TasteListData {
  exemplars?: TasteExemplar[];
  count?: number;
}

export function tasteScopeLabel(e: TasteExemplar): string {
  return e.scope?.trim() ? e.scope.trim() : "every run";
}

export function Taste() {
  const [exemplars, setExemplars] = useState<TasteExemplar[]>([]);
  const [err, setErr] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [busy, setBusy] = useState(false);
  const [title, setTitle] = useState("");
  const [body, setBody] = useState("");
  const [scope, setScope] = useState("");
  const [win, setWin] = useState(TASTE_WINDOW);

  const globalCount = useMemo(() => exemplars.filter((e) => !e.scope?.trim()).length, [exemplars]);

  async function reload() {
    setLoading(true);
    try {
      const d = await getJSON<TasteListData>("/api/taste");
      setExemplars(Array.isArray(d.exemplars) ? d.exemplars : []);
      setErr(null);
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setLoading(false);
    }
  }

  async function mutate(path: string, bodyArgs: Record<string, unknown>, after?: () => void) {
    setBusy(true);
    try {
      await postJSON(path, bodyArgs);
      after?.();
      await reload();
      setErr(null);
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setBusy(false);
    }
  }

  useEffect(() => {
    void reload();
  }, []);

  return (
    <Page
      icon={Sparkles}
      title="Taste"
      description="Curated “what good looks like” exemplars injected into runs to anchor output quality."
      actions={
        <Button variant="ghost" size="sm" onClick={() => reload()} disabled={loading} title="Reload">
          <RefreshCw className={cn("size-3.5", loading && "animate-spin")} />
        </Button>
      }
    >
      <section className="grid gap-2 sm:grid-cols-2">
        <Metric label="Exemplars" value={exemplars.length} />
        <Metric label="Global (every run)" value={globalCount} tone="accent" />
      </section>

      <form
        className="space-y-2 rounded-lg border border-border bg-card/70 p-3"
        onSubmit={(e) => {
          e.preventDefault();
          if (title.trim() && body.trim()) {
            mutate("/api/taste/create", { title: title.trim(), body: body.trim(), scope: scope.trim() }, () => {
              setTitle("");
              setBody("");
              setScope("");
            });
          }
        }}
      >
        <div className="flex flex-wrap gap-2">
          <input
            value={title}
            onChange={(e) => setTitle(e.target.value)}
            aria-label="Exemplar title"
            placeholder="Title — e.g. Good PR summary"
            className="h-9 min-w-0 flex-1 rounded-md border border-border bg-panel px-3 text-sm outline-none focus-visible:border-accent"
          />
          <input
            value={scope}
            onChange={(e) => setScope(e.target.value)}
            aria-label="Exemplar scope"
            placeholder="scope (agent slug — blank = all)"
            className="h-9 w-56 rounded-md border border-border bg-panel px-3 text-sm outline-none focus-visible:border-accent"
          />
        </div>
        <textarea
          value={body}
          onChange={(e) => setBody(e.target.value)}
          aria-label="Exemplar body"
          rows={3}
          placeholder="What good looks like — a sample answer, snippet, or artifact the model should match in quality and style."
          className="w-full resize-y rounded-md border border-border bg-panel px-3 py-2 text-sm outline-none focus-visible:border-accent"
        />
        <div className="flex justify-end">
          <Button type="submit" size="sm" disabled={busy || !title.trim() || !body.trim()}>
            <Plus className="size-3.5" /> Add exemplar
          </Button>
        </div>
      </form>

      {err && <ErrorText>{err}</ErrorText>}

      {loading && exemplars.length === 0 ? (
        <SkeletonList count={3} lines={3} />
      ) : exemplars.length === 0 ? (
        <EmptyState icon={Sparkles} title="No exemplars yet" hint="Add a “what good looks like” example. Global ones shape every run; scoped ones apply to a named agent." />
      ) : (
        <div className="grid gap-3">
          {exemplars.slice(0, win).map((e) => (
            <article key={e.id} className="rounded-lg border border-border bg-card/75 p-3">
              <div className="mb-1 flex flex-wrap items-start justify-between gap-2">
                <div className="flex min-w-0 items-center gap-2">
                  <h3 className="truncate text-sm font-semibold">{e.title || e.id}</h3>
                  <Badge variant={e.scope?.trim() ? "accent" : "default"}>
                    {e.scope?.trim() ? <Target className="mr-1 size-3" /> : <Globe className="mr-1 size-3" />}
                    {tasteScopeLabel(e)}
                  </Badge>
                </div>
                <Button size="sm" variant="ghost" disabled={busy} title="Remove" onClick={() => mutate("/api/taste/delete", { id: e.id })}>
                  <Trash2 className="size-3.5" />
                </Button>
              </div>
              <pre className="whitespace-pre-wrap break-words rounded-md border border-border/60 bg-background/45 p-2 text-xs text-foreground/90">{e.body}</pre>
              {e.tags && e.tags.length > 0 && (
                <div className="mt-1 flex flex-wrap gap-1">
                  {e.tags.map((t) => (
                    <span key={t} className="rounded-full border border-border bg-background/55 px-1.5 py-0.5 text-[10px] text-muted">
                      {t}
                    </span>
                  ))}
                </div>
              )}
            </article>
          ))}
          {exemplars.length > TASTE_WINDOW && (
            <LoadMoreFooter
              hasMore={win < exemplars.length}
              loadingMore={false}
              onLoadMore={() => setWin((w) => w + TASTE_WINDOW)}
              pageSize={Math.min(TASTE_WINDOW, Math.max(1, exemplars.length - win))}
              label="exemplars"
            />
          )}
        </div>
      )}
    </Page>
  );
}

function Metric({ label, value, tone }: { label: string; value: number; tone?: "accent" }) {
  return (
    <div className="rounded-lg border border-border bg-card/70 p-3">
      <div className="text-xs uppercase tracking-normal text-muted">{label}</div>
      <div className={cn("mt-1 text-2xl font-semibold tabular-nums", tone === "accent" ? "text-accent" : "")}>{value}</div>
    </div>
  );
}
