# M467 — File tool: atomic `replace` (no data loss on a partial write)

## Context
The file tool's `replace` op (`plugins/tools/file/file.go doReplace`) is the
"surgical, low-clobber" edit: it reads a file, does a find/replace, and writes the
result back. It is explicitly sold as cutting "clobber risk" versus a
read-and-rewrite.

## The bug (MED)
The write-back opened the existing file with `O_TRUNC`:

```go
wf, err := os.OpenFile(p, os.O_WRONLY|os.O_TRUNC|oNoFollow, info.Mode().Perm())
...
wf.WriteString(updated)
```

`O_TRUNC` zeroes the file the instant it is opened, **before** the new content is
written. If `WriteString` fails partway (ENOSPC, quota, I/O error) or the daemon
crashes between the truncate and the write completing, the user's file is left
empty or half-written and the original content is gone. The op also did not
`Sync()`, so even a clean return wasn't durable. For the op whose entire selling
point is low-clobber editing, silently destroying the file on a partial write is a
real data-integrity defect.

## The fix
A standard atomic write — `atomicWriteFile(path, data, perm)`:

1. Create a fresh temp file in the same directory (`os.CreateTemp`, which uses
   `O_EXCL` → never a pre-existing symlink).
2. Write the full content, `Sync`, `Close`, `Chmod` to the original perm.
3. `os.Rename` the temp over the target.

The original is untouched until the rename atomically swaps in the **complete** new
content, so a mid-write failure can never truncate or destroy it (the temp is just
removed). `rename` does not follow a symlink at the destination (it replaces the
entry), so the write still cannot escape the workspace.

The M440 symlink-refusal guard (the old `O_NOFOLLOW` open) is preserved with an
explicit `os.Lstat` check before the write: a symlink at `p` is refused, exactly as
before. `doReplace` now also fsyncs (via the temp `Sync`), which the old path did
not.

`writeAll` (the temp write step) is a package var so a test can inject a write
failure.

## Test + negative control
`plugins/tools/file/file_test.go`:
`TestAtomicWriteFile_PreservesOriginalOnWriteFailure` — writes `ORIGINAL-CONTENT`,
overrides `writeAll` to fail, calls `atomicWriteFile`, and asserts the original
content is intact and no temp file leaked. Existing `TestReplace_*` (unique match,
ambiguous, all, not-found, guards) and the symlink-containment tests still pass.

**Negative control:** replacing `atomicWriteFile`'s body with the non-atomic
truncate-then-write (`os.OpenFile(path, O_TRUNC)` + the failing `writeAll`) left the
file empty — the test reported `original damaged by a failed write: got "", want
ORIGINAL-CONTENT (write was not atomic)` and FAILED. Restored; test passes.

## Provenance / remaining
From the scoped review of shell/http/file tools (shell and http reviewed CLEAN —
no command-injection or SSRF bypass). The same review noted two lower items left
documented: `doWrite`'s overwrite path shares the truncate-then-write window but is
a "write the whole file" op that already `Sync`s (MED/LOW); and `doRead`'s
truncated-file branch uses a single `f.Read` that can return fewer bytes than
requested and discards the read error (LOW). Tracked.

## Verification / gate
- `plugins/tools/file` tests pass.
- gofmt-clean on staged LF blobs, `go vet` clean, `GOOS=linux` build clean, full
  `go test ./...` exit 0, `go.mod`/`go.sum` unchanged.
