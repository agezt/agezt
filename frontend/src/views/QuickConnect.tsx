import { useEffect, useMemo, useState } from "react";
import { Plug, ExternalLink, Check, RefreshCw, Sparkles } from "lucide-react";
import { getJSON, postJSON } from "@/lib/api";
import { cn } from "@/lib/utils";
import { PageHeader } from "@/components/ui/page-header";
import { Card } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import { useUI } from "@/components/ui/feedback";
import {
  PROVIDER_PRESETS,
  familyNpm,
  familyLabel,
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

  return (
    <div className="space-y-6">
      <PageHeader
        icon={Plug}
        title="Quick Connect"
        description="Paste a key and go. Connect a coding/token-plan provider in one step — no manual catalog or endpoint setup."
      />

      <Section title="Coding & token plans" hint="Drop-in coding subscriptions and pay-as-you-go APIs.">
        {coding.map((p) => (
          <PresetCard key={p.id} preset={p} connected={credentialed.has(p.id)} onConnected={refresh} ui={ui} />
        ))}
      </Section>

      <Section title="Popular providers" hint="Fast, low-friction OpenAI-compatible endpoints.">
        {popular.map((p) => (
          <PresetCard key={p.id} preset={p} connected={credentialed.has(p.id)} onConnected={refresh} ui={ui} />
        ))}
      </Section>

      <Section title="Anything else" hint="Connect any OpenAI-compatible provider by URL.">
        <CustomCard onConnected={refresh} ui={ui} />
      </Section>
    </div>
  );
}

function Section({ title, hint, children }: { title: string; hint: string; children: React.ReactNode }) {
  return (
    <div className="space-y-3">
      <div>
        <h2 className="text-sm font-semibold">{title}</h2>
        <p className="text-xs text-muted">{hint}</p>
      </div>
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
    env: args.keyEnv,
    label: "default",
    value: args.key.trim(),
    active: "true",
  });
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
  const [busy, setBusy] = useState(false);

  async function connect() {
    if (!key.trim()) {
      ui.toast("Paste an API key first", "error");
      return;
    }
    setBusy(true);
    try {
      await connectProvider({ id: preset.id, name: preset.name, family: preset.family, api, keyEnv: preset.keyEnv, model, key });
      ui.toast(`Connected ${preset.name}`, "success");
      setKey("");
      onConnected();
    } catch (e) {
      ui.toast(`${preset.name}: ${(e as Error).message}`, "error");
    } finally {
      setBusy(false);
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
          className="inline-flex items-center gap-0.5 text-[10px] text-accent hover:underline"
          title="Get an API key"
        >
          Get key <ExternalLink className="size-3" />
        </a>
      </div>

      <label className="flex flex-col gap-1">
        <span className="text-[10px] uppercase tracking-wide text-muted">API key</span>
        <Input
          type="password"
          value={key}
          onChange={(e) => setKey(e.target.value)}
          placeholder={`${preset.keyEnv}…`}
          aria-label={`${preset.name} ${preset.tagline} key`}
          className="h-8 text-xs"
        />
      </label>

      <details className="group">
        <summary className="cursor-pointer text-[10px] text-muted hover:text-fg">Endpoint & model</summary>
        <div className="mt-2 flex flex-col gap-2">
          <Input value={api} onChange={(e) => setApi(e.target.value)} aria-label="Base URL" className="h-7 font-mono text-[11px]" />
          <Input value={model} onChange={(e) => setModel(e.target.value)} aria-label="Model" className="h-7 font-mono text-[11px]" />
        </div>
      </details>

      <Button size="sm" disabled={busy} onClick={connect} className="mt-auto">
        {busy ? <RefreshCw className="size-3.5 animate-spin" /> : <Plug className="size-3.5" />}
        {connected ? "Reconnect" : "Connect"}
      </Button>
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
  const [busy, setBusy] = useState(false);

  function slug(s: string) {
    return s.toLowerCase().replace(/[^a-z0-9]+/g, "-").replace(/^-+|-+$/g, "") || "custom-provider";
  }

  async function connect() {
    if (!name.trim() || !api.trim() || !keyEnv.trim() || !model.trim() || !key.trim()) {
      ui.toast("Fill name, URL, key env, model and key", "error");
      return;
    }
    if (!/^[A-Z][A-Z0-9_]*$/.test(keyEnv.trim())) {
      ui.toast("Key env must be UPPER_SNAKE (e.g. MYPROV_API_KEY)", "error");
      return;
    }
    setBusy(true);
    try {
      await connectProvider({ id: slug(name), name: name.trim(), family, api, keyEnv: keyEnv.trim(), model, key });
      ui.toast(`Connected ${name}`, "success");
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
        <span className="text-[10px] text-muted">Any OpenAI- or Anthropic-compatible API</span>
      </div>
      <div className="grid grid-cols-1 gap-2 sm:grid-cols-2 lg:grid-cols-5">
        <Input value={name} onChange={(e) => setName(e.target.value)} placeholder="Name" aria-label="Provider name" className="h-8 text-xs" />
        <Input value={api} onChange={(e) => setApi(e.target.value)} placeholder="https://api…/v1" aria-label="Base URL" className="h-8 font-mono text-[11px]" />
        <Input value={keyEnv} onChange={(e) => setKeyEnv(e.target.value)} placeholder="MYPROV_API_KEY" aria-label="Key env var" className="h-8 font-mono text-[11px]" />
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
      <div className="flex items-center gap-2">
        <Input
          type="password"
          value={key}
          onChange={(e) => setKey(e.target.value)}
          placeholder="API key"
          aria-label="Custom provider API key"
          className="h-8 flex-1 text-xs"
        />
        <Button size="sm" disabled={busy} onClick={connect}>
          {busy ? <RefreshCw className="size-3.5 animate-spin" /> : <Plug className="size-3.5" />}
          Connect
        </Button>
      </div>
    </Card>
  );
}
