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
    // "● live" renders in the header AND in the Cockpit panel — either proves
    // the SSE stream connected.
    await expect(page.getByText("● live").first()).toBeVisible();

    const nav = page.getByRole("navigation");

    // --- Landing: the humane chat surface --------------------------------
    await expect(
      page.getByRole("heading", { level: 2, name: "Talk to your agent" }),
    ).toBeVisible();

    // --- Cockpit (nav "Overview"): live status pulled from the daemon ---
    // The seeded run shows up in the completed counter; the vitals strip and
    // widgets are real daemon state, not placeholders.
    await nav.getByRole("button", { name: "Overview" }).click();
    await expect(
      page.getByRole("heading", { level: 2, name: "Cockpit" }),
    ).toBeVisible();
    await expect(page.getByText(/success rate/)).toBeVisible();
    await expect(page.getByText(/active skills/)).toBeVisible();

    // --- Runs: the intent the harness submitted renders as a card -------
    await nav.getByRole("button", { name: "Runs" }).click();
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
    await nav.getByRole("button", { name: "World" }).click();
    await expect(
      page.getByRole("heading", { level: 2, name: "World" }),
    ).toBeVisible();

    // --- Autonomy: the proactive-heartbeat controls render + work -------
    // (M743 pause/resume, M756 beat-now, M757 cadence, M758 dial, M761 flush).
    // Pulse is on by default in the demo daemon, so the steering controls render.
    await nav.getByRole("button", { name: "Autonomy" }).click();
    await expect(page.getByRole("heading", { level: 2, name: "Autonomy" })).toBeVisible();
    await expect(page.getByRole("button", { name: /Beat now/ })).toBeVisible();
    await expect(page.getByLabel("Heartbeat cadence")).toBeVisible();
    await expect(page.getByLabel("Proactivity dial")).toBeVisible();
    // "Beat now" drives the on-demand-heartbeat route end to end (the zero-console-
    // errors guard below also covers it).
    await page.getByRole("button", { name: /Beat now/ }).click();

    // --- Policy: the decision + secret-redaction testers mount ----------
    // (M753 policy dry-run, M754 redaction check).
    await nav.getByRole("button", { name: "Policy" }).click();
    await expect(page.getByRole("heading", { level: 2, name: "Capability policy" })).toBeVisible();
    await expect(page.getByText("test a decision")).toBeVisible();
    await expect(page.getByRole("heading", { level: 2, name: "Secret redaction" })).toBeVisible();

    // --- Search: the journal's tamper-evident hash chain verifies clean -
    // (M759 integrity verify). The seeded run wrote hash-linked events.
    await nav.getByRole("button", { name: "Search" }).click();
    const verify = page.getByRole("button", { name: /verify integrity/ });
    await expect(verify).toBeVisible();
    await verify.click();
    await expect(page.getByText("chain intact")).toBeVisible();

    expect(errors, `console errors:\n${errors.join("\n")}`).toEqual([]);
  });
});
