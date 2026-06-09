import { useState } from "react";
import { RefreshCw, ChevronRight, ChevronDown, ListTree } from "lucide-react";
import { usePanel } from "@/lib/usePanel";
import { Card, CardHeader, CardTitle, CardBody } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Badge, statusVariant } from "@/components/ui/badge";
import { ErrorText } from "@/components/JsonView";
import { EmptyState } from "@/components/ui/empty";
import { fmtTime } from "@/lib/utils";
import { RunDetailLoader } from "@/components/RunDetail";

interface Run {
  correlation_id?: string;
  status?: string;
  intent?: string;
  duration_ms?: number;
  started_unix_ms?: number;
}

function RunRow({ run }: { run: Run }) {
  const [open, setOpen] = useState(false);
  return (
    <div className="border-b border-border/60">
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

export function Runs() {
  const { data, error, loading, reload } = usePanel<{ runs?: Run[] }>("/api/runs");
  const runs = data?.runs || [];
  return (
    <Card className="h-full">
      <CardHeader>
        <CardTitle>Runs</CardTitle>
        <Button variant="ghost" size="icon" className="ml-auto" onClick={reload} title="Refresh">
          <RefreshCw className={loading ? "animate-spin" : ""} />
        </Button>
      </CardHeader>
      <CardBody>
        {error ? (
          <ErrorText>{error}</ErrorText>
        ) : runs.length ? (
          runs.map((r, i) => <RunRow key={r.correlation_id || i} run={r} />)
        ) : (
          <EmptyState icon={ListTree} title="No runs yet" hint="Completed and in-flight runs will be listed here — start one from Chat or the CLI." />
        )}
      </CardBody>
    </Card>
  );
}
