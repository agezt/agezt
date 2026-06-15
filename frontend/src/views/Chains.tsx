import { useCallback, useEffect, useMemo, useState } from "react";
import { Waypoints, RefreshCw, Save, ArrowUp, ArrowDown, X, Plus, Zap, Star, Pencil, Trash2 } from "lucide-react";
import { getJSON, postJSON } from "@/lib/api";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { ErrorText } from "@/components/JsonView";
import { SkeletonList } from "@/components/ui/skeleton";
import { useUI } from "@/components/ui/feedback";
import { ModelPicker } from "@/components/ModelPicker";
import { validateChainName, moveItem, removeAt, renameChain, deleteChain } from "@/lib/chains";

// Chains is the registry of named, reusable fallback ladders (M963). A chain is
// an ordered model list; anywhere a model is picked (agent, routing, chat) you
// can select "@<chain>" instead of a single model, and the governor expands it
// to the chain's models at run time. Edit a chain here once → every reference
// picks up the change. One chain can be marked the DEFAULT, used by any run that
// resolves to no chain of its own — so even a bare agent gets a fallback ladder.

interface ChainsResp {
  chains?: Record<string, string[]>;
  default?: string;
}

export function Chains() {
  const { toast, confirm } = useUI();
  const [chains, setChains] = useState<Record<string, string[]>>({});
  const [savedChains, setSavedChains] = useState<Record<string, string[]>>({});
  const [def, setDef] = useState("");
  const [savedDef, setSavedDef] = useState("");
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

  const names = useMemo(() => Object.keys(chains).sort((a, b) => a.localeCompare(b)), [chains]);
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
    if (
      !(await confirm({
        title: `Delete chain “${name}”?`,
        message: `References to @${name} will fall through to the default chain (or the daemon model).`,
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
      <div className="flex flex-wrap items-center gap-2">
        <h2 className="flex items-center gap-2 text-sm font-semibold">
          <Waypoints className="size-4 text-accent" /> Fallback Chains
        </h2>
        <span className="text-xs text-muted">named, reusable model ladders</span>
        <div className="ml-auto flex items-center gap-2">
          <Button variant="ghost" size="sm" onClick={addChain} title="Create a new fallback chain">
            <Plus className="size-3.5" /> New chain
          </Button>
          <Button size="sm" onClick={save} disabled={!dirty || saving} title="Save chains">
            {saving ? <RefreshCw className="size-3.5 animate-spin" /> : <Save className="size-3.5" />} Save
          </Button>
          <Button variant="ghost" size="sm" onClick={reload} disabled={loading} title="Reload">
            <RefreshCw className={cn("size-3.5", loading && "animate-spin")} />
          </Button>
        </div>
      </div>

      <p className="text-xs text-muted">
        A chain is an ordered list of models tried in turn (the{" "}
        <span className="text-foreground/80">primary</span> first, then each <span className="text-foreground/80">fallback</span>).
        Pick a chain anywhere you pick a model — agents, routing, chat — and the governor expands it at run time. Mark one as the{" "}
        <span className="text-foreground/80">default</span> so even a bare run gets a fallback ladder. Changes apply live and persist.
      </p>

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
  onAdd: (id: string) => void;
  onRemove: (i: number) => void;
  onMove: (i: number, dir: -1 | 1) => void;
  onRename: () => void;
  onDelete: () => void;
  onMakeDefault: () => void;
}) {
  return (
    <div className={cn("rounded-lg border bg-card p-3", isDefault ? "border-accent/50" : "border-border")}>
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
                <span className="inline-flex items-center gap-0.5 rounded bg-accent/15 px-1.5 py-0.5 text-[9px] font-medium uppercase text-accent">
                  <Zap className="size-2.5" /> primary
                </span>
              ) : (
                <span className="rounded bg-panel px-1.5 py-0.5 text-[9px] font-medium uppercase text-muted">fallback {i}</span>
              )}
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
