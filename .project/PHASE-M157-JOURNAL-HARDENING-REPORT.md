# M157 — Journal hardening: torn-line tolerance + rotation resilience (code review)

## Why
Continuing the code-quality mandate, an independent review of the event-sourcing
foundation — the BLAKE3 hash-chained journal and the bus — was commissioned,
focused on concurrency, chain integrity, rotation/recovery, and resource handling.
It confirmed the chain logic, rotation chain-continuity, `Verify` tamper detection,
`Restore`, file-handle hygiene, and bus subscription lifecycle are correct, and
found two real bugs.

## Fixes

### C2 — Torn final-line read makes healthy reads/recovery fail (Critical)
`Range`, `Tail`, `Verify`, and crash recovery all scan segments with
`bufio.Scanner`'s default `ScanLines`, which yields an **unterminated** trailing
line as a token. But:
- `Range` runs concurrently with `Append` (durable-before-publish puts `Append` on
  the hot path of every kernel action), with no lock between them. A reader can
  observe the current segment mid-write — a final line whose bytes are present but
  whose terminating `\n` isn't yet — and `ScanLines` hands that partial JSON to
  `event.Decode`, which errors. A perfectly healthy journal thus makes `Range` /
  `Tail` / `Verify` (and `agt journal grep/stats/export`, `agt inbox`, `agt runs`,
  conversation history…) **spuriously fail** whenever a write is in flight.
- Crash recovery (`scanSegment`) hit the same path: a crash mid-write leaves a torn
  final line, and the daemon would **refuse to start** (decode error) instead of
  recovering past it.

**Fix:** every durably-written journal line ends in `\n` (`writeAndSync` appends
it), so a line missing its newline is *never* a committed record — only an
in-flight append or a torn/crashed write. `newLineScanner` now uses a custom split
(`scanCompleteLines`) that emits only newline-terminated lines and **discards a
trailing unterminated line at EOF**. A torn line can only ever be the LAST line of
the current segment (appends are serialized), so a corrupt MIDDLE line still
surfaces as a decode error. One change fixes `Range` / `Tail` / `Verify` / recovery
uniformly (all go through `newLineScanner`).

### H2 — Failed rotation wedges the journal (High)
`writeAndSync` rotated by **closing the old segment first**, then opening the next.
If the open failed, `j.curFile` was left referencing the just-closed handle and
`head`/`nextSeq` weren't advanced — even though the event had already been
written+fsynced to the old segment (durable). Result: every subsequent `Append`
failed on the closed handle (journal wedged) and `Close` would double-close, with
an in-memory/on-disk divergence that only self-healed across a restart.

**Fix:** a new `rotate()` helper opens the next segment **before** swapping; on
open failure it returns with all state unchanged (the current, now-oversized
segment stays live and usable). And because the event is already durable when
rotation runs, `writeAndSync` no longer fails an (already-committed) append if
rotation fails — it leaves the segment slightly oversized and the next append
retries rotation. No wedge, no divergence.

## Not changed (documented tradeoffs)
- **M1 (bus holds `b.mu` across `Append`'s fsync)**: real contention, but the lock
  also guards subscriber-map iteration + ordering; releasing it mid-append raises
  ordering questions. Left as a deliberate tradeoff.
- **L1 (no directory fsync on segment create)**: a power-loss durability gap for a
  freshly-rotated/created segment's directory entry. Real but power-loss-timing
  only; noted for a future durability pass.

## Files
- `kernel/journal/journal.go` — `scanCompleteLines` split in `newLineScanner`;
  `rotate()` (open-before-swap) + `writeAndSync` no longer fails on rotation error.
- `kernel/journal/journal_test.go` — three new tests.

## Tests (+3, all passing)
- `TestReaders_TolerateTornFinalLine` — a truncated, newline-less line appended to
  the live segment: `Range` and `Tail` return exactly the committed events, no error.
- `TestOpen_RecoversPastTornFinalLine` — recovery boots cleanly past a torn final
  line, head at the last committed seq.
- `TestRotate_FailureDoesNotWedge` — with the next segment path pre-occupied (so
  rotation's `O_EXCL` open always fails), all 20 appends still succeed, head
  advances, and `Range` returns them all.

## Verification
- `go.mod` / `go.sum` unchanged.
- `go vet ./...` clean; `GOOS=linux go build ./...` ok.
- `gofmt -l` (LF-normalized) clean on both touched files.
- `go test ./... -count=1` — **FAIL 0**, **1489 tests** (was 1486; +3), 61 packages;
  journal package stable across 5 runs (all Range-based consumers still pass).

## Result
The journal — the system's source of truth — no longer fails healthy reads during a
concurrent write, recovers cleanly past a crash-torn final event, and survives a
failed rotation without wedging. The chain-integrity and tamper-detection logic the
whole audit story rests on was reviewed and confirmed correct.
