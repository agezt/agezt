# M256 — Bound the acpagent tool's in-memory answer accumulation

## Why
Continuing the audit into the last IPC surface, the `acpagent` tool (which
spawns an external ACP agent subprocess and relays its streamed answer) had a
latent memory-DoS. Its `MaxOutputBytes = 60 KiB` cap — whose own comment says it
exists "so a runaway agent can't blow the output" — was applied **after** full
accumulation: the chunk callback appended every streamed chunk into a
`strings.Builder`, and only `render` truncated it at the end. So an agent that
streamed without end grew the buffer without bound and could OOM the daemon
(killing every concurrent run) before the per-run timeout reaped the process.

The other teardown concerns on this tool were already handled: `close()` shuts
stdin, waits 5 s, then `Process.Kill()`s a stuck agent, and it's `defer`red in
`Invoke` so ctx cancellation reaps the subprocess too.

## What
- **`plugins/tools/acpagent/acpagent.go`** — the `Prompt` chunk callback now
  stops appending once the accumulation reaches `MaxOutputBytes`
  (`if answer.Len() >= MaxOutputBytes { return }`). Whole chunks are appended, so
  no UTF-8 rune is split, and the overshoot is at most one message. The relayed
  answer is unchanged because `render` already truncated to `MaxOutputBytes`.

## Files
- `plugins/tools/acpagent/acpagent.go` — cap in the chunk callback (edited).
- `plugins/tools/acpagent/bound_test.go` — `TestACPAgent_BoundsRunawayStream`:
  streams 200000 bytes (≫ the 60 KiB cap) and asserts the truncation footer
  reports only a small overshoot (≤ a couple of chunks) rather than the full
  ~140 KiB overflow it would without the cap (new).

## Verification
- `go test ./plugins/tools/acpagent/` — green (the test fails against the
  pre-fix code, where the footer reports ~138 KiB truncated); full suite
  **1828 → 1829** (+1), 66 packages, `go test ./...` exit 0.
- `gofmt -l` (CRLF-normalised) clean; `go vet ./plugins/tools/acpagent/` clean.
- `GOOS=linux go build -buildvcs=false ./...` clean; `go.mod` / `go.sum`
  unchanged.

## Scope notes
- Mirrors the codebase's existing defensive pattern — mcpbridge bounds external
  reads, the http/browser tools cap response bodies — applied to the one place
  that capped only after accumulating.
- The sibling `coding` tool truncates its captured diff/agent output to 60 KiB
  the same post-hoc way, but it reads from a finite `git diff` / a completed
  command rather than an open stream, so it isn't exposed to the same unbounded
  growth; left unchanged.
