# Phase Report — Milestone M102 (journal import / restore)

> Status: **shipped** · Date: 2026-06-01 · SPEC-09 §8 (export/import/backup).

## Why

M101 made the journal exportable and re-verifiable. The other half of disaster-
recovery / migration is reading a bundle back: `agt journal import <bundle>`
seeds a fresh host from an export so the daemon boots with the recovered history
and rebuilds every projection (state/memory/world/runs/policy) from events —
because everything is an event, restoring the journal restores the system.

## What shipped

- **`journal.Restore(dir, events)`** — the journal package gains a strict,
  non-destructive restore primitive (it owns its on-disk format). It:
  - requires the slice to start at seq 0 with prev_hash == GenesisHash and to
    chain-verify end-to-end (`ErrNotFullExport` / `ErrChainBreak`), so the
    result boots through the very same scan `Open()` runs;
  - refuses a directory that already holds segments (`ErrNotEmpty`) — never
    clobbers an existing chain;
  - validates fully **before the first byte hits disk**, then writes every event
    verbatim (re-marshalled compact, one per line) as the initial segment.
- **`agt journal import <bundle> [--home <dir>]`** — offline CLI (no daemon):
  decode the bundle, call `Restore`, then **re-open the journal to confirm it
  boots cleanly** (head matches) so any problem surfaces at import, not at the
  next daemon start. Clear, distinct errors for the not-full-export and
  not-empty cases.

## Design decisions

- **Restore ≠ Append.** Re-appending would recompute seq/prev-hash and change
  every hash, destroying verifiability. Restore writes events verbatim, so the
  imported journal is byte-for-byte the exported one and still verifies.
- **Genesis-anchored only.** The boot-time `scanSegment` enforces seq-0-from-
  genesis, so a `--since` window cannot seed a journal. Import rejects that up
  front with a clear message instead of a cryptic boot failure later. (A wide
  `--since` that happens to capture seq 0 is a valid full export and imports
  fine.)
- **Offline + empty-target.** Restore is a fresh-host operation; targeting an
  empty home (daemon down) sidesteps a live daemon holding its own journal open,
  and the empty-dir guard makes the operation incapable of corrupting data.

## Tests

- `TestRestore_RoundTripBootsCleanly` — restore a 4-event chain into a fresh
  dir; re-`Open` boots, `Verify` passes, head + event count match the source.
- `TestRestore_RefusesNonEmpty` — `ErrNotEmpty` on a populated dir.
- `TestRestore_RefusesWindowedExport` — `ErrNotFullExport` on a slice missing
  genesis (and on an empty slice).
- `TestRestore_RefusesTamperedChain` — `ErrChainBreak` on a mutated payload,
  and **nothing is written** (validation precedes disk).

Test count: **1352 → 1356**. `go vet` clean, `GOOS=linux` builds, `go.mod`
unchanged, gofmt-clean.

## Live proof (full migration)

```
# source home: run a task, export
$ agt journal export --out backup.json          → exported 15 event(s) (seq 0..14)
# fresh home, daemon down:
$ agt journal import backup.json --home /restored
restored 15 event(s); head: seq=14 hash=f78e8f863464…
$ agt journal import backup.json --home /restored   # again
… already has a journal — restore only into an empty home   (exit 1)
# boot the daemon on the restored home:
$ agt journal verify   → { "ok": true }
$ agt runs list        → last 1 run(s): run-01KT230BXWH1Q6FEPNEX2XHB28   (migrated run survived)
```
