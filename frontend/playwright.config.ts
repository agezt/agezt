import { defineConfig, devices } from "@playwright/test";

// End-to-end tests drive the REAL embedded Web UI — the production bundle that
// is `go:embed`-ded into the agezt daemon — in a headless browser. The harness
// (`scripts/webui-e2e.sh`, run by `make webui-e2e` and CI) boots a keyless demo
// daemon and exports `AGEZT_WEBUI_URL`: the full Web UI URL including the
// `?token=…` query the browser authenticates with. The spec reads that env var.
//
// Kept separate from vite/vitest config: Playwright transpiles `e2e/*.spec.ts`
// itself, and Vitest's `include` is `src/**/*.test.*`, so the two suites never
// overlap.
export default defineConfig({
  testDir: "./e2e",
  fullyParallel: false,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 2 : 0,
  workers: 1,
  reporter: "list",
  timeout: 30_000,
  expect: { timeout: 10_000 },
  use: {
    trace: "on-first-retry",
    screenshot: "only-on-failure",
  },
  projects: [{ name: "chromium", use: { ...devices["Desktop Chrome"] } }],
});
