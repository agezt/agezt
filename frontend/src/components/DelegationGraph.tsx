import { useMemo } from "react";
import {
  ReactFlow,
  Background,
  Controls,
  Handle,
  Position,
  MarkerType,
  type Node,
  type Edge,
  type NodeProps,
} from "@xyflow/react";
import { clip } from "@/lib/utils";
import { money } from "@/lib/format";
import { buildDelegationTree, type RunNode } from "@/lib/delegation";

type DelegationData = {
  title: string;
  status: string;
  model: string;
  iters: number;
  spentMc: number;
  root: boolean;
  depth: number;
  selected: boolean;
};
type DelegationRFNode = Node<DelegationData, "agent">;

const statusRing: Record<string, string> = {
  running: "border-accent",
  completed: "border-good",
  failed: "border-bad",
};
const statusDot: Record<string, string> = {
  running: "bg-accent animate-pulse",
  completed: "bg-good",
  failed: "bg-bad",
};

// AgentNodeView is one run in the delegation tree: status-coloured border + dot,
// the lead/sub-agent role, the intent, and a model · iters · cost footer.
function AgentNodeView({ data }: NodeProps<DelegationRFNode>) {
  return (
    <div
      className={[
        "w-[190px] cursor-pointer rounded-lg border-2 bg-card px-3 py-2 text-left transition-colors hover:border-accent",
        data.selected ? "ring-2 ring-accent ring-offset-1 ring-offset-card" : "",
        statusRing[data.status] || "border-border",
      ].join(" ")}
    >
      <Handle type="target" position={Position.Top} className="!bg-muted" />
      <div className="flex items-center gap-1.5">
        <span className={["size-2 shrink-0 rounded-full", statusDot[data.status] || "bg-muted"].join(" ")} />
        <span className="text-xs font-semibold uppercase tracking-normal text-muted">
          {data.root ? "lead" : "sub-agent"}
        </span>
        {/* Depth badge: at depth>1 a sub-agent is itself a delegator, so the
            level is what tells a deep tree apart from a flat fan-out. */}
        {!data.root && (
          <span className="rounded-sm bg-accent/15 px-1 text-[9px] font-semibold tabular-nums text-accent">
            L{data.depth}
          </span>
        )}
        <span className="ml-auto text-xs tabular-nums text-muted">{data.status || "—"}</span>
      </div>
      <div className="mt-1 line-clamp-2 text-xs font-medium leading-snug">{clip(data.title, 80)}</div>
      <div className="mt-1.5 flex items-center justify-between text-xs tabular-nums text-muted">
        <span className="truncate">{data.model || "—"}</span>
        <span className="shrink-0">
          {data.iters ? `${data.iters} it` : ""} · {money(data.spentMc)}
        </span>
      </div>
      <Handle type="source" position={Position.Bottom} className="!bg-muted" />
    </div>
  );
}

const nodeTypes = { agent: AgentNodeView };

// DelegationGraph renders a run and its sub-agent fan-out as a live node graph:
// who delegated to whom, each agent's status, model, iterations and spend. A
// magnificent way to watch a multi-agent run unfold. Click a node to select that
// agent (the caller opens its steer cockpit).
export function DelegationGraph({
  runs,
  rootId,
  onSelect,
  selectedId,
}: {
  runs: RunNode[];
  rootId: string;
  onSelect?: (id: string) => void;
  selectedId?: string;
}) {
  const { nodes, edges } = useMemo(() => {
    const t = buildDelegationTree(runs, rootId);
    const nodes: DelegationRFNode[] = t.nodes.map((n) => ({
      id: n.id,
      type: "agent",
      position: { x: n.x, y: n.y },
      selected: n.id === selectedId,
      data: {
        title: n.intent || n.id,
        status: n.status || "",
        model: n.model || "",
        iters: Number(n.iters || 0),
        spentMc: Number(n.spentMc || 0),
        root: n.root,
        depth: n.depth,
        selected: n.id === selectedId,
      },
    }));
    const edges: Edge[] = t.edges.map((e, i) => ({
      id: `${e.from}->${e.to}-${i}`,
      source: e.from,
      target: e.to,
      markerEnd: { type: MarkerType.ArrowClosed },
      style: { stroke: "var(--accent)", strokeWidth: 1.5 },
      animated: true,
    }));
    return { nodes, edges };
  }, [runs, rootId, selectedId]);

  if (nodes.length === 0) {
    return <div className="flex h-full items-center justify-center text-muted">no run selected</div>;
  }
  return (
    <ReactFlow
      nodes={nodes}
      edges={edges}
      nodeTypes={nodeTypes}
      fitView
      proOptions={{ hideAttribution: true }}
      nodesConnectable={false}
      elementsSelectable={false}
      onNodeClick={(_, node) => onSelect?.(node.id)}
      minZoom={0.2}
    >
      <Background gap={16} color="var(--border)" />
      <Controls showInteractive={false} />
    </ReactFlow>
  );
}
