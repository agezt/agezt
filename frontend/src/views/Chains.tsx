import { useCallback, useEffect, useMemo, useState } from "react";
import { Waypoints, RefreshCw, Save, ArrowUp, ArrowDown, X, Plus, Zap, Star, Pencil, Trash2, Users, Route, AlertTriangle } from "lucide-react";
import { getJSON, postJSON } from "@/lib/api";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { ErrorText } from "@/components/JsonView";
import { SkeletonList } from "@/components/ui/skeleton";
import { useUI } from "@/components/ui/feedback";
import { PageHeader } from "@/components/ui/page-header";
import { Badge } from "@/components/ui/badge";
import { ModelPicker } from "@/components/ModelPicker";
import { validateChainName, moveItem, removeAt, renameChain, deleteChain } from "@/lib/chains";
import { modelHealth, type ModelCatalog, type ModelHealth } from "@/lib/models";

// Chains is the registry of named, reusable fallback ladders (M963). A chain is
// an ordered model list; anywhere a model is picked (agent, routing, chat) you
// can select "@<chain>" instead of a single model, and the governor expands it
// to the chain's models at run time. Edit a chain here once → every reference
// picks up the change. One chain can be marked the DEFAULT, used by any run that
// resolves to no chain of its own — so even a bare agent gets a fallback ladder.

// ChainUsage is the per-chain reference map from the backend: which agents and
// task types reference @name, and whether it is the default.
export interface ChainUsage {
  agents?: string[];
  tasks?: string[];
  default?: boolean;
}

interface ChainsResp {
  chains?: Record<string, string[]>;
  default?: string;
  // usage maps chain name → ChainUsage, plus a reserved "__dangling__" key whose
  // value is the list of @names referenced somewhere but no longer defined.
  usage?: Record<string, ChainUsage | string[]>;
}

export function Chains() {
  const { toast, confirm } = useUI();
  const [chains, setChains] = useState<Record<string, string[]>>({});
  const [savedChains, setSavedChains] = useState<Record<string, string[]>>({});
  const [def, setDef] = useState("");
  const [savedDef, setSavedDef] = useState("");
  const [usage, setUsage] = useState<Record<string, ChainUsage | string[]>>({});
  const [cat, setCat] = useState<ModelCatalog | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);

  const reload = useCallback(async () => {
    setLoading(true);
    try {
      const r = await getJSON<ChainsResp>("/api/chains");
      setChains(r.chains || {});
      setSavedChains(r.chains || {});
      setDef(r.default || "");
      setSavedDef(r.default || "");
      setUsage(r.usage || {});
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

  // The catalog drives per-model health dots (does a keyed provider serve it?).
  useEffect(() => {
    getJSON<ModelCatalog>("/api/catalog")
      .then(setCat)
      .catch(() => {
        /* health is best-effort — a missing catalog just hides the dots */
      });
  }, []);

  const health = useCallback((id: string): ModelHealth => modelHealth(cat, id), [cat]);

  const names = useMemo(() => Object.keys(chains).sort((a, b) => a.localeCompare(b)), [chains]);
  const dangling = useMemo(() => (Array.isArray(usage.__dangling__) ? (usage.__dangling__ as string[]) : []), [usage]);
  const usageFor = useCallback(
    (name: string): ChainUsage => {
      const u = usage[name];
      return u && !Array.isArray(u) ? u : {};
    },
    [usage],
  );
  const dirty = useMemo(
    () => JSON.stringify(chains) !== JSON.stringify(savedChains) || def !== savedDef,
    [chains, savedChains, def, savedDef],
  );

  function setChainModels(name: string, models: string[]) {
    setChains((c) => ({ ...c, [name]: models }));
  }
  const addModel = (name: string, id: string) => {
    if (!id) return;
    const cur = chains[name] || [];
    if (cur.includes(id)) {
      toast(`${id} is already in “${name}”`, "info");
      return;
    }
    setChainModels(name, [...cur, id]);
  };

  function addChain() {
    // Find a free default name.
    let n = "new-chain";
    for (let i = 2; n in chains; i++) n = `new-chain-${i}`;
    setChains((c) => ({ ...c, [n]: [] }));
  }

  function rename(from: string) {
    const to = window.prompt(`Rename chain “${from}” to:`, from)?.trim();
    if (!to || to === from) return;
    const bad = validateChainName(to);
    if (bad) {
      toast(bad, "error");
      return;
    }
    if (to in chains) {
      toast(`A chain named “${to}” already exists`, "error");
      return;
    }
    setChains((c) => renameChain(c, from, to));
    if (def === from) setDef(to);
  }

  async function remove(name: string) {
    const u = usageFor(name);
    const refs: string[] = [];
    if (u.agents?.length) refs.push(`${u.agents.length} agent${u.agents.length === 1 ? "" : "s"} (${u.agents.join(", ")})`);
    if (u.tasks?.length) refs.push(`${u.tasks.length} task${u.tasks.length === 1 ? "" : "s"} (${u.tasks.join(", ")})`);
    if (u.default) refs.push("the default chain");
    const message = refs.length
      ? `Still referenced by ${refs.join(" and ")} — those will fall through to the default chain (or the daemon model).`
      : `References to @${name} will fall through to the default chain (or the daemon model).`;
    if (
      !(await confirm({
        title: `Delete chain “${name}”?`,
        message,
        confirmLabel: "Delete",
        danger: true,
      }))
    )
      return;
    setChains((c) => deleteChain(c, name));
    if (def === name) setDef("");
  }

  async function save() {
    // Validate every name and that no chain is empty before sending.
    for (const name of Object.keys(chains)) {
      const bad = validateChainName(name);
      if (bad) {
        toast(`“${name}”: ${bad}`, "error");
        return;
      }
    }
    const empties = Object.keys(chains).filter((n) => (chains[n] || []).length === 0);
    if (empties.length) {
      toast(`Add at least one model to: ${empties.join(", ")}`, "error");
      return;
    }
    setSaving(true);
    try {
      const r = await postJSON<{ unknown_models?: string[]; chain_count?: number }>("/api/chains/set", {
        chains,
        default: def,
      });
      setSavedChains(chains);
      setSavedDef(def);
      if (r.unknown_models?.length) {
        toast(`Saved — but ${r.unknown_models.length} model(s) aren't in the catalog: ${r.unknown_models.join(", ")}`, "info");
      } else {
        toast(`Saved ${r.chain_count ?? 0} fallback chain${r.chain_count === 1 ? "" : "s"}`, "success");
      }
    } catch (e) {
      toast((e as Error).message, "error");
    } finally {
      setSaving(false);
    }
  }

  return (
    <div className="space-y-4">
      <PageHeader
        icon={Waypoints}
        title="Fallback Chains"
        actions={
          <>
            <Button variant="ghost" size="sm" onClick={addChain} title="Create a new fallback chain">
              <Plus className="size-3.5" /> New chain
            </Button>
            <Button variant="accent" size="sm" onClick={save} disabled={!dirty || saving} title="Save chains">
              {saving ? <RefreshCw className="size-3.5 animate-spin" /> : <Save className="size-3.5" />} Save
            </Button>
            <Button variant="ghost" size="sm" onClick={reload} disabled={loading} title="Reload">
              <RefreshCw className={cn("size-3.5", loading && "animate-spin")} />
            </Button>
          </>
        }
      />

      {dangling.length > 0 && (
        <div className="flex items-start gap-2 rounded-lg border border-warn/40 bg-warn/5 px-3 py-2 text-xs text-warn">
          <AlertTriangle className="mt-0.5 size-3.5 shrink-0" />
          <span>
            Dangling reference{dangling.length === 1 ? "" : "s"}:{" "}
            <span className="font-mono">{dangling.map((d) => `@${d}`).join(", ")}</span> {dangling.length === 1 ? "is" : "are"} used
            somewhere but no longer defined — those references fall through to the default chain. Create the chain{dangling.length === 1 ? "" : "s"} or
            update the reference.
          </span>
        </div>
      )}

      {err ? (
        <ErrorText>{err}</ErrorText>
      ) : loading && !names.length ? (
        <SkeletonList count={3} lines={3} />
      ) : names.length === 0 ? (
        <div className="rounded-lg border border-dashed border-border bg-card/40 px-3 py-8 text-center text-sm text-muted">
          No fallback chains yet.{" "}
          <button onClick={addChain} className="text-accent underline-offset-2 hover:underline">
            Create one
          </button>{" "}
          to reuse the same model ladder everywhere.
        </div>
      ) : (
        <div className="space-y-2">
          {names.map((name) => (
            <ChainCard
              key={name}
              name={name}
              models={chains[name] || []}
              isDefault={def === name}
              usage={usageFor(name)}
              health={cat ? health : undefined}
              onAdd={(id) => addModel(name, id)}
              onRemove={(i) => setChainModels(name, removeAt(chains[name] || [], i))}
              onMove={(i, d) => setChainModels(name, moveItem(chains[name] || [], i, d))}
              onRename={() => rename(name)}
              onDelete={() => remove(name)}
              onMakeDefault={() => setDef(def === name ? "" : name)}
            />
          ))}
        </div>
      )}
    </div>
  );
}

function ChainCard({
  name,
  models,
  isDefault,
  usage,
  health,
  onAdd,
  onRemove,
  onMove,
  onRename,
  onDelete,
  onMakeDefault,
}: {
  name: string;
  models: string[];
  isDefault: boolean;
  usage: ChainUsage;
  health?: (id: string) => ModelHealth; // undefined until the catalog loads
  onAdd: (id: string) => void;
  onRemove: (i: number) => void;
  onMove: (i: number, dir: -1 | 1) => void;
  onRename: () => void;
  onDelete: () => void;
  onMakeDefault: () => void;
}) {
  const agentN = usage.agents?.length ?? 0;
  const taskN = usage.tasks?.length ?? 0;
  // A chain is unhealthy if its primary can't run, or if NO model in it can run.
  const primaryBad = health && models.length > 0 && health(models[0]) !== "ok";
  const allBad = health && models.length > 0 && models.every((m) => health(m) !== "ok");
  return (
    <div className={cn("glass rounded-xl p-3", isDefault && "glow-accent")}>
      <div className="mb-2 flex items-center gap-2">
        <h3 className="font-mono text-sm font-semibold text-foreground">@{name}</h3>
        {isDefault && (
          <span className="inline-flex items-center gap-0.5 rounded bg-accent/15 px-1.5 py-0.5 text-[9px] font-medium uppercase text-accent">
            <Star className="size-2.5" /> default
          </span>
        )}
        <span className="text-[11px] text-muted">
          {models.length} model{models.length === 1 ? "" : "s"}
        </span>
        {agentN > 0 && (
          <span
            className="inline-flex items-center gap-1 rounded bg-panel px-1.5 py-0.5 text-xs text-muted"
            title={`Referenced by: ${usage.agents!.join(", ")}`}
          >
            <Users className="size-2.5" /> {agentN} agent{agentN === 1 ? "" : "s"}
          </span>
        )}
        {taskN > 0 && (
          <span
            className="inline-flex items-center gap-1 rounded bg-panel px-1.5 py-0.5 text-xs text-muted"
            title={`Task types: ${usage.tasks!.join(", ")}`}
          >
            <Route className="size-2.5" /> {taskN} task{taskN === 1 ? "" : "s"}
          </span>
        )}
        <div className="ml-auto flex items-center gap-1">
          <button
            onClick={onMakeDefault}
            className={cn("rounded p-1 transition-colors", isDefault ? "text-accent" : "text-muted hover:text-foreground")}
            title={isDefault ? "Unset as default chain" : "Use as the default chain"}
          >
            <Star className={cn("size-3.5", isDefault && "fill-accent")} />
          </button>
          <button onClick={onRename} className="rounded p-1 text-muted transition-colors hover:text-foreground" title="Rename chain">
            <Pencil className="size-3.5" />
          </button>
          <button onClick={onDelete} className="rounded p-1 text-muted transition-colors hover:text-bad" title="Delete chain">
            <Trash2 className="size-3.5" />
          </button>
        </div>
      </div>

      {models.length > 0 ? (
        <ol className="mb-2 space-y-1">
          {models.map((m, i) => (
            <li key={`${m}-${i}`} className="flex items-center gap-2 rounded-md border border-border/60 bg-panel/40 px-2 py-1 text-xs">
              {i === 0 ? (
                <Badge variant="accent">
                  <Zap className="mr-1 size-3" />
                  primary
                </Badge>
              ) : (
                <Badge variant="default">
                  <ArrowDown className="mr-1 size-3" />
                  {i}
                </Badge>
              )}
              {health && <HealthDot status={health(m)} />}
              <span className="min-w-0 flex-1 truncate font-mono text-foreground/90">{m}</span>
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
      ) : (
        <div className="mb-2 rounded-md border border-dashed border-warn/40 bg-warn/5 px-2 py-1 text-[11px] text-warn">
          Empty chain — add at least one model before saving.
        </div>
      )}

      {allBad ? (
        <div className="mb-2 flex items-center gap-1.5 rounded-md border border-bad/40 bg-bad/5 px-2 py-1 text-[11px] text-bad">
          <AlertTriangle className="size-3 shrink-0" /> No model in this chain has a keyed provider — runs using it will fall through to the daemon default.
        </div>
      ) : (
        primaryBad && (
          <div className="mb-2 flex items-center gap-1.5 rounded-md border border-warn/40 bg-warn/5 px-2 py-1 text-[11px] text-warn">
            <AlertTriangle className="size-3 shrink-0" /> The primary model can't run (no keyed provider) — a fallback will be used first.
          </div>
        )
      )}

      <div className="flex items-center gap-1.5 text-[11px] text-muted">
        <Plus className="size-3" />
        <ModelPicker
          value=""
          activeModel={models.length ? "add a fallback model" : "add the primary model"}
          onChange={onAdd}
          allowChains={false}
        />
      </div>
    </div>
  );
}

// HealthDot renders a model's run-readiness as a colored dot: green = a keyed
// provider serves it, amber = exists but needs an API key, red = unknown to the
// catalog (typo / removed). Title carries the explanation.
function HealthDot({ status }: { status: ModelHealth }) {
  const meta: Record<ModelHealth, { cls: string; title: string }> = {
    ok: { cls: "bg-good", title: "A keyed provider can run this model" },
    nokey: { cls: "bg-warn", title: "No keyed provider — add an API key under Models to run this" },
    unknown: { cls: "bg-bad", title: "Not in the catalog — check the model id (typo or removed)" },
  };
  const m = meta[status];
  return <span className={cn("size-2 shrink-0 rounded-full", m.cls)} title={m.title} aria-label={`model health: ${status}`} />;
}
