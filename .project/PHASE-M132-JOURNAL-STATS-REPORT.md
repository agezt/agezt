# M132 — `agt journal stats` (+ honest disk-pressure remedy)

## Why
Two things, one coherent concern (journal growth, understood accurately):

1. **A correctness bug I introduced in M131.** The M131 disk-pressure hint said
   "archive with `agt journal export`, then **prune**" — but there is **no journal
   prune**, and it would be unsafe: projections (state / memory / world / runs /
   policy / skill / cadence) are rebuilt from the **full** journal on boot, so
   deleting old segments destroys live state, not just audit trail. The journal is
   append-only **full-retention by design**.

2. **A real gap.** Given the journal can't be pruned, "how big is it and **what is
   filling it**" is the actual operator question feeding an archival / bigger-disk
   decision — and neither `agt disk` (bytes only) nor `agt status` (head seq only)
   answered it.

## What
- **`CmdJournalStats` + `agt journal stats [--json]`** — folds the journal once
  into: total event count, segment count, on-disk bytes, oldest/newest event
  timestamps (the time span), and a **per-event-kind breakdown** with percentages.
  Read-only; tenant-routed via `kernelFor(tenantOf(req))` so a future `--tenant`
  scopes it. Walks every segment (operator-invoked, 60s client timeout).
- **Honest remedy.** The disk-pressure hints in `agt disk` and the `agt doctor`
  disk check (M131) now point at the *real* path — archive with `agt backup` /
  `agt journal export` and move to a larger disk — and name the journal as
  full-retention, with `agt journal stats` to see what's filling it. No more
  reference to a non-existent / unsafe in-place prune.

## Files
- `kernel/controlplane/protocol.go` — `CmdJournalStats`; `server.go` dispatch.
- `kernel/controlplane/journal_stats.go` (new) — `handleJournalStats`,
  `countSegments` (reuses `dirSize` from M131).
- `cmd/agt/journal_stats.go` (new) — `cmdJournalStats`; wired into `cmdJournal`.
- `cmd/agt/doctor.go`, `cmd/agt/disk.go` — corrected disk-pressure hints.
- `kernel/controlplane/journal_stats_test.go` (new) — folds a known event mix
  (2 task.received + 1 task.completed → events 3, by_kind correct, segments ≥ 1,
  bytes > 0, newest ts > 0).

## Live proof (offline mock)
```
$ agt journal stats
events   : 19
segments : 1
size     : 10.7 KB  (append-only, full retention)
span     : 2026-06-02 → 2026-06-02 (61ms)

by kind (top events):
  llm.request                  3  (15.8%)
  routing.decision             3  (15.8%)
  budget.consumed              2  (10.5%)
  …
to reclaim space: archive with `agt backup` or `agt journal export`, then move to a larger disk
```
The breakdown reveals what dominates (LLM I/O + routing), which `disk`/`status`
can't show. A grep confirms no `prune` references remain in the disk hints.

## Verification
- 55 packages `ok`, **FAIL 0**; **1425 tests**.
- `go vet` clean, `GOOS=linux go build ./...` ok, `go.mod` / `go.sum` unchanged.
- gofmt: all new/touched files clean under LF.
