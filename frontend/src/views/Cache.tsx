import { useEffect, useState } from "react";
import { Database, RefreshCw, PiggyBank, Download, Upload, Hash } from "lucide-react";
import { getJSON } from "@/lib/api";
import { useEvents } from "@/lib/events";
import { money } from "@/lib/format";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { ErrorText } from "@/components/JsonView";
import { SkeletonList } from "@/components/ui/skeleton";
import { Page } from "@/components/ui/page";
import { EmptyState } from "@/components/ui/empty";
import { MetricWidget, MetricGrid } from "@/components/ui/metric-widget";
import { Ring } from "@/components/Widgets";

interface CacheData {
  cached_input_tokens?: number;
  cache_write_input_tokens?: number;
  saved_microcents?: number;
  calls?: number;
}

// Cache is the prompt-cache savings monitor: how much the cache has saved, the
// read vs write token split, and the priced-call count it covers.
export function Cache() {
  const { events } = useEvents();
  const [d, setD] = useState<CacheData | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);

  async function reload() {
    setLoading(true);
    try {
      setD(await getJSON<CacheData>("/api/cache"));
      setErr(null);
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setLoading(false);
    }
  }
  useEffect(() => {
    reload();
    const id = setInterval(reload, 8000);
    return () => clearInterval(id);
  }, []);

  const head = events[0]?.kind;
  useEffect(() => {
    if (head === "budget.consumed" || head === "task.completed") reload();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [head, events[0]?.id]);

  const reads = d?.cached_input_tokens ?? 0;
  const writes = d?.cache_write_input_tokens ?? 0;
  const total = reads + writes;

  return (
    <Page
      icon={Database}
      title="Prompt cache"
      description="Prompt-cache savings, read vs write token split, and priced-call count."
      width="readable"
      mode="scroll"
      actions={
        <Button variant="ghost" size="sm" onClick={reload} disabled={loading}>
          <RefreshCw className={cn("size-3.5", loading && "animate-spin")} /> Refresh
        </Button>
      }
    >
      {err ? (
        <ErrorText>{err}</ErrorText>
      ) : !d ? (
        <SkeletonList count={3} lines={1} />
      ) : (
        <>
          {/* Savings hero */}
          <div className="rounded-lg border border-good/40 bg-good/5 p-4">
            <div className="flex items-center gap-2 text-xs text-muted">
              <PiggyBank className="size-4 text-good" /> saved by prompt caching
            </div>
            <div className="mt-1 text-3xl font-semibold tabular-nums text-good">{money(d.saved_microcents)}</div>
            <div className="mt-1 text-xs text-muted">across {(d.calls ?? 0).toLocaleString()} priced call(s)</div>
          </div>

          {/* Token tiles */}
          <MetricGrid cols="repeat(auto-fill, minmax(160px, 1fr))">
            <MetricWidget icon={Download} label="cache reads" value={`${reads.toLocaleString()} tok`} tone="good" />
            <MetricWidget icon={Upload} label="cache writes" value={`${writes.toLocaleString()} tok`} tone="accent" />
            <MetricWidget icon={Hash} label="priced calls" value={(d.calls ?? 0).toLocaleString()} tone="muted" />
          </MetricGrid>

          {/* Read vs write split, with a read-share gauge */}
          {total > 0 ? (
            <div className="flex flex-col items-center gap-4 glass rounded-xl p-3 sm:flex-row">
              <Ring
                pct={(reads / total) * 100}
                center={`${Math.round((reads / total) * 100)}%`}
                label="cache reads"
                tone="good"
              />
              <div className="min-w-0 flex-1">
                <div className="mb-2 text-xs font-semibold uppercase tracking-normal text-muted">Cache token split</div>
                <div className="flex h-2.5 overflow-hidden rounded-full bg-panel">
                  <div className="h-full bg-good" style={{ width: `${(reads / total) * 100}%` }} title={`reads: ${reads}`} />
                  <div className="h-full bg-accent" style={{ width: `${(writes / total) * 100}%` }} title={`writes: ${writes}`} />
                </div>
                <div className="mt-1.5 flex gap-4 text-xs text-muted">
                  <span className="inline-flex items-center gap-1">
                    <span className="size-2 rounded-full bg-good" /> reads {reads.toLocaleString()} ({Math.round((reads / total) * 100)}%)
                  </span>
                  <span className="inline-flex items-center gap-1">
                    <span className="size-2 rounded-full bg-accent" /> writes {writes.toLocaleString()} ({Math.round((writes / total) * 100)}%)
                  </span>
                </div>
              </div>
            </div>
          ) : (
            <EmptyState
              icon={Database}
              title="No prompt-cache usage recorded yet"
              hint="Cache reads and writes appear here once priced calls start hitting the prompt cache."
            />
          )}
        </>
      )}
    </Page>
  );
}
