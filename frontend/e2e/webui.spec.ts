import { test, expect, type ConsoleMessage } from "@playwright/test";

// The full Web UI URL (incl. ?token=…) of a running demo daemon, exported by the
// harness. Failing fast here beats a confusing navigation error.
const URL = process.env.AGEZT_WEBUI_URL;

test.describe("Agezt Web UI — embedded SPA against a real daemon", () => {
  test("loads, navigates core views, renders live data, no console errors", async ({
    page,
  }) => {
    expect(URL, "AGEZT_WEBUI_URL must be set by the harness").toBeTruthy();

    // Strict-CSP regression guard: the M566 rebuild ships under `script-src
    // 'self'`, so any inline-script/eval/asset violation surfaces as a console
    // error. Collect them (and uncaught page errors) and assert none at the end.
    const errors: string[] = [];
    page.on("console", (m: ConsoleMessage) => {
      if (m.type() === "error") errors.push(m.text());
    });
    page.on("pageerror", (e) => errors.push(String(e)));

    // The harness daemon is keyless (echo mock), so the first-run Setup wizard
    // (M816) would auto-open as a full-screen overlay and hide the console
    // shell. Mark it dismissed up front — this spec covers the console, the
    // wizard has its own unit coverage.
    await page.addInitScript(() => localStorage.setItem("agezt.setup.skipped", "1"));

    // NOT `networkidle`: the UI holds an open `/events` SSE stream, so the
    // network is never idle. Wait for the DOM, then assert on elements.
    await page.goto(URL!, { waitUntil: "domcontentloaded" });

    // --- Shell + live SSE indicator -------------------------------------
    // The brand word inside the h1 is a rename button (M719) whose aria-label
    // ("Rename console") replaces it in the heading's accessible *name*, so
    // match on the visible text instead of the computed role name.
    const title = page.locator("h1", { hasText: "· console" });
    await expect(title).toBeVisible();
    await expect(title).toContainText(/agezt/i);
    // The header ConnectionChip (commit 8fca29fa) is a dot+icon indicator:
    // its visible label is screen-reader-only, so we assert on its
    // data-connection-state attribute instead of literal "● live" text.
    // The state flips to "live" once /events connects AND delivers an event,
    // which proves the live SSE stream is wired end to end.
    const conn = page.locator("[data-connection-state]").first();
    await expect(conn).toBeVisible();
    // "live" = connected + receiving events; "stale" = connected but no event
    // yet (or no event for >STALE_MS). Both prove the /events SSE stream
    // connected end to end; only "disconnected" would indicate a real wiring
    // failure. Under contended WSL runners the first event can take longer
    // than the assertion window, so the chip legitimately reads "stale".
    await expect(conn).toHaveAttribute("data-connection-state", /(live|stale)/);

    const nav = page.getByRole("navigation");
    // exact: true so a substring nav label can't hijack the match — e.g. the
    // "ACP Agents" item contains "Agents", which used to make `.last()` open it
    // instead of the roster "Agents" view.
    const openView = async (section: string, item: string) => {
      await nav.getByRole("button", { name: section, exact: true }).first().click();
      await nav.getByRole("button", { name: item, exact: true }).last().click();
    };

    // --- Landing: the humane chat surface --------------------------------
    await expect(
      page.getByRole("heading", { level: 2, name: "Talk to your agent" }),
    ).toBeVisible();

    // --- Dashboard (System → Overview): live status pulled from the daemon ---
    // The seeded run shows up in the completed counter; the vitals strip and
    // widgets are real daemon state, not placeholders.
    await openView("System", "Overview");
    // Generous timeout for the first post-nav heading: a click + React re-mount
    // + initial data fetch under WSL runner load can exceed the default 10s.
    // Same rationale as the data-connection-state live-or-stale tolerance in
    // the connection-state assertion above.
    await expect(
      page.getByRole("heading", { level: 2, name: "Dashboard" }),
    ).toBeVisible({ timeout: 30_000 });
    await expect(page.getByText(/success rate/i)).toBeVisible();
    await expect(page.getByText(/active skills/i)).toBeVisible();

    // Mobile shell regression guard: the top command bar and two-level nav may
    // scroll internally, but they must not create document-level horizontal
    // overflow. A prior header/nav layout leaked ~278px of page overflow on
    // 390px-wide screens.
    await page.setViewportSize({ width: 390, height: 900 });
    await expect.poll(async () =>
      page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth),
    ).toBeLessThanOrEqual(0);
    await page.setViewportSize({ width: 1280, height: 720 });

    // --- Runs: the intent the harness submitted renders as a card -------
    await openView("Monitor", "Runs");
    await expect(
      page.getByRole("heading", { level: 2, name: "Runs" }),
    ).toBeVisible();
    const run = page.getByRole("button", { name: /hello e2e/ });
    await expect(run).toBeVisible();
    await expect(run).toContainText("completed");

    // Expanding the run derives the detail cards (M577/M580) from its journal
    // arc — proving the journal → run-detail pipeline end to end in a browser.
    await run.click();
    await expect(page.getByText("Final answer", { exact: true })).toBeVisible();
    await expect(page.getByText("[echo] hello e2e")).toBeVisible();

    // --- World: the React Flow panel mounts -----------------------------
    await openView("Knowledge", "World");
    await expect(
      page.getByRole("heading", { level: 2, name: "World" }),
    ).toBeVisible();

    // --- Autonomy: the proactive-heartbeat controls render + work -------
    // (M743 pause/resume, M756 beat-now, M757 cadence, M758 dial, M761 flush).
    // Pulse is on by default in the demo daemon, so the steering controls render.
    await openView("Monitor", "Autonomy");
    await expect(page.getByRole("heading", { level: 2, name: "Autonomy" })).toBeVisible();
    await expect(page.getByRole("button", { name: /Beat now/ })).toBeVisible();
    await expect(page.getByLabel("Heartbeat cadence")).toBeVisible();
    await expect(page.getByLabel("Proactivity dial")).toBeVisible();
    // "Beat now" drives the on-demand-heartbeat route end to end (the zero-console-
    // errors guard below also covers it).
    await page.getByRole("button", { name: /Beat now/ }).click();

    // --- Schedules: typed cronjobs, including daemon system tasks --------
    // The schedule surface must be more than "run this prompt later": it can
    // schedule typed daemon work such as syncing models.dev/api.json with no
    // LLM agent wake.
    await openView("Automation", "Schedules");
    await expect(page.getByRole("heading", { level: 2, name: "Schedules" })).toBeVisible();
    await page.getByRole("button", { name: /New schedule/ }).click();
    await expect(page.getByText("Daemon cron presets")).toBeVisible();
    await page.getByRole("button", { name: /Catalog sync.*every 24 hours/ }).click();
    // "System task" is a ScheduleChoicePicker (role=group of aria-pressed
    // buttons), not a <select> — assert the Catalog sync choice is pressed.
    await expect(
      page.getByRole("group", { name: "System task", exact: true }).getByRole("button", { name: /Catalog sync/ }),
    ).toHaveAttribute("aria-pressed", "true");
    await expect(page.getByText(/Recommended cadence: every 24 hours/)).toBeVisible();
    await expect(page.getByText(/cron runs system task Catalog sync/).first()).toBeVisible();
    await expect(page.getByText(/no LLM/).first()).toBeVisible();
    // Close the New-schedule modal before navigating on — its overlay would
    // otherwise swallow the nav clicks.
    await page.getByRole("button", { name: "Close schedule modal" }).click();

    // --- Policy: the decision + secret-redaction testers mount ----------
    // (M753 policy dry-run, M754 redaction check).
    await openView("System", "Policy");
    await expect(page.getByRole("heading", { level: 2, name: "Capability policy" })).toBeVisible();
    // The dry-run testers live behind compact affordances now: a "Test
    // decision" button in the capabilities panel and a "Secret redaction" card.
    await expect(page.getByRole("button", { name: /Test decision/ })).toBeVisible();
    await expect(page.getByRole("heading", { name: "Secret redaction" }).first()).toBeVisible();

    // --- Search: the journal's tamper-evident hash chain verifies clean -
    // (M759 integrity verify). The seeded run wrote hash-linked events.
    await openView("Agents", "Search");
    const verify = page.getByRole("button", { name: /verify integrity/ });
    await expect(verify).toBeVisible();
    await verify.click();
    await expect(page.getByText("chain intact")).toBeVisible();

    // --- Fleet: roster agents open as identity-bearing entities ----------
    // Schedules/standing/workflows can trigger work, but roster agents must be
    // durable objects with their own identity, control, lifecycle and runtime
    // surfaces. This clicks a real daemon-backed fleet card and proves those
    // panels mount in the browser.
    await openView("Agents", "Agents");
    await expect(page.getByRole("heading", { level: 2, name: "Agents" })).toBeVisible();
    const agentCard = page.getByRole("button", { name: /Guardian · Health[\s\S]*guardian-health/ });
    await expect(agentCard).toBeVisible();
    await agentCard.click();
    await expect(page.getByText("Agent identity card")).toBeVisible();
    await expect(page.getByText("Live presence").first()).toBeVisible();
    await expect(page.getByText("Lifecycle ledger").first()).toBeVisible();
    await expect(page.getByText("Runtime doctor ledger").first()).toBeVisible();
    await expect(page.getByText("Operations passport")).toBeVisible();
    await expect(page.getByText("Mailbox wake contract")).toBeVisible();
    await page.getByRole("button", { name: /Triggers/ }).click();
    await expect(page.getByText("mailbox wake subjects")).toBeVisible();
    await expect(page.getByText(/board\.dm\./).first()).toBeVisible();
    await expect(page.getByText(/board\.help\./).first()).toBeVisible();
    await expect(page.getByText("board.broadcast").first()).toBeVisible();

    expect(errors, `console errors:\n${errors.join("\n")}`).toEqual([]);
  });
});
