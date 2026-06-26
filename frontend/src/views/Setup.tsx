import { useCallback, useEffect, useMemo, useState } from "react";
import {
  Wand2,
  Check,
  RefreshCw,
  Search,
  ArrowRight,
  ArrowLeft,
  X,
  Waypoints,
  Route,
  Plus,
  Database,
  KeyRound,
  BrainCircuit,
  LockKeyhole,
  type LucideIcon,
} from "lucide-react";
import { getJSON, postJSON, postAction } from "@/lib/api";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { useUI } from "@/components/ui/feedback";
import { Badge } from "@/components/ui/badge";
import {
  defaultSetupFallbacks,
  mergeSetupTaskRouting,
  providerKeyEnv,
  rankProviders,
  setupFallbackCandidates,
  setupModelChain,
  setupTaskSelection,
  uniqueSetupChainName,
  type SetupCatalog,
  type SetupProvider,
} from "@/lib/setup";
export {
  anyCredentialed,
  defaultSetupFallbacks,
  mergeSetupTaskRouting,
  providerKeyEnv,
  rankProviders,
  setupFallbackCandidates,
  setupModelChain,
  setupTaskSelection,
  uniqueSetupChainName,
  type SetupCatalog,
  type SetupFallbackCandidate,
  type SetupModel,
  type SetupProvider,
} from "@/lib/setup";

// First-run setup wizard (M816): guided onboarding that syncs the catalog, adds
// a provider key, picks a model, configures fallback/routing, and optionally
// sets a console password. Every step rides existing routes, so this is
// sequencing + UX, not new daemon behaviour.

// ---- the wizard ---------------------------------------------------------------

type Step = "catalog" | "provider" | "model" | "routing" | "password" | "done";
type RoutingMode = "simple" | "default" | "tasks";

interface SetupRoutingResp {
  task_types?: string[];
  chains?: Record<string, string[]>;
}

interface SetupChainsResp {
  chains?: Record<string, string[]>;
  default?: string;
}

const SETUP_STEPS: { id: Exclude<Step, "done">; label: string; detail: string; icon: LucideIcon }[] = [
  { id: "catalog", label: "Catalog", detail: "models.dev sync", icon: Database },
  { id: "provider", label: "Provider", detail: "scoped key", icon: KeyRound },
  { id: "model", label: "Model", detail: "primary brain", icon: BrainCircuit },
  { id: "routing", label: "Routing", detail: "fallback ladder", icon: Route },
  { id: "password", label: "Password", detail: "web console", icon: LockKeyhole },
];

const STEP_ORDER: Step[] = ["catalog", "provider", "model", "routing", "password", "done"];

function setupStepIndex(step: Step): number {
  return step === "done" ? SETUP_STEPS.length : Math.max(0, SETUP_STEPS.findIndex((s) => s.id === step));
}

export function Setup({
  overlay = false,
  onDone,
  onSkip,
}: {
  /** rendered as a full-screen first-run layer (vs the plain nav view) */
  overlay?: boolean;
  onDone?: () => void;
  onSkip?: () => void;
}) {
  const ui = useUI();
  const [cat, setCat] = useState<SetupCatalog | null>(null);
  const [loading, setLoading] = useState(false);
  const [step, setStep] = useState<Step>("catalog");
  const [query, setQuery] = useState("");
  const [picked, setPicked] = useState<SetupProvider | null>(null);
  const [keyVal, setKeyVal] = useState("");
  const [busy, setBusy] = useState(false);
  const [selectedModel, setSelectedModel] = useState("");
  const [fallbackModels, setFallbackModels] = useState<string[]>([]);
  const [fallbackInitFor, setFallbackInitFor] = useState("");
  const [routingMode, setRoutingMode] = useState<RoutingMode>("tasks");
  const [routingInfo, setRoutingInfo] = useState<SetupRoutingResp | null>(null);
  const [chainsInfo, setChainsInfo] = useState<SetupChainsResp | null>(null);
  const [routingTasks, setRoutingTasks] = useState<string[]>([]);
  const [providerReadyText, setProviderReadyText] = useState("Provider ready");

  const loadCatalog = useCallback(async () => {
    setLoading(true);
    try {
      const c = await getJSON<SetupCatalog>("/api/catalog");
      setCat(c);
      // Skip straight past steps already satisfied so a re-open isn't tedious.
      if ((c.providers?.length ?? 0) > 0) setStep((s) => (s === "catalog" ? "provider" : s));
    } catch (e) {
      ui.toast((e as Error).message, "error");
    } finally {
      setLoading(false);
    }
  }, [ui]);

  useEffect(() => {
    loadCatalog();
  }, [loadCatalog]);

  const providers = useMemo(() => rankProviders(cat?.providers || [], query), [cat, query]);
  const fallbackCandidates = useMemo(
    () => setupFallbackCandidates(cat, picked?.id || "", selectedModel),
    [cat, picked?.id, selectedModel],
  );
  const effectiveFallbackModels = useMemo(
    () => (fallbackModels.length || fallbackInitFor === selectedModel ? fallbackModels : defaultSetupFallbacks(fallbackCandidates)),
    [fallbackCandidates, fallbackInitFor, fallbackModels, selectedModel],
  );
  const modelChain = useMemo(
    () => setupModelChain(selectedModel, effectiveFallbackModels),
    [selectedModel, effectiveFallbackModels],
  );
  const routeTaskChoices = useMemo(() => setupTaskSelection(routingInfo?.task_types), [routingInfo?.task_types]);
  const effectiveRoutingTasks = useMemo(
    () => (routingTasks.length ? routingTasks : setupTaskSelection(routingInfo?.task_types)),
    [routingInfo?.task_types, routingTasks],
  );
  const pickedKeyEnv = picked ? providerKeyEnv(picked) : null;
  const stepNumber = Math.min(setupStepIndex(step) + 1, SETUP_STEPS.length);
  const currentStep = SETUP_STEPS[Math.min(setupStepIndex(step), SETUP_STEPS.length - 1)];

  useEffect(() => {
    if (providers.length === 0) {
      if (query.trim()) setPicked(null);
      return;
    }
    if (!picked || !providers.some((p) => p.id === picked.id)) setPicked(providers[0]);
  }, [picked, providers, query]);

  useEffect(() => {
    if (step !== "routing" || !selectedModel || fallbackInitFor === selectedModel) return;
    setFallbackModels(defaultSetupFallbacks(fallbackCandidates));
    setFallbackInitFor(selectedModel);
  }, [fallbackCandidates, fallbackInitFor, selectedModel, step]);

  useEffect(() => {
    if (step !== "routing") return;
    let live = true;
    getJSON<SetupRoutingResp>("/api/routing")
      .then((r) => {
        if (!live) return;
        setRoutingInfo(r);
        setRoutingTasks((cur) => (cur.length ? cur : setupTaskSelection(r.task_types)));
      })
      .catch(() => {
        if (live) setRoutingTasks((cur) => (cur.length ? cur : setupTaskSelection(undefined)));
      });
    getJSON<SetupChainsResp>("/api/chains")
      .then((c) => {
        if (live) setChainsInfo(c);
      })
      .catch(() => {});
    return () => {
      live = false;
    };
  }, [step]);

  async function syncCatalog() {
    setBusy(true);
    try {
      await postJSON("/api/catalog/sync", {});
      await loadCatalog();
      setStep("provider");
    } catch (e) {
      ui.toast((e as Error).message, "error");
    } finally {
      setBusy(false);
    }
  }

  async function activatePickedProvider(readyText: string) {
    if (!picked) return;
    setBusy(true);
    try {
      await postJSON("/api/config/set", { name: "AGEZT_PROVIDER", value: picked.id });
      await postAction("/api/provider/reload", {});
      const fresh = await getJSON<SetupCatalog>("/api/catalog").catch(() => null);
      if (fresh) {
        setCat(fresh);
        setPicked(fresh.providers?.find((p) => p.id === picked.id) || picked);
      }
      setProviderReadyText(readyText);
      setStep("model");
    } catch (e) {
      ui.toast((e as Error).message, "error");
    } finally {
      setBusy(false);
    }
  }

  async function applyKey() {
    if (!picked) return;
    if (picked.credentialed) {
      await activatePickedProvider("Stored provider credential reused");
      return;
    }
    const env = pickedKeyEnv;
    if (env) {
      const key = keyVal.trim();
      if (!key) {
        ui.toast("paste the API key first", "error");
        return;
      }
      setBusy(true);
      try {
        await postJSON("/api/provider/keys/add", { provider: picked.id, env, label: "default", value: key, active: true });
        await postJSON("/api/config/set", { name: "AGEZT_PROVIDER", value: picked.id });
        await postAction("/api/provider/reload", {});
        const fresh = await getJSON<SetupCatalog>("/api/catalog").catch(() => null);
        if (fresh) {
          setCat(fresh);
          setPicked(fresh.providers?.find((p) => p.id === picked.id) || picked);
        }
        setProviderReadyText("Provider API key saved");
        setKeyVal("");
        setStep("model");
      } catch (e) {
        ui.toast((e as Error).message, "error");
      } finally {
        setBusy(false);
      }
      return;
    }
    // Keyless/local provider (e.g. Ollama): just pin it.
    await activatePickedProvider("Local/keyless provider selected");
  }

  async function applyModel(modelId: string) {
    setBusy(true);
    try {
      await postJSON("/api/config/set", { name: "AGEZT_MODEL", value: modelId });
      setSelectedModel(modelId);
      setStep("routing");
    } catch (e) {
      ui.toast((e as Error).message, "error");
    } finally {
      setBusy(false);
    }
  }

  function toggleFallback(id: string) {
    setFallbackInitFor(selectedModel);
    setFallbackModels((cur) => {
      const base = cur.length || fallbackInitFor === selectedModel ? cur : defaultSetupFallbacks(fallbackCandidates);
      return base.includes(id) ? base.filter((m) => m !== id) : [...base, id];
    });
  }

  function toggleTask(task: string) {
    setRoutingTasks((cur) => {
      const base = cur.length ? cur : setupTaskSelection(routingInfo?.task_types);
      return base.includes(task) ? base.filter((t) => t !== task) : [...base, task];
    });
  }

  async function applyRouting() {
    if (!selectedModel) {
      ui.toast("pick a model first", "error");
      setStep("model");
      return;
    }
    if (routingMode === "tasks" && effectiveRoutingTasks.length === 0) {
      ui.toast("select at least one task type, or choose Simple", "error");
      return;
    }
    if (routingMode === "simple") {
      setStep("password");
      return;
    }

    setBusy(true);
    try {
      const chainName = uniqueSetupChainName(chainsInfo?.chains, "main");
      const nextChains = { ...(chainsInfo?.chains || {}), [chainName]: modelChain };
      const nextDefault = routingMode === "default" ? chainName : chainsInfo?.default || "";
      await postJSON("/api/chains/set", { chains: nextChains, default: nextDefault });
      setChainsInfo({ chains: nextChains, default: nextDefault });

      if (routingMode === "tasks") {
        const nextRouting = mergeSetupTaskRouting(routingInfo?.chains, effectiveRoutingTasks, `@${chainName}`);
        await postJSON("/api/routing/set", { chains: nextRouting });
        setRoutingInfo({ ...(routingInfo || {}), chains: nextRouting });
      }

      setStep("password");
    } catch (e) {
      ui.toast((e as Error).message, "error");
    } finally {
      setBusy(false);
    }
  }

  // Console password (M933): an optional setup step. Saved as the
  // AGEZT_WEB_PASSWORD secret (vault) via the same config-set route the model
  // step uses; the daemon applies it LIVE, so password login at the bare
  // console address works immediately — no restart, no tokened URL needed.
  const [pwVal, setPwVal] = useState("");
  const [pwVal2, setPwVal2] = useState("");
  async function applyPassword() {
    const pw = pwVal.trim();
    if (!pw) {
      ui.toast("type a password first (or Skip)", "error");
      return;
    }
    if (pw !== pwVal2.trim()) {
      ui.toast("passwords don't match", "error");
      return;
    }
    setBusy(true);
    try {
      await postJSON("/api/config/set", { name: "AGEZT_WEB_PASSWORD", value: pw });
      setPwVal("");
      setPwVal2("");
      setStep("done");
    } catch (e) {
      ui.toast((e as Error).message, "error");
    } finally {
      setBusy(false);
    }
  }

  function goBack() {
    const at = STEP_ORDER.indexOf(step);
    if (at <= 0) return;
    setStep(STEP_ORDER[at - 1]);
  }

  function finishSetup() {
    if (onDone) {
      onDone();
      return;
    }
    if (location.hash.replace(/^#\/?/, "") !== "chat") location.hash = "chat";
  }

  const body = (
    <div className="mx-auto grid w-full max-w-5xl gap-4 p-4 lg:grid-cols-[17rem_minmax(0,1fr)]">
      <aside className="rounded-lg border border-border bg-card p-4 shadow-e1">
        <div className="flex items-start gap-2">
          <div className="flex size-9 items-center justify-center rounded-md bg-accent/10 text-accent">
            <Wand2 className="size-5" />
          </div>
          <div className="min-w-0">
            <h2 className="text-lg font-semibold leading-tight">AGEZT setup</h2>
            <p className="mt-1 text-xs text-muted">Provider, model, routing, console access.</p>
          </div>
        </div>

        <div className="mt-4 rounded-md border border-border bg-panel px-3 py-2">
          <div className="flex items-center justify-between gap-2 text-xs">
            <span className="font-medium text-muted">
              Step {stepNumber} / {SETUP_STEPS.length}
            </span>
            {step === "done" ? <Badge variant="good">done</Badge> : <Badge variant="accent">{currentStep.label}</Badge>}
          </div>
          <div className="mt-2 h-1.5 overflow-hidden rounded-full bg-border">
            <div
              className="h-full rounded-full bg-accent transition-all"
              style={{ width: `${Math.min(100, (setupStepIndex(step) / SETUP_STEPS.length) * 100)}%` }}
            />
          </div>
        </div>

        <Stepper step={step} />
        {overlay && (
          <button className="mt-4 inline-flex items-center gap-1 text-xs text-muted hover:text-foreground" onClick={onSkip} aria-label="Skip setup">
            <X className="h-3.5 w-3.5" />
            Skip for now
          </button>
        )}
      </aside>

      <div className="min-w-0 space-y-4">
        <div className="flex flex-wrap items-center gap-2">
          {step !== "catalog" && step !== "done" && (
            <Button size="sm" variant="ghost" onClick={goBack} aria-label="Back">
              <ArrowLeft className="h-3.5 w-3.5" /> Back
            </Button>
          )}
          <div className="min-w-0">
            <div className="text-xs font-semibold uppercase text-muted">{step === "done" ? "Ready" : currentStep.detail}</div>
            <h3 className="text-xl font-semibold leading-tight">{step === "done" ? "You're ready" : currentStep.label}</h3>
          </div>
        </div>

        <div className="space-y-4">

      {step === "catalog" && (
        <Card title="Model catalog">
          <p className="text-sm text-muted">
            One-time offline sync of the provider/model list. No key needed.
          </p>
          <div className="mt-2 flex items-center gap-2">
            <Button size="sm" onClick={syncCatalog} disabled={busy} aria-label="Sync catalog">
              <RefreshCw className={cn("h-3.5 w-3.5", busy && "animate-spin")} /> Sync models.dev
            </Button>
            {loading && <span className="text-xs text-muted">checking…</span>}
          </div>
        </Card>
      )}

      {step === "provider" && (
        <Card title="Choose a provider and add its key">
          <div className="flex items-center gap-2 rounded-md border border-border bg-panel px-2">
            <Search className="h-3.5 w-3.5 text-muted" />
            <input
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder="search providers — minimax, anthropic, openai, ollama-local…"
              aria-label="Search providers"
              className="w-full bg-transparent py-1.5 text-sm outline-none"
            />
          </div>
          <ul className="mt-2 max-h-52 space-y-1 overflow-y-auto">
            {providers.slice(0, 40).map((p) => (
              <li key={p.id}>
                <button
                  className={cn(
                    "flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left text-sm hover:bg-accent/10",
                    picked?.id === p.id && "bg-accent/15",
                  )}
                  onClick={() => {
                    setPicked(p);
                    setSelectedModel("");
                    setFallbackModels([]);
                    setFallbackInitFor("");
                  }}
                  aria-label={`Pick ${p.id}`}
                >
                  <span className="font-mono">{p.id}</span>
                  {p.credentialed && <Badge variant="good">keyed</Badge>}
                  {!p.credentialed && !providerKeyEnv(p) && <Badge variant="default">local</Badge>}
                  <span className="ml-auto text-xs text-muted">{p.model_count ?? 0} models</span>
                </button>
              </li>
            ))}
            {providers.length === 0 && (
              <li className="rounded-md border border-dashed border-border bg-panel/40 px-2 py-2 text-xs text-muted">
                No providers match this search.
              </li>
            )}
          </ul>
          {picked && (
            <div className="mt-3 space-y-2 border-t border-border pt-3">
              {picked.credentialed ? (
                <div className="rounded-md border border-good/30 bg-good/10 px-2 py-2 text-xs text-good">
                  A stored credential is already active for {picked.id}.
                </div>
              ) : pickedKeyEnv ? (
                <label className="flex flex-col gap-1 text-[11px] text-muted">
                  {picked.id} API key — stored in the encrypted vault as {pickedKeyEnv}
                  <input
                    type="password"
                    value={keyVal}
                    onChange={(e) => setKeyVal(e.target.value)}
                    placeholder="paste your key"
                    aria-label="API key"
                    className="rounded-md border border-border bg-panel px-2 py-1 text-sm outline-none focus-visible:border-accent"
                  />
                </label>
              ) : (
                <p className="text-xs text-muted">{picked.id} is keyless (local) — nothing to paste.</p>
              )}
              <Button size="sm" variant="accent" onClick={applyKey} disabled={busy}>
                <ArrowRight className="h-3.5 w-3.5" /> Use {picked.id}
              </Button>
            </div>
          )}
        </Card>
      )}

      {step === "model" && picked && (
        <Card title={`Pick a model for ${picked.id}`}>
          <ul className="max-h-60 space-y-1 overflow-y-auto">
            {(picked.models || []).slice(0, 60).map((m) => (
              <li key={m.id}>
                <button
                  className="flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left text-sm hover:bg-accent/10"
                  onClick={() => applyModel(m.id)}
                  disabled={busy}
                  aria-label={`Use model ${m.id}`}
                >
                  <span className="font-mono">{m.id}</span>
                  {m.name && <span className="text-xs text-muted">{m.name}</span>}
                </button>
              </li>
            ))}
            {(picked.models || []).length === 0 && (
              <li className="text-xs text-muted">No models listed for this provider — re-sync the catalog.</li>
            )}
          </ul>
        </Card>
      )}

      {step === "routing" && selectedModel && (
        <Card title="Fallbacks and routing">
          <div className="rounded-md border border-border bg-panel/60 p-2">
            <div className="flex items-center gap-2 text-xs">
              <Route className="h-3.5 w-3.5 text-accent" />
              <span className="rounded bg-accent/15 px-1.5 py-0.5 text-[9px] font-medium uppercase text-accent">
                primary
              </span>
              <span className="min-w-0 truncate font-mono">{selectedModel}</span>
            </div>
            {modelChain.slice(1).map((m, i) => (
              <div key={m} className="mt-1 flex items-center gap-2 pl-5 text-xs">
                <Waypoints className="h-3 w-3 text-muted" />
                <span className="rounded bg-card px-1.5 py-0.5 text-[9px] font-medium uppercase text-muted">
                  fallback {i + 1}
                </span>
                <span className="min-w-0 truncate font-mono">{m}</span>
              </div>
            ))}
          </div>

          <div className="mt-3">
            <div className="mb-1 flex items-center gap-1.5 text-[11px] font-semibold uppercase text-muted">
              <Plus className="h-3 w-3" />
              Fallback models
            </div>
            {fallbackCandidates.length > 0 ? (
              <div className="grid gap-1 sm:grid-cols-2">
                {fallbackCandidates.map((c) => {
                  const on = effectiveFallbackModels.includes(c.id);
                  return (
                    <button
                      key={`${c.provider_id}/${c.id}`}
                      onClick={() => toggleFallback(c.id)}
                      aria-label={`${on ? "Remove" : "Add"} fallback ${c.id}`}
                      className={cn(
                        "flex min-w-0 items-center gap-2 rounded-md border px-2 py-1.5 text-left text-xs transition-colors",
                        on ? "border-accent bg-accent/10" : "border-border bg-panel hover:border-accent/60",
                      )}
                    >
                      <Check className={cn("h-3.5 w-3.5 shrink-0", on ? "text-accent" : "text-transparent")} />
                      <span className="min-w-0 flex-1">
                        <span className="block truncate font-mono">{c.id}</span>
                        <span className="block truncate text-[10px] text-muted">{c.provider_name || c.provider_id}</span>
                      </span>
                    </button>
                  );
                })}
              </div>
            ) : (
              <div className="rounded-md border border-dashed border-border bg-panel/40 px-2 py-2 text-xs text-muted">
                No extra runnable models found yet.
              </div>
            )}
          </div>

          <div className="mt-3 grid gap-2 sm:grid-cols-3">
            <ModeButton
              active={routingMode === "simple"}
              icon={Check}
              title="Simple"
              detail="primary model only"
              onClick={() => setRoutingMode("simple")}
            />
            <ModeButton
              active={routingMode === "default"}
              icon={Waypoints}
              title="Default chain"
              detail="use this ladder globally"
              onClick={() => setRoutingMode("default")}
            />
            <ModeButton
              active={routingMode === "tasks"}
              icon={Route}
              title="Core routing"
              detail="route selected task types"
              onClick={() => setRoutingMode("tasks")}
            />
          </div>

          {routingMode === "tasks" && (
            <div className="mt-3 flex flex-wrap gap-1.5">
              {routeTaskChoices.map((task) => (
                <label
                  key={task}
                  className={cn(
                    "inline-flex cursor-pointer items-center gap-1.5 rounded-full border px-2 py-1 text-xs",
                    effectiveRoutingTasks.includes(task) ? "border-accent bg-accent/10 text-accent" : "border-border bg-panel text-muted",
                  )}
                >
                  <input
                    type="checkbox"
                    className="size-3 accent-current"
                    checked={effectiveRoutingTasks.includes(task)}
                    onChange={() => toggleTask(task)}
                    aria-label={`Route ${task} through setup chain`}
                  />
                  <span className="font-mono">{task}</span>
                </label>
              ))}
            </div>
          )}

          <div className="mt-3 flex items-center gap-2">
            <Button
              size="sm"
              onClick={applyRouting}
              disabled={busy}
              aria-label={routingMode === "simple" ? "Continue without routing" : "Save routing"}
            >
              {busy ? <RefreshCw className="h-3.5 w-3.5 animate-spin" /> : <ArrowRight className="h-3.5 w-3.5" />}
              {routingMode === "simple" ? "Continue" : "Save routing"}
            </Button>
            <span className="text-xs text-muted">
              {modelChain.length} model{modelChain.length === 1 ? "" : "s"} in ladder
            </span>
          </div>
        </Card>
      )}

      {step === "password" && (
        <Card title="Console password (optional)">
          <p className="text-sm text-muted">
            Log in at the plain console address — no tokened URL needed. Stored encrypted; applies immediately.
          </p>
          <div className="mt-2 flex max-w-sm flex-col gap-2">
            <input
              type="password"
              value={pwVal}
              onChange={(e) => setPwVal(e.target.value)}
              placeholder="console password"
              aria-label="Console password"
              className="rounded-md border border-border bg-panel px-2 py-1 text-sm outline-none focus-visible:border-accent"
            />
            <input
              type="password"
              value={pwVal2}
              onChange={(e) => setPwVal2(e.target.value)}
              placeholder="repeat password"
              aria-label="Repeat console password"
              className="rounded-md border border-border bg-panel px-2 py-1 text-sm outline-none focus-visible:border-accent"
            />
            <div className="flex items-center gap-2">
              <Button size="sm" onClick={applyPassword} disabled={busy} aria-label="Set console password">
                <ArrowRight className="h-3.5 w-3.5" /> Set password
              </Button>
              <Button size="sm" variant="ghost" onClick={() => setStep("done")} aria-label="Skip password">
                Skip
              </Button>
            </div>
          </div>
        </Card>
      )}

      {step === "done" && (
        <Card title="Setup complete">
          <p className="flex items-start gap-2 text-sm">
            <Check className="mt-0.5 h-4 w-4 shrink-0 text-good" />
            <span>
              {providerReadyText}; model and routing are set. The daemon is ready to use the selected model.
            </span>
          </p>
          <div className="mt-3 flex gap-2">
            <Button size="sm" variant="accent" onClick={finishSetup} aria-label="Start chatting">
              Start chatting <ArrowRight className="h-3.5 w-3.5" />
            </Button>
            <Button size="sm" variant="ghost" onClick={() => setStep("provider")} aria-label="Add another provider">
              Add another provider
            </Button>
          </div>
        </Card>
      )}
        </div>
      </div>
    </div>
  );

  if (overlay) {
    return (
      <div className="fixed inset-0 z-[200] flex items-start justify-center overflow-y-auto bg-background/95 pt-10 backdrop-blur-sm">
        {body}
      </div>
    );
  }
  return body;
}

function Stepper({ step }: { step: Step }) {
  const at = STEP_ORDER.indexOf(step);
  return (
    <ol className="mt-4 grid gap-2 text-xs sm:grid-cols-5 lg:flex lg:flex-col">
      {SETUP_STEPS.map((s, i) => {
        const Icon = s.icon;
        const done = STEP_ORDER.indexOf(s.id) < at;
        const cur = s.id === step;
        return (
          <li
            key={s.id}
            className={cn(
              "flex min-w-0 items-center gap-2 rounded-md border px-2 py-2",
              done && "border-good/30 bg-good/10 text-good",
              cur && "border-accent/40 bg-accent/10 text-accent",
              !done && !cur && "border-border bg-panel text-muted",
            )}
          >
            <span
              className={cn(
                "flex size-6 shrink-0 items-center justify-center rounded-md border bg-card",
                done && "border-good/40",
                cur && "border-accent/50",
                !done && !cur && "border-border",
              )}
            >
              {done ? <Check className="h-3.5 w-3.5" /> : <Icon className="h-3.5 w-3.5" />}
            </span>
            <span className="min-w-0">
              <span className="block truncate font-medium">{s.label}</span>
              <span className="block truncate text-[10px] opacity-75">{s.detail}</span>
            </span>
            <span className="ml-auto hidden text-[10px] opacity-60 sm:inline lg:hidden">{i + 1}</span>
          </li>
        );
      })}
    </ol>
  );
}

function ModeButton({
  active,
  icon: Icon,
  title,
  detail,
  onClick,
}: {
  active: boolean;
  icon: LucideIcon;
  title: string;
  detail: string;
  onClick: () => void;
}) {
  return (
    <button
      type="button"
      aria-pressed={active}
      onClick={onClick}
      className={cn(
        "flex min-w-0 items-center gap-2 rounded-md border px-2 py-2 text-left transition-colors",
        active ? "border-accent bg-accent/10 text-accent" : "border-border bg-panel text-muted hover:border-accent/60",
      )}
    >
      <Icon className="h-4 w-4 shrink-0" />
      <span className="min-w-0">
        <span className="block truncate text-sm font-medium">{title}</span>
        <span className="block truncate text-[10px]">{detail}</span>
      </span>
    </button>
  );
}

function Card({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div className="rounded-lg border border-border bg-card p-3">
      <div className="mb-2 text-xs font-semibold tracking-wide text-muted uppercase">{title}</div>
      {children}
    </div>
  );
}
