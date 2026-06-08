// Command-palette item model + fuzzy filter. Pure (no React) so the ranking is
// unit-tested directly.

export interface CommandItem {
  id: string;
  label: string;
  group: string; // "View" | "Action" | "Run" ...
  hint?: string; // secondary text (e.g. a run's status)
  keywords?: string; // extra searchable text
  run: () => void;
}

// subsequence reports whether every char of `q` appears in `s` in order (a
// forgiving fuzzy match), and returns a crude score: lower is better. Returns
// -1 when there's no match.
function fuzzyScore(s: string, q: string): number {
  if (q === "") return 0;
  let si = 0;
  let score = 0;
  let lastHit = -1;
  for (let qi = 0; qi < q.length; qi++) {
    const c = q[qi];
    let found = -1;
    for (; si < s.length; si++) {
      if (s[si] === c) {
        found = si;
        si++;
        break;
      }
    }
    if (found === -1) return -1;
    // Penalise gaps between consecutive matched chars (prefers tight matches).
    score += lastHit === -1 ? found : found - lastHit - 1;
    lastHit = found;
  }
  return score;
}

// filterCommands ranks items against a query: exact substring on the label wins,
// then fuzzy over "label + group + keywords". Empty query returns all items in
// their original order (grouped display handles ordering).
export function filterCommands(items: CommandItem[], query: string): CommandItem[] {
  const q = query.trim().toLowerCase();
  if (q === "") return items;
  const scored: { item: CommandItem; score: number }[] = [];
  for (const item of items) {
    const label = item.label.toLowerCase();
    const hay = `${item.label} ${item.group} ${item.keywords || ""}`.toLowerCase();
    let score: number;
    if (label.includes(q)) {
      // Substring on label: strongly preferred; earlier match ranks higher.
      score = -1000 + label.indexOf(q);
    } else {
      const fz = fuzzyScore(hay, q);
      if (fz < 0) continue;
      score = fz;
    }
    scored.push({ item, score });
  }
  scored.sort((a, b) => a.score - b.score);
  return scored.map((s) => s.item);
}
