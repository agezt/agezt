import { test, expect, type ConsoleMessage } from "@playwright/test";

// Per-view screenshot harness. Drives every nav view and saves a PNG to
// .tmp/nav-screenshots/<id>.png so the operator can flip through the file
// list and verify each page renders real content with their own eyes. The
// companion test also asserts there are NO "This view was retired" placeholders.
//
// Run with: scripts/nav-screenshots.ps1 (boots daemon on :18800).

const URL = process.env.AGEZT_WEBUI_URL;

// Day 28 IA pass dropped nine nav rows whose labels misrepresented what their
// render returned (Health→Standing orders, Alerts→Standing orders, Search→Data
// Lake, Storage→Data Lake, Taste→Memory verbatim, Wizards→Schedules,
// Inbox→World graph, Overview→Standing orders, Messages→World graph). Those
// rows are gone from NAV entirely and the legacy ids stay in REMOVED_VIEW_IDS.
// This list therefore covers the surviving 37 view ids; Fleet is renamed to
// Agents. Run scripts/dump-nav.cjs for the static counterpart.
const VIEWS = [
  // Talk
  { id: "jarvis", section: "Talk" },
  { id: "chat", section: "Talk" },
  { id: "voice", section: "Talk" },
  // Observe
  { id: "mission", section: "Observe" },
  { id: "feed", section: "Observe" },
  { id: "runs", section: "Observe" },
  { id: "activity", section: "Observe" },
  { id: "replay", section: "Observe" },
  // Automate
  { id: "workflows", section: "Automate" },
  { id: "schedules", section: "Automate" },
  { id: "standing", section: "Automate" },
  { id: "autonomy", section: "Automate" },
  // Govern
  { id: "approvals", section: "Govern" },
  { id: "policy", section: "Govern" },
  { id: "overseer", section: "Govern" },
  { id: "council", section: "Govern" },
  // Agents (Fleet renamed)
  { id: "agents", section: "Agents" },
  { id: "roster", section: "Agents" },
  { id: "skills", section: "Agents" },
  { id: "market", section: "Agents" },
  { id: "execution-profiles", section: "Agents" },
  { id: "sandbox", section: "Agents" },
  // Knowledge
  { id: "memory", section: "Knowledge" },
  { id: "world", section: "Knowledge" },
  { id: "research", section: "Knowledge" },
  { id: "analyst", section: "Knowledge" },
  { id: "reflect", section: "Knowledge" },
  { id: "data", section: "Knowledge" },
  { id: "artifacts", section: "Knowledge" },
  // Connect
  { id: "models", section: "Connect" },
  { id: "chains", section: "Connect" },
  { id: "channels", section: "Connect" },
  { id: "mcp", section: "Connect" },
  { id: "acp", section: "Connect" },
  { id: "connections", section: "Connect" },
  // Admin
  { id: "setup", section: "Admin" },
  { id: "configcenter", section: "Admin" },
  { id: "prompts", section: "Admin" },
  { id: "backup", section: "Admin" },
];

test.describe("nav screenshots", () => {
  test("capture a PNG for every visible view", async ({ page }, info) => {
    expect(URL, "AGEZT_WEBUI_URL must be set").toBeTruthy();
    const errors: string[] = [];
    page.on("console", (m: ConsoleMessage) => {
      if (m.type() === "error") errors.push(m.text());
    });
    page.on("pageerror", (e) => errors.push(String(e)));
    await page.addInitScript(() => localStorage.setItem("agezt.setup.skipped", "1"));
    await page.goto(URL!, { waitUntil: "domcontentloaded" });

    const outDir = info.outputPath("nav-screenshots");
    // Per-view selector that proves the real content has rendered, not just
    // that "Loading X..." has gone away. For new views (research/analyst/
    // reflect/backup) the lazy chunk may take a beat longer than the generic
    // "text > 20 chars" heuristic gives it, so each view carries a marker we
    // can wait on explicitly.
    const CONTENT_MARKER: Record<string, string> = {
      research: 'textarea[placeholder*="safest way to migrate"]',
      analyst: 'select',
      reflect: 'h2:has-text("Reflect")',
      backup: 'button:has-text("Refresh")',
      memory: 'button:has-text("Teach"), h3:has-text("No memories yet")',
    };
    // Forbid the placeholder everywhere.
    for (const v of VIEWS) {
      await page.evaluate((id) => {
        location.hash = `#/${id}`;
      }, v.id);
      // Wait for lazy chunk + content. The main element includes the ViewTabs
      // strip AND the lazy-loaded body, so a "Loading..." body still produces
      // a non-empty text. Treat the page as ready only when:
      //   - the per-view marker is present in main, OR
      //   - text does NOT contain "Loading..." anywhere (not just at the start)
      //     and the body text exceeds 80 chars (real content is long; "Loading
      //     Research..." is short).
      let ok = false;
      const marker = CONTENT_MARKER[v.id];
      for (let i = 0; i < 80; i++) {
        const text = (await page.locator("main").first().textContent())?.trim() ?? "";
        if (/This view was retired/.test(text)) break;
        let markerHit = false;
        if (marker) {
          const count = await page.locator(`main ${marker}`).count();
          markerHit = count > 0;
        }
        const hasLoading = /Loading [^.]{0,30}\.\.\./.test(text);
        if (markerHit || (!hasLoading && text.length > 80)) {
          ok = true;
          break;
        }
        await page.waitForTimeout(150);
      }
      expect(ok, `${v.id} did not render real content`).toBe(true);

      // Take a screenshot.
      const filename = outDir.replace(/\\/g, "/").split("/").slice(-2, -1)[0];
      await page.screenshot({
        path: `${outDir}/${v.id}.png`,
        fullPage: false,
      });
      // Stash a per-view log line.
      console.log(`[screenshot] ${v.section.padEnd(11)} › ${v.id.padEnd(22)} → ${outDir.split(/[/\\]/).pop()}/${v.id}.png`);
    }
    expect(errors, `console errors:\n${errors.join("\n")}`).toEqual([]);
  });
});
