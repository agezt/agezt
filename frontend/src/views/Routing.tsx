import { useCallback, useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { Route, RefreshCw, Save, ArrowUp, ArrowDown, X, Plus, Zap, CornerDownRight, Download, Upload, Wand2, type LucideIcon } from "lucide-react";
import { getJSON, postJSON } from "@/lib/api";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { ErrorText } from "@/components/JsonView";
import { SkeletonList } from "@/components/ui/skeleton";
import { useUI } from "@/components/ui/feedback";
import { Page } from "@/components/ui/page";
import { Disclosure } from "@/components/ui/disclosure";
import { ModelPicker } from "@/components/ModelPicker";
import { ModelChip } from "@/components/ModelChip";
import { downloadText } from "@/lib/export";
import { type ModelCatalog } from "@/lib/models";
import { suggestChains } from "@/lib/routingSuggest";

// parseChainsJSON normalises an imported routing file into a {task: [models]} map,
// tolerating either a bare map or a {chains:{…}} wrapper. Keeps only string model
// ids and drops empty chains; throws on bad JSON or a shape that yields nothing.
export function parseChainsJSON(text: string): Record<string, string[]> {
  const data = JSON.parse(text);
  const obj = data && typeof data.chains === "object" && data.chains ? data.chains : data;
  if (!obj || typeof obj !== "object" || Array.isArray(obj)) {
    throw new Error("expected an object {task: [models]} (or a {chains:{…}} wrapper)");
  }
  const out: Record<string, string[]> = {};
  for (const [task, v] of Object.entries(obj)) {
    if (!Array.isArray(v)) continue;
    const models = v.filter((m): m is string => typeof m === "string" && m.trim() !== "").map((m) => m.trim());
    if (task.trim() && models.length) out[task.trim()] = models;
  }
  if (Object.keys(out).length === 0) throw new Error("no valid task→models chains found in the file");
  return out;
}

// Routing lets you give each agentic job (task type) its own ORDERED model chain:
// a primary model plus fallback models tried in turn (each routes to its serving
// provider). It drives the governor's per-task model fallback (M703) — edits apply
// live and persist. The main chat loop is "chat"; delegated sub-agents are
// "delegate"; the rest are the daemon's internal jobs (plan, verify, …).

const TASK_HELP: Record<string, string> = {
  chat: "The main chat / agent loop.",
  plan: "Planning a multi-step task.",
  code: "Code-generation steps.",
  verify: "Checking whether a task is complete.",
  summarize: "Compressing context / eliding tool output.",
  salience: "Scoring memory salience.",
  distill: "Distilling tool outputs into facts.",
  forge: "Authoring new skills.",
  "shadow-eval": "Judging shadow skills.",
  delegate: "Delegated sub-agents (the delegate tool).",
};

interface TaskActivity {
  fallbacks?: number;
  last_failed?: string;
  last_next?: string;
  last_reason?: string;
  last_ms?: number;
}

interface RoutingResp {
  task_types?: string[];
  chains?: Record<string, string[]>;
  activity?: Record<string, TaskActivity>;
}

function RoutingModal({
  title,
  icon: Icon,
  onClose,
  children,
}: {
  title: string;
  icon: LucideIcon;
  onClose: () => void;
  children: ReactNode;
}) {
  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/55 p-4 backdrop-blur-sm">
      <div className="w-full max-w-lg rounded-xl border border-border bg-panel shadow-2xl">
        <div className="flex items-center gap-2 border-b border-border px-4 py-3">
          <Icon className="size-4 text-accent" />
          <h2 className="text-sm font-semibold text-foreground">{title}</h2>
          <Button className="ml-auto" size="icon" variant="ghost" onClick={onClose} aria-label={`Close ${title}`}>
            <X className="size-4" />
          </Button>
        </div>
        <div className="p-4">{children}</div>
      </div>
    </div>
  );
}

export function Routing() {
  const { toast } = useUI();
  const [taskTypes, setTaskTypes] = useState<string[]>([]);
  const [chains, setChains] = useState<Record<string, string[]>>({});
  const [saved, setSaved] = useState<Record<string, string[]>>({});
  const [activity, setActivity] = useState<Record<string, TaskActivity>>({});
  const [err, setErr] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  // Named fallback chains (M963) + catalog, so a "@chain" slot in a task chain
  // renders as ⛓ chain and plain models get a health dot (M969). Best-effort.
  const [namedChains, setNamedChains] = useState<Record<string, string[]>>({});
  const [cat, setCat] = useState<ModelCatalog | null>(null);
  const fileRef = useRef<HTMLInputElement>(null);

  const reload = useCallback(async () => {
    setLoading(true);
    try {
      const r = await getJSON<RoutingResp>("/api/routing");
      setTaskTypes(r.task_types || []);
      setChains(r.chains || {});
      setSaved(r.chains || {});
      setActivity(r.activity || {});
      setErr(null);
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    reload();
  }, [reload]);

  useEffect(() => {
    getJSON<{ chains?: Record<string, string[]> }>("/api/chains").then((c) => setNamedChains(c.chains || {})).catch(() => {});
    getJSON<ModelCatalog>("/api/catalog").then(setCat).catch(() => {});
  }, []);

  // The union of known task types and any custom ones already configured.
  const rows = useMemo(() => {
    const set = new Set<string>([...taskTypes, ...Object.keys(chains)]);
    return [...set].sort((a, b) => {
      const ia = taskTypes.indexOf(a);
      const ib = taskTypes.indexOf(b);
      if (ia >= 0 && ib >= 0) return ia - ib;
      if (ia >= 0) return -1;
      if (ib >= 0) return 1;
      return a.localeCompare(b);
    });
  }, [taskTypes, chains]);

  const dirty = useMemo(() => JSON.stringify(chains) !== JSON.stringify(saved), [chains, saved]);

  function setChain(task: string, models: string[]) {
    setChains((c) => {
      const next = { ...c };
      if (models.length === 0) delete next[task];
      else next[task] = models;
      return next;
    });
  }
  const addModel = (task: string, id: string) => {
    if (!id) return;
    const cur = chains[task] || [];
    if (cur.includes(id)) {
      toast(`${id} is already in the ${task} chain`, "info");
      return;
    }
    setChain(task, [...cur, id]);
  };
  const removeAt = (task: string, i: number) => setChain(task, (chains[task] || []).filter((_, j) => j !== i));
  const move = (task: string, i: number, dir: -1 | 1) => {
    const cur = [...(chains[task] || [])];
    const j = i + dir;
    if (j < 0 || j >= cur.length) return;
    [cur[i], cur[j]] = [cur[j], cur[i]];
    setChain(task, cur);
  };

  async function save() {
    setSaving(true);
    try {
      const r = await postJSON<{ unknown_models?: string[]; task_count?: number }>("/api/routing/set", { chains });
      setSaved(chains);
      if (r.unknown_models?.length) {
        toast(`Saved — but ${r.unknown_models.length} model(s) aren't in the catalog: ${r.unknown_models.join(", ")}`, "info");
      } else {
        toast(`Routing saved (${r.task_count ?? 0} task chain${r.task_count === 1 ? "" : "s"})`, "success");
      }
    } catch (e) {
      toast((e as Error).message, "error");
    } finally {
      setSaving(false);
    }
  }

  function exportChains() {
    downloadText("agezt-routing.json", JSON.stringify({ chains }, null, 2), "application/json");
  }

  const [filling, setFilling] = useState(false);
  // Auto-fill (M928): build a suggested chain for every task from the keyed
  // providers in the catalog — one best model per provider, task-fit ordered.
  // Replaces the current (unsaved) table for review; nothing persists until Save.
  async function autoFill() {
    setFilling(true);
    try {
      const cat = await getJSON<ModelCatalog>("/api/catalog");
      const suggested = suggestChains(cat, rows);
      const n = Object.keys(suggested).length;
      if (n === 0) {
        toast("No keyed provider with usable models found — add an API key under Models first", "info");
        return;
      }
      setChains(suggested);
      toast(`Auto-filled ${n} chain(s) from your keyed providers — review and Save`, "success");
    } catch (e) {
      toast((e as Error).message, "error");
    } finally {
      setFilling(false);
    }
  }

  async function onImportFile(file: File) {
    try {
      const imported = parseChainsJSON(await file.text());
      // Merge: imported task chains override existing ones, for review then Save.
      setChains((cur) => ({ ...cur, ...imported }));
      toast(`Imported ${Object.keys(imported).length} chain(s) — review and Save`, "success");
    } catch (e) {
      toast(`Import failed: ${(e as Error).message}`, "error");
    }
  }

  return (
    <Page
      icon={Route}
      title="Routing"
      description="Per-task model chains — primary first, fallbacks in order"
      width="readable"
      mode="scroll"
      actions={
          <>
            <Button
              variant="ghost"
              size="sm"
              onClick={autoFill}
              disabled={filling}
              title="Fill every task with a suggested chain built from your keyed providers (review, then Save)"
            >
              {filling ? <RefreshCw className="size-3.5 animate-spin" /> : <Wand2 className="size-3.5" />} Auto-fill
            </Button>
            <Button variant="ghost" size="sm" onClick={() => fileRef.current?.click()} title="Import routing from a JSON file">
              <Upload className="size-3.5" /> Import
            </Button>
            <Button variant="ghost" size="sm" onClick={exportChains} disabled={Object.keys(chains).length === 0} title="Export routing to a JSON file">
              <Download className="size-3.5" /> Export
            </Button>
            <Button size="sm" onClick={save} disabled={!dirty || saving} title="Save routing">
              {saving ? <RefreshCw className="size-3.5 animate-spin" /> : <Save className="size-3.5" />} Save
            </Button>
            <Button variant="ghost" size="sm" onClick={reload} disabled={loading} title="Reload">
              <RefreshCw className={cn("size-3.5", loading && "animate-spin")} />
            </Button>
          </>
        }
    >
      <input
        ref={fileRef}
        type="file"
        accept="application/json,.json"
        className="hidden"
        aria-hidden="true"
        onChange={(e) => {
          const f = e.target.files?.[0];
          if (f) void onImportFile(f);
          e.target.value = "";
        }}
      />
      <Disclosure summary={<span className="text-xs text-muted">How task routing works</span>}>
        <p className="px-2 text-xs text-muted">
          Give each agentic job its own ordered model chain: the <span className="text-foreground/80">primary</span> model
          is tried first, then each <span className="text-foreground/80">fallback</span> in turn (each routes to its keyed
          provider). Leave a task empty to use the daemon default. Changes apply live and persist.
        </p>
      </Disclosure>

      {err ? (
        <ErrorText>{err}</ErrorText>
      ) : loading && !rows.length ? (
        <SkeletonList count={6} lines={2} />
      ) : (
        <div className="space-y-2">
          {rows.map((task) => (
            <ChainRow
              key={task}
              task={task}
              models={chains[task] || []}
              activity={activity[task]}
              namedChains={namedChains}
              cat={cat}
              onAdd={(id) => addModel(task, id)}
              onRemove={(i) => removeAt(task, i)}
              onMove={(i, d) => move(task, i, d)}
            />
          ))}
        </div>
      )}
    </Page>
  );
}

function ChainRow({
  task,
  models,
  activity,
  namedChains,
  cat,
  onAdd,
  onRemove,
  onMove,
}: {
  task: string;
  models: string[];
  activity?: TaskActivity;
  namedChains: Record<string, string[]>;
  cat: ModelCatalog | null;
  onAdd: (id: string) => void;
  onRemove: (i: number) => void;
  onMove: (i: number, dir: -1 | 1) => void;
}) {
  const fb = activity?.fallbacks ?? 0;
  const [addOpen, setAddOpen] = useState(false);
  return (
    <div className="glass rounded-xl p-3">
      <div className="mb-2 flex items-baseline gap-2">
        <h3 className="font-mono text-sm font-semibold text-foreground">{task}</h3>
        <span className="text-[11px] text-muted">{TASK_HELP[task] || "Custom task type."}</span>
        {models.length === 0 && <span className="ml-auto text-xs uppercase tracking-normal text-muted">daemon default</span>}
      </div>

      {models.length > 0 && (
        <ol className="mb-2 space-y-1">
          {models.map((m, i) => (
            <li key={`${m}-${i}`} className="flex items-center gap-2 rounded-md border border-border/60 bg-panel/40 px-2 py-1 text-xs">
              {i === 0 ? (
                <span className="inline-flex items-center gap-0.5 rounded bg-accent/15 px-1.5 py-0.5 text-[9px] font-medium uppercase text-accent">
                  <Zap className="size-2.5" /> primary
                </span>
              ) : (
                <span className="rounded bg-panel px-1.5 py-0.5 text-[9px] font-medium uppercase text-muted">fallback {i}</span>
              )}
              <span className="min-w-0 flex-1 truncate">
                <ModelChip id={m} chains={namedChains} cat={cat} />
              </span>
              <button onClick={() => onMove(i, -1)} disabled={i === 0} className="text-muted transition-colors hover:text-foreground disabled:opacity-30" title="Move up">
                <ArrowUp className="size-3.5" />
              </button>
              <button onClick={() => onMove(i, 1)} disabled={i === models.length - 1} className="text-muted transition-colors hover:text-foreground disabled:opacity-30" title="Move down">
                <ArrowDown className="size-3.5" />
              </button>
              <button onClick={() => onRemove(i)} className="text-muted transition-colors hover:text-bad" title="Remove">
                <X className="size-3.5" />
              </button>
            </li>
          ))}
        </ol>
      )}

      {fb > 0 && (
        <div className="mb-2 flex flex-wrap items-center gap-1.5 rounded-md border border-warn/30 bg-warn/5 px-2 py-1 text-[11px] text-warn">
          <CornerDownRight className="size-3 shrink-0" />
          <span className="font-medium">
            {fb} fallback{fb === 1 ? "" : "s"}
          </span>
          {activity?.last_failed && activity?.last_next && (
            <span className="font-mono text-foreground/70">
              last: {activity.last_failed} → {activity.last_next}
            </span>
          )}
          {activity?.last_reason && <span className="truncate text-muted">({activity.last_reason})</span>}
        </div>
      )}

      <div className="flex items-center justify-between gap-2 text-[11px] text-muted">
        <span>
          {models.length > 0 ? `${models.length} model chain` : "Uses daemon default"}
        </span>
        <Button size="sm" variant="ghost" onClick={() => setAddOpen(true)} aria-label={`Add model to ${task}`}>
          <Plus className="size-3.5" />
          {models.length ? "Add fallback" : "Set primary"}
        </Button>
      </div>
      {addOpen && (
        <RoutingModal title={`${models.length ? "Add fallback" : "Set primary"} · ${task}`} icon={Route} onClose={() => setAddOpen(false)}>
          <div className="space-y-3">
            <div className="rounded-lg border border-border bg-card p-3 text-xs text-muted">
              {models.length
                ? "Pick one more model for this task. It is appended after the current fallbacks; Save persists it."
                : "Pick the primary model for this task. Empty tasks continue to use daemon default until you choose one."}
            </div>
            <ModelPicker
              value=""
              activeModel={models.length ? "choose fallback model" : "choose primary model"}
              allowChains={false}
              triggerClassName="h-9 w-full max-w-none justify-between"
              onChange={(id) => {
                onAdd(id);
                if (id) setAddOpen(false);
              }}
            />
          </div>
        </RoutingModal>
      )}
    </div>
  );
}
