import { useCallback, useEffect, useMemo, useRef, useState } from "react";
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
  RotateCcw,
  History,
  AlertTriangle,
  CheckCircle2,
  Copy,
  Download,
} from "lucide-react";
import { getJSON, postAction, postJSON } from "@/lib/api";
import { Badge, statusVariant } from "@/components/ui/badge";
import { KeyValue, Muted, ErrorText } from "@/components/JsonView";
import { ToolOutput } from "@/components/DataView";
import { SkeletonList } from "@/components/ui/skeleton";
import { cn } from "@/lib/utils";
import { fmtTime, clip } from "@/lib/utils";
import { money } from "@/lib/format";
import { useUI } from "@/components/ui/feedback";
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

interface RollbackCheckpoint {
  id: string;
  kind: string;
  action?: string;
  run_id?: string;
  subject_id?: string;
  subject_name?: string;
  before_status?: string;
  before?: Record<string, unknown>;
  created_ms?: number;
  applied_ms?: number;
}

interface RemoteArtifact {
  peer: string;
  remoteCorrelation: string;
  id: string;
  ref: string;
  name: string;
  mime: string;
  kind: string;
  source: string;
  size?: number;
  createdMs?: number;
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

function subagentSpawnDetail(e: AgentEvent): string {
  return [
    payloadString(e, "task") ? clip(payloadString(e, "task"), 100) : "",
    payloadString(e, "child_correlation") ? `child: ${payloadString(e, "child_correlation")}` : "",
    payloadNumber(e, "depth") ? `depth: ${payloadNumber(e, "depth")}` : "",
    payloadString(e, "agent") ? `agent: ${payloadString(e, "agent")}` : "",
    e.payload?.async ? "async" : "",
  ].filter(Boolean).join(" · ");
}

function subagentCompletedDetail(e: AgentEvent): string {
  return [
    payloadString(e, "child_correlation") ? `child: ${payloadString(e, "child_correlation")}` : "",
    payloadNumber(e, "chars") ? `${payloadNumber(e, "chars")?.toLocaleString()} chars` : "",
    payloadString(e, "error") ? clip(payloadString(e, "error"), 100) : "",
    e.payload?.async ? "async" : "",
  ].filter(Boolean).join(" · ");
}

function stringField(v: unknown): string {
  return typeof v === "string" ? v : "";
}

function numberField(v: unknown): number | undefined {
  return typeof v === "number" && Number.isFinite(v) ? v : undefined;
}

export function remoteArtifactsFromArc(arc: AgentEvent[]): RemoteArtifact[] {
  const out: RemoteArtifact[] = [];
  const seen = new Set<string>();
  for (const e of [...arc].sort((a, b) => num(a.seq) - num(b.seq))) {
    const p = e.payload || {};
    if (p.phase !== "peer_events_mirrored" || !Array.isArray(p.artifacts)) continue;
    const peer = stringField(p.remote_peer);
    const remoteCorrelation = stringField(p.remote_correlation);
    for (const raw of p.artifacts) {
      if (!raw || typeof raw !== "object") continue;
      const row = raw as Record<string, unknown>;
      const id = stringField(row.id);
      if (!id) continue;
      const corr = stringField(row.corr) || remoteCorrelation;
      const key = `${peer}\x00${corr}\x00${id}`;
      if (seen.has(key)) continue;
      seen.add(key);
      out.push({
        peer,
        remoteCorrelation: corr,
        id,
        ref: stringField(row.ref),
        name: stringField(row.name),
        mime: stringField(row.mime),
        kind: stringField(row.kind),
        source: stringField(row.source),
        size: numberField(row.size),
        createdMs: numberField(row.created_ms),
      });
    }
  }
  return out;
}

function artifactSize(n?: number): string {
  if (n == null || !Number.isFinite(n)) return "unknown";
  if (n < 1024) return `${n} B`;
  if (n < 1024 ** 2) return `${(n / 1024).toFixed(1)} KB`;
  if (n < 1024 ** 3) return `${(n / 1024 ** 2).toFixed(1)} MB`;
  return `${(n / 1024 ** 3).toFixed(2)} GB`;
}

function remoteArtifactCommand(a: RemoteArtifact): string {
  return `agt peers artifact-get ${a.peer || "<peer>"} ${a.id} <out_file>`;
}

function CopyRemoteArtifactCommand({ artifact }: { artifact: RemoteArtifact }) {
  const [copied, setCopied] = useState(false);
  async function copy() {
    try {
      await navigator.clipboard?.writeText(remoteArtifactCommand(artifact));
      setCopied(true);
      window.setTimeout(() => setCopied(false), 1200);
    } catch {
      setCopied(false);
    }
  }
  return (
    <button
      onClick={copy}
      title={copied ? "Copied" : "Copy artifact-get command"}
      className="inline-flex size-7 shrink-0 items-center justify-center rounded-md border border-border text-muted transition-colors hover:border-accent hover:text-foreground"
    >
      <Copy className={cn("size-3.5", copied && "text-good")} />
    </button>
  );
}

function RemoteArtifactsPanel({ arc }: { arc: AgentEvent[] }) {
  const artifacts = remoteArtifactsFromArc(arc);
  if (!artifacts.length) return null;
  const visible = artifacts.slice(0, 12);
  return (
    <div>
      <div className="mb-1 flex items-center gap-1.5 text-xs font-semibold uppercase tracking-normal text-muted">
        <Download className="size-3.5 text-accent" />
        Remote artifacts ({artifacts.length})
      </div>
      <ul className="space-y-1">
        {visible.map((a) => {
          const label = a.name || a.id;
          const ref = a.ref.length > 12 ? a.ref.slice(0, 12) : a.ref;
          return (
            <li key={`${a.peer}:${a.remoteCorrelation}:${a.id}`} className="rounded-md border border-border bg-panel px-2 py-1.5 text-xs">
              <div className="flex min-w-0 items-center gap-2">
                <span className="min-w-0 flex-1 truncate font-medium text-foreground">{label}</span>
                <Badge variant="accent">{a.peer || "peer"}</Badge>
                <CopyRemoteArtifactCommand artifact={a} />
              </div>
              <div className="mt-1 flex flex-wrap items-center gap-x-2 gap-y-1 text-muted">
                <span>{a.kind || "artifact"}</span>
                <span>{artifactSize(a.size)}</span>
                {a.mime && <span>{a.mime}</span>}
                {a.remoteCorrelation && <span>{a.remoteCorrelation}</span>}
                {ref && <span className="font-mono">{ref}</span>}
              </div>
            </li>
          );
        })}
      </ul>
      {artifacts.length > visible.length && (
        <div className="mt-1 text-xs text-muted">{artifacts.length - visible.length} more remote artifact(s)</div>
      )}
    </div>
  );
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
        case "subagent.spawned":
          return {
            key: `${e.seq || i}:subagent-spawned`,
            ts: e.ts_unix_ms,
            kind: e.kind,
            phase: "delegating",
            detail: subagentSpawnDetail(e),
          };
        case "subagent.completed":
          return {
            key: `${e.seq || i}:subagent-completed`,
            ts: e.ts_unix_ms,
            kind: e.kind,
            phase: e.payload?.ok === false ? "delegation failed" : "delegation completed",
            detail: subagentCompletedDetail(e),
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

function RunPhaseTimeline({ arc }: { arc: AgentEvent[] }) {
  const steps = runPhaseSteps(arc);
  if (!steps.length) return null;
  const latest = steps[steps.length - 1];
  return (
    <div className="rounded-lg border border-border bg-panel/50 p-2.5">
      <div className="mb-2 flex flex-wrap items-center gap-2 text-xs">
        <span className="font-semibold uppercase tracking-normal text-muted">phase timeline</span>
        <Badge variant={latest.phase === "failed" ? "bad" : latest.phase === "completed" ? "good" : "accent"}>
          {latest.phase}
        </Badge>
        {latest.detail && <span className="truncate text-muted">{latest.detail}</span>}
      </div>
      <ol className="space-y-1">
        {steps.slice(-8).map((s) => (
          <li key={s.key} className="flex items-start gap-2 text-xs">
            <span className="w-16 shrink-0 font-mono text-xs text-muted">{fmtTime(s.ts)}</span>
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
              <div className="px-2.5 pt-1.5 text-xs font-semibold uppercase tracking-normal text-muted">
                Arguments
              </div>
              <ToolOutput text={c.input} />
            </div>
          )}
          {c.output && (
            <div>
              <div className="px-2.5 pt-1.5 text-xs font-semibold uppercase tracking-normal text-muted">
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
  const delegations = d.subagentsSpawned + d.subagentsCompleted + d.subagentsFailed;
  return (
    <div className="space-y-3">
      <KeyValue
        pairs={[
          ["status", <Badge variant={statusVariant(status)}>{status || "?"}</Badge>],
          ["model", d.model || <Muted>—</Muted>],
          ["iterations", d.iterations || <Muted>—</Muted>],
          [
            "delegations",
            delegations ? (
              <span>
                {d.subagentsSpawned} spawned / {d.subagentsCompleted} completed / {d.subagentsFailed} failed
              </span>
            ) : (
              <Muted>—</Muted>
            ),
          ],
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
        <div className="mb-1 text-xs font-semibold uppercase tracking-normal text-muted">
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

      <RemoteArtifactsPanel arc={arc} />

      {d.answer && (
        <div>
          <div className="mb-1 text-xs font-semibold uppercase tracking-normal text-muted">
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
      <div className="mb-2 flex items-center gap-2 text-xs font-semibold uppercase tracking-normal text-accent">
        <Navigation className="size-3.5" /> Steer this run
        {paused && (
          <span className="ml-auto inline-flex items-center gap-1 rounded-full bg-bad/15 px-2 py-0.5 text-xs font-medium text-bad">
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

function rollbackSubject(cp: RollbackCheckpoint): string {
  if (cp.subject_name) return cp.subject_name;
  return cp.subject_id ? clip(cp.subject_id, 40) : cp.id;
}

function rollbackRestoreText(cp: RollbackCheckpoint): string {
  if (cp.before_status) return `status -> ${cp.before_status}`;
  if (cp.kind === "workflow.snapshot") return "workflow snapshot";
  if (cp.kind === "file.snapshot") return cp.before?.exists === false ? "remove created file" : "file content snapshot";
  if (cp.kind === "config.setting") {
    if (cp.before?.rollbackable === false) return "audit only";
    return cp.before?.set === false ? "config -> unset" : "config -> previous value";
  }
  return cp.kind;
}

function rollbackAuditReason(cp: RollbackCheckpoint): string {
  const reason = cp.before?.non_rollbackable_reason;
  return typeof reason === "string" ? reason : "";
}

export function RunRollbackDrawer({ correlationId }: { correlationId: string }) {
  const ui = useUI();
  const [open, setOpen] = useState(false);
  const [checkpoints, setCheckpoints] = useState<RollbackCheckpoint[]>([]);
  const [loading, setLoading] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  const [busyID, setBusyID] = useState<string | null>(null);

  const load = useCallback(async () => {
    if (!correlationId) return;
    setLoading(true);
    setErr(null);
    try {
      const data = await getJSON<{ checkpoints?: RollbackCheckpoint[] }>("/api/rollback/checkpoints", {
        run_id: correlationId,
      });
      setCheckpoints(data.checkpoints || []);
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setLoading(false);
    }
  }, [correlationId]);

  useEffect(() => {
    void load();
  }, [load]);

  async function apply(cp: RollbackCheckpoint) {
    const auditReason = rollbackAuditReason(cp);
    if (cp.applied_ms || auditReason) return;
    const ok = await ui.confirm({
      title: "Apply rollback checkpoint?",
      message: `${rollbackSubject(cp)} will be restored from ${cp.id}.`,
      confirmLabel: "Apply rollback",
      danger: true,
    });
    if (!ok) return;
    setBusyID(cp.id);
    try {
      await postJSON("/api/rollback/apply", { id: cp.id });
      ui.toast("Rollback applied", "success");
      await load();
    } catch (e) {
      ui.toast(`Rollback failed: ${(e as Error).message}`, "error");
    } finally {
      setBusyID(null);
    }
  }

  const unapplied = checkpoints.filter((c) => !c.applied_ms).length;
  return (
    <div className="rounded-lg border border-border bg-panel/40">
      <button
        onClick={() => setOpen((v) => !v)}
        className="flex w-full items-center gap-2 px-2.5 py-2 text-left text-xs"
      >
        {open ? <ChevronDown className="size-3.5 text-muted" /> : <ChevronRight className="size-3.5 text-muted" />}
        <RotateCcw className="size-3.5 text-accent" />
        <span className="font-semibold uppercase tracking-normal text-muted">rollback</span>
        <Badge variant={unapplied ? "accent" : "default"}>{unapplied}</Badge>
        {loading && <span className="text-muted">loading...</span>}
      </button>
      {open && (
        <div className="border-t border-border/60 px-2.5 py-2">
          {err ? (
            <ErrorText>{err}</ErrorText>
          ) : checkpoints.length === 0 ? (
            <Muted>no checkpoints for this run</Muted>
          ) : (
            <ul className="space-y-1.5">
              {checkpoints.map((cp) => {
                const auditReason = rollbackAuditReason(cp);
                const applied = !!cp.applied_ms;
                return (
                  <li key={cp.id} className="rounded-md border border-border bg-card px-2.5 py-2 text-xs">
                    <div className="flex flex-wrap items-center gap-1.5">
                      <History className="size-3.5 text-muted" />
                      <span className="min-w-0 flex-1 truncate font-medium">{rollbackSubject(cp)}</span>
                      <Badge variant={cp.kind === "file.snapshot" ? "good" : "accent"}>{cp.kind}</Badge>
                      {auditReason && (
                        <Badge variant="warn">
                          <AlertTriangle className="mr-1 size-3" />
                          audit only
                        </Badge>
                      )}
                      {applied && (
                        <Badge variant="good">
                          <CheckCircle2 className="mr-1 size-3" />
                          applied
                        </Badge>
                      )}
                    </div>
                    <div className="mt-1 flex flex-wrap items-center gap-x-2 gap-y-1 text-muted">
                      <span>{cp.action || "checkpoint"}</span>
                      <span>{rollbackRestoreText(cp)}</span>
                      <span>{fmtTime(cp.created_ms)}</span>
                      {auditReason && <span className="text-warn">{auditReason}</span>}
                    </div>
                    <div className="mt-2 flex justify-end">
                      <button
                        onClick={() => void apply(cp)}
                        disabled={applied || !!auditReason || busyID === cp.id}
                        className="inline-flex h-7 items-center gap-1 rounded-md border border-border px-2 text-xs transition-colors hover:border-accent disabled:opacity-50"
                      >
                        <RotateCcw className={cn("size-3.5", busyID === cp.id && "animate-spin")} />
                        Apply
                      </button>
                    </div>
                  </li>
                );
              })}
            </ul>
          )}
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
  if (!arc) return <SkeletonList count={2} lines={2} />;
  return (
    <>
      {correlationId && runIsLive(arc) && <SteerControls correlationId={correlationId} arc={arc} />}
      {correlationId && <RunRollbackDrawer correlationId={correlationId} />}
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
