# Phase M845 — dead-file collector (reap stale artifacts, with confirm)

**Date:** 2026-06-11 · **Status:** DONE · **Trigger:** owner: "ölü artifact ve
files için de collector lazım … bunu onaylı veya otonom ve haber vererek ve
etkileri ile yapmak lazım." This milestone delivers the **artifact/file
collector** half of the reaper (#53); the dead-AGENT graveyard is the follow-up.

## What shipped

A collector that reaps **stale** artifacts, with an operator-confirmed dry-run →
delete flow (the "onaylı" path).

- **Engine** (`kernel/artifact/index.go`): `StaleEntries(olderThanMs)` returns the
  entries created before a cutoff (oldest-first; entries with no creation time are
  never stale, so a malformed entry isn't reaped). `Collect(olderThanMs)` deletes
  them and reports `(count, bytes)` — the blob is GC'd by the existing dedup-aware
  `Delete` only when no surviving entry references it.
- **Control plane** (`CmdArtifactCollect {older_than_days, dry_run}`): **dry_run
  defaults to true** — it only reports candidates (count, bytes, list); `dry_run
  =false` deletes. Default age 30 days.
- **Web UI** (Files view "Collect" button): runs a dry-run, and if anything
  qualifies, asks `ui.confirm` ("Collect N stale files? … ~X freed; recent files
  kept") before deleting — so a human approves the reap. "Nothing to collect" when
  clean.

## Verification

- **Unit** (`TestIndex_StaleAndCollect`): stale-before-cutoff returns the two
  oldest oldest-first; a no-created-time entry is never stale; `Collect` removes
  the old two (correct byte sum), keeps the fresh one.
- **Live HTTP** (isolated home): a freshly fetched artifact was present (count 1);
  `POST /api/artifact/collect?older_than_days=30&dry_run=true` returned
  **count 0** — recent files are correctly protected. The actual deletion path is
  covered by the unit test (deterministic on backdated `created_ms`).
- Go build + artifact/controlplane/webui tests green; frontend tsc + **517
  vitest** green; dist rebuilt (LF).

## Gate

vet + staticcheck + linux clean; gofmt swept; go.mod unchanged.

## Next (reaper arc)

- **Autonomous + notify**: a pulse observer that periodically reports how much is
  collectable (notify) and, opt-in, auto-reaps past a configurable age.
- **Dead-agent graveyard** (#53): a retired roster state (not hard-delete),
  excluded from delegation, with an impact analysis (what standing orders /
  workflows reference the agent) before retiring — with-approval or
  autonomous-with-notification.
