import { describe, it, expect } from "vitest";
import { HELP, FALLBACK_TOPIC, helpTopicFor } from "@/app/help";
import { NAV } from "@/nav";

// Coverage is DERIVED from nav.tsx, not a hand-kept list. The list version drifted
// silently: it still named views by their pre-2026-09 grouping, so removing a view
// left an unreachable topic behind and nothing noticed.
//
// The two detail routes have topics but are not nav views: `#agent/<slug>` and
// `#incident/<id>` render their own full pages.
const DETAIL_ROUTES = ["agent", "incident"];
const NAV_IDS = [...NAV.map((n) => n.id), ...DETAIL_ROUTES];

describe("help content coverage", () => {
  it("has a topic for every nav view id", () => {
    const missing = NAV_IDS.filter((id) => !HELP[id]);
    expect(missing).toEqual([]);
  });

  it("has no orphan topics for views that don't exist", () => {
    const orphans = Object.keys(HELP).filter((id) => !NAV_IDS.includes(id));
    expect(orphans).toEqual([]);
  });

  it("every topic is substantial: intro, at least one section, non-empty content", () => {
    for (const [id, t] of Object.entries(HELP)) {
      expect(t.title, id).toBeTruthy();
      expect(t.intro.length, `${id} intro`).toBeGreaterThan(40);
      expect(t.sections.length, `${id} sections`).toBeGreaterThan(0);
      for (const s of t.sections) {
        expect(s.heading, `${id} section heading`).toBeTruthy();
        const hasBody = (s.paragraphs?.length || 0) > 0 || (s.items?.length || 0) > 0;
        expect(hasBody, `${id} section "${s.heading}" must have paragraphs or items`).toBe(true);
        for (const it of s.items || []) {
          expect(it.term, `${id} item term`).toBeTruthy();
          expect(it.desc.length, `${id} item "${it.term}" desc`).toBeGreaterThan(20);
        }
      }
    }
  });

  it("every related link points at a real topic", () => {
    for (const [id, t] of Object.entries(HELP)) {
      for (const r of t.related || []) {
        expect(HELP[r.id], `${id} → related "${r.id}"`).toBeTruthy();
        expect(r.label, `${id} → related "${r.id}" label`).toBeTruthy();
      }
    }
  });

  it("falls back gracefully for unknown view ids", () => {
    expect(helpTopicFor("not-a-view")).toBe(FALLBACK_TOPIC);
    expect(helpTopicFor("chat")).toBe(HELP.chat);
  });
});
