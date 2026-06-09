import { useEffect, useRef, useState } from "react";
import { RefreshCw, ChevronRight, ChevronDown, ListTree, Search } from "lucide-react";
import { usePanel } from "@/lib/usePanel";
import { Card, CardHeader, CardTitle, CardBody } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
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

export function Runs() {
  const { data, error, loading, reload } = usePanel<{ runs?: Run[] }>("/api/runs");
  const runs = data?.runs || [];
  const focus = useRunFocus();
  const [q, setQ] = useState("");
  const query = q.trim().toLowerCase();
  const shown = query ? runs.filter((r) => runMatches(r, query)) : runs;
  return (
    <Card className="h-full">
      <CardHeader>
        <CardTitle>Runs</CardTitle>
        <Button variant="ghost" size="icon" className="ml-auto" onClick={reload} title="Refresh">
          <RefreshCw className={loading ? "animate-spin" : ""} />
        </Button>
      </CardHeader>
      <CardBody>
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
          <p className="px-1 py-2 text-xs text-muted">no runs match “{q.trim()}”</p>
        ) : (
          shown.map((r, i) => <RunRow key={r.correlation_id || i} run={r} focus={focus} />)
        )}
      </CardBody>
    </Card>
  );
}
