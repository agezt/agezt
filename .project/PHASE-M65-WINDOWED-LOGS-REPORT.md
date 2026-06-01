# Phase Report — Milestone M65 (`--since` windowing for edict log & schedule fires)

> Status: **shipped** · Date: 2026-06-01 · SPEC-08 observability.

## Why

`agt edict stats` and `agt schedule stats` accept `--since <dur>` to window their
aggregates, but their per-event counterparts — `agt edict log` and `agt schedule
fires` — did not. An operator asking "what policy decisions / scheduled firings
happened in the last hour?" had no time filter. M65 adds `--since` to both,
completing the filter parity with their stats counterparts (and `agt runs stats`).

## What shipped

- **Shared `sinceCutoff(arg)` helper (`kernel/controlplane/policy_log.go`)** —
  converts an optional `since_ms` arg into an absolute cutoff (now − since_ms),
  `0` for no window. One helper, used by both windowed folds (and ready to dedupe
  the stats handlers' inline copies later).
- **`handleEdictLog` + `handleScheduleFires`** — apply the cutoff during the
  journal walk (skip events older than the window).
- **CLI `--since <dur>`** — added to `agt edict log` and `agt schedule fires`
  (both `--since X` and `--since=X`), documented in `--help`.

## Design decisions

- **Server clock = event clock.** The cutoff is computed against the server's
  clock, the same one that stamps event `TSUnixMS`, so the comparison is
  apples-to-apples (matching the M33 `runs stats --since` semantics).
- **Parity, not novelty.** This mirrors the established windowing already in the
  stats handlers; the only new code is the shared helper + two one-line cutoffs.

## Tests

- `TestEdictLog_SinceWindow` — a 1h window includes a just-published decision; a
  1ms window after a brief sleep excludes it.
- `TestScheduleFires_SinceWindow` — same for scheduled firings.

Test count: **1303 → 1305**. `go vet` clean, `GOOS=linux` builds, `go.mod`
unchanged, gofmt-clean.

## Live proof

```
$ agt edict log --since 1h
  2026-06-01 13:01:16  allow      shell  shell  (...)
$ agt edict log --since 1ms
  no policy decisions journaled yet.
```
