import { useCallback, useEffect, useMemo, useState, type ReactNode } from "react";
import { Layers, RefreshCw, DownloadCloud, KeyRound, ChevronRight, Search, Zap, Brain, Plus, Trash2, Check, X, type LucideIcon } from "lucide-react";
import { getJSON, postJSON, postAction } from "@/lib/api";
import { cn, fmtDateTime } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { EmptyState } from "@/components/ui/empty";
import { ErrorText } from "@/components/JsonView";
import { SkeletonList } from "@/components/ui/skeleton";
import { useUI } from "@/components/ui/feedback";
import { PageHeader } from "@/components/ui/page-header";
import { Badge } from "@/components/ui/badge";
import { MetricWidget, MetricGrid } from "@/components/ui/metric-widget";

// Models is the LLM model catalog — the providers and models the daemon knows
// about, synced from models.dev/api.json (the same source as `agt catalog sync`).
// It's the "what models can I use, and is this provider keyed?" view, with a
// one-click Sync to refresh from models.dev. Provider/model SELECTION and
// multi-key management are separate follow-on areas.

interface Model {
  id: string;
  name?: string;
  family?: string;
  tool_call?: boolean;
  reasoning?: boolean;
  context?: number;
  output?: number;
  cost_input_usd_per_mtok?: number;
  cost_output_usd_per_mtok?: number;
}
interface Provider {
  id: string;
  name?: string;
  family?: string;
  api?: string;
  doc?: string;
  env?: string[];
  credentialed?: boolean;
  model_count?: number;
  models?: Model[];
}
interface CatalogResp {
  providers?: Provider[];
  api_synced_at?: string;
  api_source_url?: string;
  provider_count?: number;
}
interface KeyInfo {
  label: string;
  active: boolean;
  last4: string;
}

function ModelsModal({
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
      <div className="w-full max-w-md rounded-xl border border-border bg-panel shadow-2xl">
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

// providerKeyEnv picks the provider's API-key env var (the keyring target) from
// its env list — preferring a *_API_KEY / *_KEY / *_TOKEN name, else the first.
// Returns null for providers with no credential env (e.g. local Ollama).
function providerKeyEnv(p: Provider): string | null {
  const envs = p.env || [];
  if (!envs.length) return null;
  return envs.find((e) => /(_API_KEY|_KEY|_TOKEN)$/i.test(e)) || envs[0];
}

// RFC3339 → millis, treating the Go zero time (year <= 1) as "never".
function syncedMs(s?: string): number | null {
  if (!s) return null;
  const t = new Date(s).getTime();
  if (!Number.isFinite(t) || new Date(s).getUTCFullYear() <= 1) return null;
  return t;
}

function fmtContext(n?: number): string {
  if (!n) return "—";
  if (n >= 1000) return `${Math.round(n / 1000)}k`;
  return String(n);
}
function fmtCost(n?: number): string {
  if (n == null || n === 0) return "—";
  return `$${n.toFixed(2)}`;
}

export function Models() {
  const { toast } = useUI();
  const [data, setData] = useState<CatalogResp | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [syncing, setSyncing] = useState(false);
  const [query, setQuery] = useState("");
  const [open, setOpen] = useState<Record<string, boolean>>({});

  const reload = useCallback(async () => {
    setLoading(true);
    try {
      const d = await getJSON<CatalogResp>("/api/catalog");
      setData(d);
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

  async function sync() {
    setSyncing(true);
    try {
      const r = await postJSON<{ provider_count?: number; model_count?: number }>("/api/catalog/sync", {});
      toast(`Synced ${r.provider_count ?? 0} providers · ${r.model_count ?? 0} models`, "success");
      await reload();
    } catch (e) {
      toast(`Sync failed: ${(e as Error).message}`, "error");
    } finally {
      setSyncing(false);
    }
  }

  const providers = data?.providers || [];
  const totalModels = useMemo(() => providers.reduce((n, p) => n + (p.model_count ?? p.models?.length ?? 0), 0), [providers]);
  const syncedAt = syncedMs(data?.api_synced_at);

  const q = query.trim().toLowerCase();
  const filtered = useMemo(() => {
    if (!q) return providers;
    return providers
      .map((p) => {
        const provMatch = p.id.toLowerCase().includes(q) || (p.name || "").toLowerCase().includes(q);
        if (provMatch) return p;
        const models = (p.models || []).filter((m) => m.id.toLowerCase().includes(q) || (m.name || "").toLowerCase().includes(q));
        return models.length ? { ...p, models } : null;
      })
      .filter(Boolean) as Provider[];
  }, [providers, q]);

  return (
    <div className="space-y-4">
      <PageHeader
        icon={Layers}
        title="Models"
        actions={
          <>
            <div className="relative">
              <Search className="pointer-events-none absolute left-2 top-1/2 size-3.5 -translate-y-1/2 text-muted" />
              <Input
                value={query}
                onChange={(e) => setQuery(e.target.value)}
                placeholder="Search models…"
                className="h-8 w-40 pl-7 sm:w-52"
                aria-label="Search models"
              />
            </div>
            <Button size="sm" onClick={sync} disabled={syncing} title="Pull the latest models from models.dev">
              {syncing ? <RefreshCw className="size-3.5 animate-spin" /> : <DownloadCloud className="size-3.5" />} Sync
            </Button>
            <Button variant="ghost" size="sm" onClick={reload} disabled={loading} title="Reload">
              <RefreshCw className={cn("size-3.5", loading && "animate-spin")} />
            </Button>
          </>
        }
      />

      <MetricGrid cols="repeat(auto-fill, minmax(140px, 1fr))">
        <MetricWidget icon={Layers} label="Providers" value={providers.length} tone="muted" />
        <MetricWidget icon={Brain} label="Models" value={totalModels} tone="muted" />
        <MetricWidget
          icon={RefreshCw}
          label="Last synced"
          value={syncedAt ? fmtDateTime(syncedAt) : "Never synced"}
          tone="muted"
        />
      </MetricGrid>

      <ChatGPTSignIn onChanged={reload} />

      {err ? (
        <ErrorText>{err}</ErrorText>
      ) : !data ? (
        <SkeletonList count={5} lines={1} />
      ) : filtered.length === 0 ? (
        <EmptyState
          icon={Layers}
          title={query ? "No models match" : "No providers in the catalog"}
          hint={query ? `Nothing matches “${query}”.` : "Click “Sync models” to pull the catalog from models.dev."}
        />
      ) : (
        <div className="space-y-2">
          {filtered.map((p) => (
            <ProviderCard
              key={p.id}
              provider={p}
              open={!!open[p.id] || !!q}
              onToggle={() => setOpen((o) => ({ ...o, [p.id]: !o[p.id] }))}
              onChanged={reload}
            />
          ))}
        </div>
      )}
    </div>
  );
}

function ProviderCard({
  provider,
  open,
  onToggle,
  onChanged,
}: {
  provider: Provider;
  open: boolean;
  onToggle: () => void;
  onChanged: () => void;
}) {
  const models = provider.models || [];
  const keyEnv = providerKeyEnv(provider);
  return (
    <div className="glass rounded-xl">
      <button onClick={onToggle} className="flex w-full items-center gap-2 px-3 py-2.5 text-left">
        <ChevronRight className={cn("size-3.5 shrink-0 text-muted transition-transform", open && "rotate-90")} />
        <span className="text-sm font-semibold">{provider.name || provider.id}</span>
        <span className="font-mono text-xs text-muted">{provider.id}</span>
        {provider.credentialed ? (
          <Badge variant="good">
              <KeyRound className="size-2.5 mr-1" /> keyed
            </Badge>
        ) : (
          <Badge variant="default">
              <KeyRound className="size-2.5 mr-1" /> no key
            </Badge>
        )}
        <span className="ml-auto text-xs tabular-nums text-muted">
          {models.length || provider.model_count || 0} model{(models.length || provider.model_count) === 1 ? "" : "s"}
        </span>
      </button>
      {open && keyEnv && (
        <div className="border-t border-border/60 p-3">
          <KeyManager env={keyEnv} onChanged={onChanged} />
        </div>
      )}
      {open && models.length > 0 && (
        <div className="border-t border-border/60">
          <table className="w-full text-xs">
            <thead>
              <tr className="border-b border-border/40 text-xs text-muted">
                <th className="px-3 py-1.5 text-left">Model</th>
                <th className="px-2 py-1.5 text-right">Context</th>
                <th className="px-2 py-1.5 text-right">In $/M</th>
                <th className="px-2 py-1.5 text-right">Out $/M</th>
                <th className="px-3 py-1.5 text-right">Caps</th>
              </tr>
            </thead>
            <tbody>
              {models.map((m) => (
                <tr key={m.id} className="border-b border-border/30 last:border-0">
                  <td className="px-3 py-1.5">
                    <span className="font-mono text-foreground/90">{m.id}</span>
                  </td>
                  <td className="px-2 py-1.5 text-right tabular-nums text-muted">{fmtContext(m.context)}</td>
                  <td className="px-2 py-1.5 text-right tabular-nums text-muted">{fmtCost(m.cost_input_usd_per_mtok)}</td>
                  <td className="px-2 py-1.5 text-right tabular-nums text-muted">{fmtCost(m.cost_output_usd_per_mtok)}</td>
                  <td className="px-3 py-1.5">
                    <div className="flex items-center justify-end gap-1">
                      {m.tool_call && (
                        <Badge variant="accent"><Zap className="size-2.5 mr-0.5" />tools</Badge>
                      )}
                      {m.reasoning && (
                        <Badge variant="warn"><Brain className="size-2.5 mr-0.5" />reason</Badge>
                      )}
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}

// ChatGPTSignIn is the "Sign in with ChatGPT" card: connect a ChatGPT
// subscription (Plus/Pro) as a provider via OAuth — no API key. It gates behind
// an explicit acknowledgement that this uses an unofficial backend, opens the
// browser authorize flow (redirect lands on the daemon's 127.0.0.1:1455
// listener), polls to completion, and offers importing a local Codex CLI login.
function ChatGPTSignIn({ onChanged }: { onChanged: () => void }) {
  const { toast, confirm } = useUI();
  const [connected, setConnected] = useState(false);
  const [email, setEmail] = useState("");
  const [busy, setBusy] = useState(false);
  const [status, setStatus] = useState("");

  const refresh = useCallback(async () => {
    try {
      const r = await postJSON<{ connected?: boolean; email?: string }>("/api/provider/oauth/status", { state: "" });
      setConnected(!!r.connected);
      setEmail(r.email || "");
    } catch {
      /* unauthenticated console or daemon down — leave as disconnected */
    }
  }, []);

  useEffect(() => {
    refresh();
  }, [refresh]);

  async function signIn() {
    const ok = await confirm({
      title: "Sign in with ChatGPT?",
      message:
        "This connects your ChatGPT subscription via the same login Codex CLI uses. It relies on an unofficial OpenAI backend — it may stop working or violate OpenAI's terms, and only ever uses your own account. Continue?",
      confirmLabel: "Continue",
      danger: true,
    });
    if (!ok) return;
    setBusy(true);
    setStatus("Opening ChatGPT…");
    try {
      const r = await postJSON<{ authorize_url?: string; state?: string; error?: string }>(
        "/api/provider/oauth/start",
        { provider: "chatgpt" },
      );
      if (!r.authorize_url || !r.state) throw new Error(r.error || "could not start sign-in");
      window.open(r.authorize_url, "_blank", "noopener,noreferrer");
      setStatus("Waiting for you to authorize in the new tab…");
      for (let i = 0; i < 90; i++) {
        await new Promise((res) => setTimeout(res, 2000));
        const st = await postJSON<{ status?: string; error?: string }>("/api/provider/oauth/status", { state: r.state });
        if (st.status === "done") {
          toast("Connected ChatGPT", "success");
          setStatus("");
          await refresh();
          onChanged();
          return;
        }
        if (st.status === "error") throw new Error(st.error || "authorization failed");
      }
      throw new Error("timed out waiting for authorization");
    } catch (e) {
      setStatus("");
      toast((e as Error).message, "error");
    } finally {
      setBusy(false);
    }
  }

  async function importCodex() {
    setBusy(true);
    try {
      const r = await postJSON<{ connected?: boolean; email?: string; error?: string }>(
        "/api/provider/oauth/import",
        {},
      );
      if (!r.connected) throw new Error(r.error || "no Codex CLI login found");
      toast("Imported ChatGPT login from Codex CLI", "success");
      await refresh();
      onChanged();
    } catch (e) {
      toast((e as Error).message, "error");
    } finally {
      setBusy(false);
    }
  }

  async function disconnect() {
    setBusy(true);
    try {
      await postJSON("/api/provider/oauth/logout", {});
      toast("Disconnected ChatGPT", "info");
      await refresh();
      onChanged();
    } catch (e) {
      toast((e as Error).message, "error");
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="glass rounded-xl p-3">
      <div className="flex flex-wrap items-center gap-2">
        <KeyRound className="size-4 text-accent" />
        <span className="text-sm font-medium text-foreground">Sign in with ChatGPT</span>
        {connected ? (
          <Badge variant="good">connected{email ? ` · ${email}` : ""}</Badge>
        ) : (
          <Badge variant="default">not connected</Badge>
        )}
        <div className="ml-auto flex items-center gap-2">
          {connected ? (
            <Button variant="ghost" size="sm" onClick={disconnect} disabled={busy}>
              Disconnect
            </Button>
          ) : (
            <>
              <Button variant="ghost" size="sm" onClick={importCodex} disabled={busy} title="Use a local `codex login` session">
                Import from Codex CLI
              </Button>
              <Button size="sm" onClick={signIn} disabled={busy}>
                {busy ? <RefreshCw className="size-3.5 animate-spin" /> : <KeyRound className="size-3.5" />} Sign in with ChatGPT
              </Button>
            </>
          )}
        </div>
      </div>
      {status && <p className="mt-1.5 text-[11px] text-muted">{status}</p>}
    </div>
  );
}

// KeyManager lists the keys stored for one provider env var and lets the operator
// add another, switch the active one, or remove one — "store many, pick active".
// Values are write-only: existing keys show only a last-4 fingerprint; the add
// field is a password input. Lazy-loads when its provider card is expanded.
function KeyManager({ env, onChanged }: { env: string; onChanged: () => void }) {
  const { toast } = useUI();
  const [keys, setKeys] = useState<KeyInfo[] | null>(null);
  const [label, setLabel] = useState("");
  const [value, setValue] = useState("");
  const [makeActive, setMakeActive] = useState(false);
  const [busy, setBusy] = useState(false);
  const [adding, setAdding] = useState(false);

  const load = useCallback(async () => {
    try {
      const r = await getJSON<{ keys?: KeyInfo[] }>("/api/provider/keys", { env });
      setKeys(r.keys || []);
    } catch (e) {
      toast((e as Error).message, "error");
      setKeys([]);
    }
  }, [env, toast]);
  useEffect(() => {
    load();
  }, [load]);

  async function add() {
    if (!label.trim() || !value.trim()) return;
    setBusy(true);
    try {
      await postJSON("/api/provider/keys/add", { env, label: label.trim(), value, active: makeActive });
      toast(`Added key “${label.trim()}”${makeActive ? " (now active)" : ""}`, "success");
      setLabel("");
      setValue("");
      setMakeActive(false);
      setAdding(false);
      await load();
      onChanged();
    } catch (e) {
      toast((e as Error).message, "error");
    } finally {
      setBusy(false);
    }
  }
  async function activate(l: string) {
    setBusy(true);
    try {
      await postAction("/api/provider/keys/activate", { env, label: l });
      toast(`“${l}” is now the active key`, "success");
      await load();
      onChanged();
    } catch (e) {
      toast((e as Error).message, "error");
    } finally {
      setBusy(false);
    }
  }
  async function remove(l: string) {
    setBusy(true);
    try {
      await postAction("/api/provider/keys/remove", { env, label: l });
      toast(`Removed key “${l}”`, "success");
      await load();
      onChanged();
    } catch (e) {
      toast((e as Error).message, "error");
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="space-y-2">
      <div className="flex items-center gap-1.5 text-[11px] font-semibold text-muted">
        <KeyRound className="size-3" /> API keys
        <code className="rounded bg-panel px-1 font-mono text-xs normal-case tracking-normal text-foreground/70">{env}</code>
      </div>

      {keys === null ? (
        <p className="text-[11px] text-muted">Loading…</p>
      ) : keys.length === 0 ? (
        <div className="flex items-center justify-between gap-2 rounded-md border border-border/60 bg-panel/40 px-2 py-1.5">
          <p className="text-[11px] text-muted">No keys saved in the local keyring.</p>
          <Button size="sm" variant="ghost" onClick={() => setAdding(true)}>
            <Plus className="size-3.5" /> Add key
          </Button>
        </div>
      ) : (
        <div className="space-y-1">
          <ul className="space-y-1">
            {keys.map((k) => (
              <li key={k.label} className="flex items-center gap-2 rounded-md border border-border/60 bg-panel/40 px-2 py-1 text-xs">
                {k.active ? (
                  <Badge variant="good">
                    <Check className="size-2.5 mr-1" /> active
                  </Badge>
                ) : (
                  <button
                    onClick={() => activate(k.label)}
                    disabled={busy}
                    className="rounded border border-border px-1.5 py-0.5 text-[9px] font-medium text-muted transition-colors hover:text-foreground disabled:opacity-50"
                    title="Make this the active key"
                  >
                    activate
                  </button>
                )}
                <span className="font-medium text-foreground">{k.label}</span>
                <span className="font-mono text-xs text-muted">{k.last4}</span>
                <button
                  onClick={() => remove(k.label)}
                  disabled={busy}
                  className="ml-auto text-muted transition-colors hover:text-bad disabled:opacity-50"
                  title="Remove this key"
                >
                  <Trash2 className="size-3.5" />
                </button>
              </li>
            ))}
          </ul>
          <Button size="sm" variant="ghost" onClick={() => setAdding(true)}>
            <Plus className="size-3.5" /> Add key
          </Button>
        </div>
      )}

      {adding && (
        <ModelsModal title={`Add ${env} key`} icon={KeyRound} onClose={() => setAdding(false)}>
          <div className="space-y-3">
            <Input
              value={label}
              onChange={(e) => setLabel(e.target.value)}
              placeholder="label (e.g. work)"
              className="h-8 text-xs"
              aria-label="New key label"
            />
            <Input
              type="password"
              value={value}
              onChange={(e) => setValue(e.target.value)}
              placeholder="key value"
              autoComplete="new-password"
              className="h-8 font-mono text-xs"
              aria-label="New key value"
            />
            <label className="flex items-center gap-1 text-xs text-muted">
              <input type="checkbox" checked={makeActive} onChange={(e) => setMakeActive(e.target.checked)} className="size-3 accent-accent" />
              make active
            </label>
            <div className="flex justify-end gap-2 border-t border-border pt-3">
              <Button size="sm" variant="ghost" onClick={() => setAdding(false)}>
                Cancel
              </Button>
              <Button
                size="sm"
                onClick={add}
                disabled={busy || !label.trim() || !value.trim()}
                title="Add key"
                aria-label="Save new key"
              >
                <Plus className="size-3.5" /> Add
              </Button>
            </div>
          </div>
        </ModelsModal>
      )}
    </div>
  );
}
