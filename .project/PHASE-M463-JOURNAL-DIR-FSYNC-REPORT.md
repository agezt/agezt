# M463 — Journal: fsync the parent directory after creating a segment

## Context
The journal fsyncs each segment **file** after writing (M462 hardened that path).
But creating a file also adds a **directory entry**, and on most filesystems that
metadata is only durable after the parent directory itself is fsync'd. The journal
never did that.

## The bug (MED)
`rotate`, `openCurrent` (create path), and `Restore` create a new segment with
`O_CREATE|O_EXCL` and fsync the file's contents, then return success — at which
point the durable-before-publish contract is considered satisfied for records
written into that segment. On power loss before the directory metadata reaches
disk, the file's content may be durable while its directory entry is not, so the
freshly created/rotated segment — and every "durably published" event in it — can
disappear after reboot. This affects the first records at any segment boundary.
Severity MED (rotation-boundary records only, and only on true power loss).

## The fix
Best-effort parent-directory fsync at each new-segment site:

```go
var syncDir = func(dir string) error {
    d, err := os.Open(dir)
    if err != nil { return err }
    defer d.Close()
    return d.Sync()
}
```

Called as `_ = syncDir(j.dir)` after the new file is created in `rotate`, in
`openCurrent` when `!appendMode` (covers Open's initial segment), and after the
segment is written+closed in `Restore`. Best-effort because a directory fsync can
legitimately fail on some platforms (e.g. a directory handle on Windows, the dev
OS); the durability guarantee it adds is for the Linux deploy target, and a dir
fsync failure must not fail segment creation. `syncDir` is a package var so the
call is assertable in tests.

## Test + negative control
`kernel/journal/journal_test.go`: `TestRotate_FsyncsParentDir` — installs a
counting `syncDir`, opens a journal with tiny segments (so each append rotates),
resets the counter after Open, appends several events, and asserts `syncDir` was
invoked by rotation.

**Negative control:** removing the `syncDir(j.dir)` call from `rotate`
(`if false { ... }`) made the count stay 0 — the test reported `rotation did not
fsync the parent directory` and FAILED. Restored; test passes.

## Provenance / remaining
Second of three journal durability findings (M462 was the first). The last one
(A3, LOW) remains: `rotate` permanently stuck if the next-index segment file
already exists (`O_EXCL` fails and the error is swallowed by `_ = j.rotate()`),
which can happen after a partial prior rotation — tracked.

## Verification / gate
- `kernel/journal` tests pass.
- gofmt-clean on staged LF blobs, `go vet` clean, `GOOS=linux` build clean, full
  `go test ./...` exit 0, `go.mod`/`go.sum` unchanged.
