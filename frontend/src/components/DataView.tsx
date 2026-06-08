// DataView is the widget engine: it renders arbitrary structured data (the shape
// an agent emits as a ```json / ```widget block, or a JSON tool output) as the
// view that fits its shape — an array of objects becomes a table, an object a
// key/value card, an array of scalars a list — recursively, with a raw-JSON
// fallback. Everything is plain escaped React (no raw HTML), so it's CSP-safe.

function isPlainObject(v: unknown): v is Record<string, unknown> {
  return typeof v === "object" && v !== null && !Array.isArray(v);
}

// Columns for a table = the union of the rows' keys, first-seen order preserved.
function unionKeys(rows: Record<string, unknown>[]): string[] {
  const seen: string[] = [];
  const set = new Set<string>();
  for (const r of rows) {
    for (const k of Object.keys(r)) {
      if (!set.has(k)) {
        set.add(k);
        seen.push(k);
      }
    }
  }
  return seen;
}

// Scalars render inline; nested objects/arrays render as a compact, depth-bounded
// DataView so a table cell or value stays legible without unbounded recursion.
function Cell({ value, depth }: { value: unknown; depth: number }) {
  if (value === null || value === undefined) return <span className="text-muted/60">—</span>;
  if (typeof value === "boolean") {
    return <span className={value ? "text-good" : "text-muted"}>{value ? "true" : "false"}</span>;
  }
  if (typeof value === "number" || typeof value === "string") {
    return <span className="break-words">{String(value)}</span>;
  }
  if (depth >= 3) {
    return <code className="break-all font-mono text-[11px] text-muted">{JSON.stringify(value)}</code>;
  }
  return <DataView data={value} depth={depth + 1} />;
}

export function DataView({ data, depth = 0 }: { data: unknown; depth?: number }) {
  // Array of objects → table.
  if (Array.isArray(data) && data.length > 0 && data.every(isPlainObject)) {
    const rows = data as Record<string, unknown>[];
    const cols = unionKeys(rows);
    return (
      <div className="my-2 overflow-x-auto rounded-lg border border-border">
        <table className="w-full border-collapse text-xs">
          <thead>
            <tr className="bg-panel/70">
              {cols.map((c) => (
                <th key={c} className="border-b border-border px-2.5 py-1.5 text-left font-semibold text-foreground">
                  {c}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {rows.map((row, r) => (
              <tr key={r} className={r % 2 ? "bg-panel/20" : ""}>
                {cols.map((c) => (
                  <td key={c} className="border-t border-border px-2.5 py-1.5 align-top">
                    <Cell value={row[c]} depth={depth} />
                  </td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    );
  }

  // Array of scalars (or mixed) → list.
  if (Array.isArray(data)) {
    if (data.length === 0) return <span className="text-muted/60">(empty)</span>;
    return (
      <ul className="my-1.5 list-disc space-y-0.5 pl-5 text-xs">
        {data.map((v, i) => (
          <li key={i}>
            <Cell value={v} depth={depth} />
          </li>
        ))}
      </ul>
    );
  }

  // Object → key / value card.
  if (isPlainObject(data)) {
    const entries = Object.entries(data);
    if (entries.length === 0) return <span className="text-muted/60">{"{}"}</span>;
    return (
      <div className="my-2 overflow-hidden rounded-lg border border-border text-xs">
        {entries.map(([k, v], i) => (
          <div key={k} className={`flex gap-2 px-2.5 py-1.5 ${i ? "border-t border-border" : ""}`}>
            <span className="w-32 shrink-0 font-medium text-muted">{k}</span>
            <span className="min-w-0 flex-1">
              <Cell value={v} depth={depth} />
            </span>
          </div>
        ))}
      </div>
    );
  }

  // Scalar.
  return <Cell value={data} depth={depth} />;
}
