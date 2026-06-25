import { useEffect, useRef, useState } from "react";
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
} from "lucide-react";
import { usePanel } from "@/lib/usePanel";
import { Button } from "@/components/ui/button";
import { PageHeader } from "@/components/ui/page-header";
import { Badge, statusVariant } from "@/components/ui/badge";
import { ErrorText } from "@/components/JsonView";
import { EmptyState } from "@/components/ui/empty";
import { cn, fmtTime } from "@/lib/utils";
import { RunDetailLoader } from "@/components/RunDetail";
import { useRunFocus, clearRunFocus } from "@/lib/runfocus";
import { TabNav } from "@/components/ui/tab-nav";
import { CollapsibleSection } from "@/components/ui/collapsible-section";
import { MetricWidget, MetricGrid } from "@/components/ui/metric-widget";

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

export function Runs() {
  const { data, error, loading, reload } = usePanel<{ runs?: Run[] }>("/api/runs");
  const runs = data?.runs || [];
  const focus = useRunFocus();
  const counts = runCounts(runs);
  const [q, setQ] = useState("");

  useEffect(() => {
    if (!focus) return;
    const target = runs.find((r) => r.correlation_id === focus);
  }, [focus, runs]);

  const query = q.trim().toLowerCase();

  // Runs in this bucket, ignoring the text query — used to decide whether the filter box shows,
  // so it stays mounted (and keeps focus) even after a query narrows the visible list.
  const bucketRuns = (filter: RunBucket | null) => runs.filter((r) => !filter || runBucket(r) === filter);
  const getShown = (filter: RunBucket | null) =>
    bucketRuns(filter).filter((r) => !query || runMatches(r, query));

  const tabs = [
    {
      id: "all",
      label: "All",
      icon: ListTree,
      count: counts.total,
      content: <RunList runs={getShown(null)} bucketTotal={bucketRuns(null).length} query={query} q={q} setQ={setQ} focus={focus} error={error} />,
    },
    {
      id: "running",
      label: "Running",
      icon: CircleDot,
      count: counts.running,
      content: <RunList runs={getShown("running")} bucketTotal={bucketRuns("running").length} query={query} q={q} setQ={setQ} focus={focus} error={error} />,
    },
    {
      id: "completed",
      label: "Completed",
      icon: CheckCircle2,
      count: counts.completed,
      content: <RunList runs={getShown("completed")} bucketTotal={bucketRuns("completed").length} query={query} q={q} setQ={setQ} focus={focus} error={error} />,
    },
    {
      id: "failed",
      label: "Failed",
      icon: XOctagon,
      count: counts.failed,
      content: <RunList runs={getShown("failed")} bucketTotal={bucketRuns("failed").length} query={query} q={q} setQ={setQ} focus={focus} error={error} />,
    },
  ];

  return (
    <div className="flex flex-col gap-4">
      <PageHeader
        icon={ListTree}
        title="Runs"
        actions={
          <>
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
}: {
  runs: Run[];
  bucketTotal: number;
  query: string;
  q: string;
  setQ: (q: string) => void;
  focus?: string | null;
  error?: string | null;
}) {
  if (error) {
    return <ErrorText>{error}</ErrorText>;
  }

  if (runs.length === 0) {
    // Distinguish an empty bucket from a query that filtered everything out, so the filter
    // box isn't lost (it lives in the CollapsibleSection actions, gated on bucketTotal).
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
    <CollapsibleSection
      icon={Clock}
      title="Run history"
      count={runs.length}
      tone="muted"
      defaultOpen={true}
      actions={
        bucketTotal > 4 ? (
          <div className="relative">
            <Search className="pointer-events-none absolute left-2 top-1/2 size-3.5 -translate-y-1/2 text-muted" />
            <input
              value={q}
              onChange={(e) => setQ(e.target.value)}
              placeholder="filter runs…"
              aria-label="Filter runs"
              className="h-7 w-48 rounded-md border border-border bg-panel pl-7 pr-8 text-xs text-foreground outline-none focus-visible:border-accent"
            />
            {query && (
              <span className="absolute right-2 top-1/2 -translate-y-1/2 text-xs text-muted">
                {runs.length}
              </span>
            )}
          </div>
        ) : undefined
      }
    >
      {runs.map((r, i) => (
        <RunRow key={r.correlation_id || i} run={r} focus={focus} />
      ))}
    </CollapsibleSection>
  );
}
