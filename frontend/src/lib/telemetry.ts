import type { AgentEvent } from "@/lib/events";
import { num } from "@/lib/rundetail";

// Real-time telemetry: per-second buckets folded from the live event firehose,
// summarised into rolling rates. Pure (no React, no timers) so the bucketing and
// rate math are unit-tested directly; the Mission view owns the 1s ticker and
// the rolling window of buckets.

export interface Bucket {
  events: number;
  llm: number;
  tools: number;
  tokensIn: number;
  tokensOut: number;
  costMc: number;
}

export function emptyBucket(): Bucket {
  return { events: 0, llm: 0, tools: 0, tokensIn: 0, tokensOut: 0, costMc: 0 };
}

// addEvent folds one event's contribution into a bucket, returning a new bucket
// (immutability keeps it test-friendly and React-safe).
export function addEvent(b: Bucket, e: AgentEvent): Bucket {
  const k = e.kind || "";
  const p = e.payload || {};
  const nb = { ...b };
  nb.events += 1;
  if (k.startsWith("llm.")) nb.llm += 1;
  if (k === "tool.invoked") nb.tools += 1;
  if (k === "budget.consumed") {
    nb.tokensIn += num(p.input_tokens);
    nb.tokensOut += num(p.output_tokens);
    nb.costMc += num(p.cost_microcents);
  }
  return nb;
}

export interface Telemetry {
  windowSec: number;
  totalEvents: number;
  eventsPerSec: number;
  llmPerSec: number;
  toolsPerSec: number;
  tokensPerSec: number;
  costPerSecMc: number;
  peakEvents: number; // busiest single second in the window
}

// summarize reduces a window of per-second buckets into rolling rates. Rates are
// averaged over the bucket count (= seconds), so they read as "per second over
// the last N seconds".
export function summarize(buckets: Bucket[]): Telemetry {
  const n = Math.max(1, buckets.length);
  let events = 0;
  let llm = 0;
  let tools = 0;
  let tokens = 0;
  let cost = 0;
  let peak = 0;
  for (const b of buckets) {
    events += b.events;
    llm += b.llm;
    tools += b.tools;
    tokens += b.tokensIn + b.tokensOut;
    cost += b.costMc;
    if (b.events > peak) peak = b.events;
  }
  return {
    windowSec: n,
    totalEvents: events,
    eventsPerSec: events / n,
    llmPerSec: llm / n,
    toolsPerSec: tools / n,
    tokensPerSec: tokens / n,
    costPerSecMc: cost / n,
    peakEvents: peak,
  };
}
