import type { AgentEvent } from "@/lib/events";

// One tool call, assembled across the policy.decision / tool.invoked /
// tool.result events that share a call_id.
export interface ToolCall {
  callId: string;
  tool: string;
  capability?: string;
  allow?: boolean;
  hardDenied?: boolean;
  error?: boolean;
  input?: string; // the tool's arguments (JSON), captured from tool.invoked
  output?: string;
}

// RunDetail is the structured summary derived from a run's journaled event arc —
// the UI computes it, the kernel stays the source of truth (the raw events are
// always available below).
export interface RunDetail {
  model?: string;
  iterations: number;
  inputTokens: number;
  outputTokens: number;
  cachedTokens: number;
  costMicrocents: number;
  hasBudget: boolean;
  subagentsSpawned: number;
  subagentsCompleted: number;
  subagentsFailed: number;
  status?: string;
  answer?: string;
  toolCalls: ToolCall[];
}

export function num(v: unknown): number {
  const n = Number(v);
  return Number.isFinite(n) ? n : 0;
}

// mergeEvents unions two event lists, de-duplicating by journal seq (falling
// back to event id). Used to fold live SSE events into a run's fetched snapshot
// without double-counting an event delivered by both paths. Order is not
// guaranteed; deriveDetail and the raw-event view sort by seq themselves.
export function mergeEvents(a: AgentEvent[], b: AgentEvent[]): AgentEvent[] {
  const seen = new Set<string>();
  const out: AgentEvent[] = [];
  for (const e of [...a, ...b]) {
    const key = e.seq != null ? "s" + e.seq : e.id != null ? "i" + e.id : "";
    if (key) {
      if (seen.has(key)) continue;
      seen.add(key);
    }
    out.push(e);
  }
  return out;
}

// deriveDetail folds a run's journaled event arc into a RunDetail. Pure (no
// React), so it is unit-tested directly. Events are processed oldest→newest by
// seq, so later events (e.g. tool.result) win over earlier (tool.invoked), and
// tool calls are grouped by call_id.
export function deriveDetail(arc: AgentEvent[]): RunDetail {
  const d: RunDetail = {
    iterations: 0,
    inputTokens: 0,
    outputTokens: 0,
    cachedTokens: 0,
    costMicrocents: 0,
    hasBudget: false,
    subagentsSpawned: 0,
    subagentsCompleted: 0,
    subagentsFailed: 0,
    toolCalls: [],
  };
  const byCall = new Map<string, ToolCall>();
  const call = (id: string): ToolCall => {
    let c = byCall.get(id);
    if (!c) {
      c = { callId: id, tool: "" };
      byCall.set(id, c);
    }
    return c;
  };
  const sorted = [...arc].sort((a, b) => num(a.seq) - num(b.seq));
  for (const e of sorted) {
    const p = e.payload || {};
    switch (e.kind) {
      case "llm.request":
      case "llm.response":
        d.iterations = Math.max(d.iterations, num(p.iter) + 1);
        if (p.model) d.model = String(p.model);
        break;
      case "budget.consumed":
        d.hasBudget = true;
        d.costMicrocents += num(p.cost_microcents);
        d.inputTokens += num(p.input_tokens);
        d.outputTokens += num(p.output_tokens);
        d.cachedTokens += num(p.cached_input_tokens);
        if (p.model && !d.model) d.model = String(p.model);
        break;
      case "policy.decision": {
        const c = call(String(p.call_id || ""));
        if (p.tool) c.tool = String(p.tool);
        if (p.capability) c.capability = String(p.capability);
        c.allow = !!p.allow;
        c.hardDenied = !!p.hard_denied;
        break;
      }
      case "tool.invoked": {
        const c = call(String(p.call_id || ""));
        if (p.tool) c.tool = String(p.tool);
        // The agent loop journals the tool's arguments as `input` (json.RawMessage);
        // it lands as an object or a pre-stringified string. Keep a string so the
        // detail view can show exactly what the tool was called with — crucial for
        // autonomous runs (scheduled/standing) that never appear in the Chat view.
        if (p.input != null) c.input = typeof p.input === "string" ? p.input : JSON.stringify(p.input);
        break;
      }
      case "tool.result": {
        const c = call(String(p.call_id || ""));
        if (p.tool) c.tool = String(p.tool);
        c.error = !!p.error;
        if (p.output != null) c.output = String(p.output);
        break;
      }
      case "subagent.spawned":
        d.subagentsSpawned++;
        break;
      case "subagent.completed":
        if (p.ok === false) d.subagentsFailed++;
        else d.subagentsCompleted++;
        break;
      case "task.completed":
        d.status = "completed";
        if (p.answer != null) d.answer = String(p.answer);
        break;
      case "task.failed":
        d.status = "failed";
        if (p.error != null) d.answer = String(p.error);
        break;
    }
  }
  // Drop the synthetic empty-id bucket if nothing real landed in it.
  d.toolCalls = [...byCall.values()].filter((c) => c.callId !== "" || c.tool !== "");
  return d;
}
