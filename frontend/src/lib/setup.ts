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

export interface SetupFallbackCandidate extends SetupModel {
  provider_id: string;
  provider_name?: string;
  credentialed?: boolean;
}

const SETUP_CORE_TASKS = ["chat", "plan", "code", "delegate", "verify"];

// providerKeyEnv picks a provider's API-key env var (the keyring target),
// preferring a *_API_KEY / *_KEY / *_TOKEN name. null = keyless (local).
export function providerKeyEnv(p: SetupProvider): string | null {
  const envs = p.env || [];
  if (!envs.length) return null;
  return envs.find((e) => /(_API_KEY|_KEY|_TOKEN)$/i.test(e)) || envs[0];
}

// anyCredentialed mirrors the backend's readiness bit. API-key providers set it
// when a key exists; keyless/local providers set it when they are runnable.
export function anyCredentialed(cat: SetupCatalog | null | undefined): boolean {
  return !!cat?.providers?.some((p) => p.credentialed);
}

// rankProviders surfaces likely choices first: credentialed, then key-backed,
// then keyless/local; a query filters by id/name.
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

export function setupModelChain(primary: string, fallbacks: string[]): string[] {
  const out: string[] = [];
  const seen = new Set<string>();
  for (const raw of [primary, ...fallbacks]) {
    const id = raw.trim();
    if (!id || seen.has(id)) continue;
    seen.add(id);
    out.push(id);
  }
  return out;
}

export function setupFallbackCandidates(
  cat: SetupCatalog | null | undefined,
  pickedProviderID: string,
  primaryModelID: string,
  limit = 10,
): SetupFallbackCandidate[] {
  const rows: Array<SetupFallbackCandidate & { rank: number; ordinal: number }> = [];
  let ordinal = 0;
  const seen = new Set<string>();
  for (const p of cat?.providers || []) {
    const local = !providerKeyEnv(p);
    const usable = p.id === pickedProviderID || !!p.credentialed || local;
    if (!usable) continue;
    const rank = p.id === pickedProviderID ? 0 : p.credentialed ? 1 : 2;
    for (const m of p.models || []) {
      const id = (m.id || "").trim();
      if (!id || id === primaryModelID || seen.has(id)) continue;
      seen.add(id);
      rows.push({
        id,
        name: m.name,
        provider_id: p.id,
        provider_name: p.name,
        credentialed: p.credentialed,
        rank,
        ordinal: ordinal++,
      });
    }
  }
  return rows
    .sort((a, b) => {
      const rank = a.rank - b.rank;
      return rank !== 0 ? rank : a.ordinal - b.ordinal;
    })
    .slice(0, limit)
    .map(({ rank: _rank, ordinal: _ordinal, ...c }) => c);
}

export function defaultSetupFallbacks(candidates: SetupFallbackCandidate[], max = 2): string[] {
  return candidates.slice(0, max).map((c) => c.id);
}

export function uniqueSetupChainName(existing: Record<string, string[]> | undefined, base = "main"): string {
  const chains = existing || {};
  if (!(base in chains)) return base;
  for (let i = 2; i < 1000; i++) {
    const name = `${base}-${i}`;
    if (!(name in chains)) return name;
  }
  return `${base}-${Date.now()}`;
}

export function setupTaskSelection(taskTypes: string[] | undefined): string[] {
  const available = new Set(taskTypes && taskTypes.length ? taskTypes : SETUP_CORE_TASKS);
  const core = SETUP_CORE_TASKS.filter((t) => available.has(t));
  return core.length ? core : [...available].slice(0, 5);
}

export function mergeSetupTaskRouting(
  existing: Record<string, string[]> | undefined,
  tasks: string[],
  chainRef: string,
): Record<string, string[]> {
  const next: Record<string, string[]> = { ...(existing || {}) };
  for (const task of tasks) {
    const key = task.trim();
    if (key) next[key] = [chainRef];
  }
  return next;
}
