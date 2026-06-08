import { useState } from "react";
import { Search as SearchIcon, Loader2 } from "lucide-react";
import { getJSON } from "@/lib/api";
import type { AgentEvent } from "@/lib/events";
import { categoryOf, isErrorKind } from "@/lib/eventmeta";
import { cn, fmtTime } from "@/lib/utils";
import { DataView } from "@/components/DataView";
import { Muted, ErrorText } from "@/components/JsonView";

// Search queries the FULL journal server-side (CmdJournalGrep) — the historical
// counterpart to the live stream. Filter by free-text pattern plus
// kind/actor/correlation; results are colour-coded and payload-expandable, so
// you can find and inspect any past event across the daemon's whole history.
export function Search() {
  const [pattern, setPattern] = useState("");
  const [kind, setKind] = useState("");
  const [actor, setActor] = useState("");
  const [corr, setCorr] = useState("");
  const [results, setResults] = useState<AgentEvent[] | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [open, setOpen] = useState<string | null>(null);

  async function run() {
    setLoading(true);
    setErr(null);
    try {
      const params: Record<string, string> = { limit: "200" };
      if (pattern.trim()) params.pattern = pattern.trim();
      if (kind.trim()) params.kind = kind.trim();
      if (actor.trim()) params.actor = actor.trim();
      if (corr.trim()) params.correlation_id = corr.trim();
      const d = await getJSON<{ events?: AgentEvent[] }>("/api/journal_search", params);
      setResults(d.events || []);
    } catch (e) {
      setErr((e as Error).message);
      setResults(null);
    } finally {
      setLoading(false);
    }
  }

  function onKey(e: React.KeyboardEvent) {
    if (e.key === "Enter") run();
  }

  return (
    <div className="flex h-full min-h-0 flex-col gap-3">
      <h2 className="flex items-center gap-2 text-sm font-semibold">
        <SearchIcon className="size-4 text-accent" /> Journal search
      </h2>

      {/* Filters */}
      <div className="grid grid-cols-1 gap-2 sm:grid-cols-2 lg:grid-cols-4">
        <Field label="text" value={pattern} onChange={setPattern} onKey={onKey} placeholder="match anywhere…" autoFocus />
        <Field label="kind" value={kind} onChange={setKind} onKey={onKey} placeholder="e.g. tool.result" />
        <Field label="actor" value={actor} onChange={setActor} onKey={onKey} placeholder="e.g. agent-…" />
        <Field label="correlation" value={corr} onChange={setCorr} onKey={onKey} placeholder="run id" />
      </div>
      <div className="flex items-center gap-2">
        <button
          onClick={run}
          disabled={loading}
          className="inline-flex h-8 items-center gap-1.5 rounded-md border border-accent px-3 text-sm text-accent transition-colors hover:bg-accent hover:text-white disabled:opacity-50"
        >
          {loading ? <Loader2 className="size-4 animate-spin" /> : <SearchIcon className="size-4" />} Search
        </button>
        {results && <span className="text-xs text-muted">{results.length} match{results.length === 1 ? "" : "es"}</span>}
        <span className="ml-auto text-[11px] text-muted">searches the full journal (server-side)</span>
      </div>

      {/* Results */}
      <div className="min-h-0 flex-1 overflow-auto rounded-lg border border-border bg-card font-mono text-xs">
        {err ? (
          <div className="p-3">
            <ErrorText>{err}</ErrorText>
          </div>
        ) : !results ? (
          <div className="p-3">
            <Muted>enter a filter and search the daemon's whole history</Muted>
          </div>
        ) : results.length === 0 ? (
          <div className="p-3">
            <Muted>no events match</Muted>
          </div>
        ) : (
          <ul className="divide-y divide-border/40">
            {results.map((e, i) => {
              const cat = categoryOf(e.kind);
              const err2 = isErrorKind(e.kind);
              const id = e.id || `${e.seq}-${i}`;
              const isOpen = open === id;
              return (
                <li key={id} className={cn(err2 && "bg-bad/5")}>
                  <div
                    onClick={() => setOpen(isOpen ? null : id)}
                    className="flex cursor-pointer items-center gap-2 px-2.5 py-1 hover:bg-panel/60"
                  >
                    <span className="w-14 shrink-0 tabular-nums text-muted">{fmtTime(e.ts_unix_ms)}</span>
                    <span className="size-2 shrink-0 rounded-full" style={{ background: cat.color }} />
                    <span className="w-40 shrink-0 truncate font-medium" style={{ color: err2 ? undefined : cat.color }}>
                      <span className={cn(err2 && "text-bad")}>{e.kind}</span>
                    </span>
                    <span className="min-w-0 flex-1 truncate text-foreground/80">{e.subject}</span>
                    {e.correlation_id && (
                      <span className="shrink-0 text-[10px] text-muted">{e.correlation_id.slice(-6)}</span>
                    )}
                  </div>
                  {isOpen && (
                    <div className="border-t border-border/40 bg-panel/40 px-3 py-2">
                      <div className="mb-1 flex gap-3 text-[10px] text-muted">
                        <span>seq {e.seq ?? "—"}</span>
                        <span>actor {e.actor || "—"}</span>
                        <span>{e.correlation_id || "—"}</span>
                      </div>
                      {e.payload != null ? <DataView data={e.payload} /> : <span className="text-[11px] text-muted">no payload</span>}
                    </div>
                  )}
                </li>
              );
            })}
          </ul>
        )}
      </div>
    </div>
  );
}

function Field({
  label,
  value,
  onChange,
  onKey,
  placeholder,
  autoFocus,
}: {
  label: string;
  value: string;
  onChange: (v: string) => void;
  onKey: (e: React.KeyboardEvent) => void;
  placeholder?: string;
  autoFocus?: boolean;
}) {
  return (
    <label className="flex flex-col gap-1">
      <span className="text-[10px] uppercase tracking-wider text-muted">{label}</span>
      <input
        value={value}
        onChange={(e) => onChange(e.target.value)}
        onKeyDown={onKey}
        placeholder={placeholder}
        autoFocus={autoFocus}
        className="h-8 rounded-md border border-border bg-panel px-2 text-xs outline-none focus:border-accent"
      />
    </label>
  );
}
