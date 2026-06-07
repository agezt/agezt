# Phase M579 — Web UI unit tests (Vitest) + CI gate

**Type:** Feature (test infrastructure; closes the standing front-end test gap)
**Date:** 2026-06-08
**Branch:** `feat-webui-vitest`

## Goal

The React Web UI had grown to 17 views plus real derivation logic (M577's
`deriveDetail`) with **zero automated tests** — every change was verified only by
manual browser smoke. Add a Vitest unit suite for the pure logic and a CI gate so
front-end regressions are caught automatically, offline, on every push.

## What shipped

### Extracted pure logic — `frontend/src/lib/rundetail.ts` (new)
`deriveDetail` (folds a run's journaled event arc into the summary + tool-call
breakdown) and `num` were inlined in `Runs.tsx`; moved to a pure, React-free
module so they can be unit-tested directly. `Runs.tsx` now imports them — a
bundle-neutral refactor (the committed `kernel/webui/dist` is **unchanged**, so
`frontend-dist-in-sync` stays green without touching the embed).

### Tests (19, Vitest)
- `lib/rundetail.test.ts` (11): folds an out-of-order arc into the right summary
  (status/model/iterations/answer); sums budget tokens + cost and sets
  `hasBudget`; groups tool calls by `call_id` with capability + verdict
  (result-wins); `hasBudget=false` when no budget event; denied/hard-denied
  capture; failed-run error→answer; empty arc; `num` coercion/guards.
- `lib/format.test.ts` (5): `money` (microcents→$), `pct` (rate→%, em-dash on
  zero denom), `byDescValue`.
- `lib/utils.test.ts` (3): `clip`, `prettyJSON`, `fmtTime`.

### Tooling + CI
- `frontend/package.json`: `vitest@^4` devDependency (v4 — the v3 advisory
  GHSA-5xrq-8626-4rwp only affects the unused `--ui` server; v4 audits clean,
  `npm audit` → 0 vulnerabilities) + `"test": "vitest run"`.
- `frontend/vitest.config.ts`: node environment, `@` alias, `src/**/*.test.ts` —
  kept separate from `vite.config.ts` so tests don't pull in the React/Tailwind
  build plugins.
- `.github/workflows/ci.yml`: new **`frontend-test`** job (ubuntu, `npm ci &&
  npm test`) — the 19th CI check.
- `Makefile`: `make frontend-test`.

## Verification

- `cd frontend && npm test` → **3 files, 19 tests passed**.
- `cd frontend && npm run build` → `tsc --noEmit` (now type-checks the test files
  too) clean + Vite emitted dist **byte-identical** to the committed bundle (the
  refactor is bundle-neutral; `git status` shows no `dist/` change).
- `npm audit` → 0 vulnerabilities.
- No Go source changed → Go tree unaffected (`go.mod`/`go.sum` unchanged); Vitest
  is dev-only and never enters the `go:embed`-ded `dist`, so the "one Go
  dependency" story holds.

## Counts

- Go packages/tests unchanged (80 / 2473). New: **19 Web UI unit tests**; CI
  checks 18 → **19**.

## Out of scope (documented follow-ups)

- Component/DOM tests (jsdom + Testing Library) and Playwright E2E in CI — this
  pass covers the pure logic; rendering is still verified by manual browser smoke.
