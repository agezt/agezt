import type { AgentEvent } from "@/lib/events";
import { num } from "@/lib/rundetail";
import {
  incidentBadgeItem,
  incidentEventSummary,
  isIncidentFamilyEvent,
} from "@/lib/incidentevents";

// A ReplayStep is one meaningful moment in a run's life, derived from its
// journaled event arc. The flight recorder scrubs/plays through these, and each
// step carries the CUMULATIVE run state up to and including it — so jumping to
// any step shows exactly what the agent had spent/done by that point.
export type StepTone =
  | "received"
  | "request"
  | "response"
  | "tool"
  | "result"
  | "policy"
  | "spend"
  | "steer"
  | "done"
  | "fail"
  | "context"
  | "other";

export interface ReplayStep {
  seq: number;
  ts: number;
  kind: string;
  iter: number | null;
  title: string;
  detail: string;
  tone: StepTone;
  incident?: {
    subject?: string;
    phase?: string;
    mode?: string;
  };
  // Cumulative through this step.
  cumIn: number;
  cumOut: number;
  cumCostMc: number;
  cumTools: number;
}

// Streaming/ephemeral kinds carry no standalone meaning for a step-through (and
// are usually absent from the durable journal anyway) — drop them so the
// recorder shows the substantive arc, not thousands of token deltas.
const SKIP = new Set(["llm.token", "llm.reasoning"]);

function clip(s: unknown, n: number): string {
  const t = s == null ? "" : String(s);
  return t.length > n ? t.slice(0, n) + "…" : t;
}

// buildReplay folds a run's event arc into an ordered list of replay steps with
// running totals. Pure — unit-tested directly.
export function buildReplay(arc: AgentEvent[]): ReplayStep[] {
  const sorted = [...arc].filter((e) => e.kind && !SKIP.has(e.kind)).sort((a, b) => num(a.seq) - num(b.seq));
  const steps: ReplayStep[] = [];
  let cumIn = 0;
  let cumOut = 0;
  let cumCostMc = 0;
  let cumTools = 0;
  // Iteration is "sticky": only llm.* events stamp it, but every step in
  // between (tool calls, spend, results) belongs to the last-seen iteration, so
  // the cursor's iteration metric is meaningful on every step.
  let lastIter: number | null = null;

  for (const e of sorted) {
    const p = e.payload || {};
    if (p.iter != null) lastIter = num(p.iter);
    const iter = lastIter;
    let title = e.kind || "event";
    let detail = "";
    let tone: StepTone = "other";
    const incident = isIncidentFamilyEvent(e) ? incidentBadgeItem(e) : undefined;

    switch (e.kind) {
      case "task.received":
        title = "Task received";
        detail = clip(p.intent, 240);
        tone = "received";
        break;
      case "llm.request":
        title = `LLM request · iter ${num(p.iter) + 1}`;
        detail = `${num(p.messages)} messages · ${num(p.context_chars).toLocaleString()} ctx chars · ${num(p.tools)} tools offered`;
        tone = "request";
        break;
      case "llm.response":
        title = `LLM response · iter ${num(p.iter) + 1}`;
        detail = `stop=${p.stop_reason || "?"} · ${num(p.tool_calls)} tool call(s) · ${num(p.text_chars)} chars${num(p.reasoning_chars) ? ` · ${num(p.reasoning_chars)} reasoning` : ""}`;
        tone = "response";
        break;
      case "tool.invoked":
        cumTools += 1;
        title = `→ ${p.tool || "tool"}`;
        detail = clip(typeof p.input === "string" ? p.input : JSON.stringify(p.input ?? {}), 200);
        tone = "tool";
        break;
      case "tool.result":
        title = `← ${p.tool || "tool"}${p.error ? " (error)" : ""}`;
        detail = clip(p.output, 240);
        tone = "result";
        break;
      case "policy.decision":
        title = `policy: ${p.allow ? "allow" : p.hard_denied ? "DENY(hard)" : "DENY"} ${p.capability || ""}`;
        detail = clip(p.reason, 200);
        tone = "policy";
        break;
      case "budget.consumed":
        cumIn += num(p.input_tokens);
        cumOut += num(p.output_tokens);
        cumCostMc += num(p.cost_microcents);
        title = `spend +$${(num(p.cost_microcents) / 1e9).toFixed(4)}`;
        detail = `${num(p.input_tokens)} in / ${num(p.output_tokens)} out${num(p.cached_input_tokens) ? ` · ${num(p.cached_input_tokens)} cached` : ""} · ${p.model || ""}`;
        tone = "spend";
        break;
      case "run.paused":
        title = "⏸ paused by operator";
        tone = "steer";
        break;
      case "run.resumed":
        title = "▶ resumed by operator";
        tone = "steer";
        break;
      case "run.stepped":
        title = "⏭ stepped by operator";
        tone = "steer";
        break;
      case "run.steered":
        title = "✎ operator directive";
        detail = clip(p.directive, 240);
        tone = "steer";
        break;
      case "context.compacted":
        title = "context compacted";
        detail = `elided ${num(p.elided)} · reclaimed ${num(p.reclaimed_chars).toLocaleString()} chars`;
        tone = "context";
        break;
      case "subagent.spawned":
        title = "sub-agent spawned";
        detail = clip(p.intent || p.correlation_id, 200);
        tone = "steer";
        break;
      case "task.completed":
        title = "✓ completed";
        detail = clip(p.answer, 300);
        tone = "done";
        break;
      case "task.failed":
        title = "✗ failed";
        detail = clip(p.error || p.reason, 240);
        tone = "fail";
        break;
      default:
        title = incident ? incidentEventSummary(e) : e.kind || "event";
        detail = incident ? "" : e.subject || "";
        tone = "other";
    }

    steps.push({
      seq: num(e.seq),
      ts: num(e.ts_unix_ms),
      kind: e.kind || "",
      iter,
      title,
      detail,
      tone,
      incident,
      cumIn,
      cumOut,
      cumCostMc,
      cumTools,
    });
  }
  return steps;
}
