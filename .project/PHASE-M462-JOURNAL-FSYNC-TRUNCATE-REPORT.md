# M462 — Journal: a failed fsync no longer wedges recovery with a duplicate seq

## Context
`Journal.Append` mints `seq`/`prev_hash`, marshals the event, then calls
`writeAndSync` (write the line + fsync). It advances the in-memory chain
(`j.head`, `j.nextSeq`, `j.curBytes`) **only after** `writeAndSync` returns nil —
the durable-before-publish contract.

## The bug (MED)
```go
func (j *Journal) writeAndSync(line []byte) error {
    if _, err := j.curFile.Write(line); err != nil { ... }
    if err := j.curFile.Sync(); err != nil {
        return fmt.Errorf("journal: fsync: %w", err)   // line already in the file
    }
    j.curBytes += int64(len(line))
    ...
}
```
If `Write` fully succeeds (the complete newline-terminated line lands in the page
cache) but `Sync` fails (EIO / ENOSPC), `writeAndSync` returns the error **without**
advancing `seq`/`head`/`curBytes`. The line is still physically in the segment.
Because `nextSeq` did not advance, the **next** append reuses the same seq and
appends a second line for it (O_APPEND → end of file). The segment now holds two
lines with the same seq. On the next `Open`, `scanSegment` accepts the first, then
sees the second whose `seq != nextSeq` → `ErrChainBreak`, and the journal refuses
to boot. This is not a torn tail (M417) — both lines are newline-terminated — so
recovery cannot heal it. A single transient fsync failure permanently wedges boot
(and meanwhile desyncs `curBytes`). Severity MED (fsync failures are rare but real;
the outcome is a hard boot wedge).

## The fix
On fsync failure, truncate the just-written line back to the last committed size so
the file matches the un-advanced in-memory chain; the caller treats the append as
failed (fail-closed):
```go
if err := fsync(j.curFile); err != nil {
    _ = j.curFile.Truncate(j.curBytes)
    _, _ = j.curFile.Seek(j.curBytes, io.SeekStart)
    return fmt.Errorf("journal: fsync: %w", err)
}
```
`curBytes` is the committed size (it advances only after a successful sync), so the
truncation removes exactly the un-synced line. The `Seek` is required on platforms
where O_APPEND is emulated (Windows): after `Truncate` the handle's offset is left
past the new EOF, so the next write would land beyond `curBytes` and the OS would
zero-fill the gap (observed in testing as `\x00` bytes that corrupted the segment).
Seeking back keeps the resume offset consistent on every platform. `fsync` is a
package var (`var fsync = (*os.File).Sync`) so the failure is injectable in tests.

## Test + negative control
`kernel/journal/journal_test.go`: `TestAppend_FsyncFailureLeavesNoDuplicateSeq` —
append (seq 0, committed); override `fsync` to fail and append (must error); restore
`fsync` and append again (seq 1); close; reopen and assert it boots with head seq 1
(two committed events, the failed one truncated). Seqs are 0-indexed.

**Negative control:** disabling the truncate+seek (`if false { ... }`) made reopen
fail with `chain break: expected seq 2, got 1` — the exact duplicate-seq boot wedge
— `--- FAIL`. (With truncate but no seek, an earlier run instead corrupted the
segment with `\x00` zero-fill, which is what motivated keeping the `Seek`.) Restored;
test passes.

## Provenance / follow-ups
First of three journal durability findings from the scoped journal/bus/memory/
worldmodel review (bus, memory, worldmodel all reviewed CLEAN). The other two are
tracked: (A2 MED) no parent-directory fsync after creating/rotating a segment, so a
freshly rotated segment's directory entry can be lost on power loss; (A3 LOW)
`rotate` permanently stuck if the next-index segment already exists (O_EXCL fails,
error swallowed).

## Verification / gate
- `kernel/journal` tests pass.
- gofmt-clean on staged LF blobs, `go vet` clean, `GOOS=linux` build clean, full
  `go test ./...` exit 0, `go.mod`/`go.sum` unchanged.
