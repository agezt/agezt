import { useEffect, useState } from "react";
import { ArrowUpRight, Cpu, ArrowRight, Waypoints } from "lucide-react";
import { getJSON, postJSON } from "@/lib/api";
import { cn, fmtTime, clip } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { SkeletonList } from "@/components/ui/skeleton";
import { useUI } from "@/components/ui/feedback";
import { ModelPicker } from "@/components/ModelPicker";
import { ModelChip } from "@/components/ModelChip";
import { type AgentProfile } from "@/views/Roster";
import { summarizeProviderRoutingRow, type ProviderRoutingRow } from "@/lib/agentdetail";
import { isChainRef, chainName, type ChainsState } from "@/lib/chains";
import { type ModelCatalog } from "@/lib/models";
import { RoutingInfo, Row, editableAgentProfile } from "@/components/agentdetail/shared";

// ModelTab answers "which provider/model does this run on, and what happens when
// it fails": the agent's primary model + fallback chain, the global per-task
// chain its task_type resolves to, and the provider/fallback events its runs
// actually produced.
export function ModelTab({
  slug,
  profile,
  routing,
  provLog,
  onManage,
  onChanged,
}: {
  slug: string;
  profile: AgentProfile;
  routing: RoutingInfo | null;
  provLog: ProviderRoutingRow[] | null;
  onManage: (view: string) => void;
  onChanged: () => void;
}) {
  const { toast } = useUI();
  const taskChain = profile.task_type
    ? routing?.chains?.[profile.task_type]
    : undefined;
  const fallbacks = provLog
    ? provLog.filter((r) => r.kind === "fallback")
    : null;

  // A named fallback chain (M963): if this agent's model is "@name", expand it to
  // the chain's real models so the page shows what it will actually run, with a
  // health dot per model (M965). Best-effort fetches — absence just hides them.
  const [chains, setChains] = useState<Record<string, string[]>>({});
  const [cat, setCat] = useState<ModelCatalog | null>(null);
  useEffect(() => {
    let live = true;
    getJSON<ChainsState>("/api/chains")
      .then((c) => live && setChains(c.chains || {}))
      .catch(() => {});
    getJSON<ModelCatalog>("/api/catalog")
      .then((c) => live && setCat(c))
      .catch(() => {});
    return () => {
      live = false;
    };
  }, []);

  // Inline edit of the agent's model straight from the detail page (M970): the
  // ModelPicker surfaces the "Fallback chains" group, so you can point an agent
  // at a named chain (@name) without leaving the page. /api/agents/edit is a full
  // replace, so we send the whole profile with only the model (and, for a chain,
  // the now-redundant per-agent fallbacks cleared) changed.
  const [model, setModel] = useState(profile.model || "");
  const [saving, setSaving] = useState(false);
  useEffect(() => setModel(profile.model || ""), [profile.model]);
  const dirty = model !== (profile.model || "");
  async function saveModel() {
    setSaving(true);
    try {
      await postJSON("/api/agents/edit", {
        ref: slug,
        profile: editableAgentProfile(profile, {
          model,
          fallbacks: isChainRef(model) ? [] : profile.fallbacks || [],
        }),
      });
      toast(
        isChainRef(model)
          ? `Model set to chain @${chainName(model)}`
          : model
            ? `Model set to ${model}`
            : "Model reset to daemon default",
        "success",
      );
      onChanged();
    } catch (e) {
      toast((e as Error).message, "error");
    } finally {
      setSaving(false);
    }
  }

  const modelIsChain = isChainRef(model);
  const expandedChain = modelIsChain ? chains[chainName(model)] : undefined;
  const fallbackCount = fallbacks?.length || 0;

  return (
    <div className="space-y-3">
      <div className="space-y-2 rounded-lg border border-accent/30 bg-accent/5 p-2.5">
        <div className="flex items-center gap-1.5 text-xs font-semibold uppercase tracking-normal text-accent">
          <Cpu className="size-3" /> Change model / fallback chain
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <ModelPicker
            value={model}
            activeModel="daemon default"
            onChange={setModel}
          />
          <Button size="sm" onClick={saveModel} disabled={!dirty || saving}>
            {saving ? "Saving…" : "Save"}
          </Button>
          {dirty && <span className="text-xs text-warn">unsaved</span>}
        </div>
        <p className="text-xs text-muted">
          Pick a single model, or a{" "}
          <span className="text-accent">⛓ fallback chain</span> (defined under{" "}
          <button
            className="underline-offset-2 hover:underline"
            onClick={() => onManage("chains")}
          >
            Fallback Chains
          </button>
          ). A chain is self-contained — per-agent fallbacks are ignored when
          one is selected.
        </p>
      </div>
      <div className="space-y-1.5 rounded-lg border border-border bg-panel/30 p-2.5">
        <Row
          label={dirty ? "model (unsaved)" : "primary model"}
          value={
            modelIsChain ? (
              <span className="inline-flex items-center gap-1 font-mono text-accent">
                <Waypoints className="size-3" /> {chainName(model)}
                {expandedChain && (
                  <span className="text-muted">
                    · {expandedChain.length} model
                    {expandedChain.length === 1 ? "" : "s"}
                  </span>
                )}
              </span>
            ) : model ? (
              <span className="font-mono">{model}</span>
            ) : (
              "(daemon default)"
            )
          }
        />
        {modelIsChain && expandedChain && expandedChain.length > 0 && (
          <Row
            label="chain expands to"
            value={
              <span className="flex flex-wrap items-center gap-1">
                {expandedChain.map((m, i) => (
                  <span key={i} className="inline-flex items-center gap-1">
                    {i > 0 && <ArrowRight className="size-3 text-muted" />}
                    <ModelChip id={m} cat={cat} />
                  </span>
                ))}
              </span>
            }
          />
        )}
        {modelIsChain && expandedChain === undefined && (
          <Row
            label="chain"
            value={
              <span className="text-bad">
                @{chainName(model)} — no such chain (falls through to default)
              </span>
            }
          />
        )}
        {!modelIsChain && (
          <Row
            label="fallback chain"
            value={
              (profile.fallbacks || []).length > 0 ? (
                <span className="flex flex-wrap items-center gap-1 font-mono">
                  {(profile.fallbacks || []).map((m, i) => (
                    <span key={i} className="inline-flex items-center gap-1">
                      {i > 0 && <ArrowRight className="size-3 text-muted" />}
                      <span className="rounded bg-card px-1.5 py-0.5 text-xs">
                        {m}
                      </span>
                    </span>
                  ))}
                </span>
              ) : (
                "none — uses the per-task chain only"
              )
            }
          />
        )}
        <Row label="task type" value={profile.task_type || "—"} />
        {taskChain && taskChain.length > 0 && (
          <Row
            label="task chain (global)"
            value={<span className="font-mono">{taskChain.join(" → ")}</span>}
          />
        )}
      </div>

      <div>
        <div className="mb-1 flex items-center gap-1.5 text-xs uppercase tracking-normal text-muted">
          <Cpu className="size-3" /> routing &amp; fallbacks
        </div>
        <div className="mb-2 flex flex-wrap items-center gap-2 text-[11px]">
          <span
            className={cn(
              "rounded px-1.5 py-0.5 font-medium",
              fallbackCount > 0
                ? "bg-bad/15 text-bad"
                : "bg-good/10 text-good",
            )}
          >
            {fallbackCount > 0 ? "fallback pressure" : "stable route"}
          </span>
          <span className="text-muted">
            {fallbackCount > 0
              ? `${fallbackCount} recent fallback hop(s) recorded for this model/task`
              : "no recent fallback hop recorded for this model/task"}
          </span>
        </div>
        {!provLog ? (
          <SkeletonList count={2} lines={1} />
        ) : provLog.length === 0 ? (
          <div className="text-[11px] text-muted">
            no routing or fallback events relevant to this agent's model/task
          </div>
        ) : (
          <ul className="space-y-1">
            {provLog.slice(0, 30).map((r, i) => (
              <li key={i} className="flex items-start gap-2 text-[11px]">
                {(() => {
                  const summary = summarizeProviderRoutingRow(r);
                  return (
                    <>
                      <span
                        className={cn(
                          "rounded px-1.5 py-0.5 font-mono text-xs",
                          summary.kindTone === "bad"
                            ? "bg-bad/15 text-bad"
                            : "bg-card text-foreground/80",
                        )}
                      >
                        {summary.kindLabel}
                      </span>
                      <span
                        className={cn(
                          "rounded px-1.5 py-0.5 font-medium",
                          summary.stateTone === "bad"
                            ? "bg-bad/10 text-bad"
                            : "bg-good/10 text-good",
                        )}
                      >
                        {summary.stateLabel}
                      </span>
                      <span className="min-w-0 flex-1">
                        <span className="flex flex-wrap items-center gap-1">
                          {summary.failedModel ? (
                            <>
                              <ModelChip
                                id={summary.failedModel}
                                chains={chains}
                                cat={cat}
                              />
                              <ArrowRight className="size-3 text-muted" />
                              <ModelChip
                                id={summary.nextModel || "?"}
                                chains={chains}
                                cat={cat}
                              />
                            </>
                          ) : summary.primaryModel ? (
                            <ModelChip
                              id={summary.primaryModel}
                              chains={chains}
                              cat={cat}
                            />
                          ) : (
                            <span
                              className="block truncate text-foreground/85"
                              title={summary.primaryText}
                            >
                              {summary.primaryText}
                            </span>
                          )}
                        </span>
                        {(summary.secondaryText || summary.detail) && (
                          <span
                            className="block truncate text-muted"
                            title={summary.detail || summary.secondaryText}
                          >
                            {[
                              summary.secondaryText,
                              summary.detail ? clip(summary.detail, 80) : "",
                            ]
                              .filter(Boolean)
                              .join(" — ")}
                          </span>
                        )}
                      </span>
                      <span className="ml-auto shrink-0 font-mono text-xs text-muted opacity-70">
                        {fmtTime(r.ts_unix_ms)}
                      </span>
                    </>
                  );
                })()}
              </li>
            ))}
          </ul>
          )}
        {fallbacks && fallbacks.length > 0 && (
          <div className="mt-1.5 text-xs text-bad">
            a model in this route/chain has been failing over recently; check
            the hop reasons above before trusting the primary path
          </div>
        )}
      </div>

      <div className="flex flex-wrap gap-2">
        {modelIsChain && (
          <Button variant="ghost" size="sm" onClick={() => onManage("chains")}>
            Edit fallback chains <ArrowUpRight className="size-3.5" />
          </Button>
        )}
        <Button variant="ghost" size="sm" onClick={() => onManage("routing")}>
          Edit routing chains <ArrowUpRight className="size-3.5" />
        </Button>
      </div>
    </div>
  );
}

