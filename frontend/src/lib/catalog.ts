// Capability catalog: join the tool inventory (name + description + governing
// edict capability) with the live trust levels and usage stats into one
// "what can my agent do, and under what policy" picture. Pure + unit-tested;
// the view owns fetching and rendering.

export interface CatalogTool {
  name: string;
  description?: string;
  capability?: string;
}

export interface ToolUsage {
  calls?: number;
  errors?: number;
}

export interface CatalogRow {
  name: string;
  description: string;
  capability: string;
  level: string; // "L0".."L4", or "" when the capability isn't in the policy map
  calls: number;
  errors: number;
}

// joinCatalog merges the three sources by tool name / capability. Tools are
// returned name-sorted (the inventory already sorts, but we don't rely on it).
export function joinCatalog(
  tools: CatalogTool[],
  levels: Record<string, string> | undefined,
  byTool: Record<string, ToolUsage> | undefined,
): CatalogRow[] {
  const lv = levels || {};
  const usage = byTool || {};
  return tools
    .map((t) => {
      const cap = t.capability || "";
      const u = usage[t.name] || {};
      return {
        name: t.name,
        description: t.description || "",
        capability: cap,
        level: cap && lv[cap] ? lv[cap] : "",
        calls: Number(u.calls || 0),
        errors: Number(u.errors || 0),
      };
    })
    .sort((a, b) => a.name.localeCompare(b.name));
}

// levelTone maps a trust level to a colour class — green = freely allowed (L4),
// red = denied (L0), neutral in between. Mirrors the Policy view's scale.
export function levelTone(level: string): string {
  if (level === "L4") return "text-good border-good/40";
  if (level === "L0") return "text-bad border-bad/40";
  if (level === "L1") return "text-warn border-warn/40";
  return "text-muted border-border";
}

// splitDescription cuts a tool's documentation into a one-line gist and the
// rest. Catalog cards used to print the ENTIRE tool doc (op lists, examples,
// caveats — 10+ lines each), turning the page into a wall of prose; the gist
// is the first sentence, the rest folds behind a disclosure.
const GIST_CAP = 220;

export function splitDescription(desc: string): { gist: string; rest: string } {
  const raw = (desc || "").trim();
  if (!raw) return { gist: "", rest: "" };
  const firstLine = raw.split(/\r?\n/, 1)[0] ?? "";
  // First sentence boundary within the first line ("." followed by space/end;
  // also ! and ?). Falls back to the whole first line.
  const m = firstLine.match(/^[\s\S]*?[.!?](?=\s|$)/);
  let gist = (m ? m[0] : firstLine).trim();
  if (gist.length > GIST_CAP) gist = `${gist.slice(0, GIST_CAP).trimEnd()}…`;
  // A truncated gist ("…") no longer prefixes raw — fold the FULL text then,
  // so nothing is lost between the teaser and the disclosure.
  const rest = raw.startsWith(gist) ? raw.slice(gist.length).trim() : raw;
  return { gist, rest };
}

// capabilityCounts tallies rows per Edict capability for the filter chips,
// sorted by count then name. Structural in its input so both the Tool registry
// (CatalogRow) and the usage monitor (ToolView) can use the one implementation.
export function capabilityCounts<T extends { capability: string }>(rows: T[]): { capability: string; n: number }[] {
  const m = new Map<string, number>();
  for (const r of rows) {
    if (!r.capability) continue;
    m.set(r.capability, (m.get(r.capability) || 0) + 1);
  }
  return [...m.entries()]
    .map(([capability, n]) => ({ capability, n }))
    .sort((a, b) => (b.n !== a.n ? b.n - a.n : a.capability.localeCompare(b.capability)));
}

// filterCatalogRows narrows a tool list by free text (name / description /
// capability, case-insensitive) and an optional exact capability.
export function filterCatalogRows<T extends { name: string; description: string; capability: string }>(
  rows: T[],
  query: string,
  capability: string,
): T[] {
  const q = query.trim().toLowerCase();
  return rows.filter((r) => {
    if (capability && r.capability !== capability) return false;
    if (!q) return true;
    return (
      r.name.toLowerCase().includes(q) ||
      r.description.toLowerCase().includes(q) ||
      r.capability.toLowerCase().includes(q)
    );
  });
}
