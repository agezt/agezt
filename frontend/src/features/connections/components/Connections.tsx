import { useEffect, useMemo, useState } from "react";
import { Network, Plug, Radio, Boxes, ArrowRight, RefreshCw, CheckCircle2, AlertTriangle, Circle, KeyRound, Check, Eye, EyeOff, type LucideIcon } from "lucide-react";
import { getJSON, postJSON } from "@/app/api";
import { cn } from "@/app/utils";
import { Page } from "@/components/ui/page";
import { Card } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import { SkeletonList } from "@/components/ui/skeleton";
import { AnimatedNumber } from "@/components/AnimatedNumber";
import { useUI } from "@/components/ui/feedback";

// Connections — one cockpit for "what's actually wired up". Read-only on the
// Status tab (AI providers, channels, MCP servers, peer nodes), and the
// "Provider Keys" tab for the catalog-driven "attach a key to a provider" flow
// that used to be the standalone Quick Connect gallery. Quick Connect carried
// its own env-var names per preset, which drifted from models.dev and broke
// the keyring → catalog lookup; the new flow sources env names from the
// catalog itself so the wiring is canonical. (See providerPresets removal.)

interface Provider { id: string; name?: string; credentialed?: boolean; env?: string[]; family?: string; api?: string; models?: { id: string; name?: string }[]; model_count?: number }
interface ChannelRow { kind: string; display?: string; live?: boolean; configured?: boolean }
interface MCPServer { name: string; enabled?: boolean; attached?: boolean }
interface NodeRow { id?: string; name: string; local?: boolean; reachable?: boolean; url?: string; version?: string; status?: string }

type Tab = "status" | "keys";

function go(id: string) {
  return () => {
    location.hash = id;
  };
}

export function Connections() {
  const [tab, setTab] = useState<Tab>("status");
  return (
    <Page
      icon={Network}
      title="Connections"
      description="What's actually wired up — providers, channels, MCP servers, and peer nodes"
      width="wide"
      mode="scroll"
      actions={<TabStrip value={tab} onChange={setTab} />}
    >
      {tab === "status" ? <StatusTab onSwitchToKeys={() => setTab("keys")} /> : <ProviderKeysTab />}
    </Page>
  );
}

function TabStrip({ value, onChange }: { value: Tab; onChange: (t: Tab) => void }) {
  const tabs: { id: Tab; label: string; icon: LucideIcon }[] = [
    { id: "status", label: "Status", icon: Network },
    { id: "keys", label: "Provider Keys", icon: KeyRound },
  ];
  return (
    <div className="flex items-center gap-1 rounded-md border border-border bg-card/50 p-0.5" role="tablist" aria-label="Connections tabs">
      {tabs.map((t) => {
        const active = value === t.id;
        return (
          <button
            key={t.id}
            role="tab"
            aria-selected={active}
            onClick={() => onChange(t.id)}
            className={cn(
              "inline-flex h-7 items-center gap-1.5 rounded px-2.5 text-xs font-medium transition-colors",
              active ? "bg-accent/15 text-accent" : "text-muted hover:text-foreground",
            )}
          >
            <t.icon className="size-3.5" />
            {t.label}
          </button>
        );
      })}
    </div>
  );
}

// ---------- Status tab (the original grid) ----------

function StatusTab({ onSwitchToKeys }: { onSwitchToKeys: () => void }) {
  const [providers, setProviders] = useState<Provider[] | null>(null);
  const [channels, setChannels] = useState<ChannelRow[] | null>(null);
  const [servers, setServers] = useState<MCPServer[] | null>(null);
  const [nodes, setNodes] = useState<NodeRow[] | null>(null);
  const [err, setErr] = useState("");
  const [loading, setLoading] = useState(true);

  async function load() {
    setLoading(true);
    setErr("");
    try {
      const [cat, ch, mcp, nodeRes] = await Promise.all([
        getJSON<{ providers?: Provider[] }>("/api/catalog").catch(() => ({ providers: [] })),
        getJSON<{ channels?: ChannelRow[] }>("/api/channels").catch(() => ({ channels: [] })),
        getJSON<{ servers?: MCPServer[] }>("/api/mcp").catch(() => ({ servers: [] })),
        getJSON<{ nodes?: NodeRow[] }>("/api/nodes").catch(() => ({ nodes: [] })),
      ]);
      setProviders(cat.providers || []);
      setChannels(ch.channels || []);
      setServers(mcp.servers || []);
      setNodes(nodeRes.nodes || []);
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setLoading(false);
    }
  }
  useEffect(() => {
    load();
  }, []);

  const keyed = useMemo(() => (providers || []).filter((p) => p.credentialed), [providers]);
  const liveCh = useMemo(() => (channels || []).filter((c) => c.live), [channels]);
  const configuredCh = useMemo(() => (channels || []).filter((c) => c.configured && !c.live), [channels]);
  const attached = useMemo(() => (servers || []).filter((s) => s.attached), [servers]);
  const enabledOnly = useMemo(() => (servers || []).filter((s) => s.enabled && !s.attached), [servers]);
  const reachableNodes = useMemo(() => (nodes || []).filter((n) => n.reachable), [nodes]);
  const unreachableNodes = useMemo(() => (nodes || []).filter((n) => !n.reachable), [nodes]);

  return (
    <>
      {err && <p className="text-sm text-bad">{err}</p>}
      <div className="flex justify-end">
        <Button variant="ghost" size="sm" onClick={load} disabled={loading}>
          <RefreshCw className={cn("size-3.5", loading && "animate-spin")} /> Refresh
        </Button>
      </div>
      {loading && !providers ? (
        <SkeletonList count={3} />
      ) : (
        <div className="grid grid-cols-1 gap-4 lg:grid-cols-2 xl:grid-cols-4">
          <SectionCard
            icon={Plug}
            title="AI Providers"
            connected={keyed.length}
            total={(providers || []).length}
            connectedLabel="keyed"
            items={keyed.map((p) => ({ key: p.id, label: p.name || p.id, tone: "good" as const }))}
            emptyHint="No provider connected"
            actionLabel="Add provider"
            onAction={() => onSwitchToKeys()}
          />
          <SectionCard
            icon={Radio}
            title="Channels"
            connected={liveCh.length}
            total={(channels || []).length}
            connectedLabel="live"
            items={[
              ...liveCh.map((c) => ({ key: c.kind, label: c.display || c.kind, tone: "good" as const })),
              ...configuredCh.map((c) => ({ key: c.kind, label: `${c.display || c.kind} (restart to start)`, tone: "warn" as const })),
            ]}
            emptyHint="No channel live"
            actionLabel="Manage channels"
            onAction={go("channels")}
          />
          <SectionCard
            icon={Boxes}
            title="MCP Servers"
            connected={attached.length}
            total={(servers || []).length}
            connectedLabel="attached"
            items={[
              ...attached.map((s) => ({ key: s.name, label: s.name, tone: "good" as const })),
              ...enabledOnly.map((s) => ({ key: s.name, label: `${s.name} (enabled)`, tone: "warn" as const })),
            ]}
            emptyHint="No MCP server attached"
            actionLabel="Manage MCP"
            onAction={go("mcp")}
          />
          <SectionCard
            icon={Network}
            title="Nodes"
            connected={reachableNodes.length}
            total={(nodes || []).length}
            connectedLabel="reachable"
            items={[
              ...reachableNodes.map((n) => ({ key: n.id || n.name, label: `${n.name}${n.local ? " (local)" : ""}`, tone: "good" as const })),
              ...unreachableNodes.map((n) => ({ key: n.id || n.name, label: `${n.name} (${n.status || "unreachable"})`, tone: "warn" as const })),
            ]}
            emptyHint="No peer nodes configured"
            actionLabel="Manage nodes"
            onAction={go("config")}
          />
        </div>
      )}
    </>
  );
}

// ---------- Provider Keys tab (the replacement for Quick Connect) ----------

interface ProviderFormState {
  env: string;
  key: string;
  model: string;
  makeDefault: boolean;
  reveal: boolean;
  saving: boolean;
}

function defaultFormFor(p: Provider): ProviderFormState {
  return {
    env: p.env?.[0] || "",
    key: "",
    model: p.models?.[0]?.id || "",
    makeDefault: false,
    reveal: false,
    saving: false,
  };
}

function ProviderKeysTab() {
  const ui = useUI();
  const [providers, setProviders] = useState<Provider[] | null>(null);
  const [err, setErr] = useState("");
  const [loading, setLoading] = useState(true);
  const [filter, setFilter] = useState("");
  const [forms, setForms] = useState<Record<string, ProviderFormState>>({});

  async function refresh() {
    setLoading(true);
    setErr("");
    try {
      const r = await getJSON<{ providers?: Provider[] }>("/api/catalog");
      setProviders(r.providers || []);
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setLoading(false);
    }
  }
  useEffect(() => {
    refresh();
  }, []);

  function getForm(id: string, p: Provider): ProviderFormState {
    return forms[id] || defaultFormFor(p);
  }
  function setForm(id: string, patch: Partial<ProviderFormState>) {
    setForms((cur) => {
      const prev = cur[id] || defaultFormFor(providers?.find((p) => p.id === id) as Provider);
      return { ...cur, [id]: { ...prev, ...patch } };
    });
  }

  async function attachKey(p: Provider) {
    const f = getForm(p.id, p);
    if (!p.env || p.env.length === 0) {
      ui.toast(`${p.name || p.id} is keyless — nothing to attach`, "info");
      return;
    }
    if (!f.env) {
      ui.toast("Pick an env var", "error");
      return;
    }
    if (!f.key.trim()) {
      ui.toast("Paste a key first", "error");
      return;
    }
    setForm(p.id, { saving: true });
    try {
      await postJSON("/api/provider/keys/add", { provider: p.id, env: f.env, label: "default", value: f.key.trim(), active: true });
      if (f.makeDefault && f.model) {
        await postJSON("/api/config/set", { name: "AGEZT_PROVIDER", value: p.id });
        await postJSON("/api/config/set", { name: "AGEZT_MODEL", value: f.model });
        await postJSON("/api/provider/reload", {});
        ui.toast(`Keyed ${p.name || p.id} — now the default brain`, "success");
      } else {
        ui.toast(`Keyed ${p.name || p.id}`, "success");
      }
      setForm(p.id, { key: "", reveal: false });
      await refresh();
    } catch (e) {
      ui.toast(`${p.name || p.id}: ${(e as Error).message}`, "error");
    } finally {
      setForm(p.id, { saving: false });
    }
  }

  async function connectKeyless(p: Provider) {
    // Keyless local runtime already in catalog — just make sure it's
    // registered. Catalog-aware connect is a no-op for an id that
    // already exists, so the call is safe either way.
    setForm(p.id, { saving: true });
    try {
      await postJSON("/api/provider/connect", {
        id: p.id, name: p.name || p.id, api: p.api || "", env: "", model: p.models?.[0]?.id || "default", npm: p.family === "anthropic" ? "@ai-sdk/anthropic" : "@ai-sdk/openai-compatible",
      });
      ui.toast(`Connected ${p.name || p.id}`, "success");
      await refresh();
    } catch (e) {
      ui.toast(`${p.name || p.id}: ${(e as Error).message}`, "error");
    } finally {
      setForm(p.id, { saving: false });
    }
  }

  const visible = useMemo(() => {
    const list = providers || [];
    if (!filter.trim()) return list;
    const q = filter.toLowerCase();
    return list.filter((p) => (p.name || p.id).toLowerCase().includes(q) || p.id.toLowerCase().includes(q));
  }, [providers, filter]);

  // Split: keyless first (one-click), then keyed. We rely on `env[]` length —
  // an empty list means "no credentials required" per the catalog convention
  // (see kernel/catalog/types.go HasCredentials).
  const keyless = visible.filter((p) => !p.env || p.env.length === 0);
  const keyed = visible.filter((p) => p.env && p.env.length > 0);

  return (
    <>
      <p className="text-xs text-muted">
        Env var names are taken from the catalog (models.dev sync) — they are the same vars the Governor's credential lookup uses, so a key
        attached here is a key the daemon can actually serve. Existing catalog entries are never overwritten.
      </p>
      <div className="flex items-center gap-2">
        <Input
          aria-label="Filter providers"
          placeholder="Filter providers…"
          value={filter}
          onChange={(e) => setFilter(e.target.value)}
          className="h-8 max-w-xs text-xs"
        />
        <Button variant="ghost" size="sm" onClick={refresh} disabled={loading}>
          <RefreshCw className={cn("size-3.5", loading && "animate-spin")} /> Refresh
        </Button>
      </div>
      {err && <p className="text-sm text-bad">{err}</p>}
      {loading && !providers ? (
        <SkeletonList count={3} />
      ) : (
        <div className="space-y-4">
          {keyless.length > 0 && (
            <Section title="Keyless (local runtimes)">
              {keyless.map((p) => (
                <KeylessRow key={p.id} provider={p} busy={getForm(p.id, p).saving} onConnect={() => connectKeyless(p)} />
              ))}
            </Section>
          )}
          {keyed.length > 0 && (
            <Section title="API-key providers">
              {keyed.map((p) => {
                const f = getForm(p.id, p);
                return (
                  <KeyedRow
                    key={p.id}
                    provider={p}
                    form={f}
                    onChange={(patch) => setForm(p.id, patch)}
                    onSave={() => attachKey(p)}
                  />
                );
              })}
            </Section>
          )}
          <Section title="Anything else">
            <CustomProviderCard onConnected={refresh} />
          </Section>
        </div>
      )}
    </>
  );
}

function Section({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div className="space-y-2">
      <h2 className="text-xs font-semibold uppercase tracking-wide text-muted">{title}</h2>
      <div className="grid grid-cols-1 gap-2">{children}</div>
    </div>
  );
}

function KeyedRow({
  provider,
  form,
  onChange,
  onSave,
}: {
  provider: Provider;
  form: ProviderFormState;
  onChange: (patch: Partial<ProviderFormState>) => void;
  onSave: () => void;
}) {
  const isKeyed = provider.credentialed;
  return (
    <Card glass className="flex flex-col gap-2 p-3">
      <div className="flex flex-wrap items-center gap-2">
        <span className="text-sm font-semibold">{provider.name || provider.id}</span>
        <span className="font-mono text-[10px] text-muted">{provider.id}</span>
        {isKeyed && (
          <span className="inline-flex items-center gap-1 rounded bg-good/15 px-1.5 py-0.5 text-[9px] font-medium uppercase text-good">
            <Check className="size-3" /> keyed
          </span>
        )}
        <Badge className="text-[9px]">{provider.family || "openai-compatible"}</Badge>
        {provider.model_count !== undefined && (
          <span className="text-[10px] text-muted">{provider.model_count} models</span>
        )}
      </div>
      <div className="grid gap-2 sm:grid-cols-[auto_minmax(0,1fr)_auto_minmax(0,1.4fr)_auto]">
        <label className="flex flex-col gap-0.5 text-[10px] text-muted">
          <span>Env</span>
          {provider.env && provider.env.length > 1 ? (
            <select
              aria-label={`${provider.name || provider.id} env`}
              value={form.env}
              onChange={(e) => onChange({ env: e.target.value })}
              className="h-8 rounded-md border border-border bg-card px-2 font-mono text-[11px]"
            >
              {provider.env.map((e) => <option key={e} value={e}>{e}</option>)}
            </select>
          ) : (
            <span className="h-8 rounded-md border border-border bg-card/50 px-2 font-mono text-[11px] leading-8 text-muted">{form.env || "—"}</span>
          )}
        </label>
        <label className="flex flex-col gap-0.5 text-[10px] text-muted">
          <span>Key</span>
          <div className="relative">
            <Input
              type={form.reveal ? "text" : "password"}
              value={form.key}
              onChange={(e) => onChange({ key: e.target.value })}
              placeholder={isKeyed ? "Replace existing key" : "Paste API key"}
              aria-label={`${provider.name || provider.id} key`}
              className="h-8 pr-7 font-mono text-[11px]"
              onKeyDown={(e) => { if (e.key === "Enter") onSave(); }}
            />
            <button
              type="button"
              onClick={() => onChange({ reveal: !form.reveal })}
              aria-label={form.reveal ? "Hide key" : "Reveal key"}
              className="absolute right-1.5 top-1/2 -translate-y-1/2 text-muted hover:text-foreground"
            >
              {form.reveal ? <EyeOff className="size-3.5" /> : <Eye className="size-3.5" />}
            </button>
          </div>
        </label>
        <label className="flex flex-col gap-0.5 text-[10px] text-muted">
          <span>Default model</span>
          {provider.models && provider.models.length > 0 ? (
            <select
              aria-label={`${provider.name || provider.id} default model`}
              value={form.model}
              onChange={(e) => onChange({ model: e.target.value, makeDefault: e.target.value ? form.makeDefault : false })}
              className="h-8 rounded-md border border-border bg-card px-2 font-mono text-[11px]"
            >
              {provider.models.map((m) => <option key={m.id} value={m.id}>{m.id}</option>)}
            </select>
          ) : (
            <Input
              value={form.model}
              onChange={(e) => onChange({ model: e.target.value })}
              placeholder="model id"
              aria-label={`${provider.name || provider.id} default model`}
              className="h-8 font-mono text-[11px]"
            />
          )}
        </label>
        <label className="flex items-end gap-1.5 pb-1 text-[10px] text-muted">
          <input
            type="checkbox"
            checked={form.makeDefault}
            onChange={(e) => onChange({ makeDefault: e.target.checked })}
            disabled={!form.model}
            className="size-3.5 accent-accent"
            aria-label={`Set ${provider.name || provider.id} as default brain`}
          />
          Set as default brain
        </label>
        <div className="flex items-end">
          <Button size="sm" disabled={form.saving} onClick={onSave} className="h-8">
            {form.saving ? <RefreshCw className="size-3.5 animate-spin" /> : <KeyRound className="size-3.5" />}
            {isKeyed ? "Replace key" : "Save key"}
          </Button>
        </div>
      </div>
    </Card>
  );
}

function KeylessRow({ provider, busy, onConnect }: { provider: Provider; busy: boolean; onConnect: () => void }) {
  return (
    <Card glass className="flex items-center gap-3 p-3">
      <div className="min-w-0 flex-1">
        <div className="flex items-center gap-2">
          <span className="text-sm font-semibold">{provider.name || provider.id}</span>
          <span className="font-mono text-[10px] text-muted">{provider.id}</span>
        </div>
        <div className="text-[10px] text-muted">
          {provider.api ? <span className="font-mono">{provider.api}</span> : "local runtime · no key required"}
        </div>
      </div>
      <Button size="sm" disabled={busy} onClick={onConnect} variant={provider.credentialed ? "ghost" : "default"} className="h-8">
        {busy ? <RefreshCw className="size-3.5 animate-spin" /> : <Plug className="size-3.5" />}
        {provider.credentialed ? "Reconnect" : "Connect"}
      </Button>
    </Card>
  );
}

function CustomProviderCard({ onConnected }: { onConnected: () => void }) {
  const ui = useUI();
  const [name, setName] = useState("");
  const [api, setApi] = useState("");
  const [env, setEnv] = useState("");
  const [model, setModel] = useState("");
  const [family, setFamily] = useState<"openai-compatible" | "anthropic">("openai-compatible");
  const [key, setKey] = useState("");
  const [makeDefault, setMakeDefault] = useState(false);
  const [busy, setBusy] = useState(false);

  function slug(s: string) {
    return s.toLowerCase().replace(/[^a-z0-9]+/g, "-").replace(/^-+|-+$/g, "") || "custom-provider";
  }
  const id = slug(name);
  const effectiveEnv = (env.trim() || `${id.toUpperCase().replace(/-/g, "_")}_API_KEY`);

  async function connect() {
    if (!name.trim() || !api.trim() || !model.trim() || !key.trim()) {
      ui.toast("Fill name, URL, model and key", "error");
      return;
    }
    if (!/^[A-Z][A-Z0-9_]*$/.test(effectiveEnv)) {
      ui.toast("Key env must be UPPER_SNAKE (e.g. MYPROV_API_KEY)", "error");
      return;
    }
    setBusy(true);
    try {
      // /api/provider/connect is catalog-aware: existing ids are
      // preserved (no custom.json write); new ids get a minimal
      // partial entry. Either way, the key attaches on the separate
      // keys/add path.
      await postJSON("/api/provider/connect", {
        id, name: name.trim(), npm: family === "anthropic" ? "@ai-sdk/anthropic" : "@ai-sdk/openai-compatible", api: api.trim(), env: effectiveEnv, model: model.trim(),
      });
      await postJSON("/api/provider/keys/add", { provider: id, env: effectiveEnv, label: "default", value: key.trim(), active: true });
      if (makeDefault) {
        await postJSON("/api/config/set", { name: "AGEZT_PROVIDER", value: id });
        await postJSON("/api/config/set", { name: "AGEZT_MODEL", value: model.trim() });
        await postJSON("/api/provider/reload", {});
      }
      ui.toast(makeDefault ? `Connected ${name} — now the default brain` : `Connected ${name}`, "success");
      setName(""); setApi(""); setEnv(""); setModel(""); setKey(""); setMakeDefault(false);
      onConnected();
    } catch (e) {
      ui.toast(`${name}: ${(e as Error).message}`, "error");
    } finally {
      setBusy(false);
    }
  }

  return (
    <Card glass className="flex flex-col gap-2 p-3">
      <div className="flex flex-wrap items-center gap-2">
        <span className="text-sm font-semibold">Custom provider</span>
        <span className="text-[10px] text-muted">Any OpenAI- or Anthropic-compatible API</span>
        <span className="ml-auto font-mono text-[10px] text-muted">{id}</span>
      </div>
      <div className="grid gap-2 sm:grid-cols-2">
        <Input aria-label="Provider name" placeholder="Name" value={name} onChange={(e) => setName(e.target.value)} className="h-8 text-xs" />
        <Input aria-label="Base URL" placeholder="https://…" value={api} onChange={(e) => setApi(e.target.value)} className="h-8 font-mono text-[11px]" />
        <Input aria-label="Env var" placeholder={effectiveEnv} value={env} onChange={(e) => setEnv(e.target.value)} className="h-8 font-mono text-[11px]" />
        <Input aria-label="Model" placeholder="model id" value={model} onChange={(e) => setModel(e.target.value)} className="h-8 font-mono text-[11px]" />
        <Input aria-label="API key" type="password" placeholder="API key" value={key} onChange={(e) => setKey(e.target.value)} className="h-8 font-mono text-[11px]" />
        <div className="flex items-center gap-2">
          <div className="inline-flex rounded-md border border-border bg-card p-0.5" role="group" aria-label="Compatibility">
            {(["openai-compatible", "anthropic"] as const).map((f) => (
              <button
                key={f}
                type="button"
                aria-pressed={family === f}
                onClick={() => setFamily(f)}
                className={cn(
                  "h-7 rounded px-2 text-[10px] font-medium transition-colors",
                  family === f ? "bg-accent/15 text-accent" : "text-muted hover:text-foreground",
                )}
              >
                {f === "anthropic" ? "Anthropic" : "OpenAI"}
              </button>
            ))}
          </div>
          <label className="ml-auto inline-flex items-center gap-1.5 text-[10px] text-muted">
            <input type="checkbox" checked={makeDefault} onChange={(e) => setMakeDefault(e.target.checked)} disabled={!model.trim()} className="size-3.5 accent-accent" />
            Set as default brain
          </label>
        </div>
      </div>
      <div className="flex justify-end">
        <Button size="sm" disabled={busy} onClick={connect} aria-label="Connect custom provider">
          {busy ? <RefreshCw className="size-3.5 animate-spin" /> : <Plug className="size-3.5" />}
          Connect
        </Button>
      </div>
    </Card>
  );
}

// ---------- The legacy cockpit strip (unchanged behaviour) ----------

export function ConnectivityStrip() {
  const [s, setS] = useState<{ providers: number; channels: number; mcp: number; nodes: number } | null>(null);
  useEffect(() => {
    Promise.all([
      getJSON<{ providers?: Provider[] }>("/api/catalog").catch(() => ({ providers: [] })),
      getJSON<{ channels?: ChannelRow[] }>("/api/channels").catch(() => ({ channels: [] })),
      getJSON<{ servers?: MCPServer[] }>("/api/mcp").catch(() => ({ servers: [] })),
      getJSON<{ nodes?: NodeRow[] }>("/api/nodes").catch(() => ({ nodes: [] })),
    ]).then(([cat, ch, mcp, nodeRes]) =>
      setS({
        providers: (cat.providers || []).filter((p) => p.credentialed).length,
        channels: (ch.channels || []).filter((c) => c.live).length,
        mcp: (mcp.servers || []).filter((m) => m.attached).length,
        nodes: (nodeRes.nodes || []).filter((n) => n.reachable).length,
      }),
    );
  }, []);
  if (!s) return null;
  return (
    <button
      onClick={go("connections")}
      className="flex w-full items-center gap-3 rounded-lg border border-border bg-panel/45 p-2.5 text-left text-xs transition-colors hover:border-accent/40"
      title="Open the Connections cockpit"
    >
      <Network className="size-3.5 text-accent" />
      <span className="font-semibold uppercase tracking-normal text-accent">Connections</span>
      <span className="text-muted">
        <AnimatedNumber value={s.providers} className="font-semibold text-fg" /> provider{s.providers === 1 ? "" : "s"} keyed ·{" "}
        <AnimatedNumber value={s.channels} className="font-semibold text-fg" /> channel{s.channels === 1 ? "" : "s"} live ·{" "}
        <AnimatedNumber value={s.mcp} className="font-semibold text-fg" /> MCP attached ·{" "}
        <AnimatedNumber value={s.nodes} className="font-semibold text-fg" /> node{s.nodes === 1 ? "" : "s"} reachable
      </span>
      <ArrowRight className="ml-auto size-3.5 text-muted" />
    </button>
  );
}

function SectionCard({
  icon: Icon,
  title,
  connected,
  total,
  connectedLabel,
  items,
  emptyHint,
  actionLabel,
  onAction,
}: {
  icon: typeof Plug;
  title: string;
  connected: number;
  total: number;
  connectedLabel: string;
  items: { key: string; label: string; tone: "good" | "warn" | "muted" }[];
  emptyHint: string;
  actionLabel: string;
  onAction: () => void;
}) {
  return (
    <Card glass className="flex flex-col gap-3 p-4">
      <div className="flex items-center gap-2">
        <Icon className="size-4 text-accent" />
        <span className="text-sm font-semibold">{title}</span>
        <span className="ml-auto text-[11px] text-muted">
          <AnimatedNumber value={connected} className="font-semibold text-fg" /> {connectedLabel}
          {total > 0 && <span className="text-muted"> / {total}</span>}
        </span>
      </div>
      <div className="flex min-h-16 flex-col gap-1">
        {items.length === 0 ? (
          <p className="text-[11px] text-muted">{emptyHint}</p>
        ) : (
          items.slice(0, 8).map((it) => (
            <div key={it.key + it.label} className="flex items-center gap-2 text-xs">
              {it.tone === "good" && <Badge variant="good"><CheckCircle2 className="size-3" /></Badge>}
              {it.tone === "warn" && <Badge variant="warn"><AlertTriangle className="size-3" /></Badge>}
              {it.tone === "muted" && <Badge variant="default"><Circle className="size-3" /></Badge>}
              <span className="truncate">{it.label}</span>
            </div>
          ))
        )}
        {items.length > 8 && <span className="text-xs text-muted">+{items.length - 8} more</span>}
      </div>
      <Button variant="ghost" size="sm" onClick={onAction} className="mt-auto justify-start">
        {actionLabel} <ArrowRight className="size-3.5" />
      </Button>
    </Card>
  );
}
