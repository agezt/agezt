// Pure tidy-tree layout of a run and its sub-agent delegation subtree, derived
// from the runs list (parent_correlation links lead → sub-agents). No React —
// unit-tested directly. The kernel stays the source of truth.

export interface RunNode {
  id: string;
  parent?: string;
  status?: string;
  model?: string;
  spentMc?: number;
  iters?: number;
  intent?: string;
}

export interface LaidNode extends RunNode {
  depth: number;
  x: number;
  y: number;
  root: boolean;
}

export interface DelegationTree {
  nodes: LaidNode[];
  edges: { from: string; to: string }[];
  // Aggregates over the whole subtree (root included).
  count: number;
  totalSpentMc: number;
  maxDepth: number;
}

const GAP_X = 210;
const GAP_Y = 120;

// buildDelegationTree lays out the subtree rooted at rootId: the root run plus
// every descendant reached through parent_correlation. A classic tidy layout —
// leaves take successive horizontal slots; each parent is centred over its
// children — so even deep fan-outs read cleanly. Cycle-guarded.
export function buildDelegationTree(runs: RunNode[], rootId: string): DelegationTree {
  const byId = new Map<string, RunNode>();
  for (const r of runs) if (r.id) byId.set(r.id, r);

  const children = new Map<string, RunNode[]>();
  for (const r of runs) {
    if (r.parent && byId.has(r.parent)) {
      const arr = children.get(r.parent) || [];
      arr.push(r);
      children.set(r.parent, arr);
    }
  }

  const root = byId.get(rootId);
  if (!root) {
    return { nodes: [], edges: [], count: 0, totalSpentMc: 0, maxDepth: 0 };
  }

  const nodes: LaidNode[] = [];
  const edges: { from: string; to: string }[] = [];
  const seen = new Set<string>();
  let leaf = 0;
  let totalSpentMc = 0;
  let maxDepth = 0;

  // DFS returning the node's x slot so a parent can centre over its kids.
  function layout(node: RunNode, depth: number): number {
    if (seen.has(node.id)) return leaf; // cycle guard
    seen.add(node.id);
    totalSpentMc += Number(node.spentMc || 0);
    maxDepth = Math.max(maxDepth, depth);

    const kids = (children.get(node.id) || []).slice().sort((a, b) => a.id.localeCompare(b.id));
    let x: number;
    if (kids.length === 0) {
      x = leaf++;
    } else {
      const xs: number[] = [];
      for (const k of kids) {
        edges.push({ from: node.id, to: k.id });
        xs.push(layout(k, depth + 1));
      }
      x = (xs[0] + xs[xs.length - 1]) / 2;
    }
    nodes.push({
      ...node,
      depth,
      x: x * GAP_X,
      y: depth * GAP_Y,
      root: node.id === rootId,
    });
    return x;
  }
  layout(root, 0);

  return { nodes, edges, count: nodes.length, totalSpentMc, maxDepth };
}

// pickDefaultRoot chooses the run to show first: the newest run that actually
// has sub-agents (the most interesting tree), else the newest run overall.
export function pickDefaultRoot(runs: RunNode[]): string | undefined {
  const hasKids = new Set<string>();
  for (const r of runs) if (r.parent) hasKids.add(r.parent);
  // runs arrive newest-first from /api/runs; preserve that order.
  const withKids = runs.find((r) => hasKids.has(r.id));
  return (withKids || runs[0])?.id;
}
