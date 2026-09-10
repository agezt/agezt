// Workflows.tsx — the workflows page. Re-exports the default Workflows()
// implementation from ./page, the pure type definitions from ./types,
// and the same sub-components + helpers (CopilotPanel, RunsDrawer,
// toFlow, fromFlow, portsForNode, summarize, workflowChainKind,
// workflowRunSourceLabel, runToStatus) that the original god file
// exported. Day 23 carved the 1597-line file into types.ts + page.tsx +
// this re-export; the public surface (consumed by nav.tsx and
// features/workflows/index.ts) is unchanged.
export { Workflows as default, Workflows } from "./page";
export {
  CopilotPanel,
  RunsDrawer,
  toFlow,
  fromFlow,
  portsForNode,
  summarize,
  workflowChainKind,
  workflowRunSourceLabel,
  runToStatus,
} from "./page";
export type {
  WfNode,
  WfSettings,
  WfEdge,
  Wf,
  WorkflowChainKind,
  WfTemplate,
  WfRunNodeEvent,
  WfRun,
} from "./types";
