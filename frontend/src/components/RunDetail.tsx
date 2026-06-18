import { useEffect, useMemo, useRef, useState } from "react";
import {
  ChevronRight,
  ChevronDown,
  Wrench,
  ShieldCheck,
  ShieldX,
  Pause,
  Play,
  StepForward,
  Navigation,
  Send,
  OctagonX,
} from "lucide-react";
import { getJSON, postAction } from "@/lib/api";
import { Badge, statusVariant } from "@/components/ui/badge";
import { KeyValue, Muted, ErrorText } from "@/components/JsonView";
import { ToolOutput } from "@/components/DataView";
import { SkeletonList } from "@/components/ui/skeleton";
import { cn } from "@/lib/utils";
import { fmtTime, clip } from "@/lib/utils";
import { money } from "@/lib/format";
import { deriveDetail, num, mergeEvents, type ToolCall } from "@/lib/rundetail";
import { useEvents, type AgentEvent } from "@/lib/events";
import { IncidentBadges } from "@/components/IncidentBadges";
import {
  incidentBadgeItem,
  incidentEventSummary,
  isIncidentFamilyEvent,
} from "@/lib/incidentevents";

interface RunPhaseStep {
  key: string;
  ts?: number;
  kind: string;
  phase: string;
  detail?: string;
}

function payloadString(e: AgentEvent, key: string): string {
  const v = e.payload?.[key];
  return typeof v === "string" ? v : "";
}

function payloadNumber(e: AgentEvent, key: string): number | undefined {
  const v = e.payload?.[key];
  return typeof v === "number" ? v : undefined;
}

function wakeDetail(e: AgentEvent): string {
  const standing = payloadString(e, "standing_name") || payloadString(e, "standing_id");
  return [
    payloadString(e, "intent") ? clip(payloadString(e, "intent"), 100) : "",
    payloadString(e, "wake_source") ? `source: ${payloadString(e, "wake_source")}` : "",
    payloadString(e, "wake_reason") ? `reason: ${payloadString(e, "wake_reason")}` : "",
    payloadString(e, "schedule_id") ? `schedule: ${payloadString(e, "schedule_id")}` : "",
    standing ? `standing: ${standing}` : "",
    payloadString(e, "trigger_subject") ? `trigger: ${payloadString(e, "trigger_subject")}` : "",
    payloadString(e, "parent_correlation") ? `parent: ${payloadString(e, "parent_correlation")}` : "",
  ].filter(Boolean).join(" · ");
}

function retryDetail(e: AgentEvent): string {
  const nextAttempt = payloadNumber(e, "next_attempt");
  const maxAttempts = payloadNumber(e, "max_attempts");
  const delayMs = payloadNumber(e, "delay_ms");
  const backoff = payloadString(e, "backoff");
  const retryOn = Array.isArray(e.payload?.retry_on)
    ? e.payload.retry_on.map((v: unknown) => String(v).trim()).filter(Boolean)
    : [];
  return [
    nextAttempt != null && maxAttempts != null ? `attempt ${nextAttempt}/${maxAttempts}` : "",
    delayMs && delayMs > 0 ? `wait ${formatRetryDelay(delayMs)}` : "",
    backoff ? `backoff ${backoff}` : "",
    retryOn.length > 0 ? `retry_on ${retryOn.join(",")}` : "",
    clip(payloadString(e, "reason") || payloadString(e, "error"), 100),
  ].filter(Boolean).join(" · ");
}

function formatRetryDelay(ms: number): string {
  if (ms < 1000) return `${ms}ms`;
  const sec = Math.round(ms / 1000);
  if (sec < 60) return `${sec}s`;
  const min = Math.floor(sec / 60);
  const rem = sec % 60;
  return rem > 0 ? `${min}m ${rem}s` : `${min}m`;
}

export function runPhaseSteps(arc: AgentEvent[]): RunPhaseStep[] {
  return [...arc]
    .sort((a, b) => num(a.seq) - num(b.seq))
    .map((e, i): RunPhaseStep | null => {
      const iter = payloadNumber(e, "iter");
      const iterLabel = iter != null ? `iter ${iter}` : "";
      switch (e.kind) {
        case "task.received":
          return {
            key: `${e.seq || i}:received`,
            ts: e.ts_unix_ms,
            kind: e.kind,
            phase: "started",
            detail: wakeDetail(e),
          };
        case "llm.request":
          return {
            key: `${e.seq || i}:llm-request`,
            ts: e.ts_unix_ms,
            kind: e.kind,
            phase: "thinking",
            detail: [iterLabel, payloadString(e, "model")].filter(Boolean).join(" · "),
          };
        case "llm.response":
          return {
            key: `${e.seq || i}:llm-response`,
            ts: e.ts_unix_ms,
            kind: e.kind,
            phase: payloadNumber(e, "tool_calls") ? "planned tools" : "answered",
            detail: [iterLabel, payloadString(e, "stop_reason")].filter(Boolean).join(" · "),
          };
        case "tool.invoked":
          return {
            key: `${e.seq || i}:tool-invoked`,
            ts: e.ts_unix_ms,
            kind: e.kind,
            phase: "using tool",
            detail: [iterLabel, payloadString(e, "tool") || payloadString(e, "name")].filter(Boolean).join(" · "),
          };
        case "tool.result":
          return {
            key: `${e.seq || i}:tool-result`,
            ts: e.ts_unix_ms,
            kind: e.kind,
            phase: e.payload?.error ? "tool error" : "observed tool",
            detail: [iterLabel, payloadString(e, "tool") || payloadString(e, "name")].filter(Boolean).join(" · "),
          };
        case "agent.retry":
        case "provider.retry":
          return {
            key: `${e.seq || i}:retry`,
            ts: e.ts_unix_ms,
            kind: e.kind,
            phase: "retrying",
            detail: retryDetail(e),
          };
        case "task.continued":
          return { key: `${e.seq || i}:continued`, ts: e.ts_unix_ms, kind: e.kind, phase: "continuing", detail: iterLabel };
        case "run.paused":
        case "run.stepped":
          return { key: `${e.seq || i}:paused`, ts: e.ts_unix_ms, kind: e.kind, phase: "paused", detail: iterLabel };
        case "run.resumed":
          return { key: `${e.seq || i}:resumed`, ts: e.ts_unix_ms, kind: e.kind, phase: "resumed", detail: iterLabel };
        case "run.steered":
          return {
            key: `${e.seq || i}:steered`,
            ts: e.ts_unix_ms,
            kind: e.kind,
            phase: "steered",
            detail: clip(payloadString(e, "directive"), 100),
          };
        case "task.completed":
          return { key: `${e.seq || i}:completed`, ts: e.ts_unix_ms, kind: e.kind, phase: "completed" };
        case "task.failed":
          return {
            key: `${e.seq || i}:failed`,
            ts: e.ts_unix_ms,
            kind: e.kind,
            phase: "failed",
            detail: clip(payloadString(e, "reason") || payloadString(e, "error"), 100),
          };
      }
      return null;
    })
    .filter((s): s is RunPhaseStep => !!s);
}

export function RunPhaseTimeline({ arc }: { arc: AgentEvent[] }) {
  const steps = runPhaseSteps(arc);
  if (!steps.length) return null;
  const latest = steps[steps.length - 1];
  return (
    <div className="rounded-lg border border-border bg-panel/50 p-2.5">
      <div className="mb-2 flex flex-wrap items-center gap-2 text-xs">
        <span className="font-semibold uppercase tracking-wider text-muted">phase timeline</span>
        <Badge variant={latest.phase === "failed" ? "bad" : latest.phase === "completed" ? "good" : "accent"}>
          {latest.phase}
        </Badge>
        {latest.detail && <span className="truncate text-muted">{latest.detail}</span>}
      </div>
      <ol className="space-y-1">
        {steps.slice(-8).map((s) => (
          <li key={s.key} className="flex items-start gap-2 text-xs">
            <span className="w-16 shrink-0 font-mono text-[10px] text-muted">{fmtTime(s.ts)}</span>
            <span className="w-24 shrink-0 text-foreground/85">{s.phase}</span>
            <span className="min-w-0 flex-1 truncate text-muted">{s.detail || s.kind}</span>
          </li>
        ))}
      </ol>
    </div>
  );
}

// One governed tool call, rendered with its policy verdict. Expandable to show
// the full arguments it was called with and the result it returned — so EVERY
// run is inspectable here, including autonomous ones (scheduled / standing /
// board-triggered) that never appear in the Chat view. Mirrors the Chat ToolChip.
export function ToolCallRow({ c }: { c: ToolCall }) {
  const [open, setOpen] = useState(false);
  const hasDetail = !!c.input || !!c.output;
  return (
    <li className="py-0.5">
      <button
        onClick={() => hasDetail && setOpen((o) => !o)}
        className={cn("flex w-full items-center gap-2 text-left", !hasDetail && "cursor-default")}
      >
        {hasDetail ? (
          open ? (
            <ChevronDown className="size-3.5 shrink-0 text-muted" />
          ) : (
            <ChevronRight className="size-3.5 shrink-0 text-muted" />
          )
        ) : (
          <Wrench className="size-3.5 shrink-0 text-muted" />
        )}
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
        {!open && c.output && <span className="truncate text-muted">{clip(c.output, 100)}</span>}
      </button>
      {open && hasDetail && (
        <div className="mt-1 ml-5 rounded-md border border-border bg-panel/60">
          {c.input && (
            <div>
              <div className="px-2.5 pt-1.5 text-[10px] font-semibold uppercase tracking-wider text-muted">
                Arguments
              </div>
              <ToolOutput text={c.input} />
            </div>
          )}
          {c.output && (
            <div>
              <div className="px-2.5 pt-1.5 text-[10px] font-semibold uppercase tracking-wider text-muted">
                {c.error ? "Error" : "Result"}
              </div>
              <ToolOutput text={c.output} />
            </div>
          )}
        </div>
      )}
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

// runIsLive reports whether a run is still in flight (no terminal event yet), so
// the steering cockpit only shows for runs an operator can actually fly.
function runIsLive(arc: AgentEvent[]): boolean {
  return !arc.some((e) => e.kind === "task.completed" || e.kind === "task.failed");
}

// runPaused folds the run.paused/run.resumed timeline (latest wins by seq) into
// the current pause state, so the control reflects the live daemon — including
// pauses issued from another client or the CLI.
function runPaused(arc: AgentEvent[]): boolean {
  let paused = false;
  for (const e of [...arc].sort((a, b) => num(a.seq) - num(b.seq))) {
    if (e.kind === "run.paused" || e.kind === "run.stepped") paused = true;
    else if (e.kind === "run.resumed") paused = false;
  }
  return paused;
}

// SteerControls is the live cockpit for one in-flight run (M608): pause it at the
// next iteration boundary, single-step, resume, or inject an operator directive
// that the agent folds into its next prompt. State is derived from the run's own
// event arc, so it stays in sync with the daemon no matter who issued the action.
export function SteerControls({ correlationId, arc }: { correlationId: string; arc: AgentEvent[] }) {
  const paused = useMemo(() => runPaused(arc), [arc]);
  const [directive, setDirective] = useState("");
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  // Two-step confirm for the destructive Cancel — kills the run for good, so it
  // arms on the first click and fires on the second (kept self-contained so the
  // cockpit stays a pure, prop-driven component).
  const [confirmingCancel, setConfirmingCancel] = useState(false);

  async function act(path: string, params?: Record<string, string>) {
    setBusy(true);
    setErr(null);
    try {
      await postAction(path, { correlation: correlationId, ...params });
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setBusy(false);
    }
  }

  async function cancelRun() {
    if (!confirmingCancel) {
      setConfirmingCancel(true);
      return;
    }
    setConfirmingCancel(false);
    // The same targeted cancel `agt` uses (M32): stops THIS run by correlation id
    // without halting the kernel or touching other runs.
    await act("/api/cancel_run");
  }

  async function send() {
    const d = directive.trim();
    if (!d) return;
    await act("/api/run/steer", { directive: d });
    setDirective("");
  }

  return (
    <div className="rounded-lg border border-accent/30 bg-accent/5 p-2.5">
      <div className="mb-2 flex items-center gap-2 text-xs font-semibold uppercase tracking-wider text-accent">
        <Navigation className="size-3.5" /> Steer this run
        {paused && (
          <span className="ml-auto inline-flex items-center gap-1 rounded-full bg-bad/15 px-2 py-0.5 text-[10px] font-medium text-bad">
            <Pause className="size-3" /> paused
          </span>
        )}
      </div>
      <div className="flex flex-wrap items-center gap-1.5">
        {paused ? (
          <button
            onClick={() => act("/api/run/resume")}
            disabled={busy}
            className="inline-flex h-7 items-center gap-1 rounded-md border border-good px-2 text-xs text-good transition-colors hover:bg-good hover:text-white disabled:opacity-50"
          >
            <Play className="size-3.5" /> Resume
          </button>
        ) : (
          <button
            onClick={() => act("/api/run/pause")}
            disabled={busy}
            className="inline-flex h-7 items-center gap-1 rounded-md border border-border px-2 text-xs transition-colors hover:border-accent disabled:opacity-50"
          >
            <Pause className="size-3.5" /> Pause
          </button>
        )}
        <button
          onClick={() => act("/api/run/step")}
          disabled={busy}
          className="inline-flex h-7 items-center gap-1 rounded-md border border-border px-2 text-xs transition-colors hover:border-accent disabled:opacity-50"
          title="Run exactly one more iteration, then pause"
        >
          <StepForward className="size-3.5" /> Step
        </button>
        <button
          onClick={cancelRun}
          onBlur={() => setConfirmingCancel(false)}
          disabled={busy}
          title="Cancel this run (stops it for good, without halting the kernel)"
          className={cn(
            "ml-auto inline-flex h-7 items-center gap-1 rounded-md border px-2 text-xs transition-colors disabled:opacity-50",
            confirmingCancel
              ? "border-bad bg-bad text-white"
              : "border-bad/60 text-bad hover:bg-bad hover:text-white",
          )}
        >
          <OctagonX className="size-3.5" /> {confirmingCancel ? "Confirm cancel" : "Cancel"}
        </button>
      </div>
      <div className="mt-2 flex items-center gap-1.5">
        <input
          value={directive}
          onChange={(e) => setDirective(e.target.value)}
          onKeyDown={(e) => e.key === "Enter" && send()}
          placeholder="Inject a directive — e.g. focus on the database schema"
          disabled={busy}
          className="h-7 flex-1 rounded-md border border-border bg-panel px-2 text-xs outline-none focus:border-accent disabled:opacity-50"
        />
        <button
          onClick={send}
          disabled={busy || directive.trim() === ""}
          className={cn(
            "inline-flex h-7 items-center gap-1 rounded-md border border-accent px-2 text-xs text-accent transition-colors",
            "hover:bg-accent hover:text-white disabled:opacity-50",
          )}
        >
          <Send className="size-3.5" /> Send
        </button>
      </div>
      {err && <div className="mt-1.5 text-xs text-bad">{err}</div>}
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
  if (!arc) return <SkeletonList count={2} lines={2} />;
  return (
    <>
      {correlationId && runIsLive(arc) && <SteerControls correlationId={correlationId} arc={arc} />}
      <RunPhaseTimeline arc={arc} />
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
                  {isIncidentFamilyEvent(e) && <IncidentBadges item={incidentBadgeItem(e)} mono />}
                  <span className="truncate text-muted">
                    {incidentEventSummary(e) || e.subject}
                  </span>
                </li>
              ))}
          </ul>
        ) : (
          <Muted>no events</Muted>
        ))}
    </>
  );
}
