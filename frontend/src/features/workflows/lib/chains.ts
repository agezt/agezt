// Named reusable fallback chains (M963). A chain is a named, ordered list of
// model ids the governor tries in turn. Anywhere a model can be picked (agent
// model, per-task routing, chat) the value may instead be a chain REFERENCE —
// the token "@<name>" — which the governor expands to the chain's models at run
// time. Edit a chain in one place and every reference picks up the change.
//
// This module is pure (no fetch/DOM) so the reducer logic is unit-testable; the
// Chains view and ModelPicker fetch /api/chains and call these helpers.

const CHAIN_PREFIX = "@";

export interface ChainsState {
  chains: Record<string, string[]>;
  default: string;
}

// isChainRef reports whether a model value is actually a "@<name>" reference to
// a named chain rather than a real model id (model ids never start with "@").
export function isChainRef(value: string): boolean {
  return value.startsWith(CHAIN_PREFIX) && value.length > CHAIN_PREFIX.length;
}

// chainName extracts the bare name from a "@<name>" reference ("" if not one).
export function chainName(value: string): string {
  return isChainRef(value) ? value.slice(CHAIN_PREFIX.length) : "";
}

// chainRef builds the "@<name>" token stored as a model value.
export function chainRef(name: string): string {
  return CHAIN_PREFIX + name;
}

// chainLabel renders a model value for display: a chain reference shows as
// "⛓ name (N)" with its model count; a plain id passes through unchanged.
export function chainLabel(value: string, chains: Record<string, string[]>): string {
  if (!isChainRef(value)) return value;
  const name = chainName(value);
  const models = chains[name];
  return models && models.length ? `⛓ ${name} (${models.length})` : `⛓ ${name}`;
}

// validateChainName enforces the slug the "@name" token and backend round-trip
// (lower-case letters, digits, dashes; must start alphanumeric). Returns an
// error string or null when valid.
const NAME_RE = /^[a-z0-9][a-z0-9-]*$/;
export function validateChainName(name: string): string | null {
  const n = name.trim();
  if (!n) return "name required";
  if (!NAME_RE.test(n)) return "use lower-case letters, digits, and dashes (e.g. fast-cheap)";
  return null;
}

// moveItem returns a new array with the item at i swapped by dir (-1 up, 1 down);
// a no-op (same reference) at the edges.
export function moveItem<T>(arr: T[], i: number, dir: -1 | 1): T[] {
  const j = i + dir;
  if (j < 0 || j >= arr.length) return arr;
  const next = [...arr];
  [next[i], next[j]] = [next[j], next[i]];
  return next;
}

// removeAt returns a new array without the element at index i.
export function removeAt<T>(arr: T[], i: number): T[] {
  return arr.filter((_, j) => j !== i);
}

// renameChain returns a new chains map with key `from` renamed to `to`,
// preserving its models. No-op when from===to. The default name should be
// re-pointed by the caller if it referenced `from`.
export function renameChain(chains: Record<string, string[]>, from: string, to: string): Record<string, string[]> {
  if (from === to || !(from in chains)) return chains;
  const next: Record<string, string[]> = {};
  for (const [k, v] of Object.entries(chains)) next[k === from ? to : k] = v;
  return next;
}

// deleteChain returns a new chains map without `name`.
export function deleteChain(chains: Record<string, string[]>, name: string): Record<string, string[]> {
  const next = { ...chains };
  delete next[name];
  return next;
}
