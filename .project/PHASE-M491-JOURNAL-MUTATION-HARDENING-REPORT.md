# M491 — Mutation testing the journal: pin rotation accounting + Tail trim

## Context
Continuing the mutation-testing pass (M490) into the other integrity-critical
package: `kernel/journal`, the append-only, BLAKE3 hash-chained audit log. Ran
`go-mutesting .` from inside the package (Windows drive-letter workaround).

## Baseline & the metric caveat
Score **0.425** (105 killed / 142 survived / 247 total). Unlike redact, the journal
score is a *weak* aggregate here: the large majority of survivors are low-value
mutants that change an **error message or wrapping** (`fmt.Errorf("journal: …")` →
`_,_,_ = …`, `return err` → `_ = err`) or remove a **best-effort cleanup**
(`f.Close()`, `os.Remove(path)` on error paths). The error is still propagated and
the cleanup still happens on the success path; tests rightly don't assert exact
error strings, so these survive without indicating a real defect. Chasing them would
mean brittle string assertions, not hardening.

Two survivors, however, were **genuine behavioral gaps** in the integrity-critical
paths — and the existing rotation/Tail tests could not catch them by construction.

## The genuine gaps (closed)
1. **Rotation byte accounting** — `j.curBytes += int64(len(line))` → `j.curBytes =
   int64(len(line))` survived. The existing rotation tests use *tiny* segment
   thresholds where a single event line already exceeds the limit, so rotation fires
   per-append regardless of whether the running total accumulates. A regression to
   non-accumulating accounting would keep `curBytes` pinned at one line's size, so
   rotation would **never** fire for normal-sized events and the journal would grow
   into one unbounded segment (slow reads, no rotation). No test distinguished `+=`
   from `=`.
2. **Tail trim** — `collected = collected[len(collected)-n:]` → `…+n…` survived. The
   existing cross-segment Tail test gathers *exactly* n (one event per tiny segment,
   breaking at n), so the trim line never executes. The `+n` mutant
   (`collected[len+n:]` → out-of-range panic / mis-slice) went undetected.

## Fix
`kernel/journal/mutation_internal_test.go` (internal `package journal`):
- `TestRotate_AccountsForAccumulatedBytes` — self-calibrating: first measures one
  event line's on-disk size under a 1 GiB threshold (no rotation), then opens a
  journal with a threshold of ~3.5 lines so **no single line** can rotate, appends 12
  events, and asserts ≥2 segments exist, `Verify()` passes, and all 12 read back. With
  non-accumulating accounting only 1 segment is produced → fail.
- `TestTail_TrimsExcessToLastN` — one large segment holds 5 events; `Tail(2)` must
  gather 5 and trim to the last 2 (seqs 3,4 in order). The `+n` mutant panics /
  mis-slices here.

## Result
Score **0.437** (108 killed). The modest delta reflects the error-message-dominated
survivor pool, **not** the value of the fix: the two mutants that matter for journal
integrity are now killed (verified — their diffs no longer appear among survivors;
go-mutesting prints diffs only for survivors). The remaining `len(collected) > n` →
`>= n` survivor on the Tail line is an equivalent mutant (`collected[n-n:]` ==
`collected[0:]` when `len == n`, identical result).

## Verification / gate
- New tests pass; `go vet` + `staticcheck` clean; gofmt-clean on the staged LF blob.
- `kernel/journal` suite green; `go.mod`/`go.sum` unchanged.

## Takeaway
Mutation score is a blunt instrument on a package whose error-handling dominates the
mutant population; the right use is to *triage* survivors for genuine behavioral gaps
rather than to maximize the number. The two real gaps here — rotation accumulation
and Tail trimming — are now pinned, so a one-token regression in either (the kind no
existing test caught) would fail the suite.
