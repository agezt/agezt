// Analyst.tsx — Day 26 bring-back.
//
// "Analyst" is the second of three Thinking Partners under Knowledge. It
// reads the journal (the immutable event log the daemon emits for every
// policy decision, provider fallback, run lifecycle, etc.) and surfaces the
// shape of what's happening: top event kinds, recent actors, and the kinds
// of decisions that took the longest. The view is read-only — analysts ask
// questions of the data, not of the model — so the page never calls a
// generation endpoint; it only ever fetches /api/journal_search.
import { useEffect, useMemo, useState } from "react";
import { BarChart3, ChevronRight, Clock, Filter, Layers } from "lucide-react";
import { getJSON } from "@/app/api";
import { Button } from "@/components/ui/button";
import { cn, fmtAgo, fmtTime } from "@/app/utils/utils";

interface JournalEvent {
  ts?: number;
  kind?: string;
  actor?: string;
  subject?: string;
  correlation_id?: string;
  note?: string;
  data?: Record<string, unknown>;
}

const KIND_COLORS: Record<string, string> = {
  "policy.decision": "bg-warning/15 text-warning",
  "provider.fallback": "bg-danger/15 text-danger",
  "tool.invoke": "bg-accent/15 text-accent",
  "tool.result": "bg-success/15 text-success",
  "memory.write": "bg-info/15 text-info",
  "memory.supersede": "bg-info/15 text-info",
  "run.start": "bg-success/15 text-success",
  "run.end": "bg-success/15 text-success",
  "run.fail": "bg-danger/15 text-danger",
  "approval.request": "bg-warning/15 text-warning",
  "approval.decide": "bg-success/15 text-success",
  "channel.message": "bg-fg/10 text-fg",
};

function kindColor(kind?: string) {
  if (!kind) return "bg-fg/10 text-fg";
  return KIND_COLORS[kind] || "bg-fg/10 text-fg";
}

export function Analyst() {
  const [events, setEvents] = useState<JournalEvent[]>([]);
  const [loading, setLoading] = useState(true);
  const [filterKind, setFilterKind] = useState<string>("all");
  const [filterText, setFilterText] = useState("");
  const [limit, setLimit] = useState(50);
  const [error, setError] = useState<string | null>(null);

  async function refresh() {
    setLoading(true);
    setError(null);
    try {
      const r = await getJSON<{ events?: JournalEvent[] }>(
        `/api/journal?limit=${limit}`,
      );
      setEvents(r.events || []);
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    refresh();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const kinds = useMemo(() => {
    const counts = new Map<string, number>();
    events.forEach((e) => {
      if (!e.kind) return;
      counts.set(e.kind, (counts.get(e.kind) || 0) + 1);
    });
    return Array.from(counts.entries()).sort((a, b) => b[1] - a[1]);
  }, [events]);

  const actors = useMemo(() => {
    const counts = new Map<string, number>();
    events.forEach((e) => {
      if (!e.actor) return;
      counts.set(e.actor, (counts.get(e.actor) || 0) + 1);
    });
    return Array.from(counts.entries()).sort((a, b) => b[1] - a[1]).slice(0, 8);
  }, [events]);

  const filtered = useMemo(() => {
    const txt = filterText.trim().toLowerCase();
    return events.filter((e) => {
      if (filterKind !== "all" && e.kind !== filterKind) return false;
      if (!txt) return true;
      const blob = [
        e.kind,
        e.actor,
        e.subject,
        e.correlation_id,
        e.note,
        JSON.stringify(e.data || {}),
      ]
        .filter(Boolean)
        .join(" ")
        .toLowerCase();
      return blob.includes(txt);
    });
  }, [events, filterKind, filterText]);

  return (
    <div className="flex h-full flex-col gap-4 p-4">
      <header className="rounded-lg border border-border bg-card p-4">
        <div className="flex items-center gap-2">
          <BarChart3 className="size-5 text-accent" />
          <h2 className="text-lg font-semibold text-fg">Analyst</h2>
          <span className="rounded-full bg-accent/10 px-2 py-0.5 text-[10px] font-medium uppercase tracking-wide text-accent">
            Thinking partner
          </span>
        </div>
        <p className="mt-1 text-sm text-muted">
          Browse the journal. Filter by event kind or pattern, then drill into individual entries.
        </p>

        <div className="mt-3 flex flex-wrap items-end gap-3">
          <label className="flex flex-1 min-w-[180px] flex-col gap-1 text-xs font-medium text-muted">
            <span className="inline-flex items-center gap-1">
              <Filter className="size-3" /> Pattern
            </span>
            <input
              type="text"
              value={filterText}
              onChange={(e) => setFilterText(e.target.value)}
              placeholder="search kind, actor, subject, correlation_id…"
              className="w-full rounded-md border border-border bg-bg px-2 py-1.5 text-sm text-fg focus:border-accent focus:outline-none"
            />
          </label>
          <label className="flex flex-col gap-1 text-xs font-medium text-muted">
            <span className="inline-flex items-center gap-1">
              <Layers className="size-3" /> Kind
            </span>
            <select
              value={filterKind}
              onChange={(e) => setFilterKind(e.target.value)}
              className="rounded-md border border-border bg-bg px-2 py-1.5 text-sm text-fg focus:border-accent focus:outline-none"
            >
              <option value="all">All kinds</option>
              {kinds.map(([k]) => (
                <option key={k} value={k}>
                  {k}
                </option>
              ))}
            </select>
          </label>
          <label className="flex flex-col gap-1 text-xs font-medium text-muted">
            <span className="inline-flex items-center gap-1">
              <Clock className="size-3" /> Limit
            </span>
            <input
              type="number"
              min={10}
              max={500}
              value={limit}
              onChange={(e) => {
                const n = Number(e.target.value);
                if (Number.isFinite(n)) setLimit(Math.max(10, Math.min(500, n)));
              }}
              className="w-28 rounded-md border border-border bg-bg px-2 py-1.5 text-sm text-fg focus:border-accent focus:outline-none"
            />
          </label>
          <Button onClick={refresh} disabled={loading}>
            {loading ? "Refreshing…" : "Refresh"}
          </Button>
        </div>
      </header>

      {error && (
        <div className="rounded-lg border border-danger/40 bg-danger/5 p-3 text-sm text-danger">
          {error}
        </div>
      )}

      <div className="grid flex-1 gap-4 lg:grid-cols-[260px_1fr]">
        <aside className="flex flex-col gap-4">
          <Distribution title="Event kinds" entries={kinds} />
          <Distribution title="Top actors" entries={actors} />
        </aside>

        <main className="overflow-auto rounded-lg border border-border bg-card">
          <table className="w-full text-sm">
            <thead className="sticky top-0 bg-card text-left text-xs uppercase tracking-wide text-muted">
              <tr>
                <th className="px-3 py-2">When</th>
                <th className="px-3 py-2">Kind</th>
                <th className="px-3 py-2">Actor</th>
                <th className="px-3 py-2">Subject</th>
                <th className="px-3 py-2">Note</th>
              </tr>
            </thead>
            <tbody>
              {loading && events.length === 0 ? (
                <tr>
                  <td colSpan={5} className="px-3 py-6 text-center text-sm text-muted">
                    Loading journal…
                  </td>
                </tr>
              ) : filtered.length === 0 ? (
                <tr>
                  <td colSpan={5} className="px-3 py-6 text-center text-sm text-muted">
                    No events match the filter.
                  </td>
                </tr>
              ) : (
                filtered.map((e, i) => (
                  <tr key={i} className="border-t border-border align-top hover:bg-bg/40">
                    <td className="px-3 py-2 whitespace-nowrap text-xs text-muted">
                      {fmtAgo(e.ts)}
                    </td>
                    <td className="px-3 py-2">
                      <span className={cn("rounded-full px-2 py-0.5 text-[10px] font-medium", kindColor(e.kind))}>
                        {e.kind || "—"}
                      </span>
                    </td>
                    <td className="px-3 py-2 text-xs text-fg/90">{e.actor || "—"}</td>
                    <td className="px-3 py-2 text-xs text-fg/90">
                      {e.subject || e.correlation_id || "—"}
                    </td>
                    <td className="px-3 py-2 text-xs text-fg/80">{e.note || "—"}</td>
                  </tr>
                ))
              )}
            </tbody>
          </table>
        </main>
      </div>
    </div>
  );
}

function Distribution({
  title,
  entries,
}: {
  title: string;
  entries: [string, number][];
}) {
  const max = entries.reduce((m, [, n]) => Math.max(m, n), 1);
  return (
    <div className="rounded-lg border border-border bg-card p-3">
      <h3 className="text-sm font-semibold text-fg">{title}</h3>
      {entries.length === 0 ? (
        <p className="mt-2 text-xs text-muted">No data yet.</p>
      ) : (
        <ul className="mt-2 space-y-1.5">
          {entries.map(([label, count]) => (
            <li key={label} className="flex items-center gap-2 text-xs">
              <span className="w-24 shrink-0 truncate text-fg/80" title={label}>
                {label}
              </span>
              <span className="h-2 flex-1 overflow-hidden rounded-full bg-bg">
                <span
                  className="block h-full rounded-full bg-accent"
                  style={{ width: `${Math.max(8, (count / max) * 100)}%` }}
                />
              </span>
              <span className="w-8 shrink-0 text-right text-[10px] text-muted">{count}</span>
            </li>
          ))}
        </ul>
      )}
      <p className="mt-2 inline-flex items-center gap-1 text-[10px] uppercase tracking-wide text-muted">
        <ChevronRight className="size-3" /> {fmtTime(Date.now())} snapshot
      </p>
    </div>
  );
}
