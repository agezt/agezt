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
