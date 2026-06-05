# M433 — Plugin SDK: cap the inbound frame read

## Context
Review of the two remaining un-swept plugin surfaces (`plugins/sdk/sdk.go`,
`plugins/tools/peer/peer.go`) plus three previously-unreviewed agent tools
(acpagent, coding, notify).

**Reviewed CLEAN (no changes):**
- `plugins/tools/peer/peer.go` — the agent-supplied `peer` param is a map-key
  lookup into an operator-configured peer set (no agent-supplied URL → no SSRF);
  per-peer bearer token only ever sent to that peer's own URL (no cross-host
  leak), redacted in Describe, never in errors; 1 MiB `io.LimitReader` on both
  POST/GET, body closed, status checked; malformed peer JSON ignored into a
  zero-value struct (no panic); A→B→A federation loop bounded by an
  `X-Agezt-Mesh-Hop` counter clamped to [1,64]; context timeout propagated.
- `plugins/tools/coding/coding.go` — command operator-pinned, task via env var
  (no shell injection), writes only to `os.MkdirTemp` + a throwaway git worktree
  (no workspace-escape), cleanup on a fresh context, UTF-8-safe truncation.
- `plugins/tools/notify/notify.go` — recipients pinned to an operator allowlist
  (no SSRF/redirect), no URL/shell, RWMutex + new-map-on-Bind (race-safe),
  panic-safe, no token leakage.

## The bug
`plugins/sdk/sdk.go` (the read loop, formerly line 229): the SDK read every
inbound frame with `bufio.Reader.ReadBytes('\n')` and **no size cap**.
`ReadBytes`/`ReadSlice` accumulate into a single growing allocation until they
see a `\n`; a frame with no terminating newline — a corrupted pipe, or a partial
host write that never completes — makes the plugin allocate without bound until
it is OOM-killed. Every other newline-delimited reader in the codebase already
caps the frame: `kernel/plugin` (host side) at 16 MiB, `mcpbridge` at 16 MiB,
`kernel/acp` at 8 MiB. The SDK — the symmetric plugin side reading from the host
— was the lone gap.

A secondary interop wart (LOW): a stray blank line failed `json.Unmarshal` and
emitted a spurious `{"id":"","error":"bad request: unexpected end of JSON
input"}` frame with an empty id the host can't correlate.

## The fix
- Added `maxFrameBytes = 16 << 20` and a `readFrame(r, max)` helper mirroring
  `kernel/plugin.readFrame` exactly (`ReadSlice` in a loop, error once the
  accumulated bytes exceed `max`, `ErrBufferFull` continues). The read loop now
  calls `readFrame`; an over-cap frame is terminal (clean exit) — continuing
  would read from a desynced stream.
- Skip blank lines (`len(bytes.TrimSpace(line)) == 0`) before unmarshalling so a
  stray newline no longer produces a spurious empty-id error frame.

The fix is plugin-side, stdlib-only (B0 constraint preserved — `bytes` added, no
kernel import). 16 MiB is generous for legitimate JSON tool I/O; ordinary
frames are unaffected.

## Verification
- **`plugins/sdk/sdk_test.go`** (white-box, package `sdk`):
  - `TestReadFrame_CapsOversizedFrame`: a `max*4`-byte line with no newline
    returns an error reporting the cap (not unbounded growth).
  - `TestReadFrame_NormalFrameUnderCap`: an under-cap frame round-trips intact.
  - `TestServeRW_BlankLinesSkipped`: blank/whitespace lines before a shutdown
    produce zero output (no spurious error frames).
  - **Negative controls:** (1) neuter the cap (`&& false`) → CapsOversizedFrame
    FAILs (`got EOF`, the cap message is gone); (2) neuter the blank-line skip →
    BlankLinesSkipped FAILs (three empty-id error frames emitted). Both restored
    byte-identical.
- **Gate:** `gofmt -l` clean, `go vet` clean, `GOOS=linux go build ./...` ok,
  `go.mod`/`go.sum` unchanged. Full suite **2294** passing (was 2291; +3),
  `go test ./...` exit 0. CHANGELOG Reliability entry.

## Deferred review findings (documented, fixed separately / accepted)
- MED (acpagent): a context timeout/cancel does not unblock a parked stdout read
  — `exec.Command` (not `CommandContext`) and teardown only runs after `Prompt`
  returns, so a silent/wedged sub-agent can hang past the 5-min timeout.
  **Addressed in M434.**
- LOW (acpagent): `close()` blocks on `<-done` after `Kill()` with no bound —
  a child that never reaps pins the caller. (Folded into M434.)

## Review status
The plugin SDK and the peer/coding/notify tools are reviewed and sound. The one
remaining finding (acpagent cancellation liveness) is carried to M434.
