import { useEffect, useRef, useState } from "react";
import { RefreshCw, ChevronRight, ChevronDown, ListTree, Search } from "lucide-react";
import { usePanel } from "@/lib/usePanel";
import { Button } from "@/components/ui/button";
import { PageHeader } from "@/components/ui/page-header";
import { Badge, statusVariant } from "@/components/ui/badge";
import { ErrorText } from "@/components/JsonView";
import { EmptyState } from "@/components/ui/empty";
import { cn, fmtTime } from "@/lib/utils";
import { RunDetailLoader } from "@/components/RunDetail";
import { useRunFocus, clearRunFocus } from "@/lib/runfocus";

interface Run {
  correlation_id?: string;
  status?: string;
  intent?: string;
  duration_ms?: number;
  started_unix_ms?: number;
}

function RunRow({ run, focus }: { run: Run; focus?: string | null }) {
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);
  const isFocus = !!focus && focus === run.correlation_id;

  // When the ⌘K palette (or any deep-link) targets this run, expand it, scroll it
  // into view, and consume the focus so a later manual collapse isn't re-opened.
  useEffect(() => {
    if (!isFocus) return;
    setOpen(true);
    ref.current?.scrollIntoView?.({ block: "center", behavior: "smooth" });
    clearRunFocus();
  }, [isFocus]);

  return (
    <div ref={ref} className={cn("border-b border-border/60", isFocus && "bg-accent/5")}>
      <button
        onClick={() => setOpen((v) => !v)}
        className="flex w-full items-center gap-2 px-1 py-1.5 text-left hover:bg-panel"
      >
        {open ? <ChevronDown className="size-4 shrink-0" /> : <ChevronRight className="size-4 shrink-0" />}
        <Badge variant={statusVariant(run.status)}>{run.status || "?"}</Badge>
        <span className="truncate">{run.intent || run.correlation_id || "run"}</span>
        <span className="ml-auto shrink-0 text-muted">
          {run.duration_ms ? `${run.duration_ms}ms` : ""} {fmtTime(run.started_unix_ms)}
        </span>
      </button>
      {open && (
        <div className="px-7 pb-3 pt-1 text-sm">
          <RunDetailLoader correlationId={run.correlation_id} status={run.status} durationMs={run.duration_ms} />
        </div>
      )}
    </div>
  );
}

// runMatches tests a run against a lowercased query over its intent, status, and
// correlation id — so you can pin down a run by what it was doing, how it ended, or its
// id. Typing "failed" surfaces every failed run, "deploy" every deploy, and so on.
export function runMatches(r: Run, q: string): boolean {
  if (!q) return true;
  return `${r.intent || ""} ${r.status || ""} ${r.correlation_id || ""}`.toLowerCase().includes(q);
}

// RunBucket is the coarse outcome a run falls into for the summary band and the
// filter chips. "other" catches statuses the kernel may add later (e.g. queued)
// so the counts always sum to the total — no run silently vanishes from view.
export type RunBucket = "running" | "completed" | "failed" | "other";

// runBucket classifies one run's status, mirroring Insights' outcome buckets
// (completed / failed / running) plus an "other" catch-all. "abandoned" counts as
// failed — it ended without finishing the work, which is what the operator cares
// about when scanning for trouble.
export function runBucket(r: Run): RunBucket {
  switch ((r.status || "").toLowerCase()) {
    case "running":
      return "running";
    case "completed":
      return "completed";
    case "failed":
    case "abandoned":
      return "failed";
    default:
      return "other";
  }
}

export interface RunCounts {
  total: number;
  running: number;
  completed: number;
  failed: number;
  other: number;
}

// runCounts tallies the bucket distribution across the run list for the summary
// band. Pure and exported so it's unit-tested without rendering.
export function runCounts(runs: Run[]): RunCounts {
  const c: RunCounts = { total: runs.length, running: 0, completed: 0, failed: 0, other: 0 };
  for (const r of runs) c[runBucket(r)]++;
  return c;
}

// CHIPS drives the filter row in display order: live runs first (what's happening
// now), then the two terminal outcomes, then the catch-all. Each carries the dot
// colour so the chip reads as the same status the badge shows.
const CHIPS: { bucket: RunBucket; label: string; dot: string }[] = [
  { bucket: "running", label: "running", dot: "bg-accent" },
  { bucket: "completed", label: "completed", dot: "bg-good" },
  { bucket: "failed", label: "failed", dot: "bg-bad" },
  { bucket: "other", label: "other", dot: "bg-muted" },
];

export function Runs() {
  const { data, error, loading, reload } = usePanel<{ runs?: Run[] }>("/api/runs");
  const runs = data?.runs || [];
  const focus = useRunFocus();
  const counts = runCounts(runs);
  const [q, setQ] = useState("");
  // A null filter means "all"; clicking the active chip clears it back to all.
  const [filter, setFilter] = useState<RunBucket | null>(null);
  const query = q.trim().toLowerCase();

  // When a deep-link focuses a run that the active filter would hide, drop the
  // filter so the targeted run is actually visible when it scrolls into view.
  useEffect(() => {
    if (!focus || !filter) return;
    const target = runs.find((r) => r.correlation_id === focus);
    if (target && runBucket(target) !== filter) setFilter(null);
  }, [focus, filter, runs]);

  const shown = runs.filter(
    (r) => (!filter || runBucket(r) === filter) && (!query || runMatches(r, query)),
  );

  return (
    <div className="space-y-4">
      <PageHeader
        icon={ListTree}
        title="Runs"
        description="Completed and in-flight runs across the fleet."
        actions={
          <>
            {runs.length > 0 && <span className="text-xs text-muted">{counts.total} total</span>}
            <Button variant="ghost" size="icon" onClick={reload} title="Refresh">
              <RefreshCw className={loading ? "animate-spin" : ""} />
            </Button>
          </>
        }
      />
      <div className="glass rounded-xl p-3">
        {/* Status distribution + click-to-filter chips, so the shape of the fleet's
            recent work (how many failed, what's still running) is visible at a glance
            before you scan the list. A chip with a zero count is disabled. */}
        {runs.length > 0 && (
          <div className="mb-2 flex flex-wrap gap-1.5">
            {CHIPS.map((chip) => {
              const n = counts[chip.bucket];
              const active = filter === chip.bucket;
              return (
                <button
                  key={chip.bucket}
                  disabled={n === 0}
                  onClick={() => setFilter(active ? null : chip.bucket)}
                  aria-pressed={active}
                  aria-label={`${chip.label} ${n}`}
                  className={cn(
                    "inline-flex items-center gap-1.5 rounded-full border px-2.5 py-1 text-xs transition-colors",
                    n === 0 && "cursor-default opacity-40",
                    active
                      ? "border-accent bg-accent/10 text-foreground"
                      : "border-border text-muted hover:bg-panel",
                  )}
                >
                  <span className={cn("size-2 rounded-full", chip.dot)} />
                  {chip.label}
                  <span className="tabular-nums text-muted">{n}</span>
                </button>
              );
            })}
          </div>
        )}
        {/* Find a run by intent, status (e.g. "failed"), or id once the list grows. */}
        {runs.length > 4 && (
          <div className="relative mb-2">
            <Search className="pointer-events-none absolute left-2 top-1/2 size-3.5 -translate-y-1/2 text-muted" />
            <input
              value={q}
              onChange={(e) => setQ(e.target.value)}
              placeholder="filter runs…"
              aria-label="Filter runs"
              className="h-8 w-full rounded-md border border-border bg-panel pl-7 pr-12 text-xs text-foreground outline-none focus-visible:border-accent"
            />
            {query && (
              <span className="absolute right-2 top-1/2 -translate-y-1/2 text-[10px] text-muted">
                {shown.length}/{runs.length}
              </span>
            )}
          </div>
        )}
        {error ? (
          <ErrorText>{error}</ErrorText>
        ) : runs.length === 0 ? (
          <EmptyState icon={ListTree} title="No runs yet" hint="Completed and in-flight runs will be listed here — start one from Chat or the CLI." />
        ) : shown.length === 0 ? (
          <p className="px-1 py-2 text-xs text-muted">
            no runs match {filter ? `“${filter}”` : ""}
            {filter && query ? " + " : ""}
            {query ? `“${q.trim()}”` : ""}
          </p>
        ) : (
          shown.map((r, i) => <RunRow key={r.correlation_id || i} run={r} focus={focus} />)
        )}
      </div>
    </div>
  );
}
