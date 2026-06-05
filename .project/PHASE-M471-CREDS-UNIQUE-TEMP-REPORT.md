# M471 — Creds vault: unique temp file for atomic writes

## Context
`Store.Save()` and `Store.Rotate()` write the vault atomically (write temp →
rename). `Save` holds only the read lock (it only reads `s.data`); `Rotate` holds
the write lock.

## The bug (LOW)
Both used a **fixed** temp path:

```go
tmp := s.Path + ".tmp"
os.WriteFile(tmp, raw, 0600)
os.Rename(tmp, s.Path)
```

Two concurrent `Save()` calls both hold the read lock (which permits concurrent
readers), so they run simultaneously and race on the same `creds.json.tmp`: one can
rename a partially-written temp while the other is still writing it, leaving a torn,
unloadable vault. Not exercised by the current architecture (`agt` is a single-shot
CLI; the daemon only reads), hence LOW — but a real latent footgun, and the fixed
name is the root cause.

## The fix
A shared `atomicWriteVault(path, data)` helper used by both `Save` and `Rotate`,
writing to a **unique** temp (`os.CreateTemp(dir, ".creds-*.tmp")`), fsyncing, then
renaming and forcing 0600. A unique name per call removes the collision entirely; it
also adds an fsync the old path lacked. `Rotate` already held the write lock so it
never collided, but it now shares the same hardened helper.

## Test + negative control
`kernel/creds/creds_test.go`: `TestStore_SaveUsesUniqueTemp` — occupies the old
fixed `creds.json.tmp` path with a directory, then asserts `Save()` still succeeds
and round-trips, and leaves no unique-temp litter. This proves the fixed name is no
longer used; it is deterministic and portable (a concurrency test was rejected
because concurrent `os.Rename` to one target fails with EACCES on Windows — an
orthogonal limitation, and concurrent Save is unreachable in the real architecture
anyway, so the root-cause test is both more robust and more honest).

**Negative control:** reverting `atomicWriteVault` to the fixed `path + ".tmp"`
write made `Save()` fail (`open …/creds.json.tmp: is a directory`) — the test FAILED.
Restored; test passes.

## Verification / gate
- `kernel/creds` tests pass.
- gofmt-clean on staged LF blobs, `go vet` clean, `GOOS=linux` build clean, full
  `go test ./...` exit 0, `go.mod`/`go.sum` unchanged.
