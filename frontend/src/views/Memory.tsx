import { useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { Brain, RefreshCw, Search, Trash2, Plus, X, Pencil, Save, Download, Upload, Lock, Share2, Users, Sparkles } from "lucide-react";
import { getJSON, postAction, postJSON } from "@/lib/api";
import { downloadText } from "@/lib/export";
import { cn, fmtTime } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { useUI } from "@/components/ui/feedback";
import { SkeletonGrid } from "@/components/ui/skeleton";
import { EmptyState } from "@/components/ui/empty";
import { Muted, ErrorText } from "@/components/JsonView";
import { BreakdownBar } from "@/components/Widgets";
import { PageHeader } from "@/components/ui/page-header";

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
export function Memory() {
  const ui = useUI();
  const [records, setRecords] = useState<MemRecord[] | null>(null);
  const [q, setQ] = useState("");
  const [err, setErr] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [busy, setBusy] = useState<string | null>(null);
  const [showForm, setShowForm] = useState(false);
  const [editingId, setEditingId] = useState<string | null>(null);
  // Scope filter (M915): null = all, "" = shared brain, "<scope>" = that
  // agent's private notes. Each agent keeps its own memory; this lets the
  // owner browse them side by side.
  const [scopeFilter, setScopeFilter] = useState<string | null>(null);
  const fileRef = useRef<HTMLInputElement>(null);

  function exportMemory() {
    downloadText("agezt-memory.json", JSON.stringify({ version: 1, memory: records ?? [] }, null, 2), "application/json");
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

  // prune hard-removes soft-deleted (forgotten/superseded) records older than the
  // cutoff — the "no memory-bomb" sweep (M857). Dry-run first, then confirm.
  const PRUNE_DAYS = 30;
  async function prune() {
    try {
      const dry = await postJSON<{ prunable?: number }>("/api/memory/prune", { older_than_days: String(PRUNE_DAYS), dry_run: "true" });
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
      const res = await postJSON<{ pruned?: number }>("/api/memory/prune", { older_than_days: String(PRUNE_DAYS), dry_run: "false" });
      ui.toast(`Pruned ${res.pruned ?? 0} record(s)`, "success");
      await reload();
    } catch (e) {
      ui.toast(`prune failed: ${(e as Error).message}`, "error");
    }
  }

  // tidy collapses the near-duplicate auto-distilled notes that built up before
  // the write-time subject gate (M994) — the "1000+ saçma girdi" cleanup. Keeps
  // the strongest note per subject, forgets the rest (reversible). Dry-run first.
  async function tidy() {
    try {
      const dry = await postJSON<{ collapsed?: number }>("/api/memory/tidy", { dry_run: "true" });
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
      const res = await postJSON<{ collapsed?: number }>("/api/memory/tidy", { dry_run: "false" });
      ui.toast(`Collapsed ${res.collapsed ?? 0} duplicate note(s)`, "success");
      await reload();
    } catch (e) {
      ui.toast(`tidy failed: ${(e as Error).message}`, "error");
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

  // byType drives the breakdown bar (over ALL records, not the filtered view).
  const byType = useMemo(() => {
    const c: Record<string, number> = {};
    for (const r of records || []) c[(r.type || "FACT").toUpperCase()] = (c[(r.type || "FACT").toUpperCase()] || 0) + 1;
    return Object.entries(c).map(([label, count]) => ({ label, count }));
  }, [records]);

  // Per-agent memory map (M915): shared count + each private scope's count,
  // driving the filter chips.
  const scopes = useMemo(() => {
    const c = new Map<string, number>();
    let shared = 0;
    for (const r of records || []) {
      const s = r.tags?.scope || "";
      if (s) c.set(s, (c.get(s) || 0) + 1);
      else shared++;
    }
    return { shared, scoped: [...c.entries()].sort((a, b) => b[1] - a[1]) };
  }, [records]);

  const f = q.trim().toLowerCase();
  const shown = useMemo(() => {
    let list = [...(records || [])].sort((a, b) => (b.created_ms || 0) - (a.created_ms || 0));
    if (scopeFilter !== null) list = list.filter((r) => (r.tags?.scope || "") === scopeFilter);
    if (!f) return list;
    return list.filter((r) =>
      `${r.subject} ${r.content} ${r.type} ${Object.values(r.tags || {}).join(" ")}`.toLowerCase().includes(f),
    );
  }, [records, f, scopeFilter]);

  return (
    <div className="flex h-full min-h-0 flex-col gap-3">
      <PageHeader
        icon={Brain}
        title="Memory"
        description="Every durable fact the agent has stored — searchable by subject, content or tag."
        actions={
          <>
            {records && <span className="text-xs text-muted">{shown.length}/{records.length}</span>}
            <div className="relative">
              <Search className="pointer-events-none absolute left-2 top-1/2 size-3.5 -translate-y-1/2 text-muted" />
              <input
                value={q}
                onChange={(e) => setQ(e.target.value)}
                placeholder="search memories…"
                className="h-7 w-48 rounded-md border border-border bg-panel pl-7 pr-2 text-xs outline-none focus:border-accent"
              />
            </div>
            <Button size="sm" onClick={() => setShowForm((v) => !v)} title="Teach the agent a fact">
              {showForm ? <X className="size-3.5" /> : <Plus className="size-3.5" />} Teach
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
            <Button variant="ghost" size="sm" onClick={exportMemory} disabled={!records || records.length === 0} title="Export memories to a file">
              <Download className="size-3.5" /> Export
            </Button>
            <Button variant="ghost" size="sm" onClick={tidy} title="Collapse near-duplicate auto-distilled notes — keep the strongest per subject">
              <Sparkles className="size-3.5" /> Tidy
            </Button>
            <Button variant="ghost" size="sm" onClick={prune} title="Permanently remove forgotten/superseded records older than 30 days">
              <Trash2 className="size-3.5" /> Prune
            </Button>
            <Button variant="ghost" size="sm" onClick={reload} disabled={loading}>
              <RefreshCw className={cn("size-3.5", loading && "animate-spin")} />
            </Button>
          </>
        }
      />

      {scopes.scoped.length > 0 && (
        <div className="flex flex-wrap items-center gap-1.5" role="group" aria-label="Memory scope filter">
          <ScopeChip active={scopeFilter === null} onClick={() => setScopeFilter(null)} label="All" count={(records || []).length} />
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

      {showForm && (
        <TeachFactForm
          onAdded={(subject) => {
            setShowForm(false);
            ui.toast(subject ? `Learned: “${subject}”` : "Memory added", "success");
            void reload();
          }}
          onError={(m) => ui.toast(m, "error")}
        />
      )}

      {err ? (
        <ErrorText>{err}</ErrorText>
      ) : !records ? (
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
        <div className="min-h-0 flex-1 space-y-2 overflow-auto">
          {byType.length > 0 && <BreakdownBar segments={byType} />}
          <ul className="grid grid-cols-1 gap-2 lg:grid-cols-2">
            {shown.map((r, i) => {
              const scope = r.tags?.scope || "";
              return (
              <li key={r.id || i} className="glass rounded-xl p-3">
                <div className="mb-1 flex items-center gap-2">
                  {r.type && (
                    <span className="rounded bg-accent/15 px-1.5 py-0.5 text-[10px] font-semibold uppercase tracking-wider text-accent">
                      {r.type}
                    </span>
                  )}
                  {scope && (
                    <span
                      title={`Private to “${scope}” — only that agent recalls this note`}
                      className="flex items-center gap-1 rounded bg-panel px-1.5 py-0.5 text-[10px] font-semibold text-muted"
                    >
                      <Lock className="size-3" /> {scope}
                    </span>
                  )}
                  <span className="truncate text-sm font-semibold">{r.subject || "—"}</span>
                  <div className="ml-auto flex items-center gap-2">
                    {r.confidence != null && (
                      <span className="text-[10px] tabular-nums text-muted">conf {Math.round((r.confidence || 0) * 100)}%</span>
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
                        onClick={() => setEditingId((cur) => (cur === r.id ? null : r.id!))}
                        title={editingId === r.id ? "Close editor" : "Revise this memory"}
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
                <div className="mt-2 flex flex-wrap items-center gap-x-3 gap-y-1 text-[10px] text-muted">
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
                {editingId === r.id && r.id && (
                  <div className="mt-2">
                    <ReviseFactForm
                      record={r}
                      onRevised={() => {
                        setEditingId(null);
                        ui.toast("Memory revised", "success");
                        void reload();
                      }}
                      onError={(m) => ui.toast(m, "error")}
                    />
                  </div>
                )}
              </li>
              );
            })}
          </ul>
        </div>
      )}
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
      <div className="flex flex-wrap items-center gap-2">
        <input
          value={subject}
          onChange={(e) => setSubject(e.target.value)}
          placeholder="Subject (optional, e.g. Owner's timezone)"
          aria-label="Memory subject"
          className="h-8 min-w-0 flex-1 rounded-md border border-border bg-panel px-2 text-sm outline-none focus-visible:border-accent"
        />
        <select
          value={type}
          onChange={(e) => setType(e.target.value)}
          aria-label="Memory type"
          className="h-8 rounded-md border border-border bg-panel px-1.5 text-sm outline-none focus-visible:border-accent"
        >
          {MEM_TYPES.map((t) => (
            <option key={t} value={t}>
              {t.toLowerCase()}
            </option>
          ))}
        </select>
      </div>
      <textarea
        value={content}
        onChange={(e) => setContent(e.target.value)}
        placeholder="The fact to remember (e.g. The owner is in Istanbul, UTC+3, and prefers terse replies)…"
        aria-label="Memory content"
        className="mt-2 h-20 w-full resize-y rounded-md border border-border bg-panel p-2 text-sm outline-none placeholder:text-muted/60 focus-visible:border-accent"
      />
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
      <div className="flex flex-wrap items-center gap-2">
        <input
          value={subject}
          onChange={(e) => setSubject(e.target.value)}
          placeholder="Subject (optional)"
          aria-label="Revise memory subject"
          className="h-8 min-w-0 flex-1 rounded-md border border-border bg-panel px-2 text-sm outline-none focus-visible:border-accent"
        />
        <select
          value={type}
          onChange={(e) => setType(e.target.value)}
          aria-label="Revise memory type"
          className="h-8 rounded-md border border-border bg-panel px-1.5 text-sm outline-none focus-visible:border-accent"
        >
          {MEM_TYPES.map((t) => (
            <option key={t} value={t}>
              {t.toLowerCase()}
            </option>
          ))}
        </select>
      </div>
      <textarea
        value={content}
        onChange={(e) => setContent(e.target.value)}
        aria-label="Revise memory content"
        className="mt-2 h-20 w-full resize-y rounded-md border border-border bg-panel p-2 text-sm outline-none placeholder:text-muted/60 focus-visible:border-accent"
      />
      <p className="mt-1 text-[10px] text-muted">
        Revising keeps history: a new record supersedes the old one, which is retained for audit.
      </p>
      <div className="mt-2 flex items-center justify-end">
        <Button size="sm" onClick={save} disabled={!valid || submitting}>
          {submitting ? <RefreshCw className="size-3.5 animate-spin" /> : <Save className="size-3.5" />} Save revision
        </Button>
      </div>
    </div>
  );
}
