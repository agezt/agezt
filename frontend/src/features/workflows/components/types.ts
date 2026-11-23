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

import type { Tone } from "@/lib/tone";
import type {
  WorkflowDraftResponseWorkflow,
  WorkflowDraftResponseWorkflowEdgesItem,
  WorkflowDraftResponseWorkflowNodesItem,
  WorkflowListResponseWorkflowsItemLastRun,
  WorkflowRunsResponseRunsItem,
  WorkflowRunsResponseRunsItemNodeEventsItem,
  WorkflowTemplatesResponseTemplatesItem,
} from "@/app/api/types.generated";

// WfNode is a workflow node as the canvas reads it.
export type WfNode = {
  id: WorkflowDraftResponseWorkflowNodesItem["id"];
  type: WorkflowDraftResponseWorkflowNodesItem["type"];
  label?: WorkflowDraftResponseWorkflowNodesItem["label"];
  config?: WorkflowDraftResponseWorkflowNodesItem["config"];
  x?: WorkflowDraftResponseWorkflowNodesItem["x"];
  y?: WorkflowDraftResponseWorkflowNodesItem["y"];
  timeout_sec?: WorkflowDraftResponseWorkflowNodesItem["timeout_sec"];
  retries?: WorkflowDraftResponseWorkflowNodesItem["retries"];
  retry_delay_sec?: WorkflowDraftResponseWorkflowNodesItem["retry_delay_sec"];
};

export type WfSettings = {
  timeout_sec?: WorkflowDraftResponseWorkflowNodesItem["timeout_sec"];
  retries?: WorkflowDraftResponseWorkflowNodesItem["retries"];
  retry_delay_sec?: WorkflowDraftResponseWorkflowNodesItem["retry_delay_sec"];};
// WfEdge is a workflow edge as the canvas reads it.
export type WfEdge = {
  from: WorkflowDraftResponseWorkflowEdgesItem["from"];
  to: WorkflowDraftResponseWorkflowEdgesItem["to"];
  port?: WorkflowDraftResponseWorkflowEdgesItem["port"];
};

export type Wf = {
  id?: WorkflowDraftResponseWorkflow["id"];
  name: WorkflowDraftResponseWorkflow["name"];
  description?: WorkflowDraftResponseWorkflow["description"];
  enabled?: WorkflowDraftResponseWorkflow["enabled"];
  // The LIST answer carries the shape, not the graph: /api/workflows answers
  // with node_count and edge_count, and the canvas asks for the draft when it
  // needs the nodes themselves.
  nodes?: WfNode[] | null;
  edges?: WfEdge[] | null;
  trigger_kind?: WorkflowDraftResponseWorkflow["trigger_kind"];
  trigger_detail?: WorkflowDraftResponseWorkflow["trigger_detail"];
  node_count?: WorkflowDraftResponseWorkflow["node_count"];
  last_run?: WorkflowDraftResponseWorkflow["last_run"];
};

export interface WorkflowChainKind {
  label: string;
  /** One short sentence the badge itself cannot say. Rendered as a tooltip. */
  detail: string;
  tone: Tone;
}
export type WfTemplate = {
  name: WorkflowTemplatesResponseTemplatesItem["name"];
  title: WorkflowTemplatesResponseTemplatesItem["title"];
  description: WorkflowTemplatesResponseTemplatesItem["description"];
  category: WorkflowTemplatesResponseTemplatesItem["category"];
  node_count?: WorkflowTemplatesResponseTemplatesItem["node_count"];
  workflow: Wf;
};

// WfRunNodeEvent is a run's node event as the view reads it.
// WfRunNodeEvent is a run's node event as the view reads it.
export type WfRunNodeEvent = {
  node: WorkflowRunsResponseRunsItemNodeEventsItem["node"];
  ok?: WorkflowRunsResponseRunsItemNodeEventsItem["ok"];
  handled?: WorkflowRunsResponseRunsItemNodeEventsItem["handled"];
  port?: WorkflowRunsResponseRunsItemNodeEventsItem["port"];
  label?: WorkflowRunsResponseRunsItemNodeEventsItem["label"];
  error?: WorkflowRunsResponseRunsItemNodeEventsItem["error"];
  input?: WorkflowRunsResponseRunsItemNodeEventsItem["input"];
  output?: WorkflowRunsResponseRunsItemNodeEventsItem["output"];
  output_truncated?: WorkflowRunsResponseRunsItemNodeEventsItem["output_truncated"];
  attempts?: WorkflowRunsResponseRunsItemNodeEventsItem["attempts"];
};

export type WfRun = {
  correlation_id: WorkflowRunsResponseRunsItem["correlation_id"];
  status: WorkflowRunsResponseRunsItem["status"];
  started_ms?: WorkflowRunsResponseRunsItem["started_ms"];
  finished_ms?: WorkflowRunsResponseRunsItem["finished_ms"];
  node_events?: WfRunNodeEvent[] | null;
  error?: WorkflowRunsResponseRunsItem["error"];
  source?: WorkflowRunsResponseRunsItem["source"];
  runner?: WorkflowRunsResponseRunsItem["runner"];
  agent?: WorkflowRunsResponseRunsItem["agent"];
  schedule_id?: WorkflowRunsResponseRunsItem["schedule_id"];
  standing_id?: WorkflowRunsResponseRunsItem["standing_id"];
  trigger_subject?: WorkflowRunsResponseRunsItem["trigger_subject"];
  parent_correlation_id?: WorkflowRunsResponseRunsItem["parent_correlation_id"];
};
