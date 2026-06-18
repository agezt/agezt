# PHASE M583 — Web UI end-to-end browser test in CI (Playwright)

**Status:** DONE — local, gated (full e2e ran green locally), ready for branch/PR.
**Scope:** The last clearly-offline-verifiable webui polish item. Vitest logic
(M579) + component/DOM (M581) tests already cover units; this adds a real
**browser** E2E that drives the shipped, `go:embed`-ded SPA against a live daemon.

## What shipped

- **`frontend/e2e/webui.spec.ts`** — one Playwright test that:
  1. loads the Web UI at its tokenized URL (`?token=…`);
  2. asserts the shell (`h1 agezt · console`) + the `● live` SSE indicator;
  3. Overview → `Status` heading + real status keys (`active_runs`, `daemon`);
  4. Runs → the seeded run renders as a card (`completed` · `hello e2e`); expanding
     it shows the run-detail cards — `Final answer` + `[echo] hello e2e` — proving
     the journal → deriveDetail pipeline (M577/M580) end to end in a browser;
  5. World → the React Flow panel mounts (`World` heading);
  6. **zero console errors / pageerrors** throughout — the strict-CSP regression
     guard (M566 shipped `script-src 'self'`), now automated rather than eyeballed.
- **`frontend/playwright.config.ts`** — chromium, `workers: 1`, `retries: 2` on CI,
  reads the full URL from `AGEZT_WEBUI_URL`. Separate from vite/vitest config.
- **`scripts/webui-e2e.sh`** + **`make webui-e2e`** — boots a keyless
  `AGEZT_DEMO_ECHO` daemon with `AGEZT_WEB_ADDR`, seeds one intent (`agt run`),
  greps the tokenized Web UI URL from the daemon log, and runs `npx playwright
  test`. Mirrors `scripts/e2e-smoke.sh` (build/boot/wait-ready/cleanup-trap).
- **`scripts/webui-e2e.ps1`** + **`make webui-e2e-ps`** — Windows/PowerShell
  parity harness for the same gate. Builds temp `agezt.exe`/`agt.exe`, uses
  separate stdout/stderr daemon logs because PowerShell does not allow redirecting
  both streams to one file, resolves the tokenized URL, and runs the same
  Playwright spec against the embedded production SPA.
- **CI `webui-e2e` job** — setup-go + setup-node, build binaries, `npm ci`,
  `npx playwright install --with-deps chromium`, run the harness. New gate.

## Key navigation finding (real bug it would have caught)

The UI holds an open `/events` SSE stream, so `page.goto(..., {waitUntil:
"networkidle"})` **never settles** → 30s timeout. Fixed to `domcontentloaded` +
explicit element waits. (Any future "wait for network idle" on an SSE page is a
latent hang; the spec documents this.)

## dist byte-identity (the M581 trap, handled)

Tailwind v4 auto-scans every source file, so the new e2e spec churned the CSS
bundle (and cascaded the JS hash) on first rebuild — exactly the M581 failure mode.
Fixed with `@source not "../e2e/**"` in `src/index.css`; rebuilt dist verified
**byte-for-byte identical** to the committed bundle (`git status kernel/webui/dist`
clean), so `frontend-dist-in-sync` stays green. Vitest `include` is `src/**/*.test.*`
and tsconfig `include` is `["src", …]`, so the spec is invisible to both. Later
frontend hardening expanded the Vitest suite; use the current `npm test` output
for live counts.

## Gate (all green locally)

- `bash scripts/webui-e2e.sh <agezt> <agt>` → daemon ready, run seeded, URL
  resolved, **Playwright 1 passed**. Full real-browser proof, offline.
- `powershell -ExecutionPolicy Bypass -File scripts/webui-e2e.ps1` → binaries
  built, daemon ready, run seeded, URL resolved, **Playwright 1 passed**,
  `WEBUI-E2E PASS`.
- Re-ran the spec standalone against a hand-booted daemon: pass.
- `npm test` (Vitest) green; `npm run build` clean; embedded dist rebuilt and
  kept in sync with the production SPA bundle.
- Go gates green in the current tree (`go test ./...` plus targeted package
  checks during follow-up hardening).

## Wiring

- `frontend/package.json`: `@playwright/test ^1.60` devDep (3 pkgs, `npm audit`
  0 vulns) + `"test:e2e": "playwright test"`. `package-lock.json` updated.
- `frontend/src/index.css`: `@source not "../e2e/**"`.
- `.gitignore`: `/frontend/test-results/`, `playwright-report/`, `blob-report/`,
  `.playwright/` (the spec in `frontend/e2e/` IS committed; only run output +
  downloaded browsers are ignored).
- `.github/workflows/ci.yml`: `webui-e2e` job. `Makefile`: `webui-e2e` and
  `webui-e2e-ps` targets.
- `CHANGELOG.md` Unreleased entry (M583).

## State after M583

Offline-verifiable feature well is now genuinely dry. Remaining DEFERRED items
(Signal/Tunnels/Marketplace channels, SDK publish to PyPI/npm/crates.io) all touch
external services / secrets / networks → need an explicit owner steer; not
autonomously decidable. `agt migrate` has no real migration → skip (padding).
