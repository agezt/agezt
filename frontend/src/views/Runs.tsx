import { useState } from "react";
import { RefreshCw, ChevronRight, ChevronDown } from "lucide-react";
import { usePanel } from "@/lib/usePanel";
import { getJSON } from "@/lib/api";
import { Card, CardHeader, CardTitle, CardBody } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Badge, statusVariant } from "@/components/ui/badge";
import { Muted, ErrorText } from "@/components/JsonView";
import { fmtTime } from "@/lib/utils";
import type { AgentEvent } from "@/lib/events";

interface Run {
  correlation_id?: string;
  status?: string;
  intent?: string;
  duration_ms?: number;
  started_unix_ms?: number;
}

function RunRow({ run }: { run: Run }) {
  const [open, setOpen] = useState(false);
  const [arc, setArc] = useState<AgentEvent[] | null>(null);
  const [err, setErr] = useState<string | null>(null);

  async function toggle() {
    const next = !open;
    setOpen(next);
    if (next && !arc && run.correlation_id) {
      try {
        const d = await getJSON<{ events?: AgentEvent[] }>("/api/journal", {
          correlation_id: run.correlation_id,
          limit: "200",
        });
        setArc(d.events || []);
      } catch (e) {
        setErr((e as Error).message);
      }
    }
  }

  return (
    <div className="border-b border-border/60">
      <button
        onClick={toggle}
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
        <div className="px-7 pb-2 text-xs">
          {err ? (
            <ErrorText>{err}</ErrorText>
          ) : arc ? (
            arc.length ? (
              <ul className="space-y-0.5">
                {arc.map((e, i) => (
                  <li key={e.id || i} className="flex gap-2">
                    <span className="text-muted">{fmtTime(e.ts_unix_ms)}</span>
                    <span className="text-accent">{e.kind}</span>
                    <span className="truncate text-muted">{e.subject}</span>
                  </li>
                ))}
              </ul>
            ) : (
              <Muted>no events</Muted>
            )
          ) : (
            <Muted>loading…</Muted>
          )}
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
          <Muted>no runs yet</Muted>
        )}
      </CardBody>
    </Card>
  );
}
