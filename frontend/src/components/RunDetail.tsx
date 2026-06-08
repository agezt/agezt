import { useEffect, useRef, useState } from "react";
import { ChevronRight, ChevronDown, Wrench, ShieldCheck, ShieldX } from "lucide-react";
import { getJSON } from "@/lib/api";
import { Badge, statusVariant } from "@/components/ui/badge";
import { KeyValue, Muted, ErrorText } from "@/components/JsonView";
import { fmtTime, clip } from "@/lib/utils";
import { money } from "@/lib/format";
import { deriveDetail, num, mergeEvents, type ToolCall } from "@/lib/rundetail";
import { useEvents, type AgentEvent } from "@/lib/events";

// One governed tool call, rendered with its policy verdict and (clipped) output.
export function ToolCallRow({ c }: { c: ToolCall }) {
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

// RunDetailCards folds a run's journaled event arc into the human summary —
// status / model / tokens / cost, the tool calls it made, and its final answer.
// Pure presentation over deriveDetail; shared by the Runs view and the live
// Activity monitor so they can never disagree about how a run reads.
export function RunDetailCards({
  arc,
  status: fallbackStatus,
  durationMs,
}: {
  arc: AgentEvent[];
  status?: string;
  durationMs?: number;
}) {
  const d = deriveDetail(arc);
  const status = d.status || fallbackStatus;
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
          ["duration", durationMs ? `${durationMs}ms` : <Muted>—</Muted>],
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

// RunDetailLoader fetches a run's journaled arc once (the first time it mounts)
// and folds the live event stream into it so the detail updates as the agent
// works — the same live pattern Flow Studio uses. It renders RunDetailCards plus
// a collapsible raw-events list. Used wherever a run can be drilled into (the
// Runs history list, the live Activity monitor).
export function RunDetailLoader({
  correlationId,
  status,
  durationMs,
}: {
  correlationId?: string;
  status?: string;
  durationMs?: number;
}) {
  const { subscribe } = useEvents();
  const [arc, setArc] = useState<AgentEvent[] | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [rawOpen, setRawOpen] = useState(false);
  const fetched = useRef(false);

  // Fetch the journaled snapshot once. Merge (not overwrite) so any live events
  // that arrived before the fetch resolved are kept.
  useEffect(() => {
    if (fetched.current || !correlationId) return;
    fetched.current = true;
    getJSON<{ events?: AgentEvent[] }>("/api/journal", { correlation_id: correlationId, limit: "500" })
      .then((dat) => setArc((prev) => mergeEvents(prev || [], dat.events || [])))
      .catch((e) => setErr((e as Error).message));
  }, [correlationId]);

  // Fold the live journal stream for this correlation into the arc.
  useEffect(() => {
    if (!correlationId) return;
    return subscribe((e: AgentEvent) => {
      if (e.correlation_id !== correlationId) return;
      setArc((prev) => mergeEvents(prev || [], [e]));
    });
  }, [correlationId, subscribe]);

  if (err) return <ErrorText>{err}</ErrorText>;
  if (!arc) return <Muted>loading…</Muted>;
  return (
    <>
      <RunDetailCards arc={arc} status={status} durationMs={durationMs} />
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
  );
}
