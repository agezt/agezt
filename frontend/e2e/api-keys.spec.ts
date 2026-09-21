import { test, expect, type ConsoleMessage } from "@playwright/test";

// Focused smoke test for the API-key entry pages refactored onto the shared
// `@/features/api-keys` primitives (ApiKeyField, KeyListItem,
// ChatGPTSignInCard, useApiKeySubmit). Mounting each surface end-to-end on a
// real daemon is the only proof that the new components don't throw at
// runtime — vitest/jsdom coverage catches the unit behaviour, but only a
// browser can catch lazy-load + React-Router + real SSE wiring bugs.
//
// Run with `scripts/webui-smoke.ps1` (boots a keyless demo daemon on :18789)
// — like the other specs in this directory, it consumes `AGEZT_WEBUI_URL`
// from the harness.

const URL = process.env.AGEZT_WEBUI_URL;

test.describe("API key pages — shared primitives smoke test", () => {
  test("Models & Keys, Connections, Channels, Voice, Setup all mount cleanly", async ({ page }) => {
    expect(URL, "AGEZT_WEBUI_URL must be set by the harness").toBeTruthy();

    const errors: string[] = [];
    page.on("console", (m: ConsoleMessage) => {
      if (m.type() === "error") errors.push(m.text());
    });
    page.on("pageerror", (e) => errors.push(String(e)));

    // Skip the first-run Setup wizard overlay — the same trick the other specs
    // use. The wizard has its own unit coverage.
    await page.addInitScript(() => localStorage.setItem("agezt.setup.skipped", "1"));
    await page.goto(URL!, { waitUntil: "domcontentloaded" });

    const nav = page.getByRole("navigation");
    // Nav has two levels for some destinations: a `row` (visible label) that
    // opens a tab strip, then a `tab` inside that strip. The webui.spec.ts
    // helper signature is section → row? → item; mirror it here so failures
    // point to the actual missing affordance instead of an unrelated click.
    //
    // A `row` with only ONE child (Models & Keys is the cleanest example —
    // "Providers & Models" opens "models" directly) renders NO tab strip:
    // ViewTabs returns null for a single-view destination, so calling
    // `getByRole("tab", ...)` would time out. Probe for the tab strip before
    // clicking — the destination click already lands the user on the right view.
    const openView = async (section: string, item: string, row?: string) => {
      await nav.getByRole("button", { name: section, exact: true }).first().click();
      await nav.getByRole("button", { name: row ?? item, exact: true }).last().click();
      if (row) {
        const tab = page.getByRole("tab", { name: item, exact: true });
        if ((await tab.count()) > 0 && (await tab.isVisible().catch(() => false))) {
          await tab.click();
        }
      }
    };

    // --- Models & Keys (Connect › Providers & Models › Models & Keys) ----
    await openView("Connect", "Models & Keys", "Providers & Models");
    await expect(
      page.getByRole("heading", { level: 2, name: /Models & Keys/i }),
    ).toBeVisible({ timeout: 30_000 });
    // ChatGPTSignInCard renders the "Sign in with ChatGPT" CTA — proves the
    // shared OAuth card mounts rather than the old bespoke inline flow.
    await expect(page.getByText(/sign in with chatgpt/i).first()).toBeVisible();

    // --- Connections (Connect › Integrations › Connections) --------------
    await openView("Connect", "Connections", "Integrations");
    await expect(
      page.getByRole("heading", { level: 2, name: /Connections/i }),
    ).toBeVisible({ timeout: 30_000 });

    // --- Channels (Connect › Channels, direct row) -----------------------
    await openView("Connect", "Channels");
    await expect(
      page.getByRole("heading", { level: 2, name: /Channels/i }),
    ).toBeVisible({ timeout: 30_000 });

    // --- Voice (Talk › Voice, direct row — STT/TTS key uses ApiKeyField) -
    await openView("Talk", "Voice");
    await expect(
      page.getByRole("heading", { level: 2, name: /Voice/i }),
    ).toBeVisible({ timeout: 30_000 });
    // Open the hearing setup to surface the ApiKeyField under VoiceSetup.
    const hearing = page.getByRole("button", { name: /Set up hearing/i });
    if (await hearing.isVisible().catch(() => false)) {
      await hearing.click();
      await expect(page.getByRole("heading", { level: 4, name: /Hearing/i })).toBeVisible();
      // The Voice setup modal is a fixed-overlay (z-50) that swallows subsequent
      // nav clicks. Close it explicitly before moving on, mirroring how the
      // schedule-modal close pattern works in webui.spec.ts.
      await page.getByRole("button", { name: "Close Voice setup" }).click();
    }

    // --- Setup wizard (Admin › Setup, direct row — provider + telegram --
    //     steps now use ApiKeyField; ChatGPTSignInCard sits at the top of
    //     the provider step rather than being embedded in the wizard) ----
    await openView("Admin", "Setup");
    await expect(
      page.getByRole("heading", { level: 2, name: /Setup/i }),
    ).toBeVisible({ timeout: 30_000 });

    expect(errors, `console errors:\n${errors.join("\n")}`).toEqual([]);
  });
});
