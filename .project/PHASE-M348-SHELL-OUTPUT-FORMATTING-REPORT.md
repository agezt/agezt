# M348 — Shell tool output-formatting coverage

## Why
Priority-A coverage on the shell tool — the agent's most powerful capability. The
tool delegates isolation/limits to Warden (tested) and security-gating to Edict
(tested), but it owns the **presentation** of a command's result: it combines
stdout then stderr, prepends a `[truncated to last 64 KiB]` marker when Warden
truncated the streams, and appends an `[exit code N]` suffix on a non-zero exit.
The existing tests all drive a **real** Warden running `echo`, which can't reliably
produce a truncated stream, a stderr+stdout pair, or a specific exit code — so this
output contract (what the agent actually sees, and what it keys its next action on)
was untested. A regression in the combination/markers would silently corrupt what
the model reads back from a command.

## What
Test-only, white-box (`package shell`). Added to `plugins/tools/shell/shell_test.go`:
- a `fakeWarden` implementing `warden.Engine` (Run/EffectiveProfile/SetBus) that
  returns a canned `warden.Result`, injected via `NewWithWarden`;
- **`TestShell_CombinesStdoutThenStderr`** — stdout `out-data` + stderr `err-data`
  → output is exactly `"out-data\nerr-data"` (stderr after stdout, newline-joined);
- **`TestShell_PrependsTruncationMarker`** — `Truncated: true` → output is prefixed
  with `[truncated to last 64 KiB]` and still carries the retained data;
- **`TestShell_NonzeroExitAppendsCode`** — `ExitCode: 3` → `IsError` set and the
  output includes the `[exit code 3]` suffix.

## Verification
- `go test ./plugins/tools/shell -run 'CombinesStdout|TruncationMarker|NonzeroExitAppends' -v`
  — all three pass.
- `gofmt -l` clean; `go vet ./plugins/tools/shell/` clean; `GOOS=linux go build
  ./...` exit 0. Full suite **2075** passing (was 2072; +3), `go test ./...` exit 0.
  `go.mod` / `go.sum` unchanged.

## Scope notes
- No production change — the formatting already worked; this pins the contract the
  model depends on. Combined with the existing run/missing-command/timeout/bad-JSON
  tests and the Warden capBuffer coverage (M338), the shell tool's behaviour is now
  covered end to end.
- The `file` tool (the other security-critical tool) was already exhaustively
  containment-tested (dotdot, absolute-outside, symlink-escape, new-file-under-
  symlinked-parent), so this milestone targeted the shell tool's one genuine gap.
