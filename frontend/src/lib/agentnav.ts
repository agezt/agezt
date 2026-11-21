// agentnav (M960) — the deep-link addressing for a single agent's identity page.
// The app's hash router (App.tsx viewFromHash) is otherwise id-only; one special
// prefix, `#agent/<slug>`, addresses the full-page AgentPage so an agent is
// bookmarkable, shareable, and survives a reload — fixing "I couldn't find the
// agent's own page anywhere". Kept pure (slug parsing takes the hash as an arg)
// so it's unit-testable without a DOM.

export const AGENT_HASH_PREFIX = "agent/";

// openAgent navigates to an agent's identity page. Wherever an agent is shown
// (fleet card, roster card, avatar, activity header), clicking it lands here.
export function openAgent(slug: string): void {
  if (!slug) return;
  location.hash = AGENT_HASH_PREFIX + encodeURIComponent(slug);
}

// agentSlugFromHash extracts the agent slug from a hash like `#agent/<slug>`,
// or returns null when the hash addresses an ordinary nav view. Tolerates the
// optional leading `#`/`#/` the router also strips, and round-trips the encoding
// openAgent applied. A blank slug (`#agent/`) is treated as no selection.
export function agentSlugFromHash(hash: string = typeof location === "undefined" ? "" : location.hash): string | null {
  const raw = hash.replace(/^#\/?/, "");
  if (!raw.startsWith(AGENT_HASH_PREFIX)) return null;
  const encoded = raw.slice(AGENT_HASH_PREFIX.length);
  if (!encoded) return null;
  try {
    return decodeURIComponent(encoded) || null;
  } catch {
    // A malformed %-escape still names an agent literally rather than crashing.
    return encoded || null;
  }
}
