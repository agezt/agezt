import { describe, expect, it } from "vitest";
import { readFileSync } from "node:fs";
import { NAV_GROUPS } from "@/nav";

// docs/CONSOLE.md is the only prose map of the console, and it silently drifted
// a whole IA behind the code once already (it still described the pre-M974
// six-section nav long after eight shipped). This guard pins the map to
// nav.tsx: reorganise the sidebar and this test tells you to rewrite the doc.
const DOC = readFileSync(new URL("../../docs/CONSOLE.md", import.meta.url), "utf8");
// Whitespace is collapsed so a label the doc happens to wrap across two lines
// ("*Tool" / "registry*") still matches.
const MAP = DOC.slice(
  DOC.indexOf("## The views, at a glance"),
  DOC.indexOf("## Steering the proactive heartbeat"),
).replace(/\s+/g, " ");

describe("docs/CONSOLE.md view map", () => {
  it("exists as its own section", () => {
    expect(MAP.length).toBeGreaterThan(200);
  });

  it("names every section", () => {
    for (const g of NAV_GROUPS) expect(MAP, g.label).toContain(`**${g.label}**`);
  });

  it("names every destination and every facet", () => {
    for (const g of NAV_GROUPS) {
      for (const r of g.rows) {
        expect(MAP, `${g.label} → ${r.label}`).toContain(r.label);
        for (const v of r.views) expect(MAP, `${r.label} → ${v.label}`).toContain(v.label);
      }
    }
  });

  it("does not describe sections that no longer exist", () => {
    const live = new Set(NAV_GROUPS.map((g) => g.label));
    for (const stale of ["Converse", "Monitor", "Automation", "Observe", "Build"]) {
      if (live.has(stale)) continue;
      expect(MAP, `stale section "${stale}"`).not.toContain(`**${stale}**`);
    }
  });
});
