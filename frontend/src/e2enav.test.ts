import { describe, expect, it } from "vitest";
import { readFileSync } from "node:fs";
import { NAV_GROUPS } from "@/nav";

// Both Playwright specs pin the eight section labels as string literals — they
// drive a real browser, so they cannot import from src. That list drifted twice
// in one afternoon (Observe → Watch → Observe, Build → Agents) and neither spec
// noticed until a ten-minute e2e run timed out waiting for a button that no
// longer existed. Vitest can see both worlds, so it holds them together: this
// fails in seconds instead.
const read = (rel: string) => readFileSync(new URL(rel, import.meta.url), "utf8");

function pinnedGroups(src: string): string[] | null {
  // views.spec.ts: `const GROUPS = [ "Talk", … ] as const;`
  const decl = src.match(/const GROUPS = \[([\s\S]*?)\] as const;/);
  // webui.spec.ts: `for (const job of ["Talk", …])`
  const loop = src.match(/for \(const job of \[([\s\S]*?)\]\)/);
  const body = decl?.[1] ?? loop?.[1];
  if (!body) return null;
  return [...body.matchAll(/"([^"]+)"/g)].map((m) => m[1]);
}

// Every openView(section, item[, row]) triple webui.spec.ts drives.
function openViewCalls(src: string): { section: string; item: string; row?: string }[] {
  return [...src.matchAll(/openView\(\s*"([^"]+)",\s*"([^"]+)"(?:,\s*"([^"]+)")?\s*\)/g)].map((m) => ({
    section: m[1],
    item: m[2],
    row: m[3],
  }));
}

describe("webui.spec.ts navigates real destinations", () => {
  const calls = openViewCalls(read("../e2e/webui.spec.ts"));

  it("drives some views", () => {
    expect(calls.length).toBeGreaterThan(5);
  });

  it("names a real section, row and facet for each", () => {
    for (const c of calls) {
      const group = NAV_GROUPS.find((g) => g.label === c.section);
      expect(group, `section "${c.section}"`).toBeTruthy();
      // Without `row`, the item must BE a sidebar row; with it, the item must be
      // one of that row's tabs. A view folded into a tab stops being clickable
      // from <nav>, which is exactly how this spec broke.
      const rowLabel = c.row ?? c.item;
      const row = group!.rows.find((r) => r.label === rowLabel);
      expect(row, `${c.section} › ${rowLabel}`).toBeTruthy();
      if (c.row) {
        expect(
          row!.views.some((v) => v.label === c.item),
          `${c.section} › ${c.row} › ${c.item}`,
        ).toBe(true);
      } else {
        // A single-view row renders no tab strip, so the row label must match.
        expect(row!.views.length === 1 || row!.views[0].label === c.item, `${c.item} needs a row argument`).toBe(true);
      }
    }
  });
});

describe("e2e specs track the nav sections", () => {
  const live = NAV_GROUPS.map((g) => g.label);

  it("views.spec.ts walks every section, in order", () => {
    expect(pinnedGroups(read("../e2e/views.spec.ts"))).toEqual(live);
  });

  it("webui.spec.ts asserts every section is reachable", () => {
    expect(pinnedGroups(read("../e2e/webui.spec.ts"))).toEqual(live);
  });
});
