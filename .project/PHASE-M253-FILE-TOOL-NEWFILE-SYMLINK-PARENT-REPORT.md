# M253 — Close the symlinked-parent escape for new files in the file tool

## Why
M252 fixed the relative-vs-absolute symlink asymmetry for **existing** targets.
A deeper hole remained for **new** files: when the target doesn't exist yet,
`EvalSymlinks(clean)` fails, so `resolve()` fell back to a lexical `withinRoot`
check. That check is blind to a **symlinked parent directory** — writing
`"linkdir/new.txt"` where `<root>/linkdir` is a symlink to `/outside` is
lexically inside root, so the new file would be created in `/outside`, escaping
the workspace.

## What
- **`plugins/tools/file/file.go`** — the new-file branch of `resolve()` now
  calls `resolveNewWithinRoot`: it walks up to the **deepest existing ancestor**
  of the target, `EvalSymlinks`-resolves it, confirms the real location is
  inside root, then re-appends the non-existent suffix (which contains no
  symlinks, because those components don't exist). A symlinked parent therefore
  resolves to its real (out-of-root) location and is refused, while a genuinely
  new nested path inside root (creating parent dirs) still resolves and
  succeeds.

## Files
- `plugins/tools/file/file.go` — `resolveNewWithinRoot` + new-file branch
  (edited).
- `plugins/tools/file/file_test.go` — 2 tests: a new file under a symlinked
  parent dir is refused (and nothing is written outside root); a legitimate new
  nested path still works (new).

## Verification
- `go test ./plugins/tools/file/` — green; full suite **1823 → 1825** (+2), 66
  packages, `go test ./...` exit 0.
- `gofmt -l` (CRLF-normalised) clean; `go vet ./plugins/tools/file/` clean.
- `GOOS=linux go build -buildvcs=false ./...` clean; `go.mod` / `go.sum`
  unchanged.

## Scope notes
- Together M252 + M253 close the file tool's symlink-containment surface for
  both existing targets (any access path) and new targets (symlinked parent),
  for relative and absolute inputs alike. The original M1 policy comment
  ("no `..` escape, no absolute paths outside root, no symlink escape") now holds
  on every code path.
- Third milestone in the post-vision tool-security sweep (M251 http redirects,
  M252 file abs-path symlink, M253 file new-file symlinked parent).
