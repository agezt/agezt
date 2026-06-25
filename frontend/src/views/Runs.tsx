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
  const [bucket, setBucket] = useState<RunBucket | null>(null);

  useEffect(() => {
    if (!focus) return;
    const target = runs.find((r) => r.correlation_id === focus);
  }, [focus, runs]);

  const query = q.trim().toLowerCase();

  const shown = runs.filter(
    (r) => (!bucket || runBucket(r) === bucket) && (!query || runMatches(r, query)),
  );

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

      <RunBucketFilters counts={counts} bucket={bucket} setBucket={setBucket} />
      <RunList runs={shown} total={runs.length} query={query} q={q} setQ={setQ} focus={focus} error={error} />
    </div>
  );
}

function RunBucketFilters({
  counts,
  bucket,
  setBucket,
}: {
  counts: RunCounts;
  bucket: RunBucket | null;
  setBucket: (bucket: RunBucket | null) => void;
}) {
  const filters: Array<{ id: RunBucket | null; label: string; icon: typeof ListTree; count: number }> = [
    { id: null, label: "all", icon: ListTree, count: counts.total },
    { id: "running", label: "running", icon: CircleDot, count: counts.running },
    { id: "completed", label: "completed", icon: CheckCircle2, count: counts.completed },
    { id: "failed", label: "failed", icon: XOctagon, count: counts.failed },
    { id: "other", label: "other", icon: Clock, count: counts.other },
  ];
  return (
    <div className="flex flex-wrap items-center gap-1 rounded-xl border border-border bg-panel/50 p-1" aria-label="Run status filters">
      {filters.map((f) => {
        const Icon = f.icon;
        const active = bucket === f.id;
        return (
          <button
            key={f.label}
            type="button"
            aria-pressed={active}
            aria-label={`${f.label} ${f.count}`}
            disabled={f.count === 0}
            onClick={() => setBucket(active ? null : f.id)}
            className={cn(
              "inline-flex items-center gap-1.5 rounded-lg px-3 py-1.5 text-xs font-medium transition-all duration-150 outline-none focus-visible:ring-2 focus-visible:ring-accent/50 disabled:cursor-not-allowed disabled:opacity-45",
              active ? "bg-accent/15 text-accent shadow-sm" : "text-muted hover:text-foreground",
            )}
          >
            <Icon className="size-3.5" aria-hidden />
            {f.label}
            <span className={cn("inline-flex min-w-4 items-center justify-center rounded-full px-1 text-[10px] tabular-nums", active ? "bg-accent/20 text-accent" : "bg-panel text-muted")}>
              {f.count}
            </span>
          </button>
        );
      })}
    </div>
  );
}

function RunList({
  runs,
  total,
  query,
  q,
  setQ,
  focus,
  error,
}: {
  runs: Run[];
  total: number;
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
    return (
      <EmptyState
        icon={ListTree}
        title={total === 0 ? "No runs yet" : "no runs match"}
        hint={total === 0 ? "Completed and in-flight runs will be listed here — start one from Chat or the CLI." : "Clear the filters to see the full run history."}
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
        total > 4 ? (
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
                {runs.length}/{total}
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
