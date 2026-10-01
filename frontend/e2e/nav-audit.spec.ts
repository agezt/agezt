import { test, expect, type ConsoleMessage } from "@playwright/test";

// Comprehensive nav audit — visits every single nav view in the running Web UI
// and asserts the rendered DOM is real content (not the "view was retired"
// placeholder that the Day 23 cleanup briefly littered across 35 nav slots).
//
// This is the runtime counterpart to the static `scripts/dump-nav.cjs` audit.
// Run with `scripts/nav-audit.ps1` (boots a keyless demo daemon on :18799) or
// `make webui-e2e`; consumes `AGEZT_WEBUI_URL` from the harness like the
// other specs.

const URL = process.env.AGEZT_WEBUI_URL;

// The full list of nav views that must render real content. Mirrors the static
// audit in scripts/dump-nav.cjs; keeping it explicit here so a single missed
// row fails loudly in CI.
//
// Day 28 IA pass dropped nine nav rows whose labels misrepresented what their
// render returned (Health→Standing orders, Alerts→Standing orders, Search→Data
// Lake, Storage→Data Lake, Taste→Memory verbatim, Wizards→Schedules,
// Inbox→World graph, Overview→Standing orders, Messages→World graph). Those
// rows are gone from NAV entirely and the legacy ids stay in
// REMOVED_VIEW_IDS. This list therefore covers the surviving views only.
//
// 2026-10-01: three more ids were retired by the same rule — `activity` and
// `replay` (Runs' own tabs, which rendered the Runs component) and `prompts`
// (the Identity row, which rendered Skills). They are in VIEW_ALIASES rather
// than REMOVED_VIEW_IDS, so a deep link to them renders the real Runs/Skills
// page. Dropping them from this list does NOT drop them from coverage: see the
// alias-routing case below.
const VIEWS: { section: string; row: string; id: string }[] = [
  // Talk
  { section: "Talk", row: "Jarvis", id: "jarvis" },
  { section: "Talk", row: "Chat", id: "chat" },
  { section: "Talk", row: "Voice", id: "voice" },
  // Observe (Monitor was renamed from Overview in Day 28)
  { section: "Observe", row: "Monitor", id: "mission" },
  { section: "Observe", row: "Monitor", id: "feed" },
  { section: "Observe", row: "Runs", id: "runs" },
  // Automate
  { section: "Automate", row: "Workflows", id: "workflows" },
  { section: "Automate", row: "Triggers", id: "schedules" },
  { section: "Automate", row: "Triggers", id: "standing" },
  { section: "Automate", row: "Autonomy", id: "autonomy" },
  // Govern
  { section: "Govern", row: "Approvals", id: "approvals" },
  { section: "Govern", row: "Policy", id: "policy" },
  { section: "Govern", row: "Oversight", id: "overseer" },
  { section: "Govern", row: "Oversight", id: "council" },
  // Agents
  { section: "Agents", row: "Agents", id: "agents" },
  { section: "Agents", row: "Roster", id: "roster" },
  { section: "Agents", row: "Skills", id: "skills" },
  { section: "Agents", row: "Capabilities", id: "market" },
  { section: "Agents", row: "Capabilities", id: "execution-profiles" },
  { section: "Agents", row: "Sandbox", id: "sandbox" },
  // Knowledge
  { section: "Knowledge", row: "Memory", id: "memory" },
  { section: "Knowledge", row: "World", id: "world" },
  { section: "Knowledge", row: "Thinking Partners", id: "research" },
  { section: "Knowledge", row: "Thinking Partners", id: "analyst" },
  { section: "Knowledge", row: "Thinking Partners", id: "reflect" },
  { section: "Knowledge", row: "Data & Files", id: "data" },
  { section: "Knowledge", row: "Data & Files", id: "artifacts" },
  // Connect
  { section: "Connect", row: "Providers & Models", id: "models" },
  { section: "Connect", row: "Routing", id: "chains" },
  { section: "Connect", row: "Channels", id: "channels" },
  { section: "Connect", row: "Integrations", id: "mcp" },
  { section: "Connect", row: "Integrations", id: "acp" },
  { section: "Connect", row: "Integrations", id: "connections" },
  // Admin
  { section: "Admin", row: "Setup", id: "setup" },
  { section: "Admin", row: "Config Center", id: "configcenter" },
  { section: "Admin", row: "Backups", id: "backup" },
];

// Retired ids that VIEW_ALIASES routes somewhere real. These are the ones a
// bookmark, a help chip or ⌘K history can still arrive on, so they get the same
// "must render real content" treatment as a live view — with the extra
// assertion that they landed on the surface the alias names, not merely on
// something that happens to have text in it.
const ALIASED: { id: string; resolvesTo: string }[] = [
  { id: "activity", resolvesTo: "runs" },
  { id: "replay", resolvesTo: "runs" },
  { id: "prompts", resolvesTo: "skills" },
];

test.describe("nav audit — every visible view renders real content", () => {
  test("no nav slot renders the 'view was retired' placeholder", async ({ page }) => {
    expect(URL, "AGEZT_WEBUI_URL must be set by the harness").toBeTruthy();

    const errors: string[] = [];
    page.on("console", (m: ConsoleMessage) => {
      if (m.type() === "error") errors.push(`[${m.type()}] ${m.text()}`);
    });
    page.on("pageerror", (e) => errors.push(`[pageerror] ${String(e)}`));

    await page.addInitScript(() => localStorage.setItem("agezt.setup.skipped", "1"));
    await page.goto(URL!, { waitUntil: "domcontentloaded" });

    const nav = page.getByRole("navigation");
    const visited: string[] = [];

    for (const v of VIEWS) {
      // Drive the active view directly by URL hash — that's how deep links
      // and bookmarks arrive, so it's the most honest end-to-end check.
      await page.evaluate((id) => {
        location.hash = `#/${id}`;
      }, v.id);
      // Wait for the lazy chunk to load. RouteLoading shows "Loading X..."
      // until then. We poll the main area's text and treat either "loading"
      // or "real content > 20 chars" as success — and "retired" as a hard
      // failure regardless.
      let lastText = "";
      let lastStatus: "loading" | "ok" | "retired" | "empty" = "loading";
      for (let i = 0; i < 60; i++) {
        lastText = (await page.locator("main").first().textContent())?.trim() ?? "";
        if (/This view was retired/.test(lastText)) {
          lastStatus = "retired";
          break;
        }
        if (/^Loading\b/.test(lastText)) {
          lastStatus = "loading";
        } else if (lastText.length > 20) {
          lastStatus = "ok";
          break;
        } else {
          lastStatus = "empty";
        }
        await page.waitForTimeout(150);
      }
      expect(lastStatus, `${v.id} did not finish loading in 9s (status=${lastStatus}, text="${lastText.slice(0, 80)}")`).toBe("ok");

      visited.push(v.id);
    }

    expect(visited.length).toBe(VIEWS.length);
    expect(errors, `console errors:\n${errors.join("\n")}`).toEqual([]);
  });

  // A retired id that resolves to a real page must land on THAT page. Asserting
  // only "some content appeared" would pass even if the alias silently pointed
  // somewhere else, which is exactly the bug class this exists for.
  test("a retired view id resolves to the surface its alias names", async ({ page }) => {
    expect(URL, "AGEZT_WEBUI_URL must be set by the harness").toBeTruthy();
    await page.addInitScript(() => localStorage.setItem("agezt.setup.skipped", "1"));
    await page.goto(URL!, { waitUntil: "domcontentloaded" });

    for (const a of ALIASED) {
      await page.evaluate((id) => {
        location.hash = `#/${id}`;
      }, a.id);
      // The alias is rewritten in the hash by viewFromHash, so assert on the
      // hash rather than on page text: it says exactly which surface the app
      // decided this id means.
      await page.waitForFunction(
        (target) => location.hash.replace(/^#\/?/, "") === target,
        a.resolvesTo,
        { timeout: 5000 },
      ).catch(() => {
        throw new Error(
          `#/${a.id} did not resolve to its alias target "#/${a.resolvesTo}"; hash is "${await page.evaluate(() => location.hash)}"`,
        );
      });
    }
  });
});
