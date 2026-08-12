import { test, expect, type ConsoleMessage, type Page } from "@playwright/test";

// Breadth companion to webui.spec.ts. That spec goes DEEP on ~10 views, driving
// real daemon behaviour (journal verify, beat-now, run detail). This one goes
// WIDE: it visits every view the nav offers and asserts only that the view
// actually rendered something and logged no errors.
//
// Why this exists. During the M981/M982 design sweeps an uncontrolled
// `ui/tab-nav.tsx` ended up selecting no tab, so Dashboard, Runs and Status
// Overview rendered a BLANK panel in the shipped app. Every unit test passed —
// the components were fine in isolation, and the stale view tests had drifted
// with the redesign. Only mounting each view against a real daemon in a real
// browser catches that class.
//
// The view list is DERIVED FROM THE DOM, never hardcoded: the spec reads the
// nav's own group and item buttons. A view added to `src/nav.tsx` is covered
// the day it lands, with no list to update — the same drift-alarm posture the
// Go side uses for channel factories and boot tools.

const URL = process.env.AGEZT_WEBUI_URL;

// Views deliberately skipped, each for a stated reason. Keep this list short:
// every entry is a view nothing checks.
const SKIP = new Set<string>([
  // Flow Studio mounts React Flow on a full-bleed canvas with its own async
  // layout pass; webui.spec.ts covers the workflow surface through Workflows.
  "Flow Studio",
]);

// The eight section-rail groups. Pinned rather than derived: the rail and the
// item list are both plain buttons inside the same <nav>, so there is no
// accessible distinction to filter on — and webui.spec.ts already asserts this
// exact set is present, so a group rename fails there first with a clearer
// message. ITEMS are still read from the DOM, which is where views actually get
// added.
const GROUPS = [
  "Talk",
  "Observe",
  "Automate",
  "Govern",
  "Knowledge",
  "Connect",
  "Build",
  "Admin",
] as const;

/** Item labels of the currently-open section: every nav button that is not a group. */
async function sectionItems(page: Page): Promise<string[]> {
  const labels = await page.getByRole("navigation").getByRole("button").allInnerTexts();
  return labels
    .map((t) => t.trim().split("\n")[0].trim())
    .filter((label) => label && !GROUPS.includes(label as (typeof GROUPS)[number]));
}

/** True once <main> holds real rendered content rather than an empty panel. */
async function mainRendered(page: Page): Promise<boolean> {
  return page
    .waitForFunction(
      () => {
        const el = document.querySelector("main");
        if (!el) return false;
        return (el.textContent ?? "").trim().length > 0 && el.querySelectorAll("*").length > 3;
      },
      undefined,
      { timeout: 15_000 },
    )
    .then(() => true)
    .catch(() => false);
}

test.describe("Agezt Web UI — every nav view mounts against a real daemon", () => {
  // Mounting ~55 views, each a click + React re-mount + first data fetch, is
  // well past the default per-test budget on a contended runner.
  test.setTimeout(10 * 60_000);

  test("no view renders blank and none logs a console error", async ({ page }) => {
    expect(URL, "AGEZT_WEBUI_URL must be set by the harness").toBeTruthy();

    // Errors are attributed to the view being visited, so a failure names the
    // culprit instead of dumping one undifferentiated pile at the end.
    let current = "(shell)";
    const errors: string[] = [];
    const note = (text: string) => errors.push(`[${current}] ${text}`);
    page.on("console", (m: ConsoleMessage) => {
      if (m.type() === "error") note(m.text());
    });
    page.on("pageerror", (e) => note(String(e)));

    // Same first-run handling as webui.spec.ts: the keyless demo daemon would
    // otherwise open the Setup wizard as a full-screen overlay.
    await page.addInitScript(() => localStorage.setItem("agezt.setup.skipped", "1"));
    // NOT networkidle — the console holds an open /events SSE stream.
    await page.goto(URL!, { waitUntil: "domcontentloaded" });

    const nav = page.getByRole("navigation");
    await expect(page.locator("h1", { hasText: "· console" })).toBeVisible();

    const visited: string[] = [];
    const blank: string[] = [];

    for (const group of GROUPS) {
      await nav.getByRole("button", { name: group, exact: true }).first().click();

      // Read AFTER opening the group, so the item list reflects this group
      // only. Re-read per group rather than caching: the nav re-renders and
      // stale handles would go detached.
      const items = await sectionItems(page);
      // A group whose item list came back empty means the reading logic broke,
      // not that the section is empty — every group ships views.
      expect(items.length, `no nav items found under ${group}`).toBeGreaterThan(0);

      for (const item of items) {
        if (SKIP.has(item)) continue;
        current = `${group} › ${item}`;

        // exact: true and .last() mirror webui.spec.ts's openView — a substring
        // label ("Agents" inside "ACP Agents") must not hijack the match.
        const button = nav.getByRole("button", { name: item, exact: true }).last();
        if (!(await button.isVisible().catch(() => false))) continue;
        await button.click();

        // Deliberately weak on CONTENT, strong on EMPTINESS. Views legitimately
        // differ — most use the Page scaffold with an h2, but Chat, Jarvis and
        // a few others are intentionally headerless — so demanding a heading
        // would fail on correct views. What no correct view does is render an
        // empty main panel.
        if (!(await mainRendered(page))) blank.push(current);
        visited.push(current);
      }
    }

    current = "(summary)";
    // Report BOTH failure modes together — fixing a blank view and then
    // discovering an unrelated console error on the next run wastes a cycle.
    expect(blank, `views that rendered an empty main panel:\n${blank.join("\n")}`).toEqual([]);
    expect(errors, `console errors:\n${errors.join("\n")}`).toEqual([]);
    // Printed so a passing run still reports its own breadth — a spec that
    // silently stops covering things is worse than one that fails.
    console.log(`views mounted: ${visited.length}\n  ${visited.join("\n  ")}`);
    // A silent drop from the full nav to a handful would mean the nav-reading
    // logic broke, not that the app shrank. This is the vacuity guard.
    expect(visited.length, `views visited: ${visited.join(", ")}`).toBeGreaterThanOrEqual(40);
  });
});
