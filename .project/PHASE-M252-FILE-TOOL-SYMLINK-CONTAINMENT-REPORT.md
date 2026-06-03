# M252 — Close an absolute-path symlink-containment bypass in the file tool

## Why
Continuing the tool-security audit, the `file` tool's `resolve()` containment
function had an asymmetry. For a **relative** path it resolved symlinks
(`os.Lstat` → `filepath.EvalSymlinks`) and checked the *real* location was
inside root. For an **absolute** path it only did a lexical `withinRoot` check
and returned early — **no symlink resolution**. So a symlink living inside the
workspace root but pointing outside it (e.g. `<root>/link → /etc/passwd`) was
refused when the agent referenced it relatively (`"link"`) but **slipped through
when referenced by its absolute path** (`"<root>/link"`), letting the agent
read or write the off-root target — a workspace-containment escape.

## What
- **`plugins/tools/file/file.go`** — `resolve()` now computes the cleaned
  absolute path for both the relative and absolute cases, then runs the **same**
  existence → `EvalSymlinks` → `withinRoot` check for both. The absolute branch
  no longer returns before the symlink check. New-file behaviour (writing to a
  path that doesn't exist yet) is unchanged — it still falls back to the lexical
  containment check so legitimate new files inside root succeed.

## Files
- `plugins/tools/file/file.go` — unified `resolve()` containment (edited).
- `plugins/tools/file/file_test.go` — `TestContainment_SymlinkEscapeBlockedBothPaths`:
  a symlink inside root → an outside secret is refused via BOTH its relative and
  absolute path (skips where the platform can't create symlinks) (new).

## Verification
- `go test ./plugins/tools/file/` — green (new test passes; the absolute case
  would read the secret under the old code); full suite **1822 → 1823** (+1), 66
  packages, `go test ./...` exit 0.
- `gofmt -l` (CRLF-normalised) clean; `go vet ./plugins/tools/file/` clean.
- `GOOS=linux go build -buildvcs=false ./...` clean; `go.mod` / `go.sum`
  unchanged.

## Scope notes
- A deeper, separate hardening remains for **new files under a symlinked parent
  directory** (both branches still do a lexical-only check when the target
  itself doesn't exist yet) — a candidate follow-up; this milestone fixes the
  clear, demonstrable relative-vs-absolute inconsistency.
- Second milestone in the post-vision tool-security sweep (after M251's http
  redirect allowlist).
