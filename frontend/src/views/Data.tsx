import { useEffect, useMemo, useState, type ReactNode } from "react";
import {
  Database,
  RefreshCw,
  Plus,
  Trash2,
  Pencil,
  Search,
  X,
  Lock,
  Wallet,
  CheckCircle2,
  Circle,
  CalendarDays,
  Flame,
  StickyNote,
  Bookmark,
  User,
  Mail,
  Phone,
  ExternalLink,
  Save,
} from "lucide-react";
import { getJSON, postJSON, postAction } from "@/lib/api";
import { cn, fmtTime } from "@/lib/utils";
import { safeHref } from "@/lib/markdown";
import { Button } from "@/components/ui/button";
import { PageHeader } from "@/components/ui/page-header";
import { Badge } from "@/components/ui/badge";
import { TabNav } from "@/components/ui/tab-nav";
import { MetricWidget, MetricGrid } from "@/components/ui/metric-widget";
import { SkeletonList } from "@/components/ui/skeleton";
import { EmptyState } from "@/components/ui/empty";
import { ErrorText } from "@/components/JsonView";
import { useUI } from "@/components/ui/feedback";

// Data view (M836): the human window onto the Personal Data Lake (kernel/datalake,
// M834/M835) — the structured collections agents build and fill via the `db` tool.
// Pick a collection on the left, browse/search its records on the right, and add /
// edit / delete rows by hand. Reads /api/data/collections + /api/data/records;
// mutates via /api/data/{insert,update,delete}. A generic table for now; bespoke
// per-collection app views (expense/calendar/…) layer on top in a later milestone,
// keyed off each schema's `view`.

interface FieldDef {
  name: string;
  type?: string;
  label?: string;
}
interface Collection {
  name: string;
  title?: string;
  icon?: string;
  view?: string;
  desc?: string;
  fields?: FieldDef[];
  builtin?: boolean;
  system?: boolean;
  count?: number;
}
interface DataRecord {
  id: string;
  fields: Record<string, unknown>;
  created_ms?: number;
  updated_ms?: number;
  created_by?: string;
  updated_by?: string;
}

function fmtCell(v: unknown): string {
  if (v === null || v === undefined) return "";
  if (typeof v === "boolean") return v ? "✓" : "✗";
  if (Array.isArray(v)) return v.join(", ");
  if (typeof v === "object") return JSON.stringify(v);
  return String(v);
}

export function dataRecordWriter(r: Pick<DataRecord, "created_by" | "updated_by">): string {
  return r.updated_by || r.created_by || "unknown";
}

export function dataLakeActorAgent(actor?: string): string {
  const raw = (actor || "").trim();
  if (!raw) return "";
  const idx = raw.indexOf(":");
  return idx > 0 ? raw.slice(0, idx) : raw;
}

export function dataRecordAttribution(r: Pick<DataRecord, "created_by" | "updated_by" | "created_ms" | "updated_ms">): string {
  const created = r.created_by ? `created by ${r.created_by}${r.created_ms ? ` ${fmtTime(r.created_ms)}` : ""}` : "creator unknown";
  const updated = r.updated_by ? `updated by ${r.updated_by}${r.updated_ms ? ` ${fmtTime(r.updated_ms)}` : ""}` : "";
  return [created, updated].filter(Boolean).join(" · ");
}

export function dataLakeAgents(records: Pick<DataRecord, "created_by" | "updated_by">[]): string[] {
  return Array.from(
    new Set(records.flatMap((r) => [dataLakeActorAgent(r.created_by), dataLakeActorAgent(r.updated_by)]).filter((x) => x.trim() !== "")),
  ).sort((a, b) => a.localeCompare(b));
}

export function filterDataRecordsByAgent(records: DataRecord[], agent: string): DataRecord[] {
  const a = agent.trim();
  if (!a) return records;
  return records.filter((r) => dataLakeActorAgent(r.created_by) === a || dataLakeActorAgent(r.updated_by) === a);
}

export function Data() {
  const ui = useUI();
  const [cols, setCols] = useState<Collection[]>([]);
  const [active, setActive] = useState<string | null>(null);
  const [records, setRecords] = useState<DataRecord[]>([]);
  const [search, setSearch] = useState("");
  const [agentFilter, setAgentFilter] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [loadingCols, setLoadingCols] = useState(false);
  const [loadingRecs, setLoadingRecs] = useState(false);
  const [editing, setEditing] = useState<DataRecord | "new" | null>(null);

  const activeCol = useMemo(() => cols.find((c) => c.name === active) ?? null, [cols, active]);
  const agentOptions = useMemo(() => dataLakeAgents(records), [records]);
  const visibleRecords = useMemo(() => filterDataRecordsByAgent(records, agentFilter), [records, agentFilter]);

  async function loadCollections() {
    setLoadingCols(true);
    try {
      const d = await getJSON<{ collections: Collection[] }>("/api/data/collections");
      const list = d.collections ?? [];
      setCols(list);
      setError(null);
      setActive((cur) => cur ?? (list[0]?.name ?? null));
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setLoadingCols(false);
    }
  }

  async function loadRecords(name: string, q: string) {
    setLoadingRecs(true);
    try {
      const params: Record<string, string> = { collection: name, limit: "500" };
      if (q.trim()) params.search = q.trim();
      const d = await getJSON<{ records: DataRecord[] }>("/api/data/records", params);
      setRecords(d.records ?? []);
      setError(null);
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setLoadingRecs(false);
    }
  }

  useEffect(() => {
    loadCollections();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    if (active) loadRecords(active, search);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [active]);

  // Debounced search.
  useEffect(() => {
    if (!active) return;
    const t = setTimeout(() => loadRecords(active, search), 250);
    return () => clearTimeout(t);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [search]);

  async function delRecord(r: DataRecord) {
    if (!active) return;
    const ok = await ui.confirm({ title: "Delete record?", message: r.id, confirmLabel: "Delete", danger: true });
    if (!ok) return;
    try {
      await postAction("/api/data/delete", { collection: active, id: r.id });
      ui.toast("record deleted", "success");
      loadRecords(active, search);
      loadCollections();
    } catch (e) {
      ui.toast((e as Error).message, "error");
    }
  }

  async function saveRecord(fields: Record<string, unknown>, id?: string) {
    if (!active) return;
    try {
      if (id) await postJSON("/api/data/update", { collection: active, id, record: fields });
      else await postJSON("/api/data/insert", { collection: active, record: fields });
      ui.toast(id ? "record updated" : "record added", "success");
      setEditing(null);
      loadRecords(active, search);
      loadCollections();
    } catch (e) {
      ui.toast((e as Error).message, "error");
    }
  }

  const fieldDefs: FieldDef[] = activeCol?.fields?.length
    ? activeCol.fields
    : // No schema fields → infer columns from the union of record keys.
      Array.from(new Set(visibleRecords.flatMap((r) => Object.keys(r.fields ?? {})))).map((name) => ({ name }));

  return (
    <div className="flex h-full min-h-0 flex-col gap-3">
      <PageHeader
        icon={Database}
        title="Data Lake"
        actions={
          <Button variant="ghost" size="sm" onClick={loadCollections} disabled={loadingCols} title="Reload">
            <RefreshCw className={cn("size-3.5", loadingCols && "animate-spin")} />
          </Button>
        }
      />
      <div className="flex min-h-0 flex-1 gap-3">
      {/* Collection sidebar */}
      <div className="flex w-56 shrink-0 flex-col gap-2 overflow-y-auto">
        {loadingCols && cols.length === 0 ? (
          <SkeletonList count={6} lines={1} />
        ) : (
          <ul className="space-y-1">
            {cols.map((c) => (
              <li key={c.name}>
                <button
                  onClick={() => setActive(c.name)}
                  className={cn(
                    "flex w-full items-center gap-2 rounded-lg border px-2.5 py-1.5 text-left text-sm transition-colors",
                    active === c.name
                      ? "border-accent bg-accent/10 text-accent"
                      : "border-border bg-card text-foreground/90 hover:border-accent/50",
                  )}
                >
                  <Database className="size-3.5 shrink-0 opacity-70" />
                  <span className="min-w-0 flex-1 truncate">{c.title || c.name}</span>
                  {c.builtin && <Lock className="size-3 opacity-40" />}
                  <span className="text-[11px] text-muted">{c.count ?? 0}</span>
                </button>
              </li>
            ))}
          </ul>
        )}
      </div>

      {/* Records panel */}
      <div className="flex min-w-0 flex-1 flex-col gap-2">
        {error && <ErrorText>{error}</ErrorText>}
        {activeCol ? (
          <>
            <div className="flex items-center gap-2">
              <div className="min-w-0">
                <div className="flex items-center gap-2 text-sm font-semibold">
                  {activeCol.title || activeCol.name}
                  {activeCol.view && activeCol.view !== "table" && <Badge variant="default">{activeCol.view}</Badge>}
                </div>
                {activeCol.desc && <div className="truncate text-[11px] text-muted">{activeCol.desc}</div>}
              </div>
              <div className="relative ml-auto">
                <Search className="pointer-events-none absolute left-2 top-1/2 size-3.5 -translate-y-1/2 text-muted" />
                <input
                  value={search}
                  onChange={(e) => setSearch(e.target.value)}
                  placeholder="search…"
                  className="w-44 rounded-md border border-border bg-panel py-1 pl-7 pr-2 text-xs outline-none focus-visible:border-accent"
                />
              </div>
              {agentOptions.length > 0 && (
                <div className="flex max-w-[22rem] flex-wrap gap-1.5" role="group" aria-label="Filter records by agent" title="Filter by the agent/user that created or last updated a record">
                  {["", ...agentOptions].map((agent) => {
                    const selected = agentFilter === agent;
                    return (
                      <button
                        key={agent || "all"}
                        type="button"
                        aria-pressed={selected}
                        onClick={() => setAgentFilter(agent)}
                        className={cn(
                          "inline-flex h-7 items-center rounded-md border px-2 text-xs font-medium transition-colors",
                          selected
                            ? "border-accent bg-accent/15 text-accent"
                            : "border-border bg-panel text-muted hover:border-accent/60 hover:text-foreground",
                        )}
                      >
                        {agent || "All writers"}
                      </button>
                    );
                  })}
                </div>
              )}
              <Button size="sm" onClick={() => setEditing("new")}>
                <Plus className="size-3.5" /> Add
              </Button>
            </div>

            {loadingRecs && records.length === 0 ? (
              <SkeletonList count={6} lines={1} />
            ) : records.length === 0 ? (
              <EmptyState
                icon={Database}
                title="No records yet"
                hint={`Add one by hand, or ask an agent to fill "${activeCol.name}" with the db tool.`}
              />
            ) : visibleRecords.length === 0 ? (
              <EmptyState
                icon={Database}
                title="No records for this writer"
                hint="Clear the writer filter or pick another agent."
              />
            ) : activeCol.view === "expense" ? (
              <ExpenseView records={visibleRecords} onEdit={(r) => setEditing(r)} onDelete={delRecord} />
            ) : activeCol.view === "tasks" ? (
              <TasksView records={visibleRecords} onEdit={(r) => setEditing(r)} onDelete={delRecord} onToggle={(r) => saveRecord({ done: !truthy(r.fields?.done) }, r.id)} />
            ) : activeCol.view === "calendar" ? (
              <CalendarView records={visibleRecords} onEdit={(r) => setEditing(r)} onDelete={delRecord} />
            ) : activeCol.view === "habits" ? (
              <HabitsView records={visibleRecords} onEdit={(r) => setEditing(r)} onDelete={delRecord} />
            ) : activeCol.view === "notes" ? (
              <NotesView records={visibleRecords} onEdit={(r) => setEditing(r)} onDelete={delRecord} />
            ) : activeCol.view === "bookmarks" ? (
              <BookmarksView records={visibleRecords} onEdit={(r) => setEditing(r)} onDelete={delRecord} />
            ) : activeCol.view === "contacts" ? (
              <ContactsView records={visibleRecords} onEdit={(r) => setEditing(r)} onDelete={delRecord} />
            ) : (
              <div className="min-h-0 flex-1 overflow-auto glass rounded-xl">
                <table className="w-full border-collapse text-sm">
                  <thead className="sticky top-0 bg-panel text-left text-[11px] uppercase tracking-normal text-muted">
                    <tr>
                      {fieldDefs.map((f) => (
                        <th key={f.name} className="border-b border-border px-3 py-2 font-semibold">
                          {f.label || f.name}
                        </th>
                      ))}
                      <th className="border-b border-border px-3 py-2 text-right font-semibold">·</th>
                    </tr>
                  </thead>
                  <tbody>
                    {visibleRecords.map((r) => (
                      <tr key={r.id} className="group hover:bg-panel/50">
                        {fieldDefs.map((f) => (
                          <td
                            key={f.name}
                            className="max-w-[22ch] truncate border-b border-border/60 px-3 py-1.5"
                            title={fmtCell(r.fields?.[f.name])}
                          >
                            {fmtCell(r.fields?.[f.name])}
                          </td>
                        ))}
                        <td className="whitespace-nowrap border-b border-border/60 px-2 py-1.5 text-right">
                          <div className="flex items-center justify-end gap-1">
                            <span
                              className="hidden rounded bg-panel px-1.5 py-0.5 font-mono text-xs text-muted sm:inline-flex"
                              title={dataRecordAttribution(r)}
                            >
                              {dataRecordWriter(r)}
                            </span>
                            <button
                              onClick={() => setEditing(r)}
                              className="rounded p-1 text-muted transition-colors hover:bg-accent/10 hover:text-accent"
                              title={`edit · added by ${r.created_by || "?"} ${fmtTime(r.created_ms)}${r.updated_by ? ` · updated by ${r.updated_by}` : ""}`}
                              aria-label="edit record"
                            >
                              <Pencil className="size-3.5" />
                            </button>
                            <button
                              onClick={() => delRecord(r)}
                              className="rounded p-1 text-muted transition-colors hover:bg-bad/10 hover:text-bad"
                              title="delete record"
                              aria-label="delete record"
                            >
                              <Trash2 className="size-3.5" />
                            </button>
                          </div>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </>
        ) : (
          !loadingCols && <EmptyState icon={Database} title="No collections" hint="Built-in collections seed at startup; agents can create more with the db tool." />
        )}
      </div>
      </div>

      {editing && activeCol && (
        <RecordEditor
          collection={activeCol}
          record={editing === "new" ? null : editing}
          onClose={() => setEditing(null)}
          onSave={saveRecord}
        />
      )}
    </div>
  );
}

// truthy coerces a stored field (bool, "true"/"1"/"yes", number) to a boolean —
// the data lake keeps whatever the agent/user wrote, so a "done" flag may arrive
// in several shapes.
function truthy(v: unknown): boolean {
  if (typeof v === "boolean") return v;
  if (typeof v === "number") return v !== 0;
  if (typeof v === "string") return ["true", "1", "yes", "✓", "done"].includes(v.toLowerCase());
  return false;
}

function num(v: unknown): number {
  const n = typeof v === "number" ? v : Number(String(v ?? "").replace(/[^0-9.-]/g, ""));
  return Number.isFinite(n) ? n : 0;
}

function fmtMoney(n: number): string {
  return n.toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 2 });
}

interface BespokeProps {
  records: DataRecord[];
  onEdit: (r: DataRecord) => void;
  onDelete: (r: DataRecord) => void;
}

// ExpenseView is the app-like layout for the "expense" collection (M856): summary
// cards (total / this month / count), a by-category breakdown, and the recent
// expenses list — instead of a raw table.
function ExpenseView({ records, onEdit, onDelete }: BespokeProps) {
  const total = records.reduce((s, r) => s + num(r.fields?.amount), 0);
  const ym = new Date().toISOString().slice(0, 7); // YYYY-MM (local-ish; dates are stored as YYYY-MM-DD)
  const thisMonth = records
    .filter((r) => String(r.fields?.date ?? "").startsWith(ym))
    .reduce((s, r) => s + num(r.fields?.amount), 0);

  const byCat = new Map<string, number>();
  for (const r of records) {
    const cat = String(r.fields?.category ?? "uncategorized") || "uncategorized";
    byCat.set(cat, (byCat.get(cat) ?? 0) + num(r.fields?.amount));
  }
  const cats = [...byCat.entries()].sort((a, b) => b[1] - a[1]);
  const maxCat = cats.length ? cats[0][1] : 0;

  const recent = [...records].sort((a, b) => String(b.fields?.date ?? "").localeCompare(String(a.fields?.date ?? "")));

  return (
    <div className="min-h-0 flex-1 space-y-3 overflow-auto">
      <MetricGrid>
        <MetricWidget icon={Wallet} label="Total" value={fmtMoney(total)} tone="muted" />
        <MetricWidget icon={CalendarDays} label="This month" value={fmtMoney(thisMonth)} tone="muted" />
        <MetricWidget icon={Database} label="Records" value={records.length} tone="muted" />
      </MetricGrid>

      {cats.length > 0 && (
        <div className="glass rounded-xl p-3">
          <ul className="space-y-1.5">
            {cats.slice(0, 8).map(([cat, amt]) => (
              <li key={cat} className="flex items-center gap-2 text-xs">
                <span className="w-28 shrink-0 truncate text-foreground/85">{cat}</span>
                <div className="h-2 flex-1 overflow-hidden rounded-full bg-panel">
                  <div className="h-full rounded-full bg-accent/70" style={{ width: `${maxCat ? (amt / maxCat) * 100 : 0}%` }} />
                </div>
                <span className="w-20 shrink-0 text-right font-mono text-foreground/80">{fmtMoney(amt)}</span>
              </li>
            ))}
          </ul>
        </div>
      )}

      <ul className="space-y-1">
        {recent.map((r) => (
          <li key={r.id} className="group flex items-center gap-3 glass rounded-xl px-3 py-2 text-sm">
            <span className="w-24 shrink-0 font-mono text-[11px] text-muted">{String(r.fields?.date ?? "")}</span>
            <span className="min-w-0 flex-1 truncate">{String(r.fields?.item ?? "—")}</span>
            {r.fields?.category != null && String(r.fields.category) !== "" && (
              <Badge variant="default">{String(r.fields.category)}</Badge>
            )}
            <span className="w-24 shrink-0 text-right font-mono font-semibold">{fmtMoney(num(r.fields?.amount))}</span>
            <RowActions r={r} onEdit={onEdit} onDelete={onDelete} />
          </li>
        ))}
      </ul>
    </div>
  );
}

// TasksView is the app-like checklist for the "tasks" collection (M856): toggle a
// task done by clicking its check, pending above done.
function TasksView({ records, onEdit, onDelete, onToggle }: BespokeProps & { onToggle: (r: DataRecord) => void }) {
  const pending = records.filter((r) => !truthy(r.fields?.done));
  const done = records.filter((r) => truthy(r.fields?.done));

  const Row = (r: DataRecord) => {
    const isDone = truthy(r.fields?.done);
    return (
      <li key={r.id} className="group flex items-center gap-2.5 glass rounded-xl px-3 py-2 text-sm">
        <button onClick={() => onToggle(r)} title={isDone ? "mark not done" : "mark done"} className="shrink-0 text-muted hover:text-accent">
          {isDone ? <CheckCircle2 className="size-4 text-good" /> : <Circle className="size-4" />}
        </button>
        <span className={cn("min-w-0 flex-1 truncate", isDone && "text-muted line-through")}>{String(r.fields?.title ?? "—")}</span>
        {r.fields?.priority != null && String(r.fields.priority) !== "" && <Badge variant="default">{String(r.fields.priority)}</Badge>}
        {r.fields?.due != null && String(r.fields.due) !== "" && <span className="shrink-0 font-mono text-[11px] text-muted">{String(r.fields.due)}</span>}
        <RowActions r={r} onEdit={onEdit} onDelete={onDelete} />
      </li>
    );
  };

  return (
    <div className="min-h-0 flex-1 space-y-3 overflow-auto">
      <div>
        <div className="mb-1 text-[11px] font-semibold uppercase tracking-normal text-muted">To do · {pending.length}</div>
        <ul className="space-y-1">{pending.map(Row)}</ul>
      </div>
      {done.length > 0 && (
        <div>
          <div className="mb-1 text-[11px] font-semibold uppercase tracking-normal text-muted">Done · {done.length}</div>
          <ul className="space-y-1 opacity-80">{done.map(Row)}</ul>
        </div>
      )}
    </div>
  );
}

const str = (v: unknown): string => (v == null ? "" : String(v));
function tagList(v: unknown): string[] {
  if (Array.isArray(v)) return v.map(String);
  if (typeof v === "string") return v.split(",").map((s) => s.trim()).filter(Boolean);
  return [];
}

// CalendarView (M860): an agenda grouped by date, soonest first — upcoming events
// above past ones.
function CalendarView({ records, onEdit, onDelete }: BespokeProps) {
  const sorted = [...records].sort((a, b) => str(a.fields?.date).localeCompare(str(b.fields?.date)));
  const today = new Date().toISOString().slice(0, 10);
  const upcoming = sorted.filter((r) => str(r.fields?.date) >= today);
  const past = sorted.filter((r) => str(r.fields?.date) < today).reverse();
  const Item = (r: DataRecord) => (
    <li key={r.id} className="group flex items-start gap-3 glass rounded-xl px-3 py-2 text-sm">
      <div className="flex w-16 shrink-0 flex-col items-center rounded-md bg-panel py-1">
        <CalendarDays className="size-3.5 text-accent" />
        <span className="mt-0.5 font-mono text-xs text-muted">{str(r.fields?.date).slice(5) || "—"}</span>
      </div>
      <div className="min-w-0 flex-1">
        <div className="truncate font-medium">{str(r.fields?.title) || "—"}</div>
        <div className="flex flex-wrap gap-x-3 text-[11px] text-muted">
          {str(r.fields?.time) && <span>{str(r.fields?.time)}</span>}
          {str(r.fields?.location) && <span>· {str(r.fields?.location)}</span>}
        </div>
      </div>
      <RowActions r={r} onEdit={onEdit} onDelete={onDelete} />
    </li>
  );
  return (
    <div className="min-h-0 flex-1 space-y-3 overflow-auto">
      <div>
        <div className="mb-1 text-[11px] font-semibold uppercase tracking-normal text-muted">Upcoming · {upcoming.length}</div>
        <ul className="space-y-1">{upcoming.map(Item)}</ul>
      </div>
      {past.length > 0 && (
        <div>
          <div className="mb-1 text-[11px] font-semibold uppercase tracking-normal text-muted">Past · {past.length}</div>
          <ul className="space-y-1 opacity-70">{past.map(Item)}</ul>
        </div>
      )}
    </div>
  );
}

// HabitsView (M860): streak cards — biggest streak first.
function HabitsView({ records, onEdit, onDelete }: BespokeProps) {
  const sorted = [...records].sort((a, b) => num(b.fields?.streak) - num(a.fields?.streak));
  return (
    <div className="grid min-h-0 flex-1 grid-cols-2 gap-2 overflow-auto md:grid-cols-3">
      {sorted.map((r) => (
        <div key={r.id} className="group flex flex-col glass rounded-xl p-3">
          <div className="flex items-center gap-2">
            <span className="min-w-0 flex-1 truncate font-medium">{str(r.fields?.name) || "—"}</span>
            <RowActions r={r} onEdit={onEdit} onDelete={onDelete} />
          </div>
          <div className="mt-2 flex items-center gap-1.5">
            <Flame className={cn("size-5", num(r.fields?.streak) > 0 ? "text-warn" : "text-muted")} />
            <span className="font-mono text-xl font-semibold">{num(r.fields?.streak)}</span>
            <span className="text-[11px] text-muted">day streak</span>
          </div>
          <div className="mt-1 flex flex-wrap gap-x-3 text-[11px] text-muted">
            {str(r.fields?.cadence) && <span>{str(r.fields?.cadence)}</span>}
            {str(r.fields?.last) && <span>· last {str(r.fields?.last)}</span>}
          </div>
        </div>
      ))}
    </div>
  );
}

// NotesView (M860): a card grid (title + body + tags).
function NotesView({ records, onEdit, onDelete }: BespokeProps) {
  return (
    <div className="grid min-h-0 flex-1 grid-cols-1 gap-2 overflow-auto md:grid-cols-2 xl:grid-cols-3">
      {records.map((r) => (
        <div key={r.id} className="group flex flex-col glass rounded-xl p-3">
          <div className="flex items-center gap-2">
            <StickyNote className="size-3.5 shrink-0 text-accent" />
            <span className="min-w-0 flex-1 truncate font-medium">{str(r.fields?.title) || "untitled"}</span>
            <RowActions r={r} onEdit={onEdit} onDelete={onDelete} />
          </div>
          {str(r.fields?.body) && <p className="mt-1.5 line-clamp-5 whitespace-pre-wrap text-xs text-foreground/80">{str(r.fields?.body)}</p>}
          <div className="mt-auto flex flex-wrap gap-1 pt-2">
            {tagList(r.fields?.tags).map((t) => (
              <Badge key={t} variant="default">{t}</Badge>
            ))}
          </div>
        </div>
      ))}
    </div>
  );
}

// BookmarksView (M860): a list of links you can open, with tags.
function BookmarksView({ records, onEdit, onDelete }: BespokeProps) {
  return (
    <ul className="min-h-0 flex-1 space-y-1 overflow-auto">
      {records.map((r) => {
        const url = str(r.fields?.url);
        // Only link when the scheme is navigable (http/https/mailto/…); a stored
        // url like "javascript:alert(1)" would otherwise execute on click (XSS).
        const href = safeHref(url);
        return (
          <li key={r.id} className="group flex items-center gap-3 glass rounded-xl px-3 py-2 text-sm">
            <Bookmark className="size-3.5 shrink-0 text-accent" />
            <div className="min-w-0 flex-1">
              <div className="truncate font-medium">{str(r.fields?.title) || url || "—"}</div>
              {url && (
                href ? (
                  <a href={href} target="_blank" rel="noreferrer" className="inline-flex items-center gap-1 truncate text-[11px] text-accent hover:underline">
                    <ExternalLink className="size-3" /> {url}
                  </a>
                ) : (
                  <span className="inline-flex items-center gap-1 truncate text-[11px] text-muted" title="blocked non-navigable URL">
                    <ExternalLink className="size-3" /> {url}
                  </span>
                )
              )}
            </div>
            <div className="hidden shrink-0 flex-wrap gap-1 sm:flex">
              {tagList(r.fields?.tags).map((t) => (
                <Badge key={t} variant="default">{t}</Badge>
              ))}
            </div>
            <RowActions r={r} onEdit={onEdit} onDelete={onDelete} />
          </li>
        );
      })}
    </ul>
  );
}

// ContactsView (M860): a card grid (name + email + phone + company).
function ContactsView({ records, onEdit, onDelete }: BespokeProps) {
  const sorted = [...records].sort((a, b) => str(a.fields?.name).localeCompare(str(b.fields?.name)));
  return (
    <div className="grid min-h-0 flex-1 grid-cols-1 gap-2 overflow-auto md:grid-cols-2 xl:grid-cols-3">
      {sorted.map((r) => (
        <div key={r.id} className="group flex flex-col glass rounded-xl p-3">
          <div className="flex items-center gap-2">
            <span className="flex size-7 shrink-0 items-center justify-center rounded-full bg-panel text-accent">
              <User className="size-3.5" />
            </span>
            <div className="min-w-0 flex-1">
              <div className="truncate font-medium">{str(r.fields?.name) || "—"}</div>
              {str(r.fields?.company) && <div className="truncate text-[11px] text-muted">{str(r.fields?.company)}</div>}
            </div>
            <RowActions r={r} onEdit={onEdit} onDelete={onDelete} />
          </div>
          <div className="mt-2 space-y-0.5 text-[11px] text-muted">
            {str(r.fields?.email) && (
              <a href={`mailto:${str(r.fields?.email)}`} className="flex items-center gap-1.5 hover:text-accent">
                <Mail className="size-3" /> <span className="truncate">{str(r.fields?.email)}</span>
              </a>
            )}
            {str(r.fields?.phone) && (
              <div className="flex items-center gap-1.5">
                <Phone className="size-3" /> {str(r.fields?.phone)}
              </div>
            )}
          </div>
        </div>
      ))}
    </div>
  );
}

function SummaryCard({ label, value, icon }: { label: string; value: string; icon?: ReactNode }) {
  return (
    <div className="glass rounded-xl p-3">
      <div className="flex items-center gap-1.5 text-[11px] uppercase tracking-normal text-muted">
        {icon}
        {label}
      </div>
      <div className="mt-1 font-mono text-lg font-semibold">{value}</div>
    </div>
  );
}

function RowActions({ r, onEdit, onDelete }: { r: DataRecord; onEdit: (r: DataRecord) => void; onDelete: (r: DataRecord) => void }) {
  return (
    <span className="flex shrink-0 items-center gap-1">
      <button onClick={() => onEdit(r)} className="rounded p-1 text-muted hover:bg-accent/10 hover:text-accent" title="edit" aria-label="edit record">
        <Pencil className="size-3.5" />
      </button>
      <button onClick={() => onDelete(r)} className="rounded p-1 text-muted hover:bg-bad/10 hover:text-bad" title="delete" aria-label="delete record">
        <Trash2 className="size-3.5" />
      </button>
    </span>
  );
}

function RecordEditor({
  collection,
  record,
  onClose,
  onSave,
}: {
  collection: Collection;
  record: DataRecord | null;
  onClose: () => void;
  onSave: (fields: Record<string, unknown>, id?: string) => void;
}) {
  const defs: FieldDef[] = collection.fields?.length
    ? collection.fields
    : Object.keys(record?.fields ?? {}).map((name) => ({ name }));
  const [vals, setVals] = useState<Record<string, string>>(() => {
    const init: Record<string, string> = {};
    for (const f of defs) {
      const v = record?.fields?.[f.name];
      init[f.name] = v === undefined || v === null ? "" : typeof v === "object" ? JSON.stringify(v) : String(v);
    }
    return init;
  });

  function coerce(def: FieldDef, raw: string): unknown {
    const s = raw.trim();
    if (s === "") return null;
    if (def.type === "number" || def.type === "money") {
      const n = Number(s);
      return Number.isFinite(n) ? n : s;
    }
    if (def.type === "bool") return s === "true" || s === "1" || s === "yes" || s === "✓";
    if (def.type === "tags") return s.split(",").map((x) => x.trim()).filter(Boolean);
    return s;
  }

  function submit() {
    const fields: Record<string, unknown> = {};
    for (const f of defs) fields[f.name] = coerce(f, vals[f.name] ?? "");
    onSave(fields, record?.id);
  }

  const titleId = `data-record-editor-title-${collection.name}`;
  const mode = record ? "Edit" : "Add";

  return (
    <div className="fixed inset-0 z-[200] flex items-center justify-center bg-black/70 p-4 backdrop-blur-sm" onClick={onClose}>
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
        className="flex max-h-[88vh] w-full max-w-lg flex-col overflow-hidden rounded-xl border border-border bg-card shadow-2xl"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-start gap-3 border-b border-border px-4 py-3">
          <span className="mt-0.5 grid size-9 shrink-0 place-items-center rounded-lg bg-accent/12 text-accent">
            <Database className="size-4" />
          </span>
          <div className="min-w-0 flex-1">
            <div id={titleId} className="truncate text-sm font-semibold">
              {mode} {collection.title || collection.name} record
            </div>
            <div className="mt-1 flex flex-wrap items-center gap-1.5 text-[11px] text-muted">
              <Badge variant="default">{collection.view || "table"}</Badge>
              <span>{defs.length} fields</span>
              {record?.id && <span className="font-mono">id {record.id}</span>}
            </div>
          </div>
          <button onClick={onClose} className="text-muted hover:text-foreground" aria-label="Close">
            <X className="size-4" />
          </button>
        </div>
        <div className="min-h-0 flex-1 space-y-2 overflow-auto p-4">
          {defs.map((f) => {
            const inputId = `data-field-${collection.name}-${f.name}`;
            return (
            <label key={f.name} htmlFor={inputId} className="grid gap-1 rounded-lg border border-border/70 bg-panel/50 p-2.5">
              <span className="flex items-center gap-2 text-xs font-medium text-foreground/90">
                <span className="min-w-0 flex-1 truncate">{f.label || f.name}</span>
                {f.type && <span className="rounded bg-card px-1.5 py-0.5 font-mono text-[10px] text-muted">{f.type}</span>}
              </span>
              {f.type === "note" ? (
                <textarea
                  id={inputId}
                  rows={3}
                  value={vals[f.name] ?? ""}
                  onChange={(e) => setVals((v) => ({ ...v, [f.name]: e.target.value }))}
                  className="w-full resize-y rounded-md border border-border bg-card px-2 py-1.5 text-sm outline-none focus-visible:border-accent"
                />
              ) : (
                <input
                  id={inputId}
                  value={vals[f.name] ?? ""}
                  onChange={(e) => setVals((v) => ({ ...v, [f.name]: e.target.value }))}
                  placeholder={f.type === "bool" ? "true / false" : f.type === "tags" ? "comma,separated" : ""}
                  className="w-full rounded-md border border-border bg-card px-2 py-1.5 text-sm outline-none focus-visible:border-accent"
                />
              )}
            </label>
          );
          })}
        </div>
        <div className="flex items-center justify-end gap-2 border-t border-border px-4 py-2">
          <Button variant="ghost" size="sm" onClick={onClose}>
            Cancel
          </Button>
          <Button size="sm" onClick={submit}>
            <Save className="size-3.5" />
            {record ? "Save" : "Add"}
          </Button>
        </div>
      </div>
    </div>
  );
}
