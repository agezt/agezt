# M440 — File tool: O_NOFOLLOW on the write paths (close the create-new TOCTOU)

## Context
Resolving the deferred LOW from M427: *"doWrite create-new lacks O_NOFOLLOW
(narrow concurrent-Invoke TOCTOU)."* M427 closed the search/glob symlink-escape;
this completes the write-side defense.

## The gap
`resolve()` symlink-resolves a path and rejects out-of-root targets. For an
EXISTING path it returns the EvalSymlinks-resolved real path (so the final
component is never a symlink); for a NEW (not-yet-existing) path it validates the
deepest existing ancestor and returns the lexical target. In the new-file case
there is a narrow TOCTOU: between `resolve()` and `os.OpenFile(p, …|O_CREATE…)`,
a concurrent process could plant a symlink at `p` pointing outside the workspace,
and a plain `O_CREATE` open would follow it — an out-of-root write. `doReplace`'s
`os.WriteFile` (O_CREATE|O_TRUNC) had the same window.

Exploitability is genuinely narrow: the file tool itself cannot create symlinks,
so a file-tool-only agent cannot plant the link; it requires a *separate*
concurrent writer to the workspace and precise timing. Hence LOW — but
`O_NOFOLLOW` closes it definitively at no cost to legitimate writes.

## The fix
- `oNoFollow` constant, build-tagged: `syscall.O_NOFOLLOW` on `//go:build unix`
  (`nofollow_unix.go`), `0` on `//go:build !unix` (`nofollow_other.go`). The
  symlink-swap TOCTOU is a Unix sandbox concern; Windows symlink creation is
  privileged, and `O_NOFOLLOW` is POSIX-only.
- `doWrite`: `flag := os.O_WRONLY | os.O_CREATE | oNoFollow` (append/trunc as
  before).
- `doReplace`: `os.WriteFile` replaced with an explicit
  `os.OpenFile(p, O_WRONLY|O_TRUNC|oNoFollow, origPerm)` + write + close.

`O_NOFOLLOW` affects only the FINAL path component, and never triggers in normal
use (resolve returns a non-symlink real path or a fresh name); it fires only on
the planted-symlink race, failing the open with ELOOP instead of following it.

## Verification
- **`plugins/tools/file/nofollow_unix_test.go`** (`//go:build unix`)
  `TestONoFollow_RefusesSymlink`: opening a symlink with the exact flag combo
  `doWrite` uses must fail, while the same open WITHOUT `O_NOFOLLOW` succeeds
  (the behavior the guard prevents). This *is* the negative control in one test —
  if `oNoFollow` were the `0` no-op, the guarded open would succeed and the test
  would fail.
- **Platform note (transparent):** the dev host is win32; this Unix-only test
  cannot execute here. It was verified by `GOOS=linux go test -c` (compiles for
  linux) + `GOOS=linux go vet` (clean) + code review + the documented `O_NOFOLLOW`
  kernel contract. The Windows path uses the `0` no-op and the **full existing
  file-tool suite passes on Windows**, confirming no regression to normal
  write/append/replace. The unix behavior test runs on any Unix host/CI.
- **Gate:** staged (LF) blobs gofmt-clean, `go vet` clean (Windows) and
  `GOOS=linux go vet` clean, `GOOS=linux go build ./...` ok, `go.mod`/`go.sum`
  unchanged. Full suite **2314** on Windows (the +1 unix-tagged test runs on
  Unix), `go test ./...` exit 0. CHANGELOG Security entry.

## Review status
The file tool's symlink-escape defense now spans read/search/glob (M427) and the
write/replace create paths (M440). Remaining file-tool items: none outstanding.
