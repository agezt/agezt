# Phase M580 — Live SSE updates for Web UI run-detail cards

**Type:** Feature (Web UI; M577 follow-up)
**Date:** 2026-06-08
**Branch:** `feat-webui-live-runs`

## Goal

M577 fetched a run's event arc once on expand — static. Make an expanded run
update LIVE: fold the journal SSE stream into its arc so the detail cards
(status, tool calls, iterations, tokens) progress as the agent works, the same
live pattern Flow Studio uses for node recolour.

## What shipped

### `frontend/src/views/Runs.tsx`
`RunRow` now:
- Fetches the journaled snapshot once (first open) via a `useRef` guard, and
  **merges** it into the arc (rather than overwriting) so live events that
  arrived before the fetch resolved are kept.
- While open, `subscribe`s to the live journal stream and folds every event
  matching this run's `correlation_id` into the arc. `deriveDetail` recomputes →
  the cards update. Unsubscribes on collapse/unmount (effect cleanup).

### `frontend/src/lib/rundetail.ts` — `mergeEvents` (new, pure)
Unions two event lists, de-duplicating by journal `seq` (falling back to event
`id`), so an event delivered by both the snapshot fetch and the live stream is
counted once, and `seq 0` (a real first event) is preserved. Order-independent;
`deriveDetail` and the raw-event view sort by seq themselves.

### `frontend/src/lib/events.tsx` — stable `subscribe`
`subscribe` is now `useCallback`-memoized (the listener set is a ref), so it's
stable across renders and consumers can use it as a `useEffect` dependency
without re-subscribing on every incoming event. (Also tightens Flow Studio's
existing subscription, which previously re-ran on every event.)

### Tests
- `lib/rundetail.test.ts`: +3 `mergeEvents` cases (dedup by seq across snapshot
  + live; preserve seq 0 / dedup by id when seq absent; empty live = unchanged).
  Vitest total 19 → **22**.

## Verification

- **Vitest:** `npm test` → 3 files, **22 tests** pass.
- **Build:** `npm run build` → `tsc --noEmit` clean + dist rebuilt (JS hash
  changed; committed as LF). `go test ./kernel/webui` + full
  `GOMAXPROCS=3 go test ./... -p 2` → exit 0 (80 packages). No Go source changed.
- **Runtime smoke (criterion-7):** booted the real daemon, opened the Runs view
  in a browser (Playwright):
  - a completed `say hello` run expanded → cards render; **0 console errors**.
  - a `DEMO_LOOP` run (100 journaled events, 17 iterations, loop-guard) →
    the cards rendered the **failed** state correctly: red `failed` badge, the
    `shell` tool call with an `error` badge + the loop-guard message, `raw events
    (100)`. **0 console errors**.
  - Note: a deterministic mid-run *visual* capture isn't feasible with the
    offline mock (runs complete in ~165 ms). The live-append path itself is
    unit-tested (`mergeEvents`) and uses the identical, already-proven Flow Studio
    `subscribe` pattern; SSE liveness on the page is established by prior
    milestones (the `● live` feed). The browser smoke confirms no regression and
    correct rendering across completed/failed runs.

## Counts

- Go packages/tests unchanged (80 / 2473). Web UI unit tests 19 → **22**.

## Out of scope (documented follow-ups)

- A per-row "running" affordance driven by the live status (the detail card status
  already reflects the live arc; the collapsed row badge still shows the
  list-snapshot status until Refresh).
- Component/DOM tests (jsdom + Testing Library) and Playwright E2E in CI.
