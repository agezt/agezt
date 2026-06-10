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
} from "lucide-react";
import { getJSON, postAction, postJSON } from "@/lib/api";
import { cn, clip } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { useUI } from "@/components/ui/feedback";
import { SkeletonList } from "@/components/ui/skeleton";
import { EmptyState } from "@/components/ui/empty";
import { Badge } from "@/components/ui/badge";
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
export const NODE_META: Record<string, { label: string; accent: string }> = {
  trigger: { label: "Trigger", accent: "border-good" },
  tool: { label: "Tool", accent: "border-accent" },
  llm: { label: "LLM", accent: "border-[#a78bfa]" },
  condition: { label: "If/Else", accent: "border-warn" },
  transform: { label: "Transform", accent: "border-accent" },
  delay: { label: "Delay", accent: "border-[#f472b6]" },
  http: { label: "HTTP", accent: "border-[#60a5fa]" },
  code: { label: "Code", accent: "border-[#34d399]" },
  map: { label: "Map", accent: "border-accent" },
  filter: { label: "Filter", accent: "border-accent" },
  switch: { label: "Switch", accent: "border-warn" },
  merge: { label: "Merge", accent: "border-[#fbbf24]" },
  approval: { label: "Approval", accent: "border-warn" },
  subworkflow: { label: "Sub-Workflow", accent: "border-[#818cf8]" },
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

// toFlow converts a stored workflow into React Flow nodes/edges. Pure +
// unit-tested. Unpositioned nodes get a simple grid so old graphs render.
export function toFlow(wf: Wf): { nodes: RFNode[]; edges: RFEdge[] } {
  const nodes: RFNode[] = (wf.nodes || []).map((n, i) => ({
    id: n.id,
    type: "wf",
    position: { x: n.x ?? (i % 4) * 240, y: n.y ?? Math.floor(i / 4) * 140 },
    data: { wfType: n.type, label: n.label || "", config: n.config || {} },
  }));
  const edges: RFEdge[] = (wf.edges || []).map((e, i) => ({
    id: `e${i}-${e.from}-${e.to}`,
    source: e.from,
    target: e.to,
    sourceHandle: e.port || "out",
    label: e.port && e.port !== "out" ? e.port : undefined,
    markerEnd: { type: MarkerType.ArrowClosed },
  }));
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
      const d = n.data as { wfType: string; label: string; config: Record<string, unknown> };
      return {
        id: n.id,
        type: d.wfType,
        label: d.label || undefined,
        config: Object.keys(d.config || {}).length ? d.config : undefined,
        x: Math.round(n.position.x),
        y: Math.round(n.position.y),
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

type WfNodeData = { wfType: string; label: string; config: Record<string, unknown>; status?: string };
type WfRFNode = RFNode<WfNodeData, "wf">;

const statusRing: Record<string, string> = {
  done: "bg-good/15 !border-good",
  failed: "bg-bad/15 !border-bad",
};

function WfNodeView({ data, selected }: NodeProps<WfRFNode>) {
  const meta = NODE_META[data.wfType] || { label: data.wfType, accent: "border-border" };
  const ports = portsForNode(data.wfType, data.config);
  return (
    <div
      className={cn(
        "w-[200px] rounded-lg border-2 bg-card px-3 pt-2 pb-3 transition-colors",
        meta.accent,
        data.status && statusRing[data.status],
        selected && "ring-2 ring-accent/60",
      )}
    >
      {data.wfType !== "trigger" && <Handle type="target" position={Position.Top} className="!bg-muted" />}
      <div className="text-[10px] font-semibold tracking-wide text-muted uppercase">{meta.label}</div>
      <div className="truncate text-xs font-semibold">{clip(data.label || data.wfType, 26)}</div>
      <div className="truncate text-[10px] text-muted">{clip(summarize(data.wfType, data.config), 30)}</div>
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
  );
}

const nodeTypes = { wf: WfNodeView };

// ---- per-type config forms ----------------------------------------------------

type FieldKind = "text" | "textarea" | "number" | "select" | "json";
interface FieldSpec {
  key: string;
  label: string;
  kind: FieldKind;
  options?: string[];
  placeholder?: string;
}

const FIELD_SPECS: Record<string, FieldSpec[]> = {
  trigger: [
    { key: "kind", label: "Kind", kind: "select", options: ["manual", "cron", "event"] },
    { key: "interval_sec", label: "Interval seconds (cron)", kind: "number", placeholder: "e.g. 300" },
    { key: "daily_at", label: "Daily at HH:MM (cron)", kind: "text", placeholder: "09:00" },
    { key: "subject", label: "Subject glob (event)", kind: "text", placeholder: "task.failed | board.dm.* | memory.>" },
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

// NodePanel edits the selected node's label + type-specific config. JSON
// fields are edited as text and parsed on apply.
function NodePanel({
  node,
  onApply,
  onDelete,
  onError,
}: {
  node: WfRFNode;
  onApply: (label: string, config: Record<string, unknown>) => void;
  onDelete: () => void;
  onError: (msg: string) => void;
}) {
  const specs = FIELD_SPECS[node.data.wfType] || [];
  const [label, setLabel] = useState(node.data.label);
  const [vals, setVals] = useState<Record<string, string>>(() => {
    const out: Record<string, string> = {};
    for (const f of specs) {
      const v = node.data.config[f.key];
      out[f.key] = f.kind === "json" ? (v ? JSON.stringify(v) : "") : v == null ? "" : String(v);
    }
    return out;
  });
  // Re-seed when another node is selected.
  useEffect(() => {
    setLabel(node.data.label);
    const out: Record<string, string> = {};
    for (const f of specs) {
      const v = node.data.config[f.key];
      out[f.key] = f.kind === "json" ? (v ? JSON.stringify(v) : "") : v == null ? "" : String(v);
    }
    setVals(out);
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
      } else {
        config[f.key] = raw;
      }
    }
    onApply(label.trim(), config);
  }

  const meta = NODE_META[node.data.wfType];
  return (
    <div className="flex w-72 shrink-0 flex-col gap-2 overflow-y-auto border-l border-border bg-card p-3">
      <div className="flex items-center justify-between">
        <span className="text-xs font-semibold">{meta?.label || node.data.wfType}</span>
        <span className="font-mono text-[10px] text-muted">{node.id}</span>
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
          ) : f.kind === "select" ? (
            <select
              value={vals[f.key] || (f.options?.[0] ?? "")}
              onChange={(e) => setVals((s) => ({ ...s, [f.key]: e.target.value }))}
              aria-label={f.label}
              className={inputCls}
            >
              {(f.options || []).map((o) => (
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
      <div className="mt-1 flex items-center justify-between">
        {node.data.wfType !== "trigger" ? (
          <Button size="sm" variant="ghost" aria-label="Delete node" onClick={onDelete}>
            <Trash2 className="h-3.5 w-3.5" /> Delete
          </Button>
        ) : (
          <span className="text-[10px] text-muted">the trigger is permanent</span>
        )}
        <Button size="sm" onClick={apply} aria-label="Apply node config">
          Apply
        </Button>
      </div>
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

// ---- run history (M806) --------------------------------------------------------

export interface WfRunNodeEvent {
  node: string;
  ok?: boolean;
  handled?: boolean;
  port?: string;
  label?: string;
  error?: string;
}
export interface WfRun {
  correlation_id: string;
  status: string; // running | completed | failed
  started_ms?: number;
  finished_ms?: number;
  node_events?: WfRunNodeEvent[];
  error?: string;
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
    <div className="rounded-lg border border-border bg-card p-2">
      <div className="mb-1 flex items-center gap-2">
        <span className="text-[10px] font-semibold tracking-wide text-muted uppercase">Run history</span>
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
                {r.error && <span className="truncate text-bad">{clip(r.error, 40)}</span>}
                <span className="ml-auto font-mono text-[10px] text-muted">{clip(r.correlation_id, 18)}</span>
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

  // Editor state.
  const [editing, setEditing] = useState<Wf | null>(null);
  const [nodes, setNodes, onNodesChange] = useNodesState<RFNode>([]);
  const [edges, setEdges, onEdgesChange] = useEdgesState<RFEdge>([]);
  const [selected, setSelected] = useState<string | null>(null);
  const [running, setRunning] = useState(false);
  const [copilotOpen, setCopilotOpen] = useState(false);
  const [runsOpen, setRunsOpen] = useState(false);
  const [nodeStatus, setNodeStatus] = useState<Record<string, string>>({});
  const editingName = useRef<string | null>(null);
  editingName.current = editing?.name ?? null;

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
        }
        if (ev.kind === "workflow.started" && ev.subject === "workflow." + editingName.current) {
          setNodeStatus({});
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
    try {
      const res = await postJSON<{ executed?: string[] }>("/api/workflows/run", {
        ref: editing.name,
        payload: {},
      });
      ui.toast(`run completed — ${(res.executed || []).length} node(s)`, "success");
    } catch (e) {
      ui.toast((e as Error).message, "error");
    } finally {
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
              const dur =
                run.finished_ms && run.started_ms ? ` in ${((run.finished_ms - run.started_ms) / 1000).toFixed(1)}s` : "";
              ui.toast(`replaying ${run.status} run${dur} — ${(run.node_events || []).length} node event(s)`, "info");
            }}
            onError={(msg) => ui.toast(msg, "error")}
          />
        )}
        <div className="flex min-h-0 flex-1 rounded-lg border border-border">
          <div className="flex w-36 shrink-0 flex-col gap-1 overflow-y-auto border-r border-border bg-card p-2">
            <div className="mb-1 text-[10px] font-semibold tracking-wide text-muted uppercase">Nodes</div>
            {Object.entries(NODE_META)
              .filter(([t]) => t !== "trigger")
              .map(([t, m]) => (
                <button
                  key={t}
                  onClick={() => addNode(t)}
                  aria-label={`Add ${m.label} node`}
                  className={cn(
                    "rounded-md border-2 bg-panel px-2 py-1 text-left text-[11px] transition-colors hover:bg-accent/10",
                    m.accent,
                  )}
                >
                  {m.label}
                </button>
              ))}
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
              onApply={(label, config) => {
                setNodes((ns) =>
                  ns.map((n) =>
                    n.id === selectedNode.id ? { ...n, data: { ...(n.data as WfNodeData), label, config } } : n,
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
        <p className="text-[11px] text-muted">
          Connect nodes by dragging from a bottom handle (condition/switch/error ports are labelled). Data flows with{" "}
          <span className="font-mono">{"{{node_id.output}}"}</span> templates; the run replays live on the canvas.
        </p>
      </div>
    );
  }

  // ---- list mode ----
  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div className="flex items-center gap-2">
          <Network className="h-4 w-4 text-accent" />
          <h2 className="text-sm font-semibold">Workflows</h2>
          {list && <span className="text-xs text-muted">{list.length} workflow(s)</span>}
        </div>
        <div className="flex items-center gap-1.5">
          <Button size="sm" variant="ghost" onClick={reload} disabled={loading} aria-label="Refresh">
            <RefreshCw className={cn("h-3.5 w-3.5", loading && "animate-spin")} />
          </Button>
          <Button size="sm" onClick={() => setCreating((v) => !v)}>
            {creating ? <X className="h-3.5 w-3.5" /> : <Plus className="h-3.5 w-3.5" />}
            {creating ? "Close" : "New workflow"}
          </Button>
        </div>
      </div>

      <p className="text-xs text-muted">
        Typed-node graphs — triggers (manual/cron/event), tools, LLM steps, branches, loops over data, human approval
        gates — executed under the same policy and budget governance as everything else, every run journaled.
      </p>

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
          <Button size="sm" onClick={startNew} aria-label="Create workflow">
            <Plus className="h-3.5 w-3.5" /> Open canvas
          </Button>
        </div>
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
        {(list || []).map((w) => (
          <li key={w.id || w.name} className="rounded-lg border border-border bg-card p-3">
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
            {w.description && <div className="mt-1 text-xs text-muted">{w.description}</div>}
          </li>
        ))}
      </ul>
    </div>
  );
}
