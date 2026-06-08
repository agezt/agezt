import { useEffect, useMemo, useState } from "react";
import { Network, RefreshCw } from "lucide-react";
import { getJSON } from "@/lib/api";
import { useEvents } from "@/lib/events";
import { money } from "@/lib/format";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { Muted, ErrorText } from "@/components/JsonView";
import { DelegationGraph } from "@/components/DelegationGraph";
import { buildDelegationTree, pickDefaultRoot, type RunNode } from "@/lib/delegation";

interface ApiRun {
  correlation_id?: string;
  parent_correlation?: string;
  status?: string;
  model?: string;
  spent_mc?: number;
  iters?: number;
  intent?: string;
  started_unix_ms?: number;
}

function toRunNodes(runs: ApiRun[]): RunNode[] {
  return runs.map((r) => ({
    id: r.correlation_id || "",
    parent: r.parent_correlation || undefined,
    status: r.status,
    model: r.model,
    spentMc: r.spent_mc,
    iters: r.iters,
    intent: r.intent,
  }));
}

// Agents is the multi-agent monitor: pick a run and watch its sub-agent
// delegation tree as a live node graph — who delegated to whom, each agent's
// status, model, iterations and spend, with the whole-tree totals up top.
export function Agents() {
  const { events } = useEvents();
  const [runs, setRuns] = useState<ApiRun[] | null>(null);
  const [sel, setSel] = useState<string>("");
  const [err, setErr] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);

  async function reload() {
    setLoading(true);
    try {
      const d = await getJSON<{ runs?: ApiRun[] }>("/api/runs");
      const list = d.runs || [];
      setRuns(list);
      setErr(null);
      setSel((cur) => cur || pickDefaultRoot(toRunNodes(list)) || "");
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setLoading(false);
    }
  }
  useEffect(() => {
    reload();
    const id = setInterval(reload, 6000);
    return () => clearInterval(id);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // Nudge on run lifecycle + delegation events.
  const head = events[0]?.kind;
  useEffect(() => {
    if (head === "task.completed" || head === "task.failed" || head === "subagent.spawned" || head === "task.received")
      reload();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [head, events[0]?.id]);

  const nodes = useMemo(() => (runs ? toRunNodes(runs) : []), [runs]);
  const tree = useMemo(() => (sel ? buildDelegationTree(nodes, sel) : null), [nodes, sel]);

  // Runs that are roots (no parent) — the selectable lead runs.
  const roots = useMemo(() => nodes.filter((n) => !n.parent), [nodes]);

  return (
    <div className="flex h-full min-h-0 flex-col gap-3">
      <div className="flex flex-wrap items-center gap-2">
        <h2 className="flex items-center gap-2 text-sm font-semibold">
          <Network className="size-4 text-accent" /> Agents
        </h2>
        <select
          value={sel}
          onChange={(e) => setSel(e.target.value)}
          className="h-8 max-w-[55%] flex-1 rounded-md border border-border bg-panel px-2 text-xs outline-none focus:border-accent"
        >
          {roots.length === 0 && <option value="">no runs yet</option>}
          {roots.map((r) => (
            <option key={r.id} value={r.id}>
              {(r.status === "running" ? "● " : "") + (r.intent ? r.intent.slice(0, 70) : r.id)}
            </option>
          ))}
        </select>
        <Button variant="ghost" size="sm" onClick={reload} title="Reload">
          <RefreshCw className={cn("size-3.5", loading && "animate-spin")} />
        </Button>
      </div>

      {tree && tree.count > 0 && (
        <div className="flex flex-wrap gap-2 text-xs">
          <Stat label="agents" value={tree.count} />
          <Stat label="sub-agents" value={Math.max(0, tree.count - 1)} />
          <Stat label="depth" value={tree.maxDepth} />
          <Stat label="tree spend" value={money(tree.totalSpentMc)} />
        </div>
      )}

      {err ? (
        <ErrorText>{err}</ErrorText>
      ) : !runs ? (
        <Muted>loading…</Muted>
      ) : roots.length === 0 ? (
        <Muted>no runs yet — start one from Chat or the CLI</Muted>
      ) : (
        <div className="min-h-0 flex-1 overflow-hidden rounded-lg border border-border bg-card">
          {sel && <DelegationGraph runs={nodes} rootId={sel} />}
        </div>
      )}
    </div>
  );
}

function Stat({ label, value }: { label: string; value: number | string }) {
  return (
    <div className="rounded-md border border-border bg-card px-2.5 py-1">
      <span className="text-muted">{label} </span>
      <span className="font-semibold tabular-nums">{value}</span>
    </div>
  );
}
