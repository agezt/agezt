// Shared agent-identity helpers (M948): a deterministic hue + monogram per
// agent slug, so an agent wears the same colour and initials everywhere — the
// roster, the fleet strip, the cockpit. Pure + unit-tested. Re-exported from
// views/Roster for back-compat with existing imports.

// agentHue maps a slug to a stable hue (0..359) via a tiny string hash.
export function agentHue(slug: string): number {
  let h = 0;
  for (let i = 0; i < slug.length; i++) h = (h * 31 + slug.charCodeAt(i)) % 360;
  return h;
}

// initials derives a 1–2 char monogram for the avatar: from the name's words if
// present, else the first two slug characters.
export function initials(name: string | undefined, slug: string): string {
  const src = (name || "").trim();
  if (src) {
    const words = src.split(/\s+/).filter(Boolean);
    if (words.length >= 2) return (words[0][0] + words[1][0]).toUpperCase();
    return src.slice(0, 2).toUpperCase();
  }
  return slug.slice(0, 2).toUpperCase();
}
