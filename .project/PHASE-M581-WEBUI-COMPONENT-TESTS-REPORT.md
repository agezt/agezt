# Phase M581 — Web UI component/DOM tests (jsdom + Testing Library)

**Type:** Feature (test infrastructure; M579 follow-up)
**Date:** 2026-06-08
**Branch:** `feat-webui-component-tests`

## Goal

M579 added Vitest for pure logic; rendering was still only manually smoke-tested.
Add component/DOM tests (`@testing-library/react` + `jsdom`) so presentational
components are exercised against a real DOM in CI — without slowing the
fast pure-logic tests.

## What shipped

### Component tests (6, jsdom)
- `src/components/ui/badge.test.tsx` — `Badge` renders children + applies the
  variant colour class; `statusVariant` maps run/plan statuses to variants.
- `src/components/JsonView.test.tsx` — `JsonView` pretty-prints into a `<pre>`;
  `KeyValue` renders a `<dt>/<dd>` per pair; `Muted`/`ErrorText` render children
  with the right tone class.

Each file opts into jsdom per-file via `// @vitest-environment jsdom`, so the
node-environment logic tests (M579/M580) stay fast. `afterEach(cleanup)` unmounts
between cases.

### Tooling
- `frontend/package.json`: `jsdom`, `@testing-library/react`, `@testing-library/dom`
  devDependencies (`npm audit` → 0 vulnerabilities).
- `frontend/vitest.config.ts`: `include` widened to `src/**/*.test.{ts,tsx}`.
- `frontend/src/index.css`: **`@source not "./**/*.test.{ts,tsx}"`** — excludes
  test files from Tailwind v4's automatic content scan, so class strings that
  appear only in test assertions don't leak into (or churn) the shipped CSS. With
  this, the committed `kernel/webui/dist` is **unchanged** by this PR.

## Verification

- **Vitest:** `npm test` → 5 files, **28 tests** (22 logic + 6 component) pass.
- **Build:** `npm run build` → `tsc --noEmit` (type-checks the .tsx tests) clean +
  Vite emitted dist **byte-identical** to the committed bundle (`git status` shows
  no `dist/` change — the `@source not` exclusion confirmed: a build without it
  churned the CSS hash; with it, the hash reverts).
- `npm audit` → 0 vulnerabilities. No Go source changed (`go.mod`/`go.sum`
  unchanged); the test tooling never enters the `go:embed`-ded `dist`.
- The new `frontend-test` CI job (M579) runs these automatically.

## Counts

- Go packages/tests unchanged (80 / 2473). Web UI unit tests 22 → **28**.

## Out of scope (documented follow-ups)

- Tests for the data-fetching views (would need to mock `fetch`/`EventSource`).
- Playwright E2E in CI (heavier; the manual browser smoke covers full-page flows).
