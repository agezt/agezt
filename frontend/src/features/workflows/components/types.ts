// types.ts — pure workflow type definitions (extracted from Workflows.tsx).
// Day 23 god file split (3. attempt — minimal + types-only).
//
// Workflows.tsx used to be 1597 satir with everything inline. The split
// is intentionally conservative: only type declarations move here; the
// rest of the file (helpers + sub-components + main page) stays in
// page.tsx as a single co-located unit. Future splits can peel off
// nodes.tsx / panels.tsx / flow.ts once the cross-import graph is
// mapped out (see scripts/dev/split-workflows-god-file.py for the
// aggressive 8-file attempt that was rolled back).

import type { Node as RFNode } from "@xyflow/react";
import type { Tone } from "@/lib/tone";

export interface WfNode {
  id: string;
  type: string;
  label?: string;
  config?: Record<string, unknown>;
  x?: number;
  y?: number;
  // Reliability settings (M808) — per-node, outside config.
  timeout_sec?: number;
  retries?: number;
  retry_delay_sec?: number;
}
export interface WfSettings {
  timeout_sec?: number;
  retries?: number;
  retry_delay_sec?: number;
}
export interface WfEdge {
  from: string;
  to: string;
  port?: string;
}
export interface Wf {
  id?: string;
  name: string;
  description?: string;
  enabled?: boolean;
  nodes: WfNode[];
  edges?: WfEdge[];
  trigger_kind?: string;
  trigger_detail?: string;
  node_count?: number;
  /** Newest run, folded from the journal by the list handler (with_runs). */
  last_run?: { status?: string; at_ms?: number; duration_ms?: number };
}
export interface WorkflowChainKind {
  label: string;
  /** One short sentence the badge itself cannot say. Rendered as a tooltip. */
  detail: string;
  tone: Tone;
}
export interface WfTemplate {
  name: string;
  title: string;
  description: string;
  category: string;
  node_count?: number;
  workflow: Wf;
}
export interface WfRunNodeEvent {
  node: string;
  ok?: boolean;
  handled?: boolean;
  port?: string;
  label?: string;
  error?: string;
  // Per-node data snippets (M808): what the node consumed and produced.
  input?: string;
  output?: string;
  output_truncated?: boolean;
  attempts?: number;
}
export interface WfRun {
  correlation_id: string;
  status: string; // running | completed | failed
  started_ms?: number;
  finished_ms?: number;
  node_events?: WfRunNodeEvent[];
  error?: string;
  source?: string;
  runner?: string;
  agent?: string;
  schedule_id?: string;
  standing_id?: string;
  trigger_subject?: string;
  parent_correlation_id?: string;
}
