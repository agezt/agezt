import { useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { Brain, RefreshCw, Search, Trash2, Plus, X, Pencil, Save, Download, Upload, Lock, Share2, Users, Sparkles, ShieldCheck, AlertTriangle, UserRound, Tags, FileText, History, type LucideIcon } from "lucide-react";
import { getJSON, postAction, postJSON } from "@/lib/api";
import { downloadText } from "@/lib/export";
import { cn, fmtTime } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { useUI } from "@/components/ui/feedback";
import { SkeletonGrid } from "@/components/ui/skeleton";
import { EmptyState } from "@/components/ui/empty";
import { Muted, ErrorText } from "@/components/JsonView";
import { BreakdownBar } from "@/components/Widgets";
import { Page } from "@/components/ui/page";
import { MetricWidget, MetricGrid } from "@/components/ui/metric-widget";
import { Badge } from "@/components/ui/badge";
import { Disclosure } from "@/components/ui/disclosure";
import { useMemoryLogPager, useMemoryPager } from "@/lib/cursorPager";
import { LoadMoreFooter } from "@/components/ui/load-more-footer";
import { LogHistoryPanel } from "@/components/LogHistoryPanel";

interface MemRecord {
  id?: string;
  type?: string;
  subject?: string;
  content?: string;
  confidence?: number;
  created_ms?: number;
  last_seen_ms?: number;
  source_event?: string;
  // Provenance (M851): who added the record, and who most recently wrote it.
  added_by?: string;
  updated_by?: string;
  tags?: Record<string, string>;
}

interface MemoryAudit {
  total?: number;
  usable?: number;
  expired?: number;
  suspended?: number;
  contradiction_load?: number;
  contradictions?: Array<{ key?: string; ids?: string[]; subject?: string; type?: string; scope?: string }>;
}

// parseMemoryJSON normalises an exported memory file into a list of re-addable
// `memory_add` arg objects. Accepts a bare array, a {memory:[…]} or a {records:[…]}
// wrapper (the list shape). Keeps only entries with content (what the daemon
// requires), carrying optional subject/type/confidence; drops kernel-assigned
// identity/lifecycle fields (id/timestamps/source_event/tags) so a re-add stores
// fresh — though memory is content-addressed, so re-importing the same fact dedupes
// rather than duplicating. Throws on bad JSON / nothing valid.
export function parseMemoryJSON(text: string): Record<string, unknown>[] {
  const data = JSON.parse(text);
  const arr = Array.isArray(data)
    ? data
    : Array.isArray((data as { memory?: unknown[] })?.memory)
      ? (data as { memory: unknown[] }).memory
      : Array.isArray((data as { records?: unknown[] })?.records)
        ? (data as { records: unknown[] }).records
        : null;
  if (!arr) throw new Error("expected an array of memories (or a {memory:[…]} / {records:[…]} wrapper)");
  const out: Record<string, unknown>[] = [];
  for (const raw of arr) {
    if (!raw || typeof raw !== "object" || Array.isArray(raw)) continue;
    const r = raw as Record<string, unknown>;
    const content = typeof r.content === "string" ? r.content.trim() : "";
    if (!content) continue;
    const args: Record<string, unknown> = { content };
    if (typeof r.subject === "string" && r.subject.trim()) args.subject = r.subject.trim();
    if (typeof r.type === "string" && r.type.trim()) args.type = r.type.trim().toUpperCase();
    if (typeof r.confidence === "number" && r.confidence > 0) args.confidence = r.confidence;
    out.push(args);
  }
  if (out.length === 0) throw new Error("no valid memories (each needs content) found");
  return out;
}

// Memory is the knowledge browser: every durable fact the agent has stored —
// searchable by subject/content/tag, each card showing its type, confidence,
// age and source, with one-click forget.
// PROFILE_PREFIX namespaces the learned operator-profile facets (M1000); they are
// ordinary PREFERENCE records the daemon injects into every run.
const PROFILE_PREFIX = "operator profile: ";

// MEM_PAGE_SIZE is the cursor-pager page size: a busy store no longer ships
// every record on first paint — the view loads a page at a time.
const MEM_PAGE_SIZE = 100;

export function Memory() {
  const ui = useUI();
  // Cursor-paginated record list: /api/memory pages on (CreatedMS, ID) with
  // limit+cursor; the pager pulls MEM_PAGE_SIZE at a time and LoadMoreFooter
  // extends. Client-side search/scope filters apply over the loaded pages.
  const {
    paged,
    error: err,
    loading,
    loadMore,
    loadingMore,
    moreError,
    hasMore,
    reload: reloadList,
  } = useMemoryPager(MEM_PAGE_SIZE);
  const records = paged as unknown as MemRecord[];
  // Store-wide active-record count from the list envelope ("N of TOTAL loaded").
  const [total, setTotal] = useState<number | null>(null);
  const [q, setQ] = useState("");
  const [busy, setBusy] = useState<string | null>(null);
  const [bulkBusy, setBulkBusy] = useState(false);
  // Bulk-cleanup selection: ids of currently loaded records ticked for a
  // one-shot soft-delete via /api/memory/bulk_forget.
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [audit, setAudit] = useState<MemoryAudit | null>(null);
  const [showForm, setShowForm] = useState(false);
  const [editingId, setEditingId] = useState<string | null>(null);
  // Scope filter (M915): null = all, "" = shared brain, "<scope>" = that
  // agent's private notes. Each agent keeps its own memory; this lets the
  // owner browse them side by side.
  const [scopeFilter, setScopeFilter] = useState<string | null>(null);
  const fileRef = useRef<HTMLInputElement>(null);

  // Cursor-paginated write/forget history (the journal-backed /api/memory_log).
  const {
    paged: writeLogRows,
    loading: writeLogLoading,
    loadMore: loadMoreWriteLog,
    loadingMore: loadingMoreWriteLog,
    moreError: writeLogError,
    hasMore: hasMoreWriteLog,
  } = useMemoryLogPager(50);

  // loadedOnce discriminates boot (skeleton) from a genuinely empty store:
  // the pager starts with an empty page for one pre-fetch frame, so track the
  // first list load's completion explicitly.
  const sawLoad = useRef(false);
  const [loadedOnce, setLoadedOnce] = useState(false);
  useEffect(() => {
    if (loading) sawLoad.current = true;
    else if (sawLoad.current) setLoadedOnce(true);
  }, [loading]);

  // Selection survives load-more but drops ids that left the loaded set
  // (forgotten elsewhere, or a reload shrank the pages).
  useEffect(() => {
    setSelected((prev) => {
      if (prev.size === 0) return prev;
      const live = new Set(paged.map((r) => r.id));
      const next = new Set([...prev].filter((id) => live.has(id)));
      return next.size === prev.size ? prev : next;
    });
  }, [paged]);

  // Export fetches its own snapshot (up to the server's 1000-record page cap)
  // so a backup isn't silently truncated to the pages loaded on screen.
  async function exportMemory() {
    try {
      const d = await getJSON<{ records?: MemRecord[] }>("/api/memory", { limit: "1000" });
      downloadText("agezt-memory.json", JSON.stringify({ version: 1, memory: d.records ?? [] }, null, 2), "application/json");
    } catch (e) {
      ui.toast(`export failed: ${(e as Error).message}`, "error");
    }
  }

  // Restore memories from a file: re-add each via memory_add. Memory is
  // content-addressed, so re-importing a fact the agent already knows dedupes
  // (no duplicate) — import is naturally idempotent, unlike standing/schedules.
  async function importMemory(file: File) {
    try {
      const list = parseMemoryJSON(await file.text());
      let added = 0;
      for (const args of list) {
        try {
          await postJSON("/api/memory/add", args);
          added++;
        } catch {
          /* skip one the daemon rejects; keep importing the rest */
        }
      }
      ui.toast(`Imported ${added}/${list.length} memor${list.length === 1 ? "y" : "ies"}`, added ? "success" : "error");
      void reload();
    } catch (e) {
      ui.toast(`Import failed: ${(e as Error).message}`, "error");
    }
  }

  // reloadMeta refreshes the sidecar reads: the hygiene audit plus the store's
  // total active-record count. The list itself is cursor-paged, so the header's
  // "N of TOTAL loaded" needs the envelope total from a cheap limit=1 probe
  // (the pager hook doesn't surface it).
  async function reloadMeta() {
    const [a, t] = await Promise.all([
      getJSON<MemoryAudit>("/api/memory/audit").catch(() => null),
      getJSON<{ total?: number }>("/api/memory", { limit: "1" }).catch(() => null),
    ]);
    setAudit(a && typeof a.usable === "number" ? a : null);
    setTotal(t && typeof t.total === "number" ? t.total : null);
  }

  // reload refreshes both the paged list (back to page 1) and the meta reads.
  async function reload() {
    reloadList();
    await reloadMeta();
  }
  // rebuildProfile (M1000) synthesizes the operator profile from accumulated
  // memory on demand — the daemon also does this on a daily timer.
  async function rebuildProfile() {
    try {
      const res = await postAction<{ facets_written?: number; input_records?: number }>("/api/profile/rebuild", {});
      const n = res.facets_written ?? 0;
      const inp = res.input_records ?? 0;
      ui.toast(inp === 0 ? "Nothing to learn from yet — no accumulated memory" : `Operator profile rebuilt: ${n} facet${n === 1 ? "" : "s"} from ${inp} memories`, inp === 0 ? "info" : "success");
      await reload();
    } catch (e) {
      ui.toast((e as Error).message, "error");
    }
  }
  useEffect(() => {
    // The pager fetches the list on mount by itself; only the meta reads
    // need a kick here.
    void reloadMeta();
    // eslint-disable-next-line react-hooks/exhaustive-deps
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

  // prune hard-removes soft-deleted (forgotten/superseded) records older than the
  // cutoff — the "no memory-bomb" sweep (M857). Dry-run first, then confirm.
  const PRUNE_DAYS = 30;
  async function prune() {
    try {
      const dry = await postAction<{ prunable?: number }>("/api/memory/prune", { older_than_days: String(PRUNE_DAYS), dry_run: "true" });
      const n = dry.prunable ?? 0;
      if (n === 0) {
        ui.toast("Nothing to prune — no soft-deleted records older than 30 days", "success");
        return;
      }
      const ok = await ui.confirm({
        title: "Prune old deleted memories?",
        message: `${n} forgotten/superseded record${n === 1 ? "" : "s"} older than ${PRUNE_DAYS} days will be permanently removed. Active memories are never touched.`,
        confirmLabel: "Prune",
        danger: true,
      });
      if (!ok) return;
      const res = await postAction<{ pruned?: number }>("/api/memory/prune", { older_than_days: String(PRUNE_DAYS), dry_run: "false" });
      ui.toast(`Pruned ${res.pruned ?? 0} record(s)`, "success");
      await reload();
    } catch (e) {
      ui.toast(`prune failed: ${(e as Error).message}`, "error");
    }
  }

  // tidy collapses the near-duplicate auto-distilled notes that built up before
  // the write-time subject gate (M994) — the "1000+ nonsense entries" cleanup. Keeps
  // the strongest note per subject, forgets the rest (reversible). Dry-run first.
  async function tidy() {
    try {
      const dry = await postAction<{ collapsed?: number }>("/api/memory/tidy", { dry_run: "true" });
      const n = dry.collapsed ?? 0;
      if (n === 0) {
        ui.toast("Nothing to tidy — no duplicate distilled notes", "success");
        return;
      }
      const ok = await ui.confirm({
        title: "Tidy duplicate distilled notes?",
        message: `${n} near-duplicate auto-distilled note${n === 1 ? "" : "s"} will be collapsed — the strongest note per subject is kept, the rest forgotten (reversible). Curated memories you wrote are never touched.`,
        confirmLabel: "Tidy",
      });
      if (!ok) return;
      const res = await postAction<{ collapsed?: number }>("/api/memory/tidy", { dry_run: "false" });
      ui.toast(`Collapsed ${res.collapsed ?? 0} duplicate note(s)`, "success");
      await reload();
    } catch (e) {
      ui.toast(`tidy failed: ${(e as Error).message}`, "error");
    }
  }

  async function cleanLowValue() {
    try {
      const dry = await postAction<{ rejected?: number; scanned?: number }>("/api/memory/clean", { dry_run: "true" });
      const n = dry.rejected ?? 0;
      if (n === 0) {
        ui.toast("Nothing to clean — no low-value active memories found", "success");
        return;
      }
      const ok = await ui.confirm({
        title: "Clean low-value memories?",
        message: `${n} record${n === 1 ? "" : "s"} look like logs, transient notes, or low-value automatic memories. They will be permanently deleted because log output is not memory.`,
        confirmLabel: "Delete",
      });
      if (!ok) return;
      const res = await postAction<{ removed?: number }>("/api/memory/clean", { dry_run: "false" });
      ui.toast(`Deleted ${res.removed ?? 0} low-value record(s)`, "success");
      await reload();
    } catch (e) {
      ui.toast(`clean failed: ${(e as Error).message}`, "error");
    }
  }

  // promote shares a private (agent-scoped) record with every agent (M915) —
  // the selective-sharing valve: agents keep their own notes by default, the
  // owner promotes the few worth everyone knowing.
  async function promote(id: string, scope: string, subject?: string) {
    const ok = await ui.confirm({
      title: "Share this memory with every agent?",
      message: `${subject ? `“${subject}” is` : "This note is"} currently private to “${scope}”. Promoting moves it into the shared memory all agents recall.`,
      confirmLabel: "Share",
    });
    if (!ok) return;
    setBusy(id);
    try {
      await postAction("/api/memory/promote", { id });
      ui.toast("Memory shared with every agent", "success");
      await reload();
    } catch (e) {
      ui.toast(`promote failed: ${(e as Error).message}`, "error");
    } finally {
      setBusy(null);
    }
  }

  // byType drives the breakdown bar (over all LOADED records, not the filtered view).
  const byType = useMemo(() => {
    const c: Record<string, number> = {};
    for (const r of records) c[(r.type || "FACT").toUpperCase()] = (c[(r.type || "FACT").toUpperCase()] || 0) + 1;
    return Object.entries(c).map(([label, count]) => ({ label, count }));
  }, [records]);

  // Per-agent memory map (M915): shared count + each private scope's count,
  // driving the filter chips.
  const scopes = useMemo(() => {
    const c = new Map<string, number>();
    let shared = 0;
    for (const r of records) {
      const s = r.tags?.scope || "";
      if (s) c.set(s, (c.get(s) || 0) + 1);
      else shared++;
    }
    return { shared, scoped: [...c.entries()].sort((a, b) => b[1] - a[1]) };
  }, [records]);

  const f = q.trim().toLowerCase();
  const shown = useMemo(() => {
    // Operator-profile facets (M1000) have their own card — keep them out of the
    // general record list so they aren't shown twice.
    let list = [...records]
      .filter((r) => !(r.subject || "").startsWith(PROFILE_PREFIX))
      .sort((a, b) => (b.created_ms || 0) - (a.created_ms || 0));
    if (scopeFilter !== null) list = list.filter((r) => (r.tags?.scope || "") === scopeFilter);
    if (!f) return list;
    return list.filter((r) =>
      `${r.subject} ${r.content} ${r.type} ${Object.values(r.tags || {}).join(" ")}`.toLowerCase().includes(f),
    );
  }, [records, f, scopeFilter]);

  // The learned operator profile (M1000): PREFERENCE facets on reserved subjects.
  const profile = useMemo(
    () => records.filter((r) => (r.subject || "").startsWith(PROFILE_PREFIX)),
    [records],
  );
  const editingRecord = useMemo(
    () => (editingId ? records.find((r) => r.id === editingId) || null : null),
    [editingId, records],
  );

  // ─────────────── bulk select + cleanup (over loaded/filtered records) ───────────────
  const shownIds = useMemo(() => shown.map((r) => r.id).filter((id): id is string => !!id), [shown]);
  const allSelected = shownIds.length > 0 && shownIds.every((id) => selected.has(id));

  function toggleSelect(id: string) {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }

  // Select-all covers the records currently shown (loaded pages after
  // search/scope filtering); toggling off removes only those.
  function toggleSelectAll() {
    setSelected((prev) => {
      const next = new Set(prev);
      if (allSelected) for (const id of shownIds) next.delete(id);
      else for (const id of shownIds) next.add(id);
      return next;
    });
  }

  // bulkForget soft-deletes every selected record in one call (idempotent
  // server-side; tombstones stay recoverable until prune).
  async function bulkForget() {
    const ids = [...selected];
    if (ids.length === 0) return;
    const ok = await ui.confirm({
      title: `Forget ${ids.length} memory record${ids.length === 1 ? "" : "s"}?`,
      message: "Soft-delete — the records stay recoverable until the next prune.",
      confirmLabel: "Forget",
      danger: true,
    });
    if (!ok) return;
    setBulkBusy(true);
    try {
      const res = await postJSON<{ forgotten?: number; not_found?: number }>("/api/memory/bulk_forget", { ids });
      const n = res.forgotten ?? ids.length;
      ui.toast(`Forgot ${n} record${n === 1 ? "" : "s"}`, "success");
      setSelected(new Set());
      await reload();
    } catch (e) {
      ui.toast(`bulk forget failed: ${(e as Error).message}`, "error");
    } finally {
      setBulkBusy(false);
    }
  }

  return (
    <Page
      icon={Brain}
      title="Memory"
      width="wide"
      actions={
        <>
          {loadedOnce && (
            <span className="text-xs text-muted">
              {shown.length}/{records.length}
              {total != null && total !== records.length ? ` · ${records.length} of ${total} loaded` : ""}
            </span>
          )}
            <div className="relative">
              <Search className="pointer-events-none absolute left-2 top-1/2 size-3.5 -translate-y-1/2 text-muted" />
              <input
                value={q}
                onChange={(e) => setQ(e.target.value)}
                placeholder="search memories..."
                className="h-7 w-48 rounded-md border border-border bg-panel pl-7 pr-2 text-xs outline-none focus:border-accent"
              />
            </div>
            <Button size="sm" onClick={() => setShowForm(true)} title="Teach the agent a fact">
              <Plus className="size-3.5" /> Teach
            </Button>
            <input
              ref={fileRef}
              type="file"
              accept="application/json,.json"
              className="hidden"
              aria-hidden="true"
              onChange={(e) => {
                const f = e.target.files?.[0];
                if (f) void importMemory(f);
                e.target.value = "";
              }}
            />
            <Button variant="ghost" size="sm" onClick={() => fileRef.current?.click()} title="Import memories from a file">
              <Upload className="size-3.5" /> Import
            </Button>
            <Button variant="ghost" size="sm" onClick={exportMemory} disabled={records.length === 0} title="Export memories to a file">
              <Download className="size-3.5" /> Export
            </Button>
            <Button variant="ghost" size="sm" onClick={tidy} title="Collapse near-duplicate auto-distilled notes — keep the strongest per subject">
              <Sparkles className="size-3.5" /> Tidy
            </Button>
            <Button variant="ghost" size="sm" onClick={cleanLowValue} title="Forget active records that look like logs or low-value automatic notes">
              <ShieldCheck className="size-3.5" /> Clean
            </Button>
            <Button variant="ghost" size="sm" onClick={prune} title="Permanently remove forgotten/superseded records older than 30 days">
              <Trash2 className="size-3.5" /> Prune
            </Button>
            <Button variant="ghost" size="sm" onClick={reload} disabled={loading}>
              <RefreshCw className={cn("size-3.5", loading && "animate-spin")} />
            </Button>
          </>
        }
    >
      <MemoryPanel
        icon={UserRound}
        title="Operator Profile"
        status={`${profile.length} facet${profile.length === 1 ? "" : "s"}`}
        tone="accent"
      >
        <div className="flex items-center justify-between gap-2 pb-2">
          <p className="text-xs text-muted">
            What AGEZT has learned about you — injected into every run so it knows who it works for. Synthesized daily from your accumulated memory.
          </p>
          <Button size="sm" variant="ghost" onClick={rebuildProfile} title="Rebuild the operator profile from accumulated memory now">
            <Sparkles className="size-3.5" /> Rebuild
          </Button>
        </div>
        {profile.length === 0 ? (
          <p className="text-xs text-muted">No profile yet — it builds up as you work, or click Rebuild to synthesize one now.</p>
        ) : (
          <ul className="space-y-1.5">
            {profile.map((r) => (
              <li key={r.id} className="text-sm">
                <span className="font-medium capitalize text-accent">{(r.subject || "").slice(PROFILE_PREFIX.length)}</span>
                <span className="text-muted">: {r.content}</span>
              </li>
            ))}
          </ul>
        )}
      </MemoryPanel>

      {audit && (
        <MemoryPanel
          icon={ShieldCheck}
          title="Hygiene"
          status={`${audit.usable ?? 0} usable`}
          tone="good"
        >
          <MetricGrid>
            <MetricWidget
              icon={ShieldCheck}
              label="Usable"
              value={audit.usable ?? 0}
              tone="good"
            />
            <MetricWidget
              icon={Brain}
              label="Expired"
              value={audit.expired ?? 0}
              tone={(audit.expired ?? 0) > 0 ? "bad" : "muted"}
            />
            <MetricWidget
              icon={Brain}
              label="Suspended"
              value={audit.suspended ?? 0}
              tone={(audit.suspended ?? 0) > 0 ? "bad" : "muted"}
            />
            <MetricWidget
              icon={AlertTriangle}
              label="Conflict load"
              value={audit.contradiction_load ?? 0}
              tone={(audit.contradiction_load ?? 0) > 0 ? "warn" : "muted"}
            />
          </MetricGrid>
        </MemoryPanel>
      )}

      {scopes.scoped.length > 0 && (
        <div className="flex flex-wrap items-center gap-1.5" role="group" aria-label="Memory scope filter">
          <ScopeChip active={scopeFilter === null} onClick={() => setScopeFilter(null)} label="All" count={records.length} />
          <ScopeChip
            active={scopeFilter === ""}
            onClick={() => setScopeFilter((cur) => (cur === "" ? null : ""))}
            label="Shared"
            count={scopes.shared}
            icon={<Users className="size-3" />}
          />
          {scopes.scoped.map(([s, n]) => (
            <ScopeChip
              key={s}
              active={scopeFilter === s}
              onClick={() => setScopeFilter((cur) => (cur === s ? null : s))}
              label={s}
              count={n}
              icon={<Lock className="size-3" />}
            />
          ))}
        </div>
      )}

      {err ? (
        <ErrorText>{err}</ErrorText>
      ) : !loadedOnce ? (
        <SkeletonGrid count={6} />
      ) : shown.length === 0 ? (
        records.length === 0 ? (
          <EmptyState
            icon={Brain}
            title="No memories yet"
            hint="As the agent works, it distills durable facts and preferences here — searchable and one-click to forget."
          />
        ) : (
          <Muted>no memories match “{q.trim()}”</Muted>
        )
      ) : (
        <div className="space-y-2">
          {byType.length > 0 && <BreakdownBar segments={byType} />}
          <div className="flex flex-wrap items-center gap-2 rounded-lg border border-border bg-panel/45 px-3 py-1.5">
            <label className="flex cursor-pointer items-center gap-2 text-xs text-muted">
              <input
                type="checkbox"
                className="size-3.5 accent-accent"
                checked={allSelected}
                onChange={toggleSelectAll}
                aria-label="Select all loaded records"
              />
              Select all ({shownIds.length})
            </label>
            {selected.size > 0 && (
              <div className="ml-auto flex items-center gap-2" role="toolbar" aria-label="Bulk memory actions">
                <span className="text-xs font-semibold text-foreground">{selected.size} selected</span>
                <Button size="sm" variant="danger" onClick={bulkForget} disabled={bulkBusy} title="Soft-delete every selected record">
                  <Trash2 className="size-3.5" /> Forget selected
                </Button>
                <Button size="sm" variant="ghost" onClick={() => setSelected(new Set())} disabled={bulkBusy}>
                  <X className="size-3.5" /> Clear selection
                </Button>
              </div>
            )}
          </div>
          <ul className="grid grid-cols-1 gap-2 lg:grid-cols-2">
            {shown.map((r, i) => {
              const scope = r.tags?.scope || "";
              return (
              <li key={r.id || i} className="glass rounded-xl p-3">
                <div className="mb-1 flex items-center gap-2">
                  {r.id && (
                    <input
                      type="checkbox"
                      className="size-3.5 shrink-0 accent-accent"
                      checked={selected.has(r.id)}
                      onChange={() => toggleSelect(r.id!)}
                      aria-label={`Select memory ${r.subject || r.id}`}
                    />
                  )}
                  {r.type && (
                    <span className="rounded bg-accent/15 px-1.5 py-0.5 text-xs font-semibold uppercase tracking-normal text-accent">
                      {r.type}
                    </span>
                  )}
                  {scope && (
                    <span
                      title={`Private to “${scope}” — only that agent recalls this note`}
                      className="flex items-center gap-1 rounded bg-panel px-1.5 py-0.5 text-xs font-semibold text-muted"
                    >
                      <Lock className="size-3" /> {scope}
                    </span>
                  )}
                  <span className="truncate text-sm font-semibold">{r.subject || "—"}</span>
                  <div className="ml-auto flex items-center gap-2">
                    {r.confidence != null && (
                      <span className="text-xs tabular-nums text-muted">conf {Math.round((r.confidence || 0) * 100)}%</span>
                    )}
                    {r.id && scope && (
                      <button
                        onClick={() => promote(r.id!, scope, r.subject)}
                        disabled={busy === r.id}
                        title="Share with every agent (promote to shared memory)"
                        className="text-muted transition-colors hover:text-accent disabled:opacity-50"
                      >
                        <Share2 className="size-3.5" />
                      </button>
                    )}
                    {r.id && (
                      <button
                        onClick={() => setEditingId(r.id!)}
                        title="Revise this memory"
                        className={cn(
                          "transition-colors",
                          editingId === r.id ? "text-accent" : "text-muted hover:text-accent",
                        )}
                      >
                        <Pencil className="size-3.5" />
                      </button>
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
                <div className="mt-2 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted">
                  {r.created_ms ? <span>{fmtTime(r.created_ms)}</span> : null}
                  {r.added_by && (
                    <span title={r.updated_by && r.updated_by !== r.added_by ? `last updated by ${r.updated_by}` : "who added this"}>
                      by <span className="text-foreground/70">{r.added_by}</span>
                      {r.updated_by && r.updated_by !== r.added_by && (
                        <> · upd. <span className="text-foreground/70">{r.updated_by}</span></>
                      )}
                    </span>
                  )}
                  {r.tags &&
                    Object.entries(r.tags)
                      .filter(([k]) => k !== "scope") // scope has its own badge up top
                      .map(([k, v]) => (
                        <span key={k} className="rounded-full bg-panel px-1.5 py-0.5">
                          {k}:{v}
                        </span>
                      ))}
                </div>
              </li>
              );
            })}
          </ul>
          <LoadMoreFooter
            hasMore={hasMore}
            loadingMore={loadingMore}
            moreError={moreError}
            onLoadMore={loadMore}
            pageSize={MEM_PAGE_SIZE}
            label="memory"
          />
        </div>
      )}

      {showForm && (
        <MemoryModal title="Teach memory" onClose={() => setShowForm(false)}>
          <TeachFactForm
            onAdded={(subject) => {
              setShowForm(false);
              ui.toast(subject ? `Learned: “${subject}”` : "Memory added", "success");
              void reload();
            }}
            onError={(m) => ui.toast(m, "error")}
          />
        </MemoryModal>
      )}

      {editingRecord && (
        <MemoryModal title={`Revise ${editingRecord.subject || "memory"}`} onClose={() => setEditingId(null)}>
          <ReviseFactForm
            record={editingRecord}
            onRevised={() => {
              setEditingId(null);
              ui.toast("Memory revised", "success");
              void reload();
            }}
            onError={(m) => ui.toast(m, "error")}
          />
        </MemoryModal>
      )}

      <LogHistoryPanel
        icon={History}
        title="Write history"
        rows={writeLogRows}
        loading={writeLogLoading}
        loadMore={loadMoreWriteLog}
        loadingMore={loadingMoreWriteLog}
        moreError={writeLogError}
        hasMore={hasMoreWriteLog}
        pageSize={50}
        renderRow={(r) => (
          <>
            <Badge variant="default">{String(r.op || "")}</Badge>
            <span className="font-mono text-foreground">{String(r.record_id || "")}</span>
            <span className="shrink-0 text-muted">{String(r.agent || "")}</span>
            <span className="min-w-0 flex-1 truncate text-muted">{String(r.summary || "")}</span>
          </>
        )}
      />
    </Page>
  );
}

function MemoryPanel({
  icon: Icon,
  title,
  status,
  tone,
  children,
}: {
  icon: typeof Brain;
  title: string;
  status: string;
  tone: "accent" | "good" | "warn" | "muted";
  children: ReactNode;
}) {
  const toneCls: Record<typeof tone, string> = {
    accent: "border-accent/35 bg-accent/5 text-accent",
    good: "border-good/35 bg-good/5 text-good",
    warn: "border-warn/35 bg-warn/5 text-warn",
    muted: "border-border bg-panel text-muted",
  };
  return (
    <section className="rounded-xl border border-border bg-card/70 p-3 shadow-e1">
      <div className="mb-2 flex items-center gap-2">
        <span className={cn("grid size-8 shrink-0 place-items-center rounded-lg border", toneCls[tone])}>
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

function MemoryModal({ title, onClose, children }: { title: string; onClose: () => void; children: ReactNode }) {
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
            <Brain className="size-4" />
          </span>
          <h3 className="text-sm font-semibold text-foreground">{title}</h3>
          <button className="ml-auto rounded-md p-1 text-muted transition-colors hover:bg-panel hover:text-foreground" onClick={onClose} aria-label="Close memory modal">
            <X className="size-4" />
          </button>
        </div>
        {children}
      </div>
    </div>
  );
}

// ScopeChip is one filter chip in the per-agent memory map (M915): All, Shared,
// or one agent's private scope. Clicking an active chip clears the filter.
function ScopeChip({
  active,
  onClick,
  label,
  count,
  icon,
}: {
  active: boolean;
  onClick: () => void;
  label: string;
  count: number;
  icon?: ReactNode;
}) {
  return (
    <button
      onClick={onClick}
      aria-pressed={active}
      className={cn(
        "flex items-center gap-1 rounded-full border px-2 py-0.5 text-[11px] transition-colors",
        active
          ? "border-accent bg-accent/15 text-accent"
          : "border-border bg-panel text-muted hover:border-accent/50 hover:text-foreground",
      )}
    >
      {icon}
      {label}
      <span className="tabular-nums opacity-70">{count}</span>
    </button>
  );
}

// MEM_TYPES are the record types an operator would manually teach. The agent also
// writes SUMMARY/RELATION on its own; those aren't useful to hand-author.
const MEM_TYPES = ["FACT", "PREFERENCE", "OBSERVATION"];

function MemoryFormBlock({
  icon: Icon,
  title,
  meta,
  children,
  defaultOpen = false,
}: {
  icon: LucideIcon;
  title: string;
  meta: string;
  children: ReactNode;
  defaultOpen?: boolean;
}) {
  return (
    <Disclosure
      defaultOpen={defaultOpen}
      className="rounded-lg border border-border bg-panel/45"
      summaryClassName="px-2.5 py-2"
      contentClassName="px-2.5 pb-2"
      summary={
        <span className="flex min-w-0 items-center gap-2">
          <span className="grid size-7 shrink-0 place-items-center rounded-md border border-border bg-background/70 text-accent">
            <Icon className="size-3.5" />
          </span>
          <span className="min-w-0 flex-1">
            <span className="block truncate text-sm font-semibold text-foreground">{title}</span>
            <span className="block truncate text-[11px] font-normal text-muted">{meta}</span>
          </span>
        </span>
      }
    >
      {children}
    </Disclosure>
  );
}

function MemoryTypePicker({
  value,
  onChange,
  label,
}: {
  value: string;
  onChange: (value: string) => void;
  label: string;
}) {
  return (
    <div className="flex flex-wrap items-center gap-1.5" role="group" aria-label={label}>
      {MEM_TYPES.map((t) => (
        <button
          key={t}
          type="button"
          aria-pressed={value === t}
          onClick={() => onChange(t)}
          className={cn(
            "inline-flex h-8 items-center gap-1.5 rounded-md border px-2 text-xs font-medium transition-colors",
            value === t
              ? "border-accent bg-accent/15 text-accent"
              : "border-border bg-panel text-muted hover:border-accent/60 hover:text-foreground",
          )}
        >
          {t === "FACT" ? <Brain className="size-3.5" /> : t === "PREFERENCE" ? <UserRound className="size-3.5" /> : <Sparkles className="size-3.5" />}
          {t.toLowerCase()}
        </button>
      ))}
    </div>
  );
}

// TeachFactForm lets the owner teach the agent a durable fact or preference from
// the UI (M718) — the write side of the knowledge browser. It posts to memory_add
// (tagged source=operator) so the fact is recalled into future runs like any other.
export function TeachFactForm({
  onAdded,
  onError,
}: {
  onAdded: (subject: string) => void;
  onError: (msg: string) => void;
}) {
  const [subject, setSubject] = useState("");
  const [content, setContent] = useState("");
  const [type, setType] = useState("FACT");
  const [submitting, setSubmitting] = useState(false);

  const valid = content.trim() !== "";

  async function add() {
    if (!valid) return;
    setSubmitting(true);
    try {
      await postJSON("/api/memory/add", { content: content.trim(), subject: subject.trim(), type });
      onAdded(subject.trim());
    } catch (e) {
      onError((e as Error).message);
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div className="rounded-lg border border-accent/30 bg-card p-3">
      <div className="mb-3 flex flex-wrap items-center justify-between gap-2">
        <div className="flex min-w-0 items-center gap-2">
          <div className="grid size-8 place-items-center rounded-lg border border-accent/25 bg-accent/10 text-accent">
            <Brain className="size-4" />
          </div>
          <div className="min-w-0">
            <div className="truncate text-sm font-semibold text-foreground">{subject.trim() || "New memory"}</div>
            <div className="text-[11px] text-muted">{type.toLowerCase()} record</div>
          </div>
        </div>
        <div className="flex flex-wrap gap-1.5">
          <Badge variant={valid ? "good" : "warn"}>{valid ? "content ready" : "needs content"}</Badge>
          <Badge variant={type === "PREFERENCE" ? "accent" : "default"}>{type.toLowerCase()}</Badge>
        </div>
      </div>

      <div className="space-y-2">
        <MemoryFormBlock
          icon={FileText}
          title="Fact"
          meta={content.trim() ? `${content.trim().slice(0, 48)}${content.trim().length > 48 ? "..." : ""}` : "what should be recalled"}
          defaultOpen
        >
          <textarea
            value={content}
            onChange={(e) => setContent(e.target.value)}
            placeholder="The fact to remember"
            aria-label="Memory content"
            className="h-20 w-full resize-y rounded-md border border-border bg-panel p-2 text-sm outline-none placeholder:text-muted/60 focus-visible:border-accent"
          />
        </MemoryFormBlock>

        <MemoryFormBlock
          icon={Tags}
          title="Classification"
          meta={subject.trim() || "shared memory"}
          defaultOpen
        >
          <div className="flex flex-wrap items-center gap-2">
            <input
              value={subject}
              onChange={(e) => setSubject(e.target.value)}
              placeholder="Subject (optional, e.g. Owner's timezone)"
              aria-label="Memory subject"
              className="h-8 min-w-0 flex-1 rounded-md border border-border bg-panel px-2 text-sm outline-none focus-visible:border-accent"
            />
            <MemoryTypePicker value={type} onChange={setType} label="Memory type" />
          </div>
        </MemoryFormBlock>
      </div>
      <div className="mt-2 flex items-center justify-end">
        <Button size="sm" onClick={add} disabled={!valid || submitting}>
          {submitting ? <RefreshCw className="size-3.5 animate-spin" /> : <Plus className="size-3.5" />} Remember it
        </Button>
      </div>
    </div>
  );
}

// ReviseFactForm revises a stored fact (M731). Memory is content-addressed and
// immutable, so "editing" means SUPERSEDING: a new record is written and the old
// one's superseded_by points to it (history retained; recall uses the new one). The
// form prefills the current subject/type/content; Save posts memory_supersede with
// the old id. Revising to identical content is a backend no-op.
export function ReviseFactForm({
  record,
  onRevised,
  onError,
}: {
  record: MemRecord;
  onRevised: () => void;
  onError: (msg: string) => void;
}) {
  const [subject, setSubject] = useState(record.subject ?? "");
  const [content, setContent] = useState(record.content ?? "");
  const [type, setType] = useState((record.type || "FACT").toUpperCase());
  const [submitting, setSubmitting] = useState(false);

  const valid = content.trim() !== "";

  async function save() {
    if (!valid || !record.id) return;
    setSubmitting(true);
    try {
      const args: Record<string, unknown> = {
        old_id: record.id,
        content: content.trim(),
        subject: subject.trim(),
        type,
      };
      if (record.confidence != null) args.confidence = record.confidence;
      await postJSON("/api/memory/supersede", args);
      onRevised();
    } catch (e) {
      onError((e as Error).message);
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div className="rounded-lg border border-accent/30 bg-card p-3">
      <div className="mb-3 flex flex-wrap items-center justify-between gap-2">
        <div className="flex min-w-0 items-center gap-2">
          <div className="grid size-8 place-items-center rounded-lg border border-accent/25 bg-accent/10 text-accent">
            <Pencil className="size-4" />
          </div>
          <div className="min-w-0">
            <div className="truncate text-sm font-semibold text-foreground">{subject.trim() || record.id || "Memory revision"}</div>
            <div className="text-[11px] text-muted">supersedes current record</div>
          </div>
        </div>
        <div className="flex flex-wrap gap-1.5">
          <Badge variant={valid ? "good" : "warn"}>{valid ? "revision ready" : "needs content"}</Badge>
          {record.confidence != null ? <Badge variant="accent">{Math.round(record.confidence * 100)}%</Badge> : null}
        </div>
      </div>

      <div className="space-y-2">
        <MemoryFormBlock
          icon={FileText}
          title="Replacement"
          meta={content.trim() ? `${content.trim().slice(0, 48)}${content.trim().length > 48 ? "..." : ""}` : "new content required"}
          defaultOpen
        >
          <textarea
            value={content}
            onChange={(e) => setContent(e.target.value)}
            aria-label="Revise memory content"
            className="h-20 w-full resize-y rounded-md border border-border bg-panel p-2 text-sm outline-none placeholder:text-muted/60 focus-visible:border-accent"
          />
        </MemoryFormBlock>

        <MemoryFormBlock
          icon={Tags}
          title="Classification"
          meta={subject.trim() || "shared memory"}
          defaultOpen
        >
          <div className="flex flex-wrap items-center gap-2">
            <input
              value={subject}
              onChange={(e) => setSubject(e.target.value)}
              placeholder="Subject (optional)"
              aria-label="Revise memory subject"
              className="h-8 min-w-0 flex-1 rounded-md border border-border bg-panel px-2 text-sm outline-none focus-visible:border-accent"
            />
            <MemoryTypePicker value={type} onChange={setType} label="Revise memory type" />
          </div>
        </MemoryFormBlock>

        <MemoryFormBlock icon={History} title="History" meta="old record remains auditable">
          <div className="text-xs text-muted">A revision writes a replacement record and keeps the previous version for audit.</div>
        </MemoryFormBlock>
      </div>
      <div className="mt-2 flex items-center justify-end">
        <Button size="sm" onClick={save} disabled={!valid || submitting}>
          {submitting ? <RefreshCw className="size-3.5 animate-spin" /> : <Save className="size-3.5" />} Save revision
        </Button>
      </div>
    </div>
  );
}
