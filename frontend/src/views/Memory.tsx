import { useEffect, useMemo, useState } from "react";
import { Brain, RefreshCw, Search, Trash2 } from "lucide-react";
import { getJSON, postAction } from "@/lib/api";
import { cn, fmtTime } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { useUI } from "@/components/ui/feedback";
import { SkeletonGrid } from "@/components/ui/skeleton";
import { Muted, ErrorText } from "@/components/JsonView";
import { BreakdownBar } from "@/components/Widgets";

interface MemRecord {
  id?: string;
  type?: string;
  subject?: string;
  content?: string;
  confidence?: number;
  created_ms?: number;
  last_seen_ms?: number;
  source_event?: string;
  tags?: Record<string, string>;
}

// Memory is the knowledge browser: every durable fact the agent has stored —
// searchable by subject/content/tag, each card showing its type, confidence,
// age and source, with one-click forget.
export function Memory() {
  const ui = useUI();
  const [records, setRecords] = useState<MemRecord[] | null>(null);
  const [q, setQ] = useState("");
  const [err, setErr] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [busy, setBusy] = useState<string | null>(null);

  async function reload() {
    setLoading(true);
    try {
      const d = await getJSON<{ records?: MemRecord[] }>("/api/memory");
      setRecords(d.records || []);
      setErr(null);
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setLoading(false);
    }
  }
  useEffect(() => {
    reload();
  }, []);

  async function forget(id: string, subject?: string) {
    const ok = await ui.confirm({
      title: "Forget this memory?",
      message: subject ? `“${subject}” will be permanently removed.` : "This memory will be permanently removed.",
      confirmLabel: "Forget",
      danger: true,
    });
    if (!ok) return;
    setBusy(id);
    try {
      await postAction("/api/memory/forget", { id });
      ui.toast("Memory forgotten", "success");
      await reload();
    } catch (e) {
      ui.toast(`forget failed: ${(e as Error).message}`, "error");
    } finally {
      setBusy(null);
    }
  }

  // byType drives the breakdown bar (over ALL records, not the filtered view).
  const byType = useMemo(() => {
    const c: Record<string, number> = {};
    for (const r of records || []) c[(r.type || "FACT").toUpperCase()] = (c[(r.type || "FACT").toUpperCase()] || 0) + 1;
    return Object.entries(c).map(([label, count]) => ({ label, count }));
  }, [records]);

  const f = q.trim().toLowerCase();
  const shown = useMemo(() => {
    const list = [...(records || [])].sort((a, b) => (b.created_ms || 0) - (a.created_ms || 0));
    if (!f) return list;
    return list.filter((r) =>
      `${r.subject} ${r.content} ${r.type} ${Object.values(r.tags || {}).join(" ")}`.toLowerCase().includes(f),
    );
  }, [records, f]);

  return (
    <div className="flex h-full min-h-0 flex-col gap-3">
      <div className="flex flex-wrap items-center gap-2">
        <h2 className="flex items-center gap-2 text-sm font-semibold">
          <Brain className="size-4 text-accent" /> Memory
        </h2>
        {records && <span className="text-xs text-muted">{shown.length}/{records.length}</span>}
        <div className="relative ml-auto">
          <Search className="pointer-events-none absolute left-2 top-1/2 size-3.5 -translate-y-1/2 text-muted" />
          <input
            value={q}
            onChange={(e) => setQ(e.target.value)}
            placeholder="search memories…"
            className="h-7 w-48 rounded-md border border-border bg-panel pl-7 pr-2 text-xs outline-none focus:border-accent"
          />
        </div>
        <Button variant="ghost" size="sm" onClick={reload} disabled={loading}>
          <RefreshCw className={cn("size-3.5", loading && "animate-spin")} />
        </Button>
      </div>

      {err ? (
        <ErrorText>{err}</ErrorText>
      ) : !records ? (
        <SkeletonGrid count={6} />
      ) : shown.length === 0 ? (
        <Muted>{records.length === 0 ? "no memories yet" : "no memories match"}</Muted>
      ) : (
        <div className="min-h-0 flex-1 space-y-2 overflow-auto">
          {byType.length > 0 && <BreakdownBar segments={byType} />}
          <ul className="grid grid-cols-1 gap-2 lg:grid-cols-2">
            {shown.map((r, i) => (
              <li key={r.id || i} className="rounded-lg border border-border bg-card p-3">
                <div className="mb-1 flex items-center gap-2">
                  {r.type && (
                    <span className="rounded bg-accent/15 px-1.5 py-0.5 text-[10px] font-semibold uppercase tracking-wider text-accent">
                      {r.type}
                    </span>
                  )}
                  <span className="truncate text-sm font-semibold">{r.subject || "—"}</span>
                  <div className="ml-auto flex items-center gap-2">
                    {r.confidence != null && (
                      <span className="text-[10px] tabular-nums text-muted">conf {Math.round((r.confidence || 0) * 100)}%</span>
                    )}
                    {r.id && (
                      <button
                        onClick={() => forget(r.id!, r.subject)}
                        disabled={busy === r.id}
                        title="Forget this memory"
                        className="text-muted transition-colors hover:text-bad disabled:opacity-50"
                      >
                        <Trash2 className="size-3.5" />
                      </button>
                    )}
                  </div>
                </div>
                <p className="whitespace-pre-wrap break-words text-xs text-foreground/85">{r.content}</p>
                <div className="mt-2 flex flex-wrap items-center gap-x-3 gap-y-1 text-[10px] text-muted">
                  {r.created_ms ? <span>{fmtTime(r.created_ms)}</span> : null}
                  {r.tags &&
                    Object.entries(r.tags).map(([k, v]) => (
                      <span key={k} className="rounded-full bg-panel px-1.5 py-0.5">
                        {k}:{v}
                      </span>
                    ))}
                </div>
              </li>
            ))}
          </ul>
        </div>
      )}
    </div>
  );
}
