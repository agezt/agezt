import { useMemo } from "react";
import { ReactFlow, Background, Controls, MarkerType, type Node, type Edge } from "@xyflow/react";
import { clip } from "@/lib/utils";

interface Entity {
  id: string;
  name?: string;
  kind?: string;
  weight?: number;
}
interface WorldEdge {
  from: string;
  to: string;
  verb?: string;
}

const MAX = 40;

// WorldGraph lays entities on a circle and draws relations between them — a
// node-link view of the world model, rendered with React Flow (CSP-safe, no
// external libs beyond the bundled React Flow).
export function WorldGraph({ entities, edges }: { entities: Entity[]; edges: WorldEdge[] }) {
  const { nodes, rfEdges } = useMemo(() => {
    const ents = (entities || []).filter((e) => e && e.id).slice(0, MAX);
    const n = ents.length;
    const R = Math.max(140, n * 22);
    const present = new Set(ents.map((e) => e.id));
    const nodes: Node[] = ents.map((e, i) => {
      const a = (2 * Math.PI * i) / n - Math.PI / 2;
      return {
        id: e.id,
        position: { x: R * Math.cos(a), y: R * Math.sin(a) },
        data: { label: clip((e.kind ? e.kind + ": " : "") + (e.name || e.id), 22) },
        style: {
          fontSize: 11,
          padding: 6,
          borderRadius: 8,
          border: "1px solid var(--accent)",
          background: "var(--card)",
          color: "var(--foreground)",
          width: 150,
        },
      };
    });
    const rfEdges: Edge[] = (edges || [])
      .filter((ed) => ed && present.has(ed.from) && present.has(ed.to) && ed.from !== ed.to)
      .map((ed, i) => ({
        id: `${ed.from}->${ed.to}-${i}`,
        source: ed.from,
        target: ed.to,
        label: ed.verb,
        markerEnd: { type: MarkerType.ArrowClosed },
        style: { stroke: "var(--border)" },
      }));
    return { nodes, rfEdges };
  }, [entities, edges]);

  if (nodes.length < 2) {
    return <div className="flex h-full items-center justify-center text-muted">not enough entities to graph</div>;
  }
  return (
    <ReactFlow
      nodes={nodes}
      edges={rfEdges}
      fitView
      proOptions={{ hideAttribution: true }}
      nodesConnectable={false}
      elementsSelectable={false}
      minZoom={0.1}
    >
      <Background gap={16} color="var(--border)" />
      <Controls showInteractive={false} />
    </ReactFlow>
  );
}
