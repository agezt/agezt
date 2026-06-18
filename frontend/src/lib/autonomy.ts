import type { AgentEvent } from "@/lib/events";

export interface AutonomyItem {
  seq: number;
  ts_unix_ms?: number;
  kind: string;
  subject?: string;
  category: string;
  title: string;
  correlation_id?: string;
  detail?: string;
  agent?: string;
  target_agent?: string;
  delegate_to?: string;
  delegated_by?: string;
  root_agent?: string;
  chain_depth?: number;
  incident_id?: string;
  root_incident_id?: string;
  parent_incident_id?: string;
  phase?: string;
  mode?: string;
  resolution?: string;
  routing_task_type?: string;
  routing_task_model_chain?: string[];
  routing_force_generation?: number;
}

const AUTONOMY_KINDS = new Set([
  "schedule.fired",
  "standing.fired",
  "standing.created",
  "standing.error",
  "assure.verdict",
  "skill.created",
  "skill.promoted",
  "skill.quarantined",
  "skill.reverted",
  "briefing.sent",
  "board.posted",
]);

const AUTONOMY_SUBJECTS = new Set([
  "doctor.auto_repair",
  "agent.repair",
  "agent.wake",
  "agent.resolve",
]);

export function autonomyEventMatches(ev: AgentEvent): boolean {
  if (AUTONOMY_SUBJECTS.has(String(ev.subject || ""))) return true;
  return AUTONOMY_KINDS.has(String(ev.kind || ""));
}

export function filterDoctorAutonomy(
  items: AutonomyItem[] | null | undefined,
  limit = 8,
): AutonomyItem[] {
  return (items || [])
    .filter((item) => item.category === "doctor")
    .slice(0, limit);
}

export interface DoctorIncident {
  root: string;
  depth: number;
  latestTs?: number;
  items: AutonomyItem[];
}

export function doctorIncidentGroups(
  items: AutonomyItem[] | null | undefined,
  limit = 8,
): DoctorIncident[] {
  const groups = new Map<string, DoctorIncident>();
  for (const item of items || []) {
    if (item.category !== "doctor") continue;
    const root = String(item.root_agent || item.agent || "agent").trim();
    const depth = typeof item.chain_depth === "number" ? item.chain_depth : 0;
    const key = `${root}::${depth}`;
    const existing = groups.get(key) || { root, depth, latestTs: 0, items: [] };
    existing.items.push(item);
    existing.latestTs = Math.max(existing.latestTs || 0, item.ts_unix_ms || 0);
    groups.set(key, existing);
  }
  return Array.from(groups.values())
    .sort((a, b) => (b.latestTs || 0) - (a.latestTs || 0))
    .slice(0, limit);
}

export interface DoctorIncidentNode {
  id: string;
  parentId?: string;
  rootId: string;
  rootAgent: string;
  depth: number;
  latestTs?: number;
  items: AutonomyItem[];
  latest?: AutonomyItem;
  children: DoctorIncidentNode[];
}

export interface DoctorIncidentTree {
  id: string;
  rootAgent: string;
  latestTs?: number;
  roots: DoctorIncidentNode[];
}

export interface DoctorIncidentTreeOpsSummary {
  label: string;
  detail: string;
  tone: DoctorIncidentPhaseTone;
  hops: number;
  maxDepth: number;
  operatorEvents: number;
  failureEvents: number;
  latestPhase?: string;
}

export function doctorIncidentTrees(
  items: AutonomyItem[] | null | undefined,
  limit = 8,
): DoctorIncidentTree[] {
  const byTree = new Map<string, AutonomyItem[]>();
  for (const item of items || []) {
    if (item.category !== "doctor") continue;
    const treeId = doctorIncidentRootId(item);
    const rows = byTree.get(treeId) || [];
    rows.push(item);
    byTree.set(treeId, rows);
  }
  return Array.from(byTree.entries())
    .map(([id, rows]) => buildDoctorIncidentTree(id, rows))
    .sort((a, b) => (b.latestTs || 0) - (a.latestTs || 0))
    .slice(0, limit);
}

function buildDoctorIncidentTree(
  id: string,
  rows: AutonomyItem[],
): DoctorIncidentTree {
  const nodeMap = new Map<string, DoctorIncidentNode>();
  for (const row of rows) {
    const nodeId = doctorIncidentNodeId(row);
    const existing = nodeMap.get(nodeId) || {
      id: nodeId,
      parentId: doctorIncidentParentId(row),
      rootId: id,
      rootAgent: String(row.root_agent || row.agent || "agent").trim(),
      depth: typeof row.chain_depth === "number" ? row.chain_depth : 0,
      latestTs: 0,
      items: [],
      latest: undefined,
      children: [],
    };
    existing.items.push(row);
    if ((row.ts_unix_ms || 0) >= (existing.latestTs || 0)) {
      existing.latestTs = row.ts_unix_ms || 0;
      existing.latest = row;
      existing.parentId = doctorIncidentParentId(row);
      existing.rootAgent = String(row.root_agent || row.agent || "agent").trim();
      existing.depth = typeof row.chain_depth === "number" ? row.chain_depth : 0;
    }
    nodeMap.set(nodeId, existing);
  }
  for (const node of nodeMap.values()) {
    node.items.sort((a, b) => (b.ts_unix_ms || 0) - (a.ts_unix_ms || 0));
  }
  const roots: DoctorIncidentNode[] = [];
  for (const node of nodeMap.values()) {
    if (node.parentId && node.parentId !== node.id) {
      const parent = nodeMap.get(node.parentId);
      if (parent) {
        parent.children.push(node);
        continue;
      }
    }
    roots.push(node);
  }
  const sortNodes = (nodes: DoctorIncidentNode[]) => {
    nodes.sort((a, b) => (b.latestTs || 0) - (a.latestTs || 0));
    for (const node of nodes) sortNodes(node.children);
  };
  sortNodes(roots);
  return {
    id,
    rootAgent: roots[0]?.rootAgent || String(rows[0]?.root_agent || rows[0]?.agent || "agent").trim(),
    latestTs: Math.max(...rows.map((row) => row.ts_unix_ms || 0), 0),
    roots,
  };
}

export function doctorIncidentTreeOpsSummary(
  tree: DoctorIncidentTree,
): DoctorIncidentTreeOpsSummary {
  const nodes = flattenDoctorIncidentNodes(tree.roots);
  const items = nodes.flatMap((node) => node.items);
  const latest = items.reduce<AutonomyItem | undefined>(
    (best, item) =>
      !best || (item.ts_unix_ms || 0) >= (best.ts_unix_ms || 0) ? item : best,
    undefined,
  );
  const latestPhase = doctorIncidentPhase(latest);
  const failureEvents = items.filter(
    (item) => doctorIncidentPhase(item)?.tone === "bad",
  ).length;
  const operatorEvents = items.filter(
    (item) => doctorIncidentSource(item) === "operator",
  ).length;
  const maxDepth = nodes.reduce((max, node) => Math.max(max, node.depth), 0);
  const tone =
    failureEvents > 0 && latestPhase?.tone !== "good"
      ? "bad"
      : latestPhase?.tone || "muted";
  const label =
    tone === "bad"
      ? "needs owner"
      : tone === "good"
        ? "resolved"
        : operatorEvents > 0
          ? "operator touched"
          : "doctor active";
  const detail = [
    `${nodes.length} hop${nodes.length === 1 ? "" : "s"}`,
    `depth ${maxDepth}`,
    latestPhase ? `latest ${latestPhase.label}` : undefined,
    operatorEvents > 0 ? `${operatorEvents} operator` : undefined,
    failureEvents > 0 ? `${failureEvents} failure` : undefined,
  ]
    .filter(Boolean)
    .join(" / ");

  return {
    label,
    detail,
    tone,
    hops: nodes.length,
    maxDepth,
    operatorEvents,
    failureEvents,
    latestPhase: latestPhase?.label,
  };
}

function flattenDoctorIncidentNodes(
  nodes: DoctorIncidentNode[],
): DoctorIncidentNode[] {
  const out: DoctorIncidentNode[] = [];
  for (const node of nodes) {
    out.push(node);
    out.push(...flattenDoctorIncidentNodes(node.children));
  }
  return out;
}

function doctorIncidentNodeId(item: AutonomyItem): string {
  const root = String(item.root_agent || item.agent || "agent").trim();
  const depth = typeof item.chain_depth === "number" ? item.chain_depth : 0;
  return String(item.incident_id || `${doctorIncidentRootId(item)}::${root}::${depth}`);
}

function doctorIncidentRootId(item: AutonomyItem): string {
  const root = String(item.root_agent || item.agent || "agent").trim();
  return String(item.root_incident_id || item.incident_id || `${root}::root`);
}

function doctorIncidentParentId(item: AutonomyItem): string | undefined {
  const parent = String(item.parent_incident_id || "").trim();
  if (parent) return parent;
  const depth = typeof item.chain_depth === "number" ? item.chain_depth : 0;
  if (depth <= 0) return undefined;
  return String(item.root_incident_id || `${String(item.root_agent || item.agent || "agent").trim()}::root`);
}

export function doctorIncidentLabel(
  item: Pick<
    AutonomyItem,
    | "delegated_by"
    | "root_agent"
    | "chain_depth"
    | "delegate_to"
    | "incident_id"
    | "root_incident_id"
  >,
): string {
  const bits: string[] = [];
  if (item.delegated_by) bits.push(`delegated by ${item.delegated_by}`);
  if (item.root_agent) bits.push(`root ${item.root_agent}`);
  if (typeof item.chain_depth === "number" && item.chain_depth > 0)
    bits.push(`hop ${item.chain_depth}`);
  if (item.delegate_to) bits.push(`to ${item.delegate_to}`);
  return bits.join(" · ");
}

export function doctorIncidentNodeTitle(node: DoctorIncidentNode): string {
  const latest = node.latest;
  if (!latest) return node.rootAgent;
  return String(
    latest.delegate_to ||
      latest.target_agent ||
      (node.depth > 0 ? latest.delegated_by : latest.agent) ||
      node.rootAgent,
  ).trim();
}

export type DoctorIncidentSource = "doctor" | "operator";
export type DoctorIncidentPhaseTone =
  | "accent"
  | "warn"
  | "good"
  | "bad"
  | "muted";

export function doctorIncidentSource(
  item: Pick<AutonomyItem, "subject"> | null | undefined,
): DoctorIncidentSource {
  return String(item?.subject || "").trim().startsWith("agent.")
    ? "operator"
    : "doctor";
}

export function doctorIncidentSourceLabel(
  item: Pick<AutonomyItem, "subject" | "mode" | "phase"> | null | undefined,
): string {
  const source = doctorIncidentSource(item);
  if (source === "operator") return "operator";
  if (String(item?.mode || "").trim() === "degraded") return "doctor";
  if (String(item?.phase || "").trim().startsWith("delegation_")) return "doctor";
  return "doctor";
}

export function doctorIncidentPhase(
  item: Pick<AutonomyItem, "subject" | "mode" | "phase"> | null | undefined,
): { label: string; tone: DoctorIncidentPhaseTone } | null {
  const subject = String(item?.subject || "").trim();
  const mode = String(item?.mode || "").trim();
  const phase = String(item?.phase || "").trim();
  if (!phase) return null;
  if (subject === "agent.repair" || subject === "agent.wake" || subject === "agent.resolve") {
    switch (phase) {
      case "requested":
        return { label: "requested", tone: "accent" };
      case "completed":
        return { label: "completed", tone: "good" };
      case "failed":
        return { label: "failed", tone: "bad" };
      default:
        return { label: phase, tone: "muted" };
    }
  }
  switch (phase) {
    case "routing_forced_failed_detected":
      return { label: "forced failed", tone: "bad" };
    case "routing_force_exhausted_detected":
      return { label: "exhausted", tone: "bad" };
    case "routing_unstable_detected":
      return { label: "unstable", tone: "bad" };
    case "queued":
      return { label: mode === "routing" ? "routing" : "queued", tone: "warn" };
    case "routing_rollback_queued":
      return { label: "rollback", tone: "warn" };
    case "completed":
      return {
        label:
          mode === "degraded"
            ? "repaired"
            : mode === "routing"
              ? "rerouted"
              : "applied",
        tone: "good",
      };
    case "routing_rollback_completed":
      return { label: "rolled back", tone: "good" };
    case "failed":
      return { label: mode === "routing" ? "routing failed" : "failed", tone: "bad" };
    case "routing_rollback_failed":
      return { label: "rollback failed", tone: "bad" };
    case "escalation_woke":
      return { label: "owner woke", tone: "accent" };
    case "escalation_answered":
      return { label: "answered", tone: "good" };
    case "resolution_applied":
      return { label: "applied", tone: "good" };
    case "escalation_skipped":
      return { label: "skipped", tone: "muted" };
    case "escalation_failed":
      return { label: "owner failed", tone: "bad" };
    case "resolution_failed":
      return { label: "resolution failed", tone: "bad" };
    case "delegation_queued":
      return { label: "delegated", tone: "accent" };
    case "delegation_woke":
      return { label: "delegate woke", tone: "accent" };
    case "delegation_failed":
      return { label: "delegate failed", tone: "bad" };
    default:
      return { label: phase.replaceAll("_", " "), tone: "muted" };
  }
}
