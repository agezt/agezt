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

interface PlanNode {
  id: string;
  kind?: string;
  deps?: string[];
  intent?: string;
  description?: string;
}
export interface Plan {
  name?: string;
  nodes?: PlanNode[];
}

type PlanNodeData = {
  label: string;
  kind: string;
  body: string;
  status?: string;
};
type PlanRFNode = Node<PlanNodeData, "plan">;

const NODE_W = 180;
const NODE_H = 56;
const GAP_X = 40;
const GAP_Y = 70;

const statusRing: Record<string, string> = {
  running: "border-accent bg-accent/15",
  done: "border-good bg-good/15",
  failed: "border-bad bg-bad/15",
};

// PlanNodeView is the custom React Flow node. Loop nodes are rounded boxes; gate
// nodes get a distinct amber accent + clipped corners so HITL stops stand out.
// The border/fill recolour live from run status (running/done/failed).
function PlanNodeView({ data }: NodeProps<PlanRFNode>) {
  const isGate = data.kind === "gate";
  const ring = data.status ? statusRing[data.status] : "";
  return (
    <div
      className={[
        "flex h-[56px] w-[180px] flex-col justify-center border-2 px-3 text-center transition-colors",
        isGate ? "border-warn" : "border-accent",
        ring,
        isGate ? "rounded-md [clip-path:polygon(8%_0,92%_0,100%_50%,92%_100%,8%_100%,0_50%)]" : "rounded-lg",
        "bg-card",
      ].join(" ")}
    >
      <Handle type="target" position={Position.Top} className="!bg-muted" />
      <div className="truncate text-xs font-semibold">{clip(data.label, 22)}</div>
      <div className="truncate text-xs text-muted">{clip(data.body, 28)}</div>
      <Handle type="source" position={Position.Bottom} className="!bg-muted" />
    </div>
  );
}

const nodeTypes = { plan: PlanNodeView };

// layout assigns each node a depth (longest dependency chain, cycle-safe via a
// bounded relaxation) and lays the graph out top-down.
function layout(plan: Plan, status: Record<string, string>): { nodes: PlanRFNode[]; edges: Edge[] } {
  const planNodes = (plan.nodes || []).filter((n) => n && n.id);
  if (!planNodes.length) return { nodes: [], edges: [] };
  const byId = new Map(planNodes.map((n) => [n.id, n]));

  const depth = new Map<string, number>();
  planNodes.forEach((n) => depth.set(n.id, 0));
  for (let pass = 0; pass < planNodes.length; pass++) {
    let changed = false;
    for (const n of planNodes) {
      for (const d of n.deps || []) {
        if (!byId.has(d)) continue;
        const cand = (depth.get(d) ?? 0) + 1;
        if ((depth.get(n.id) ?? 0) < cand) {
          depth.set(n.id, cand);
          changed = true;
        }
      }
    }
    if (!changed) break;
  }

  const layers: PlanNode[][] = [];
  for (const n of planNodes) {
    const d = depth.get(n.id) ?? 0;
    (layers[d] ||= []).push(n);
  }

  const nodes: PlanRFNode[] = [];
  layers.forEach((layer, li) => {
    const rowW = layer.length * NODE_W + (layer.length - 1) * GAP_X;
    layer.forEach((n, i) => {
      nodes.push({
        id: n.id,
        type: "plan",
        position: { x: i * (NODE_W + GAP_X) - rowW / 2, y: li * (NODE_H + GAP_Y) },
        data: {
          label: n.id,
          kind: n.kind || "?",
          body: `${n.kind || "?"}${n.intent || n.description ? ": " + (n.intent || n.description) : ""}`,
          status: status[n.id],
        },
      });
    });
  });

  const edges: Edge[] = [];
  for (const n of planNodes) {
    for (const d of n.deps || []) {
      if (!byId.has(d)) continue;
      edges.push({
        id: `${d}->${n.id}`,
        source: d,
        target: n.id,
        markerEnd: { type: MarkerType.ArrowClosed },
      });
    }
  }
  return { nodes, edges };
}

export function PlanDag({ plan, status }: { plan: Plan; status: Record<string, string> }) {
  const { nodes, edges } = useMemo(() => layout(plan, status), [plan, status]);
  if (!nodes.length) {
    return (
      <div className="flex h-full items-center justify-center text-muted">no nodes to draw</div>
    );
  }
  return (
    <ReactFlow
      nodes={nodes}
      edges={edges}
      nodeTypes={nodeTypes}
      fitView
      proOptions={{ hideAttribution: true }}
      nodesDraggable={false}
      nodesConnectable={false}
      elementsSelectable={false}
      minZoom={0.2}
    >
      <Background gap={16} color="var(--border)" />
      <Controls showInteractive={false} />
    </ReactFlow>
  );
}
