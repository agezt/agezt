import { useEffect, useMemo, useRef, useState } from "react";
import {
  RefreshCw,
  ChevronRight,
  ChevronDown,
  ListTree,
  Search,
  CheckCircle2,
  XOctagon,
  Clock,
  CircleDot,
  CircleStop,
  Loader2,
} from "lucide-react";
import { postAction } from "@/lib/api";
import { useUI } from "@/components/ui/feedback";
import { useEvents } from "@/lib/events";
import { buildLiveRunContexts, type LiveRunContext } from "@/lib/liveruncontext";
import { Button } from "@/components/ui/button";
import { PageHeader } from "@/components/ui/page-header";
import { Badge, statusVariant } from "@/components/ui/badge";
import { ErrorText } from "@/components/JsonView";
import { EmptyState } from "@/components/ui/empty";
import { cn, fmtTime } from "@/lib/utils";
import { RunDetailLoader } from "@/components/RunDetail";
import { useRunFocus, clearRunFocus } from "@/lib/runfocus";
import { TabNav } from "@/components/ui/tab-nav";
import { MetricWidget, MetricGrid } from "@/components/ui/metric-widget";
import { useCursorPager } from "@/lib/cursorPager";

interface Run {
  correlation_id?: string;
  status?: string;
  intent?: string;
  duration_ms?: number;
  started_unix_ms?: number;
  // Server-folded live activity (authoritative, present only for running runs) —
  // a fallback for the client-side event fold so the phase shows on a fresh load.
  phase?: string;
  tool?: string;
  // Index signature so this shape satisfies useCursorPager's
  // `Record<string, unknown>` constraint — the helper reads fields by
  // string key when deduping across pages.
  [k: string]: unknown;
}

function RunRow({ run, focus, ctx }: { run: Run; focus?: string | null; ctx?: LiveRunContext }) {
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);
  const isFocus = !!focus && focus === run.correlation_id;
  // For a still-running row, surface what the agent is doing right now. Prefer the
  // client-side event fold (most real-time); fall back to the server-folded phase
  // (authoritative from the full journal) so it shows even on a fresh page load.
  const livePhase = run.status === "running" ? (ctx?.phase ?? run.phase) : undefined;
  const liveTool = ctx?.tool ?? run.tool;
  const ui = useUI();
  const [stopping, setStopping] = useState(false);
  const corr = run.correlation_id || "";

  // Cancel an in-flight run from the monitor (the daemon-side cancel_run, M32).
  // Confirmed because it ends real work; the run's terminal event then refreshes
  // the list. Guarded so a double-click can't fire twice.
  const stop = async () => {
    if (!corr || stopping) return;
    if (
      !(await ui.confirm({
        title: "Stop this run?",
        message: `“${run.intent || corr}” will be cancelled. Work already done is kept; the run ends now.`,
        confirmLabel: "Stop run",
        danger: true,
      }))
    )
      return;
    setStopping(true);
    try {
      await postAction("/api/cancel_run", { correlation: corr });
      ui.toast("Run cancelled", "success");
    } catch (err) {
      ui.toast(`Couldn't stop the run: ${err}`, "error");
    } finally {
      setStopping(false);
    }
  };

  useEffect(() => {
    if (!isFocus) return;
    setOpen(true);
    ref.current?.scrollIntoView?.({ block: "center", behavior: "smooth" });
    clearRunFocus();
  }, [isFocus]);

  return (
    <div ref={ref} className={cn("border-b border-border/40", isFocus && "bg-accent/5")}>
      <div className="flex items-center">
      <button
        onClick={() => setOpen((v) => !v)}
        className="flex flex-1 items-center gap-2 px-1 py-1.5 text-left hover:bg-panel"
      >
        {open ? <ChevronDown className="size-4 shrink-0" /> : <ChevronRight className="size-4 shrink-0" />}
        <Badge variant={statusVariant(run.status)}>{run.status || "?"}</Badge>
        <span className="truncate">{run.intent || run.correlation_id || "run"}</span>
        {livePhase && (
          <span
            className="hidden shrink-0 items-center gap-1 rounded bg-accent/10 px-1.5 py-0.5 text-xs text-accent sm:inline-flex"
            title={ctx?.agent ? `${ctx.agent} · ${livePhase}` : livePhase}
          >
            <CircleDot className="size-3 animate-pulse" />
            {liveTool ? `${livePhase} · ${liveTool}` : livePhase}
          </span>
        )}
        <span className="ml-auto shrink-0 text-muted">
          {run.duration_ms ? `${run.duration_ms}ms` : ""} {fmtTime(run.started_unix_ms)}
        </span>
      </button>
      {run.status === "running" && (
        <button
          onClick={(e) => {
            e.stopPropagation();
            void stop();
          }}
          disabled={stopping}
          title="Stop this run"
          aria-label="Stop this run"
          className="shrink-0 px-2 py-1.5 text-muted transition-colors hover:text-bad disabled:opacity-50"
        >
          <CircleStop className={cn("size-4", stopping && "animate-pulse")} />
        </button>
      )}
      </div>
      {open && (
        <div className="px-7 pb-3 pt-1 text-sm">
          <RunDetailLoader correlationId={run.correlation_id} status={run.status} durationMs={run.duration_ms} />
        </div>
      )}
    </div>
  );
}

export function runMatches(r: Run, q: string): boolean {
  if (!q) return true;
  return `${r.intent || ""} ${r.status || ""} ${r.correlation_id || ""}`.toLowerCase().includes(q);
}

export type RunBucket = "running" | "completed" | "failed" | "other";

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

export function runCounts(runs: Run[]): RunCounts {
  const c: RunCounts = { total: runs.length, running: 0, completed: 0, failed: 0, other: 0 };
  for (const r of runs) c[runBucket(r)]++;
  return c;
}

// Run page size used by the cursor pager. We pick 50 — large enough that the
// first page covers what the operator typically wants to scan, small enough that
// reloading on every event-driven refresh doesn't pull a 10 MB JSON body.
export const RUN_PAGE_SIZE = 50;

interface RunsPage {
  runs: Run[];
  /** Opaque "<ms>:<seq>" boundary of the last row we just emitted, or null
   * when the server returned a short page (terminal — no more rows). The
   * pager's `loadMore` passes this back as `cursor` on the next request. */
  next_cursor: string | null;
}

/**
 * useRunsPager is the load-more state machine used by the Runs view. The
 * actual rendering lives in `Runs`; this hook is exported so the load-more
 * behaviour can be unit-tested without rendering the whole page.
 *
 * The hook composes the existing `usePanel` (which already does polling,
 * auth, error retry, and the live-event reload) with a separate `loadMore`
 * that hits /api/runs?cursor=… on demand. The two share the first page —
 * `usePanel` gets us the leading 50 rows; the pager extends from there.
 *
 * Implementation note: this is now a thin wrapper around the generic
 * `useCursorPager` helper (lib/cursorPager) — kept as a named export so
 * existing tests at `@/views/Runs` keep importing it from a stable path.
 */
export function useRunsPager() {
  return useCursorPager<Run>(
    "/api/runs",
    "runs",
    "correlation_id",
    RUN_PAGE_SIZE,
  );
}

export function Runs() {
  const { paged, error, loading, loadMore, loadingMore, moreError, hasMore, reload } = useRunsPager();
  const { events, connected } = useEvents();
  const focus = useRunFocus();
  const counts = runCounts(paged);
  const [q, setQ] = useState("");
  // Per-run live context (current phase/tool/agent) folded from the event stream,
  // shared with the Overseer monitor. Keyed by correlation id.
  const liveCtx = useMemo(() => buildLiveRunContexts(events), [events]);

  // Live: refetch (debounced) when a run's state changes on the event stream, so the
  // list and the metric counters update within ~1s instead of only on manual reload.
  // Open run detail panels keep their state across reloads (RunRow is keyed by id).
  const debounce = useRef<ReturnType<typeof setTimeout> | null>(null);
  const head = events[0]?.kind;
  useEffect(() => {
    if (
      head === "task.received" ||
      head === "task.completed" ||
      head === "task.failed" ||
      head === "schedule.fired"
    ) {
      if (debounce.current) clearTimeout(debounce.current);
      debounce.current = setTimeout(reload, 600);
    }
    return () => {
      if (debounce.current) clearTimeout(debounce.current);
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [head, events[0]?.id]);

  // Slow fallback poll so the list still freshens if the event stream drops.
  useEffect(() => {
    const id = setInterval(reload, 10000);
    return () => clearInterval(id);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const query = q.trim().toLowerCase();

  // Runs in this bucket, ignoring the text query — used to decide whether the filter box shows,
  // so it stays mounted (and keeps focus) even after a query narrows the visible list.
  const bucketRuns = (filter: RunBucket | null) =>
    paged.filter((r) => !filter || runBucket(r) === filter);
  const getShown = (filter: RunBucket | null) =>
    bucketRuns(filter).filter((r) => !query || runMatches(r, query));

  // hasMoreVisibleOnly is true when the pager has more pages AND no client-
  // side filter is narrowing the list. We pass the same pager bits to every
  // RunList so each bucket's footer can drive loadMore. The bucket filter
  // applies client-side over the paged data: the "Load more" pulls another
  // 50 raw runs (mixed statuses) and the bucket selector re-filters for
  // display.
  const pagerProps = {
    hasMore,
    loadingMore,
    moreError,
    onLoadMore: loadMore,
  };

  const tabs = [
    {
      id: "all",
      label: "All",
      icon: ListTree,
      count: counts.total,
      content: <RunList runs={getShown(null)} bucketTotal={bucketRuns(null).length} query={query} q={q} setQ={setQ} focus={focus} error={error} liveCtx={liveCtx} pager={pagerProps} />,
    },
    {
      id: "running",
      label: "Running",
      icon: CircleDot,
      count: counts.running,
      content: <RunList runs={getShown("running")} bucketTotal={bucketRuns("running").length} query={query} q={q} setQ={setQ} focus={focus} error={error} liveCtx={liveCtx} pager={pagerProps} />,
    },
    {
      id: "completed",
      label: "Completed",
      icon: CheckCircle2,
      count: counts.completed,
      content: <RunList runs={getShown("completed")} bucketTotal={bucketRuns("completed").length} query={query} q={q} setQ={setQ} focus={focus} error={error} liveCtx={liveCtx} pager={pagerProps} />,
    },
    {
      id: "failed",
      label: "Failed",
      icon: XOctagon,
      count: counts.failed,
      content: <RunList runs={getShown("failed")} bucketTotal={bucketRuns("failed").length} query={query} q={q} setQ={setQ} focus={focus} error={error} liveCtx={liveCtx} pager={pagerProps} />,
    },
  ];

  return (
    <div className="flex flex-col gap-4">
      <PageHeader
        icon={ListTree}
        title="Runs"
        actions={
          <>
            <span
              className={cn(
                "flex items-center gap-1 rounded-full px-2 py-0.5 text-xs font-medium",
                connected ? "bg-good/10 text-good" : "bg-muted/20 text-muted",
              )}
              title={
                connected
                  ? "Live event stream connected — runs update automatically"
                  : "Stream disconnected — polling every 10s"
              }
            >
              <CircleDot className={cn("size-3", connected && "animate-pulse")} />
              {connected ? "live" : "offline"}
            </span>
            <Button variant="ghost" size="icon" onClick={reload} title="Refresh">
              <RefreshCw className={loading ? "animate-spin" : ""} />
            </Button>
          </>
        }
      />

      <MetricGrid cols="repeat(auto-fill, minmax(140px, 1fr))">
        <MetricWidget
          icon={ListTree}
          label="Total runs"
          value={counts.total}
          tone="muted"
        />
        <MetricWidget
          icon={CircleDot}
          label="Running"
          value={counts.running}
          tone="accent"
          pulse={counts.running > 0}
        />
        <MetricWidget
          icon={CheckCircle2}
          label="Completed"
          value={counts.completed}
          tone="good"
        />
        <MetricWidget
          icon={XOctagon}
          label="Failed"
          value={counts.failed}
          tone={counts.failed > 0 ? "bad" : "muted"}
        />
      </MetricGrid>

      <TabNav tabs={tabs} />
    </div>
  );
}

function RunList({
  runs,
  bucketTotal,
  query,
  q,
  setQ,
  focus,
  error,
  liveCtx,
  pager,
}: {
  runs: Run[];
  bucketTotal: number;
  query: string;
  q: string;
  setQ: (q: string) => void;
  focus?: string | null;
  error?: string | null;
  liveCtx: Record<string, LiveRunContext>;
  pager?: {
    hasMore: boolean;
    loadingMore: boolean;
    moreError?: string | null;
    onLoadMore: () => void;
  };
}) {
  if (error) {
    return <ErrorText>{error}</ErrorText>;
  }

  if (runs.length === 0) {
    // Distinguish an empty bucket from a query that filtered everything out, so the filter
    // box isn't lost (it lives in the panel header, gated on bucketTotal).
    if (query && bucketTotal > 4) {
      return (
        <EmptyState icon={Search} title="No runs match" hint={`Nothing matches “${query}”. Clear the filter to see all ${bucketTotal} runs.`} />
      );
    }
    return (
      <EmptyState
        icon={ListTree}
        title="No runs yet"
        hint="Completed and in-flight runs will be listed here — start one from Chat or the CLI."
      />
    );
  }

  return (
    <section className="overflow-hidden rounded-xl bg-card shadow-e1">
      <div className="flex flex-wrap items-center gap-2 border-b border-border/50 px-3 py-2">
        <span className="grid size-8 shrink-0 place-items-center rounded-lg bg-accent/8 text-accent">
          <Clock className="size-4" />
        </span>
        <div className="min-w-0 flex-1">
          <h3 className="text-sm font-semibold">Run history</h3>
          <div className="text-xs text-muted">{runs.length} visible</div>
        </div>
        {bucketTotal > 4 && (
          <div className="relative">
            <Search className="pointer-events-none absolute left-2 top-1/2 size-3.5 -translate-y-1/2 text-muted" />
            <input
              value={q}
              onChange={(e) => setQ(e.target.value)}
              placeholder="filter runs…"
              aria-label="Filter runs"
              className="h-7 w-48 rounded-md border border-border bg-panel pl-7 pr-8 text-xs text-foreground outline-none focus-glow"
            />
            {query && (
              <span className="absolute right-2 top-1/2 -translate-y-1/2 text-xs text-muted">
                {runs.length}
              </span>
            )}
          </div>
        )}
      </div>
      {runs.map((r, i) => (
        <RunRow key={r.correlation_id || i} run={r} focus={focus} ctx={liveCtx[r.correlation_id || ""]} />
      ))}
      {/* Pager footer (M-pending): load 50 more runs at a time. We only show the
          footer when the page count is > RUN_PAGE_SIZE worth of content visible
          — a stable terminal page hides it. The `pager` prop is optional so the
          test suite can render RunList without a real hook. */}
      {pager && (
        <div className="flex flex-col items-center gap-1 border-t border-border/50 px-3 py-3">
          {pager.hasMore ? (
            <>
              <button
                onClick={pager.onLoadMore}
                disabled={pager.loadingMore}
                aria-busy={pager.loadingMore || undefined}
                className="inline-flex items-center gap-2 rounded-md border border-border bg-card px-3 py-1.5 text-xs text-foreground hover:border-accent disabled:opacity-50"
              >
                {pager.loadingMore ? (
                  <>
                    <Loader2 className="size-3 animate-spin" /> loading…
                  </>
                ) : (
                  <>Load 50 more</>
                )}
              </button>
              {pager.moreError && (
                <p className="text-[11px] text-bad" role="alert">
                  couldn't load more: {pager.moreError}
                </p>
              )}
            </>
          ) : (
            <p className="text-xs text-muted">— end of run history —</p>
          )}
        </div>
      )}
    </section>
  );
}
