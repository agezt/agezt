import { test, expect, type ConsoleMessage } from "@playwright/test";

// The full Web UI URL (incl. ?token=…) of a running demo daemon, exported by the
// harness. Failing fast here beats a confusing navigation error.
const URL = process.env.AGEZT_WEBUI_URL;

test.describe("Agezt Web UI — embedded SPA against a real daemon", () => {
  test("shows truthful Jarvis and Voice readiness when speech providers are absent", async ({ page }) => {
    expect(URL, "AGEZT_WEBUI_URL must be set by the harness").toBeTruthy();
    const errors: string[] = [];
    page.on("console", (m: ConsoleMessage) => {
      if (m.type() === "error") errors.push(m.text());
    });
    page.on("pageerror", (e) => errors.push(String(e)));
    await page.addInitScript(() => localStorage.setItem("agezt.setup.skipped", "1"));
    await page.goto(URL!, { waitUntil: "domcontentloaded" });

    const nav = page.getByRole("navigation");
    await nav.getByRole("button", { name: "Talk", exact: true }).first().click();
    await nav.getByRole("button", { name: "Jarvis", exact: true }).last().click();
    await expect(page.getByRole("heading", { level: 2, name: "Jarvis" })).toBeVisible();
    await expect(page.getByText("Voice needs setup").first()).toBeVisible();
    await expect(page.getByText("provider not configured").first()).toBeVisible();

    await nav.getByRole("button", { name: "Voice", exact: true }).last().click();
    await expect(page.getByRole("heading", { level: 2, name: "Voice" })).toBeVisible();
    await expect(page.getByText("Hearing: setup required")).toBeVisible();
    const setup = page.getByRole("button", { name: "Set up hearing" });
    await expect(setup).toBeVisible();
    await setup.click();
    await expect(page.getByRole("heading", { level: 4, name: "Hearing" })).toBeVisible();
    await expect(page.getByRole("heading", { level: 4, name: "Voice" })).toBeVisible();
    await expect(page.getByText("0/2 ready")).toBeVisible();

    expect(errors, `console errors:\n${errors.join("\n")}`).toEqual([]);
  });

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
    // The cockpit is organized by operator jobs, not internal subsystems.
    // All eight destinations must remain visible and keyboard-addressable.
    // Pinned here AND in views.spec.ts; src/e2enav.test.ts holds both to nav.tsx.
    for (const job of ["Talk", "Observe", "Automate", "Govern", "Agents", "Knowledge", "Connect", "Admin"]) {
      await expect(nav.getByRole("button", { name: job, exact: true }).first()).toBeVisible();
    }
    // exact: true so a substring nav label can't hijack the match — e.g. the
    // "ACP Agents" item contains "Agents", which used to make `.last()` open it
    // instead of the roster "Agents" view.
    // The sidebar lists DESTINATIONS; a destination with several facets renders
    // a tab strip inside <main>, not in <nav>. Pass `row` when the view you want
    // is one of those facets — src/e2enav.test.ts checks every triple below
    // still exists in nav.tsx.
    const openView = async (section: string, item: string, row?: string) => {
      await nav.getByRole("button", { name: section, exact: true }).first().click();
      await nav.getByRole("button", { name: row ?? item, exact: true }).last().click();
      if (row) await page.getByRole("tab", { name: item, exact: true }).click();
    };

    // --- Chat: the humane chat surface is part of the nav, not the landing
    // (landing is Observe › Overview — Dashboard — since the 2026-09 IA pass;
    // the chat quick opener lives in MiniChat and the Cmd+K "New chat"
    // shortcut). Navigate to it explicitly so the lazy chunk for the legacy
    // Chat panel loads before we assert the EmptyState h2.
    await openView("Talk", "Chat");
    // Generous timeout: clicking the row + lazy-loading the Chat chunk + the
    // chat engine's first render + InitialMessage fetch can exceed the
    // default 10s under load. Same rationale as the 30s Overview timeout
    // above.
    await expect(
      page.getByRole("heading", { level: 2, name: "Talk to your agent" }),
    ).toBeVisible({ timeout: 30_000 });

    // --- Standing orders (Automate › Triggers › Standing orders): the operator's
    // "what fires autonomously right now" view. Day 28 retired the misleading
    // "Overview" first tab from the Observe section (it had been aliased to
    // the same Standing surface), so this navigates the unambiguous Triggers
    // path that surfaces the same real backend data without promising a
    // Dashboard that no longer exists.
    await openView("Automate", "Standing orders", "Triggers");
    // Generous timeout: clicking + lazy-loaded chunk + journal-fetch under
    // WSL runner load can exceed the default 10s.
    await expect(
      page.getByRole("heading", { level: 2, name: "Standing orders" }),
    ).toBeVisible({ timeout: 30_000 });
    // Real daemon state — the seeded fleet gives us the pulse.observer reaper
    // guardian + budget + routing guardians out of the box.
    await expect(page.getByText(/wake rules/i)).toBeVisible();
    await expect(
      page.getByText(/Guardian · (Doctor|Health|Budget|Routing|Stuck)/i).first(),
    ).toBeVisible();

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
    await openView("Observe", "Runs");
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
    await openView("Automate", "Autonomy");
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
    await openView("Automate", "Schedules", "Triggers");
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
    // Declutter law: the prose execution-contract block is gone; the form shows
    // one cadence confirmation line and the typed task's executor facts instead.
    await expect(page.getByText(/every 24 hours/).first()).toBeVisible();
    await expect(page.getByText(/no LLM/).first()).toBeVisible();
    // Close the New-schedule modal before navigating on — its overlay would
    // otherwise swallow the nav clicks.
    await page.getByRole("button", { name: "Close schedule modal" }).click();

    // --- Policy: the decision + secret-redaction testers mount ----------
    // (M753 policy dry-run, M754 redaction check).
    await openView("Govern", "Policy");
    await expect(page.getByRole("heading", { level: 2, name: "Capability policy" })).toBeVisible();
    // The dry-run testers live behind compact affordances now: a "Test
    // decision" button in the capabilities panel and a "Secret redaction" card.
    await expect(page.getByRole("button", { name: /Test decision/ })).toBeVisible();
    await expect(page.getByRole("heading", { name: "Secret redaction" }).first()).toBeVisible();

    // --- Search: the journal-verify affordance retired from the UI in Day 28.
    // The "Search" nav row used to alias to Data Lake (SearchView = Data in
    // nav.tsx); Day 28 dropped both the row and its alias entirely rather than
    // keep misleading the operator that "#search" goes anywhere near the
    // journal. Tamper-evidence runs every minute on the daemon and is covered
    // by kernel/journal_verify_test.go — the cron is the auditor, the e2e is
    // not. We navigate directly to a different Knowledge surface that has
    // real data so the breadth walk stays loud on broken consoles.
    await openView("Knowledge", "World");

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
    // Declutter law: the header is a glance layer of MetricWidgets, the six
    // grouped tabs carry everything else (no passport/ledger prose cards).
    await expect(page.getByText("Presence").first()).toBeVisible();
    await expect(page.getByText("Next wake").first()).toBeVisible();
    await expect(page.getByText("Spend today").first()).toBeVisible();
    await expect(page.getByText("How does this run?")).toBeVisible();
    await expect(page.getByText("Operations passport")).toHaveCount(0);
    await page.getByRole("button", { name: /Wiring/ }).click();
    await expect(page.getByText("mailbox wake subjects")).toBeVisible();
    await expect(page.getByText(/board\.dm\./).first()).toBeVisible();
    await expect(page.getByText(/board\.help\./).first()).toBeVisible();
    await expect(page.getByText("board.broadcast").first()).toBeVisible();

    expect(errors, `console errors:\n${errors.join("\n")}`).toEqual([]);
  });
});
