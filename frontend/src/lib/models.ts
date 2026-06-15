// The model catalog powers the Chat model picker: /api/catalog returns the full
// provider→model inventory (with capabilities + pricing + whether the provider is
// credentialed). These pure helpers flatten and filter it for the picker UI.

export interface CatalogModel {
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

export interface CatalogProvider {
  id: string;
  name?: string;
  family?: string;
  credentialed?: boolean;
  model_count?: number;
  models?: CatalogModel[];
}

export interface ModelCatalog {
  providers?: CatalogProvider[];
  provider_count?: number;
}

// A flattened, picker-ready option: one model under its provider, carrying the
// capability + pricing facts the picker shows as badges.
export interface ModelOption {
  providerId: string;
  providerName: string;
  credentialed: boolean;
  id: string; // model id — what gets sent as the run's model override
  name: string;
  toolCall: boolean;
  reasoning: boolean;
  context: number;
  costInput: number; // USD per Mtok
  costOutput: number;
}

// One provider group with its (already-filtered) options, for the grouped list.
export interface ModelGroup {
  providerId: string;
  providerName: string;
  credentialed: boolean;
  options: ModelOption[];
}

export function flattenModels(cat: ModelCatalog | null | undefined): ModelOption[] {
  const out: ModelOption[] = [];
  for (const p of cat?.providers || []) {
    for (const m of p.models || []) {
      out.push({
        providerId: p.id,
        providerName: p.name || p.id,
        credentialed: !!p.credentialed,
        id: m.id,
        name: m.name || m.id,
        toolCall: !!m.tool_call,
        reasoning: !!m.reasoning,
        context: Number(m.context || 0),
        costInput: Number(m.cost_input_usd_per_mtok || 0),
        costOutput: Number(m.cost_output_usd_per_mtok || 0),
      });
    }
  }
  return out;
}

// filterModels narrows by a free-text query over model id/name, provider, and
// family — case-insensitive, all terms must match (AND).
export function filterModels(opts: ModelOption[], query: string): ModelOption[] {
  const q = query.trim().toLowerCase();
  if (!q) return opts;
  const terms = q.split(/\s+/);
  return opts.filter((o) => {
    const hay = `${o.id} ${o.name} ${o.providerId} ${o.providerName}`.toLowerCase();
    return terms.every((t) => hay.includes(t));
  });
}

// groupByProvider buckets options under their provider, preserving first-seen
// provider order; credentialed providers sort before un-credentialed ones so the
// models you can actually run float to the top.
export function groupByProvider(opts: ModelOption[]): ModelGroup[] {
  const order: string[] = [];
  const byId = new Map<string, ModelGroup>();
  for (const o of opts) {
    let g = byId.get(o.providerId);
    if (!g) {
      g = { providerId: o.providerId, providerName: o.providerName, credentialed: o.credentialed, options: [] };
      byId.set(o.providerId, g);
      order.push(o.providerId);
    }
    g.options.push(o);
  }
  const groups = order.map((id) => byId.get(id)!);
  return groups.sort((a, b) => Number(b.credentialed) - Number(a.credentialed));
}

// pinnedOptions resolves an ordered model-id list (e.g. the chat task's routing
// chain) into picker options, preserving the given order: first match per id,
// preferring a credentialed provider when several serve the same model. Ids not
// in opts (filtered out by search, or unknown to the catalog) are skipped — the
// pinned group simply shrinks. Powers the picker's "routing chain first" group
// (M931) so the models the chat will actually fall back through lead the list.
export function pinnedOptions(opts: ModelOption[], ids: string[]): ModelOption[] {
  const out: ModelOption[] = [];
  for (const id of ids) {
    if (!id || out.some((o) => o.id === id)) continue;
    const matches = opts.filter((o) => o.id === id);
    if (!matches.length) continue;
    out.push(matches.find((o) => o.credentialed) ?? matches[0]);
  }
  return out;
}

// findModelContext looks up a model's context window (tokens) in the catalog by
// model id — what the chat's context bar divides by. 0 when the model isn't in
// the catalog (the bar then shows absolute usage without a percentage).
export function findModelContext(cat: ModelCatalog | null | undefined, modelId: string): number {
  if (!modelId) return 0;
  for (const p of cat?.providers || []) {
    for (const m of p.models || []) {
      if (m.id === modelId) return Number(m.context || 0);
    }
  }
  return 0;
}

// ModelHealth reflects whether a model id can actually run right now:
//  - "ok": at least one CREDENTIALED provider serves this model id
//  - "nokey": the model exists in the catalog but no credentialed provider has
//    it (add an API key to run it)
//  - "unknown": no provider in the catalog lists this id (typo / removed model)
export type ModelHealth = "ok" | "nokey" | "unknown";

// modelHealth resolves a model id against the catalog. Used to flag chain models
// that won't run (M965) so a fallback ladder can't silently contain dead links.
export function modelHealth(cat: ModelCatalog | null | undefined, modelId: string): ModelHealth {
  if (!modelId) return "unknown";
  let found = false;
  for (const p of cat?.providers || []) {
    for (const m of p.models || []) {
      if (m.id === modelId) {
        if (p.credentialed) return "ok";
        found = true;
      }
    }
  }
  return found ? "nokey" : "unknown";
}

// fmtContext renders a context window compactly: 128000 → "128K", 1000000 → "1M".
export function fmtContext(n: number): string {
  if (!n) return "";
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(n % 1_000_000 ? 1 : 0)}M`;
  if (n >= 1000) return `${Math.round(n / 1000)}K`;
  return String(n);
}
