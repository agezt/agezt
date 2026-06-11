import { useEffect, useMemo, useState } from "react";
import {
  Database,
  RefreshCw,
  Plus,
  Trash2,
  Search,
  X,
  Lock,
} from "lucide-react";
import { getJSON, postJSON, postAction } from "@/lib/api";
import { cn, fmtTime } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
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

export function Data() {
  const ui = useUI();
  const [cols, setCols] = useState<Collection[]>([]);
  const [active, setActive] = useState<string | null>(null);
  const [records, setRecords] = useState<DataRecord[]>([]);
  const [search, setSearch] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [loadingCols, setLoadingCols] = useState(false);
  const [loadingRecs, setLoadingRecs] = useState(false);
  const [editing, setEditing] = useState<DataRecord | "new" | null>(null);

  const activeCol = useMemo(() => cols.find((c) => c.name === active) ?? null, [cols, active]);

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
      Array.from(new Set(records.flatMap((r) => Object.keys(r.fields ?? {})))).map((name) => ({ name }));

  return (
    <div className="flex h-full min-h-0 gap-3">
      {/* Collection sidebar */}
      <div className="flex w-56 shrink-0 flex-col gap-2 overflow-y-auto">
        <div className="flex items-center gap-2">
          <h2 className="flex items-center gap-2 text-sm font-semibold">
            <Database className="size-4 text-accent" /> Data Lake
          </h2>
          <Button variant="ghost" size="sm" className="ml-auto" onClick={loadCollections} disabled={loadingCols} title="Reload">
            <RefreshCw className={cn("size-3.5", loadingCols && "animate-spin")} />
          </Button>
        </div>
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
            ) : (
              <div className="min-h-0 flex-1 overflow-auto rounded-lg border border-border">
                <table className="w-full border-collapse text-sm">
                  <thead className="sticky top-0 bg-panel text-left text-[11px] uppercase tracking-wide text-muted">
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
                    {records.map((r) => (
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
                        <td className="whitespace-nowrap border-b border-border/60 px-3 py-1.5 text-right">
                          <button
                            onClick={() => setEditing(r)}
                            className="text-muted opacity-0 transition-opacity hover:text-accent group-hover:opacity-100"
                            title={`edit · added by ${r.created_by || "?"} ${fmtTime(r.created_ms)}`}
                          >
                            edit
                          </button>
                          <button
                            onClick={() => delRecord(r)}
                            className="ml-2 text-muted opacity-0 transition-opacity hover:text-bad group-hover:opacity-100"
                            title="delete"
                          >
                            <Trash2 className="inline size-3.5" />
                          </button>
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

  return (
    <div className="fixed inset-0 z-[200] flex items-center justify-center bg-black/70 p-4 backdrop-blur-sm" onClick={onClose}>
      <div
        className="flex max-h-[88vh] w-full max-w-md flex-col overflow-hidden rounded-xl border border-border bg-card shadow-2xl"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-center gap-2 border-b border-border px-4 py-2">
          <span className="flex-1 text-sm font-semibold">
            {record ? "Edit" : "Add"} {collection.title || collection.name} record
          </span>
          <button onClick={onClose} className="text-muted hover:text-foreground" aria-label="Close">
            <X className="size-4" />
          </button>
        </div>
        <div className="min-h-0 flex-1 space-y-2 overflow-auto p-4">
          {defs.map((f) => (
            <label key={f.name} className="block">
              <span className="mb-0.5 block text-[11px] font-medium text-muted">
                {f.label || f.name}
                {f.type && <span className="ml-1 opacity-60">({f.type})</span>}
              </span>
              {f.type === "note" ? (
                <textarea
                  rows={3}
                  value={vals[f.name] ?? ""}
                  onChange={(e) => setVals((v) => ({ ...v, [f.name]: e.target.value }))}
                  className="w-full rounded-md border border-border bg-panel px-2 py-1 text-sm outline-none focus-visible:border-accent"
                />
              ) : (
                <input
                  value={vals[f.name] ?? ""}
                  onChange={(e) => setVals((v) => ({ ...v, [f.name]: e.target.value }))}
                  placeholder={f.type === "bool" ? "true / false" : f.type === "tags" ? "comma,separated" : ""}
                  className="w-full rounded-md border border-border bg-panel px-2 py-1 text-sm outline-none focus-visible:border-accent"
                />
              )}
            </label>
          ))}
        </div>
        <div className="flex items-center justify-end gap-2 border-t border-border px-4 py-2">
          <Button variant="ghost" size="sm" onClick={onClose}>
            Cancel
          </Button>
          <Button size="sm" onClick={submit}>
            {record ? "Save" : "Add"}
          </Button>
        </div>
      </div>
    </div>
  );
}
