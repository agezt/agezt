# Phase Report — Milestone M110 (`agt doctor` catalog freshness)

> Status: **shipped** · Date: 2026-06-02 · SPEC-08 / cost accuracy.

## Why

Model pricing lives in the synced API catalog and drives every cost estimate and
budget-enforcement decision. If the catalog goes stale, those numbers silently
drift from reality — spend tracking and `agt budget`/`budget check` (M107) become
wrong without any error. The operator's go-to diagnostic should flag it.

## What shipped

- **`checkCatalog` doctor check** — reads `api_synced_at` from `CmdCatalogList`
  and WARNs when the catalog hasn't been synced in over 21 days (hint:
  `agt catalog sync`). A never-synced catalog (offline/mock, pre-sync) or an
  unreachable call is an informational OK, never a FAIL.
- **`catalogCheckFromSync(apiSyncedAt, now)`** — the pure staleness verdict,
  unit-testable; handles the zero-time (`0001-01-01`) and unparseable cases as
  OK so a fresh/offline daemon never false-alarms.

## Tests

- `TestCatalogCheckFromSync` — fresh (2 days) → OK; stale (30 days) → WARN;
  empty / zero-time / unparseable → OK.

Test count: **1370 → 1371**. `go vet` clean, `GOOS=linux` builds, `go.mod`
unchanged, gofmt-clean.

## Live proof

```
$ agt doctor                                   # offline mock, never synced
  [OK  ] catalog freshness : no API catalog synced (offline/mock, or pre-sync)
$ # with a meta.json synced 2026-01-01:
  [WARN] catalog freshness : API catalog last synced 151 day(s) ago — model pricing may be stale
         ↳ refresh with `agt catalog sync` so cost estimates and budget enforcement use current prices
```
