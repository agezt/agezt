import { useEffect, useState } from "react";
import { Database, RefreshCw, PiggyBank, Download, Upload, Hash } from "lucide-react";
import { getJSON } from "@/lib/api";
import { useEvents } from "@/lib/events";
import { money } from "@/lib/format";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { Muted, ErrorText } from "@/components/JsonView";

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
    <div className="space-y-4">
      <div className="flex items-center gap-2">
        <h2 className="flex items-center gap-2 text-sm font-semibold">
          <Database className="size-4 text-accent" /> Prompt cache
        </h2>
        <Button variant="ghost" size="sm" className="ml-auto" onClick={reload} disabled={loading}>
          <RefreshCw className={cn("size-3.5", loading && "animate-spin")} /> Refresh
        </Button>
      </div>

      {err ? (
        <ErrorText>{err}</ErrorText>
      ) : !d ? (
        <Muted>loading…</Muted>
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
          <div className="grid grid-cols-2 gap-2 sm:grid-cols-3">
            <Tile icon={Download} label="cache reads" value={`${reads.toLocaleString()} tok`} />
            <Tile icon={Upload} label="cache writes" value={`${writes.toLocaleString()} tok`} />
            <Tile icon={Hash} label="priced calls" value={(d.calls ?? 0).toLocaleString()} />
          </div>

          {/* Read vs write split */}
          {total > 0 ? (
            <div className="rounded-lg border border-border bg-card p-3">
              <div className="mb-2 text-xs font-semibold uppercase tracking-wider text-muted">Cache token split</div>
              <div className="flex h-2.5 overflow-hidden rounded-full bg-panel">
                <div className="h-full bg-good" style={{ width: `${(reads / total) * 100}%` }} title={`reads: ${reads}`} />
                <div className="h-full bg-accent" style={{ width: `${(writes / total) * 100}%` }} title={`writes: ${writes}`} />
              </div>
              <div className="mt-1.5 flex gap-4 text-xs text-muted">
                <span className="inline-flex items-center gap-1">
                  <span className="size-2 rounded-full bg-good" /> reads {Math.round((reads / total) * 100)}%
                </span>
                <span className="inline-flex items-center gap-1">
                  <span className="size-2 rounded-full bg-accent" /> writes {Math.round((writes / total) * 100)}%
                </span>
              </div>
            </div>
          ) : (
            <Muted>no prompt-cache usage recorded yet</Muted>
          )}
        </>
      )}
    </div>
  );
}

function Tile({ icon: Icon, label, value }: { icon: typeof Database; label: string; value: string }) {
  return (
    <div className="rounded-lg border border-border bg-card px-3 py-2.5">
      <div className="flex items-center gap-1.5 text-xs text-muted">
        <Icon className="size-3.5" /> {label}
      </div>
      <div className="mt-1 text-lg font-semibold tabular-nums">{value}</div>
    </div>
  );
}
