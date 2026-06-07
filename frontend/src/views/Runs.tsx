import { useState } from "react";
import { RefreshCw, ChevronRight, ChevronDown, Wrench, ShieldCheck, ShieldX } from "lucide-react";
import { usePanel } from "@/lib/usePanel";
import { getJSON } from "@/lib/api";
import { Card, CardHeader, CardTitle, CardBody } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Badge, statusVariant } from "@/components/ui/badge";
import { KeyValue, Muted, ErrorText } from "@/components/JsonView";
import { fmtTime, clip } from "@/lib/utils";
import { money } from "@/lib/format";
import { deriveDetail, num, type ToolCall } from "@/lib/rundetail";
import type { AgentEvent } from "@/lib/events";

interface Run {
  correlation_id?: string;
  status?: string;
  intent?: string;
  duration_ms?: number;
  started_unix_ms?: number;
}

function ToolCallRow({ c }: { c: ToolCall }) {
  return (
    <li className="flex items-center gap-2 py-0.5">
      <Wrench className="size-3.5 shrink-0 text-muted" />
      <span className="font-medium">{c.tool || "tool"}</span>
      {c.capability && <Badge variant="accent">{c.capability}</Badge>}
      {c.allow === false ? (
        <Badge variant="bad">
          <ShieldX className="mr-1 size-3" />
          {c.hardDenied ? "hard-denied" : "denied"}
        </Badge>
      ) : (
        <Badge variant="good">
          <ShieldCheck className="mr-1 size-3" />
          allowed
        </Badge>
      )}
      {c.error && <Badge variant="bad">error</Badge>}
      {c.output && <span className="truncate text-muted">{clip(c.output, 100)}</span>}
    </li>
  );
}

function RunDetailCards({ arc, run }: { arc: AgentEvent[]; run: Run }) {
  const d = deriveDetail(arc);
  const status = d.status || run.status;
  return (
    <div className="space-y-3">
      <KeyValue
        pairs={[
          ["status", <Badge variant={statusVariant(status)}>{status || "?"}</Badge>],
          ["model", d.model || <Muted>—</Muted>],
          ["iterations", d.iterations || <Muted>—</Muted>],
          [
            "tokens",
            d.hasBudget ? (
              <span>
                {d.inputTokens.toLocaleString()} in / {d.outputTokens.toLocaleString()} out
                {d.cachedTokens ? ` (${d.cachedTokens.toLocaleString()} cached)` : ""}
              </span>
            ) : (
              <Muted>—</Muted>
            ),
          ],
          ["cost", d.hasBudget ? money(d.costMicrocents) : <Muted>—</Muted>],
          ["duration", run.duration_ms ? `${run.duration_ms}ms` : <Muted>—</Muted>],
        ]}
      />

      <div>
        <div className="mb-1 text-xs font-semibold uppercase tracking-wide text-muted">
          Tool calls ({d.toolCalls.length})
        </div>
        {d.toolCalls.length ? (
          <ul>
            {d.toolCalls.map((c, i) => (
              <ToolCallRow key={c.callId || i} c={c} />
            ))}
          </ul>
        ) : (
          <Muted>no tool calls</Muted>
        )}
      </div>

      {d.answer && (
        <div>
          <div className="mb-1 text-xs font-semibold uppercase tracking-wide text-muted">
            {status === "failed" ? "Error" : "Final answer"}
          </div>
          <p className="whitespace-pre-wrap break-words rounded-md border border-border bg-panel p-2 text-xs">
            {clip(d.answer, 600)}
          </p>
        </div>
      )}
    </div>
  );
}

function RunRow({ run }: { run: Run }) {
  const [open, setOpen] = useState(false);
  const [arc, setArc] = useState<AgentEvent[] | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [rawOpen, setRawOpen] = useState(false);

  async function toggle() {
    const next = !open;
    setOpen(next);
    if (next && !arc && run.correlation_id) {
      try {
        const dat = await getJSON<{ events?: AgentEvent[] }>("/api/journal", {
          correlation_id: run.correlation_id,
          limit: "500",
        });
        setArc(dat.events || []);
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
        <div className="px-7 pb-3 pt-1 text-sm">
          {err ? (
            <ErrorText>{err}</ErrorText>
          ) : arc ? (
            <>
              <RunDetailCards arc={arc} run={run} />
              <button
                onClick={() => setRawOpen((v) => !v)}
                className="mt-2 flex items-center gap-1 text-xs text-muted hover:text-foreground"
              >
                {rawOpen ? <ChevronDown className="size-3" /> : <ChevronRight className="size-3" />}
                raw events ({arc.length})
              </button>
              {rawOpen &&
                (arc.length ? (
                  <ul className="mt-1 space-y-0.5 text-xs">
                    {[...arc]
                      .sort((a, b) => num(a.seq) - num(b.seq))
                      .map((e, i) => (
                        <li key={e.id || i} className="flex gap-2">
                          <span className="text-muted">{fmtTime(e.ts_unix_ms)}</span>
                          <span className="text-accent">{e.kind}</span>
                          <span className="truncate text-muted">{e.subject}</span>
                        </li>
                      ))}
                  </ul>
                ) : (
                  <Muted>no events</Muted>
                ))}
            </>
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
