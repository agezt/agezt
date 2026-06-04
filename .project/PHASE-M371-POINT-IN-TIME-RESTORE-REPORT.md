# M371 — Point-in-time restore `agt restore --at` (SPEC-09 §5/§8)

## SPEC audit (read-vs-code)
SPEC-09 §5 (Backup & point-in-time restore) and §8 (CLI surface) define:

> **Point-in-time restore (free from event-sourcing):** "return to the state as
> of last Tuesday 14:00" = replay the journal up to that sequence/timestamp. No
> special backup format needed; the journal *is* the time machine.
> `agt restore --at <timestamp|seq>` produces a projection at that point
> (non-destructively — it appends, never rewrites; you can branch a recovered
> state).

**Verified gap:** `agt restore <bundle>` (M113) restores a backup *bundle*, and
`agt journal import` seeds a journal from a *full export*, but **neither does
point-in-time** — there was no way to reconstruct the state as of a past
sequence/timestamp from the live journal. The journal-as-time-machine capability
§5 highlights was missing. Genuine SPEC-09 §5/§8 gap, offline-verifiable.

## What
- **`cmd/agt/backup.go`** — `cmdRestore` gains a point-in-time mode:
  `agt restore --at <seq|RFC3339> --to <dir> [--home <src>]`. It opens the source
  journal read-only, verifies its chain, collects the contiguous genesis→cutoff
  prefix (stops at the FIRST event past the cutoff so the slice stays a gap-free
  chain even if timestamps aren't perfectly monotonic), and `journal.Restore`s it
  into a fresh `--to` home, then confirms the result boots (`verifyHomeJournal`).
  `parseAtSpec` reads a plain integer as a sequence and anything else as an
  RFC3339 timestamp. The bundle-restore mode is unchanged; `--to` is rejected
  without `--at`, and a positional archive with `--at` is rejected.
- Non-destructive by construction: the source is opened read-only and never
  written; the branch is a separate home. `journal.Restore` already enforces an
  empty target and full chain verification before touching disk.

## Verification
- **`cmd/agt/restore_pit_test.go`** (5 tests, offline): by-seq cutoff yields the
  exact genesis→N prefix that verifies AND leaves the source untouched;
  cutoff-beyond-head restores all (clamp); timestamp branch (future → all, past →
  clean "no events" error); argument-shape usage errors (missing `--to`, `--to`
  without `--at`, `--at` with a positional archive, unparseable `--at`); refusal
  to clobber a non-empty `--to`.
- **Live daemon demo**: a mock daemon produced a journal to seq 11 (2 runs);
  after stopping it, `agt restore --at 5 --to <branch>` reported "restored
  point-in-time state up to seq 5: 6 event(s), head seq 5", the branch journal
  held seq 0..5, and the source stayed at seq 11 (untouched).
- `gofmt -l` clean; `go vet` clean; `GOOS=linux go build ./...` exit 0. Full
  suite **2128** passing (was 2123; +5), `go test ./...` 0 failures.
  `go.mod`/`go.sum` unchanged. CHANGELOG (Added, user-visible).

## Scope notes
- SPEC-09 now broadly complete: §1 ULID/content-address discipline (everywhere),
  §2/§4 export/import bundle (M101/M102/M103), §5 backup+restore (M113) + inspect
  (M266) + **point-in-time (M371)**, integrity (`agt journal verify`).
- Granular export scopes (§3 `--scope agent:/task:/skill:/memory:`) and competitor
  migration (§6 `agt migrate openclaw|hermes`) remain — larger features, recorded
  for honest tracking, not closed this turn.
