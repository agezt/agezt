import { describe, it, expect } from "vitest";
import { HELP, FALLBACK_TOPIC, helpTopicFor } from "@/lib/help";

// Mirror of the view ids in App.tsx NAV_GROUPS. When a view is added to the
// sidebar, add its id here AND write its topic in lib/help.ts — this test is
// the guard that keeps the in-app manual complete.
const NAV_IDS = [
  // Converse
  "chat", "inbox", "files", "data", "board", "approvals",
  // Monitor
  "mission", "health", "activity", "autonomy", "alerts", "feed", "insights", "runs", "budget",
  // Agents
  "agents", "roster", "overseer", "council", "toolforge", "mcp", "sandbox", "flow", "replay", "analyst", "search",
  // Automation
  "workflows", "schedules", "standing",
  // Knowledge
  "memory", "world", "skills", "reflect",
  // System
  "overview", "setup", "system", "persona", "prompts", "configcenter", "config",
  "providers", "models", "routing", "tools", "catalog", "policy", "cache", "storage", "backup",
];

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
