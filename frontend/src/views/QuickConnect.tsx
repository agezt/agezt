import { useEffect, useMemo, useState } from "react";
import { Plug, ExternalLink, Check, RefreshCw, Sparkles } from "lucide-react";
import { getJSON, postJSON } from "@/lib/api";
import { cn } from "@/lib/utils";
import { Card } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import { useUI } from "@/components/ui/feedback";
import {
  PROVIDER_PRESETS,
  familyNpm,
  familyLabel,
  providerEnvFromId,
  providerScopedKeyLabel,
  type ProviderPreset,
  type PresetFamily,
} from "@/lib/providerPresets";

interface ProviderRow {
  id: string;
  credentialed?: boolean;
}

// The catalog list (/api/catalog) reports every known provider + a credentialed
// flag — unlike /api/providers, which is the routing traffic monitor.
const CATALOG_ENDPOINT = "/api/catalog";

// QuickConnect — a branded gallery to connect AI coding/token-plan providers in
// one step: pick a card, paste a key, and the provider is registered (custom.json)
// + keyed + reloaded live. Base URL + model stay editable per card so a best-effort
// endpoint is never a hard block. A "Custom" card connects any OpenAI-compatible API.
export function QuickConnect() {
  const ui = useUI();
  const [credentialed, setCredentialed] = useState<Set<string>>(new Set());

  async function refresh() {
    try {
      const r = await getJSON<{ providers?: ProviderRow[] }>(CATALOG_ENDPOINT);
      setCredentialed(new Set((r.providers || []).filter((p) => p.credentialed).map((p) => p.id)));
    } catch {
      /* best-effort badge only */
    }
  }
  useEffect(() => {
    refresh();
  }, []);

  const coding = useMemo(() => PROVIDER_PRESETS.filter((p) => p.category === "coding"), []);
  const popular = useMemo(() => PROVIDER_PRESETS.filter((p) => p.category === "popular"), []);
  const local = useMemo(() => PROVIDER_PRESETS.filter((p) => p.category === "local"), []);

  return (
    <div className="space-y-6">
      <div className="flex items-center gap-3">
        <span className="grid size-9 shrink-0 place-items-center rounded-xl bg-gradient-to-br from-accent/25 to-accent2/20 text-accent ring-1 ring-inset ring-accent/30">
          <Plug className="size-5" />
        </span>
        <h2 className="text-gradient text-base font-bold leading-tight tracking-tight">Quick Connect</h2>
      </div>

      <Section title="Coding & token plans">
        {coding.map((p) => (
          <PresetCard key={p.id} preset={p} connected={credentialed.has(p.id)} onConnected={refresh} ui={ui} />
        ))}
      </Section>

      <Section title="Popular providers">
        {popular.map((p) => (
          <PresetCard key={p.id} preset={p} connected={credentialed.has(p.id)} onConnected={refresh} ui={ui} />
        ))}
      </Section>

      <Section title="Local runtimes (no key)">
        {local.map((p) => (
          <PresetCard key={p.id} preset={p} connected={credentialed.has(p.id)} onConnected={refresh} ui={ui} />
        ))}
      </Section>

      <Section title="Anything else">
        <CustomCard onConnected={refresh} ui={ui} />
      </Section>
    </div>
  );
}

function Section({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div className="space-y-3">
      <h2 className="text-sm font-semibold">{title}</h2>
      <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">{children}</div>
    </div>
  );
}

type UIToast = ReturnType<typeof useUI>;

// connectProvider registers the provider (custom.json) then stores the key.
async function connectProvider(args: {
  id: string;
  name: string;
  family: PresetFamily;
  api: string;
  keyEnv: string;
  model: string;
  key: string;
}) {
  await postJSON("/api/provider/connect", {
    id: args.id,
    name: args.name,
    npm: familyNpm(args.family),
    api: args.api.trim(),
    env: args.keyEnv,
    model: args.model.trim(),
  });
  await postJSON("/api/provider/keys/add", {
    provider: args.id,
    env: args.keyEnv,
    label: "default",
    value: args.key.trim(),
    active: true,
  });
}

// connectKeyless registers a local runtime that needs no API key.
async function connectKeyless(args: { id: string; name: string; family: PresetFamily; api: string; model: string }) {
  await postJSON("/api/provider/connect", {
    id: args.id,
    name: args.name,
    npm: familyNpm(args.family),
    api: args.api.trim(),
    env: "",
    model: args.model.trim(),
  });
}

// probeProvider checks whether an endpoint is reachable (daemon-side, SSRF-guarded).
async function probeProvider(api: string, key: string) {
  return postJSON<{ ok: boolean; reachable?: boolean; authorized?: boolean; models?: number; error?: string }>(
    "/api/provider/probe",
    { url: api.trim(), key: key.trim() },
  );
}

// setDefaultProvider pins this provider + model as the daemon's primary brain.
// AGEZT_PROVIDER / AGEZT_MODEL are ApplyLive, so the reload makes it active with
// no restart.
async function setDefaultProvider(id: string, model: string) {
  await postJSON("/api/config/set", { name: "AGEZT_PROVIDER", value: id });
  await postJSON("/api/config/set", { name: "AGEZT_MODEL", value: model.trim() });
  await postJSON("/api/provider/reload", {});
}

function Glyph({ color, glyph }: { color: string; glyph: string }) {
  return (
    <span
      className="flex size-9 shrink-0 items-center justify-center rounded-lg text-sm font-bold text-white shadow-e1"
      style={{ background: color }}
    >
      {glyph}
    </span>
  );
}

function PresetCard({
  preset,
  connected,
  onConnected,
  ui,
}: {
  preset: ProviderPreset;
  connected: boolean;
  onConnected: () => void;
  ui: UIToast;
}) {
  const [api, setApi] = useState(preset.api);
  const [model, setModel] = useState(preset.model);
  const [key, setKey] = useState("");
  const [makeDefault, setMakeDefault] = useState(false);
  const [busy, setBusy] = useState(false);
  const [probe, setProbe] = useState<{ reachable?: boolean; authorized?: boolean; models?: number; error?: string } | null>(null);
  const [probing, setProbing] = useState(false);
  const keyScope = providerScopedKeyLabel(preset.id, preset.keyEnv);

  async function connect() {
    if (!preset.keyless && !key.trim()) {
      ui.toast("Paste an API key first", "error");
      return;
    }
    setBusy(true);
    try {
      if (preset.keyless) {
        await connectKeyless({ id: preset.id, name: preset.name, family: preset.family, api, model });
      } else {
        await connectProvider({ id: preset.id, name: preset.name, family: preset.family, api, keyEnv: preset.keyEnv, model, key });
      }
      if (makeDefault) await setDefaultProvider(preset.id, model);
      ui.toast(makeDefault ? `Connected ${preset.name} — now the default brain` : `Connected ${preset.name}`, "success");
      setKey("");
      onConnected();
    } catch (e) {
      ui.toast(`${preset.name}: ${(e as Error).message}`, "error");
    } finally {
      setBusy(false);
    }
  }

  async function check() {
    setProbing(true);
    setProbe(null);
    try {
      const r = await probeProvider(api, key);
      if (r.ok) setProbe(r);
      else setProbe({ error: r.error || "unreachable" });
    } catch (e) {
      setProbe({ error: (e as Error).message });
    } finally {
      setProbing(false);
    }
  }

  return (
    <Card glass className="flex flex-col gap-3 p-4">
      <div className="flex items-start gap-3">
        <Glyph color={preset.color} glyph={preset.glyph} />
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2">
            <span className="truncate text-sm font-semibold">{preset.name}</span>
            {connected && (
              <span className="inline-flex items-center gap-1 rounded bg-good/15 px-1.5 py-0.5 text-[9px] font-medium uppercase text-good">
                <Check className="size-3" /> keyed
              </span>
            )}
          </div>
          <div className="mt-0.5 flex flex-wrap items-center gap-1">
            <Badge className="text-[9px]">{preset.tagline}</Badge>
            <span className="text-[9px] text-muted">{familyLabel(preset.family)}</span>
          </div>
        </div>
        <a
          href={preset.signupUrl}
          target="_blank"
          rel="noreferrer noopener"
          className="inline-flex items-center gap-0.5 text-xs text-accent hover:underline"
          title="Get an API key"
        >
          Get key <ExternalLink className="size-3" />
        </a>
      </div>

      {preset.keyless ? (
        <span className="text-[11px] text-muted">No API key — just Connect. Make sure the local server is running.</span>
      ) : (
        <label className="flex flex-col gap-1">
          <span className="flex items-center justify-between gap-2 text-xs uppercase tracking-wide text-muted">
            <span>API key</span>
            <span className="max-w-[65%] truncate rounded bg-panel px-1.5 py-0.5 font-mono text-[9px] normal-case tracking-normal text-accent">
              {keyScope}
            </span>
          </span>
          <Input
            type="password"
            value={key}
            onChange={(e) => setKey(e.target.value)}
            placeholder={`Paste key for ${preset.name}`}
            aria-label={`${preset.name} ${preset.tagline} key`}
            className="h-8 text-xs"
          />
        </label>
      )}

      <details className="group">
        <summary className="cursor-pointer text-xs text-muted hover:text-fg">Endpoint, model & key scope</summary>
        <div className="mt-2 flex flex-col gap-2">
          <Input value={api} onChange={(e) => setApi(e.target.value)} aria-label="Base URL" className="h-7 font-mono text-[11px]" />
          <Input value={model} onChange={(e) => setModel(e.target.value)} aria-label="Model" className="h-7 font-mono text-[11px]" />
          {!preset.keyless && (
            <div className="rounded border border-border bg-panel/60 px-2 py-1 font-mono text-[10px] text-muted">
              {keyScope}
            </div>
          )}
        </div>
      </details>

      <label className="flex items-center gap-1.5 text-[11px] text-muted">
        <input type="checkbox" checked={makeDefault} onChange={(e) => setMakeDefault(e.target.checked)} className="size-3.5 accent-accent" />
        Set as default brain
      </label>

      <div className="mt-auto flex items-center gap-2">
        <Button size="sm" disabled={busy} onClick={connect}>
          {busy ? <RefreshCw className="size-3.5 animate-spin" /> : <Plug className="size-3.5" />}
          {connected ? "Reconnect" : "Connect"}
        </Button>
        <Button variant="ghost" size="sm" disabled={probing} onClick={check} title="Check the endpoint is reachable">
          {probing ? <RefreshCw className="size-3.5 animate-spin" /> : <Check className="size-3.5" />}
          Check
        </Button>
      </div>
      {probe && (
        <span
          className={cn(
            "text-[11px]",
            probe.error ? "text-bad" : probe.authorized ? "text-good" : probe.reachable ? "text-warn" : "text-bad",
          )}
        >
          {probe.error
            ? `unreachable: ${probe.error}`
            : probe.authorized
              ? `✓ reachable (${probe.models ?? 0} models)`
              : probe.reachable
                ? "reachable — needs a valid key"
                : "unreachable"}
        </span>
      )}
    </Card>
  );
}

function CustomCard({ onConnected, ui }: { onConnected: () => void; ui: UIToast }) {
  const [name, setName] = useState("");
  const [api, setApi] = useState("");
  const [keyEnv, setKeyEnv] = useState("");
  const [model, setModel] = useState("");
  const [family, setFamily] = useState<PresetFamily>("openai-compatible");
  const [key, setKey] = useState("");
  const [makeDefault, setMakeDefault] = useState(false);
  const [busy, setBusy] = useState(false);

  function slug(s: string) {
    return s.toLowerCase().replace(/[^a-z0-9]+/g, "-").replace(/^-+|-+$/g, "") || "custom-provider";
  }

  const providerId = slug(name);
  const derivedKeyEnv = providerEnvFromId(providerId);
  const effectiveKeyEnv = (keyEnv.trim() || derivedKeyEnv).toUpperCase();
  const keyScope = providerScopedKeyLabel(providerId, effectiveKeyEnv);

  async function connect() {
    if (!name.trim() || !api.trim() || !model.trim() || !key.trim()) {
      ui.toast("Fill name, URL, model and key", "error");
      return;
    }
    if (!/^[A-Z][A-Z0-9_]*$/.test(effectiveKeyEnv)) {
      ui.toast("Key env must be UPPER_SNAKE (e.g. MYPROV_API_KEY)", "error");
      return;
    }
    setBusy(true);
    try {
      await connectProvider({ id: providerId, name: name.trim(), family, api, keyEnv: effectiveKeyEnv, model, key });
      if (makeDefault) await setDefaultProvider(providerId, model);
      ui.toast(makeDefault ? `Connected ${name} — now the default brain` : `Connected ${name}`, "success");
      setKey("");
      onConnected();
    } catch (e) {
      ui.toast(`${name}: ${(e as Error).message}`, "error");
    } finally {
      setBusy(false);
    }
  }

  return (
    <Card glass className="flex flex-col gap-2 p-4 sm:col-span-2 lg:col-span-3">
      <div className="flex items-center gap-2">
        <Sparkles className="size-4 text-accent" />
        <span className="text-sm font-semibold">Custom provider</span>
        <span className="text-xs text-muted">Any OpenAI- or Anthropic-compatible API</span>
      </div>
      <div className="grid grid-cols-1 gap-2 sm:grid-cols-2 lg:grid-cols-4">
        <Input value={name} onChange={(e) => setName(e.target.value)} placeholder="Name" aria-label="Provider name" className="h-8 text-xs" />
        <Input value={api} onChange={(e) => setApi(e.target.value)} placeholder="https://api…/v1" aria-label="Base URL" className="h-8 font-mono text-[11px]" />
        <Input value={model} onChange={(e) => setModel(e.target.value)} placeholder="model-id" aria-label="Model" className="h-8 font-mono text-[11px]" />
        <select
          value={family}
          onChange={(e) => setFamily(e.target.value as PresetFamily)}
          aria-label="Compatibility"
          className={cn("h-8 rounded-md border border-border bg-card px-2 text-xs")}
        >
          <option value="openai-compatible">OpenAI-compatible</option>
          <option value="anthropic">Anthropic-compatible</option>
        </select>
      </div>
      <details className="group">
        <summary className="cursor-pointer text-xs text-muted hover:text-fg">Provider id & vault scope</summary>
        <div className="mt-2 grid gap-2 sm:grid-cols-[minmax(0,1fr)_minmax(11rem,15rem)]">
          <div className="rounded border border-border bg-panel/60 px-2 py-1.5 font-mono text-[10px] text-muted">
            provider id: <span className="text-foreground">{providerId}</span>
            <br />
            vault target: <span className="text-accent">{keyScope}</span>
          </div>
          <Input
            value={keyEnv}
            onChange={(e) => setKeyEnv(e.target.value)}
            placeholder={`auto: ${derivedKeyEnv}`}
            aria-label="Key env var override"
            className="h-8 font-mono text-[11px]"
          />
        </div>
      </details>
      <div className="flex items-center gap-2">
        <Input
          type="password"
          value={key}
          onChange={(e) => setKey(e.target.value)}
          placeholder="API key"
          aria-label="Custom provider API key"
          className="h-8 flex-1 text-xs"
        />
        <label className="flex items-center gap-1.5 whitespace-nowrap text-[11px] text-muted">
          <input type="checkbox" checked={makeDefault} onChange={(e) => setMakeDefault(e.target.checked)} className="size-3.5 accent-accent" />
          Set as default
        </label>
        <Button size="sm" disabled={busy} onClick={connect}>
          {busy ? <RefreshCw className="size-3.5 animate-spin" /> : <Plug className="size-3.5" />}
          Connect
        </Button>
      </div>
    </Card>
  );
}
