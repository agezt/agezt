// features/workflows — Day 12 sixth feature carve-out.
//
// Owns:
//   - Workflows page (Workflows.tsx, 1597 satır — god file; carries the
//     ReactFlow-based editor with WfNode/WfEdge/Wf shapes, NODE_META,
//     portsForNode + summarize helpers, toFlow/fromFlow serializers,
//     CopilotPanel + RunsDrawer sub-components, workflowChainKind +
//     runToStatus + workflowRunSourceLabel formatters)
//   - Chains page (Chains.tsx, 401 satır — the chain CRUD view + the
//     ChainUsage type)
//   - lib/chains.ts (83 satır — the chain ref helpers: isChainRef +
//     chainName + chainRef + chainLabel + validateChainName + moveItem
//     + removeAt + renameChain + deleteChain + the ChainsState type;
//     moveItem/removeAt are generic array helpers that originated here
//     and stay colocated with the chain refs because renaming +
//     deleting chains is what the test exercises them with)
//
// Public surface (re-exported here so the rest of the codebase reads
// @/features/workflows and never needs to know about the sub-paths):
//   - components/Workflows.tsx, Chains.tsx — the 2 page-level views
//   - components/Workflows.tsx's reusable sub-components +
//     helpers: CopilotPanel, RunsDrawer, toFlow, fromFlow,
//     portsForNode, summarize, workflowChainKind,
//     workflowRunSourceLabel, runToStatus — all directly tested in
//     components/Workflows.test.tsx so they're part of the contract
//   - types.ts — WfNode, WfSettings, WfEdge, Wf, WorkflowChainKind,
//     WfTemplate, WfRunNodeEvent, WfRun, ChainsState (from lib)
//
// The 1 lib file (chains) stays package-internal — it's the
// feature's business logic; only the views + sub-components + types
// are surfaced. External code reaches it via @/features/workflows/
// lib/chains (allowed but not advertised).
//
// Cross-feature deps:
//   - @/app/api (getJSON, postJSON, postAction) — Day 3
//   - @/app/events (useEvents) — Day 4
//   - @/app/utils (cn, clip, fmtWhen) — Day 3
//   - @/components/ui/* (Button, Page, Disclosure, SkeletonList,
//     EmptyState, Badge, ErrorText, LoadMoreFooter) — shared UI
//     primitives
//   - @/components/ModelChip + @/components/ModelPicker — shared
//     model-picker widgets that import lib/chains
//     (isChainRef + chainName + chainRef + ChainsState); after the
//     move they read from @/features/workflows/lib/chains
//   - @/features/agents/components/agentdetail/ModelTab.tsx +
//     roster/form.tsx — both consume lib/chains (isChainRef +
//     chainName + ChainsState) for chain-ref UI in the agent detail
//     panel and the agent create/edit form
//
// The 1597-line Workflows.tsx god file is the biggest single risk in
// this feature; it carries ReactFlow glue + 8 type definitions + 5
// helpers + 4 sub-components in one file. A future Day-12b (or
// Day 26+ integration) should split it into:
//   - components/Workflows/flow.ts (toFlow/fromFlow + types)
//   - components/Workflows/nodes.tsx (WfNodeView + NODE_META +
//     portsForNode + summarize)
//   - components/Workflows/panels.tsx (NodePanel + NodeOptionPicker
//     + WorkflowModal + reliabilitySpecs)
//   - components/Workflows/CopilotPanel.tsx
//   - components/Workflows/RunsDrawer.tsx
//   - components/Workflows.tsx (the page + Workflows() default +
//     LastRun + freshID + TemplatePicker glue)
// Day 12's commit only MOVES the file; the split is deferred so
// typecheck + test verification stays focused on the carve-out.
//
// See docs/FRONTEND-REFACTOR-PLAN.md for the carve-out rationale.
export { Workflows } from "./components/Workflows";
export { Chains } from "./components/Chains";
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
} from "./components/Workflows";
export type { ChainUsage } from "./components/Chains";
export * from "./types";
