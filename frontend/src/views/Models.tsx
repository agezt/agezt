import { useCallback, useEffect, useMemo, useState } from "react";
import { Layers, RefreshCw, DownloadCloud, KeyRound, ChevronRight, Search, Zap, Brain } from "lucide-react";
import { getJSON, postJSON } from "@/lib/api";
import { cn, fmtDateTime } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { EmptyState } from "@/components/ui/empty";
import { ErrorText } from "@/components/JsonView";
import { SkeletonList } from "@/components/ui/skeleton";
import { useUI } from "@/components/ui/feedback";

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
      <div className="flex flex-wrap items-center gap-2">
        <h2 className="flex items-center gap-2 text-sm font-semibold">
          <Layers className="size-4 text-accent" /> Models
        </h2>
        {data && (
          <span className="text-xs text-muted">
            {providers.length} providers · {totalModels} models
          </span>
        )}
        <div className="relative ml-auto">
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
          {syncing ? <RefreshCw className="size-3.5 animate-spin" /> : <DownloadCloud className="size-3.5" />} Sync models
        </Button>
        <Button variant="ghost" size="sm" onClick={reload} disabled={loading} title="Reload">
          <RefreshCw className={cn("size-3.5", loading && "animate-spin")} />
        </Button>
      </div>

      <p className="text-xs text-muted">
        {syncedAt ? (
          <>
            Last synced {fmtDateTime(syncedAt)}
            {data?.api_source_url ? (
              <>
                {" "}
                from <code className="rounded bg-panel px-1">{data.api_source_url}</code>
              </>
            ) : null}
            .
          </>
        ) : (
          <>Never synced — click “Sync models” to pull the catalog from models.dev.</>
        )}
      </p>

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
            <ProviderCard key={p.id} provider={p} open={!!open[p.id] || !!q} onToggle={() => setOpen((o) => ({ ...o, [p.id]: !o[p.id] }))} />
          ))}
        </div>
      )}
    </div>
  );
}

function ProviderCard({ provider, open, onToggle }: { provider: Provider; open: boolean; onToggle: () => void }) {
  const models = provider.models || [];
  return (
    <div className="rounded-lg border border-border bg-card">
      <button onClick={onToggle} className="flex w-full items-center gap-2 px-3 py-2.5 text-left">
        <ChevronRight className={cn("size-3.5 shrink-0 text-muted transition-transform", open && "rotate-90")} />
        <span className="text-sm font-semibold">{provider.name || provider.id}</span>
        <span className="font-mono text-[10px] text-muted">{provider.id}</span>
        {provider.credentialed ? (
          <span className="inline-flex items-center gap-1 rounded bg-good/15 px-1.5 py-0.5 text-[9px] font-medium uppercase text-good" title="An API key is configured for this provider">
            <KeyRound className="size-2.5" /> keyed
          </span>
        ) : (
          <span className="inline-flex items-center gap-1 rounded bg-panel px-1.5 py-0.5 text-[9px] font-medium uppercase text-muted" title="No API key configured">
            <KeyRound className="size-2.5" /> no key
          </span>
        )}
        <span className="ml-auto text-[10px] tabular-nums text-muted">
          {models.length || provider.model_count || 0} model{(models.length || provider.model_count) === 1 ? "" : "s"}
        </span>
      </button>
      {open && models.length > 0 && (
        <div className="border-t border-border/60">
          <table className="w-full text-xs">
            <thead className="text-[10px] uppercase tracking-wide text-muted">
              <tr className="border-b border-border/40">
                <th className="px-3 py-1.5 text-left font-medium">Model</th>
                <th className="px-2 py-1.5 text-right font-medium">Context</th>
                <th className="px-2 py-1.5 text-right font-medium">In $/M</th>
                <th className="px-2 py-1.5 text-right font-medium">Out $/M</th>
                <th className="px-3 py-1.5 text-right font-medium">Caps</th>
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
                        <span className="inline-flex items-center gap-0.5 rounded bg-accent/15 px-1 text-[9px] text-accent" title="Supports tool calls">
                          <Zap className="size-2.5" /> tools
                        </span>
                      )}
                      {m.reasoning && (
                        <span className="inline-flex items-center gap-0.5 rounded bg-violet-500/15 px-1 text-[9px] text-violet-300" title="Reasoning model">
                          <Brain className="size-2.5" /> reason
                        </span>
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
