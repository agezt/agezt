import { useCallback, useEffect, useMemo, useState } from "react";
import { Wand2, Check, RefreshCw, Search, ArrowRight, X } from "lucide-react";
import { getJSON, postJSON, postAction } from "@/lib/api";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { useUI } from "@/components/ui/feedback";
import { Badge } from "@/components/ui/badge";

// First-run setup wizard (M816): a guided three-step onboarding — sync the
// catalog, add a provider key, pick a model — that lands a brand-new user on a
// thinking daemon without touching a terminal. Every step rides EXISTING
// routes (catalog / provider-keys / config-set), so this is sequencing + UX,
// not new daemon behaviour. Auto-opens as a full-screen overlay when no
// provider is credentialed; also reachable any time from the Setup nav entry.

// ---- shared shapes + pure helpers (exported for tests) ------------------------

export interface SetupModel {
  id: string;
  name?: string;
}
export interface SetupProvider {
  id: string;
  name?: string;
  env?: string[];
  credentialed?: boolean;
  model_count?: number;
  models?: SetupModel[];
}
export interface SetupCatalog {
  providers?: SetupProvider[];
  provider_count?: number;
}

// providerKeyEnv picks a provider's API-key env var (the keyring target),
// preferring a *_API_KEY / *_KEY / *_TOKEN name. null = keyless (local). Mirrors
// the helper in Models.tsx so the two key-entry flows agree.
export function providerKeyEnv(p: SetupProvider): string | null {
  const envs = p.env || [];
  if (!envs.length) return null;
  return envs.find((e) => /(_API_KEY|_KEY|_TOKEN)$/i.test(e)) || envs[0];
}

// anyCredentialed reports whether the catalog already has a usable provider —
// the signal that decides whether the wizard auto-opens. Exported so App can
// reuse the exact rule.
export function anyCredentialed(cat: SetupCatalog | null | undefined): boolean {
  return !!cat?.providers?.some((p) => p.credentialed);
}

// rankProviders surfaces likely choices first: credentialed, then ones that
// take a key, then keyless/local; a query filters by id/name. Pure + tested.
export function rankProviders(providers: SetupProvider[], query: string): SetupProvider[] {
  const q = query.trim().toLowerCase();
  const matched = q
    ? providers.filter((p) => p.id.toLowerCase().includes(q) || (p.name || "").toLowerCase().includes(q))
    : providers;
  return [...matched].sort((a, b) => {
    const score = (p: SetupProvider) => (p.credentialed ? 0 : (p.env?.length ? 1 : 2));
    const d = score(a) - score(b);
    return d !== 0 ? d : a.id.localeCompare(b.id);
  });
}

// ---- the wizard ---------------------------------------------------------------

type Step = "catalog" | "provider" | "model" | "password" | "done";

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

  async function syncCatalog() {
    setBusy(true);
    try {
      await postJSON("/api/catalog/sync", {});
      await loadCatalog();
      setStep("provider");
      ui.toast("catalog synced", "success");
    } catch (e) {
      ui.toast((e as Error).message, "error");
    } finally {
      setBusy(false);
    }
  }

  async function applyKey() {
    if (!picked) return;
    const env = providerKeyEnv(picked);
    if (env) {
      const key = keyVal.trim();
      if (!key) {
        ui.toast("paste the API key first", "error");
        return;
      }
      setBusy(true);
      try {
        await postJSON("/api/provider/keys/add", { env, label: "default", value: key, active: "true" });
        await postJSON("/api/config/set", { name: "AGEZT_PROVIDER", value: picked.id });
        await postAction("/api/provider/reload", {});
        ui.toast(`${picked.id} key saved`, "success");
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
    setBusy(true);
    try {
      await postJSON("/api/config/set", { name: "AGEZT_PROVIDER", value: picked.id });
      await postAction("/api/provider/reload", {});
      ui.toast(`${picked.id} selected (keyless)`, "success");
      setStep("model");
    } catch (e) {
      ui.toast((e as Error).message, "error");
    } finally {
      setBusy(false);
    }
  }

  async function applyModel(modelId: string) {
    setBusy(true);
    try {
      await postJSON("/api/config/set", { name: "AGEZT_MODEL", value: modelId });
      ui.toast(`model set to ${modelId}`, "success");
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
      ui.toast("console password set — you can now log in without the tokened URL", "success");
      setPwVal("");
      setPwVal2("");
      setStep("done");
    } catch (e) {
      ui.toast((e as Error).message, "error");
    } finally {
      setBusy(false);
    }
  }

  const body = (
    <div className="mx-auto flex w-full max-w-2xl flex-col gap-4 p-4">
      <div className="flex items-center gap-2">
        <Wand2 className="h-5 w-5 text-accent" />
        <h2 className="text-lg font-semibold">Set up AGEZT</h2>
        {overlay && (
          <button
            className="ml-auto text-xs text-muted hover:text-foreground"
            onClick={onSkip}
            aria-label="Skip setup"
          >
            <X className="mr-1 inline h-3.5 w-3.5" />
            Skip for now
          </button>
        )}
      </div>
      <Stepper step={step} />

      {step === "catalog" && (
        <Card title="Model catalog">
          <p className="text-sm text-muted">
            AGEZT needs the models.dev catalog (provider + model list) before you can pick one. This is a
            one-time offline sync — no key required.
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
                  onClick={() => setPicked(p)}
                  aria-label={`Pick ${p.id}`}
                >
                  <span className="font-mono">{p.id}</span>
                  {p.credentialed && <Badge variant="good">keyed</Badge>}
                  {!p.credentialed && !providerKeyEnv(p) && <Badge variant="default">local</Badge>}
                  <span className="ml-auto text-[10px] text-muted">{p.model_count ?? 0} models</span>
                </button>
              </li>
            ))}
          </ul>
          {picked && (
            <div className="mt-3 space-y-2 border-t border-border pt-3">
              {providerKeyEnv(picked) ? (
                <label className="flex flex-col gap-1 text-[11px] text-muted">
                  {picked.id} API key — stored in the encrypted vault as {providerKeyEnv(picked)}
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
              <Button size="sm" onClick={applyKey} disabled={busy}>
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
                  {m.name && <span className="text-[10px] text-muted">{m.name}</span>}
                </button>
              </li>
            ))}
            {(picked.models || []).length === 0 && (
              <li className="text-xs text-muted">No models listed for this provider — re-sync the catalog.</li>
            )}
          </ul>
        </Card>
      )}

      {step === "password" && (
        <Card title="Console password (optional)">
          <p className="text-sm text-muted">
            Set a password and you can open this console at its plain address and log in — no tokened URL
            needed. The token from the daemon banner keeps working too. Stored in the encrypted vault;
            applies immediately.
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
        <Card title="You're ready">
          <p className="flex items-center gap-2 text-sm">
            <Check className="h-4 w-4 text-good" /> Provider key saved and your model is set — the daemon is
            thinking with a real model now.
          </p>
          <div className="mt-3 flex gap-2">
            <Button size="sm" onClick={onDone} aria-label="Start chatting">
              Start chatting <ArrowRight className="h-3.5 w-3.5" />
            </Button>
            <Button size="sm" variant="ghost" onClick={() => setStep("provider")} aria-label="Add another provider">
              Add another provider
            </Button>
          </div>
        </Card>
      )}
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
  const steps: { id: Step; label: string }[] = [
    { id: "catalog", label: "Catalog" },
    { id: "provider", label: "Provider + key" },
    { id: "model", label: "Model" },
    { id: "password", label: "Password" },
  ];
  const order: Step[] = ["catalog", "provider", "model", "password", "done"];
  const at = order.indexOf(step);
  return (
    <div className="flex items-center gap-2 text-[11px]">
      {steps.map((s, i) => {
        const done = order.indexOf(s.id) < at;
        const cur = s.id === step;
        return (
          <div key={s.id} className="flex items-center gap-2">
            <span
              className={cn(
                "flex h-5 w-5 items-center justify-center rounded-full border text-[10px]",
                done && "border-good bg-good/15 text-good",
                cur && "border-accent text-accent",
                !done && !cur && "border-border text-muted",
              )}
            >
              {done ? <Check className="h-3 w-3" /> : i + 1}
            </span>
            <span className={cn(cur ? "text-foreground" : "text-muted")}>{s.label}</span>
            {i < steps.length - 1 && <span className="text-muted">→</span>}
          </div>
        );
      })}
    </div>
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
