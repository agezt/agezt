import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  ReactFlow,
  Background,
  Controls,
  Handle,
  Position,
  MarkerType,
  addEdge,
  useNodesState,
  useEdgesState,
  type Node as RFNode,
  type Edge as RFEdge,
  type NodeProps,
  type Connection,
} from "@xyflow/react";
import {
  Network,
  RefreshCw,
  Plus,
  X,
  Play,
  Trash2,
  Save,
  ArrowLeft,
  Power,
  PowerOff,
  Sparkles,
  History,
  Zap,
  Wrench,
  GitBranch,
  Wand2,
  Clock,
  Globe,
  Code2,
  List,
  Filter as FilterIcon,
  Split,
  Merge as MergeIcon,
  ShieldCheck,
  Workflow as WorkflowIcon,
  type LucideIcon,
} from "lucide-react";
import { getJSON, postAction, postJSON } from "@/lib/api";
import { cn, clip } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { useUI } from "@/components/ui/feedback";
import { SkeletonList } from "@/components/ui/skeleton";
import { EmptyState } from "@/components/ui/empty";
import { Badge } from "@/components/ui/badge";
import { PageHeader } from "@/components/ui/page-header";
import { ErrorText } from "@/components/JsonView";
import { useEvents } from "@/lib/events";

// ---- graph model (mirrors kernel/workflow) ----------------------------------

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

// WfSettings is the per-node reliability tuple the panel edits.
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
}

// Node-type palette: color accents + which extra OUTPUT ports a node offers.
// "out" is the default port (wire port "" on save); failable nodes add an
// "error" branch; condition/switch declare their own.
// Each node type carries a border accent (class), an icon, and a color (a CSS
// value usable in inline style) so the canvas reads as a colourful flowchart —
// coloured header band + icon per type — rather than a grid of grey boxes.
export const NODE_META: Record<string, { label: string; accent: string; icon: LucideIcon; color: string }> = {
  trigger: { label: "Trigger", accent: "border-good", icon: Zap, color: "var(--good)" },
  tool: { label: "Tool", accent: "border-accent", icon: Wrench, color: "var(--accent)" },
  llm: { label: "LLM", accent: "border-[#a78bfa]", icon: Sparkles, color: "#a78bfa" },
  condition: { label: "If/Else", accent: "border-warn", icon: GitBranch, color: "var(--warn)" },
  transform: { label: "Transform", accent: "border-accent", icon: Wand2, color: "var(--accent)" },
  delay: { label: "Delay", accent: "border-[#f472b6]", icon: Clock, color: "#f472b6" },
  http: { label: "HTTP", accent: "border-[#60a5fa]", icon: Globe, color: "#60a5fa" },
  code: { label: "Code", accent: "border-[#34d399]", icon: Code2, color: "#34d399" },
  map: { label: "Map", accent: "border-accent", icon: List, color: "var(--accent)" },
  filter: { label: "Filter", accent: "border-accent", icon: FilterIcon, color: "var(--accent)" },
  switch: { label: "Switch", accent: "border-warn", icon: Split, color: "var(--warn)" },
  merge: { label: "Merge", accent: "border-[#fbbf24]", icon: MergeIcon, color: "#fbbf24" },
  approval: { label: "Approval", accent: "border-warn", icon: ShieldCheck, color: "var(--warn)" },
  subworkflow: { label: "Sub-Workflow", accent: "border-[#818cf8]", icon: WorkflowIcon, color: "#818cf8" },
};

const FAILABLE = new Set(["tool", "llm", "http", "code", "approval", "subworkflow"]);

// portsForNode lists a node's SOURCE ports in display order. Pure +
// unit-tested ("out" stands for the kernel's default "" port).
export function portsForNode(type: string, config?: Record<string, unknown>): string[] {
  if (type === "condition") return ["true", "false"];
  if (type === "switch") {
    const cases = (config?.cases as { port?: string }[] | undefined) || [];
    const ports = cases.map((c) => c.port || "").filter(Boolean);
    return [...ports, "default"];
  }
  const ports = ["out"];
  if (FAILABLE.has(type)) ports.push("error");
  return ports;
}

// summarize renders a node's one-line config gist for the canvas card. Pure +
// unit-tested.
export function summarize(type: string, config?: Record<string, unknown>): string {
  const c = config || {};
  const s = (k: string) => String(c[k] ?? "");
  switch (type) {
    case "trigger": {
      const kind = s("kind") || "manual";
      if (kind === "cron") return c.interval_sec ? `cron every ${c.interval_sec}s` : `cron daily ${s("daily_at")}`;
      if (kind === "event") return `event on ${s("subject")}`;
      if (kind === "webhook") return "webhook (POST /hooks/…)";
      return "manual";
    }
    case "tool":
      return s("tool");
    case "llm":
      return s("prompt");
    case "condition":
      return `${s("left")} ${s("op")} ${s("right")}`;
    case "transform":
      return s("template");
    case "delay":
      return `${s("seconds")}s`;
    case "http":
      return `${s("method")} ${s("url")}`;
    case "code":
      return s("language");
    case "map":
    case "filter":
      return s("items");
    case "switch":
      return s("value");
    case "merge":
      return s("mode") || "any";
    case "approval":
      return s("description");
    case "subworkflow":
      return s("workflow");
    default:
      return "";
  }
}

export function workflowExecutionContract(w: Pick<Wf, "trigger_kind" | "trigger_detail" | "enabled">): string {
  const state = w.enabled === false ? "disabled" : "enabled";
  const trigger = w.trigger_kind
    ? `${w.trigger_kind}${w.trigger_detail ? ` (${w.trigger_detail})` : ""}`
    : "manual/API";
  return `${state} reusable chain · trigger ${trigger} · runnable by user, agent, schedule, or webhook`;
}

export interface WorkflowRunContractSummary {
  label: string;
  detail: string;
  tone: "good" | "warn" | "muted";
}

export function workflowRunContractSummary(w: Pick<Wf, "trigger_kind" | "trigger_detail" | "enabled">): WorkflowRunContractSummary {
  const contract = workflowExecutionContract(w);
  if (w.enabled === false) {
    return {
      label: "draft chain",
      detail: `${contract} · auto triggers disarmed, manual/user/agent test runs still allowed`,
      tone: "muted",
    };
  }
  if (w.trigger_kind === "cron") {
    return {
      label: "scheduled chain",
      detail: `${contract} · cron trigger starts the graph without making it an agent identity`,
      tone: "warn",
    };
  }
  if (w.trigger_kind === "event" || w.trigger_kind === "webhook") {
    return {
      label: "reactive chain",
      detail: `${contract} · external/event wake starts the graph under workflow policy gates`,
      tone: "good",
    };
  }
  return {
    label: "manual/shared chain",
    detail: `${contract} · run on demand from user, agent, or schedule`,
    tone: "muted",
  };
}

export interface WorkflowInvocationPassport {
  label: string;
  detail: string;
  tone: "good" | "warn" | "muted";
}

export function workflowInvocationPassport(w: Pick<Wf, "trigger_kind" | "trigger_detail" | "enabled" | "node_count"> & { nodes?: WfNode[] }): WorkflowInvocationPassport {
  const trigger = w.trigger_kind
    ? `${w.trigger_kind}${w.trigger_detail ? ` (${w.trigger_detail})` : ""}`
    : "manual/API";
  const nodeCount = w.node_count ?? w.nodes?.length ?? 0;
  const nodeText = nodeCount > 0 ? `${nodeCount} node${nodeCount === 1 ? "" : "s"}` : "node count unknown";
  if (w.enabled === false) {
    return {
      label: "test-only invocation",
      detail: `${nodeText} · not an agent identity · auto trigger ${trigger} disarmed · user or agent can still run tests manually · past runs stay journaled`,
      tone: "muted",
    };
  }
  if (w.trigger_kind === "cron") {
    return {
      label: "cron-invoked graph",
      detail: `${nodeText} · cron starts the graph, not an agent · user, agent, schedule, or webhook may also run it through workflow policy · past runs stay journaled`,
      tone: "warn",
    };
  }
  if (w.trigger_kind === "event" || w.trigger_kind === "webhook") {
    return {
      label: "event-invoked graph",
      detail: `${nodeText} · event/webhook starts the graph, not an agent · user, agent, or schedule may also run it through workflow policy · past runs stay journaled`,
      tone: "good",
    };
  }
  return {
    label: "shared invocation graph",
    detail: `${nodeText} · not an agent identity · user, agent, schedule, or webhook can run it through workflow policy · past runs stay journaled`,
    tone: "muted",
  };
}

export function workflowIdentityBoundary(w: Pick<Wf, "trigger_kind" | "trigger_detail" | "enabled" | "node_count"> & { nodes?: WfNode[] }): WorkflowInvocationPassport {
  const nodeCount = w.node_count ?? w.nodes?.length ?? 0;
  const nodeText = nodeCount > 0 ? `${nodeCount} graph node${nodeCount === 1 ? "" : "s"}` : "graph nodes unknown";
  if (w.enabled === false) {
    return {
      label: "draft graph boundary",
      detail: `${nodeText} retained; disabled workflow owns no soul, memory, inbox, provider route, retry, or repair policy`,
      tone: "muted",
    };
  }
  if (w.trigger_kind === "cron") {
    return {
      label: "scheduled graph boundary",
      detail: `${nodeText}; cron runs the chain under daemon or invoking-agent authority, while identity and memory stay outside the workflow`,
      tone: "warn",
    };
  }
  if (w.trigger_kind === "event" || w.trigger_kind === "webhook") {
    return {
      label: "reactive graph boundary",
      detail: `${nodeText}; event/webhook wakes the saved chain through workflow policy, not an autonomous agent identity`,
      tone: "good",
    };
  }
  return {
    label: "shared graph boundary",
    detail: `${nodeText}; users, agents, schedules, and webhooks may invoke it without turning the workflow into an agent`,
    tone: "muted",
  };
}

// toFlow converts a stored workflow into React Flow nodes/edges. Pure +
// unit-tested. Unpositioned nodes get a simple grid so old graphs render.
export function toFlow(wf: Wf): { nodes: RFNode[]; edges: RFEdge[] } {
  const nodes: RFNode[] = (wf.nodes || []).map((n, i) => ({
    id: n.id,
    type: "wf",
    position: { x: n.x ?? (i % 4) * 240, y: n.y ?? Math.floor(i / 4) * 140 },
    data: {
      wfType: n.type,
      label: n.label || "",
      config: n.config || {},
      settings: {
        timeout_sec: n.timeout_sec,
        retries: n.retries,
        retry_delay_sec: n.retry_delay_sec,
      },
    },
  }));
  const edges: RFEdge[] = (wf.edges || []).map((e, i) => {
    // Animated flowing edges read as data in motion; the error branch is red so
    // failure paths are obvious at a glance, the rest ride the accent colour.
    const stroke = e.port === "error" ? "var(--bad)" : "var(--accent)";
    return {
      id: `e${i}-${e.from}-${e.to}`,
      source: e.from,
      target: e.to,
      sourceHandle: e.port || "out",
      label: e.port && e.port !== "out" ? e.port : undefined,
      animated: true,
      style: { stroke, strokeWidth: 2 },
      markerEnd: { type: MarkerType.ArrowClosed, color: stroke },
    };
  });
  return { nodes, edges };
}

// fromFlow converts the canvas back to the kernel's wire shape. Pure +
// unit-tested ("out" handle → default "" port).
export function fromFlow(
  name: string,
  description: string,
  rfNodes: RFNode[],
  rfEdges: RFEdge[],
): Wf {
  return {
    name,
    description,
    nodes: rfNodes.map((n) => {
      const d = n.data as { wfType: string; label: string; config: Record<string, unknown>; settings?: WfSettings };
      return {
        id: n.id,
        type: d.wfType,
        label: d.label || undefined,
        config: Object.keys(d.config || {}).length ? d.config : undefined,
        x: Math.round(n.position.x),
        y: Math.round(n.position.y),
        timeout_sec: d.settings?.timeout_sec || undefined,
        retries: d.settings?.retries || undefined,
        retry_delay_sec: d.settings?.retry_delay_sec || undefined,
      };
    }),
    edges: rfEdges.map((e) => ({
      from: e.source,
      to: e.target,
      port: e.sourceHandle && e.sourceHandle !== "out" ? e.sourceHandle : undefined,
    })),
  };
}

// ---- canvas node -------------------------------------------------------------

type WfNodeData = {
  wfType: string;
  label: string;
  config: Record<string, unknown>;
  settings?: WfSettings;
  status?: string;
};
type WfRFNode = RFNode<WfNodeData, "wf">;

const statusRing: Record<string, string> = {
  running: "!border-accent breathe",
  done: "bg-good/12 !border-good",
  failed: "bg-bad/12 !border-bad",
};

function WfNodeView({ data, selected }: NodeProps<WfRFNode>) {
  const meta = NODE_META[data.wfType];
  const accent = meta?.accent || "border-border";
  const color = meta?.color || "var(--muted)";
  const label = meta?.label || data.wfType;
  const Icon = meta?.icon;
  const ports = portsForNode(data.wfType, data.config);
  return (
    <div
      className={cn(
        "w-[200px] overflow-hidden rounded-lg border-2 bg-card transition-colors",
        accent,
        data.status && statusRing[data.status],
        selected && "ring-2 ring-accent/60",
      )}
    >
      {data.wfType !== "trigger" && <Handle type="target" position={Position.Top} className="!bg-muted" />}
      {/* Coloured header band: a tint of the type colour + icon, so the graph
          reads at a glance by colour and shape, not by reading labels. */}
      <div
        className="flex items-center gap-1.5 px-2.5 py-1 text-xs font-semibold tracking-wide uppercase"
        style={{ color, backgroundColor: `color-mix(in oklch, ${color} 14%, transparent)` }}
      >
        {Icon && <Icon className="size-3" />}
        <span className="truncate">{label}</span>
      </div>
      <div className="px-3 pb-3 pt-1.5">
        <div className="truncate text-xs font-semibold">{clip(data.label || data.wfType, 26)}</div>
        <div className="truncate text-xs text-muted">{clip(summarize(data.wfType, data.config), 30)}</div>
      {ports.map((p, i) => (
        <Handle
          key={p}
          id={p}
          type="source"
          position={Position.Bottom}
          style={{ left: `${((i + 1) / (ports.length + 1)) * 100}%` }}
          className={cn("!bg-muted", p === "error" && "!bg-bad")}
          title={p}
        />
      ))}
      {ports.length > 1 && (
        <div className="mt-1 flex justify-around text-[8px] text-muted">
          {ports.map((p) => (
            <span key={p} className={cn(p === "error" && "text-bad")}>
              {p}
            </span>
          ))}
        </div>
      )}
      </div>
    </div>
  );
}

const nodeTypes = { wf: WfNodeView };

// ---- per-type config forms ----------------------------------------------------

type FieldKind = "text" | "textarea" | "number" | "select" | "json" | "bool";
interface FieldSpec {
  key: string;
  label: string;
  kind: FieldKind;
  options?: string[];
  placeholder?: string;
}

const FIELD_SPECS: Record<string, FieldSpec[]> = {
  trigger: [
    { key: "kind", label: "Kind", kind: "select", options: ["manual", "cron", "event", "webhook"] },
    { key: "interval_sec", label: "Interval seconds (cron)", kind: "number", placeholder: "e.g. 300" },
    { key: "daily_at", label: "Daily at HH:MM (cron)", kind: "text", placeholder: "09:00" },
    { key: "subject", label: "Subject glob (event)", kind: "text", placeholder: "task.failed | board.dm.* | memory.>" },
    {
      key: "secret",
      label: "Secret (webhook, ≥12 chars) — callers POST /hooks/<name> with X-Agezt-Secret",
      kind: "text",
      placeholder: "long-random-string",
    },
    {
      key: "reply",
      label: "Reply mode (webhook) — the caller waits and receives the run's outputs",
      kind: "bool",
    },
  ],
  tool: [
    { key: "tool", label: "Tool name", kind: "text", placeholder: "shell, memory, forge_x, mcp_x_y…" },
    { key: "args", label: "Args (JSON, templated)", kind: "json", placeholder: '{"command":"echo {{trigger.payload.x}}"}' },
  ],
  llm: [
    { key: "prompt", label: "Prompt (templated)", kind: "textarea", placeholder: "Summarize: {{fetch.output}}" },
    { key: "system", label: "System (optional)", kind: "textarea" },
    { key: "model", label: "Model (blank = default)", kind: "text" },
  ],
  condition: [
    { key: "left", label: "Left (templated)", kind: "text", placeholder: "{{check.output.score}}" },
    { key: "op", label: "Op", kind: "select", options: ["equals", "not_equals", "contains", "not_empty", "empty", "gt", "lt"] },
    { key: "right", label: "Right", kind: "text" },
  ],
  transform: [{ key: "template", label: "Template", kind: "textarea", placeholder: "got: {{fetch.output}}" }],
  delay: [{ key: "seconds", label: "Seconds (≤600)", kind: "number" }],
  http: [
    { key: "method", label: "Method", kind: "select", options: ["GET", "POST"] },
    { key: "url", label: "URL (templated)", kind: "text", placeholder: "https://api.example.com/{{trigger.payload.path}}" },
    { key: "headers", label: "Headers (JSON)", kind: "json", placeholder: '{"Accept":"application/json"}' },
    { key: "body", label: "Body (templated)", kind: "textarea" },
  ],
  code: [
    { key: "language", label: "Language", kind: "select", options: ["python", "node", "deno"] },
    { key: "code", label: "Code (input arrives in ./stdin.txt; print the result)", kind: "textarea" },
    { key: "input", label: "Input (templated JSON)", kind: "text", placeholder: '{"n": {{trigger.payload.n}}}' },
  ],
  map: [
    { key: "items", label: "Items path", kind: "text", placeholder: "{{fetch.output.items}}" },
    { key: "template", label: "Per-item template ({{item}}, {{index}})", kind: "textarea" },
  ],
  filter: [
    { key: "items", label: "Items path", kind: "text", placeholder: "{{fetch.output.items}}" },
    { key: "left", label: "Left ({{item}} usable)", kind: "text", placeholder: "{{item.score}}" },
    { key: "op", label: "Op", kind: "select", options: ["equals", "not_equals", "contains", "not_empty", "empty", "gt", "lt"] },
    { key: "right", label: "Right", kind: "text" },
  ],
  switch: [
    { key: "value", label: "Value (templated)", kind: "text", placeholder: "{{trigger.payload.team}}" },
    { key: "cases", label: 'Cases (JSON: [{"equals":"x","port":"p"}])', kind: "json" },
  ],
  merge: [{ key: "mode", label: "Mode", kind: "select", options: ["any", "all"] }],
  approval: [
    { key: "description", label: "What the operator reads (templated)", kind: "textarea" },
    { key: "capability", label: "Capability label (optional)", kind: "text" },
  ],
  subworkflow: [
    { key: "workflow", label: "Workflow name", kind: "text" },
    { key: "payload", label: "Payload (templated)", kind: "text", placeholder: '{"name":"{{trigger.payload.name}}"}' },
  ],
};

const inputCls =
  "rounded-md border border-border bg-panel px-2 py-1 text-sm text-foreground outline-none focus-visible:border-accent w-full";

// reliabilitySpecs lists the per-node settings the panel edits: a timeout
// for anything that does work, retries only where failure is transient by
// nature (the failable set).
function reliabilitySpecs(wfType: string): FieldSpec[] {
  if (wfType === "trigger") return [];
  const out: FieldSpec[] = [
    { key: "timeout_sec", label: "Timeout seconds per attempt (0 = none)", kind: "number", placeholder: "30" },
  ];
  if (FAILABLE.has(wfType)) {
    out.push(
      { key: "retries", label: "Retries on failure (0..5)", kind: "number", placeholder: "2" },
      { key: "retry_delay_sec", label: "Delay between attempts seconds (0..60)", kind: "number", placeholder: "5" },
    );
  }
  return out;
}

// NodePanel edits the selected node's label + type-specific config +
// reliability settings, and shows the node's LAST RUN data (input/output/
// attempts) when a run touched it. JSON fields are edited as text and
// parsed on apply.
function NodePanel({
  node,
  runInfo,
  onApply,
  onTest,
  onDelete,
  onError,
}: {
  node: WfRFNode;
  runInfo?: WfRunNodeEvent;
  onApply: (label: string, config: Record<string, unknown>, settings: WfSettings) => void;
  /** run JUST this node with mock upstream data (M811); resolves when the probe returns */
  onTest?: (data: Record<string, unknown>) => Promise<void>;
  onDelete: () => void;
  onError: (msg: string) => void;
}) {
  const [testData, setTestData] = useState("");
  const [testing, setTesting] = useState(false);
  const specs = FIELD_SPECS[node.data.wfType] || [];
  const relSpecs = reliabilitySpecs(node.data.wfType);
  const seed = () => {
    const out: Record<string, string> = {};
    for (const f of specs) {
      const v = node.data.config[f.key];
      out[f.key] = f.kind === "json" ? (v ? JSON.stringify(v) : "") : v == null ? "" : String(v);
    }
    const s = node.data.settings || {};
    for (const f of relSpecs) {
      const v = s[f.key as keyof WfSettings];
      out[f.key] = v ? String(v) : "";
    }
    return out;
  };
  const [label, setLabel] = useState(node.data.label);
  const [vals, setVals] = useState<Record<string, string>>(seed);
  // Re-seed when another node is selected.
  useEffect(() => {
    setLabel(node.data.label);
    setVals(seed());
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [node.id]);

  function apply() {
    const config: Record<string, unknown> = {};
    for (const f of specs) {
      const raw = (vals[f.key] || "").trim();
      if (raw === "") continue;
      if (f.kind === "number") {
        const n = Number(raw);
        if (!Number.isFinite(n)) {
          onError(`${f.label}: not a number`);
          return;
        }
        config[f.key] = n;
      } else if (f.kind === "json") {
        try {
          config[f.key] = JSON.parse(raw);
        } catch {
          onError(`${f.label}: invalid JSON`);
          return;
        }
      } else if (f.kind === "bool") {
        if (raw === "true") config[f.key] = true; // false stays omitted (no zero-noise)
      } else {
        config[f.key] = raw;
      }
    }
    const settings: WfSettings = {};
    for (const f of relSpecs) {
      const raw = (vals[f.key] || "").trim();
      if (raw === "") continue;
      const n = Number(raw);
      if (!Number.isFinite(n) || n < 0) {
        onError(`${f.label}: not a number`);
        return;
      }
      settings[f.key as keyof WfSettings] = n;
    }
    onApply(label.trim(), config, settings);
  }

  const meta = NODE_META[node.data.wfType];
  return (
    <div className="flex w-72 shrink-0 flex-col gap-2 overflow-y-auto border-l border-border bg-card p-3">
      <div className="flex items-center justify-between">
        <span className="flex items-center gap-1.5 text-xs font-semibold">
          {meta?.icon && <meta.icon className="size-3.5" style={{ color: meta.color }} />}
          {meta?.label || node.data.wfType}
        </span>
        <span className="font-mono text-xs text-muted">{node.id}</span>
      </div>
      <label className="flex flex-col gap-1 text-[11px] text-muted">
        Label
        <input value={label} onChange={(e) => setLabel(e.target.value)} aria-label="Node label" className={inputCls} />
      </label>
      {specs.map((f) => (
        <label key={f.key} className="flex flex-col gap-1 text-[11px] text-muted">
          {f.label}
          {f.kind === "textarea" || f.kind === "json" ? (
            <textarea
              value={vals[f.key] || ""}
              onChange={(e) => setVals((s) => ({ ...s, [f.key]: e.target.value }))}
              rows={f.kind === "textarea" ? 4 : 2}
              placeholder={f.placeholder}
              aria-label={f.label}
              className={cn(inputCls, "resize-y font-mono text-xs")}
            />
          ) : f.kind === "select" || f.kind === "bool" ? (
            <select
              value={vals[f.key] || (f.kind === "bool" ? "false" : (f.options?.[0] ?? ""))}
              onChange={(e) => setVals((s) => ({ ...s, [f.key]: e.target.value }))}
              aria-label={f.label}
              className={inputCls}
            >
              {(f.kind === "bool" ? ["false", "true"] : f.options || []).map((o) => (
                <option key={o} value={o}>
                  {o}
                </option>
              ))}
            </select>
          ) : (
            <input
              value={vals[f.key] || ""}
              onChange={(e) => setVals((s) => ({ ...s, [f.key]: e.target.value }))}
              placeholder={f.placeholder}
              aria-label={f.label}
              className={inputCls}
            />
          )}
        </label>
      ))}
      {relSpecs.length > 0 && (
        <>
          <div className="mt-1 text-xs font-semibold tracking-wide text-muted uppercase">Reliability</div>
          {relSpecs.map((f) => (
            <label key={f.key} className="flex flex-col gap-1 text-[11px] text-muted">
              {f.label}
              <input
                value={vals[f.key] || ""}
                onChange={(e) => setVals((s) => ({ ...s, [f.key]: e.target.value }))}
                placeholder={f.placeholder}
                aria-label={f.label}
                className={inputCls}
              />
            </label>
          ))}
        </>
      )}
      <div className="mt-1 flex items-center justify-between">
        {node.data.wfType !== "trigger" ? (
          <Button size="sm" variant="ghost" aria-label="Delete node" onClick={onDelete}>
            <Trash2 className="h-3.5 w-3.5" /> Delete
          </Button>
        ) : (
          <span className="text-xs text-muted">the trigger is permanent</span>
        )}
        <Button size="sm" onClick={apply} aria-label="Apply node config">
          Apply
        </Button>
      </div>
      {onTest && node.data.wfType !== "trigger" && (
        <>
          <div className="mt-1 text-xs font-semibold tracking-wide text-muted uppercase">Test this node</div>
          <label className="flex flex-col gap-1 text-[11px] text-muted">
            Upstream data (JSON, optional) — node ids → their outputs
            <textarea
              value={testData}
              onChange={(e) => setTestData(e.target.value)}
              rows={3}
              placeholder={'{"trigger":{"payload":{"x":1}},"fetch":{"output":{"status":200}}}'}
              aria-label="Test data"
              className={cn(inputCls, "resize-y font-mono text-xs")}
            />
          </label>
          <Button
            size="sm"
            variant="ghost"
            disabled={testing}
            aria-label="Test node"
            onClick={async () => {
              let data: Record<string, unknown> = {};
              const raw = testData.trim();
              if (raw !== "") {
                try {
                  data = JSON.parse(raw);
                } catch {
                  onError("test data: invalid JSON");
                  return;
                }
              }
              setTesting(true);
              try {
                await onTest(data);
              } finally {
                setTesting(false);
              }
            }}
          >
            <Play className={cn("h-3.5 w-3.5", testing && "animate-pulse")} />
            {testing ? "Testing…" : "Test node"}
          </Button>
        </>
      )}
      {runInfo && (
        <div className="mt-2 space-y-1 rounded-md border border-border bg-panel p-2" aria-label="Last run data">
          <div className="flex items-center gap-2">
            <span className="text-xs font-semibold tracking-wide text-muted uppercase">Last run</span>
            <Badge variant={runInfo.ok !== false || runInfo.handled ? "good" : "bad"}>
              {runInfo.ok !== false ? "ok" : runInfo.handled ? "rescued" : "failed"}
            </Badge>
            {runInfo.port && <span className="font-mono text-xs text-muted">port {runInfo.port}</span>}
            {(runInfo.attempts ?? 1) > 1 && (
              <span className="text-xs text-warn">{runInfo.attempts} attempts</span>
            )}
          </div>
          {runInfo.input && (
            <div>
              <div className="text-xs text-muted">input</div>
              <pre className="max-h-24 overflow-auto rounded bg-card p-1 text-xs whitespace-pre-wrap">{runInfo.input}</pre>
            </div>
          )}
          {runInfo.output && (
            <div>
              <div className="text-xs text-muted">
                output{runInfo.output_truncated ? " (truncated)" : ""}
              </div>
              <pre className="max-h-32 overflow-auto rounded bg-card p-1 text-xs whitespace-pre-wrap">{runInfo.output}</pre>
            </div>
          )}
          {runInfo.error && <div className="text-xs text-bad">{runInfo.error}</div>}
        </div>
      )}
    </div>
  );
}

// ---- the view -----------------------------------------------------------------

let idCounter = 0;
function freshID(type: string, taken: Set<string>): string {
  for (;;) {
    idCounter++;
    const id = `${type}_${idCounter}`;
    if (!taken.has(id)) return id;
  }
}

// ---- copilot ------------------------------------------------------------------

// CopilotPanel (M802/M805): describe the workflow in plain language and the
// daemon's copilot designs a validated graph — or, when the canvas already
// holds a real graph, REFINE it: the copilot sees the current canvas (unsaved
// edits included) plus the change request and returns the full revision.
// Either way the result comes back UNSAVED — the canvas shows it for review,
// Save persists it. Exported for tests.
export function CopilotPanel({
  name,
  graph,
  onDraft,
  onError,
}: {
  name: string;
  /** the current canvas graph; when it has more than the trigger, Refine is offered */
  graph?: Wf;
  onDraft: (wf: Wf) => void;
  onError: (msg: string) => void;
}) {
  const [text, setText] = useState("");
  const [busy, setBusy] = useState(false);
  const canRefine = (graph?.nodes?.length ?? 0) > 1;

  async function send(mode: "draft" | "refine") {
    const description = text.trim();
    if (!description) {
      onError(mode === "refine" ? "describe the change first" : "describe the workflow first");
      return;
    }
    setBusy(true);
    try {
      const res =
        mode === "refine"
          ? await postJSON<{ workflow?: Wf }>("/api/workflows/refine", {
              workflow: graph,
              instruction: description,
            })
          : await postJSON<{ workflow?: Wf }>("/api/workflows/draft", { description, name });
      if (!res.workflow) throw new Error("the copilot returned no workflow");
      onDraft(res.workflow);
      setText("");
    } catch (e) {
      onError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="flex items-end gap-2 rounded-lg border border-accent/30 bg-card p-2">
      <label className="flex flex-1 flex-col gap-1 text-[11px] text-muted">
        {canRefine
          ? "Copilot — describe a change to refine the current canvas, or a whole new workflow to replace it (unsaved until you Save)"
          : "Copilot — describe what this workflow should do; the draft replaces the canvas (unsaved until you Save)"}
        <textarea
          value={text}
          onChange={(e) => setText(e.target.value)}
          placeholder={
            canRefine
              ? "e.g. add an approval step before the notify, and make the cron daily at 07:30"
              : "e.g. every morning at 09:00 fetch https://example.com/status, and if it doesn't contain OK, ask me for approval and then notify the team"
          }
          aria-label="Copilot description"
          rows={2}
          className={inputCls}
        />
      </label>
      {canRefine && (
        <Button size="sm" onClick={() => send("refine")} disabled={busy} aria-label="Refine with copilot">
          <Sparkles className={cn("h-3.5 w-3.5", busy && "animate-pulse")} />
          {busy ? "Working…" : "Refine canvas"}
        </Button>
      )}
      <Button
        size="sm"
        variant={canRefine ? "ghost" : "default"}
        onClick={() => send("draft")}
        disabled={busy}
        aria-label="Draft with copilot"
      >
        {!canRefine && <Sparkles className={cn("h-3.5 w-3.5", busy && "animate-pulse")} />}
        {busy ? "Working…" : canRefine ? "Draft fresh" : "Draft onto canvas"}
      </Button>
    </div>
  );
}

// WfTemplate is one built-in gallery entry (M807).
export interface WfTemplate {
  name: string;
  title: string;
  description: string;
  category: string;
  node_count?: number;
  workflow: Wf;
}

// ---- run history (M806) --------------------------------------------------------

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

export function workflowRunSourceLabel(run: Pick<WfRun, "runner" | "source" | "agent" | "schedule_id" | "standing_id" | "trigger_subject">): string {
  const runner = run.runner || run.source || "manual";
  if (runner === "agent" && run.agent) return `agent ${run.agent}`;
  if (runner === "schedule") return run.schedule_id ? `schedule ${run.schedule_id}` : "schedule";
  if (runner === "webhook") return run.trigger_subject || "webhook";
  if (runner === "standing") return run.standing_id ? `standing ${run.standing_id}` : "standing";
  if (run.source === "schedule") return run.schedule_id ? `schedule ${run.schedule_id}` : "schedule";
  return runner;
}

// runToStatus folds a past run's node events into the canvas status map —
// the same ok/handled rule the live SSE replay uses. Pure + unit-tested.
export function runToStatus(run: WfRun): Record<string, string> {
  const out: Record<string, string> = {};
  for (const ev of run.node_events || []) {
    out[ev.node] = ev.ok !== false || ev.handled === true ? "done" : "failed";
  }
  return out;
}

// RunsDrawer lists a workflow's past runs (folded from the journal) and lets
// the operator replay any of them on the canvas. Exported for tests.
export function RunsDrawer({
  name,
  onReplay,
  onError,
}: {
  name: string;
  onReplay: (run: WfRun) => void;
  onError: (msg: string) => void;
}) {
  const [runs, setRuns] = useState<WfRun[] | null>(null);

  const load = useCallback(async () => {
    try {
      const d = await getJSON<{ runs?: WfRun[] }>("/api/workflows/runs", { ref: name });
      setRuns(d.runs || []);
    } catch (e) {
      onError((e as Error).message);
      setRuns([]);
    }
  }, [name, onError]);

  useEffect(() => {
    load();
  }, [load]);

  return (
    <div className="glass rounded-xl p-2">
      <div className="mb-1 flex items-center gap-2">
        <span className="text-xs font-semibold tracking-wide text-muted uppercase">Run history</span>
        <Button size="sm" variant="ghost" onClick={load} aria-label="Refresh runs">
          <RefreshCw className="h-3 w-3" />
        </Button>
      </div>
      {runs === null && <div className="text-xs text-muted">loading…</div>}
      {runs !== null && runs.length === 0 && (
        <div className="text-xs text-muted">No runs yet — Save &amp; Run records the first arc in the journal.</div>
      )}
      <ul className="max-h-36 space-y-1 overflow-y-auto">
        {(runs || []).map((r) => {
          const when = r.started_ms ? new Date(r.started_ms).toLocaleString() : "?";
          const dur =
            r.finished_ms && r.started_ms ? ` · ${((r.finished_ms - r.started_ms) / 1000).toFixed(1)}s` : "";
          const source = workflowRunSourceLabel(r);
          return (
            <li key={r.correlation_id}>
              <button
                className="flex w-full items-center gap-2 rounded-md px-2 py-1 text-left text-xs hover:bg-accent/10"
                onClick={() => onReplay(r)}
                aria-label={`Replay run ${r.correlation_id}`}
              >
                <Badge variant={r.status === "completed" ? "good" : r.status === "failed" ? "bad" : "default"}>
                  {r.status}
                </Badge>
                <span className="text-muted">{when}</span>
                <span>
                  {(r.node_events || []).length} node(s){dur}
                </span>
                <span className="rounded bg-panel px-1.5 py-0.5 font-mono text-xs text-muted" title={`runner: ${source}`}>
                  {source}
                </span>
                {r.error && <span className="truncate text-bad">{clip(r.error, 40)}</span>}
                <span className="ml-auto font-mono text-xs text-muted">{clip(r.correlation_id, 18)}</span>
              </button>
            </li>
          );
        })}
      </ul>
    </div>
  );
}

export function Workflows() {
  const ui = useUI();
  const { subscribe } = useEvents();
  const [list, setList] = useState<Wf[] | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [creating, setCreating] = useState(false);
  const [newName, setNewName] = useState("");
  const [templates, setTemplates] = useState<WfTemplate[] | null>(null);
  const [fromTemplate, setFromTemplate] = useState("");

  // Editor state.
  const [editing, setEditing] = useState<Wf | null>(null);
  const [nodes, setNodes, onNodesChange] = useNodesState<RFNode>([]);
  const [edges, setEdges, onEdgesChange] = useEdgesState<RFEdge>([]);
  const [selected, setSelected] = useState<string | null>(null);
  const [running, setRunning] = useState(false);
  const [copilotOpen, setCopilotOpen] = useState(false);
  const [runsOpen, setRunsOpen] = useState(false);
  const [nodeStatus, setNodeStatus] = useState<Record<string, string>>({});
  // Per-node last-run data (M808): input/output/attempts snippets, from the
  // live SSE arc or a replayed historical run.
  const [nodeRunInfo, setNodeRunInfo] = useState<Record<string, WfRunNodeEvent>>({});
  const editingName = useRef<string | null>(null);
  editingName.current = editing?.name ?? null;
  // Stable handle for the SSE subscription (it must not re-subscribe per render).
  const uiRef = useRef(ui);
  uiRef.current = ui;

  async function reload() {
    setLoading(true);
    try {
      const d = await getJSON<{ workflows?: Wf[] }>("/api/workflows");
      setList(d.workflows || []);
      setErr(null);
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    reload();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // Live node status from the journal arc while a run is in flight.
  useEffect(
    () =>
      subscribe((ev) => {
        if (!editingName.current || ev.payload?.workflow !== editingName.current) {
          if (ev.kind === "workflow.node") return;
        }
        if (ev.kind === "workflow.node" && ev.payload?.workflow === editingName.current) {
          const ok = ev.payload.ok !== false || ev.payload.handled === true;
          setNodeStatus((s) => ({ ...s, [ev.payload.node]: ok ? "done" : "failed" }));
          setNodeRunInfo((s) => ({ ...s, [ev.payload.node]: ev.payload as WfRunNodeEvent }));
        }
        if (ev.kind === "workflow.started" && ev.subject === "workflow." + editingName.current) {
          setNodeStatus({});
          setNodeRunInfo({});
        }
        // Async run terminals (M810): the run we fired (or any external
        // trigger's) finishing lands as a toast + releases the Run button.
        if (ev.subject === "workflow." + editingName.current) {
          if (ev.kind === "workflow.completed") {
            setRunning(false);
            uiRef.current.toast(
              `run completed — ${((ev.payload?.executed as string[]) || []).length} node(s)`,
              "success",
            );
          } else if (ev.kind === "workflow.failed") {
            setRunning(false);
            uiRef.current.toast(`run failed: ${ev.payload?.error ?? "see the journal"}`, "error");
          }
        }
      }),
    [subscribe],
  );

  // Reflect status into node data so the canvas recolors.
  useEffect(() => {
    setNodes((ns) =>
      ns.map((n) => {
        const st = nodeStatus[n.id];
        const data = n.data as WfNodeData;
        if (data.status === st) return n;
        return { ...n, data: { ...data, status: st } };
      }),
    );
  }, [nodeStatus, setNodes]);

  // The gallery loads lazily, the first time the create form opens.
  useEffect(() => {
    if (!creating || templates !== null) return;
    getJSON<{ templates?: WfTemplate[] }>("/api/workflows/templates")
      .then((d) => setTemplates(d.templates || []))
      .catch(() => setTemplates([]));
  }, [creating, templates]);

  function openEditor(wf: Wf) {
    const { nodes: ns, edges: es } = toFlow(wf);
    setNodes(ns);
    setEdges(es);
    setSelected(null);
    setNodeStatus({});
    setEditing(wf);
  }

  async function openByName(name: string) {
    try {
      const d = await getJSON<{ workflow?: Wf }>("/api/workflows/show", { ref: name });
      if (d.workflow) openEditor(d.workflow);
    } catch (e) {
      ui.toast((e as Error).message, "error");
    }
  }

  function startNew() {
    const name = newName.trim();
    if (!/^[a-z0-9][a-z0-9._-]{0,63}$/.test(name)) {
      ui.toast("name must be lowercase letters/digits/dots/dashes", "error");
      return;
    }
    setCreating(false);
    setNewName("");
    const tpl = templates?.find((t) => t.name === fromTemplate);
    setFromTemplate("");
    if (tpl) {
      // Instantiate: the template's graph under the new name, UNSAVED —
      // review on the canvas, Save persists.
      openEditor({ ...tpl.workflow, name, id: undefined, enabled: undefined });
      ui.toast(`canvas loaded from template "${tpl.title}" — review, then Save`, "info");
      return;
    }
    openEditor({
      name,
      nodes: [{ id: "start", type: "trigger", x: 0, y: 0 }],
      edges: [],
    });
  }

  function addNode(type: string) {
    const taken = new Set(nodes.map((n) => n.id));
    const id = freshID(type, taken);
    setNodes((ns) => [
      ...ns,
      {
        id,
        type: "wf",
        position: { x: 120 + ns.length * 30, y: 120 + ns.length * 30 },
        data: { wfType: type, label: "", config: {} },
      },
    ]);
    setSelected(id);
  }

  const onConnect = useCallback(
    (c: Connection) =>
      setEdges((es) =>
        addEdge(
          {
            ...c,
            label: c.sourceHandle && c.sourceHandle !== "out" ? c.sourceHandle : undefined,
            markerEnd: { type: MarkerType.ArrowClosed },
          },
          es,
        ),
      ),
    [setEdges],
  );

  async function saveGraph(): Promise<boolean> {
    if (!editing) return false;
    const wf = fromFlow(editing.name, editing.description || "", nodes, edges);
    try {
      await postJSON("/api/workflows/save", { workflow: wf });
      ui.toast(`${editing.name} saved`, "success");
      reload();
      return true;
    } catch (e) {
      ui.toast((e as Error).message, "error");
      return false;
    }
  }

  async function runGraph() {
    if (!editing || running) return;
    if (!(await saveGraph())) return; // run what you see
    setRunning(true);
    setNodeStatus({});
    setNodeRunInfo({});
    try {
      // Async (M810): the daemon accepts immediately and the canvas follows
      // the run live on the SSE arc — a long run is no longer hostage to
      // the HTTP proxy's timeout. Completion lands as a toast from the
      // workflow.completed/failed subscription below.
      await postJSON("/api/workflows/run", { ref: editing.name, payload: {}, async: true });
      ui.toast("run started — watching it live on the canvas", "info");
    } catch (e) {
      ui.toast((e as Error).message, "error");
      setRunning(false);
    }
  }

  const selectedNode = useMemo(
    () => (selected ? (nodes.find((n) => n.id === selected) as WfRFNode | undefined) : undefined),
    [selected, nodes],
  );

  // ---- editor mode ----
  if (editing) {
    return (
      <div className="flex h-[calc(100vh-160px)] min-h-[480px] flex-col gap-2">
        <div className="flex flex-wrap items-center gap-2">
          <Button size="sm" variant="ghost" aria-label="Back to list" onClick={() => setEditing(null)}>
            <ArrowLeft className="h-3.5 w-3.5" />
          </Button>
          <Network className="h-4 w-4 text-accent" />
          <span className="font-mono text-sm">{editing.name}</span>
          <span className="text-xs text-muted">
            {nodes.length} node(s) · {edges.length} edge(s)
          </span>
          <span className="ml-auto flex items-center gap-1.5">
            <Button
              size="sm"
              variant="ghost"
              onClick={() => setCopilotOpen((v) => !v)}
              aria-label="Toggle copilot"
            >
              <Sparkles className="h-3.5 w-3.5" /> Copilot
            </Button>
            <Button size="sm" variant="ghost" onClick={() => setRunsOpen((v) => !v)} aria-label="Toggle run history">
              <History className="h-3.5 w-3.5" /> Runs
            </Button>
            <Button size="sm" variant="ghost" onClick={saveGraph} aria-label="Save workflow">
              <Save className="h-3.5 w-3.5" /> Save
            </Button>
            <Button size="sm" onClick={runGraph} disabled={running} aria-label="Run workflow">
              <Play className={cn("h-3.5 w-3.5", running && "animate-pulse")} />
              {running ? "Running…" : "Save & Run"}
            </Button>
          </span>
        </div>
        {copilotOpen && (
          <CopilotPanel
            name={editing.name}
            graph={fromFlow(editing.name, editing.description || "", nodes, edges)}
            onDraft={(wf) => {
              const { nodes: ns, edges: es } = toFlow(wf);
              setNodes(ns);
              setEdges(es);
              setSelected(null);
              setNodeStatus({});
              setEditing((cur) => (cur ? { ...cur, description: wf.description || cur.description } : cur));
              ui.toast(
                `copilot drafted ${wf.nodes?.length ?? 0} node(s) — review the canvas, then Save`,
                "success",
              );
            }}
            onError={(msg) => ui.toast(msg, "error")}
          />
        )}
        {runsOpen && (
          <RunsDrawer
            name={editing.name}
            onReplay={(run) => {
              setNodeStatus(runToStatus(run));
              const info: Record<string, WfRunNodeEvent> = {};
              for (const ev of run.node_events || []) info[ev.node] = ev;
              setNodeRunInfo(info);
              const dur =
                run.finished_ms && run.started_ms ? ` in ${((run.finished_ms - run.started_ms) / 1000).toFixed(1)}s` : "";
              ui.toast(`replaying ${run.status} run${dur} — ${(run.node_events || []).length} node event(s)`, "info");
            }}
            onError={(msg) => ui.toast(msg, "error")}
          />
        )}
        <div className="flex min-h-0 flex-1 rounded-lg border border-border">
          <div className="flex w-36 shrink-0 flex-col gap-1 overflow-y-auto border-r border-border bg-card p-2">
            <div className="mb-1 text-xs font-semibold tracking-wide text-muted uppercase">Nodes</div>
            {Object.entries(NODE_META)
              .filter(([t]) => t !== "trigger")
              .map(([t, m]) => {
                const Icon = m.icon;
                return (
                  <button
                    key={t}
                    onClick={() => addNode(t)}
                    aria-label={`Add ${m.label} node`}
                    className={cn(
                      "flex items-center gap-1.5 rounded-md border-2 bg-panel px-2 py-1 text-left text-[11px] transition-colors hover:bg-accent/10",
                      m.accent,
                    )}
                  >
                    <Icon className="size-3 shrink-0" style={{ color: m.color }} />
                    {m.label}
                  </button>
                );
              })}
          </div>
          <div className="min-w-0 flex-1">
            <ReactFlow
              nodes={nodes}
              edges={edges}
              nodeTypes={nodeTypes}
              onNodesChange={onNodesChange}
              onEdgesChange={onEdgesChange}
              onConnect={onConnect}
              onNodeClick={(_, n) => setSelected(n.id)}
              onPaneClick={() => setSelected(null)}
              fitView
              deleteKeyCode={["Delete", "Backspace"]}
              proOptions={{ hideAttribution: true }}
              minZoom={0.2}
            >
              <Background gap={16} color="var(--border)" />
              <Controls showInteractive={false} />
            </ReactFlow>
          </div>
          {selectedNode && (
            <NodePanel
              node={selectedNode}
              runInfo={nodeRunInfo[selectedNode.id]}
              onTest={async (data) => {
                // Probe JUST this node against the CURRENT canvas (unsaved
                // edits included); the result lands in the Last-run card via
                // the SSE event the probe publishes.
                const wf = fromFlow(editing.name, editing.description || "", nodes, edges);
                try {
                  const res = await postJSON<{ port?: string; attempts?: number }>(
                    "/api/workflows/test_node",
                    { workflow: wf, node: selectedNode.id, data },
                  );
                  const extra =
                    (res.port ? ` — port ${res.port}` : "") +
                    ((res.attempts ?? 1) > 1 ? ` — ${res.attempts} attempts` : "");
                  ui.toast(`node test ok${extra}`, "success");
                } catch (e) {
                  ui.toast((e as Error).message, "error");
                }
              }}
              onApply={(label, config, settings) => {
                setNodes((ns) =>
                  ns.map((n) =>
                    n.id === selectedNode.id
                      ? { ...n, data: { ...(n.data as WfNodeData), label, config, settings } }
                      : n,
                  ),
                );
                ui.toast("node updated — Save to persist", "success");
              }}
              onDelete={() => {
                setNodes((ns) => ns.filter((n) => n.id !== selectedNode.id));
                setEdges((es) => es.filter((e) => e.source !== selectedNode.id && e.target !== selectedNode.id));
                setSelected(null);
              }}
              onError={(msg) => ui.toast(msg, "error")}
            />
          )}
        </div>
      </div>
    );
  }

  // ---- list mode ----
  return (
    <div className="space-y-3">
      <PageHeader
        icon={Network}
        title="Workflows"
        actions={
          <>
            {list && <span className="text-xs text-muted">{list.length} workflow(s)</span>}
            <Button size="sm" variant="ghost" onClick={reload} disabled={loading} aria-label="Refresh">
              <RefreshCw className={cn("h-3.5 w-3.5", loading && "animate-spin")} />
            </Button>
            <Button size="sm" onClick={() => setCreating((v) => !v)}>
              {creating ? <X className="h-3.5 w-3.5" /> : <Plus className="h-3.5 w-3.5" />}
              {creating ? "Close" : "New workflow"}
            </Button>
          </>
        }
      />

      {creating && (
        <div className="flex items-end gap-2 rounded-lg border border-accent/30 bg-card p-3">
          <label className="flex flex-1 flex-col gap-1 text-[11px] text-muted">
            Name — permanent handle (lowercase)
            <input
              value={newName}
              onChange={(e) => setNewName(e.target.value)}
              placeholder="e.g. daily-triage"
              aria-label="Workflow name"
              className={inputCls}
            />
          </label>
          <label className="flex flex-col gap-1 text-[11px] text-muted">
            Start from
            <select
              value={fromTemplate}
              onChange={(e) => setFromTemplate(e.target.value)}
              aria-label="Start from template"
              className={inputCls}
            >
              <option value="">Blank canvas</option>
              {(templates || []).map((t) => (
                <option key={t.name} value={t.name}>
                  {t.title} ({t.node_count ?? t.workflow?.nodes?.length ?? "?"} nodes)
                </option>
              ))}
            </select>
          </label>
          <Button size="sm" onClick={startNew} aria-label="Create workflow">
            <Plus className="h-3.5 w-3.5" /> Open canvas
          </Button>
        </div>
      )}
      {creating && fromTemplate && (
        <p className="text-[11px] text-muted">
          {templates?.find((t) => t.name === fromTemplate)?.description}
        </p>
      )}

      {err && <ErrorText>{err}</ErrorText>}
      {!list && !err && <SkeletonList count={3} />}
      {list && list.length === 0 && !creating && (
        <EmptyState
          icon={Network}
          title="No workflows yet"
          hint="Create one and build it on the canvas — or save a graph JSON with `agt workflow save --file`."
        />
      )}

      <ul className="space-y-2">
        {(list || []).map((w) => {
          const contract = workflowExecutionContract(w);
          const runContract = workflowRunContractSummary(w);
          const invocation = workflowInvocationPassport(w);
          const identity = workflowIdentityBoundary(w);
          return (
          <li key={w.id || w.name} className="glass rounded-xl p-3">
            <div className="flex flex-wrap items-center gap-2">
              <button
                className="font-mono text-sm text-foreground hover:text-accent"
                onClick={() => openByName(w.name)}
                aria-label={`Open ${w.name}`}
              >
                {w.name}
              </button>
              <Badge variant={w.enabled ? "good" : "default"}>{w.enabled ? "enabled" : "disabled"}</Badge>
              {w.trigger_kind && (
                <span className="text-xs text-muted">
                  {w.trigger_kind}
                  {w.trigger_detail ? ` (${w.trigger_detail})` : ""}
                </span>
              )}
              <span className="text-xs text-muted">{w.node_count ?? "?"} node(s)</span>
              <span className="ml-auto flex items-center gap-1">
                <Button
                  size="sm"
                  variant="ghost"
                  aria-label={w.enabled ? `Disable ${w.name}` : `Enable ${w.name}`}
                  onClick={async () => {
                    try {
                      await postAction("/api/workflows/enable", { ref: w.name, enabled: w.enabled ? "false" : "true" });
                      reload();
                    } catch (e) {
                      ui.toast((e as Error).message, "error");
                    }
                  }}
                >
                  {w.enabled ? <Power className="h-3.5 w-3.5" /> : <PowerOff className="h-3.5 w-3.5" />}
                </Button>
                <Button
                  size="sm"
                  variant="ghost"
                  aria-label={`Remove ${w.name}`}
                  onClick={async () => {
                    if (
                      !(await ui.confirm({
                        title: `Remove workflow ${w.name}?`,
                        message: "The graph is deleted; past runs stay in the journal.",
                        confirmLabel: "Remove",
                        danger: true,
                      }))
                    )
                      return;
                    try {
                      await postAction("/api/workflows/remove", { ref: w.name });
                      ui.toast(`${w.name} removed`, "success");
                      reload();
                    } catch (e) {
                      ui.toast((e as Error).message, "error");
                    }
                  }}
                >
                  <Trash2 className="h-3.5 w-3.5" />
                </Button>
              </span>
            </div>
            <div
              className={cn(
                "mt-2 flex min-w-0 items-center gap-1.5 rounded-md border px-2 py-1 text-[11px] text-muted",
                runContract.tone === "good"
                  ? "border-good/30 bg-good/5"
                  : runContract.tone === "warn"
                    ? "border-warn/30 bg-warn/5"
                    : "border-border/60 bg-panel/40",
              )}
              title={runContract.detail}
            >
              <WorkflowIcon className={cn("size-3 shrink-0", runContract.tone === "good" ? "text-good" : runContract.tone === "warn" ? "text-warn" : "text-accent")} />
              <span className="font-semibold text-foreground/70">{runContract.label}</span>
              <span className="truncate">{contract}</span>
            </div>
            <div
              className={cn(
                "mt-1.5 flex min-w-0 items-center gap-1.5 rounded-md border px-2 py-1 text-[11px] text-muted",
                invocation.tone === "good"
                  ? "border-good/25 bg-good/5"
                  : invocation.tone === "warn"
                    ? "border-warn/30 bg-warn/5"
                    : "border-border/60 bg-card/35",
              )}
              title={invocation.detail}
            >
              <Play className={cn("size-3 shrink-0", invocation.tone === "good" ? "text-good" : invocation.tone === "warn" ? "text-warn" : "text-accent")} />
              <span className="font-semibold text-foreground/70">{invocation.label}</span>
              <span className="truncate">{invocation.detail}</span>
            </div>
            <div
              className={cn(
                "mt-1.5 flex min-w-0 items-center gap-1.5 rounded-md border px-2 py-1 text-[11px] text-muted",
                identity.tone === "good"
                  ? "border-good/25 bg-good/5"
                  : identity.tone === "warn"
                    ? "border-warn/30 bg-warn/5"
                    : "border-border/60 bg-panel/35",
              )}
              title={identity.detail}
            >
              <ShieldCheck className={cn("size-3 shrink-0", identity.tone === "good" ? "text-good" : identity.tone === "warn" ? "text-warn" : "text-accent")} />
              <span className="font-semibold text-foreground/70">{identity.label}</span>
              <span className="truncate">{identity.detail}</span>
            </div>
            {w.description && <div className="mt-1 text-xs text-muted">{w.description}</div>}
          </li>
          );
        })}
      </ul>
    </div>
  );
}
